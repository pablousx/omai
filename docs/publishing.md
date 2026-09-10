# Publishing omai

omai is distributed as the public `pablousx/omai` GitHub repository plus versioned release assets. Omarchy installs the repository by Git URL. Marketplace submission is a separate optional step; see the [Omarchy publishing guide](https://plugins.omarchy.org/publish.html).

## Repository setup

Confirm the remote points to `github.com/pablousx/omai`, the default branch is `main`, the MIT license is present, and GitHub Actions and private vulnerability reporting are enabled. Protect `main` with the CI check and restrict creation/movement of release tags to maintainers. Keep published release tags and assets immutable.

Ordinary CI runs on GitHub-hosted Ubuntu workers. The release workflow additionally requires a self-hosted Linux x86-64 runner labeled `omarchy` and `omai-release`, with Omarchy 4.0.3+ and a running Wayland session. Use an isolated test account or a one-job ephemeral runner inside a filesystem sandbox. A sandboxed runner must have an empty home, no access to the host home or session credentials, read-only system files, and only its temporary working directory writable. Expose the Wayland socket for fixture panels through `WAYLAND_DISPLAY` and `XDG_RUNTIME_DIR`; the tests capture only their own panels. Register an up-to-date Actions runner compatible with the pinned Node-based actions. Pull requests never execute on this native runner. An ephemeral runner deregisters after its job; remove its temporary credentials and files after verifying completion.

The native release job is mandatory. Without an available runner and a passing QML suite, the workflow cannot create its draft release. It checks the exact tagged checkout and uploads fixture-panel screenshots for review.

## Prepare a release

1. Update `manifest.json`, the Go `Version` constant, the launcher's setup status, `tests/fixtures/status.json`, and the successful-version literals/assertions in `scripts/test-qml.py` together. Preserve deliberately mismatched versions and historical documentation. `scripts/check-release.py` checks production version consistency; the native suite additionally checks QML fixture agreement. Keep canonical/local schema versions at `1` unless an explicit migration is implemented.
2. Update the changelog, release notes, compatibility record, and any affected operations instructions. The Go toolchain pin lives only in `mise.toml`; CI and source setup read it there.
3. Run `mise run check`, `mise run audit`, and `mise run qml`. Verify clean setup, relay between two disposable machines, update/recovery, and service removal. The installer tests simulate release downloads, systemd, failed activation, and a killed updater.
4. Run `mise run release`. Inspect `build/release/omai_VERSION_linux_amd64.tar.gz` and `SHA256SUMS`; the archive contains only `omai` and `LICENSE`. Build twice if changing packaging to verify deterministic checksums.
5. Capture fixture panels with `python3 scripts/test-qml.py --screenshots build/previews`, inspect them, and refresh `docs/screenshots/` when UI changes. No real desktop or provider content belongs in these images.

## Create and publish

Create and push the exact `vVERSION` tag only after the release commit is reviewed. The release workflow reruns CI, requires the native Omarchy check, builds the pinned compiler's artifact, verifies tag/manifest/binary agreement, and creates a **draft** GitHub release with the archive, checksums, license, and `docs/release-notes.md`.

Review the draft assets, native screenshots, checks, installation instructions, and release notes. Publish the draft deliberately in GitHub. Until it is published, unauthenticated setup cannot download its assets; use `scripts/setup --source` for local development. Do not advertise installation of a new manifest version before its public assets are available.

After publication, verify the documented Git URL installation and download URLs from a clean Omarchy test account. Confirm that the downloaded binary reports the tagged version, setup completes, local synchronization works, and removal stops its daemon. These live distribution checks require the actual published repository and cannot be substituted by fixture tests.

For a release failure, fix the problem on `main` and publish a new patch version. Do not replace existing published assets or force-move a release tag. Users can restore interrupted installations with `omai install --recover` and the previous completed installation with `omai install --rollback`.

## Submit to the Omarchy marketplace

After the public release is downloadable, use the [official submission form](https://github.com/omacom/omarchy-plugin-marketplace/issues/new?template=submit-plugin.yml). Submit `https://github.com/pablousx/omai` under **Developer Tools**, with **AI**, **Bar**, and **Quickshell** tags. The root `preview.png` is a fixture-only screenshot for the listing.

Maintainer notes should explain the companion download and checksum verification, Git and Python requirements, the explicit setup action before configuration changes, optional Git remote, and service removal. The website, privacy policy, and terms are linked from the README. Complete the form's ownership, documentation, consent, and license checklist from the actual release contents.

Watch the submission's automated validation and address any reported fixes. Marketplace listing requires maintainer approval; a submitted or validated issue is not yet an approved listing.

## Update the published plugin

Publish runtime changes under a new release version using the same preparation, tag, native gate, draft inspection, and public-download checks. Do not replace previously published assets or reuse a public version for changed runtime code. Normal source changes follow the PR and required-check process in [GITHUB_SETUP.md](../GITHUB_SETUP.md).

For an already listed plugin, the marketplace currently uses its [verification form](https://github.com/omacom/omarchy-plugin-marketplace/issues/new?template=verify-plugin.yml): choose **Verify and publish a newer upstream commit**, identify `pablousx.omai`, and supply the full target SHA. A source push or GitHub release does not automatically promote the marketplace snapshot. Inspect the current [verification guide](https://github.com/omacom/omarchy-plugin-marketplace/blob/main/VERIFICATION.md) and any existing omai request before submitting. The initial listing request is [#6131](https://github.com/omacom/omarchy-plugin-marketplace/issues/6131); check its live state rather than assuming it has been approved.

Updating a user's machine is a separate operation: update the installed Git checkout with `omarchy plugin update pablousx.omai --yes`, then use **Update omai** in the panel (or the installed `scripts/plugin-setup update` bridge for an authorized unattended update). Verify the companion and manifest agree and preserve paused/stopped service state. See the [local-install skill](../skills/omai-local-install/SKILL.md).

The [release skill](../skills/omai-release/SKILL.md) has task-specific guidance for version coordination, an isolated ephemeral native runner, draft verification, and marketplace submissions/updates. It does not authorize publication by itself.
