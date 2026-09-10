---
name: omai-local-install
description: Install, enable, update, or remove omai on an Omarchy desktop, including its separate companion binary. Use for local installation requests; use omai-release to publish a new upstream version.
---

# Install or update omai locally

Read [AGENTS.md](../../AGENTS.md), [operations](../../docs/operations.md), and the available Omarchy skill before changing desktop state. If this skill is symlinked, resolve its physical directory for repository references. Honor the user's installation/update request; do not require a second confirmation for the same authorized action.

## Inspect before changing

Check the installed plugin's manifest and Git status, shell discovery, companion version, and user-service state. Default locations:

- Plugin: `~/.config/omarchy/plugins/pablousx.omai` (Omarchy's installer uses this home-relative path).
- Companion: `${XDG_DATA_HOME:-$HOME/.local/share}/omai/bin/omai`.
- Local preferences: `${XDG_CONFIG_HOME:-$HOME/.config}/omai/config.json`.
- State: `${XDG_STATE_HOME:-$HOME/.local/state}/omai/`.

Prefer existence/version/status checks to displaying configuration contents. Do not delete a legacy `pablousx.relai` installation as part of installing omai unless migration/removal is requested. Do not silently migrate old paths or reset an existing Git remote.

## Fresh installation

Verify that the remote manifest's version has public release assets, then run:

```sh
omarchy plugin add https://github.com/pablousx/omai.git --enable --yes
```

The first setup action installs the companion automatically. To finish a requested local install without choosing synchronization settings, preinstall it using the installed checkout:

```sh
"$HOME/.config/omarchy/plugins/pablousx.omai/scripts/setup" --install-only
```

With no existing omai configuration this downloads and verifies the exact release binary and leaves the plugin in setup state. On an existing configured installation the same bootstrap can update service integration and restart a previously running service, so inspect first.

For a new setup, the user selects a Git remote or local-only mode, provider import, and then **Start syncing** in the panel. A request to install does not choose these settings or authorize uploading configuration to a guessed repository. If the user explicitly requests automated setup, obtain only missing choices and use the documented CLI arguments; never create a remote merely because the field is blank.

## Update an existing installation

Confirm the checkout is Git-managed and inspect uncommitted changes. Check upstream's manifest version and public release availability before updating: `omarchy plugin update` follows upstream HEAD, not a release tag or a marketplace-approved snapshot.

```sh
omarchy plugin update pablousx.omai --yes
```

Then use **Update omai** in the panel. For an authorized unattended update, the equivalent installed bridge is:

```sh
"$HOME/.config/omarchy/plugins/pablousx.omai/scripts/plugin-setup" update
```

The plugin checkout and companion are separate artifacts. Verify their versions agree afterward. Preserve paused/stopped state and existing settings; do not run setup again, start a stopped daemon, or replace a failed download with an unrequested source build. If fast-forward fails, preserve the installed edits and determine how to reconcile them rather than resetting the checkout.

For an explicitly requested unpublished development build, follow [Contributing](../../CONTRIBUTING.md#local-plugin-development). Stage the complete plugin outside the watched directory before moving it into place. Source builds need mise and the repository's Go pin; a Git checkout that updates normally is preferable for routine use.

## Verify or recover

Validate the installed manifest, confirm one bar placement, check the companion's version and health, and compare service state before/after. The CLI launcher may not exist before setup, so invoke the data-directory binary directly. Resolve shell rescan timeouts with the bounded checks in [debugging](../omai-debugging/SKILL.md); a timeout can follow a successful clone or placement, so inspect before retrying.

If an installation is interrupted, use the documented `install --recover` or `install --rollback` path from operations. Never replace the entire user's config/state directories to repair an update.

When removal is requested, stop/remove service integration before removing the widget:

```sh
omai daemon uninstall
omarchy plugin remove pablousx.omai --yes
```

Configuration, Git history, the companion binary, and recovery data are retained. Delete retained data only when that additional cleanup is requested. Conclude with installed version, bar/setup state, and the user's next step if setup choices remain.
