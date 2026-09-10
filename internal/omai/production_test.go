package omai

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRetentionProtectsRecoveryAndLatestRollback(t *testing.T) {
	p := fake(t)
	now := time.Now()
	makeJournal := func(age int, phase string) string {
		id := fmt.Sprint(now.AddDate(0, 0, -age).UnixNano())
		if e := atomicJSON(filepath.Join(p.State, "transactions", id, "journal.json"), Journal{ID: id, Phase: phase}); e != nil {
			t.Fatal(e)
		}
		return id
	}
	old := makeJournal(40, "complete")
	recent := makeJournal(10, "complete")
	latest := makeJournal(1, "complete")
	pending := makeJournal(50, "pending")
	unreadable := makeJournal(60, "complete")
	put(t, filepath.Join(p.State, "transactions", unreadable, "journal.json"), "{")
	c, e := p.LoadConfig()
	if e != nil {
		t.Fatal(e)
	}
	c.Retention = Retention{Count: 3, Bytes: 1}
	if e = atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	before := get(t, filepath.Join(p.State, "transactions", old, "journal.json"))
	preview, e := p.Backups(true, true)
	if e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.State, "transactions", old, "journal.json")) != before {
		t.Fatal("dry run changed backup")
	}
	if len(preview.Warnings) == 0 {
		t.Fatal("missing protected data warning")
	}
	if _, e = p.Backups(true, false); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{old, recent} {
		if _, e = os.Stat(filepath.Join(p.State, "transactions", id)); !os.IsNotExist(e) {
			t.Fatal("eligible backup retained", id)
		}
	}
	for _, id := range []string{latest, pending, unreadable} {
		if _, e = os.Stat(filepath.Join(p.State, "transactions", id, "journal.json")); e != nil {
			t.Fatal("protected backup lost", id, e)
		}
	}
}

func TestRetentionBoundariesAndDailyCleanup(t *testing.T) {
	p := fake(t)
	now := time.Now()
	for i := 0; i < 4; i++ {
		id := fmt.Sprint(now.Add(-time.Duration(i) * time.Hour).UnixNano())
		if e := atomicJSON(filepath.Join(p.State, "transactions", id, "journal.json"), Journal{ID: id, Phase: "complete"}); e != nil {
			t.Fatal(e)
		}
	}
	out, e := p.backupPlan(Retention{Count: 2}, now)
	if e != nil {
		t.Fatal(e)
	}
	removed := 0
	for _, b := range out.Items {
		if b.Remove {
			removed++
		}
	}
	if removed != 2 {
		t.Fatalf("wanted two oldest removed: %+v", out)
	}
	if _, e = p.autoPrune(Retention{Count: 2}); e != nil {
		t.Fatal(e)
	}
	out, e = p.backupPlan(Retention{}, now)
	if e != nil || len(out.Items) != 2 {
		t.Fatal(out, e)
	}
	if _, e = p.autoPrune(Retention{Count: 1}); e != nil {
		t.Fatal(e)
	}
	out, _ = p.backupPlan(Retention{}, now)
	if len(out.Items) != 2 {
		t.Fatal("cleanup ran twice in a day")
	}
	// Empty directory simulates interruption between unlinking a journal and rmdir.
	if e = os.MkdirAll(filepath.Join(p.State, "transactions", "123"), 0700); e != nil {
		t.Fatal(e)
	}
	if e = p.recover(); e != nil {
		t.Fatal("empty interrupted cleanup blocks recovery", e)
	}
}

func TestRetentionPreservesActualRollbackAndLegacyConfig(t *testing.T) {
	p := fake(t)
	// Legacy config has no retention object.
	put(t, filepath.Join(p.Config, "config.json"), `{"version":1,"branch":"main","providers":["codex"]}`)
	put(t, filepath.Join(p.Source, "instructions.md"), "Before.\n")
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "instructions.md"), "After.\n")
	syncLocal(t, p)
	c, _ := p.LoadConfig()
	c.Retention = Retention{Count: 1, Bytes: 1}
	if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	if _, e := p.Backups(true, false); e != nil {
		t.Fatal(e)
	}
	if e := p.Rollback(""); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "Before.\n" {
		t.Fatal("latest backup not usable")
	}
	if !p.IsPaused() {
		t.Fatal("rollback did not pause")
	}
}

func TestStatusReportsCorruptionAndCurrentVersion(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.State, "status.json"), `{"version":"0.1.0","health":"healthy"}`)
	if p.Status().Version != Version {
		t.Fatal("cached binary version leaked")
	}
	for _, file := range []string{"status.json", "baseline.json", "conflicts.json"} {
		t.Run(file, func(t *testing.T) {
			path := filepath.Join(p.State, file)
			old, e := fileImage(path)
			if e != nil {
				t.Fatal(e)
			}
			put(t, path, "{")
			s := p.Status()
			if s.Health != "error" || s.Error == "" {
				t.Fatalf("corrupt %s hidden: %+v", file, s)
			}
			if e = atomicFile(path, old); e != nil {
				t.Fatal(e)
			}
		})
	}
	put(t, filepath.Join(p.Config, "config.json"), "{")
	s, issues := p.Doctor()
	if s.Health != "error" || len(issues) == 0 {
		t.Fatal("invalid config misreported as first run")
	}
}

func TestSyncReportsStatusPersistenceFailure(t *testing.T) {
	p := fake(t)
	if e := os.Mkdir(filepath.Join(p.State, "status.json"), 0700); e != nil {
		t.Fatal(e)
	}
	s, e := p.Sync(SyncOptions{LocalOnly: true})
	if e == nil || s.Health != "error" || !strings.Contains(e.Error(), "persist sync status") {
		t.Fatal(s, e)
	}
}

func TestUnknownNativeValuesRemainExact(t *testing.T) {
	p := fake(t)
	path := filepath.Join(p.Home, ".claude/settings.json")
	put(t, path, `{"localCounter":9007199254740993,"localDecimal":0.1234567890123456789}`)
	changeManifest(t, p, func(m *Manifest) { m.Settings["claude"] = map[string]any{"language": "English"} })
	syncLocal(t, p)
	value := get(t, path)
	if !strings.Contains(value, "9007199254740993") || !strings.Contains(value, "0.1234567890123456789") {
		t.Fatal("unknown numeric values rounded", value)
	}
	path = filepath.Join(p.Home, ".codex/config.toml")
	put(t, path, "mcp_servers = 'local sentinel'\n")
	changeManifest(t, p, func(m *Manifest) { m.MCP["example"] = Server{Command: "example-mcp"} })
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); e == nil {
		t.Fatal("overwrote scalar container")
	}
	if get(t, path) != "mcp_servers = 'local sentinel'\n" {
		t.Fatal("unknown scalar lost")
	}
}

func TestMalformedHooksAndMCPStayLocal(t *testing.T) {
	for _, value := range []any{"30", false, float64(-1)} {
		hook := []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "echo done", "timeout": value}}}}
		if validateHook("claude", "Stop", hook) == nil {
			t.Fatal("bad timeout accepted", value)
		}
	}
	if _, e := decodeServer("codex", map[string]any{"command": "mcp-test", "args": "lost"}); e == nil {
		t.Fatal("invalid args silently discarded")
	}
	p := fake(t)
	path := filepath.Join(p.Home, ".claude/settings.json")
	put(t, path, `{"hooks":{"Stop":[{"hooks":[{"type":"prompt","prompt":"local only"}]}]}}`)
	changeManifest(t, p, func(m *Manifest) { m.Settings["claude"] = map[string]any{"language": "English"} })
	syncLocal(t, p)
	if !strings.Contains(get(t, path), "local only") {
		t.Fatal("unsupported hook lost")
	}
}

func TestSetupRetryPreservesSource(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "Keep this.\n")
	if e := p.Setup(SetupOptions{}); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "Keep this.\n" {
		t.Fatal("retry reset source")
	}
	if e := p.Setup(SetupOptions{Remote: "git@example.com:other.git"}); e == nil {
		t.Fatal("retry silently changed remote")
	}
}

// systemctl fixture maintains independent active/enabled state and can fail one
// activation. No test can contact the real user service manager.
func fakeSystemctl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stub := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$OMAI_SERVICE_TEST/calls"
shift
case "$1" in
 is-active) test -f "$OMAI_SERVICE_TEST/active" ;;
 is-enabled) test -f "$OMAI_SERVICE_TEST/enabled" ;;
 stop) rm -f "$OMAI_SERVICE_TEST/active" ;;
 disable) rm -f "$OMAI_SERVICE_TEST/enabled"; if [ "${2:-}" = --now ]; then rm -f "$OMAI_SERVICE_TEST/active"; fi ;;
 enable) touch "$OMAI_SERVICE_TEST/enabled"; if [ "${2:-}" = --now ]; then touch "$OMAI_SERVICE_TEST/active"; fi ;;
 start) if [ -f "$OMAI_SERVICE_TEST/fail" ]; then rm "$OMAI_SERVICE_TEST/fail"; exit 1; fi; touch "$OMAI_SERVICE_TEST/active" ;;
 daemon-reload) : ;;
 *) exit 1 ;;
esac
`
	put(t, filepath.Join(dir, "systemctl"), stub)
	if e := os.Chmod(filepath.Join(dir, "systemctl"), 0700); e != nil {
		t.Fatal(e)
	}
	t.Setenv("OMAI_SERVICE_TEST", dir)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return dir
}

func TestInstallationPreservesServiceStateAndRestoresFailedUpgrade(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint(active), func(t *testing.T) {
			dir := fakeSystemctl(t)
			p := fake(t)
			dest, unit, cli := p.installTargets()
			put(t, dest, "old binary")
			put(t, unit, "# Managed by omai\nold unit\n")
			put(t, cli, "#!/bin/sh\n# Managed by omai\nold CLI\n")
			if active {
				put(t, filepath.Join(dir, "active"), "")
				put(t, filepath.Join(dir, "enabled"), "")
				put(t, filepath.Join(dir, "fail"), "")
			}
			if e := p.Control("pause"); e != nil {
				t.Fatal(e)
			}
			e := p.Install(false, false)
			if active {
				if e == nil {
					t.Fatal("failed restart reported success")
				}
				if get(t, dest) != "old binary" || !strings.Contains(get(t, unit), "old unit") || !strings.Contains(get(t, cli), "old CLI") {
					t.Fatal("failed upgrade not restored")
				}
				if !serviceQuery("is-active") || !serviceQuery("is-enabled") {
					t.Fatal("prior active state lost")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				if serviceQuery("is-active") || serviceQuery("is-enabled") {
					t.Fatal("update activated stopped service")
				}
				if get(t, dest+".previous") != "old binary" {
					t.Fatal("previous binary missing")
				}
			}
			if !p.IsPaused() {
				t.Fatal("update resumed sync")
			}
			if _, e := os.Stat(filepath.Join(p.State, "install-pending.json")); !os.IsNotExist(e) {
				t.Fatal("pending install not cleared", e)
			}
		})
	}
}

func TestInstallRecoveryAndConcurrentInstaller(t *testing.T) {
	p := fake(t)
	dest, _, _ := p.installTargets()
	before := FileImage{true, []byte("before"), 0700}
	after := FileImage{true, []byte("after"), 0700}
	j := installation{Changes: []Change{{dest, before, after}}}
	if e := atomicFile(dest, after); e != nil {
		t.Fatal(e)
	}
	if e := atomicJSON(filepath.Join(p.State, "install-pending.json"), j); e != nil {
		t.Fatal(e)
	}
	if e := p.RecoverInstall(false); e != nil {
		t.Fatal(e)
	}
	if get(t, dest) != "before" {
		t.Fatal("interrupted install not recovered")
	}
	u, e := lock(filepath.Join(p.State, "install.lock"))
	if e != nil {
		t.Fatal(e)
	}
	if e = p.Install(false, true); e == nil {
		t.Fatal("concurrent installer acquired lock")
	}
	u()
	put(t, dest, "external edit")
	if e = atomicJSON(filepath.Join(p.State, "install-pending.json"), j); e != nil {
		t.Fatal(e)
	}
	if e = p.RecoverInstall(false); e == nil {
		t.Fatal("newer edit overwritten")
	}
	if get(t, dest) != "external edit" {
		t.Fatal("external edit lost")
	}
}

func TestUninstallOnlyOwnedFilesAndRetainsData(t *testing.T) {
	dir := fakeSystemctl(t)
	p := fake(t)
	if e := p.InstallService(); e != nil {
		t.Fatal(e)
	}
	dest, unit, cli := p.installTargets()
	before := get(t, filepath.Join(p.Source, "omai.json"))
	put(t, cli, "unrelated")
	if e := p.UninstallService(); e == nil {
		t.Fatal("removed unrelated launcher")
	}
	put(t, cli, "#!/bin/sh\n# Managed by omai\nexit 0\n")
	if e := p.UninstallService(); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{unit, cli, filepath.Join(dir, "active"), filepath.Join(dir, "enabled")} {
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("service file remains", path, e)
		}
	}
	if _, e := os.Stat(dest); e != nil {
		t.Fatal("uninstall deleted binary", e)
	}
	if get(t, filepath.Join(p.Source, "omai.json")) != before {
		t.Fatal("uninstall changed source")
	}
}

func TestRetentionValidationAndUnsafeJournal(t *testing.T) {
	p := fake(t)
	c, _ := p.LoadConfig()
	c.Retention.Days = -1
	if e := atomicJSON(filepath.Join(p.Config, "config.json"), c); e != nil {
		t.Fatal(e)
	}
	if _, e := p.LoadConfig(); e == nil {
		t.Fatal("negative retention accepted")
	}
	id := "123"
	if e := atomicJSON(filepath.Join(p.State, "transactions", id, "journal.json"), Journal{ID: "456", Phase: "complete"}); e != nil {
		t.Fatal(e)
	}
	if _, e := p.readJournal(id); e == nil {
		t.Fatal("mismatched journal accepted")
	}
}

func TestBoundedGitOutputCannotBypassLimit(t *testing.T) {
	out := boundedOutput{limit: 16}
	if _, e := io.Copy(&out, strings.NewReader(strings.Repeat("x", 32))); e == nil {
		t.Fatal("Git output limit bypassed")
	}
	if out.buffer.Len() > 16 {
		t.Fatal("buffer exceeded its limit")
	}
}
func TestInvalidLocalPathsAndBranchesAreRejected(t *testing.T) {
	p := FakePaths(t.TempDir())
	p.Data = "relative/data"
	if e := p.Setup(SetupOptions{}); e == nil {
		t.Fatal("relative XDG root accepted")
	}
	if e := p.Install(false, true); e == nil {
		t.Fatal("relative installation path accepted")
	}
	for _, name := range []string{"main:other", "../bad", "a..b", "a/.hidden", "a.lock", "-main", "main\n"} {
		if e := validBranch(name); e == nil {
			t.Fatal("invalid branch accepted", name)
		}
	}
	for _, name := range []string{"main", "sync/laptop", "ai-setup"} {
		if e := validBranch(name); e != nil {
			t.Fatal(name, e)
		}
	}
}
func TestGuidedSetupValidationCancellationAndResume(t *testing.T) {
	fakeSystemctl(t)
	p := FakePaths(t.TempDir())
	var output bytes.Buffer
	input := strings.NewReader("not a remote\n\nMy laptop\ninvalid-provider\ncodex\nn\n")
	if e := p.GuidedSetup(input, &output); e == nil || !strings.Contains(e.Error(), "cancelled") {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(p.Config, "config.json")); !os.IsNotExist(e) {
		t.Fatal("cancelled setup changed config")
	}
	if !strings.Contains(output.String(), "absolute local path") || !strings.Contains(output.String(), "Choose codex") {
		t.Fatal("missing validation feedback", output.String())
	}
	if e := p.Setup(SetupOptions{}); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p.Source, "instructions.md"), "Keep saved setup.\n")
	output.Reset()
	if e := p.GuidedSetup(strings.NewReader("y\n"), &output); e != nil {
		t.Fatal(e)
	}
	if get(t, filepath.Join(p.Source, "instructions.md")) != "Keep saved setup.\n" {
		t.Fatal("resumed setup changed source")
	}
}

func TestSyncReportsApplyTimeWriteFailure(t *testing.T) {
	p := fake(t)
	if e := os.Mkdir(filepath.Join(p.State, "last-apply.json"), 0700); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p.Source, "instructions.md"), "Keep this application.\n")
	_, e := p.Sync(SyncOptions{LocalOnly: true})
	if e == nil || !strings.Contains(e.Error(), "last application time") {
		t.Fatal("lost application metadata error", e)
	}
	if get(t, filepath.Join(p.Home, ".claude/CLAUDE.md")) != "Keep this application.\n" {
		t.Fatal("application data changed after metadata error")
	}
}
