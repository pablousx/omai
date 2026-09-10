# omai

**Your AI setup, in sync.**

[Website](https://sites.steralynx.com/omai/) · [Privacy](site/privacy.html) · [Terms](site/terms.html)

omai is an Omarchy shell plugin that synchronizes global configuration for Codex, Claude Code, and OpenCode. One editable source preserves each provider’s native configuration across computers. A companion daemon imports supported provider-side edits, applies changes locally while offline, and synchronizes through an ordinary Git remote.

omai manages instructions, rules, text skills and resources, portable agents, commands, MCP declarations, supported hooks, plugin declarations, and selected non-secret settings. Projects, credentials, AI runtimes, and package installation remain outside omai.

![omai healthy panel](docs/screenshots/healthy.png)

## Install

Requirements: **Linux x86-64, Omarchy 4.0.3 or later with the Quattro plugin interface**, Git, curl, Python 3.11+, and a systemd user session. Setup uses your existing Git authentication. Normal synchronization needs the installed binary and Git; no Go compiler is required.

```sh
omarchy plugin add https://github.com/pablousx/omai.git --enable --yes
```

Click **✦** in the bar and choose **Set up omai**. Complete the form in the plugin window: enter an existing Git remote or leave it blank for local synchronization, name this computer, and choose an initial configuration. Click **Start syncing** to install the companion executable if needed and start automatic synchronization. Progress, errors, and retries stay in the panel; no terminal setup command is needed. omai never creates an external repository.

Use a dedicated remote you control. The synchronized instructions, hooks, skills, and plugin declarations can affect what AI tools execute. Only non-secret content belongs in that repository; secret detection cannot identify every disguised credential.

The plugin installer clones and enables the plugin. The panel's setup action downloads the exact manifest version over HTTPS and verifies its checksum, archive, and embedded version before installation. If that version is already installed, setup reuses it and can run offline. Cancelling the form preserves its draft until the shell reloads and starts no installation or synchronization.

For an unpublished local copy, the panel offers **Build local copy and retry** after an installation failure. This explicit option requires mise and the pinned Go compiler; it keeps setup inside the plugin. Developer installation and optional headless commands are documented in [Contributing](CONTRIBUTING.md#local-plugin-development).

To start setup again, open **Advanced → Clear settings (keeps configs)** and confirm. omai stops automatic sync and clears its connection settings and preferences. Your canonical configs, provider files, Git history, and recovery backups stay in place. The panel returns to the setup form.

## How it works

Edit `~/.config/omai/source/` or supported global provider files. omai scans every two seconds and attempts remote synchronization every thirty seconds. It compares semantic values against the last applied baseline, preserves unknown native values, and avoids feedback commits from its own writes. Git operations have time and output limits; unavailable remotes preserve local work for retry. Running AI sessions may need restarting to reload their configuration.

| Location | Contents | Synchronized? |
| --- | --- | --- |
| `~/.config/omai/config.json` | Remote, branch, provider selection, local labels, intervals, retention | No |
| `~/.config/omai/source/` | Canonical human-editable configuration | Validated files only |
| `~/.local/share/omai/bin/omai` | Installed binary and its `.previous` backup | No |
| `~/.local/share/omai/repository.git` | Local Git transport repository | Source commits only |
| `~/.local/state/omai/` | Baselines, conflicts, recovery journals, installation records | Never |

omai honors `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, and `XDG_STATE_HOME`. Machine and peer names are local annotations, not presence tracking. Git commits use `omai <omai@localhost>`.

The source can contain:

```text
source/
├── omai.json
├── providers/codex/instructions.md
├── providers/codex/skills/review/SKILL.md
├── providers/codex/rules/default.rules
├── providers/claude/instructions.md
├── providers/claude/mcp/documents.json
├── providers/opencode/tui.json
├── agents/reviewer.md
├── commands/check.md
├── hooks/opencode/notify.js
└── personal/editor.json
```

Discovered instructions, skills, rules, and MCPs stay scoped to their provider. Settings remain provider-specific in `omai.json`. The panel reports instruction, setting, skill, and MCP counts for each provider. Existing explicitly shared entries remain supported. An empty setup is valid. Examples are opt-in. See the [format guide](docs/format.md), [sample source](examples/source), and [native compatibility notes](docs/compatibility.md).

## Commands and panel

| Command | Behavior |
| --- | --- |
| `omai inventory` | Relevant file metadata only |
| `omai status [--json]` | Health, daemon state, providers, conflicts, backup usage |
| `omai doctor [--json]` | Diagnose configuration, compatibility, recovery, and service issues |
| `omai sync [--local]` | Reconcile now; `--local` skips Git |
| `omai conflicts [--json]` | List conflicts and available choices |
| `omai conflicts --show KEY` | Inspect saved conflict versions |
| `omai resolve KEY --take CHOICE` | Record a decision and retry synchronization |
| `omai rollback [ID]` | Restore configuration preimages and pause synchronization |
| `omai backups list [--json]` | Inspect retained and cleanup-eligible history |
| `omai backups prune [--dry-run] [--json]` | Apply or preview retention cleanup |
| `omai daemon run` | Foreground daemon; exits on SIGINT/SIGTERM |
| `omai daemon install\|start\|stop\|restart` | Install or control the user service |
| `omai daemon pause\|resume` | Pause or resume reconciliation |
| `omai daemon uninstall` | Disable the service and remove omai-owned service/launcher files |
| `omai install --recover` | Restore an interrupted binary/service installation |
| `omai install --rollback` | Restore the previous installation |
| `omai personal add NAME --source personal/FILE --path '~/destination'` | Enroll an existing non-secret text file |

The panel leads with sync health and the next useful action: setup, sync, reconnect, resume, or start automatic syncing. **Sync details** expands provider coverage, computer labels, timestamps, and items kept local. **Advanced** contains inline checks and recovery history, updates, stopping the watcher, undo, and reset. Conflict versions can be reviewed in the panel; choosing a version, undoing an apply, and clearing settings require confirmation.

Use arrows or `h/j/k/l` to move between buttons, Tab/Shift-Tab to follow their reading order, and Enter/Space to activate. Escape cancels a confirmation or returns from details before closing the panel. `r` refreshes the current view; `s` syncs from the dashboard when unpaused. Disabled controls are skipped. Setup keeps its draft on cancellation and validates the remote before starting. Operations show progress and continue if you close the panel. See the [UX review](docs/ux-review.md).

Exit codes: `0` success, `1` validation/setup/other error, `2` unresolved conflicts, `3` delayed remote synchronization. `status` remains readable when something needs attention; `doctor` returns `1` when it finds an issue.

## Conflicts and recovery

Concurrent edits are preserved and require an explicit choice. Git merges independent files; different changes to the same file require a decision. omai never selects a winner by timestamp, inserts conflict markers, or force-pushes.

```sh
omai conflicts --show git/instructions.md
omai resolve git/instructions.md --take local
# Alternatively: --take remote
```

Provider conflicts offer their provider names and `canonical`. Decisions remain valid only while their saved variants still match. Remote history rewrites are reported as `git/history` and require restoring the history or deliberately choosing a new branch.

Every application records durable preimages before writes. Linux directory descriptors prevent following symlinks during writes, and an operation lock serializes changes. Interrupted transactions recover before another sync. Recovery and rollback refuse to overwrite newer external edits.

Rollback pauses synchronization so you can inspect the restored configuration. Resume with `omai daemon resume`; the restored source becomes an ordinary new Git commit.

Recovery history is cleaned automatically at most once per day after a successful sync: defaults are 30 days, 100 journals, and 512 MiB. Pending or unreadable journals and the latest rollback generation are always protected. Protected data can exceed the limits; status and doctor report this. [Operations](docs/operations.md) explains inspection and cleanup.

## Updates and removal

```sh
omarchy plugin update pablousx.omai
```

Then choose **Update omai** in the panel. The widget identifies a plugin/binary version mismatch and disables configuration-changing actions until it is resolved. Updates preserve paused/stopped state and restart only a running daemon. Failed activation restores the previous binary, launcher, service, and service state. Retry setup to recover an interrupted installation; an installed 1.x binary also supports `omai install --recover` offline.

To stop synchronization and remove the shell plugin:

```sh
omai daemon uninstall
omarchy plugin remove pablousx.omai
```

Source, provider files, the installed binary, Git history, and recovery data remain available. Disabling or removing only the widget does not stop its separate daemon.

## Supported boundaries

- Linux global configuration only; alternate Codex/Claude config roots are refused. Project and policy-managed overrides remain local.
- Portable agents include description and prompt. Unknown native restrictions stay local. Codex commands become explicitly invoked `omai-command-*` skills.
- Hooks retain their provider-specific schemas. omai supports the documented command-hook subset and OpenCode JavaScript plugin files.
- Settings use an allowlist. Unknown values remain local, including precise JSON numbers. JSONC/TOML comments may be reformatted when managed values change.
- Resources and enrolled personal files must be UTF-8 text, at most 2 MiB each. Canonical source is limited to 16 MiB and 2,048 files. Linked user skills and rules are imported as ordinary file snapshots; their local package targets remain read-only. Changes from another computer that would replace a linked target require attention. Canonical symlinks, hard links, binary resources, built-in skills, credential stores, histories, sessions, caches, and machine identifiers are excluded.

## Development and publishing

```sh
mise run check
mise run audit
mise run qml                  # Native Omarchy/Wayland session required
mise run release              # Deterministic binary archive and SHA256SUMS
```

Tests use disposable homes, local bare remotes, and fixture services. They do not change your AI configuration or publish source. See [architecture](docs/architecture.md), [validation](docs/validation.md), [contributing](CONTRIBUTING.md), and the [publishing runbook](docs/publishing.md).

The responsive HTML website lives in `site/`. See [website preview and GitHub Pages publishing](docs/website.md).
