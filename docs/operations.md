# Operations

## Local configuration

`~/.config/relai/config.json` stays local. Existing version-1 configuration remains compatible:

```json
{
  "version": 1,
  "remote": "git@example.com:personal/ai-setup.git",
  "branch": "main",
  "machine": "This computer",
  "peers": ["Other laptop"],
  "providers": ["codex", "claude", "opencode"],
  "poll_seconds": 2,
  "sync_seconds": 30,
  "retention": {"days": 30, "count": 100, "bytes": 536870912}
}
```

Omit `providers` for automatic detection from executables and global configuration directories. An explicit list can manage a provider before its executable is installed. Removing a provider leaves its files in place. Machine and peer labels are annotations; no presence records or hardware identifiers are exchanged.

The daemon reloads configuration each pass. Intervals default to two and thirty seconds, accept up to 86,400 seconds, and use defaults when below one. Git operations time out after twenty seconds each; a slow network operation can delay the next local scan. Git subprocess output is bounded and transport diagnostics never reproduce credential-bearing URLs.

The source format and local configuration version remain `1` in Relai 1.0. Missing retention fields, or zero values, select the defaults. Negative limits are rejected. Retention days cannot exceed 36,500.

Changing the remote or branch is an explicit edit to local configuration. Relative local remote paths, credential-bearing URLs, arbitrary remote helpers, and invalid branch names are rejected. Use a dedicated repository containing only Relai's source format.

## Git authentication

Set up your normal Git/SSH credentials before enabling remote synchronization. Relai does not collect passwords, access tokens, or private keys. HTTPS credential helpers and SSH agents remain local. The daemon cannot answer terminal prompts.

For authentication failures, check access to the dedicated repository from your normal terminal. Ensure the systemd user environment contains the agent socket your SSH setup uses. Inspect `systemctl --user show-environment` locally; do not attach its output to bug reports because environment variables may contain secrets. A remote outage leaves local changes queued for the next retry.

## Setup and daemon lifecycle

The service lives at `$XDG_CONFIG_HOME/systemd/user/relai.service`. It runs the binary from the Relai data directory with a private umask, `NoNewPrivileges`, bounded stop time, and a restart policy. Its PATH includes the user's mise shims and local commands. Relai does not enable systemd lingering.

Open the bar plugin, choose **Set up Relai**, complete the inline form, and click **Start syncing**. The panel installs the matching executable if necessary, saves configuration, and starts the service. Opening or cancelling the form has no setup side effects. The following commands are optional administration interfaces:

```sh
relai daemon start
relai daemon stop
relai daemon restart
relai daemon pause
relai daemon resume
systemctl --user status relai.service
journalctl --user -u relai.service
```

If setup saved configuration but service installation failed, the form stays open with the error and your choices so you can retry. Setup will not silently replace an existing remote or unrelated launcher/service file. After closing the form, **Start daemon** repairs a missing service from saved configuration. The CLI still accepts `relai setup` for terminal administration; `--yes` is required with unattended setup flags.

Disabling the widget leaves the daemon running. A stopped daemon is never automatically restarted by opening the panel. Start it explicitly after inspecting the configuration.

### Clear settings while keeping configs

Choose **Advanced → Clear settings (keeps configs)** in the panel, then confirm **Clear settings**. This disables and stops the Relai user service, removes local `config.json` and the pause preference, and returns to inline setup. Remote/branch selection, computer/peer labels, provider selection, intervals, and retention preferences return to defaults on the next setup.

Canonical files in `source/`, all provider files, the Git transport repository, reconciliation baselines/conflicts, recovery journals, installed executable, and plugin remain. The operation does not roll back or rewrite configuration files. Setup reuses the existing source. Cancelling the confirmation changes nothing.

The backend requires `relai settings clear --yes` for automation. It serializes against installation and synchronization, refuses an unfinished installation recovery or unrelated service, and refuses to reset while a foreground daemon still holds its lock. If stopping the service fails, settings are retained. A failed reset can be retried; it does not automatically restart synchronization.

## Binary updates and recovery

Update the plugin checkout through `omarchy plugin update pablousx.relai`, then select **Update Relai**. Installation runs inside the panel without a terminal. The exact manifest version is downloaded over HTTPS; checksum, archive-member, size, and binary-version validation happen before installation. A missing release produces an error without replacing the installation. For unpublished local copies, **Build local copy and retry** explicitly selects a source build using mise and the pinned compiler.

The updater writes `state/install-pending.json` before stopping a running daemon or changing files. The record covers binary, previous binary, launcher, unit, and prior running/enabled state. Installation is serialized separately from configuration transactions. Successful updates restart an already-running daemon and preserve paused/stopped state. Activation failure restores the preimages; external edits block restoration instead of being overwritten.

Retry the setup action after interruption. With an installed 1.x executable, recovery also works offline:

```sh
relai install --recover
relai install --rollback
```

`--recover` restores an unfinished installation. `--rollback` restores the last actual installation recorded in `state/install-backups/latest.json`. Both preserve canonical configuration and configuration rollback journals. If the installed binary cannot run, use a freshly verified release binary or rerun `scripts/setup --source` to perform recovery. The `.previous` binary may be from version 0.1 and therefore lack the recovery command.

Installation backups are separate from retention cleanup. The latest installation record and `.previous` executable are retained; legacy installation backups remain available for explicit archival.

## Configuration recovery and retention

Each `state/transactions/ID/journal.json` contains before/after contents, modes, temporary names, and transaction phase. Journals may include local secret fields from mixed native files and must remain private. They are never Git inputs or suitable unredacted bug-report attachments.

Pending configuration transactions recover on the next sync. A target that matches neither recorded image blocks recovery. Preserve the external edit, inspect the saved images locally, and restore the target to its recorded before or after version before retrying. Relai offers no force flag to discard newer edits.

`relai rollback [ID]` restores a completed transaction when its applied contents still match. It also restores prior canonical source when available and pauses reconciliation. Review the result before `relai daemon resume`; Git records restoration as a new commit.

```sh
relai backups list
relai backups prune --dry-run
relai backups prune
relai status --json
```

Cleanup deletes completed or recovered journals older than the configured age, then the oldest eligible journals until count and byte limits are satisfied. It always retains pending/unknown phases, unreadable journals, unfamiliar directory contents, and the newest completed rollback generation. Protected history can exceed configured limits and is reported. Dry runs use the same selection policy without deleting files.

Automatic cleanup runs under the operation lock at most daily after successful reconciliation, and does not run while paused. A crash between deleting an eligible journal and removing its empty directory does not block recovery. Byte/count limits concern configuration journals, not the Git object database, installed binaries, or installation backups. Monitor total storage with:

```sh
du -sh ~/.local/state/relai ~/.local/share/relai
```

## Removal

```sh
relai daemon uninstall
omarchy plugin remove pablousx.relai
```

Uninstall validates ownership before disabling/stopping the service and removing its unit and public launcher. It refuses unrelated files and asks you to complete a pending installation recovery first. Provider files, canonical source, local configuration, Git repository, binary, and recovery data remain available. These can be archived or deleted deliberately after inspection.

## Health

- **Setup:** initial setup is needed.
- **Local:** watching without a remote.
- **Healthy:** the last complete reconciliation succeeded.
- **Pending:** local changes await remote synchronization.
- **Offline:** the remote step is delayed; local changes are preserved.
- **Conflict:** saved variants need a decision.
- **Paused:** reconciliation is suspended.
- **Error:** validation, permission, state, or recovery needs attention.

The panel separately reports a stopped daemon, an unavailable status command, and a plugin/binary version mismatch. `doctor` diagnoses corrupt state without treating it as a fresh installation. Status always reports the executing binary's version, regardless of persisted older status.
