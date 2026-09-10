# Security

omai runs as your user and writes AI configuration that can influence executable hooks, plugins, and MCP servers. Use a dedicated remote controlled by people you trust. Checksums detect mismatched or damaged release downloads; the release repository and HTTPS distribution remain part of the trust boundary.

Credential/history/session stores are excluded. Content scanning is an additional check, not a proof that arbitrary text is secret-free. Local recovery journals can contain secrets from mixed provider files; they must never be uploaded or attached to public issues.

For a vulnerability involving credential exposure, arbitrary writes, recovery corruption, or release integrity, use GitHub's private vulnerability reporting for `pablousx/omai` when enabled. If private reporting is unavailable, open a minimal issue requesting a private contact without exploit details, secrets, provider files, or journals. The publishing runbook requires enabling private reporting before release.

Include affected version, a minimal reproduction using fake data, impact, and any suggested fix. Ordinary support reports belong in public issues after removing private information. Version 1.x is the maintained release line; security fixes ship in the latest patch release.
