# Canonical format (version 1)

`source/omai.json` is strict JSON. Instructions and resources are ordinary separate text files; there is no generated configuration that you have to edit in the plugin checkout.

```json
{
  "version": 1,
  "settings": {
    "codex": { "model_reasoning_effort": "high" },
    "claude": { "language": "English" },
    "opencode": { "autoupdate": false }
  },
  "mcp": {
    "documents": {
      "command": "documents-mcp",
      "args": ["--readonly"],
      "env": ["DOCUMENTS_ROOT"]
    },
    "remote": {
      "url": "https://example.com/mcp",
      "bearer_env": "MCP_BEARER"
    }
  },
  "hooks": {
    "claude": {
      "Stop": [{ "hooks": [{ "type": "command", "command": "printf 'finished\\n'" }] }]
    }
  },
  "plugins": {
    "codex": { "example@marketplace": { "enabled": true } },
    "claude": { "example@marketplace": true },
    "opencode": { "opencode-example-plugin": true }
  },
  "personal": {
    "editor": { "path": "~/.config/my-editor/preferences.json", "source": "personal/editor.json" }
  }
}
```

Examples above are declarations, not recommendations to install packages. Empty objects are valid. The first setup creates empty declarations. Personal mappings must refer to a source file and use distinct source/destination paths.

## Provider-specific resources

Newly discovered native resources use `providers/PROVIDER/`, where `PROVIDER` is `codex`, `claude`, or `opencode`:

- `instructions.md` restores to that provider’s global `AGENTS.md` or `CLAUDE.md`.
- `skills/NAME/` includes the skill and its text supporting files.
- `rules/` preserves native Markdown rules and Codex `.rules` files, including nested paths.
- `mcp/NAME.json` preserves that provider’s native server declaration.
- Codex also includes `AGENTS.override.md` and `hooks.json`; OpenCode includes `tui.json`.

The same resource name in two providers stays independent. Codex discovers `~/.agents/skills` first and falls back to `~/.codex/skills` for names absent from the primary directory. Hidden built-ins are excluded. Linked user skills/rules are snapshotted as text, including supporting files, without changing their package targets. On a computer without the link, restoration creates ordinary files. A remote edit cannot overwrite a local linked target; omai reports that boundary instead.

Existing common entries below remain shared deliberately. Do not combine shared and provider-specific declarations that target the same native file or setting. Update omai on every participating computer before using the expanded provider paths; earlier binaries safely reject unfamiliar paths.

## Instructions and rules

`instructions.md` is shared global prose. `rules/*.md` are additional global rules. Claude receives separate rule files. Codex and OpenCode receive the rules as clearly delimited blocks after the main instructions in `AGENTS.md`.

Editing the main prose or a rule block imports that portion back. Keep the `<!-- omai:rule NAME -->` delimiters intact; malformed delimiters block reconciliation. New unmarked global instruction files are discovered in the provider-specific location above. Existing shared bindings retain this block-based behavior.

## Skills and resources

A skill lives at `skills/NAME/SKILL.md`, with its ordinary supporting files beneath the same directory. Use each tool's common `name` and `description` frontmatter. Preserve relative resource paths. An executable source resource becomes executable for each provider; other permission bits remain machine-local. The `omai-command-` skill prefix is reserved for translated Codex commands.

UTF-8 scripts, Markdown, JSON, YAML, templates, SVG and other text resources are supported. Binary files and excluded cache/history/credential paths are reported and stay local. omai does not install a skill's runtime or dependencies.

## Agents and commands

Portable agent files use only `description` frontmatter and a prompt body:

```markdown
---
description: Review changes for correctness
---

Find concurrency bugs and explain concrete failure cases.
```

Codex receives a native agent TOML file with `name`, `description` and `developer_instructions`. Claude/OpenCode receive Markdown agents. Existing native options remain in their original file; those options are not inferred or translated between providers.

Commands are files under `commands/NAME.md`. Claude/OpenCode receive native command files. Codex receives an explicit skill at `~/.agents/skills/omai-command-NAME/SKILL.md`; invoke `$omai-command-NAME`. The generated skill header is owned by omai. Edit its body to change the portable command, or edit the canonical command directly. Native placeholder and permission syntax inside command prose may be provider-specific; use simple portable instructions when sharing a command.

## MCP

For shared `omai.json` MCP entries, a server has exactly one `command` or HTTP(S) `url`. Local commands must be PATH executables; use mise to provision them. `args` is an ordered string array. URLs may not contain credentials, queries or fragments. Only stdio and HTTP transports are portable in omai 1.0.

`env` is a list of environment variable names, sorted during normalization. Codex receives `env_vars`; Claude receives `${NAME}` references; OpenCode receives `{env:NAME}` references. `bearer_env` uses the provider's environment-based bearer-header representation. omai never expands these names into credential values. Keep values in each tool's ordinary local environment or authentication store.

Newly discovered `providers/PROVIDER/mcp/NAME.json` declarations retain native command arrays, transport types, enabled flags, timeouts, and other non-secret options without translating them across providers. Credential literals are omitted and preserved locally; environment references are retained. Provider-specific declarations allow native absolute command paths, which must also exist on the receiving computer. Credential command arguments and credential-bearing URLs are rejected. omai does not test MCP connections or launch servers.

## Hooks and plugins

`hooks.codex` and `hooks.claude` are separate native event dictionaries. Supported events: `SessionStart`, `SessionEnd`, `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `Stop`, `PreCompact`, `PostCompact`, `SubagentStart`, `SubagentStop`, `PermissionRequest`. Matcher groups accept `matcher` and `hooks`; command handlers accept `type`, `command`, `timeout`, `async`. Unsupported event/handler/field forms are reported. Matchers must be strings, `async` must be boolean, and timeouts must be positive numbers; Codex `SessionEnd` timeouts are limited to three seconds.

Codex’s separate `hooks.json` is also discovered and preserved as a provider-specific JSON file, subject to content validation.

OpenCode's event API differs. Put a native `.js` plugin under `hooks/opencode/`; omai installs it in the global `plugins/` directory and automatically discovers existing global `.js` plugins and imports edits to managed files. omai itself never executes hooks.

Plugin declarations are provider-specific identifiers. Codex maps to the `plugins` TOML table's enabled flag; Claude maps to `enabledPlugins`; OpenCode maps to the `plugin` package list. Remove an OpenCode entry to disable it; its native list has no false entry. omai does not install marketplaces, fetch plugin packages or copy plugin caches. The providers retain their native plugin loading behavior.

## Settings allowlist

| Provider | Supported keys |
| --- | --- |
| Codex | Model/reasoning, approvals/sandbox, instructions, context limits, notifications, display, feature flags, TUI preferences, shell environment policy, skill configuration, profiles, and model-provider options |
| Claude | Model/effort, language/output, permissions, attribution, status line, sandbox, display preferences, update channel, MCP enablement preferences, skill overrides, and non-secret environment values |
| OpenCode | Models, default agent, theme, updates/sharing, instructions, permissions, agents/commands, formatters/LSP, watchers, provider options, compaction, tools, and experimental preferences; separate `tui.json` |

The exact root allowlists are in `internal/omai/safety.go`. Structured Codex settings, Claude environment settings, and OpenCode provider options use dotted leaf keys in `omai.json`, so local credentials can remain untouched beside synchronized preferences. Values may be strings, numbers, booleans, arrays, or objects as appropriate. Project trust, account state, model-availability onboarding state, and unknown roots remain local. Each provider’s model names stay scoped to that provider; omai does not guess equivalence between models.

## Personal files

Enroll one existing, non-secret file explicitly:

```sh
omai personal add editor \
  --source personal/editor.json \
  --path '~/.config/my-editor/preferences.json'
```

The destination is home-relative. The source is copied into the canonical `personal/` directory after validation. Later edits flow both ways. Destinations cannot overlap omai state, managed AI roots, credential stores, plugin installation directories or the public binary. Directory/glob enrollment is deliberately unavailable.

Deleting a managed personal file, or its canonical source file, propagates the deletion. omai retains the mapping with `"deleted": true` so a newly attached computer cannot silently resurrect the file. A conflicting pre-existing file on a new computer requires a decision. Adding the source file again clears the tombstone.

To stop managing a personal file while keeping the destination, remove both its mapping and canonical file. omai leaves the destination in place. Keep the source and mapping together when editing.
