# Dependabot whole-file ownership

Status: approved design for future work. Tailor does not implement this contract. The [current specification](../SPECIFICATION.md#go-ecosystem-support) remains authoritative until the follow-on work ships.

## Goal

Tailor will compose one `.github/dependabot.yml` from the Go declaration without replacing an unadopted file.

The minimal design uses whole-file ownership. It does not merge arbitrary YAML. GitHub Actions and Nix remain independent of Go in every generated variant.

## Current behaviour

Current code has these behaviours:

- [`swatch.Render`](../../internal/swatch/go.go) selects `swatches/go/dependabot-disabled.yml` only when Go is explicitly false. True and absent use `swatches/.github/dependabot.yml`.
- [`Config.GoDeclared` and `Config.GoEnabled`](../../internal/config/languages.go) distinguish absent, false, and true declarations.
- [The swatch registry](../../internal/swatch/registry.go) registers Dependabot as a `first-fit` health swatch.
- [`prepareGoSwatches`](../../internal/alter/go.go) and [`processSwatch`](../../internal/alter/swatch.go) preserve an existing first-fit file during a normal alteration. `always` and `--recut` can replace it.
- [The managed registry](../../internal/alter/managed_registry.go) does not contain Dependabot. Its fragment-removal policy cannot disable one ecosystem in a shared file.

Tailor's own `.github/dependabot.yml` is unmarked. It has custom Go groups and commit-message settings, and its Actions entry uses `directories`. This design preserves that file.

The future contract deliberately changes current behaviour. Active first-fit will reconcile an owned file. `always` and `--recut` will preserve an unmarked Dependabot file.

## Ownership consent

The exact ownership marker is the first line:

```text
# Managed by Tailor: .github/dependabot.yml
```

The line must end with LF or CRLF. A byte-order mark, leading blank line, partial marker, wrong path, extra text, or missing newline does not grant ownership.

The marker grants ownership of the complete file. Matching canonical bytes never grant ownership. Canonical matching detects the prior Go state only after the marker grants ownership.

Whole-file ownership means that reconciliation discards custom schedules, groups, registries, comments, commit-message settings, and other custom fields. With Go false, Tailor retains canonical Actions and Nix entries, not custom versions of those entries.

Removing the marker relinquishes ownership. All active modes then preserve the unmarked file. `alteration: never` stops all Dependabot inspection.

> [!WARNING]
> Adding the marker today does not activate these protections. Current `always` and `--recut` behaviour can replace the file before this complete contract ships.

### Future manual adoption

After the future implementation ships, adopt a custom file as follows:

1. Save the current file in Git or a user-controlled backup.
2. Review both supported templates and every custom field that replacement will discard.
3. Keep the file unmarked if any custom field must remain.
4. Resolve any `.github/dependabot.yaml` entry manually.
5. Set `languages.go` explicitly to `true` or `false` for the first reconciliation of a custom marked file.
6. Add the exact marker as the first line of the reviewed regular `.github/dependabot.yml` file.
7. Run `tailor baste` and review the complete preview scope.
8. Run `tailor alter` only after approval.
9. Review the generated file, then commit it through the normal workflow.

The first adoption of custom marked content requires an explicit Go Boolean. An absent declaration cannot identify whether custom content previously enabled Go.

`tailor baste` is read-only. `tailor alter` can change other declared resources in the same run. Neither command is a Dependabot-only operation.

Tailor will add no adoption command, flag, ownership ledger, or automatic migration. A language declaration, `always`, and `--recut` do not grant ownership.

## Composition precedence

The planner will apply these rules in order:

1. Resolve the effective swatch entry after existing default merging.
2. If the entry is omitted or `never`, skip all Dependabot inspection and writing.
3. For an active entry, inspect only the two allowed destination names and their parents.
4. Reject unsafe paths and any `.github/dependabot.yaml` entry before reading or writing `.yml` content.
5. If `.yml` is missing, select a canonical body from the declaration table.
6. If `.yml` is unmarked, preserve every byte and report manual adoption guidance.
7. If `.yml` is marked, determine the selected Go state.
8. Render one complete marked output, then compare exact bytes.
9. Publish only a changed, safe plan after the snapshot recheck.

Mode and safety gates override ownership and declaration rules. Ownership gates override canonical state detection. Declaration selection then controls the complete generated body.

The generated output is the exact marker line plus one canonical template body. Canonical fixtures contain the body only, without the marker.

## Alteration and ownership matrix

Active means `first-fit` or `always`, with or without `--recut`.

| Effective mode | File state | Future result |
| --- | --- | --- |
| `never`, including `--recut` | Any state, including unsafe paths and alternate files | Skip without Dependabot inspection or writes. |
| Omitted | Any state | Skip without Dependabot inspection or writes. |
| Active | Unsafe parent, final symlink, directory, special file, unreadable input, or oversized input | Report a conflict, preserve all paths, and stop before mutation. |
| Active | Any `.github/dependabot.yaml` entry exists | Report a conflict, preserve both names, and create no `.yml`. |
| `first-fit` | Missing `.yml`, no conflict | Create marked canonical output from the declaration table. |
| `always` | Missing `.yml`, no conflict | Create marked canonical output from the declaration table. |
| `first-fit --recut` | Missing `.yml`, no conflict | Create marked canonical output from the declaration table. |
| `always --recut` | Missing `.yml`, no conflict | Create marked canonical output from the declaration table. |
| `first-fit` | Unmarked regular `.yml` | Preserve all bytes and report manual adoption guidance. |
| `always` | Unmarked regular `.yml` | Preserve all bytes and report manual adoption guidance. |
| `first-fit --recut` | Unmarked regular `.yml` | Preserve all bytes and report manual adoption guidance. |
| `always --recut` | Unmarked regular `.yml` | Preserve all bytes and report manual adoption guidance. |
| `first-fit` | Owned regular `.yml` | Reconcile by declaration, skip identical output, or report an absent-state conflict. |
| `always` | Owned regular `.yml` | Reconcile by declaration, skip identical output, or report an absent-state conflict. |
| `first-fit --recut` | Owned regular `.yml` | Reconcile by declaration, skip identical output, or report an absent-state conflict. |
| `always --recut` | Owned regular `.yml` | Reconcile by declaration, skip identical output, or report an absent-state conflict. |

An unreadable unmarked file is still protected. Tailor must complete safe inspection before it can classify the file as unmarked, so a read failure is a conflict.

An alternate entry means any filesystem entry at `.github/dependabot.yaml`. A regular file, directory, special file, final symlink, or dangling symlink blocks active management.

Default merging can restore an omitted entry. Use `alteration: never` for a durable opt-out.

## Declaration matrix

Every canonical variant contains GitHub Actions and Nix.

| `languages.go` | Missing file | Existing owned file |
| --- | --- | --- |
| `true` | Create the canonical Go-enabled variant. | Render the canonical Go-enabled variant. Replace changed output or skip identical bytes. |
| `false` | Create the canonical Go-disabled variant. | Render the canonical Go-disabled variant. Replace changed output or skip identical bytes. Never delete the complete file. |
| Absent | Create the Go-enabled variant for legacy compatibility. | Retain a recognised prior Go state, then render its current variant. Report a conflict for an unknown prior state. |

New Tailor configurations explicitly declare false. New projects therefore normally receive the Go-disabled variant. A legacy configuration with no declaration receives the Go-enabled variant only when the destination is missing.

### Owned state transitions

| Recognised prior state | New declaration | Future result |
| --- | --- | --- |
| Go enabled | `false` | Replace with canonical Actions and Nix only. |
| Go enabled | Absent | Keep Go enabled. Replace with the current enabled variant, or skip identical bytes. |
| Go disabled | `true` | Replace with canonical Actions, Go, and Nix. |
| Go disabled | Absent | Keep Go disabled. Replace with the current disabled variant, or skip identical bytes. |
| Either | The same explicit Boolean | Render that Boolean variant and skip identical bytes. |
| Unknown or customised owned bytes | Absent | Report a conflict and preserve bytes. Request explicit `true` or `false`, or marker removal. |
| Unknown or customised owned bytes | `true` or `false` | Replace the complete file under the recorded whole-file consent. |

The absent-state rule prevents silent enablement or disablement. It does not preserve custom fields after the user supplies an explicit Boolean.

## Canonical state recognition

Absent-state detection uses a finite, reviewed set of canonical bodies. It does not parse YAML or search for substrings.

The initial set contains the two current template bodies:

- `swatches/.github/dependabot.yml`, mapped to Go enabled.
- `swatches/go/dependabot-disabled.yml`, mapped to Go disabled.

The comparison starts immediately after the exact marker newline. Marker and body line endings are independent for recognition. For each canonical body, Tailor must recognise all four combinations of an LF or CRLF marker newline and an exact LF or whole-body CRLF canonical body. All four combinations map to the same prior Go state. Tailor does not trim comments, whitespace, keys, or trailing content.

Rendering always uses the marker with an LF newline followed by the exact LF canonical template body. A recognised CRLF marker or body can therefore cause one complete-file replacement. The next run must report unchanged bytes.

Future template changes must retain recognised historical bodies as private compatibility fixtures. Each fixture must map to exactly one Go state. An unknown or ambiguous match is a conflict.

Canonical matching identifies state only. It never grants ownership.

## Safety and filesystem limits

The dedicated planner will inspect only `.github/dependabot.yml` and `.github/dependabot.yaml` beneath the rooted project directory. It will use `Lstat` and will not follow final symlinks.

For active management, Tailor will reject:

- linked parents.
- a final symlink at either allowed name.
- directories and special files.
- read failures.
- input or planned output above 1 MiB (1,048,576 bytes).

The planner must reject unsafe paths before any mutation, including when a later ownership result would preserve an unmarked file. `never` and omission bypass inspection, including unsafe-path inspection.

Before publication, Tailor will recheck both names, parent safety, destination type, marker, and exact snapshot bytes. A changed type, marker, alternate entry, or byte sequence is drift and causes a conflict. Tailor will not silently replan.

A missing-file publication must not replace a file that appears after preflight. A changed-file publication will use rooted temporary creation, file sync, close, atomic rename, and directory sync.

Atomic rename prevents partial content for one file. It does not make the complete run transactional, and a writer can still race between the final check and rename.

## Preview and reporting

Preview and apply will report one of these Dependabot outcomes:

| Outcome | Meaning |
| --- | --- |
| Create | The destination is missing and the planner selected a canonical variant. |
| Complete-file replacement | An owned file differs from the selected canonical output. |
| Unchanged | An owned file already equals the complete selected output. |
| Preserved, unadopted | A safe regular `.yml` lacks the exact marker. |
| Skipped | The effective entry is omitted or `never`. |
| Conflict | Safety, alternate-name, state-recognition, or drift checks failed. |

For replacement, the report will name the selected ecosystems and state that custom fields will disappear. It must not print old custom values, registry values, secrets, or a complete old-file diff.

`tailor baste` will create no file, directory, temporary file, or ownership record. Reporting after `tailor alter` will include only confirmed results when a write fails part-way through the wider run.

## One writer

Dependabot processing will move to one dedicated whole-file planner. The implementation must exclude `.github/dependabot.yml` from ordinary swatch dispatch and from `prepareGoSwatches` when that planner owns the operation.

One planner will produce one result and at most one writer for the destination. Ordinary dispatch must not perform a second write.

Dependabot remains a configured health swatch. The design does not assign it the managed registry's fragment policy.

## Reuse boundaries

The future implementation can reuse these current helpers directly where their contracts match:

- [`Config.GoDeclared` and `Config.GoEnabled`](../../internal/config/languages.go) for declaration state.
- [`swatch.Render`](../../internal/swatch/go.go) for the two current canonical bodies.
- [`managedMarker` and `hasManagedMarker`](../../internal/alter/managed_plan.go) for the exact first-line marker.
- [`checkParents`](../../internal/alter/swatch.go) for rooted parent checks.

The implementation can use current managed code as patterns, not unchanged drop-in functions:

- [`prepareManagedExecution`](../../internal/alter/managed_reporting.go) for complete preflight before mutation.
- [`renderManagedFiles`](../../internal/alter/managed_render.go) for deterministic rendering.
- [`managedExcludedPaths` and `managedPlannedResults`](../../internal/alter/managed_reporting.go) for one-writer exclusion and reporting.
- [`writeManagedFile`](../../internal/alter/managed_write.go) for rooted publication and sync steps.

Current ordering, ownership errors, symlink rules, and policy semantics differ from this contract. In particular, [`recheckManagedPlanFile`](../../internal/alter/managed_apply.go) replans from a fresh snapshot. Dependabot needs an exact drift check against its recorded snapshot instead.

## Rejected alternatives

| Alternative | Reason |
| --- | --- |
| Infer ownership from canonical bytes | Matching content is not user consent. |
| Treat a Go declaration as consent | A language choice does not authorise loss of custom Dependabot fields. |
| Let `always` or `--recut` grant consent | These modes do not communicate whole-file adoption. |
| Merge arbitrary YAML | Comments, ordering, unsupported fields, and future GitHub options make a general merge unsafe. |
| Remove only a managed Go fragment | Dependabot has one shared YAML file, and the current fragment registry does not model entries inside it. |
| Delete the file when Go is false | Actions and Nix remain enabled independently. |
| Add a flag or ownership ledger | The exact file marker records sufficient whole-file consent. |
| Parse YAML to infer an absent prior state | Equivalent syntax and custom content can hide an ambiguous state. Finite canonical fixtures are deterministic. |

## Acceptance matrix

| Area | Required cases |
| --- | --- |
| Modes | Cross `first-fit`, `always`, and both recut forms with missing, unmarked, owned, and conflicting files. Prove omission and `never` perform no inspection. |
| Declarations | Cover true, false, and absent for missing and owned files. Prove true-to-false, true-to-absent, false-to-true, false-to-absent, and repeated application. |
| Composition | Prove that both canonical variants retain Actions and Nix. Prove that false removes only `gomod`. |
| Ownership | Accept the exact LF and CRLF marker lines. Reject a byte-order mark, leading blank line, partial marker, wrong path, extra text, missing marker newline, and matching unmarked content. |
| Canonical fixtures | For enabled, disabled, and retained historical bodies, cross LF and CRLF marker newlines with exact LF and whole-body CRLF forms. Prove that all four combinations retain the same prior Go state, render the LF marker plus exact LF body, replace CRLF bytes once when needed, and report unchanged on repetition. Reject changed whitespace, comments, unknown bodies, and ambiguous fixture mappings. Fixtures exclude the marker. |
| Custom files | Preserve Tailor's current unmarked file and other custom files under first-fit, always, and recut. Require an explicit Boolean for first reconciliation after custom marked adoption. |
| Alternate name | Reject alternate-only and both-name states. Cover regular files, directories, special files, final symlinks, and dangling symlinks at `.yaml`. |
| Destinations | Cover missing files, regular files, linked parents, final symlinks, directories, special files, unreadable files, and both 1 MiB limits. |
| Safe inspection | Prove that an unreadable unmarked file conflicts before ownership classification. Prove that omission and `never` inspect neither filename. |
| Drift | Reject byte drift, type drift, marker removal, alternate appearance, and parent changes. Do not clobber a file that appears during missing-file publication. |
| Preview | Prove that `baste` writes no destination, directory, temporary file, or ownership record. Report outcome and selected ecosystems without old values or secrets. |
| Integration | Prove one result and one writer. Exclude Dependabot from ordinary dispatch and Go preparation. Preserve unrelated Go, managed development, and configuration-default behaviour. |
| Failure reporting | Inject publication failures and report only confirmed changes. Document the final check-to-rename race without promising a transaction. |

## Bounded future implementation plan

These phases are proposals. They grant no production authority and create no tracked assignment.

| Phase | Exact scope | Work and dependency |
| --- | --- | --- |
| F1: Pure planner and rendering | Add `internal/alter/dependabot.go` and `internal/alter/dependabot_test.go`. | Reuse `internal/swatch/go.go` and both templates without changing public behaviour. Prove mode, state, marker, canonical fixture, and deterministic-byte rules. Depends on this accepted design. |
| F2: One safe execution path | Change `internal/alter/dependabot.go`, `internal/alter/alter.go`, `internal/alter/go.go`, and `internal/alter/managed_reporting.go`. Add `internal/alter/dependabot_apply.go` and `internal/alter/dependabot_apply_test.go`. | Connect preflight, ordinary-dispatch exclusion, preview, reporting, exact snapshot checks, and rooted publication. Preserve unrelated managed policies. Depends on F1. |
| F3: Regression acceptance | Change `internal/alter/dependabot_test.go`, `internal/alter/dependabot_apply_test.go`, `internal/alter/go_test.go`, `internal/alter/managed_integration_test.go`, and `internal/swatch/go_test.go`. | Prove every acceptance row, one writer, and write-free preview. Depends on F2. |
| F4: Ship current-behaviour documentation | Change `docs/design/dependabot-ownership.md`, `docs/SPECIFICATION.md`, `wiki/Configuration.md`, and `wiki/Commands.md`. | Change future statements to current only after F3 passes. Depends on F3. |

Run these commands for future implementation acceptance:

```sh
TMPDIR=/tmp just test
just lint
```

The future implementation estimate is **M, 3 points** because it changes several files in one subsystem with tests and documentation. This value is a planning estimate only, not a live tracker assignment. The coordinator must verify the live team scale before assigning an estimate.

WW-284 remains a focused S design scope.

## References

- [Current specification](../SPECIFICATION.md#go-ecosystem-support)
- [Go renderer](../../internal/swatch/go.go)
- [Go declaration helpers](../../internal/config/languages.go)
- [Ordinary swatch processing](../../internal/alter/swatch.go)
- [Managed ownership checks](../../internal/alter/managed_plan.go)
- [Managed preflight](../../internal/alter/managed_preflight.go)
- [Managed reporting](../../internal/alter/managed_reporting.go)
- [Managed publication](../../internal/alter/managed_write.go)
- [Dependabot configuration file](https://docs.github.com/en/code-security/concepts/supply-chain-security/about-the-dependabot-yml-file)
- [Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)
- [Supported ecosystems](https://docs.github.com/en/code-security/reference/supply-chain-security/supported-ecosystems-and-repositories)
