omai 1.0 synchronizes supported global Codex, Claude Code, and OpenCode configuration from one editable source, with offline operation, ordinary Git transport, explicit conflicts, and recovery journals.


Global sync now discovers provider-specific instructions, complete text skills/resources (including linked and legacy Codex skills), native rules and MCP declarations. Expanded global preferences, Codex hooks.json, OpenCode tui.json and existing global JavaScript plugins synchronize too. The panel shows coverage counts for each provider.

This release adds verified prebuilt installation, recoverable upgrades, safe service removal, automatic backup retention, keyboard-accessible conflict actions, and release validation.

Requirements: Linux x86-64, Omarchy 4.0.3 or later with the Quattro plugin interface, Git, curl, Python 3.11+, and a systemd user session. Existing version-1 canonical and local configuration remain compatible. Automatic backup cleanup now defaults to 30 days, 100 journals, and 512 MiB, while protecting pending recovery and the latest rollback.

Install the plugin from https://github.com/pablousx/omai.git and choose **Set up omai**. Complete the form and start sync inside the plugin window; it installs the companion executable automatically. Existing users should update the plugin checkout and choose **Update omai**, which also runs in the panel. Unpublished local copies can explicitly build from source using the panel's retry action.

Review the README, operations guide, compatibility limits, and SHA256SUMS before deploying. No provider authentication or paid model invocation is part of the automated test suite.
