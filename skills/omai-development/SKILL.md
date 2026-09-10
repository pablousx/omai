---
name: omai-development
description: Develop and test the omai Go synchronization engine, provider adapters, CLI, or Omarchy QML interface. Use for code changes in pablousx/omai; use the separate website, release, or local-install skill for those tasks.
---

# Develop omai

Work in the intended omai checkout and read [AGENTS.md](../../AGENTS.md). If this skill was discovered through a local symlink, resolve its physical location before following repository-relative references. Do not apply it to another project's source.

## Choose the implementation boundary

Read the relevant section of [architecture](../../docs/architecture.md) and [Contributing](../../CONTRIBUTING.md). Engine changes belong in `internal/omai`; CLI surface changes also touch `cmd/omai/main.go`; panel work belongs in `qml/`. Avoid reimplementing parsing or state transitions in QML.

For provider changes, read [compatibility](../../docs/compatibility.md) and [format](../../docs/format.md). Verify current upstream behavior against official provider documentation or local source before broadening support. A parser round trip does not prove the provider uses the generated value. Test provider-specific discovery, explicitly shared entries, unknown/native local fields, deletion propagation, and same-named resources where relevant.

For recovery changes, follow the transaction and installation invariants in AGENTS.md. Add a regression that demonstrates the actual failure and verifies preservation of existing data. Test interruption, concurrent changes, and retry only where the changed path can affect them; avoid tests that merely mirror new implementation text.

## Run the right checks

Use `mise.toml` for the compiler, not a remembered Go version. Typical iteration:

```sh
mise exec -- go test ./internal/omai -run TestRelevantCase -count=1
mise run check
```

Replace `TestRelevantCase` with an existing or newly added regression name. `mise run check` builds a fresh binary before CLI/installer integration. A standalone `mise run e2e` or `mise run install-test` also builds through task dependencies. Passing an old `build/omai` directly to a script can test stale code.

For QML/status-contract changes:

```sh
mise run validate
python3 scripts/test-qml.py --state healthy
mise run qml
python3 scripts/test-qml.py --screenshots build/previews
```

Read `states` in `scripts/test-qml.py` for current scenario names. Review the fixture captures for the affected states; refresh `docs/screenshots/` and the root `preview.png` when their visible behavior changes. The suite requires a real Wayland connection and installed Omarchy UI modules. It creates its own temporary home/runtime and screenshots only fixture panels.

If sandbox restrictions prevent cache writes, use writable temporary `GOCACHE` and `GOMODCACHE` directories and the installed compiler matching the `mise.toml` pin. If mise itself cannot write its state, invoke that exact compiler's executable rather than falling back to an arbitrary `go` on PATH. Request only the execution access needed for native sockets or downloads; a blocked socket is not a failed application test.

## Keep the installed desktop separate

Prefer fixture panels over installing every edit. When the user asks for a live development install, use [local installation](../omai-local-install/SKILL.md) and the staging guidance in Contributing. Stage complete directories outside the watched plugin tree before replacement. A normal Git-managed installed checkout should remain updateable; preserve local edits instead of resetting them.

Do not run `scripts/setup --source`, `--install-only`, or `--build-only` against the real home merely to build or test: all of these can install an executable. `mise run build` only builds into `build/`.

## Finish the change

Update the relevant compatibility, operations, UX, and validation docs when behavior changes. Keep the release version unchanged during ordinary development; coordinate all version-bearing files when preparing a release. Follow the PR template and required checks in [GITHUB_SETUP.md](../../GITHUB_SETUP.md). Publishing or updating the user's installed copy is a separate requested operation.
