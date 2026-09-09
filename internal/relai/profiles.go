package relai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Existing common entries stay shared. Newly discovered resources belong to
// their provider, so equally named skills or servers cannot overwrite another
// provider's configuration.
func discoveredKey(provider, key string, values Values, old map[string]Binding) string {
	if _, ok := values[key]; ok {
		return key
	}
	for _, b := range old {
		if b.Key == key {
			return key
		}
	}
	if strings.HasPrefix(key, "mcp/") {
		key += ".json"
	}
	return "providers/" + provider + "/" + key
}

func makeProviderBinding(p Paths, provider, key string, m Manifest) (Binding, bool) {
	parts := strings.SplitN(key, "/", 3)
	if len(parts) != 3 || parts[1] != provider || !allowedSource(key) {
		return Binding{}, false
	}
	local := parts[2]
	b := Binding{Provider: provider, Key: key, Kind: "file", Format: "file", Path: filepath.Join(nativeRoot(p, provider), local)}
	switch {
	case local == "instructions.md":
		name := "AGENTS.md"
		if provider == "claude" {
			name = "CLAUDE.md"
		}
		b.Path = filepath.Join(nativeRoot(p, provider), name)
	case strings.HasPrefix(local, "mcp/"):
		name := strings.TrimSuffix(strings.TrimPrefix(local, "mcp/"), ".json")
		b, _ = makeBinding(p, provider, "mcp/"+name, m)
		b.Key, b.Kind = key, "native-mcp"
	case provider == "codex" && strings.HasPrefix(local, "skills/"):
		b.Path = filepath.Join(p.Home, ".agents", local)
		legacy := filepath.Join(nativeRoot(p, provider), local)
		skill := strings.Split(local, "/")[1]
		if _, e := os.Lstat(filepath.Join(p.Home, ".agents/skills", skill)); os.IsNotExist(e) {
			if _, e := os.Lstat(legacy); e == nil {
				b.Path = legacy
			}
		}
	}
	if b.Kind == "file" {
		b.Linked = hasResourceLink(b.Path)
	}
	return b, true
}

func hasResourceLink(path string) bool {
	for path != string(filepath.Separator) {
		if st, e := os.Lstat(path); e == nil && st.Mode()&os.ModeSymlink != 0 {
			return true
		}
		path = filepath.Dir(path)
	}
	return false
}

func openBindingDocument(b Binding, protected []string) (*Document, error) {
	if !b.Linked {
		return openDocument(b.Path, b.Format)
	}
	// Dereference only discovered resource files, never configuration/state
	// stores. The resolved target is opened without further link traversal and
	// is read-only for Relai; source snapshots contain ordinary file contents.
	resolved, e := filepath.EvalSymlinks(b.Path)
	if e != nil {
		return nil, e
	}
	if e = safeRel(strings.TrimPrefix(resolved, "/")); e != nil {
		return nil, fmt.Errorf("linked resource targets an excluded location: %w", e)
	}
	for _, root := range protected {
		if resolved == root || strings.HasPrefix(resolved, root+"/") {
			return nil, errors.New("linked resources cannot target Relai's own data")
		}
	}
	d, e := openDocument(resolved, b.Format)
	if e == nil {
		d.Path = b.Path
	}
	return d, e
}

func discoverProviderResources(p Paths, provider, kind string, values Values, old map[string]Binding, keys map[string]bool, warnings *[]string) error {
	roots := []string{filepath.Join(nativeRoot(p, provider), kind)}
	if kind == "skills" && provider == "codex" {
		roots = []string{filepath.Join(p.Home, ".agents/skills"), filepath.Join(p.Home, ".codex/skills")}
	}
	seen := map[string]bool{}
	visited := 0
	for _, root := range roots {
		var walk func(string, string, map[string]bool) error
		walk = func(path, rel string, ancestors map[string]bool) error {
			visited++
			if visited > 8192 {
				return errors.New("resource discovery exceeds 8192 entries")
			}
			if rel != "" {
				if kind == "skills" && provider == "codex" && root == roots[len(roots)-1] {
					skill := strings.Split(filepath.ToSlash(rel), "/")[0]
					if _, e := os.Lstat(filepath.Join(roots[0], skill)); e == nil {
						return nil
					}
				}
				if strings.HasPrefix(filepath.Base(rel), ".") || safeRel(rel) != nil {
					return nil
				}
				if kind == "skills" && strings.HasPrefix(rel, "relai-command-") {
					return nil
				}
			}
			info, e := os.Stat(path)
			if os.IsNotExist(e) {
				return nil
			}
			if e != nil {
				*warnings = append(*warnings, provider+": unreadable "+kind+"/"+rel)
				return nil
			}
			if info.IsDir() {
				resolved, e := filepath.EvalSymlinks(path)
				if e != nil || ancestors[resolved] || len(ancestors) >= 32 {
					*warnings = append(*warnings, provider+": cyclic or deeply nested resource "+kind+"/"+rel)
					return nil
				}
				if safeRel(strings.TrimPrefix(resolved, "/")) != nil {
					*warnings = append(*warnings, provider+": linked resource targets an excluded location: "+kind+"/"+rel)
					return nil
				}
				for _, protected := range []string{p.Config, p.Data, p.State} {
					if resolved == protected || strings.HasPrefix(resolved, protected+"/") {
						*warnings = append(*warnings, provider+": linked resource targets Relai data: "+kind+"/"+rel)
						return nil
					}
				}
				ancestors[resolved] = true
				defer delete(ancestors, resolved)
				entries, e := os.ReadDir(path)
				if e != nil {
					return e
				}
				for _, entry := range entries {
					if e = walk(filepath.Join(path, entry.Name()), filepath.Join(rel, entry.Name()), ancestors); e != nil {
						return e
					}
				}
				return nil
			}
			if !info.Mode().IsRegular() || seen[rel] {
				return nil
			}
			seen[rel] = true
			key := discoveredKey(provider, kind+"/"+filepath.ToSlash(rel), values, old)
			if allowedSource(key) {
				keys[key] = true
			}
			return nil
		}
		if e := walk(root, "", map[string]bool{}); e != nil {
			return e
		}
	}
	return nil
}

func nativeMCP(provider string, data []byte) (map[string]any, error) {
	if !json.Valid(data) {
		return nil, errors.New("MCP declaration must be valid JSON")
	}
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if e := d.Decode(&m); e != nil || m == nil {
		return nil, errors.New("MCP declaration must be an object")
	}
	command := m["command"]
	endpoint, _ := m["url"].(string)
	if command == nil && endpoint == "" {
		return nil, errors.New("MCP declaration needs a command or URL")
	}
	if command != nil && endpoint != "" {
		return nil, errors.New("MCP declaration cannot combine command and URL")
	}
	if endpoint != "" {
		u, e := url.Parse(endpoint)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, errors.New("MCP URL must be HTTP(S) without credentials, query or fragment")
		}
	}
	if command != nil {
		if provider == "opencode" {
			args, ok := command.([]any)
			if !ok || len(args) == 0 {
				return nil, errors.New("OpenCode MCP command must be a nonempty string array")
			}
			for _, arg := range args {
				if _, ok := arg.(string); !ok {
					return nil, errors.New("MCP command arguments must be strings")
				}
			}
		} else if s, ok := command.(string); !ok || s == "" {
			return nil, errors.New("MCP command must be a string")
		}
	}
	if args, exists := m["args"]; exists {
		list, ok := args.([]any)
		if !ok {
			return nil, errors.New("MCP args must be an array")
		}
		for _, arg := range list {
			if _, ok := arg.(string); !ok {
				return nil, errors.New("MCP args must be strings")
			}
		}
	}
	for _, name := range []string{"args", "command"} {
		if args, ok := m[name].([]any); ok {
			for _, value := range args {
				arg, _ := value.(string)
				for _, flag := range []string{"--token", "--api-key", "--apikey", "--password", "--secret", "--authorization"} {
					if strings.ToLower(arg) == flag || strings.HasPrefix(strings.ToLower(arg), flag+"=") {
						return nil, errors.New("credential arguments are blocked; use MCP environment references")
					}
				}
			}
		}
	}
	if e := scanContent("MCP declaration", data); e != nil {
		return nil, e
	}
	return m, nil
}

func validateProfileFile(key string, data []byte) error {
	if key != "providers/codex/hooks.json" && key != "providers/opencode/tui.json" {
		return nil
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil || object == nil {
		return errors.New("provider configuration must be a JSON object")
	}
	return nil
}

// Split nested native options into transportable configuration and private
// literals. Private values are neither canonical values nor deletion targets.
func splitMCPFields(m map[string]any) (map[string]any, map[string]any) {
	public, private := map[string]any{}, map[string]any{}
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			pub, priv := splitMCPFields(nested)
			if len(pub) > 0 || len(nested) == 0 {
				public[k] = pub
			}
			if len(priv) > 0 {
				private[k] = priv
			}
		} else if scanContent("MCP option", raw(map[string]any{k: v})) != nil {
			private[k] = v
		} else {
			public[k] = v
		}
	}
	return public, private
}

func mergePrivateMCP(next, private map[string]any) {
	for k, v := range private {
		if _, ok := next[k]; !ok {
			next[k] = v
		} else if nested, ok := next[k].(map[string]any); ok {
			if child, ok := v.(map[string]any); ok {
				mergePrivateMCP(nested, child)
			}
		}
	}
}
