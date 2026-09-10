package omai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
)

const Version = "1.0.0"

var Providers = []string{"codex", "claude", "opencode"}

type Paths struct{ Home, Config, Data, State, Source string }

func (p Paths) validatePaths() error {
	for _, path := range []string{p.Home, p.Config, p.Data, p.State, p.Source} {
		if !filepath.IsAbs(path) {
			return errors.New("omai requires absolute HOME and XDG paths")
		}
	}
	return nil
}

func NewPaths(home string) Paths {
	p := Paths{Home: home}
	base := func(env, fallback string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		return filepath.Join(home, fallback)
	}
	p.Config = filepath.Join(base("XDG_CONFIG_HOME", ".config"), "omai")
	p.Data = filepath.Join(base("XDG_DATA_HOME", ".local/share"), "omai")
	p.State = filepath.Join(base("XDG_STATE_HOME", ".local/state"), "omai")
	p.Source = filepath.Join(p.Config, "source")
	return p
}

// FakePaths deliberately ignores the process's XDG/provider overrides.
func FakePaths(home string) Paths {
	return Paths{home, filepath.Join(home, ".config/omai"), filepath.Join(home, ".local/share/omai"), filepath.Join(home, ".local/state/omai"), filepath.Join(home, ".config/omai/source")}
}

type Config struct {
	Retention   Retention `json:"retention,omitempty"`
	Version     int       `json:"version"`
	Remote      string    `json:"remote,omitempty"`
	Branch      string    `json:"branch"`
	Providers   []string  `json:"providers,omitempty"`
	Machine     string    `json:"machine"`
	Peers       []string  `json:"peers,omitempty"`
	PollSeconds int       `json:"poll_seconds"`
	SyncSeconds int       `json:"sync_seconds"`
}
type Personal struct {
	Path    string `json:"path"`
	Source  string `json:"source"`
	Deleted bool   `json:"deleted,omitempty"`
}
type Manifest struct {
	Version  int                       `json:"version"`
	Settings map[string]map[string]any `json:"settings"`
	MCP      map[string]Server         `json:"mcp"`
	Hooks    map[string]map[string]any `json:"hooks"`
	Plugins  map[string]map[string]any `json:"plugins"`
	Personal map[string]Personal       `json:"personal"`
}
type Server struct {
	Command   string   `json:"command,omitempty"`
	Args      []string `json:"args,omitempty"`
	URL       string   `json:"url,omitempty"`
	Env       []string `json:"env,omitempty"`
	BearerEnv string   `json:"bearer_env,omitempty"`
}
type Agent struct {
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}
type Blob struct {
	Data []byte `json:"data"`
	Mode uint32 `json:"mode"`
}
type Tree map[string]Blob
type Values map[string]json.RawMessage

func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
func same(a, b json.RawMessage) bool {
	var av, bv any
	if len(a) > 0 {
		if json.Unmarshal(a, &av) != nil {
			return false
		}
	}
	if len(b) > 0 {
		if json.Unmarshal(b, &bv) != nil {
			return false
		}
	}
	return reflect.DeepEqual(av, bv)
}
func textValue(v json.RawMessage) string { var s string; _ = json.Unmarshal(v, &s); return s }
func object(v json.RawMessage) map[string]any {
	m := map[string]any{}
	_ = json.Unmarshal(v, &m)
	return m
}
func sortedKeys[M ~map[string]V, V any](m M) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}
func jsonBytes(v any) []byte { b, _ := json.MarshalIndent(v, "", "  "); return append(b, '\n') }
func readJSON(path string, v any) error {
	b, e := readRegularLimit(path, maxInternalFileSize)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func defaultManifest() Manifest {
	return Manifest{Version: 1, Settings: map[string]map[string]any{}, MCP: map[string]Server{}, Hooks: map[string]map[string]any{}, Plugins: map[string]map[string]any{}, Personal: map[string]Personal{}}
}
func (p Paths) LoadConfig() (Config, error) {
	var c Config
	if e := p.validatePaths(); e != nil {
		return c, e
	}
	e := readJSON(filepath.Join(p.Config, "config.json"), &c)
	if e != nil {
		return c, e
	}
	if c.Version != 1 {
		return c, errors.New("unsupported local config version")
	}
	if e := c.Retention.validate(); e != nil {
		return c, e
	}
	if e := validRemote(c.Remote); e != nil {
		return c, e
	}
	if c.Branch == "" {
		c.Branch = "main"
	}
	if e := validBranch(c.Branch); e != nil {
		return c, e
	}
	if c.PollSeconds > 86400 || c.SyncSeconds > 86400 {
		return c, errors.New("polling intervals must not exceed 86400 seconds")
	}
	if c.PollSeconds < 1 {
		c.PollSeconds = 2
	}
	if c.SyncSeconds < 1 {
		c.SyncSeconds = 30
	}
	for _, x := range c.Providers {
		if !isProvider(x) {
			return c, fmt.Errorf("unknown provider %q", x)
		}
	}
	return c, nil
}
func isProvider(s string) bool {
	for _, p := range Providers {
		if p == s {
			return true
		}
	}
	return false
}

// Source directories are an explicit allowlist. Nothing else can enter Git.
func allowedSource(path string) bool {
	if path == "omai.json" || path == "instructions.md" {
		return true
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return false
	}
	switch parts[0] {
	case "providers":
		if len(parts) < 3 || !isProvider(parts[1]) {
			return false
		}
		rest := strings.Join(parts[2:], "/")
		if (parts[1] == "codex" && rest == "hooks.json") || (parts[1] == "opencode" && rest == "tui.json") {
			return true
		}
		if rest == "instructions.md" || (parts[1] == "codex" && rest == "AGENTS.override.md") {
			return true
		}
		if len(parts) >= 5 && parts[2] == "skills" {
			return true
		}
		if len(parts) == 4 && parts[2] == "mcp" && strings.HasSuffix(rest, ".json") {
			return validName(strings.TrimSuffix(parts[3], ".json")) == nil
		}
		return len(parts) >= 4 && parts[2] == "rules" && (strings.HasSuffix(rest, ".md") || (parts[1] == "codex" && strings.HasSuffix(rest, ".rules")))
	case "skills":
		return len(parts) >= 3
	case "rules", "agents", "commands":
		return len(parts) == 2 && strings.HasSuffix(path, ".md")
	case "hooks":
		return len(parts) >= 3 && parts[1] == "opencode" && strings.HasSuffix(path, ".js")
	case "personal":
		return len(parts) >= 2
	}
	return false
}
func loadTree(dir string) (Tree, error) {
	t := Tree{}
	e := filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == dir {
			return noSymlink(path)
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(filepath.Base(rel), ".omai-") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e := safeRel(rel); e != nil {
			return e
		}
		if e := noSymlink(path); e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if !allowedSource(rel) {
			return fmt.Errorf("unapproved source file %s", rel)
		}
		b, e := readRegular(path)
		if e != nil {
			return e
		}
		if e = scanContent(rel, b); e != nil {
			return e
		}
		s, e := d.Info()
		if e != nil {
			return e
		}
		mode := uint32(0600)
		if s.Mode()&0111 != 0 {
			mode = 0700
		}
		t[rel] = Blob{b, mode}
		return nil
	})
	return t, e
}
func parseSource(t Tree) (Manifest, Values, error) {
	m := defaultManifest()
	v := Values{}
	total := 0
	for path, blob := range t {
		if e := safeRel(path); e != nil {
			return m, v, e
		}
		if !allowedSource(path) {
			return m, v, fmt.Errorf("unapproved source path %s", path)
		}
		if e := scanContent(path, blob.Data); e != nil {
			return m, v, e
		}
		if e := validateProfileFile(path, blob.Data); e != nil {
			return m, v, fmt.Errorf("%s: %w", path, e)
		}
		parts := strings.Split(path, "/")
		if len(parts) == 4 && parts[0] == "providers" && parts[2] == "mcp" {
			if _, e := nativeMCP(parts[1], blob.Data); e != nil {
				return m, v, fmt.Errorf("%s: %w", path, e)
			}
		}
		total += len(blob.Data)
	}
	if total > maxSourceSize || len(t) > 2048 {
		return m, v, errors.New("source exceeds 16 MiB or 2048 files")
	}

	b, ok := t["omai.json"]
	if !ok {
		return m, v, errors.New("source/omai.json is missing")
	}
	dec := json.NewDecoder(bytes.NewReader(b.Data))
	dec.DisallowUnknownFields()
	if e := dec.Decode(&m); e != nil {
		return m, v, fmt.Errorf("omai.json: %w", e)
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return m, v, errors.New("omai.json must contain exactly one JSON object")
	}
	if m.Version != 1 {
		return m, v, errors.New("unsupported source version")
	}
	for p, settings := range m.Settings {
		if !isProvider(p) {
			return m, v, fmt.Errorf("unknown provider %s", p)
		}
		for k, x := range settings {
			if e := validateSetting(p, k, x); e != nil {
				return m, v, e
			}
			v["settings/"+p+"/"+k] = raw(x)
		}
	}
	for name, s := range m.MCP {
		if e := validName(name); e != nil {
			return m, v, e
		}
		if e := validateServer(s); e != nil {
			return m, v, fmt.Errorf("mcp %s: %w", name, e)
		}
		sort.Strings(s.Env)
		s.Env = slices.Compact(s.Env)
		v["mcp/"+name] = raw(s)
	}
	for p, events := range m.Hooks {
		if p != "claude" && p != "codex" {
			return m, v, fmt.Errorf("%s hooks: use hooks/opencode/*.js for OpenCode", p)
		}
		for name, x := range events {
			if e := validateHook(p, name, x); e != nil {
				return m, v, e
			}
			v["hooks/"+p+"/"+name] = raw(x)
		}
	}
	for p, plugins := range m.Plugins {
		if !isProvider(p) {
			return m, v, fmt.Errorf("unknown plugin provider %s", p)
		}
		for name, x := range plugins {
			if strings.ContainsAny(name, "\x00\n") {
				return m, v, errors.New("invalid plugin name")
			}
			if e := validatePlugin(p, name, x); e != nil {
				return m, v, e
			}
			v["plugins/"+p+"/"+name] = raw(x)
		}
	}
	for _, path := range sortedKeys(t) {
		if path == "omai.json" {
			continue
		}
		b := t[path]
		if e := scanContent(path, b.Data); e != nil {
			return m, v, e
		}
		if strings.HasPrefix(path, "agents/") {
			a, e := parseAgent(b.Data)
			if e != nil {
				return m, v, fmt.Errorf("%s: %w", path, e)
			}
			v[path] = raw(a)
		} else {
			v[path] = raw(b)
		}
	}
	destinations := map[string]bool{}
	sources := map[string]bool{}
	for name, f := range m.Personal {
		if e := validName(name); e != nil {
			return m, v, e
		}
		if !strings.HasPrefix(f.Source, "personal/") {
			return m, v, errors.New("personal source must be under personal/")
		}
		if e := safePersonal(f.Path); e != nil {
			return m, v, e
		}
		dest := strings.TrimPrefix(f.Path, "~/")
		if destinations[dest] || sources[f.Source] {
			return m, v, errors.New("personal files require distinct destinations and sources")
		}
		destinations[dest] = true
		sources[f.Source] = true
		if e := safeRel(f.Source); e != nil {
			return m, v, e
		}
		_, present := t[f.Source]
		f.Deleted = !present
		m.Personal[name] = f
	}
	for path := range t {
		if strings.HasPrefix(path, "personal/") && !sources[path] {
			return m, v, fmt.Errorf("personal source %s has no explicit mapping", path)
		}
	}
	return m, v, nil
}
func sourceWithValues(t Tree, m Manifest, v Values) (Tree, error) {
	out := Tree{}
	m.Settings = map[string]map[string]any{}
	m.MCP = map[string]Server{}
	m.Hooks = map[string]map[string]any{}
	m.Plugins = map[string]map[string]any{}
	for name, f := range m.Personal {
		f.Deleted = v[f.Source] == nil
		m.Personal[name] = f
	}
	for k, b := range v {
		parts := strings.SplitN(k, "/", 3)
		var dst map[string]map[string]any
		switch parts[0] {
		case "settings":
			dst = m.Settings
		case "hooks":
			if len(parts) > 1 && parts[1] != "opencode" {
				dst = m.Hooks
			}
		case "plugins":
			dst = m.Plugins
		}
		if dst != nil {
			if len(parts) != 3 {
				return nil, errors.New("invalid value key")
			}
			if dst[parts[1]] == nil {
				dst[parts[1]] = map[string]any{}
			}
			var x any
			if e := json.Unmarshal(b, &x); e != nil {
				return nil, e
			}
			dst[parts[1]][parts[2]] = x
			continue
		}
		if parts[0] == "mcp" {
			var s Server
			if e := json.Unmarshal(b, &s); e != nil {
				return nil, e
			}
			m.MCP[strings.TrimPrefix(k, "mcp/")] = s
			continue
		}
		if parts[0] == "agents" {
			var a Agent
			if e := json.Unmarshal(b, &a); e != nil {
				return nil, e
			}
			out[k] = Blob{agentMarkdown(a), 0600}
			if old, ok := t[k]; ok {
				prior, err := parseAgent(old.Data)
				if err == nil && same(raw(prior), raw(a)) {
					out[k] = old
				}
			}
			continue
		}
		var f Blob
		if e := json.Unmarshal(b, &f); e != nil {
			return nil, e
		}
		out[k] = f
	}
	out["omai.json"] = Blob{jsonBytes(m), 0600}
	// Keep original formatting when the manifest's meaning did not change.
	var old any
	var fresh any
	_ = json.Unmarshal(t["omai.json"].Data, &old)
	_ = json.Unmarshal(out["omai.json"].Data, &fresh)
	if same(raw(old), raw(fresh)) {
		out["omai.json"] = t["omai.json"]
	}
	if _, _, e := parseSource(out); e != nil {
		return nil, e
	}
	return out, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	for _, v := range values {
		seen[v] = true
	}
	return sortedKeys(seen)
}
