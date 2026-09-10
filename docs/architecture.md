# Architecture

The Go binary owns synchronization. The QML widget reads JSON status and invokes the public CLI with argument arrays; it does not parse provider configurations or interpolate shell commands. Omarchy mounts `qml/Omai.qml` through the standard `bar-widget` manifest contract, with a `Panel`, `BarIconButton` and `KeyboardPanel` from its UI module.

## Reconciliation

The canonical tree becomes semantic values: individual settings, MCP servers, hook events, plugin declarations, agents and resource files. Bindings map each value to a provider file and, for structured formats, its native field. The local baseline stores the last canonical tree and the expected value at each binding.

Each pass reads and validates the source, then reads only documented global configuration surfaces. It discovers safe new values in those surfaces. An unchanged provider value is ignored, even if omai previously reformatted its file. A provider-only change becomes a canonical proposal. Different concurrent proposals, or a different canonical edit since the baseline, become a conflict. Deletions are represented as explicit local tombstones so they do not bounce back as new additions.

A complete plan is built before writing. The plan preserves fields outside a binding, records file modes and preimages, and checks every current file against its captured preimage. A durable journal precedes the first write. Each write uses a sibling temporary file and a rename through a pinned Linux directory descriptor. The baseline is part of the same transaction. A successful generation is therefore identifiable even after a process restart.

Unknown JSON/TOML values are retained semantically. Formatting and comments are not retained when a changed document must be serialized. File watching uses bounded periodic scans, which also notice atomic renames and newly created resource directories. A daemon lock prevents duplicate daemons; the operation lock serializes each pass with setup, enrollment and rollback.

## Git transport

The source directory is not a Git checkout. omai's bare transport repository lives in the data directory. A fresh temporary Git index receives only validated source blobs, written with `hash-object`, `update-index`, `write-tree` and `commit-tree`. There is no `git add -A`, working-tree checkout, user hook execution, package invocation or host-identity discovery.

Local reconciliation happens before network operations. Offline changes apply to local tools and remain queued as ordinary commits. Fetches use a temporary ref; explicit ancestry checks detect remote history rewrites before updating the tracking ref. Incoming trees must pass path, object-mode, size, content and schema validation before they enter the source.

The three-way merge uses the common ancestor and complete file values. Disjoint files merge. A file changed differently on both sides, including delete/modify, remains unresolved. There are no line-based auto-resolutions, timestamps selecting a winner, forced ref updates on the remote or force pushes. Merge commits retain both parents. Initial attachment to an existing remote avoids manufacturing an empty merge commit when the local source is still empty.

Git subprocesses have bounded execution time, noninteractive authentication, disabled repository hooks and fixed commit identity. Authentication stays in the user's ordinary Git/SSH credential mechanisms. Captured transport errors do not print credential-bearing URLs. A failed push retains the local commit and retries later.

## Recovery

Pending journals include exact temporary paths. Recovery verifies that each target still matches its before or after image, removes any tracked partial temporary file, and restores the before images. It refuses to overwrite an unrelated external edit. Incomplete journal preparation cannot have touched targets and is ignored. Completed journals remain available as backups.

Rollback is a new transaction. For an application generation, the previous baseline supplies the prior canonical tree, including canonical edits that originally occurred in an editor before the application transaction began. The source must still match the recorded applied tree. The daemon pauses after rollback to give the user time to inspect the restored setup. Resuming records the restored source in Git without rewriting history.

Conflict choices are explicit local records tied to a fingerprint of every available version. Multiple decisions can be made incrementally. Any changed version invalidates the corresponding decision. Completed decisions are removed so a future conflict cannot inherit an old choice.

## Scope and trust

Only the canonical source is a synchronization input. Mixed native files such as `~/.claude.json` are never copied wholesale into it; only recognized fields are extracted. Full-file preimages can contain native local fields, so recovery data remains in the private state directory. The format is an allowlist with excluded path classes and content checks, not a promise to identify every possible secret hidden in arbitrary prose.

The code uses Go's standard library and generic TOML, JSONC and YAML parsers. It does not call, wrap, vendor or depend on another configuration synchronization product.

## Installation and operations

The bootstrap reads the plugin manifest, downloads its exact tagged Linux archive over HTTPS, verifies SHA-256 and archive members, and checks the executable version. Explicit source builds obtain their compiler pin from `mise.toml`. Staging happens outside the read-only plugin checkout.

The plugin's inline setup form invokes `scripts/plugin-setup` with an argument array. The bridge reuses a matching installed executable, otherwise runs the verified bootstrap without terminal input, then submits the form's configuration to the backend. Setup and updates share the existing installation lock and recovery path. Opening or cancelling the form never invokes installation. Source building requires the panel's explicit retry action; download failures never silently select source execution.

The staged binary owns installation. An installation lock and durable `install-pending.json` precede service shutdown and file replacement. Recovery checks all before/after images before restoring binary, launcher, unit, and service state. Installation records never become configuration rollback generations. Failed service activation restores the prior installation; newer external edits cause refusal.

Configuration retention runs under the operation lock after successful reconciliation, at most daily. It prunes eligible completed/recovered journals by age, then oldest first for count and size, while protecting pending/unreadable state and the newest completed rollback generation. Status reads backup metadata instead of deserializing backup payloads. The CLI and panel expose usage and explicit cleanup preview.

Unknown JSON numbers retain their original precision. Structured writes refuse to replace scalar containers. Native hook/MCP validation rejects malformed supported fields, and Git output collectors enforce memory limits independently of execution deadlines.

Provider resources live under `providers/<provider>/` and never acquire bindings to another provider. Existing shared source entries retain explicit cross-provider translation. Discovery follows user skill/rule links only for bounded, validated reads; writes to linked targets are refused. Structured settings are tracked at preference leaves, while native MCP declarations preserve provider-specific options and retain excluded credential literals locally.
