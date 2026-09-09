package relai

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderProfilesKeepInstructionsSkillsAndMCPSeparate(t *testing.T) {
	p := fake(t)
	for _, provider := range Providers {
		root := nativeRoot(p, provider)
		name := "AGENTS.md"
		if provider == "claude" {
			name = "CLAUDE.md"
		}
		put(t, filepath.Join(root, name), provider+" instructions\n")
		skillRoot := filepath.Join(root, "skills")
		if provider == "codex" {
			skillRoot = filepath.Join(p.Home, ".agents/skills")
		}
		put(t, filepath.Join(skillRoot, "review/SKILL.md"), provider+" skill\n")
	}
	put(t, filepath.Join(p.Home, ".codex/config.toml"), `model = "codex-model"
[features]
context_management = true
[tui]
theme = "dark"
[tui.model_availability_nux]
local_model = 1
[projects."/private/project"]
trust_level = "trusted"
[mcp_servers.tools]
command = "/opt/codex-mcp"
args = ["--readonly"]
enabled = false
startup_timeout_sec = 40
[mcp_servers.tools.env]
API_KEY = "LOCAL_PRIVATE_VALUE"
LANG = "en_US.UTF-8"
`)
	put(t, filepath.Join(p.Home, ".claude.json"), `{"oauthAccount":{"token":"LOCAL_TOKEN"},"mcpServers":{"tools":{"type":"sse","url":"https://claude.example/mcp","headers":{"Authorization":"Bearer ${CLAUDE_MCP_TOKEN}"}}}}`)
	put(t, filepath.Join(p.Home, ".claude/settings.json"), `{"permissions":{"allow":["Read"]},"env":{"EDITOR":"vim","API_KEY":"OTHER_PRIVATE_VALUE"}}`)
	put(t, filepath.Join(p.Home, ".config/opencode/opencode.json"), `{"mcp":{"tools":{"type":"local","command":["/opt/opencode-mcp","serve"],"enabled":false,"timeout":5000}},"permission":{"edit":"ask"}}`)
	s := syncLocal(t, p)
	for _, provider := range Providers {
		prefix := filepath.Join(p.Source, "providers", provider)
		if get(t, filepath.Join(prefix, "instructions.md")) != provider+" instructions\n" || get(t, filepath.Join(prefix, "skills/review/SKILL.md")) != provider+" skill\n" {
			t.Fatal("provider contents were mixed", provider)
		}
		if _, e := os.Stat(filepath.Join(prefix, "mcp/tools.json")); e != nil {
			t.Fatal("MCP not imported", provider, e)
		}
	}
	canonical := get(t, filepath.Join(p.Source, "providers/codex/mcp/tools.json"))
	if strings.Contains(canonical, "LOCAL_PRIVATE_VALUE") || !strings.Contains(canonical, "startup_timeout_sec") || !strings.Contains(canonical, `"enabled": false`) {
		t.Fatal("native MCP options lost or credentials imported", canonical)
	}
	manifest := get(t, filepath.Join(p.Source, "relai.json"))
	for _, absent := range []string{"LOCAL_TOKEN", "OTHER_PRIVATE_VALUE", "model_availability_nux", "/private/project"} {
		if strings.Contains(manifest, absent) {
			t.Fatal("local-only value imported", absent)
		}
	}
	for _, present := range []string{"features.context_management", "tui.theme", "env.EDITOR", "permissions"} {
		if !strings.Contains(manifest, present) {
			t.Fatal("global setting absent", present)
		}
	}
	if syncLocal(t, p).Generation != s.Generation {
		t.Fatal("provider imports created a feedback loop")
	}
	// Canonical edits affect only their provider, retaining local MCP secrets.
	put(t, filepath.Join(p.Source, "providers/codex/instructions.md"), "New Codex instructions\n")
	var server map[string]any
	if e := json.Unmarshal([]byte(canonical), &server); e != nil {
		t.Fatal(e)
	}
	server["enabled"] = true
	put(t, filepath.Join(p.Source, "providers/codex/mcp/tools.json"), string(jsonBytes(server)))
	syncLocal(t, p)
	if get(t, filepath.Join(p.Home, ".claude/CLAUDE.md")) != "claude instructions\n" {
		t.Fatal("Codex instruction edit changed Claude")
	}
	if !strings.Contains(get(t, filepath.Join(p.Home, ".codex/config.toml")), "LOCAL_PRIVATE_VALUE") {
		t.Fatal("local MCP credentials were removed")
	}
}

func TestLinkedAndLegacySkillsAreSnapshottedWithoutChangingTargets(t *testing.T) {
	p := fake(t)
	packageDir := filepath.Join(p.Home, "package-skills", "linked")
	put(t, filepath.Join(packageDir, "SKILL.md"), "Linked skill\n")
	put(t, filepath.Join(packageDir, "references/usage.md"), "Supporting content\n")
	link := filepath.Join(p.Home, ".agents/skills/linked")
	if e := os.MkdirAll(filepath.Dir(link), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(packageDir, link); e != nil {
		t.Fatal(e)
	}
	put(t, filepath.Join(p.Home, ".codex/skills/legacy/SKILL.md"), "Legacy user skill\n")
	put(t, filepath.Join(p.Home, ".codex/skills/.system/builtin/SKILL.md"), "Built-in stays local\n")
	syncLocal(t, p)
	if get(t, filepath.Join(p.Source, "providers/codex/skills/linked/references/usage.md")) != "Supporting content\n" {
		t.Fatal("linked skill resources missing")
	}
	if get(t, filepath.Join(p.Source, "providers/codex/skills/legacy/SKILL.md")) != "Legacy user skill\n" {
		t.Fatal("legacy skill missing")
	}
	put(t, filepath.Join(packageDir, "SKILL.md"), "Package update\n")
	syncLocal(t, p)
	if get(t, filepath.Join(p.Source, "providers/codex/skills/linked/SKILL.md")) != "Package update\n" {
		t.Fatal("package change not imported")
	}
	put(t, filepath.Join(p.Source, "providers/codex/skills/linked/SKILL.md"), "Incoming edit\n")
	if _, e := p.Sync(SyncOptions{LocalOnly: true}); e == nil {
		t.Fatal("canonical edit wrote through a symlink")
	}
	if get(t, filepath.Join(packageDir, "SKILL.md")) != "Package update\n" {
		t.Fatal("package-managed target changed")
	}
	if _, e := os.Readlink(link); e != nil {
		t.Fatal("local symlink was replaced")
	}
}

func TestLinkedResourcesCannotTraverseExcludedStoresOrCycles(t *testing.T) {
	p := fake(t)
	root := filepath.Join(p.Home, ".agents/skills")
	put(t, filepath.Join(p.Data, "SKILL.md"), "Relai private sentinel\n")
	put(t, filepath.Join(p.Home, ".ssh", "SKILL.md"), "private sentinel\n")
	if e := os.MkdirAll(root, 0700); e != nil {
		t.Fatal(e)
	}
	for name, target := range map[string]string{"bad": filepath.Join(p.Home, ".ssh"), "loop": root, "relai": p.Data} {
		if e := os.Symlink(target, filepath.Join(root, name)); e != nil {
			t.Fatal(e)
		}
	}
	s := syncLocal(t, p)
	if len(s.Warnings) < 3 {
		t.Fatal("excluded linked resources not reported")
	}
	if _, e := os.Stat(filepath.Join(p.Source, "providers/codex/skills/bad/SKILL.md")); !os.IsNotExist(e) {
		t.Fatal("excluded linked data imported")
	}
}

func TestProviderProfilesRelayAcrossMachines(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, e := exec.Command("git", "init", "--bare", remote).CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	a, b := FakePaths(filepath.Join(root, "A")), FakePaths(filepath.Join(root, "B"))
	for _, p := range []Paths{a, b} {
		if e := p.Setup(SetupOptions{Remote: remote, Providers: Providers}); e != nil {
			t.Fatal(e)
		}
	}
	put(t, filepath.Join(a.Home, ".codex/AGENTS.md"), "Codex only\n")
	put(t, filepath.Join(a.Home, ".claude/CLAUDE.md"), "Claude only\n")
	put(t, filepath.Join(a.Home, ".config/opencode/AGENTS.md"), "OpenCode only\n")
	put(t, filepath.Join(a.Home, ".agents/skills/check/SKILL.md"), "Codex skill\n")
	put(t, filepath.Join(a.Home, ".claude/skills/check/SKILL.md"), "Claude skill\n")
	put(t, filepath.Join(a.Home, ".config/opencode/skills/check/SKILL.md"), "OpenCode skill\n")
	put(t, filepath.Join(a.Home, ".codex/config.toml"), "[features]\ncontext_management = true\n[mcp_servers.tools]\ncommand = 'codex-mcp'\n")
	put(t, filepath.Join(a.Home, ".claude.json"), `{"mcpServers":{"tools":{"command":"claude-mcp","args":["--readonly"]}}}`)
	put(t, filepath.Join(a.Home, ".config/opencode/opencode.json"), `{"mcp":{"tools":{"type":"local","command":["opencode-mcp"],"enabled":false}}}`)
	run := func(p Paths) {
		t.Helper()
		if _, e := p.Sync(SyncOptions{}); e != nil {
			t.Fatal(e)
		}
	}
	run(a)
	run(b)
	for path, text := range map[string]string{
		".codex/AGENTS.md": "Codex only\n", ".claude/CLAUDE.md": "Claude only\n", ".config/opencode/AGENTS.md": "OpenCode only\n",
		".agents/skills/check/SKILL.md": "Codex skill\n", ".claude/skills/check/SKILL.md": "Claude skill\n", ".config/opencode/skills/check/SKILL.md": "OpenCode skill\n",
	} {
		if get(t, filepath.Join(b.Home, path)) != text {
			t.Fatal("incorrect remote restore", path)
		}
	}
	for _, provider := range Providers {
		binding, _ := makeProviderBinding(b, provider, "providers/"+provider+"/mcp/tools.json", defaultManifest())
		d, e := openDocument(binding.Path, binding.Format)
		if e != nil {
			t.Fatal(e)
		}
		server, exists := getField(d.Map, binding.Field)
		if !exists || !strings.Contains(string(raw(server)), provider+"-mcp") {
			t.Fatal("MCP restore mixed providers", provider)
		}
	}
	put(t, filepath.Join(b.Home, ".claude/CLAUDE.md"), "Claude edited on B\n")
	run(b)
	run(a)
	if get(t, filepath.Join(a.Home, ".claude/CLAUDE.md")) != "Claude edited on B\n" || get(t, filepath.Join(a.Home, ".codex/AGENTS.md")) != "Codex only\n" {
		t.Fatal("provider-specific edit crossed providers")
	}
	ids, e := a.SnapshotIDs()
	if e != nil {
		t.Fatal(e)
	}
	run(a)
	run(b)
	after, e := a.SnapshotIDs()
	if e != nil || ids["local"] != after["local"] {
		t.Fatal("feedback commit", e)
	}
}

func TestAuxiliaryProviderFilesAndMultiplePlugins(t *testing.T) {
	p := fake(t)
	files := map[string]string{
		".codex/hooks.json":                  `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"notify-send done"}]}]}}`,
		".codex/rules/default.rules":         `prefix_rule(pattern=["git", "status"], decision="allow")`,
		".config/opencode/tui.json":          `{"theme":"dark","keybinds":{"leader":"ctrl+x"}}`,
		".config/opencode/plugins/notify.js": `export const Notify = async () => ({})`,
	}
	for path, data := range files {
		put(t, filepath.Join(p.Home, path), data)
	}
	put(t, filepath.Join(p.Home, ".config/opencode/opencode.json"), `{"plugin":["first-plugin","second-plugin"]}`)
	syncLocal(t, p)
	for native, key := range map[string]string{
		".codex/hooks.json":                  "providers/codex/hooks.json",
		".codex/rules/default.rules":         "providers/codex/rules/default.rules",
		".config/opencode/tui.json":          "providers/opencode/tui.json",
		".config/opencode/plugins/notify.js": "hooks/opencode/notify.js",
	} {
		if get(t, filepath.Join(p.Source, key)) != files[native] {
			t.Fatal("auxiliary file missing", key)
		}
	}
	syncLocal(t, p)
	put(t, filepath.Join(p.Source, "providers/codex/hooks.json"), `{"broken":`)
	if _, err := p.Sync(SyncOptions{LocalOnly: true}); err == nil {
		t.Fatal("malformed provider JSON accepted")
	}
	if get(t, filepath.Join(p.Home, ".codex/hooks.json")) != files[".codex/hooks.json"] {
		t.Fatal("malformed source replaced native hooks")
	}
}
