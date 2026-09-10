package omai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/tidwall/jsonc"
	"gopkg.in/yaml.v3"
)

type Document struct {
	Path, Format string
	Image        FileImage
	Map          map[string]any
	Body         string
}

func openDocument(path, format string) (*Document, error) {
	f, e := fileImage(path)
	if e != nil {
		return nil, e
	}
	if len(f.Data) > maxFileSize {
		return nil, fmt.Errorf("provider file exceeds 2 MiB: %s", path)
	}
	d := &Document{Path: path, Format: format, Image: f, Map: map[string]any{}}
	if !f.Exists || len(bytes.TrimSpace(f.Data)) == 0 {
		return d, nil
	}
	switch format {
	case "json":
		body := jsonc.ToJSON(f.Data)
		if !json.Valid(body) {
			return nil, fmt.Errorf("cannot parse %s; invalid JSON left untouched", path)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		e = decoder.Decode(&d.Map)
	case "toml":
		e = toml.Unmarshal(f.Data, &d.Map)
	case "agent":
		d.Map, d.Body, e = markdown(f.Data)
	case "instructions":
		d.Map, e = parseInstructions(f.Data)
	}
	if e != nil {
		return nil, fmt.Errorf("cannot parse %s; left untouched: %w", path, e)
	}
	if d.Map == nil {
		return nil, fmt.Errorf("%s must contain an object", path)
	}
	return d, nil
}
func markdown(b []byte) (map[string]any, string, error) {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	m := map[string]any{}
	if !strings.HasPrefix(s, "---\n") {
		return m, s, nil
	}
	idx := strings.Index(s[4:], "\n---\n")
	if idx < 0 {
		return nil, "", errors.New("unterminated YAML frontmatter")
	}
	idx += 4
	if e := yaml.Unmarshal([]byte(s[4:idx]), &m); e != nil {
		return nil, "", e
	}
	return m, strings.TrimPrefix(s[idx+5:], "\n"), nil
}
func parseAgent(b []byte) (Agent, error) {
	m, body, e := markdown(b)
	if e != nil {
		return Agent{}, e
	}
	for k := range m {
		if k != "description" {
			return Agent{}, fmt.Errorf("portable agent field %s is unsupported; use only description and body", k)
		}
	}
	desc, _ := m["description"].(string)
	if desc == "" {
		return Agent{}, errors.New("agent needs description frontmatter")
	}
	return Agent{desc, body}, nil
}
func agentMarkdown(a Agent) []byte {
	return []byte("---\ndescription: " + string(raw(a.Description)) + "\n---\n\n" + a.Prompt)
}

const rulePrefix = "<!-- omai:rule "

func parseInstructions(b []byte) (map[string]any, error) {
	s := string(b)
	m := map[string]any{}
	i := strings.Index(s, rulePrefix)
	if i < 0 {
		m["instructions"] = s
		return m, nil
	}
	m["instructions"] = strings.TrimSuffix(s[:i], "\n\n")
	for len(s[i:]) > 0 {
		tail := s[i:]
		end := strings.Index(tail, " -->\n")
		if !strings.HasPrefix(tail, rulePrefix) || end < 0 {
			return nil, errors.New("damaged omai rule markers")
		}
		name := tail[len(rulePrefix):end]
		if e := safeRel(name); e != nil {
			return nil, e
		}
		finish := "\n<!-- /omai:rule " + name + " -->"
		n := strings.Index(tail[end+5:], finish)
		if n < 0 {
			return nil, errors.New("damaged omai rule end marker")
		}
		content := tail[end+5 : end+5+n]
		m["rule:"+name] = content
		s = strings.TrimPrefix(tail[end+5+n+len(finish):], "\n\n")
		if strings.TrimSpace(s) == "" {
			break
		}
		i = 0
	}
	return m, nil
}
func (d *Document) encode() ([]byte, error) {
	switch d.Format {
	case "json":
		return jsonBytes(d.Map), nil
	case "toml":
		return toml.Marshal(d.Map)
	case "agent":
		b, e := yaml.Marshal(d.Map)
		if e != nil {
			return nil, e
		}
		return []byte("---\n" + string(b) + "---\n\n" + d.Body), nil
	case "instructions":
		s, _ := d.Map["instructions"].(string)
		for _, k := range sortedKeys(d.Map) {
			if strings.HasPrefix(k, "rule:") {
				name := strings.TrimPrefix(k, "rule:")
				content, _ := d.Map[k].(string)
				s += "\n\n" + rulePrefix + name + " -->\n" + content + "\n<!-- /omai:rule " + name + " -->"
			}
		}
		return []byte(s), nil
	}
	return nil, errors.New("unknown document format")
}
func getField(m map[string]any, path []string) (any, bool) {
	if len(path) == 0 {
		return m, true
	}
	cur := m
	for _, k := range path[:len(path)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	v, ok := cur[path[len(path)-1]]
	return v, ok
}
func setField(m map[string]any, path []string, v any, exists bool) error {
	cur := m
	for _, k := range path[:len(path)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			if _, present := cur[k]; present {
				return fmt.Errorf("cannot manage field inside non-object %s; existing value left untouched", k)
			}
			if !exists {
				return nil
			}
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	k := path[len(path)-1]
	if exists {
		cur[k] = v
	} else {
		delete(cur, k)
	}
	return nil
}
func nativeRoot(p Paths, provider string) string {
	switch provider {
	case "codex":
		return filepath.Join(p.Home, ".codex")
	case "claude":
		return filepath.Join(p.Home, ".claude")
	default:
		base := filepath.Dir(p.Config)
		return filepath.Join(base, "opencode")
	}
}
func configFile(p Paths, provider string) (string, string) {
	r := nativeRoot(p, provider)
	switch provider {
	case "codex":
		return filepath.Join(r, "config.toml"), "toml"
	case "claude":
		return filepath.Join(r, "settings.json"), "json"
	default:
		if _, e := os.Stat(filepath.Join(r, "opencode.jsonc")); e == nil {
			return filepath.Join(r, "opencode.jsonc"), "json"
		}
		return filepath.Join(r, "opencode.json"), "json"
	}
}
