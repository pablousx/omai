---
name: omai-release
description: Prepare, validate, and publish an omai version or update its Omarchy marketplace listing. Use for upstream release work in pablousx/omai; local desktop updates and website deployments have separate skills.
---

# Release and publish omai

Read [AGENTS.md](../../AGENTS.md), [publishing](../../docs/publishing.md), and [GitHub configuration](../../GITHUB_SETUP.md). Resolve the physical skill directory if loaded through a discovery symlink. Establish which outcomes the user requested: release preparation, public release, marketplace submission/update, Pages deployment, or local installation. Carry existing authorization forward; do not create extra approval rounds for an already-authorized publication.

## Establish the current state

Inspect worktree changes, remote URL, branch, manifest version, existing tags/releases, relevant Actions runs, and runner availability. Read current marketplace status when listing work is in scope. Use the actual workflows and official publishing guides; historical validation records are evidence for their recorded commit, not today's checkout.

The stable identity is `pablousx.omai` in `pablousx/omai`. Keep it for updates. Normal source changes use a PR, resolved conversations, passing `Repository checks` and `check`, and squash merge. Do not routinely use the owner's bypass. Never tag a pre-squash branch commit accidentally: record and validate the intended release commit after merge.

## Prepare the new version

For a plugin/companion release, choose a new unused stable SemVer. Read the old version before editing and update these together:

- `manifest.json`: plugin `version`.
- `internal/omai/model.go`: Go `Version`.
- `scripts/omai`: the missing-companion status JSON version.
- `tests/fixtures/status.json`: healthy fixture version.
- `scripts/test-qml.py`: successful fake setup version and the `widget.pluginVersion` assertion (currently literal release strings).
- `CHANGELOG.md`, `docs/release-notes.md`, and affected compatibility/operations/validation records.

Search for the old version and classify each occurrence. Preserve deliberate mismatch fixtures (such as `0.1.0`), historical release notes, schema version `1`, and archive format assumptions. Do not globally replace every number or bump the Go pin just because omai is changing version. `scripts/check-release.py` currently checks production metadata, not every QML fixture; the native suite catches the latter.

Documentation/skill or website-only changes do not require a new plugin release by themselves. When publishing runtime changes, keep QML, scripts, and binary compatible within the chosen version rather than reusing an existing public release version for new runtime behavior.

## Validate and tag

Run the release checks in `docs/publishing.md`: full check, dependency audit, all native QML scenarios, fixture screenshot review, pinned-compiler packaging, and extracted-artifact verification. If packaging changed, compare two clean builds. Extract into a **fresh** directory containing the archive/checksums: `scripts/verify-release.py` creates `omai` exclusively and intentionally refuses to overwrite the binary already in `build/release/`.

Before pushing a tag, ensure the native runner is available. Read [native runner preparation](references/native-runner.md) when provisioning or diagnosing it. Do not remove or skip the native gate to get a release out.

Use a new annotated `vVERSION` tag for the reviewed commit. Record its peeled commit SHA and select the release workflow run for that SHA. Tag push triggers [release.yml](../../.github/workflows/release.yml); manual dispatch on `main` alone skips its tag-only native/draft jobs. Application CI and native checks must both succeed before the workflow can create the draft.

The installed plugin updater follows upstream HEAD, so merging a bumped manifest can precede available assets. Keep this publication interval short, do not advertise/update clients until downloads are public, and preserve the bootstrap's safe failure if someone updates early.

## Inspect and publish the draft

Download the archive, `SHA256SUMS`, and `LICENSE` from the draft. Verify checksums, member names/modes, embedded binary version, release notes, and the native-preview artifact for the exact tag. Compare the archive with the corresponding local deterministic build when available. Do not mistake authenticated access to a draft for public download availability.

Once the requested publication is authorized and checks pass, publish the existing draft, then verify it is non-draft and its anonymous download URLs work. Do not create a duplicate release or upload replacement assets over an existing public release.

In a fresh isolated home, clone the public tag, validate its manifest, and run `scripts/setup --install-only` before creating any config. Confirm the installed version and run the downloaded binary through the disposable CLI/installer suites. These prove public distribution and fixture lifecycle behavior. A real systemd/session activation check is additional evidence and requires an authorized disposable Omarchy account or local-install task; do not claim it from stubbed service tests.

If infrastructure failed without code changes, diagnose and rerun the appropriate failed job for the same SHA (provision another ephemeral runner if needed). If code must change, release a new patch tag. Never force-move a released tag, replace published assets, disable verification, or silently source-build as a substitute for testing public downloads. Inspect an already-created draft before rerunning a workflow that would try to create it again.

## Complete the requested distribution steps

For marketplace submission or an update to a listed snapshot, read [marketplace procedure](references/marketplace.md). For Pages use [website maintenance](../omai-website/SKILL.md); for this desktop use [local installation](../omai-local-install/SKILL.md). These operations have separate state and verification.

Report release version/URL, relevant checks, and any marketplace issue/deployment URLs. Say explicitly whether a listing is merely submitted, validated, or approved. Installer/service capabilities can require marketplace maintainer review without indicating a defect to fix.
