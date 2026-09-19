# MCP entry adoption contract

Status: future design. Tailor does not implement this contract. The [current specification](../SPECIFICATION.md#model-context-protocol-support) remains authoritative and keeps existing client files unchanged.

This design adds explicit ownership of one named MCP server entry. It does not grant ownership of a complete client file.

## Boundaries

The future design has these rules:

- Keep the fixed internal server registry and provider definitions from [MCP client composition](mcp-composition.md).
- Keep package provisioning separate from client entry ownership.
- Support local command transports first. Remote transports need a separate security review.
- Do not inspect global client settings, trust stores, approval stores, authentication stores, or credential stores.
- Do not infer consent from a declaration, file contents, an old starter, `always`, `first-fit`, `never`, or `--recut`.
- Do not add a public registry, custom destination, custom server name, or custom provider definition.
- Do not change current create-missing-only behaviour until implementation and specification work ships separately.

A server name, endpoint, or transport change is a reviewed migration. It does not inherit credentials or approval state.

## Supported adapters

Adapter IDs identify Tailor's internal syntax contract. They are not minimum client releases.

| Adapter ID | Allowed project destination | Native behaviour that affects the contract |
| --- | --- | --- |
| `claude-project-json-v1` | `.mcp.json` | `mcpServers`; a winning scope supplies a complete server entry. |
| `codex-project-toml-v1` | `.codex/config.toml` | `mcp_servers`; trusted project files form directory layers. |
| `opencode-v1-jsonc-v1` | Explicitly selected `opencode.json` or `opencode.jsonc` | `mcp`; V1 layers merge and use `enabled`. |
| `pi-adapter-2.32.1-v1` | `.pi/mcp.json` | `mcpServers`; `pi-mcp-adapter` 2.32.1 merges fields across shared sources. |

Pi support is limited to the tested adapter version 2.32.1. A different Pi adapter version must fail its version assertion until it passes compatibility tests.

OpenCode support is V1 only. OpenCode V2 needs a separate adapter because it uses `mcp.servers`, `disabled`, and complete same-name replacement. Tailor must not infer a generation from `$schema`, which is common to both documented generations.

Each adapter must assert its expected root and entry grammar before it edits a file. A missing version assertion, an ambiguous generation, or an unsupported project alternative is a conflict.

OpenCode requires one explicit destination in the selector. Tailor rejects a run when both `opencode.json` and `opencode.jsonc` exist, even if the selector names one. No adapter searches parent, global, user, or alternate files.

## Consent interface

The proposed selectors are repeatable on `baste` and `alter`:

```text
--adopt-mcp=adapter:path:server
--release-mcp=adapter:path:server
```

Each component must equal an allowlisted adapter, its allowed project-relative path, and a fixed registry server name. Tailor rejects repeated selectors for the same tuple, malformed selectors, custom paths, custom names, and adapter-path mismatches. Different server tuples can share one destination.

`baste` validates and previews selectors without writing a client file or ledger. It can report required recovery, but it does not execute recovery, create a lock, or clean a temporary file. `alter` requires the same explicit selector and normal approval. A selector authorises only that tuple in that run.

`--adopt-mcp` can authorise a missing target or an existing exact entry. An existing entry qualifies only when its complete supported definition equals the current registry definition. Unknown fields, literal credentials, custom fields, disabled differences, and unsupported fields block adoption. Tailor does not normalise them.

`--release-mcp` changes only the ownership ledger. It does not inspect or change the client file, remote state, package, trust, approval, or authentication state. Release blocks package removal for that capability in the same run.

Release rejects a pending transaction for the tuple or destination. It performs no client inspection or automatic cleanup. The operator must first resolve the pending operation through authorised recovery.

When recovery cannot establish safe state, the operator must stop competing Tailor writers and review the local state. The operator must inspect only the ledger and temporary metadata. The operator must quarantine the ledger and the recorded temporary file without opening, publishing, or committing temporary contents. The operator must preserve the actual client configuration. After the operator resolves or relinquishes pending ownership, a later adoption requires fresh explicit consent. Tailor has no cleanup queue, new release flag, or automatic abandonment path.

### Command and mode matrix

| Command or mode | No selector | Adopt selector | Release selector |
| --- | --- | --- | --- |
| `baste` | Preview the current legacy or owned lifecycle without writes, cleanup, or lock creation. | Validate consent and preview entry and ledger operations without writes, cleanup, or lock creation. | Preview a ledger-only release without client inspection, cleanup, lock creation, or remote calls. |
| `alter` | Apply the current legacy or owned lifecycle after normal approval. | Apply the authorised tuple after normal approval. | Apply a ledger-only release and preserve the package in this run. |
| `alter --recut` | Use the same MCP lifecycle as `alter`. | Use the same adoption contract as `alter`. Recut grants no ownership. | Use the same ledger-only release as `alter`. |
| `fit`, `measure`, or `docket` | Keep current behaviour. | Reject the unsupported selector. | Reject the unsupported selector. |
| Swatch mode `always`, `first-fit`, or `never` | It controls no entry ownership. | It does not strengthen adoption consent. | It does not prevent ledger release. |

### Selector and declaration matrix

| Declaration | Selector | Entry or ledger state | Result |
| --- | --- | --- | --- |
| True | None | Whole file missing | Keep legacy create-missing-only file creation. Do not record ownership. |
| True | None | Existing file, entry missing | Preserve the shared file bytes and do not add the entry. Do not record ownership. |
| True | Adopt | Missing file or entry | Preview or create the entry, then record ownership. |
| True | Adopt | Existing exact supported definition, unowned | Record ownership without changing entry bytes. |
| True | Adopt | Existing non-exact, secret-bearing, custom, or disabled definition | Conflict. Preserve bytes and create no record. |
| True | Adopt | Already owned | Conflict because adoption is already complete. |
| True | Release | Owned or tombstoned, with no pending operation | Change the ledger to released. Preserve client bytes. |
| False | None | Any unowned state | Keep legacy package-only removal and warning. Preserve the client file. |
| False | None | Unchanged owned entry | Remove the entry and write a deliberate-removal tombstone. |
| False | None | Changed or deleted owned entry | Conflict. Preserve the ledger and block package removal. |
| False | Adopt | Any state | Reject the contradictory request. |
| False | Release | Ledger record exists, with no pending operation | Release ledger ownership only. Preserve package and client bytes in this run. |
| Absent | None | Any state | Preserve the entry and ledger. Do not inspect the client for this server. |
| Absent | Adopt | Any state | Reject the contradictory request. |
| Absent | Release | Ledger record exists, with no pending operation | Release ledger ownership only, without client inspection. |
| Any | Release | Pending transaction for the tuple or destination | Reject without client inspection or cleanup. Require authorised recovery or reviewed manual quarantine. |
| Any | Adopt and release for one tuple | Any state | Reject the contradictory selectors before inspection. |

`--recut` and swatch modes do not change this matrix. An old starter with matching bytes remains unowned until explicit adoption.

When every server declaration is absent and no selector exists, Tailor defers recovery. It can inspect ledger metadata, but it performs no client inspection, cleanup, or lock creation. An active different server can require shared-file parsing. During that parse, absent server entries and records keep their exact bytes and values.

## Owned lifecycle

| Declaration and owned state | Future result |
| --- | --- |
| True, unchanged entry | Update the complete supported definition when its registry revision changes. |
| True, user-edited entry | Report drift and stop before writes. |
| True, user-deleted entry | Report deletion drift and stop. Do not recreate it. |
| True, deliberate-removal tombstone | Reinsert and restore owned status only when the file is safe and the entry remains absent. Otherwise, report a conflict. |
| False, unchanged entry | Remove only that entry and record a deliberate-removal tombstone. |
| False, deliberate-removal tombstone | Make no change. |
| False, changed or deleted entry | Report drift and stop. |
| Absent | Preserve the entry and record without inspection for this server. |
| Released | Treat the entry as unowned until a later explicit adoption. |

The tombstone distinguishes intentional removal under a false declaration from user deletion. A repeated false declaration against a tombstone changes nothing. A true declaration reinserts the entry only when the file is safe and the entry remains absent. A present entry or unsafe file is a conflict. Removal means that Tailor no longer supplies the project entry. A lower-precedence definition can become active.

A conflict on any adopted entry blocks all package removals for that capability before writes. Package removal waits until every adopted client removal commits. Unadopted legacy entries retain the current package-only removal and warning.

## Local ownership ledger

The ledger path is:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/tailor/mcp/<sha256(canonical-project-root)>/state.json
```

The canonical project root is also stored in the ledger. Moving or cloning a project changes the binding and requires fresh adoption.

Tailor must require user-owned state directories with mode `0700`, a regular ledger with mode `0600`, and no symlink or hardlink in the ledger path. Tailor must reject an unsafe owner, mode, parent, or file.

Schema 1 stores:

- schema number, canonical project root, and monotonically increasing revision;
- adapter ID, allowlisted destination, and stable registry server name;
- explicit consent operation ID and status: `owned`, `deliberate-removal`, or `released`;
- registry definition revision;
- SHA-256 of canonical generated non-secret entry bytes;
- at most one pending operation for one destination transaction.

A pending operation stores its operation ID, tuple, action, pre-file presence, pre-entry fingerprint or absence, post-file presence, post-entry fingerprint or absence, intended final mode, and temporary filename. Entry fingerprints cover canonical generated non-secret entries. Tailor must never persist a hash of an observed secret-bearing entry.

Entry removal always records a present post-file and an absent post-entry. A missing shared file never matches successful entry removal. Ledger-only adoption and release record equal pre-state and post-state. Recovery resolves that equality in favour of durable consent and finalises the ledger operation.

Fingerprints detect expected state. They prove neither consent nor authenticity. Same-user state tampering is outside this trust boundary. Ledger paths and fingerprints disclose project metadata, so Tailor must not publish them automatically.

## Inspection and preview

Preview reports the client, server, operation, and changed field names. It does not print field values, full diffs, environment values, credential command output, or fingerprints.

Tailor must not resolve secret references or execute credential commands. Literal credential fields block adoption. Supported references remain literal generated data.

Tailor leaves disabled choices intact. Because exact adoption requires the registry definition, a disabled difference remains user-owned rather than being reset.

## Preservation editors

All editors operate on validated token spans. Decode and encode is not permitted for shared-file publication.

### JSON and JSONC

The editor must validate the complete supported grammar, decoded key uniqueness, expected root key, and object entry form. It can replace or remove only the target entry span and necessary adjacent comma and whitespace spans.

For JSONC, comments remain byte-for-byte unchanged. A comment whose attachment to the target or neighbour is ambiguous blocks editing. The editor preserves ordering, unknown unrelated fields, indentation, line endings, trailing newline state, and all non-target bytes.

### TOML

The first TOML editor supports only explicit `[mcp_servers.<name>]` tables and their contiguous explicit subtables. It rejects inline tables, dotted-key ownership forms, reopened tables, non-contiguous subtables, duplicate decoded keys, and duplicate tables.

The editor changes only the target table range and necessary separating blank lines. Comments with ambiguous table attachment block editing. All unrelated bytes remain unchanged.

### File safety

Every adapter rejects malformed input, duplicate decoded keys, unsupported valid syntax, files above 1 MiB, symlinks, files with a link count above one, special files, linked or unsafe parents, and unsafe alternate files.

A replacement keeps the reviewed destination permission bits. A new client file uses the current safe starter mode. A temporary rewrite file starts at mode `0600` for creation, writing, and file sync. Tailor then sets the reviewed destination mode, syncs the metadata, closes the file, performs the final recheck, and publishes it. The temporary file never receives broader permissions than the reviewed destination.

Tailor rechecks the parent identity, destination identity, type, permissions, link count, and exact snapshot bytes after the early snapshot. It performs the same exact recheck immediately before publication, after the temporary file is closed. The final-mode temporary file exists for this short window. An unrelated writer can still race after the final check. The design does not claim filesystem compare-and-swap or a multi-file atomic transaction.

## Publication and recovery

Tailor serialises its writers with a user-local project lock. It preflights every affected tuple before any mutation. It removes a package only after all required entry removals commit.

Tailor processes one tuple per serial destination transaction. Different server tuples for one destination use separate transactions and a fresh snapshot for each transaction.

For each destination-edit transaction, Tailor follows this order:

1. Acquire and validate the local lock.
2. Read a fresh safe destination snapshot and build token edits in memory.
3. Write the ledger with the pending operation, sync the ledger file, rename it atomically, and sync its directory.
4. Recheck the parent identity, destination identity, type, permissions, link count, and exact snapshot bytes.
5. Create an exclusive sibling temporary destination with mode `0600`.
6. Write and file-sync the temporary destination at mode `0600`.
7. Set the reviewed destination mode, sync the temporary file metadata, and close the temporary file.
8. Repeat the exact parent identity, destination identity, type, permissions, link count, and byte recheck immediately before publication.
9. Publish the temporary file with rooted no-clobber creation for an absent file, or rooted atomic rename for an existing file.
10. Sync the destination directory.
11. Write the final ledger revision without the pending operation, sync the ledger file, rename it atomically, and sync its directory.
12. Remove verified stale temporary files and release the lock.

False removes only the target entry through a preservation rewrite. Tailor never unlinks the shared destination, even when that removal leaves no entries.

Ledger-only adoption of an unchanged existing entry and release use durable pending and final ledger revisions. Adoption uses its reviewed preflight snapshot but publishes no destination. Release neither reads nor publishes a destination. For both operations, pre-state and post-state are equal, and durable consent selects the post-state during recovery.

No operation stores a whole-file backup or secret snapshot. A temporary rewrite file can contain existing client content. It starts at mode `0600` and can have the recorded intended final mode only after its file sync. Tailor records its random name only after durable consent exists.

Process startup never changes the ledger, lock, client file, or temporary file. Only an authorised `alter` or recovery operation can remove a recorded temporary file. Before cleanup, Tailor verifies the state directory, project binding, random-name pattern, expected directory, regular-file type, owner, link count of one, and mode. The mode must be `0600` or the recorded intended final mode. Tailor never follows links or opens, publishes, or replays that content. An unverified file is a blocker. After a crash, Tailor rebuilds output from a fresh destination snapshot. `baste` and an all-absent run with no selector defer cleanup and recovery.

### Recovery matrix

| Pending comparison | Recovery |
| --- | --- |
| Pre-state and post-state are equal for ledger-only adoption or release | Treat the state as post-state. Finalise the durable ledger operation without client inspection. |
| File presence and entry state match post-state | Remove only a verified temporary file. Finalise and sync the ledger record. |
| File presence and entry state match pre-state only | Remove only a verified temporary file. Retry from a fresh snapshot only when pending consent and the current declaration still authorise the action. |
| Shared file is absent after an entry-removal operation | It does not match the present-file, absent-entry post-state. Block recovery. |
| File presence or entry state matches neither state | Preserve unverified data. Block editing and require operator review and manual local-state quarantine. |
| Ledger is missing, corrupt, unbound, or lacks durable pending consent | Do not inspect matching bytes to infer ownership. Block adopted editing and require fresh explicit adoption after operator review. |

Recovery after each repeated crash reaches the same result. A completed operation repeated against its final ledger and entry is a no-change result.

### Failure boundaries

Tests must inject failure before and after every sync and publication boundary:

- lock creation and lock sync;
- pending temporary creation, write, file sync, close, rename, and ledger directory sync;
- early destination snapshot recheck;
- destination temporary creation, write, file sync, mode change, metadata sync, and close;
- final exact parent and destination recheck, publication, and destination directory sync;
- final ledger temporary creation, write, file sync, close, rename, and ledger directory sync;
- safe temporary cleanup and lock release.

A failure before durable pending consent changes no destination. A failure after destination publication leaves a recoverable pending record. A directory sync failure reports an uncertain durable state and forces recovery before another operation.

## Synthetic contract fixtures

Use `playwright` and synthetic `search` as managed servers, plus unrelated `notes`. No synthetic server enters the production registry.

The JSON and JSONC fixture keeps `notes`, an unknown root field, comments, key order, CRLF or LF form, and permissions unchanged. The TOML fixture keeps unrelated tables and comments unchanged.

Required cases are:

1. Adopt a missing `playwright` entry while preserving `search` and `notes`.
2. Adopt an existing exact `playwright` entry without changing client bytes.
3. Reject adoption when `playwright` is disabled, contains an unknown field, or contains a literal secret.
4. Update unchanged owned `playwright` while preserving a disabled unowned `search` and unrelated `notes`.
5. Report edited owned `playwright` as drift without writes.
6. Report user-deleted owned `playwright` as deletion drift without repair.
7. Remove unchanged owned `playwright` under false and create a deliberate-removal tombstone.
8. Repeat false against that tombstone as a no-change result.
9. Reinsert from that tombstone when true, the file is safe, and the entry remains absent.
10. Reject true against that tombstone when the entry exists or the file is unsafe.
11. Preserve an absent managed server's entry and record while another active server changes.
12. Distinguish no-selector true with a missing whole file from an existing file with a missing entry.
13. Release ownership without reading or changing the client file or package, and reject release when the tuple or destination is pending.
14. Reject duplicate tuple selectors, but process different server tuples for one destination serially with fresh snapshots.
15. Reject duplicate keys, ambiguous comments, unsupported TOML forms, alternate-file conflicts, unsafe files, and concurrent changes.
16. Exercise every failure boundary, then repeat each adoption, removal, tombstone, release, and recovery outcome twice to prove idempotence.

Each successful edit compares all bytes outside target spans with the original fixture. Secret tests also assert that logs, previews, ledger files, and fingerprints contain no observed secret or secret-derived hash.

## Validation and future acceptance

This design can be validated with isolated parser and filesystem fixtures. It requires no external client or MCP call.

Later provider acceptance must record these results separately for each exact tested client version:

- syntax acceptance;
- project-file discovery and native precedence;
- local executable discovery;
- authentication without credential migration;
- a fresh-client MCP call after normal trust and approval.

An existing MCP process does not validate a changed project configuration.

## Follow-on scopes

These are future proposals. They are not filed issues, and no live estimate is inferred.

| Scope | Size ceiling | Dependency |
| --- | --- | --- |
| Shared ledger, lock, publication, recovery, and secure cleanup | L | This contract |
| JSON and JSONC token-span editor | L | Shared transaction interfaces |
| TOML explicit-table token-span editor | L | Shared transaction interfaces |
| Claude project adapter | M | Shared ledger and JSON editor |
| Codex project adapter | M | Shared ledger and TOML editor |
| OpenCode V1 adapter | M | Shared ledger and JSONC editor |
| Pi adapter 2.32.1 | M | Shared ledger and JSON editor |

Adapters depend on the shared components. OpenCode V2 remains a separate gated design. Parser span behaviour, comment attachment rules, and exact fresh-client versions remain bounded missing evidence.

## Sources

Current Tailor evidence:

- [`docs/SPECIFICATION.md`](../SPECIFICATION.md), Model Context Protocol support and managed write guarantees.
- [`docs/design/mcp-composition.md`](mcp-composition.md), fixed server registry and create-missing-only boundary.
- `internal/alter/managed_mcp_registry.go`, fixed server identities and provider definitions.
- `internal/alter/managed_preflight.go`, current destination snapshots.
- `internal/alter/managed_write.go`, rooted per-file publication and sync boundaries.

Provider evidence from the research record:

- [Claude Code MCP reference](https://code.claude.com/docs/en/mcp), scopes, complete-entry precedence, and authentication boundaries.
- [Codex configuration reference](https://developers.openai.com/codex/config-reference) and [advanced configuration](https://developers.openai.com/codex/config-advanced), MCP fields and trusted project layers.
- [OpenCode V1 configuration](https://opencode.ai/docs/config/) and [MCP servers](https://opencode.ai/docs/mcp-servers/), JSONC, layers, `mcp`, and `enabled`.
- [OpenCode V2 configuration](https://opencode.ai/v2/docs/config) and [MCP servers](https://opencode.ai/v2/docs/mcp-servers/), `mcp.servers`, `disabled`, and same-name replacement.
- [Pi MCP adapter documentation](https://github.com/nicobailon/pi-mcp-adapter) and [loader source](https://raw.githubusercontent.com/nicobailon/pi-mcp-adapter/main/config.ts), import constraints, field merging, and credential cleanup. The exact tested local adapter is 2.32.1.
