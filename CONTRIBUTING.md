# Contributing

omai targets Linux global configuration and Omarchy's Quattro plugin interface. Keep provider-specific semantics explicit and preserve fields outside supported bindings. Changes to source or state formats require compatibility fixtures and documented migration behavior.

Use the Go compiler pinned in `mise.toml`:

```sh
mise run check
mise run audit
mise run qml
mise run release
```

`check` runs formatting checks, race tests, vet, shell syntax, fresh-binary CLI integration, installer tests, metadata checks, and the native manifest validator when installed. Install ShellCheck to include shell lint locally; CI requires it. QML validation requires Omarchy and a Wayland session. It creates independent fixture panels and can save screenshots with `python3 scripts/test-qml.py --screenshots build/previews`.

Tests must use temporary homes, fixture service managers, and local bare remotes. Do not point automated tests at live provider configuration, credentials, or the user's real omai service. New failure-handling behavior needs tests for preserving existing data and recovering after interruption.

Keep documentation and compatibility references aligned with adapter behavior. Avoid claiming native-provider support solely because parser round trips pass. Use official provider documentation and representative fixtures; never require authentication or paid model calls in the test suite.

## Local plugin development

Before public distribution, copy the manifest, `qml/`, `scripts/`, `cmd/`, `internal/`, `go.mod`, `go.sum`, `mise.toml`, and license into an Omarchy user plugin folder named `pablousx.omai`. Stage the complete directory outside `~/.config/omarchy/plugins/` before moving it into place: copying individual source files into a watched plugin causes repeated shell reloads. Enable it with `omarchy plugin enable pablousx.omai --section right` after discovery. Normal setup then happens through **Set up omai** in the panel. If a release is unavailable, choose **Build local copy and retry** there.

For compiler or headless development, the lower-level commands remain available:

```sh
./scripts/setup --source --install-only
~/.local/share/omai/bin/omai setup --yes --no-service \
  --remote /absolute/path/to/remote.git --providers codex,claude,opencode
~/.local/share/omai/bin/omai daemon run
```

These are developer/automation interfaces, not a second step in plugin onboarding. `--build-only` builds and installs only the executable. Source builds run with the pinned compiler in a temporary directory outside the checkout. Custom XDG paths must be absolute.

Pull requests should explain the concrete problem, resulting behavior, and relevant validation. File ordinary bugs with a minimal non-secret reproduction, omai version, Omarchy version, and expected/actual behavior. See [SECURITY.md](SECURITY.md) for sensitive reports and [publishing](docs/publishing.md) for release maintenance.

## Agent maintenance guides

[AGENTS.md](AGENTS.md) is the project entry point for coding agents. The versioned skills in [skills/](skills/) cover development, debugging, releases and marketplace updates, local installation/updates, and the static website. They refer to this repository's existing runbooks rather than maintaining separate copies of product documentation. Read the relevant `SKILL.md` directly if the agent host does not discover repository skills automatically.

For Codex discovery on a development machine, optionally link these five skill directories into `${CODEX_HOME:-$HOME/.codex}/skills/`. Resolve this checkout's absolute path first, keep the tracked sources here, and never overwrite an existing unrelated skill. Recreate links if the checkout moves; do not link to the watched installed-plugin copy. Root AGENTS routing works without global installation.

When editing these guides, use the available skill-creator validator for each changed skill, verify local Markdown references and UI metadata, and run `python3 .github/scripts/check_repository.py` plus `git diff --check`. A prose-only maintenance update does not require installing omai, creating a new release, or running the entire native suite.

## Community and repository checks

Follow the [code of conduct](CODE_OF_CONDUCT.md). See [GitHub repository configuration](GITHUB_SETUP.md) for community-file validation, required checks, maintainer bypass, and the shared setup script. Existing project checks above remain required.
