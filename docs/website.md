# Website

The public website is a self-contained static site in `site/`: the landing page, privacy policy, terms of service, and local CSS, JavaScript, and SVG assets. No build step, external fonts, analytics, or cookies are required. Essential content, navigation, installation instructions, and FAQ work without JavaScript. JavaScript adds sample sync states, command copying, and a color theme preference stored in this browser.

The design follows [omadocs](https://pablousx.github.io/omadocs/): Tokyo Night and Everforest palettes, local monospace fonts, square window borders, a desktop preview, and a large footer wordmark. The preview uses illustrative configuration and supports arrow keys, Home, and End. Policy pages share the same navigation and themes, with an on-page contents list and a light print layout. All resources are local; no framework or package installation is needed to publish or view the site.

## Preview and validate

```sh
python3 scripts/check-site.py
python3 -m http.server 8000 --directory site --bind 127.0.0.1
```

Open `http://127.0.0.1:8000`. Check desktop and narrow mobile layouts, Tokyo Night and Everforest themes, keyboard navigation, and both legal pages. The checker also runs in CI and before deployment. It validates page metadata, local links and anchors, self-contained resources, and the publication directory.

The September 10, 2026 browser review covered all three pages at 320, 390, 760, 768, 1024, and 1440 CSS pixels in Chromium, in both themes. Twelve axe accessibility audits passed at mobile and desktop sizes. Command copying, keyboard-operated preview tabs, FAQs, theme persistence between pages, print styles, and navigation without JavaScript were also checked. No external resource requests or browser errors occurred. Browser review tools are development-only and are not included in `site/`.

## Publish with GitHub Pages

1. Push `site/`, `scripts/check-site.py`, and `.github/workflows/pages.yml` to the repository's default branch. Include `docs/website.md` for the publishing instructions.
2. In repository **Settings → Pages → Build and deployment**, select **GitHub Actions**.
3. In **Actions → Publish website to GitHub Pages**, select **Run workflow** on `main`.
4. Open the deployment URL and check the landing page, both policy pages, theme switching, and installation command.

The workflow publishes only `site/` and runs manually. Application source, local configuration, and build artifacts are outside the uploaded directory. Regular pushes run checks but do not publish the website.

The canonical URL is `https://pablousx.github.io/omai/`, matching this repository’s GitHub Pages configuration. Relative asset and navigation URLs work under the `/omai/` project path and during local preview. No `CNAME` file is needed. If a custom domain is configured later, update the canonical and Open Graph URLs in all three HTML pages and the README website link.

Review privacy and terms against actual application behavior before publication and whenever data handling changes. Hosting is covered by [GitHub's privacy statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement). Deployment follows [GitHub's custom Pages workflow documentation](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).
