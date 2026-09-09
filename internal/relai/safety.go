package relai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
)

const maxFileSize = 2 << 20
const maxSourceSize = 16 << 20
const maxInternalFileSize = 512 << 20

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,100}$`)
var envRE = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var credentialRE = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|\b(?:sk-(?:proj-|ant-)?|gh[pousr]_|github_pat_|xox[baprs]-)[a-zA-Z0-9_-]{12,}|\bAKIA[A-Z0-9]{16}\b)`)
var assignmentRE = regexp.MustCompile(`(?i)(?:api[_-]?key|(?:access[_-]?|refresh[_-]?)?token|password|passwd|(?:client[_-]?)?secret|authorization|bearer|machine[_-]?id|device[_-]?id)\s*["']?\s*[:=]\s*["']?([^\s,"'\n}]+)`)
var bearerReferenceRE = regexp.MustCompile(`Bearer (\$\{[A-Z_][A-Z0-9_]*\}|\{env:[A-Z_][A-Z0-9_]*\})`)
var deniedParts = map[string]bool{".ssh": true, ".gnupg": true, ".aws": true, ".azure": true, "gcloud": true, "credentials": true, "credential": true, "auth": true, "tokens": true, "token": true, "secrets": true, "secret": true, "history": true, "histories": true, "sessions": true, "session": true, "databases": true, "database": true, "memories": true, "memory": true, "logs": true, "log": true, "caches": true, "cache": true, "telemetry": true, "machine-id": true, "machine_id": true, "device-id": true, "node_modules": true, ".git": true, ".env": true}

func validName(s string) error {
	if !nameRE.MatchString(s) || strings.Contains(s, "..") {
		return errors.New("invalid resource name")
	}
	return nil
}
func safeRel(s string) error {
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("control characters in paths are forbidden")
		}
	}
	if s == "" || filepath.IsAbs(s) || filepath.Clean(s) != s || strings.ContainsAny(s, "\\\x00\n\r") {
		return fmt.Errorf("unsafe relative path %q", s)
	}
	for _, part := range strings.Split(s, "/") {
		low := strings.ToLower(part)
		stem := strings.TrimSuffix(low, filepath.Ext(low))
		if strings.HasPrefix(low, ".relai-") || part == ".." || part == "." || deniedParts[low] || deniedParts[stem] || strings.HasPrefix(low, ".env.") || strings.HasSuffix(low, ".db") || strings.HasSuffix(low, ".sqlite") || strings.HasSuffix(low, ".sqlite3") || strings.HasSuffix(low, ".log") || strings.HasSuffix(low, ".jsonl") || strings.Contains(low, "history") || strings.Contains(low, "credential") {
			return fmt.Errorf("excluded path %q", s)
		}
	}
	return nil
}
func safePersonal(s string) error {
	s = strings.TrimPrefix(s, "~/")
	if e := safeRel(s); e != nil {
		return e
	}
	for _, p := range []string{".codex", ".claude", ".claude.json", ".agents", ".config/opencode", ".config/relai", ".local/share/relai", ".local/state/relai", ".config/systemd", ".config/omarchy/plugins", ".local/bin", ".gitconfig", ".netrc", ".npmrc", ".pypirc", ".bash_history"} {
		if s == p || strings.HasPrefix(s, p+"/") {
			return fmt.Errorf("personal path overlaps a protected location: %s", s)
		}
	}
	return nil
}
func scanContent(path string, b []byte) error {
	if len(b) > maxFileSize {
		return fmt.Errorf("%s exceeds 2 MiB", path)
	}
	if !utf8.Valid(b) || strings.IndexByte(string(b), 0) >= 0 {
		return fmt.Errorf("%s: binary resources are unsupported; text resources only", path)
	}
	for _, c := range b {
		if c < 32 && c != '\n' && c != '\r' && c != '\t' {
			return fmt.Errorf("%s: non-text control bytes are unsupported", path)
		}
	}
	if credentialRE.Match(b) {
		return fmt.Errorf("%s: credential-like content blocked (value withheld)", path)
	}
	for _, m := range assignmentRE.FindAllSubmatch(bearerReferenceRE.ReplaceAll(b, []byte("$1")), -1) {
		v := string(m[1])
		if strings.HasPrefix(v, "${") || strings.HasPrefix(v, "$") || strings.HasPrefix(v, "{env:") || v == "null" || v == "false" || v == "true" {
			continue
		}
		return fmt.Errorf("%s: sensitive literal blocked (value withheld)", path)
	}
	// Credential-bearing URLs are never canonical data.
	for _, word := range strings.Fields(string(b)) {
		word = strings.Trim(word, "\"',[]{}")
		if strings.Contains(word, "://") {
			u, e := url.Parse(word)
			if e == nil && u.User != nil {
				return fmt.Errorf("%s: URL credentials blocked", path)
			}
		}
	}
	return nil
}
func noSymlink(path string) error {
	abs, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	cur := string(filepath.Separator)
	for _, p := range strings.Split(strings.TrimPrefix(abs, "/"), "/") {
		cur = filepath.Join(cur, p)
		s, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink refused: %s", cur)
		}
	}
	return nil
}
func readRegular(path string) ([]byte, error) { return readRegularLimit(path, maxFileSize) }
func readRegularLimit(path string, limit int64) ([]byte, error) {
	if e := noSymlink(path); e != nil {
		return nil, e
	}
	f, e := openRegularNoFollow(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if st, ok := s.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return nil, errors.New("hard-linked files are unsupported")
	}
	if !s.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	if s.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d byte limit", limit)
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("file exceeds %d byte limit", limit)
	}
	return b, e
}

var settingKeys = map[string]string{
	"codex":    "model model_provider model_reasoning_effort model_reasoning_summary model_verbosity approval_policy sandbox_mode personality web_search developer_instructions model_instructions_file project_doc_max_bytes project_doc_fallback_filenames model_context_window model_auto_compact_token_limit review_model service_tier notify file_opener hide_agent_reasoning show_raw_agent_reasoning check_for_update_on_startup suppress_unstable_features_warning",
	"claude":   "model effortLevel language outputStyle alwaysThinkingEnabled respectGitignore includeCoAuthoredBy permissions attribution statusLine sandbox spinnerTipsEnabled spinnerVerbs spinnerTipsOverride terminalProgressBar showTurnDuration cleanupPeriodDays fastMode plansDirectory additionalDirectories autoUpdatesChannel disableAllHooks enabledMcpjsonServers disabledMcpjsonServers enableAllProjectMcpServers skillOverrides",
	"opencode": "model small_model default_agent theme autoupdate share instructions permission agent command formatter lsp watcher disabled_providers enabled_providers compaction tools snapshot experimental",
}

var structuredSettingRoots = map[string]string{
	"codex":    "features tui sandbox_workspace_write shell_environment_policy skills profiles model_providers",
	"claude":   "env",
	"opencode": "provider",
}

func safeSetting(p, k string) bool {
	if p == "codex" && (k == "tui.model_availability_nux" || strings.HasPrefix(k, "tui.model_availability_nux.")) {
		return false
	}
	root, _, nested := strings.Cut(k, ".")
	return strings.Contains(" "+settingKeys[p]+" ", " "+k+" ") || (nested && strings.Contains(" "+structuredSettingRoots[p]+" ", " "+root+" "))
}

// Use separate semantic values for structured preferences so local-only fields
// and credentials can stay untouched while neighboring settings synchronize.
func discoveredSettings(provider string, values map[string]any) map[string]any {
	out := map[string]any{}
	var visit func(string, any)
	visit = func(key string, x any) {
		root := strings.SplitN(key, ".", 2)[0]
		if strings.Contains(" "+structuredSettingRoots[provider]+" ", " "+root+" ") {
			if m, ok := x.(map[string]any); ok {
				for k, v := range m {
					if !strings.Contains(k, ".") {
						visit(key+"."+k, v)
					}
				}
				return
			}
		}
		out[key] = x
	}
	for k, v := range values {
		visit(k, v)
	}
	return out
}
func validateSetting(p, k string, x any) error {
	if !safeSetting(p, k) {
		return fmt.Errorf("unsupported or unsafe setting %s/%s", p, k)
	}
	switch k {
	case "alwaysThinkingEnabled", "respectGitignore", "includeCoAuthoredBy":
		if _, ok := x.(bool); !ok {
			return fmt.Errorf("%s/%s must be boolean", p, k)
		}
	case "autoupdate":
		switch x.(type) {
		case bool, string:
		default:
			return errors.New("opencode/autoupdate must be boolean or string")
		}
	default:
		if x == nil {
			return fmt.Errorf("%s/%s: null is not a preference", p, k)
		}
	}
	// Include the key in the scan, since a credential stored in env.API_KEY or
	// provider.options.apiKey must not be mistaken for an ordinary string.
	return scanContent("setting "+p+"/"+k, raw(map[string]any{k: x}))
}
func validateServer(s Server) error {
	if (s.Command == "") == (s.URL == "") {
		return errors.New("specify exactly one of command or url")
	}
	if s.Command != "" {
		if strings.ContainsAny(s.Command, "\r\n\x00") {
			return errors.New("invalid command")
		}
		if filepath.IsAbs(s.Command) {
			return errors.New("use a PATH executable managed by mise, not a machine path")
		}
	}
	if s.URL != "" {
		u, e := url.Parse(s.URL)
		if e != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("MCP URL must be HTTP(S) without credentials, query or fragment")
		}
	}
	for _, k := range s.Env {
		if !envRE.MatchString(k) {
			return errors.New("MCP env must contain environment variable names only")
		}
	}
	for _, arg := range s.Args {
		lower := strings.ToLower(arg)
		for _, name := range []string{"--token", "--api-key", "--apikey", "--password", "--secret", "--authorization"} {
			if lower == name || strings.HasPrefix(lower, name+"=") {
				return errors.New("credential arguments are blocked; use MCP environment references")
			}
		}
	}
	if s.BearerEnv != "" && !envRE.MatchString(s.BearerEnv) {
		return errors.New("bearer_env must name an environment variable")
	}
	return scanContent("MCP", raw(s))
}
func validateHook(p, event string, x any) error {
	events := " SessionStart SessionEnd PreToolUse PostToolUse UserPromptSubmit Stop PreCompact PostCompact SubagentStart SubagentStop PermissionRequest "
	if p == "claude" {
		events += "Notification StopFailure "
	}
	if !strings.Contains(events, " "+event+" ") {
		return fmt.Errorf("unsupported %s hook event %s", p, event)
	}
	groups, ok := x.([]any)
	if !ok {
		return errors.New("hooks must be arrays of matcher groups")
	}
	for _, g := range groups {
		m, ok := g.(map[string]any)
		if !ok {
			return errors.New("invalid hook matcher group")
		}
		for key := range m {
			if key != "matcher" && key != "hooks" {
				return fmt.Errorf("unsupported hook group field %s", key)
			}
		}
		if v, ok := m["matcher"]; ok {
			if _, ok := v.(string); !ok {
				return errors.New("hook matcher must be a string")
			}
		}
		hs, ok := m["hooks"].([]any)
		if !ok {
			return errors.New("hook group needs hooks array")
		}
		for _, h := range hs {
			a, ok := h.(map[string]any)
			if !ok || a["type"] != "command" {
				return errors.New("Relai supports command hook handlers only")
			}
			for key := range a {
				if key != "type" && key != "command" && key != "timeout" && key != "async" {
					return fmt.Errorf("unsupported command hook field %s", key)
				}
			}
			if cmd, ok := a["command"].(string); !ok || cmd == "" {
				return errors.New("hook requires command")
			}
			if v, ok := a["async"]; ok {
				if _, ok := v.(bool); !ok {
					return errors.New("hook async must be boolean")
				}
			}
			if v, ok := a["timeout"]; ok {
				var seconds float64
				switch n := v.(type) {
				case float64:
					seconds = n
				case int64:
					seconds = float64(n)
				case int:
					seconds = float64(n)
				case json.Number:
					var e error
					seconds, e = n.Float64()
					if e != nil {
						return errors.New("invalid hook timeout")
					}
				default:
					return errors.New("hook timeout must be a number")
				}
				if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
					return errors.New("hook timeout must be positive")
				}
				if p == "codex" && event == "SessionEnd" && seconds > 3 {
					return errors.New("Codex SessionEnd timeout must not exceed 3 seconds")
				}
			}
		}
	}
	return scanContent("hook", raw(x))
}
func validatePlugin(p, name string, x any) error {
	if name == "" || strings.ContainsAny(name, "\x00\r\n") {
		return errors.New("invalid plugin name")
	}
	if p == "opencode" && (strings.HasPrefix(name, "file:") || filepath.IsAbs(name)) {
		return errors.New("OpenCode plugin declarations must be package names; local JavaScript hooks go in hooks/opencode")
	}
	if e := scanContent("plugin", raw(x)); e != nil {
		return e
	}
	if p == "codex" {
		m, ok := x.(map[string]any)
		if !ok || len(m) != 1 {
			return errors.New("Codex plugin declaration must contain enabled only")
		}
		_, ok = m["enabled"].(bool)
		if !ok {
			return errors.New("Codex plugin enabled must be boolean")
		}
		return nil
	}
	if _, ok := x.(bool); !ok {
		return errors.New("plugin declaration must be boolean")
	}
	if p == "opencode" && x == false {
		return errors.New("remove an OpenCode plugin declaration to disable it")
	}
	return nil
}
