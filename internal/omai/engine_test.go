package omai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fake(t *testing.T) Paths {
	t.Helper()
	p := FakePaths(t.TempDir())
	if e := p.Setup(SetupOptions{Providers: Providers}); e != nil {
		t.Fatal(e)
	}
	return p
}
func put(t *testing.T, path, content string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte(content), 0600); e != nil {
		t.Fatal(e)
	}
}
func get(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func syncLocal(t *testing.T, p Paths) Status {
	t.Helper()
	s, e := p.Sync(SyncOptions{LocalOnly: true})
	if e != nil {
		cs, _ := p.conflicts()
		t.Fatalf("%v conflicts=%s", e, jsonBytes(cs))
	}
	return s
}
func changeManifest(t *testing.T, p Paths, f func(*Manifest)) {
	t.Helper()
	var m Manifest
	if e := readJSON(filepath.Join(p.Source, "omai.json"), &m); e != nil {
		t.Fatal(e)
	}
	f(&m)
	put(t, filepath.Join(p.Source, "omai.json"), string(jsonBytes(m)))
}
func TestAdaptersRoundTripAndUnknownSettings(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Home, ".codex/config.toml"), "# existing\nunknown = 'keep'\n[local]\nlast_seen = 1979-05-27T07:32:00Z\n")
	put(t, filepath.Join(p.Home, ".claude/settings.json"), `{"localUnknown":{"enabled":true},"env":{"LOCAL_SECRET":"must-remain-local"}}`)
	put(t, filepath.Join(p.Home, ".claude.json"), `{"oauthAccount":{"token":"LOCAL_TOKEN"},"projects":{"/private":{"history":"LOCAL_HISTORY"}}}`)
	put(t, filepath.Join(p.Home, ".config/opencode/opencode.jsonc"), "{\n// retained semantically\n\"localUnknown\": 7,\n}")
	put(t, filepath.Join(p.Source, "instructions.md"), "Use concise answers.\n")
	put(t, filepath.Join(p.Source, "rules/style.md"), "Prefer clear names.\n")
	put(t, filepath.Join(p.Source, "skills/review/SKILL.md"), "---\nname: review\ndescription: Review code\n---\nRead resources/check.sh.\n")
	put(t, filepath.Join(p.Source, "skills/review/resources/check.sh"), "#!/bin/sh\nprintf '%s\\n' checked\n")
	if e := os.Chmod(filepath.Join(p.Source, "skills/review/resources/check.sh"), 0700); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p.Source, "agents/reviewer.md"), "---\ndescription: Review code\n---\n\nFind correctness issues.\n")
	put(t, filepath.Join(p.Source, "commands/check.md"), "Run the full test suite.\n")
	changeManifest(t, p, func(m *Manifest) {
		m.Settings["codex"] = map[string]any{"model": "test-model"}
		m.Settings["claude"] = map[string]any{"language": "English"}
		m.Settings["opencode"] = map[string]any{"theme": "system"}
		m.MCP["filesystem"] = Server{Command: "mcp-fs", Args: []string{"--readonly"}, Env: []string{"MCP_ROOT"}}
		m.MCP["remote"] = Server{URL: "https://example.com/mcp", BearerEnv: "MCP_BEARER"}
		m.Hooks["claude"] = map[string]any{"Stop": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "printf done"}}}}}
		m.Plugins["codex"] = map[string]any{"sample@market": map[string]any{"enabled": true}}
		m.Plugins["claude"] = map[string]any{"sample@market": true}
		m.Plugins["opencode"] = map[string]any{"opencode-example": true}
	})
	first := syncLocal(t, p)
	for _, path := range []string{".codex/AGENTS.md", ".claude/CLAUDE.md", ".config/opencode/AGENTS.md"} {
		if !strings.Contains(get(t, filepath.Join(p.Home, path)), "Use concise answers.") {
			t.Fatal(path)
		}
	}
	codex := get(t, filepath.Join(p.Home, ".codex/config.toml"))
	if !strings.Contains(codex, "unknown = 'keep'") && !strings.Contains(codex, `unknown = "keep"`) {
		t.Fatal(codex)
	}
	if strings.Contains(codex, `last_seen = '`) || strings.Contains(codex, `last_seen = "`) {
		t.Fatal("unknown TOML timestamp changed type")
	}
	if !strings.Contains(get(t, filepath.Join(p.Home, ".claude.json")), "LOCAL_TOKEN") {
		t.Fatal("unknown credential lost")
	}
	if strings.Contains(get(t, filepath.Join(p.Source, "omai.json")), "LOCAL_TOKEN") {
		t.Fatal("credential imported")
	}
	for i := 0; i < 3; i++ {
		s := syncLocal(t, p)
		if s.Generation != first.Generation {
			t.Fatalf("feedback loop %d -> %d", first.Generation, s.Generation)
		}
	}
	path := filepath.Join(p.Home, ".claude/CLAUDE.md")
	put(t, path, "Be direct.\n")
	syncLocal(t, p)
	if get(t, filepath.Join(p.Source, "instructions.md")) != "Be direct.\n" {
		t.Fatal("reverse import failed")
	}
	if !strings.Contains(get(t, filepath.Join(p.Home, ".codex/AGENTS.md")), "Be direct.") {
		t.Fatal("relay failed")
	}
	// Unknown provider agent frontmatter survives a canonical edit.
	put(t, filepath.Join(p.Home, ".claude/agents/reviewer.md"), "---\ndescription: Review code\nmodel: local-model\n---\n\nFind correctness issues.\n")
	put(t, filepath.Join(p.Source, "agents/reviewer.md"), "---\ndescription: Review code\n---\n\nFind concurrency bugs.\n")
	syncLocal(t, p)
	if !strings.Contains(get(t, filepath.Join(p.Home, ".claude/agents/reviewer.md")), "local-model") {
		t.Fatal("agent settings lost")
	}
}
func TestConcurrentProviderChangesAndDeletion(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "base\n")
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "canonical\n")
	put(t, filepath.Join(p.Home, ".claude/CLAUDE.md"), "provider\n")
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); !errors.Is(e, ErrConflict) {
		t.Fatalf("expected conflict, got %v", e)
	}
	if get(t, filepath.Join(p.Home, ".claude/CLAUDE.md")) != "provider\n" {
		t.Fatal("overwrote conflict")
	}
	if _, e := p.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"instructions.md": "claude"}}); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "provider\n" {
		t.Fatal("wrong resolution")
	}
	// File deletion imports once and stays deleted; a null tombstone cannot loop.
	if e := os.Remove(filepath.Join(p.Home, ".claude/CLAUDE.md")); e != nil {
		t.Fatal(e)
	}
	syncLocal(t, p)
	syncLocal(t, p)
	if _, e := os.Stat(filepath.Join(p.Source, "instructions.md")); !os.IsNotExist(e) {
		t.Fatal("canonical deletion not relayed")
	}
}
func TestRollbackAndCrashRecovery(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "before\n")
	syncLocal(t, p)
	put(t, filepath.Join(p.Home, ".claude/CLAUDE.md"), "after\n")
	syncLocal(t, p)
	if e := p.Rollback(""); e != nil {
		t.Fatal(e)
	}
	if !p.IsPaused() {
		t.Fatal("rollback must pause")
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "before\n" {
		t.Fatal("canonical rollback failed")
	}
	if e := p.Resume(); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(p.Home, "recovery.txt")
	put(t, path, "before")
	before, _ := fileImage(path)
	after := FileImage{true, []byte("after"), 0600}
	j := Journal{ID: "123", Phase: "pending", Changes: []Change{{path, before, after}}}
	if e := atomicJSON(filepath.Join(p.State, "transactions/123/journal.json"), j); e != nil {
		t.Fatal(e)
	}
	if e := atomicFile(path, after); e != nil {
		t.Fatal(e)
	}
	if e := p.recover(); e != nil {
		t.Fatal(e)
	}
	if get(t, path) != "before" {
		t.Fatal("recovery failed")
	}
	j.ID = "124"
	if e := atomicJSON(filepath.Join(p.State, "transactions/124/journal.json"), j); e != nil {
		t.Fatal(e)
	}
	put(t, path, "external edit")
	if e := p.recover(); e == nil {
		t.Fatal("must not overwrite drift during recovery")
	}
}
func TestSecretAndPathExclusions(t *testing.T) {
	for _, path := range []string{".ssh/id_rsa", ".config/tool/auth.json", "../outside", ".env", "personal/history.jsonl", "skills/a/cache/data", "personal/state.sqlite"} {
		if safeRel(path) == nil {
			t.Errorf("accepted %s", path)
		}
	}
	for _, s := range []string{"api_key = 'abc123-private-value'", "-----BEGIN RSA PRIVATE KEY-----", "https://user:password@example.com"} {
		if scanContent("test", []byte(s)) == nil {
			t.Errorf("accepted sensitive content")
		}
	}
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "safe\n")
	syncLocal(t, p)
	target := filepath.Join(p.Home, "outside")
	put(t, target, "do not change")
	if e := os.Remove(filepath.Join(p.Home, ".claude/CLAUDE.md")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(target, filepath.Join(p.Home, ".claude/CLAUDE.md")); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); e == nil {
		t.Fatal("symlink must block")
	}
	if get(t, target) != "do not change" {
		t.Fatal("followed symlink")
	}
}
func TestTwoMachinesGitOfflineConflictRecovery(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	cmd := exec.Command("git", "init", "--bare", remote)
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("%v: %s", e, out)
	}
	mk := func(label string) Paths {
		p := FakePaths(filepath.Join(root, label))
		if e := p.Setup(SetupOptions{Remote: remote, Providers: Providers, Machine: label}); e != nil {
			t.Fatal(e)
		}
		return p
	}
	a, b := mk("A"), mk("B")
	put(t, filepath.Join(a.Source, "instructions.md"), "base\n")
	run := func(p Paths) {
		t.Helper()
		if _, e := p.Sync(SyncOptions{}); e != nil {
			t.Fatal(e)
		}
	}
	run(a)
	run(b)
	if !strings.Contains(get(t, filepath.Join(b.Home, ".codex/AGENTS.md")), "base") {
		t.Fatal("initial relay failed")
	}
	ids1, e := a.SnapshotIDs()
	if e != nil {
		t.Fatal(e)
	}
	run(a)
	ids2, _ := a.SnapshotIDs()
	if ids1["local"] != ids2["local"] {
		g, _ := a.gitStore(Config{Branch: "main"})
		diff, _ := g.run(nil, nil, "diff", ids1["local"], ids2["local"])
		t.Fatalf("feedback-loop commit: %s", diff)
	}
	offline := remote + "-offline"
	if e := os.Rename(remote, offline); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(a.Home, ".claude/CLAUDE.md"), "offline A\n")
	if _, e := a.Sync(SyncOptions{}); !errors.Is(e, ErrOffline) {
		t.Fatalf("offline: %v", e)
	}
	if !strings.Contains(get(t, filepath.Join(a.Home, ".config/opencode/AGENTS.md")), "offline A") {
		t.Fatal("offline local propagation failed")
	}
	put(t, filepath.Join(b.Source, "instructions.md"), "offline B\n")
	if _, e := b.Sync(SyncOptions{}); !errors.Is(e, ErrOffline) {
		t.Fatal(e)
	}
	if e := os.Rename(offline, remote); e != nil {
		t.Fatal(e)
	}
	run(a)
	if _, e := b.Sync(SyncOptions{}); !errors.Is(e, ErrConflict) {
		t.Fatalf("expected Git conflict: %v", e)
	}
	if get(t, filepath.Join(b.Source, "instructions.md")) != "offline B\n" {
		t.Fatal("conflict overwritten")
	}
	if _, e := b.Sync(SyncOptions{GitChoices: map[string]string{"git/instructions.md": "local"}}); e != nil {
		t.Fatal(e)
	}
	run(a)
	if get(t, filepath.Join(a.Source, "instructions.md")) != "offline B\n" {
		t.Fatal("resolution not relayed")
	}
	// Independent file edits merge without selecting a winner for overlapping changes.
	put(t, filepath.Join(a.Source, "rules/a.md"), "A rule\n")
	put(t, filepath.Join(b.Source, "rules/b.md"), "B rule\n")
	run(a)
	run(b)
	run(a)
	for _, p := range []Paths{a, b} {
		if get(t, filepath.Join(p.Source, "rules/a.md")) != "A rule\n" || get(t, filepath.Join(p.Source, "rules/b.md")) != "B rule\n" {
			t.Fatal("disjoint merge failed")
		}
	}
	// Recovery after an interrupted transaction is independent of network availability.
	path := filepath.Join(b.Home, ".claude/CLAUDE.md")
	before, _ := fileImage(path)
	after := FileImage{true, []byte("interrupted"), 0600}
	j := Journal{ID: "456", Phase: "pending", Changes: []Change{{path, before, after}}}
	if e := atomicJSON(filepath.Join(b.State, "transactions/456/journal.json"), j); e != nil {
		t.Fatal(e)
	}
	if e := atomicFile(path, after); e != nil {
		t.Fatal(e)
	}
	run(b)
	if get(t, path) != string(before.Data) {
		t.Fatal("recovery failed")
	}
	// The remote contains only canonical data, with a constant author.
	out, e := exec.Command("git", "--git-dir="+remote, "log", "--all", "--format=%an <%ae>").Output()
	if e != nil {
		t.Fatal(e)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "omai <omai@localhost>" {
			t.Fatal("machine identity in Git")
		}
	}
	tree, e := exec.Command("git", "--git-dir="+remote, "ls-tree", "-r", "--name-only", "refs/heads/main").Output()
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range strings.Fields(string(tree)) {
		if !allowedSource(name) {
			t.Fatal("unexpected remote path", name)
		}
	}
}
func TestDaemonDetectsChanges(t *testing.T) {
	p := fake(t)
	c, _ := p.LoadConfig()
	c.PollSeconds = 1
	c.SyncSeconds = 1
	if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Daemon(ctx) }()
	defer func() {
		cancel()
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	put(t, filepath.Join(p.Source, "instructions.md"), "automatic\n")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, e := os.ReadFile(filepath.Join(p.Home, ".claude/CLAUDE.md"))
		if e == nil && string(b) == "automatic\n" {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("daemon failed to relay")
}
func TestUnknownJSONPreserved(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Home, ".claude/settings.json"), `{"unknown":{"nested":[1,2,3]},"language":"English"}`)
	syncLocal(t, p)
	changeManifest(t, p, func(m *Manifest) { m.Settings["claude"]["language"] = "Spanish" })
	syncLocal(t, p)
	var d map[string]any
	if e := json.Unmarshal([]byte(get(t, filepath.Join(p.Home, ".claude/settings.json"))), &d); e != nil {
		t.Fatal(e)
	}
	if d["unknown"] == nil {
		t.Fatal("unknown key lost")
	}
}

func TestMultipleConflictsRetainExplicitChoicesAndRejectStaleChoices(t *testing.T) {
	p := fake(t)
	for _, key := range []string{"instructions.md", "rules/style.md"} {
		put(t, filepath.Join(p.Source, key), "base\n")
	}
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "canonical\n")
	put(t, filepath.Join(p.Source, "rules/style.md"), "canonical rule\n")
	put(t, filepath.Join(p.Home, ".claude/CLAUDE.md"), "provider\n")
	put(t, filepath.Join(p.Home, ".claude/rules/style.md"), "provider rule\n")
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e := p.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"instructions.md": "claude"}}); !errors.Is(e, ErrConflict) {
		t.Fatal("second conflict should remain", e)
	}
	if _, e := p.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"rules/style.md": "canonical"}}); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "provider\n" {
		t.Fatal("first decision forgotten")
	}
	if get(t, filepath.Join(p.Home, ".claude/rules/style.md")) != "canonical rule\n" {
		t.Fatal("second decision ignored")
	}
	// Start another pair and edit the first after deciding: stale decisions expire.
	put(t, filepath.Join(p.Source, "instructions.md"), "C2\n")
	put(t, filepath.Join(p.Home, ".claude/CLAUDE.md"), "P2\n")
	put(t, filepath.Join(p.Source, "rules/style.md"), "C rule 2\n")
	put(t, filepath.Join(p.Home, ".claude/rules/style.md"), "P rule 2\n")
	_, _ = p.Sync(SyncOptions{LocalOnly: true})
	_, _ = p.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"instructions.md": "claude"}})
	put(t, filepath.Join(p.Home, ".claude/CLAUDE.md"), "P3\n")
	if _, e := p.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"rules/style.md": "canonical"}}); !errors.Is(e, ErrConflict) {
		t.Fatal("stale decision applied", e)
	}
	cs, _ := p.Conflicts()
	if len(cs) != 1 || cs[0].Key != "instructions.md" {
		t.Fatalf("unexpected conflicts: %v", cs)
	}
}
func TestRollbackCanonicalEditAndDrift(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "first\n")
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "second\n")
	syncLocal(t, p)
	if e := p.Rollback(""); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "first\n" {
		t.Fatal("hand-edited source did not roll back")
	}
	if get(t, filepath.Join(p.Home, ".claude/CLAUDE.md")) != "first\n" {
		t.Fatal("provider did not roll back")
	}
	if e := p.Resume(); e != nil {
		t.Fatal(e)
	}
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "third\n")
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "unsynced\n")
	if e := p.Rollback(""); e == nil {
		t.Fatal("rollback overwrote unobserved edits")
	}
}
func TestPersonalFilesAndSafeLargeResources(t *testing.T) {
	p := fake(t)
	dest := filepath.Join(p.Home, ".config/editor/preferences.json")
	put(t, dest, `{"theme":"dark"}`)
	if e := p.Enroll("editor", "personal/editor.json", "~/.config/editor/preferences.json"); e != nil {
		t.Fatal(e)
	}
	syncLocal(t, p)
	put(t, dest, `{"theme":"light"}`)
	syncLocal(t, p)
	if get(t, filepath.Join(p.Source, "personal/editor.json")) != `{"theme":"light"}` {
		t.Fatal("personal import failed")
	}
	// Baselines and journals may be larger than an individual resource.
	put(t, filepath.Join(p.Source, "skills/large/SKILL.md"), "---\nname: large\ndescription: Large text resource\n---\nUse data.txt\n")
	put(t, filepath.Join(p.Source, "skills/large/data.txt"), strings.Repeat("resource data\n", 50000))
	s := syncLocal(t, p)
	if next := syncLocal(t, p); next.Generation != s.Generation {
		t.Fatal("large baseline loop")
	}
	changeManifest(t, p, func(m *Manifest) {
		m.Personal["duplicate"] = Personal{Path: "~/.config/editor/preferences.json", Source: "personal/editor.json"}
	})
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); e == nil {
		t.Fatal("accepted ambiguous personal mappings")
	}
}
func TestOfflineAndGitConflictsStayVisibleBetweenRetries(t *testing.T) {
	p := fake(t)
	c, _ := p.LoadConfig()
	c.Remote = filepath.Join(t.TempDir(), "unreachable.git")
	if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Sync(SyncOptions{}); !errors.Is(e, ErrOffline) {
		t.Fatal(e)
	}
	s := syncLocal(t, p)
	if s.Health != "offline" {
		t.Fatalf("offline health lost: %s", s.Health)
	}
	if e := p.setConflicts("git", []Conflict{{Key: "git/instructions.md", Kind: "git", Reason: "test", Choices: Values{"local": nil, "remote": nil}}}); e != nil {
		t.Fatal(e)
	}
	s = syncLocal(t, p)
	if s.Health != "conflict" || len(s.Conflicts) != 1 {
		t.Fatal("Git conflict disappeared between retries")
	}
}
func TestCredentialArgumentsAndRawLiteralsNeverImport(t *testing.T) {
	for _, literal := range []string{"TOKEN = 'PRIVATE_VALUE'", "password: SUPERSECRET", "client_secret = abcdef"} {
		if scanContent("fixture", []byte(literal)) == nil {
			t.Fatal("accepted literal")
		}
	}
	if validateServer(Server{Command: "tool", Args: []string{"--api-key", "private-value"}}) == nil {
		t.Fatal("credential flag allowed")
	}
	p := fake(t)
	put(t, filepath.Join(p.Home, ".codex/config.toml"), "[mcp_servers.private]\ncommand='tool'\nargs=['--token','private-value']\n")
	s := syncLocal(t, p)
	if len(s.Warnings) == 0 {
		t.Fatal("unsafe server was not reported")
	}
	if strings.Contains(get(t, filepath.Join(p.Source, "omai.json")), "private-value") {
		t.Fatal("secret leaked")
	}
}
func TestTransactionCrashChild(t *testing.T) {
	home := os.Getenv("OMAI_CRASH_TEST_HOME")
	if home == "" {
		return
	}
	p := FakePaths(home)
	changes := []Change{}
	for i := 0; i < 500; i++ {
		path := filepath.Join(home, "files", fmt.Sprintf("%04d.txt", i))
		before, e := fileImage(path)
		if e != nil {
			t.Fatal(e)
		}
		changes = append(changes, Change{path, before, FileImage{true, []byte("new value"), 0600}})
	}
	if _, e := p.transaction(changes); e != nil {
		t.Fatal(e)
	}
}
func TestKilledWriterRecoversWithoutOrphanSourceFiles(t *testing.T) {
	p := fake(t)
	for i := 0; i < 500; i++ {
		put(t, filepath.Join(p.Home, "files", fmt.Sprintf("%04d.txt", i)), "original")
	}
	child := exec.Command(os.Args[0], "-test.run=^TestTransactionCrashChild$")
	child.Env = append(os.Environ(), "OMAI_CRASH_TEST_HOME="+p.Home)
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(10 * time.Second)
	observed := false
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.Join(p.Home, "files/0005.txt"))
		if string(b) == "new value" {
			observed = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	_ = child.Process.Kill()
	_ = child.Wait()
	if !observed {
		t.Fatal("child did not start applying transaction")
	}
	if e := p.recover(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 500; i++ {
		if get(t, filepath.Join(p.Home, "files", fmt.Sprintf("%04d.txt", i))) != "original" {
			t.Fatal("partial transaction remained")
		}
	}
	ds, e := os.ReadDir(filepath.Join(p.Home, "files"))
	if e != nil {
		t.Fatal(e)
	}
	if len(ds) != 500 {
		t.Fatal("orphan temporary files remained")
	}
}

func TestPersonalDeletionSurvivesNewMachineAttachment(t *testing.T) {
	a := fake(t)
	dest := "~/.config/editor/prefs.json"
	put(t, filepath.Join(a.Home, ".config/editor/prefs.json"), `{"theme":"dark"}`)
	if e := a.Enroll("editor", "personal/prefs.json", dest); e != nil {
		t.Fatal(e)
	}
	syncLocal(t, a)
	if e := os.Remove(filepath.Join(a.Home, ".config/editor/prefs.json")); e != nil {
		t.Fatal(e)
	}
	syncLocal(t, a)
	syncLocal(t, a)
	var m Manifest
	if e := readJSON(filepath.Join(a.Source, "omai.json"), &m); e != nil {
		t.Fatal(e)
	}
	if !m.Personal["editor"].Deleted {
		t.Fatal("deletion was not recorded")
	}
	b := fake(t)
	tree, e := loadTree(a.Source)
	if e != nil {
		t.Fatal(e)
	}
	old, e := loadTree(b.Source)
	if e != nil {
		t.Fatal(e)
	}
	changes, e := treeChanges(b.Source, old, tree)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = b.transaction(changes); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(b.Home, ".config/editor/prefs.json"), `{"theme":"existing"}`)
	if _, e = b.Sync(SyncOptions{LocalOnly: true}); !errors.Is(e, ErrConflict) {
		t.Fatal("a new machine resurrected a deleted personal file", e)
	}
	if _, e = b.Sync(SyncOptions{LocalOnly: true, Choices: map[string]string{"personal/prefs.json": "canonical"}}); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(b.Home, ".config/editor/prefs.json")); !os.IsNotExist(e) {
		t.Fatal("deletion resolution failed")
	}
}
func TestServiceInstallUsesPrivatePathsAndProtectsExistingCLI(t *testing.T) {
	p := FakePaths(filepath.Join(t.TempDir(), `home with space % and "quote"`))
	if e := p.Setup(SetupOptions{}); e != nil {
		t.Fatal(e)
	}
	fakeBin := t.TempDir()
	record := filepath.Join(t.TempDir(), "calls.txt")
	stub := filepath.Join(fakeBin, "systemctl")
	put(t, stub, "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$OMAI_SYSTEMCTL_RECORD\"\n")
	if e := os.Chmod(stub, 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))
	t.Setenv("OMAI_SYSTEMCTL_RECORD", record)
	cliPath := filepath.Join(p.Home, ".local/bin/omai")
	put(t, cliPath, "unrelated command")
	if e := p.InstallService(); e == nil {
		t.Fatal("overwrote an unrelated CLI")
	}
	if get(t, cliPath) != "unrelated command" {
		t.Fatal("CLI changed")
	}
	if e := os.Remove(cliPath); e != nil {
		t.Fatal(e)
	}
	if e := p.InstallService(); e != nil {
		t.Fatal(e)
	}
	unit := get(t, filepath.Join(filepath.Dir(p.Config), "systemd/user/omai.service"))
	if !strings.Contains(unit, "%%") || !strings.Contains(unit, "\\\"") || !strings.Contains(unit, "UMask=0077") {
		t.Fatal("bad unit escaping or permissions")
	}
	if !strings.Contains(get(t, record), "--user enable --now omai.service") {
		t.Fatal("service was not enabled")
	}
	if !strings.Contains(get(t, cliPath), "# Managed by omai") {
		t.Fatal("CLI marker absent")
	}
	ds, e := os.ReadDir(filepath.Join(p.State, "install-backups"))
	if e != nil || len(ds) == 0 {
		t.Fatal("installation has no backup", e)
	}
}
func TestRemoteHistoryRewriteIsNeverAcceptedSilently(t *testing.T) {
	p := fake(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	if b, e := exec.Command("git", "init", "--bare", remote).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, b)
	}
	c, _ := p.LoadConfig()
	c.Remote = remote
	if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p.Source, "instructions.md"), "first\n")
	if _, e := p.Sync(SyncOptions{}); e != nil {
		t.Fatal(e)
	}
	ids, _ := p.SnapshotIDs()
	first := ids["local"]
	put(t, filepath.Join(p.Source, "instructions.md"), "second\n")
	if _, e := p.Sync(SyncOptions{}); e != nil {
		t.Fatal(e)
	}
	if b, e := exec.Command("git", "--git-dir="+remote, "update-ref", "refs/heads/main", first).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, b)
	}
	if _, e := p.Sync(SyncOptions{}); !errors.Is(e, ErrConflict) {
		t.Fatal("accepted rewritten remote", e)
	}
	cs, _ := p.Conflicts()
	if len(cs) != 1 || cs[0].Key != "git/history" {
		t.Fatal(cs)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "second\n" {
		t.Fatal("rewrite replaced canonical state")
	}
	tip, e := exec.Command("git", "--git-dir="+remote, "rev-parse", "refs/heads/main").Output()
	if e != nil {
		t.Fatal(e)
	}
	if strings.TrimSpace(string(tip)) != first {
		t.Fatal("omai pushed over a rewritten branch")
	}
}
func TestMultipleGitConflictDecisions(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	if out, e := exec.Command("git", "init", "--bare", remote).CombinedOutput(); e != nil {
		t.Fatalf("%v %s", e, out)
	}
	a, b := fake(t), fake(t)
	for _, p := range []Paths{a, b} {
		c, _ := p.LoadConfig()
		c.Remote = remote
		if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"instructions.md", "rules/a.md"} {
		put(t, filepath.Join(a.Source, key), "base\n")
	}
	for _, p := range []Paths{a, b} {
		if _, e := p.Sync(SyncOptions{}); e != nil {
			t.Fatal(e)
		}
	}
	for _, key := range []string{"instructions.md", "rules/a.md"} {
		put(t, filepath.Join(a.Source, key), "A\n")
		put(t, filepath.Join(b.Source, key), "B\n")
	}
	if _, e := a.Sync(SyncOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e := b.Sync(SyncOptions{}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e := b.Sync(SyncOptions{GitChoices: map[string]string{"git/instructions.md": "remote"}}); !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
	if _, e := b.Sync(SyncOptions{GitChoices: map[string]string{"git/rules/a.md": "local"}}); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(b.Source, "instructions.md")) != "A\n" || get(t, filepath.Join(b.Source, "rules/a.md")) != "B\n" {
		t.Fatal("incremental Git decisions not retained")
	}
}
func TestUntrustedRemoteFilesAreRejectedBeforeCheckout(t *testing.T) {
	p := fake(t)
	c, _ := p.LoadConfig()
	g, e := p.gitStore(c)
	if e != nil {
		t.Fatal(e)
	}
	// Manufacture a Git tree containing a forbidden pathname without ever using
	// a provider's real credential store or a network remote.
	blob, e := g.run([]byte("synthetic fixture"), nil, "hash-object", "-w", "--stdin")
	if e != nil {
		t.Fatal(e)
	}
	line := "100644 blob " + strings.TrimSpace(string(blob)) + "\t.env\n"
	tree, e := g.run([]byte(line), nil, "mktree")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.snapshot(strings.TrimSpace(string(tree))); e == nil {
		t.Fatal("accepted forbidden remote path")
	}
	line = "120000 blob " + strings.TrimSpace(string(blob)) + "\tinstructions.md\n"
	tree, e = g.run([]byte(line), nil, "mktree")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = g.snapshot(strings.TrimSpace(string(tree))); e == nil {
		t.Fatal("accepted remote symlink")
	}
}

func TestSourceEditsDuringPlanningOrNetworkCannotBeOverwritten(t *testing.T) {
	p := fake(t)
	path := filepath.Join(p.Source, "instructions.md")
	put(t, path, "captured\n")
	captured, e := loadTree(p.Source)
	if e != nil {
		t.Fatal(e)
	}
	put(t, path, "newer edit\n")
	if _, e = treeChanges(p.Source, captured, captured); e == nil {
		t.Fatal("planning accepted a changed preimage")
	}
	remote := Tree{}
	for k, v := range captured {
		remote[k] = v
	}
	remote["instructions.md"] = Blob{Data: []byte("remote version\n"), Mode: 0600}
	if e = p.adoptTree(remote, captured); e == nil {
		t.Fatal("remote adoption overwrote a concurrent edit")
	}
	if get(t, path) != "newer edit\n" {
		t.Fatal("concurrent source edit lost")
	}
}
