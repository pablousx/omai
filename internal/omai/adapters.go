package omai

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Binding struct {
	Provider, Key, Path, Format, Kind string
	Field                             []string
	Linked                            bool `json:",omitempty"`
}

func (b Binding) ID() string { return b.Provider + ":" + b.Key }

type AdapterSet struct {
	Bindings       map[string]Binding
	Docs           map[string]*Document
	Warnings       []string
	ProtectedRoots []string
}

func (a *AdapterSet) doc(b Binding) (*Document, error) {
	if d := a.Docs[b.Path]; d != nil {
		return d, nil
	}
	d, e := openBindingDocument(b, a.ProtectedRoots)
	if e != nil {
		return nil, e
	}
	a.Docs[b.Path] = d
	return d, nil
}
func detect(p Paths, c Config) []string {
	if len(c.Providers) > 0 {
		return append([]string{}, c.Providers...)
	}
	out := []string{}
	for _, name := range Providers {
		_, bin := exec.LookPath(name)
		r := nativeRoot(p, name)
		_, dir := os.Stat(r)
		if bin == nil || dir == nil {
			out = append(out, name)
		}
	}
	return out
}
func makeBinding(p Paths, provider, key string, m Manifest) (Binding, bool) {
	b := Binding{Provider: provider, Key: key}
	root := nativeRoot(p, provider)
	b.Path, b.Format = configFile(p, provider)
	parts := strings.SplitN(key, "/", 3)
	if provider == "personal" && parts[0] != "personal" {
		return b, false
	}
	switch parts[0] {
	case "providers":
		return makeProviderBinding(p, provider, key, m)
	case "settings":
		if len(parts) != 3 || parts[1] != provider {
			return b, false
		}
		b.Kind = "field"
		b.Field = strings.Split(parts[2], ".")
	case "mcp":
		b.Kind = "mcp"
		name := strings.TrimPrefix(key, "mcp/")
		switch provider {
		case "codex":
			b.Field = []string{"mcp_servers", name}
		case "claude":
			b.Path = filepath.Join(p.Home, ".claude.json")
			b.Field = []string{"mcpServers", name}
		case "opencode":
			b.Field = []string{"mcp", name}
		}
	case "hooks":
		if len(parts) == 3 && parts[1] == "opencode" {
			if provider != "opencode" {
				return b, false
			}
			b.Kind = "file"
			b.Format = "file"
			b.Path = filepath.Join(root, "plugins", parts[2])
			break
		}
		if len(parts) != 3 || parts[1] != provider {
			return b, false
		}
		b.Kind = "field"
		b.Field = []string{"hooks", parts[2]}
	case "plugins":
		if len(parts) != 3 || parts[1] != provider {
			return b, false
		}
		b.Kind = "field"
		switch provider {
		case "codex":
			b.Kind = "plugin"
			b.Field = []string{"plugins", parts[2]}
		case "claude":
			b.Field = []string{"enabledPlugins", parts[2]}
		case "opencode":
			b.Kind = "plugin-list"
			b.Field = []string{"plugin"}
		}
	case "instructions.md":
		b.Kind = "instructions"
		b.Format = "instructions"
		b.Field = []string{"instructions"}
		name := "AGENTS.md"
		if provider == "claude" {
			name = "CLAUDE.md"
		}
		b.Path = filepath.Join(root, name)
	case "rules":
		if provider == "claude" {
			b.Kind = "file"
			b.Format = "file"
			b.Path = filepath.Join(root, key)
		} else {
			b.Kind = "instructions"
			b.Format = "instructions"
			b.Path = filepath.Join(root, "AGENTS.md")
			b.Field = []string{"rule:" + strings.TrimPrefix(key, "rules/")}
		}
	case "skills":
		b.Kind = "file"
		b.Format = "file"
		b.Path = filepath.Join(root, key)
		if provider == "codex" {
			b.Path = filepath.Join(p.Home, ".agents", key)
		}
	case "agents":
		b.Kind = "agent"
		b.Format = "agent"
		b.Path = filepath.Join(root, key)
		if provider == "codex" {
			b.Format = "toml"
			b.Path = strings.TrimSuffix(b.Path, ".md") + ".toml"
		}
	case "commands":
		b.Kind = "file"
		b.Format = "file"
		b.Path = filepath.Join(root, key)
		if provider == "codex" {
			b.Kind = "command-skill"
			name := strings.TrimSuffix(strings.TrimPrefix(key, "commands/"), ".md")
			b.Path = filepath.Join(p.Home, ".agents/skills", "omai-command-"+name, "SKILL.md")
		}
	case "personal":
		if provider != "personal" {
			return b, false
		}
		for _, f := range m.Personal {
			if f.Source == key {
				b.Kind = "file"
				b.Format = "file"
				b.Path = filepath.Join(p.Home, strings.TrimPrefix(f.Path, "~/"))
				return b, true
			}
		}
		return b, false
	default:
		return b, false
	}
	if b.Kind == "file" || b.Kind == "instructions" {
		b.Linked = hasResourceLink(b.Path)
	}
	return b, true
}
func buildAdapters(p Paths, c Config, m Manifest, v Values, old map[string]Binding) (*AdapterSet, error) {
	a := &AdapterSet{Bindings: map[string]Binding{}, Docs: map[string]*Document{}, ProtectedRoots: []string{p.Config, p.Data, p.State}}
	for _, f := range m.Personal {
		target := filepath.Join(p.Home, strings.TrimPrefix(f.Path, "~/"))
		for _, protected := range []string{p.Config, p.Data, p.State, nativeRoot(p, "codex"), nativeRoot(p, "claude"), nativeRoot(p, "opencode"), filepath.Join(p.Home, ".agents")} {
			if target == protected || strings.HasPrefix(target, protected+string(filepath.Separator)) {
				return nil, errors.New("personal destination overlaps omai or a provider root")
			}
		}
	}
	providers := detect(p, c)
	keys := map[string]bool{}
	for k := range v {
		keys[k] = true
	}
	for _, b := range old {
		keys[b.Key] = true
	}
	// Discover only documented global config surfaces. Never traverse provider data directories.
	for _, provider := range providers {
		file, format := configFile(p, provider)
		d, e := openDocument(file, format)
		if e != nil {
			return nil, e
		}
		a.Docs[file] = d
		for k := range discoveredSettings(provider, d.Map) {
			if safeSetting(provider, k) {
				keys["settings/"+provider+"/"+k] = true
			} else if k != "mcp_servers" && k != "mcp" && k != "hooks" && k != "plugins" && k != "enabledPlugins" && k != "plugin" && k != "$schema" {
				a.Warnings = append(a.Warnings, provider+": unmanaged setting "+k+" stays local")
			}
		}
		mcpKey := "mcp_servers"
		if provider == "opencode" {
			mcpKey = "mcp"
		}
		md := d
		if provider == "claude" {
			mcpKey = "mcpServers"
			path := filepath.Join(p.Home, ".claude.json")
			md, e = openDocument(path, "json")
			if e != nil {
				return nil, e
			}
			a.Docs[path] = md
		}
		if servers, ok := md.Map[mcpKey].(map[string]any); ok {
			for name := range servers {
				if validName(name) == nil {
					key := discoveredKey(provider, "mcp/"+name, v, old)
					keys[key] = true
				}
			}
		}
		if hooks, ok := d.Map["hooks"].(map[string]any); ok && provider != "opencode" {
			for name, x := range hooks {
				if validateHook(provider, name, x) == nil {
					keys["hooks/"+provider+"/"+name] = true
				} else {
					a.Warnings = append(a.Warnings, provider+": unsupported or unsafe hook "+name+" left local")
				}
			}
		}
		pk := "plugins"
		if provider == "claude" {
			pk = "enabledPlugins"
		}
		if entries, ok := d.Map[pk].(map[string]any); ok {
			for name, x := range entries {
				if provider == "codex" {
					if mm, ok := x.(map[string]any); ok {
						x = map[string]any{"enabled": mm["enabled"]}
					}
				}
				if validatePlugin(provider, name, x) == nil {
					keys["plugins/"+provider+"/"+name] = true
				}
			}
		}
		if provider == "opencode" {
			if entries, ok := d.Map["plugin"].([]any); ok {
				for _, x := range entries {
					if s, ok := x.(string); ok && !strings.HasPrefix(s, "file:") && !filepath.IsAbs(s) {
						keys["plugins/opencode/"+s] = true
					}
				}
			}
		}
		root := nativeRoot(p, provider)
		auxiliary := ""
		if provider == "codex" {
			auxiliary = "hooks.json"
		}
		if provider == "opencode" {
			auxiliary = "tui.json"
		}
		if auxiliary != "" {
			if _, e := os.Lstat(filepath.Join(root, auxiliary)); e == nil {
				keys["providers/"+provider+"/"+auxiliary] = true
			}
		}
		if provider == "opencode" {
			entries, e := os.ReadDir(filepath.Join(root, "plugins"))
			if e != nil && !os.IsNotExist(e) {
				return nil, e
			}
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".js") && safeRel(entry.Name()) == nil {
					keys["hooks/opencode/"+entry.Name()] = true
				}
			}
		}
		inst := "AGENTS.md"
		if provider == "claude" {
			inst = "CLAUDE.md"
		}
		if _, e := os.Lstat(filepath.Join(root, inst)); e == nil {
			keys[discoveredKey(provider, "instructions.md", v, old)] = true
		}
		if provider == "codex" {
			if _, e := os.Lstat(filepath.Join(root, "AGENTS.override.md")); e == nil {
				keys["providers/codex/AGENTS.override.md"] = true
			}
		}
		for _, kind := range []string{"skills", "rules", "agents", "commands"} {
			if kind == "skills" || kind == "rules" {
				if e := discoverProviderResources(p, provider, kind, v, old, keys, &a.Warnings); e != nil {
					return nil, e
				}
				continue
			}
			if kind == "rules" && provider != "claude" {
				continue
			}
			if kind == "commands" && provider == "codex" {
				continue
			}
			dir := filepath.Join(root, kind)
			if kind == "skills" && provider == "codex" {
				dir = filepath.Join(p.Home, ".agents/skills")
			}
			e := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if os.IsNotExist(err) {
					return nil
				}
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(dir, path)
				if rel == "." {
					return noSymlink(path)
				}
				if d.Type()&os.ModeSymlink != 0 {
					a.Warnings = append(a.Warnings, provider+": symlink resource skipped: "+kind+"/"+rel)
					return nil
				}
				if kind == "skills" && strings.HasPrefix(rel, "omai-command-") {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if e := safeRel(filepath.ToSlash(rel)); e != nil {
					a.Warnings = append(a.Warnings, provider+": excluded resource "+kind+"/"+rel)
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
				if d.IsDir() {
					return nil
				}
				rel = filepath.ToSlash(rel)
				if kind != "skills" && (strings.Contains(rel, "/") || (!strings.HasSuffix(rel, ".md") && !(kind == "agents" && provider == "codex" && strings.HasSuffix(rel, ".toml")))) {
					return nil
				}
				if kind == "agents" && provider == "codex" {
					rel = strings.TrimSuffix(rel, ".toml") + ".md"
				}
				key := kind + "/" + rel
				if allowedSource(key) {
					keys[key] = true
				}
				return nil
			})
			if e != nil {
				return nil, e
			}
		}
	}
	// Personal destinations are explicitly enrolled in the canonical manifest.
	for _, f := range m.Personal {
		keys[f.Source] = true
	}
	for _, key := range sortedKeys(keys) {
		for _, provider := range append(providers, "personal") {
			b, ok := makeBinding(p, provider, key, m)
			if ok {
				if prior, owned := old[b.ID()]; owned && b.Linked && !prior.Linked {
					return nil, fmt.Errorf("managed resource became a symlink: %s", b.Key)
				}
				a.Bindings[b.ID()] = b
			}
		}
	}
	// Retain old personal destinations long enough to observe a removed enrollment,
	// but never delete them: stopping management leaves the personal file intact.
	claimed := map[string]string{}
	wholeFiles := map[string]string{}
	for _, b := range a.Bindings {
		if b.Kind == "file" {
			wholeFiles[b.Path] = b.Key
		}
	}
	for _, b := range a.Bindings {
		if owner, exists := wholeFiles[b.Path]; exists && owner != b.Key {
			return nil, fmt.Errorf("overlapping shared and provider-specific entries: %s and %s", owner, b.Key)
		}
		if b.Kind == "plugin-list" {
			continue // Each declaration owns one item of the native package list.
		}
		slot := b.Path + "\x00" + strings.Join(b.Field, "\x00")
		if oldKey, exists := claimed[slot]; exists && oldKey != b.Key {
			return nil, fmt.Errorf("shared and provider-specific entries target the same setting: %s and %s", oldKey, b.Key)
		}
		claimed[slot] = b.Key
	}
	return a, nil
}
func blobValue(data []byte, mode uint32) json.RawMessage {
	m := uint32(0600)
	if mode&0111 != 0 {
		m = 0700
	}
	return raw(Blob{data, m})
}
func (a *AdapterSet) read(b Binding) (json.RawMessage, error) {
	d, e := a.doc(b)
	if e != nil {
		return nil, e
	}
	if b.Kind == "file" || b.Kind == "command-skill" {
		if !d.Image.Exists {
			return nil, nil
		}
		data := d.Image.Data
		if b.Kind == "command-skill" {
			prefix := commandPrefix(b.Key)
			if !strings.HasPrefix(string(data), prefix) {
				return nil, errors.New("Codex command skill header changed; edit the command body or canonical command")
			}
			data = data[len(prefix):]
		}
		if e := scanContent(b.Key, data); e != nil {
			return nil, e
		}
		if e := validateProfileFile(b.Key, data); e != nil {
			return nil, e
		}
		return blobValue(data, d.Image.Mode), nil
	}
	if b.Kind == "agent" {
		if !d.Image.Exists {
			return nil, nil
		}
		desc, _ := d.Map["description"].(string)
		prompt := d.Body
		if b.Provider == "codex" {
			prompt, _ = d.Map["developer_instructions"].(string)
		}
		if desc == "" {
			return nil, errors.New("agent without a description is unsupported")
		}
		v := raw(Agent{desc, prompt})
		if e := scanContent(b.Key, v); e != nil {
			return nil, e
		}
		return v, nil
	}
	x, ok := getField(d.Map, b.Field)
	if b.Kind == "plugin-list" {
		if !ok {
			return nil, nil
		}
		list, ok := x.([]any)
		if !ok {
			return nil, errors.New("OpenCode plugin must be an array")
		}
		name := strings.TrimPrefix(b.Key, "plugins/opencode/")
		for _, n := range list {
			if n == name {
				return raw(true), nil
			}
		}
		return nil, nil
	}
	if !ok {
		return nil, nil
	}
	if strings.HasPrefix(b.Key, "settings/") {
		if e := validateSetting(b.Provider, strings.Join(b.Field, "."), x); e != nil {
			return nil, e
		}
	}
	if b.Kind == "instructions" {
		s, ok := x.(string)
		if !ok {
			return nil, errors.New("invalid instructions")
		}
		if s == "" && !d.Image.Exists {
			return nil, nil
		}
		if e := scanContent(b.Key, []byte(s)); e != nil {
			return nil, e
		}
		return blobValue([]byte(s), 0600), nil
	}
	if b.Kind == "mcp" {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, errors.New("unsupported MCP declaration")
		}
		s, e := decodeServer(b.Provider, m)
		if e != nil {
			return nil, e
		}
		return raw(s), nil
	}
	if b.Kind == "native-mcp" {
		fields, ok := x.(map[string]any)
		if !ok {
			return nil, errors.New("MCP declaration must be an object")
		}
		public, private := splitMCPFields(fields)
		if len(private) > 0 {
			a.Warnings = append(a.Warnings, b.Provider+": private MCP options kept local: "+b.Key)
		}
		data := jsonBytes(public)
		if _, e := nativeMCP(b.Provider, data); e != nil {
			return nil, e
		}
		return blobValue(data, 0600), nil
	}
	if b.Kind == "plugin" {
		m, ok := x.(map[string]any)
		if !ok {
			return nil, errors.New("unsupported plugin declaration")
		}
		x = map[string]any{"enabled": m["enabled"]}
	}
	v := raw(x)
	if e := scanContent(b.Key, v); e != nil {
		return nil, e
	}
	return v, nil
}
func commandPrefix(key string) string {
	name := strings.TrimSuffix(strings.TrimPrefix(key, "commands/"), ".md")
	return "---\nname: omai-command-" + name + "\ndescription: " + string(raw("Personal command: "+name)) + "\ndisable-model-invocation: true\n---\n\n"
}
func (a *AdapterSet) write(b Binding, v json.RawMessage) error {
	if b.Linked {
		return fmt.Errorf("%s is supplied by a local symlink; its target is kept unchanged", b.Key)
	}
	d, e := a.doc(b)
	if e != nil {
		return e
	}
	if b.Kind == "file" || b.Kind == "command-skill" {
		if v == nil {
			d.Image = FileImage{}
			return nil
		}
		var f Blob
		if e = json.Unmarshal(v, &f); e != nil {
			return e
		}
		if b.Kind == "command-skill" {
			f.Data = append([]byte(commandPrefix(b.Key)), f.Data...)
		}
		mode := uint32(0600)
		if d.Image.Exists {
			mode = d.Image.Mode
		}
		if f.Mode&0100 != 0 {
			mode |= 0100
		} else {
			mode &^= 0111
		}
		d.Image = FileImage{true, f.Data, mode}
		return nil
	}
	if b.Kind == "agent" {
		if v == nil {
			d.Image = FileImage{}
			d.Map = nil
			return nil
		}
		var ag Agent
		if e = json.Unmarshal(v, &ag); e != nil {
			return e
		}
		d.Map["description"] = ag.Description
		if b.Provider == "codex" {
			d.Map["name"] = strings.TrimSuffix(filepath.Base(b.Key), ".md")
			d.Map["developer_instructions"] = ag.Prompt
		} else {
			d.Body = ag.Prompt
			if b.Provider == "opencode" {
				if _, ok := d.Map["mode"]; !ok {
					d.Map["mode"] = "subagent"
				}
			}
		}
		d.Image.Exists = true
		return nil
	}
	if b.Kind == "plugin-list" {
		list, _ := d.Map["plugin"].([]any)
		name := strings.TrimPrefix(b.Key, "plugins/opencode/")
		out := []any{}
		for _, x := range list {
			if x != name {
				out = append(out, x)
			}
		}
		if v != nil && string(v) == "true" {
			out = append(out, name)
		}
		d.Map["plugin"] = out
		d.Image.Exists = true
		return nil
	}
	var x any
	if v != nil {
		if e = json.Unmarshal(v, &x); e != nil {
			return e
		}
	}
	if b.Kind == "instructions" && v != nil {
		var f Blob
		_ = json.Unmarshal(v, &f)
		x = string(f.Data)
	}
	if b.Kind == "mcp" && v != nil {
		var s Server
		_ = json.Unmarshal(v, &s)
		old, _ := getField(d.Map, b.Field)
		prior, _ := old.(map[string]any)
		x = encodeServer(b.Provider, s, prior)
	}
	if b.Kind == "native-mcp" && v != nil {
		var f Blob
		if e = json.Unmarshal(v, &f); e != nil {
			return e
		}
		x, e = nativeMCP(b.Provider, f.Data)
		if e != nil {
			return e
		}
		prior, _ := getField(d.Map, b.Field)
		old, _ := prior.(map[string]any)
		_, private := splitMCPFields(old)
		mergePrivateMCP(x.(map[string]any), private)
	}
	if b.Kind == "plugin" && v != nil {
		old, _ := getField(d.Map, b.Field)
		prior, _ := old.(map[string]any)
		if prior == nil {
			prior = map[string]any{}
		}
		prior["enabled"] = object(v)["enabled"]
		x = prior
	}
	if e := setField(d.Map, b.Field, x, v != nil); e != nil {
		return e
	}
	d.Image.Exists = true
	return nil
}
func decodeServer(provider string, m map[string]any) (Server, error) {
	s := Server{}
	for _, key := range []string{"url", "bearer_token_env_var"} {
		if v, ok := m[key]; ok {
			if _, ok := v.(string); !ok {
				return s, errors.New("MCP " + key + " must be a string")
			}
		}
	}
	if v, ok := m["args"]; ok {
		if _, ok := v.([]any); !ok {
			return s, errors.New("MCP args must be an array")
		}
	}
	if v, ok := m["env_vars"]; ok {
		if _, ok := v.([]any); !ok {
			return s, errors.New("MCP env_vars must be an array")
		}
	}
	if provider == "opencode" {
		if v, ok := m["command"]; ok {
			if _, ok := v.([]any); !ok {
				return s, errors.New("OpenCode MCP command must be an array")
			}
		}
		kind, _ := m["type"].(string)
		if kind != "local" && kind != "remote" {
			return s, errors.New("unsupported OpenCode MCP transport")
		}
	} else if v, ok := m["command"]; ok {
		if _, ok := v.(string); !ok {
			return s, errors.New("MCP command must be a string")
		}
	}
	s.URL, _ = m["url"].(string)
	s.Command, _ = m["command"].(string)
	if provider == "opencode" {
		if command, ok := m["command"].([]any); ok && len(command) > 0 {
			s.Command, _ = command[0].(string)
			for _, x := range command[1:] {
				z, ok := x.(string)
				if !ok {
					return s, errors.New("MCP command arguments must be strings")
				}
				s.Args = append(s.Args, z)
			}
		}
	} else {
		if args, ok := m["args"].([]any); ok {
			for _, x := range args {
				z, ok := x.(string)
				if !ok {
					return s, errors.New("MCP args must be strings")
				}
				s.Args = append(s.Args, z)
			}
		}
	}
	if provider == "claude" && s.URL != "" {
		kind, _ := m["type"].(string)
		if kind != "http" && kind != "streamable-http" {
			return s, errors.New("only HTTP and stdio MCP transports are portable")
		}
	}
	if provider == "codex" {
		if vars, ok := m["env_vars"].([]any); ok {
			for _, x := range vars {
				if z, ok := x.(string); ok {
					s.Env = append(s.Env, z)
				} else {
					return s, errors.New("remote MCP environment references are unsupported")
				}
			}
		}
		s.BearerEnv, _ = m["bearer_token_env_var"].(string)
	} else {
		envKey := "env"
		if provider == "opencode" {
			envKey = "environment"
		}
		if vars, ok := m[envKey].(map[string]any); ok {
			for k, x := range vars {
				z, _ := x.(string)
				if (provider == "claude" && z == "${"+k+"}") || (provider == "opencode" && z == "{env:"+k+"}") {
					s.Env = append(s.Env, k)
				}
			}
		}
		if headers, ok := m["headers"].(map[string]any); ok {
			z, _ := headers["Authorization"].(string)
			prefix := "Bearer ${"
			if provider == "opencode" {
				prefix = "Bearer {env:"
			}
			if strings.HasPrefix(z, prefix) && strings.HasSuffix(z, "}") {
				s.BearerEnv = strings.TrimSuffix(strings.TrimPrefix(z, prefix), "}")
			}
		}
	}
	sort.Strings(s.Env)
	return s, validateServer(s)
}
func encodeServer(provider string, s Server, prior map[string]any) map[string]any {
	m := map[string]any{}
	for k, v := range prior {
		m[k] = v
	}
	for _, k := range []string{"command", "args", "url"} {
		delete(m, k)
	}
	if s.URL != "" {
		m["url"] = s.URL
	} else {
		m["command"] = s.Command
		if len(s.Args) > 0 {
			m["args"] = s.Args
		}
	}
	if provider == "codex" {
		delete(m, "env_vars")
		delete(m, "bearer_token_env_var")
		if len(s.Env) > 0 {
			m["env_vars"] = s.Env
		}
		if s.BearerEnv != "" {
			m["bearer_token_env_var"] = s.BearerEnv
		}
		return m
	}
	envKey := "env"
	if provider == "claude" {
		if s.URL != "" {
			m["type"] = "http"
		} else {
			m["type"] = "stdio"
		}
	} else {
		envKey = "environment"
		delete(m, "args")
		if s.URL != "" {
			m["type"] = "remote"
		} else {
			m["type"] = "local"
			m["command"] = append([]string{s.Command}, s.Args...)
		}
	}
	env, _ := m[envKey].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	for k, x := range env {
		if x == "${"+k+"}" || x == "{env:"+k+"}" {
			delete(env, k)
		}
	}
	for _, k := range s.Env {
		if provider == "claude" {
			env[k] = "${" + k + "}"
		} else {
			env[k] = "{env:" + k + "}"
		}
	}
	if len(env) > 0 {
		m[envKey] = env
	} else {
		delete(m, envKey)
	}
	headers, _ := m["headers"].(map[string]any)
	if headers == nil {
		headers = map[string]any{}
	}
	if v, ok := headers["Authorization"].(string); ok && (strings.HasPrefix(v, "Bearer ${") || strings.HasPrefix(v, "Bearer {env:")) {
		delete(headers, "Authorization")
	}
	if s.BearerEnv != "" {
		if provider == "claude" {
			headers["Authorization"] = "Bearer ${" + s.BearerEnv + "}"
		} else {
			headers["Authorization"] = "Bearer {env:" + s.BearerEnv + "}"
		}
	}
	if len(headers) > 0 {
		m["headers"] = headers
	} else {
		delete(m, "headers")
	}
	return m
}
