---
name: omai-website
description: Maintain omai's responsive static landing page, privacy policy, terms, and GitHub Pages deployment. Use for site work in pablousx/omai, independently of native plugin releases.
---

# Maintain the omai website

Read [AGENTS.md](../../AGENTS.md) and [website operations](../../docs/website.md). Resolve the physical skill directory when following links from a local discovery symlink. The published directory is `site/`; application source and maintenance skills must stay outside it.

## Preserve the design and behavior

Use the existing Tokyo Night/Everforest palette, monospace typography, square borders, desktop preview, generous spacing, and consistent navigation/footer across `index.html`, `privacy.html`, and `terms.html`. The reference is [omadocs](https://pablousx.github.io/omadocs/); inspect its current published source when a new visual comparison is needed.

Keep the site plain HTML/CSS with small progressive-enhancement JavaScript and local SVG assets. Essential content, links, installation instructions, and native-details FAQs work without JavaScript. No external fonts, runtime framework, analytics, or third-party embeds are currently used. Theme choice is stored only as `omai-theme` in browser local storage; changing that behavior requires matching policy text.

The desktop mockup shows synthetic states. Keep it clearly labeled as a preview. Preserve keyboard tab behavior, visible focus, skip link, readable narrow layouts, usable touch targets, reduced motion, and printable legal pages. Use existing code/SVG rather than raster generation for simple interface shapes.

Base install instructions and policy claims on the actual application. Distinguish local state from optional Git transport, public hosting metadata from application telemetry, and removing a widget from stopping its daemon. Preserve the MIT license's rights. Revise policy dates only when the policy changes, and consult current authoritative hosting/legal sources when their claims need updating.

## Check the result

```sh
python3 scripts/check-site.py
node --check site/assets/site.js
python3 -m http.server 8000 --directory site --bind 127.0.0.1
```

The static checker validates required pages, metadata, local links/anchors/resources, and the publication boundary. For layout or interaction changes, inspect actual browser rendering at relevant narrow/mobile, tablet, and desktop widths, in both themes. The original review used 320, 390, 760, 768, 1024, and 1440 CSS pixels; choose cases that exercise changed breakpoints rather than rerunning every combination for prose edits.

Check overflow, keyboard tabs, copy success/fallback, unavailable local storage, FAQ behavior, cross-page theme persistence, no-JavaScript use, reduced motion, and print where affected. Audit accessibility when layout/control changes warrant it. Keep browser packages, screenshots, and reports in temporary/development output, never in `site/`.

## Publish only the requested site change

A plugin tag does not deploy Pages. After the site changes are merged and deployment is authorized:

```sh
gh api repos/pablousx/omai/pages --jq '{html_url,build_type,status,cname}'
gh workflow run pages.yml --repo pablousx/omai --ref main
```

Select the run for the intended commit and wait for completion. For first-time setup, configure Pages to use GitHub Actions; do not reset existing custom-domain configuration blindly. Verify all three public pages and their assets, preferably by comparing downloaded bytes to the reviewed commit.

Use GitHub's configured `html_url` to establish the live URL. The current site is `https://pablousx.github.io/omai/`. A copied custom hostname was previously found to return 404; a canonical link is not evidence that a hostname is configured. Keep relative asset/navigation paths working under `/omai/`, and update all canonical/Open Graph URLs plus the README/homepage only when the public location actually changes.

See [the Pages workflow](../../.github/workflows/pages.yml) for the upload directory and pinned actions. Report the live URL and deployment result without implying that a website deployment publishes or updates the plugin.
