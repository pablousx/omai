# Validation record

Validated Relai 1.0.0 on Linux x86-64 with Go 1.26.6 and the installed native Omarchy shell on 2026-09-08. All installation, synchronization, and recovery checks used disposable homes and repositories.

| Check | Result |
| --- | --- |
| `scripts/check` | Passed: release metadata, formatting, race tests, vet, shell/workflow checks, fresh build, end-to-end tests, installer tests, and native manifest validation |
| `go test -race ./...` | Passed, including transaction/install locks, interrupted recovery, retention limits and protected journals, version-1 data, corrupt state, and failed status/timestamp writes |
| `go vet ./...` | Passed |
| `python3 scripts/test-e2e.py build/relai` | Passed against the final binary: two actual daemons, automatic watching/import/export, offline retry, explicit conflicts, no feedback commits, rollback and excluded-store sentinels |
| `python3 scripts/test-install.py build/relai` | Passed: verified and unavailable downloads, bad checksums/archives/versions, custom XDG paths, read-only plugin, concurrent installation, failed activation, a killed updater and offline recovery, paused/stopped service preservation, unrelated files, and safe removal |
| `python3 scripts/test-qml.py --screenshots build/qml-preview` | Passed all 19 native scenarios, including inline setup, cancellation with draft preservation, duplicate submission rejection, source retry after failure, background update, Advanced clear-settings confirmation/cancellation and failure, health/error states, and keyboard conflict actions; fixture screenshots also verify the panel opens at the top |
| Provider profiles | Passed: two computers restore distinct instructions, same-named skills and native MCP servers for all three providers; no feedback commits; provider edits stay scoped; nested settings and local credential preservation; linked/legacy skills, excluded paths/cycles, auxiliary JSON and multiple plugin declarations |
| Clear settings | Tests verify service stop/disable, preservation of canonical/provider files and recovery/transport state, refusal while busy or with unsafe targets, clean setup status, and setup again with existing source |
| Plugin setup bridge | Fresh installation and service activation without stdin passed in a disposable home; same-version setup succeeded with downloads unavailable; failed update preserved the executable and explicit source retry succeeded |
| Real source bootstrap | `scripts/setup --source --build-only` built with the pinned compiler and a disposable home; no service activation, checkout mutation, or leftover module cache |
| `systemd-analyze --user verify` on the generated service | Passed with a disposable home/runtime; service control was stubbed, so no real service was enabled |
| Shell syntax and ShellCheck 0.11.0 | Passed for the bootstrap, terminal action, check, and launcher scripts |
| actionlint 1.7.12 | Passed for both GitHub Actions workflows |
| govulncheck 1.8.0 | No vulnerabilities found in `./...` |
| `omarchy plugin validate /home/pablousx/dev/relai` | Passed |
| Release packaging | Two builds produced identical archive checksums; archive contents, static ELF binary, embedded version, and `v1.0.0` tag agreement passed validation |
| Extracted release artifact | End-to-end synchronization and installer lifecycle tests passed against the extracted binary |
| Documentation | Local Markdown links checked; setup, inline form, healthy, and conflict fixture screenshots reviewed |

The prepared distribution is `build/release/relai_1.0.0_linux_amd64.tar.gz`, with `build/release/SHA256SUMS`. The archive contains the binary and MIT license. Build outputs are ignored by Git. Reviewed fixture screenshots are in [screenshots/](screenshots/).

QML tests require a Wayland compositor because Omarchy's `KeyboardPanel` uses the native PanelWindow backend. They use temporary homes/runtime directories and fixture status responses, briefly open fixture panels for screenshots, and never change shell settings. Screenshots capture only the fixture panel. On systems with restrictive execution sandboxes, Quickshell and systemd's validator may need permission for local socket operations.

The tests did not modify real Codex, Claude Code or OpenCode configuration, enable the user's daemon, publish data or create an external repository. Native adapters were verified through their parser/serialization round trips and documented formats; the suite does not authenticate to providers or invoke AI models.

GitHub-hosted CI and the release workflow have not run remotely. The workspace does not contain usable Git metadata, so repository identity, credentials, branch protection, and the mandatory native Omarchy runner still require verification during [publishing](publishing.md). No release or marketplace submission was published; live public-download and installation checks follow publication.

Expanded discovery was also previewed against copies of this computer’s global files in a disposable home: two Codex skills with nine text files, Codex hooks and rules, five Claude hook events, three Codex preferences, and one OpenCode JavaScript plugin. No global instruction file or direct native MCP declaration existed in the inspected roots. This preview did not contact the configured remote.

After the disposable-home checks, the verified update was installed into this user’s existing Relai plugin. The running daemon imported 13 canonical files totaling 72,332 bytes and reported healthy remote synchronization with no pending changes. Relai settings and the inspected native provider files were unchanged by installation. No reset was performed.
