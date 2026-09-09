# Native format compatibility

Relai 1.0 targets Linux global configuration. Native references were checked on September 8, 2026. It does not wrap provider CLIs. This table is also the boundary for safe reverse import.

| Feature | Codex | Claude Code | OpenCode |
| --- | --- | --- | --- |
| Main instructions | `~/.codex/AGENTS.md` | `~/.claude/CLAUDE.md` | `$XDG_CONFIG_HOME/opencode/AGENTS.md` |
| Additional global rules | Native `~/.codex/rules/*.rules` and Markdown; shared rules use marked blocks | `~/.claude/rules/**/*.md` | Native `rules/**/*.md`; shared rules use marked blocks |
| Skills/resources | `~/.agents/skills/`, legacy `~/.codex/skills/` fallback | `~/.claude/skills/` | `$XDG_CONFIG_HOME/opencode/skills/` |
| Agents | `~/.codex/agents/*.toml` | `~/.claude/agents/*.md` | `$XDG_CONFIG_HOME/opencode/agents/*.md` |
| Commands | Explicit `relai-command-*` skills | `~/.claude/commands/*.md` | `$XDG_CONFIG_HOME/opencode/commands/*.md` |
| MCP | `config.toml`: `mcp_servers` | `~/.claude.json`: **top-level** `mcpServers` | `opencode.json[c]`: `mcp` |
| Hooks | `hooks.json` and supported `config.toml` hooks | `settings.json`: native `hooks` | Global `plugins/*.js` |
| Plugin declarations | `plugins.<id>.enabled` | `enabledPlugins` | `plugin` package list |
| Non-secret settings | Selected `config.toml` keys | Selected `settings.json` keys | Selected `opencode.json[c]` keys |

`XDG_CONFIG_HOME` defaults to `~/.config`. If both OpenCode JSON and JSONC files exist, Relai manages JSONC and leaves the JSON file local. Existing alternate roots and project/policy overrides may have higher precedence; `doctor` reports known override cases. Alternate `CODEX_HOME` and `CLAUDE_CONFIG_DIR` roots are refused by setup/sync so a foreground invocation and systemd cannot accidentally manage different roots. Legacy Codex skills are discovered when their name is absent from the primary skills directory. Provider marketplace databases and account-provided apps stay local.

New instructions, skills, rules and MCP servers remain provider-specific. MCP declarations preserve native options and environment references while excluding credential literals.

Unknown native fields are preserved in the original file and never copied into the canonical settings dictionary. Native agent restrictions remain local. Linked user skills/rules are imported as read-only snapshots with their text supporting files. Built-in hidden skills, binary resources, hard links, unsupported inline hook forms and unsupported settings stay local. A changed managed resource that becomes invalid stops reconciliation; an unowned invalid resource remains local with a warning.

Counts report entries directly managed for each provider, not all effective inherited configuration. OpenCode can also discover the [shared agent and Claude skill directories](https://opencode.ai/docs/skills/); those files are captured under their owning Codex/Claude profile when that provider is selected. Relai does not duplicate them into OpenCode’s directory.

Provider reload behavior belongs to the provider. Config changes are written immediately by Relai's next scan, but a running AI session may retain settings until its next session. MCP credentials and plugin package installation remain native provider responsibilities; Relai synchronizes declarations only.

## References

- Codex's global configuration keys, MCP tables, native hooks and plugin enabled flags: [configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference).
- Codex user skills and native agent files: [skills](https://learn.chatgpt.com/docs/build-skills) and [subagents](https://learn.chatgpt.com/docs/agent-configuration/subagents).
- Claude settings and its mixed user-level MCP file: [settings](https://code.claude.com/docs/en/settings), [MCP scopes](https://code.claude.com/docs/en/mcp), and [the .claude directory](https://code.claude.com/docs/en/claude-directory).
- OpenCode global configuration and MCP encodings: [config](https://opencode.ai/docs/config/) and [MCP servers](https://opencode.ai/docs/mcp-servers/).
- OpenCode resource locations and extension API: [skills](https://opencode.ai/docs/skills/), [agents](https://opencode.ai/docs/agents/), [commands](https://opencode.ai/docs/commands/), and [plugins](https://opencode.ai/docs/plugins/).

The Omarchy manifest and QML host contract were checked against the locally installed `omarchy-plugin-add`, `omarchy-plugin-validate`, `Ui/Panel.qml`, `Ui/KeyboardPanel.qml` and first-party widget examples under `/usr/share/omarchy/`. No files in that tree were modified.

## Release validation

The supported hook subset was checked against the current [Codex hooks reference](https://learn.chatgpt.com/docs/hooks) and [Claude hooks reference](https://code.claude.com/docs/en/hooks). Matcher values must be strings, `async` must be boolean, and timeouts must be positive numbers. Codex `SessionEnd` timeouts cannot exceed three seconds. Unsupported handler/group fields remain local and are reported; Relai does not broaden its allowlist automatically when providers add features.

Codex inline hooks remain in `config.toml`; separate `hooks.json` files synchronize in the Codex profile after JSON and content validation. Native Codex [agent files](https://learn.chatgpt.com/docs/agent-configuration/subagents) retain unknown fields while synchronizing description and developer instructions. The OpenCode [MCP reference](https://opencode.ai/docs/mcp-servers/) defines local command arrays and remote URLs used by the adapter.

Regression fixtures verify malformed declarations, unknown fields, exact JSON numeric values, and refusal to replace non-object containers. Automated tests do not authenticate to providers or invoke models. Runtime behavior beyond these documented surfaces remains the provider's responsibility.
