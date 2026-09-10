# GitHub repository configuration

This repository uses the shared open-source baseline. Project code, licensing, native desktop validation, release procedures, and website publishing keep their existing workflows.

## Required checks

`Repository checks`, `check` must pass from GitHub Actions, with the pull request up to date before merging. `Repository checks` validates community/configuration files and tests the setup tooling. The other checks validate the project itself. Native Omarchy/QML checks remain in the project's documented development/release process.

The [baseline ruleset](.github/rulesets/open-source.json) requires pull requests, resolved review conversations, and squash merging, and blocks force pushes and default-branch deletion. The [CI ruleset](.github/rulesets/ci.json) requires the checks above. Both grant **pablousx an Always allow bypass**, including direct pushes and overriding checks. Normal changes should use pull requests and passing CI. Required approvals remain at zero for solo maintenance; use `--approvals 1` when another maintainer is available.

## Apply or reconcile settings

Use an authenticated `gh` CLI with repository administration permission. From the repository root:

```bash
python3 .github/scripts/setup_repository.py --repo pablousx/omai
python3 .github/scripts/setup_repository.py --repo pablousx/omai --apply
```

The first command only previews. The second updates the two named rulesets, shared labels, squash-only merging, automatic deletion of merged branches, read-only Actions defaults, and Dependabot alerts/security updates. It enables private vulnerability reporting, secret scanning, and push protection on this public repository. It enables issues and disables wiki/project features; discussions, Pages, releases, environments, and visibility are unchanged.

The script preserves unrelated labels and rulesets and updates existing managed rulesets instead of duplicating them. It requires successful checks on the current default-branch commit before setting mandatory CI. If a setting fails, earlier changes may already be applied: review the output, resolve the failure, and rerun. A fork's administrator should supply the fork's `OWNER/REPO` and review CODEOWNERS; the script resolves that owner's bypass ID unless `--maintainer LOGIN` is given explicitly.

GitHub template files and JSON do not automatically synchronize repository settings. Review changes and rerun this script when the policy changes.

## Validate locally

Configuration checks use Python 3.12+ with PyYAML (`python3-yaml` on Debian/Ubuntu). These developer-only tools live under `.github/` and are not runtime dependencies.

```bash
python3 .github/scripts/check_repository.py
python3 -m unittest discover -s .github/tests -v
mise run check
mise run audit
```

[Dependabot](.github/dependabot.yml) checks Actions and any configured package ecosystems weekly. [Release categories](.github/release.yml) are available when generating GitHub release notes; publishing remains a maintainer action. [CODEOWNERS](.github/CODEOWNERS) routes review requests to pablousx without requiring a second maintainer.

See [Contributing](CONTRIBUTING.md), [Security](SECURITY.md), [Support](SUPPORT.md), and the [Code of conduct](CODE_OF_CONDUCT.md).
