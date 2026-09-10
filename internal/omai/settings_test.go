package omai

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearSettingsKeepsConfigsAndRecovery(t *testing.T) {
	service := fakeSystemctl(t)
	p := fake(t)
	_, unit, _ := p.installTargets()
	put(t, unit, p.ServiceUnit())
	put(t, filepath.Join(service, "active"), "")
	put(t, filepath.Join(service, "enabled"), "")
	kept := []string{
		filepath.Join(p.Source, "instructions.md"),
		filepath.Join(p.Home, ".codex", "AGENTS.md"),
		filepath.Join(p.Home, ".claude", "CLAUDE.md"),
		filepath.Join(p.Home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(p.Data, "repository.git", "sentinel"),
		filepath.Join(p.State, "transactions", "1", "journal.json"),
		filepath.Join(p.State, "baseline.json"),
	}
	for _, path := range kept {
		put(t, path, "Preserve this content.\n")
	}
	put(t, filepath.Join(p.State, "paused.json"), `{"reason":"user paused"}`)
	put(t, filepath.Join(p.State, "status.json"), `{"health":"conflict","error":"old error","configured":true,"conflicts":[{"key":"old"}],"machines":[{"name":"old"}]}`)
	if e := p.ClearSettings(); e != nil {
		t.Fatal(e)
	}
	for _, path := range kept {
		if get(t, path) != "Preserve this content.\n" {
			t.Fatalf("changed preserved file %s", path)
		}
	}
	for _, path := range []string{filepath.Join(p.Config, "config.json"), filepath.Join(p.State, "paused.json"), filepath.Join(service, "active"), filepath.Join(service, "enabled")} {
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			t.Fatalf("not cleared/stopped: %s: %v", path, e)
		}
	}
	s := p.Status()
	if s.Configured || s.Daemon || s.Paused || s.Health != "setup" || s.Error != "" || len(s.Conflicts) != 0 || len(s.Machines) != 0 {
		t.Fatalf("stale onboarding status: %+v", s)
	}
	if e := p.ClearSettings(); e != nil {
		t.Fatal("reset must be retryable", e)
	}
}

func TestClearSettingsThenSetupReusesSource(t *testing.T) {
	p := fake(t)
	put(t, filepath.Join(p.Source, "instructions.md"), "Existing source\n")
	if e := p.ClearSettings(); e != nil {
		t.Fatal(e)
	}
	if e := p.Setup(SetupOptions{Remote: "git@example.com:new/omai.git", Machine: "New label"}); e != nil {
		t.Fatal(e)
	}
	c, e := p.LoadConfig()
	if e != nil || c.Remote != "git@example.com:new/omai.git" || c.Machine != "New label" || get(t, filepath.Join(p.Source, "instructions.md")) != "Existing source\n" {
		t.Fatal("setup did not preserve source with new preferences", c, e)
	}
}

func TestClearSettingsRefusesUnsafeOrBusyTargets(t *testing.T) {
	for _, kind := range []string{"install", "operation", "daemon", "pending", "unrelated", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			p := fake(t)
			config := filepath.Join(p.Config, "config.json")
			before := get(t, config)
			switch kind {
			case "install", "operation", "daemon":
				u, e := lock(filepath.Join(p.State, kind+".lock"))
				if e != nil {
					t.Fatal(e)
				}
				defer u()
			case "pending":
				put(t, filepath.Join(p.State, "install-pending.json"), "{}")
			case "unrelated":
				_, unit, _ := p.installTargets()
				put(t, unit, "[Service]\nExecStart=/usr/bin/unrelated\n")
			case "symlink":
				outside := filepath.Join(p.Home, "outside")
				put(t, outside, "Keep")
				if e := os.Symlink(outside, filepath.Join(p.State, "paused.json")); e != nil {
					t.Fatal(e)
				}
			}
			if e := p.ClearSettings(); e == nil {
				t.Fatal("unsafe reset accepted")
			}
			if get(t, config) != before {
				t.Fatal("settings changed on refusal")
			}
		})
	}
}
