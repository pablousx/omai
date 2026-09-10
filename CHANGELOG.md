# Changelog

## 1.0.0 — 2026-09-10

- Publish the plugin as **omai**, with the `pablousx.omai` plugin identity.
- Discover provider-specific instructions, complete text skills/resources (including linked and legacy Codex skills), native rules, and MCP declarations. Expanded global preferences, Codex hooks.json, OpenCode tui.json, and existing global JavaScript plugins synchronize too.
- Show provider coverage counts, contextual sync controls, progressive details, inline diagnostics/recovery/conflict review, and consistent keyboard/hover/progress feedback.
- Add the responsive Omarchy-inspired website, privacy policy, terms of service, and GitHub Pages publishing workflow.

- Advanced **Clear settings (keeps configs)** stops sync and returns to setup while preserving canonical/provider files and recovery history, with inline confirmation.
- Plugin setup uses an inline form with installation, progress, error recovery, and explicit local-source retry; setup and updates do not launch a terminal.
- Exact-version Linux x86-64 release downloads with checksum, archive, and binary-version verification; explicit pinned source builds remain available.
- Durable installation recovery, previous-installation rollback, safe service removal, and preservation of stopped/paused state during updates.
- Retryable setup and actionable status errors, including corrupt state and plugin/binary version mismatches.
- Automatic bounded configuration backup retention with inspection and dry-run cleanup.
- Keyboard-accessible conflict choices, duplicate-action protection, consistent panel controls, and distinct offline/conflict results.
- Precise unknown JSON values, protected non-object native fields, stronger hook/MCP validation, and bounded Git output.
- Automated race, integration, installer, QML, vulnerability, packaging, and release gates.

## 0.1.0

Initial global synchronization engine, provider adapters, Git transport, conflict decisions, configuration journals, daemon, and Omarchy bar widget.
