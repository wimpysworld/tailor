# Dependabot whole-file ownership

Status: implemented.

## Goal

Tailor composes one `.github/dependabot.yml` from the Go declaration without replacing an unadopted file.

Dependabot uses whole-file ownership. Tailor does not merge arbitrary YAML. GitHub Actions and Nix remain in every generated variant.

## Ownership consent

The exact ownership marker is the first line:

```text
# Managed by Tailor: .github/dependabot.yml
```

The line must end with LF or CRLF. A byte-order mark, leading blank line, partial marker, wrong path, extra text, or missing newline does not grant ownership.

The marker grants ownership of the complete file. Reconciliation can discard custom schedules, groups, registries, comments, commit-message settings, and other custom fields. With Go false, Tailor retains canonical Actions and Nix entries, not custom versions of those entries.

Removing the marker relinquishes ownership. All active modes then preserve the unmarked file. A matching canonical body without the marker never grants ownership.

> [!WARNING]
> Back up a custom file before you add the marker. The next reconciliation can replace the complete file and remove custom fields.

### Manual adoption

1. Save the current file in Git or another user-controlled backup.
2. Review both supported templates and every custom field that replacement will discard.
3. Keep the file unmarked if any custom field must remain.
4. Resolve any `.github/dependabot.yaml` entry manually.
5. Set `languages.go` explicitly to `true` or `false` for the first reconciliation of custom marked content.
6. Add the exact marker as the first line of the reviewed regular `.github/dependabot.yml` file.
7. Run `tailor baste` and review the complete preview.
8. Run `tailor alter` only after approval.
9. Review the generated file, then commit it through the normal workflow.

An absent Go declaration cannot identify whether a custom body previously enabled Go. Tailor adds no adoption command, flag, ownership ledger, or automatic migration. A language declaration, `always`, and `--recut` do not grant ownership.

## Reconciliation

Tailor applies these rules in order:

1. Resolve the effective swatch entry after default merging.
2. If the entry is omitted or `never`, skip all Dependabot inspection and writing.
3. For an active entry, inspect only `.github/dependabot.yml`, `.github/dependabot.yaml`, and their parents.
4. Reject unsafe paths and any alternate `.yaml` entry before reading or writing `.yml` content.
5. If `.yml` is missing, select a canonical body from the declaration table.
6. If `.yml` is unmarked, preserve every byte and report manual adoption guidance.
7. If `.yml` is marked, determine the selected Go state.
8. Render one complete marked output and compare exact bytes.
9. Publish only a changed, safe plan after the snapshot recheck.

Active means `first-fit` or `always`, with or without `--recut`. Both active modes always reconcile owned files. Neither mode replaces an unmarked file.

| Effective mode and state | Result |
| --- | --- |
| Omitted or `never` | Skip without inspecting either filename. |
| Active, unsafe path or alternate `.yaml` entry | Report a conflict and make no Dependabot write. |
| Active, missing `.yml` | Create the selected marked canonical file. |
| Active, unmarked regular `.yml` | Preserve all bytes and report adoption guidance. |
| Active, owned regular `.yml` | Replace changed output, report unchanged output, or report an absent-state conflict. |

An alternate entry is any filesystem entry at `.github/dependabot.yaml`. A regular file, directory, special file, final symlink, or dangling symlink blocks active management.

Default merging can restore an omitted entry. Use `alteration: never` for a durable opt-out.

## Go declaration

Every canonical variant contains GitHub Actions and Nix.

| `languages.go` | Missing file | Existing owned file |
| --- | --- | --- |
| `true` | Create the Go-enabled variant. | Render the Go-enabled variant. |
| `false` | Create the Go-disabled variant. | Render the Go-disabled variant. Never delete the complete file. |
| Absent | Create the Go-enabled variant for legacy compatibility. | Retain a recognised prior Go state. Reject an unknown prior body. |

An explicit Boolean replaces customised owned content with the selected canonical variant. An absent declaration only accepts a body from the finite canonical set.

## Canonical state and line endings

Absent-state detection compares exact bodies. It does not parse YAML or search for substrings. The set contains the current enabled and disabled templates and private historical compatibility bodies. Each body maps to one Go state.

Tailor recognises an LF or CRLF marker newline. It recognises each canonical body in exact LF form or whole-body CRLF form. Marker and body line endings are independent, so all four combinations identify the same state.

Rendering always produces an LF marker and the exact LF current body. A recognised historical or CRLF file is therefore replaced once. The next run reports unchanged bytes. Unknown, changed, or ambiguous owned content with an absent Go declaration is a conflict.

## Safety and publication

Active management rejects linked parents, final symlinks, directories, special files, read failures, and input or planned output above 1 MiB (1,048,576 bytes). An unreadable unmarked file is a conflict because Tailor must read it before ownership classification.

Preflight runs after configuration default merging and before repository context, authentication, wiki enablement, or other writes. Publication occurs in the swatch stage after the licence stage. A repository or licence failure therefore leaves the Dependabot plan unpublished.

Before publication, Tailor rechecks the project root, both destination names, parent identity, destination type and identity, marker state, and exact snapshot bytes. Drift is a conflict. Tailor does not silently make a new plan.

For a missing `.github` parent, Pages or wiki processing can create the parent first. Tailor tracks that directory through the private identity that it captured before publication and accepts it only when the current run published the same directory without replacement. Linux uses `renameat2` with `RENAME_NOREPLACE`. macOS uses `renameatx_np` with `RENAME_EXCL`.

On other platforms, missing-parent publication returns `errors.ErrUnsupported`. Existing safe parents use the normal rooted file path, but runtime acceptance for those platforms is not claimed.

A changed file uses a sibling temporary file, file sync, close, atomic rename, and directory sync. A missing file uses protected no-replace publication. Tailor performs a final snapshot check before publication, but another writer can still race between that check and rename. Atomic publication protects one file only. The complete `alter` run is not a transaction.

Errors from file, directory, root, temporary-file, and tracked-parent closes are returned. If publication succeeded before a later sync, cleanup, close, or stage error, the report includes the confirmed Dependabot change and returns the error. Earlier confirmed changes in the wider run also remain.

## Preview and reporting

`tailor baste` creates no file, directory, temporary file, or ownership record. It reports creation, complete-file replacement, unchanged owned content, preserved unmarked content, skipped management, or a conflict.

Replacement output names the selected ecosystems and warns that custom fields will disappear. Reports do not print old custom values, registry values, secrets, or a complete old-file diff.

Dependabot has one dedicated planner and at most one writer. Ordinary swatch dispatch and Go preparation exclude `.github/dependabot.yml`.

## Decision record

Whole-file ownership was selected because a general YAML merge cannot preserve comments, ordering, unsupported fields, and future GitHub options safely. The exact marker records explicit consent and keeps Tailor's current custom unmarked file protected.

| Rejected alternative | Reason |
| --- | --- |
| Infer ownership from canonical bytes | Matching content is not user consent. |
| Treat Go, `always`, or `--recut` as consent | These controls do not authorise custom-field loss. |
| Merge arbitrary YAML | Equivalent forms and unknown fields make preservation unsafe. |
| Remove only a managed Go fragment | Dependabot is one shared YAML file, not a registered fragment set. |
| Delete the file when Go is false | Actions and Nix remain enabled independently. |
| Add a flag or ownership ledger | The exact first-line marker records sufficient consent. |
| Parse YAML to infer an absent prior state | A finite exact-body set makes ambiguous or customised state a conflict. |

## Implementation references

- [Planner and canonical state](../../internal/alter/dependabot.go)
- [Inspection and publication](../../internal/alter/dependabot_apply.go)
- [Parent identity tracking](../../internal/alter/dependabot_parents.go)
- [Reporting](../../internal/alter/dependabot_reporting.go)
- [Execution order](../../internal/alter/alter.go)
- [Go preparation](../../internal/alter/go.go)
- [Dependabot configuration file](https://docs.github.com/en/code-security/concepts/supply-chain-security/about-the-dependabot-yml-file)
- [Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)
