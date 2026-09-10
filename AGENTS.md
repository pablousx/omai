# Maintaining omai

omai is a Linux Omarchy Quattro plugin plus a Go companion that synchronizes supported global AI-tool configuration. Start with [README.md](README.md) for product behavior. Instructions here apply throughout this repository; the current user's request and higher-priority instructions take precedence.

## Project identity and task routing

- Repository: `pablousx/omai`; plugin ID: `pablousx.omai`; entry point: `qml/Omai.qml`.
- Website: `https://pablousx.github.io/omai/`; source: `site/`.
- Read the current version from `manifest.json` and the Go pin from `mise.toml`. Do not assume a version, release status, installed state, or marketplace approval from an earlier session.
- Repository skills are maintained in `skills/`. Read only the relevant skill and its needed references, even if your host does not list these skills automatically. Command examples assume this checkout's root.

| Task | Skill |
| --- | --- |
| Go engine, provider adapters, QML, tests, refactoring | [omai-development](skills/omai-development/SKILL.md) |
| Broken sync, conflicts, UI failures, installation recovery | [omai-debugging](skills/omai-debugging/SKILL.md) |
| New release, publishing, updating a marketplace listing | [omai-release](skills/omai-release/SKILL.md) |
| Install or update omai on a user's desktop | [omai-local-install](skills/omai-local-install/SKILL.md) |
| Landing/policy pages, browser checks, GitHub Pages | [omai-website](skills/omai-website/SKILL.md) |

A request to update the published plugin normally concerns a new release; a request to update the local installation concerns the installed checkout and companion. Use conversation context to select the intended operation.

## Working boundaries

Inspect `git status -sb` before editing. Preserve unrelated changes. When a clean checkout is behind upstream, inspect the incoming commits before a fast-forward. Use the repository's PR and squash-merge process in [GITHUB_SETUP.md](GITHUB_SETUP.md); `Repository checks` and `check` are required. The maintainer bypass is not the default workflow.

Creating or revising instructions does not itself authorize publication, local service activation, marketplace messages, or changes to repository settings. Follow the user's existing authorization without asking again for already-authorized work. Complete reversible preparation and checks before any missing approval. Do not interpret a release, website deployment, marketplace submission, and local installation as interchangeable actions.

Develop in this checkout and test in disposable homes. Installed copies under `~/.config/omarchy/plugins/` are watched by the running shell. Do not edit the user's installed plugin or global AI configuration as a shortcut for testing. For an authorized local desktop change, use the available Omarchy skill and installed `omarchy` commands. Read `/usr/share/omarchy/` as reference; do not modify it.

## Code map

| Area | Files and reference |
| --- | --- |
| CLI and JSON contract | `cmd/omai/main.go`, `internal/omai/model.go`, `scripts/omai` |
| Reconciliation, baselines, conflicts | `internal/omai/engine.go`, `conflicts.go`, [architecture](docs/architecture.md) |
| Native provider bindings and discovery | `internal/omai/adapters.go`, `profiles.go`, `document.go`, [compatibility](docs/compatibility.md) |
| Source schema and validation | `internal/omai/model.go`, `files.go`, `safety.go`, [format](docs/format.md) |
| Git transport | `internal/omai/git.go` |
| Transactions and retention | `internal/omai/files.go`, `nofollow_linux.go`, `backups.go` |
| Setup, service, binary lifecycle | `internal/omai/setup.go`, `install.go`, `settings.go`, `scripts/setup`, `scripts/plugin-setup` |
| Native desktop UI | `qml/Omai.qml`, `qml/SetupForm.qml`, [UX review](docs/ux-review.md) |
| Distribution | `manifest.json`, `.github/workflows/`, `scripts/*release*.py`, [publishing](docs/publishing.md) |
| Runtime diagnosis and recovery | [operations](docs/operations.md), [security reporting](SECURITY.md) |

## Invariants to preserve

- Global configuration only. Project overrides, credential stores, sessions, histories, caches, and provider marketplace/account databases remain outside sync.
- Newly discovered content stays scoped to its provider. Explicitly shared source entries remain supported. Preserve unknown native values and exact JSON numbers; preserve semantics rather than promising comment/format retention.
- Only validated canonical source enters Git. Local preferences, baselines, conflicts, recovery journals, and installation records never become sync inputs. Journals can contain secret fields from mixed native files.
- Validate a complete plan before writes. Preserve preimages, file modes, operation/install locks, durable journals, and refusal to overwrite newer external edits. Do not weaken symlink/hard-link defenses or size/output/time limits.
- Local reconciliation precedes remote sync. Retain offline work; preserve concurrent variants for explicit decisions. No timestamp-selected winners, force pushes, or history rewrites to resolve conflicts.
- Setup requires the user's chosen configuration. Updates preserve paused/stopped state. Removing the widget does not stop its daemon. Clearing settings, configuration rollback, and installation rollback are different operations.
- Keep the plugin ID and local paths stable across releases. Release SemVer is separate from canonical/local schema version `1`; schema changes need deliberate migration behavior and compatibility fixtures.
- QML invokes public CLI argument arrays and consumes status JSON. Keep configuration parsing and writes in Go. Preserve inline setup, accessible keyboard navigation, explicit conflict/undo confirmations, and nonblocking operation feedback.

## Verification proportional to the change

Use the compiler in `mise.toml`; `mise run ...` selects it. No provider login, paid model call, real Git sync destination, or user's service is needed for automated tests.

| Change | Relevant checks |
| --- | --- |
| Go behavior | Focused regression test, then `mise run check` |
| QML or status contract | `mise run qml`, `mise run validate`; CLI checks when the contract changes |
| Bootstrap, service, recovery | `mise run check`; `python3 scripts/test-install.py build/omai --verify-unit` |
| Workflow/repository settings | Commands in `GITHUB_SETUP.md` plus pinned actionlint from CI |
| Static site | `python3 scripts/check-site.py`, `node --check site/assets/site.js`, relevant browser checks |
| Guides/skills only | Skill frontmatter, metadata, local links, repository checker, and `git diff --check`; no application rebuild solely for prose |
| Release | Full check, vulnerability audit, native QML, packaging, and public-distribution checks in the release skill |

`scripts/check` is the combined local gate; CI additionally makes ShellCheck/workflow lint mandatory, checks the generated unit, audits dependencies, and verifies extracted release artifacts. Local QML tests are not a substitute for the mandatory native job on the exact release tag. Report what ran and distinguish automated fixtures from actual installed-service verification.

## Maintaining these instructions

Keep cross-cutting rules here, task workflows in the relevant skill, and product details in existing docs. Change commands when their implementation changes; link to current sources rather than copying entire manuals. Keep project skills as ordinary tracked files: `scripts/check-release.py` rejects symlinks inside the distribution. Optional local skill-discovery links belong outside this repository; see [Contributing](CONTRIBUTING.md#agent-maintenance-guides).

Do not store credentials, machine-specific runtime state, runner tokens, real provider content, or temporary validation artifacts in guides or skills. Update release/validation records with dates and evidence, and recheck external publishing procedures before relying on them.
