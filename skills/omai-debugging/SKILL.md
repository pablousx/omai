---
name: omai-debugging
description: Diagnose omai sync, provider import, conflicts, native panel, daemon, and installation failures. Use for debugging pablousx/omai or an installed omai instance while preserving configuration and recovery evidence.
---

# Debug omai

Read [AGENTS.md](../../AGENTS.md) and the relevant [operations](../../docs/operations.md) section. Resolve the physical skill directory if it is linked into a local skill catalog. Identify whether the failure concerns the development checkout, an installed plugin, or its separate companion/service.

## Establish the failing layer

Collect a minimal, relevant status summary; do not dump provider files or the user's environment into the transcript. Use the installed companion directly when the public launcher is unavailable:

```sh
omarchy version
omarchy plugin list --json
omai version
omai status --json
omai doctor --json
systemctl --user show omai.service --property=LoadState,ActiveState,UnitFileState
journalctl --user -u omai.service --since '-10 minutes' --no-pager
```

Before setup, `omai` may not be on PATH; the executable is `${XDG_DATA_HOME:-$HOME/.local/share}/omai/bin/omai`. Filter status/plugin lists and logs to the failing fields. Paths, messages, and remote errors can still be private. Do not print `systemctl --user show-environment`, credentials, `.claude.json`, source trees, or recovery journals wholesale.

`doctor` returning `1` means it found an issue, not that diagnosis failed. CLI results distinguish `1` validation/setup failures, `2` conflicts, and `3` delayed remote synchronization. `status` remains useful when attention is required. Read the command implementation if a result contradicts these expectations.

| Symptom | First investigation |
| --- | --- |
| `setup`, `configured=false` | Normal first-run state; do not start importing configuration without setup choices |
| Plugin/binary mismatch | Compare installed manifest with the directly invoked companion; update both layers |
| Offline/pending | Check the chosen remote's reachability and local authentication; preserve local commits |
| Conflict | Inspect the named saved variants locally; a user decision is required to choose content |
| Paused or stopped | Determine whether intentional; opening the panel must not resume/start it |
| Unexpected provider import | Reproduce with a minimal fake native file; check scope, exclusions, allowlists, and override roots |
| Recovery refusal | Preserve target files and pending journals; investigate newer external edits before retry |
| Missing bar icon / rescan timeout | Check actual plugin discovery and bar placement; an install may have partly succeeded |

## Separate shell access problems from plugin defects

An execution sandbox can block the user bus or Wayland socket and make commands report that the shell is not running. Retry the relevant read with authorized local socket access before concluding that Omarchy crashed.

After a rescan timeout, first check whether the process is still running and whether the manifest, plugin catalog, and `~/.config/omarchy/shell.json` already contain omai. The shell can recover without a restart. A bounded retry that worked during installation is:

```sh
env OMARCHY_SHELL_IPC_TIMEOUT=10s omarchy plugin list --json
env OMARCHY_SHELL_IPC_TIMEOUT=10s omarchy plugin enable pablousx.omai
```

The second command is a mutation: use it when enabling/repairing the installation is within the request. Do not repeatedly clone, add duplicate bar entries, reset shell settings, or assume `active=false` alone proves a bar widget failed; check placement and actual behavior. Use the available Omarchy skill before changing desktop state. If a process actually dumped core, use the available crash-diagnosis skill; a timeout alone is not evidence of a crash.

## Reproduce and repair

Move the failing case into disposable homes and local bare remotes. Use `internal/omai/*_test.go` for adapter/transaction issues, `scripts/test-e2e.py` for daemon interactions, `scripts/test-install.py` for lifecycle failures, and `scripts/test-qml.py --state NAME` for panel behavior. For a code fix, continue through [development](../omai-development/SKILL.md).

Keep recovery actions distinct:

- `omai install --recover`: finish recovery of an interrupted binary/service installation.
- `omai install --rollback`: restore the last recorded installation, preserving its service state.
- `omai rollback [ID]`: restore a configuration transaction and pause synchronization.
- `omai settings clear --yes`: stop/disable sync and clear preferences; retain configuration and recovery data.

These commands change state. Use the user's requested recovery objective; do not substitute clearing settings for diagnosis or choose conflict variants automatically. Do not remove journals, force-push, overwrite newer edits, or disable validation to make a symptom disappear. If a required decision or external dependency is missing, explain that specific blocker and preserve a reviewable reproduction.

Report the cause supported by evidence, the repair, verification, and any remaining uncertainty. Distinguish fixture tests, public-download tests, and tests of the user's actual running service.
