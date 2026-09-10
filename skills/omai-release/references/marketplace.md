# Omarchy marketplace submission and updates

Check the live [publishing guide](https://plugins.omarchy.org/publish.html), [submission guide](https://github.com/omacom/omarchy-plugin-marketplace/blob/main/SUBMISSION.md), and [verification guide](https://github.com/omacom/omarchy-plugin-marketplace/blob/main/VERIFICATION.md) before an external action. Forms and approval rules change independently of omai.

## Identify the existing listing first

The initial omai submission is [issue #6131](https://github.com/omacom/omarchy-plugin-marketplace/issues/6131). On September 10, 2026 it was validated and awaiting installer/service capability review; that is a dated observation, not a current approval guarantee. Check the issue and marketplace catalog before deciding between initial submission and a listed-plugin update.

```sh
gh issue view 6131 --repo omacom/omarchy-plugin-marketplace \
  --json url,state,labels,comments
gh issue list --repo omacom/omarchy-plugin-marketplace \
  --search 'omai in:title' --state all
```

Read only relevant report fields. Do not copy encoded baseline comment markers into human-written content or attempt to set marketplace maintainer approval labels yourself.

## Initial submission

For an unlisted plugin, use the existing open submission where appropriate; do not create another issue for every release. If none exists and submission is requested, retrieve the current `submit-plugin.yml` form from the marketplace repository.

Expected identity and classification:

- Repository URL: `https://github.com/pablousx/omai`.
- Plugin ID: `pablousx.omai`, matching the root manifest.
- Category: `Developer Tools`.
- Tags: `AI`, `Bar`, `Quickshell` (the form currently permits one to three).
- Title starts with `[Plugin]:` and names omai.
- Root `preview.png` is an optional fixture-only screenshot.

The form currently requires repository/category/tags, optional tag suggestion, maintainer notes, and confirmed documentation/license/rights/consent acknowledgments. Retrieve the actual labels/checklist wording when generating a CLI body; do not rely on a stale copied template. Do not claim ownership or permissions beyond the user's project and assets.

Maintainer notes should describe:

- The published version and real install command.
- Explicit setup choices before sync starts; local-only operation and optional user-selected Git remote.
- Linux/Omarchy/Git/curl/Python/systemd requirements and the separately installed companion.
- Exact-version release downloads, checksum/archive/version validation, and explicit source-build fallback.
- Configuration's execution implications, excluded credential stores, and the limitations of arbitrary-text secret detection.
- `omai daemon uninstall` before removing the widget; what data remains afterward.
- MIT licensing, fixture preview provenance, website/policy links, and exact release validation evidence.

Prepare a body file and create/edit the intended issue only within the user's authorized marketplace task. The CLI does not render an issue form automatically: supply the current form headings and values explicitly. After submission, wait for automated validation and address concrete `needs-fixes` findings. A completed workflow does not by itself mean the report passed.

`validated` means manifest/repository/Quattro checks passed. `security-review-required` may reflect legitimate installer or service-management capabilities, without a required code change. Read the report. Maintainers decide listing approval; neither validation nor a GitHub release means the plugin is listed. Do not weaken installation behavior or hide capabilities to avoid review.

## Update an already listed plugin

As checked September 10, 2026, the marketplace uses **one verification form**, not an `update-plugin.yml` form:

`https://github.com/omacom/omarchy-plugin-marketplace/issues/new?template=verify-plugin.yml`

Fetch the current form, then use its newer-upstream-commit action. It currently asks for:

| Field | Value for an omai update |
| --- | --- |
| Verification action | `Verify and publish a newer upstream commit` |
| Plugin ID | `pablousx.omai` |
| Repository URL | `https://github.com/pablousx/omai` |
| Target commit | Full 40-character SHA of the current upstream HEAD intended for review |
| Verification acknowledgment | Current form acknowledgment, explicitly confirmed when supported by the task |

The issue title begins with `[Verify]:`. Check for an existing request for the same target before creating another. Resolve the upstream SHA immediately before submission and ensure the candidate contains the intended published version. If it differs from the release tag, inspect the intervening diff and do not claim the release tests cover runtime changes added afterward.

Do not request the separate standard-installation override action merely to publish a normal update. That path has its own eligibility, acknowledgments, and maintainer approval requirements.

The existing marketplace snapshot remains unchanged until the candidate receives compatibility validation, a fresh baseline scan, explicit maintainer approval, tests, and deployment. A release publication or source push does not automatically promote it. Changes after review require fresh evidence for the new target SHA. For an initial submission still awaiting approval, follow that issue's current workflow instead of claiming an already-listed snapshot exists.

## Report distribution states separately

State the public release version/URL, submission or verification issue URL, validation outcome, and whether maintainer review is pending. If approval is outside the user's control, the completed deliverable is a verified release plus the correctly submitted request, with the remaining marketplace decision identified clearly.

Omarchy's ordinary Git-based installer/updater follows mutable upstream HEAD; a verified marketplace snapshot is not a guarantee that an unpinned local checkout matches that snapshot. Read [local installation](../../omai-local-install/SKILL.md) when asked to update a user's machine.
