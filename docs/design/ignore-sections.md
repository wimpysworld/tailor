# Adopted `.gitignore` sections

Status: approved design for future work. Tailor does not reconcile these sections yet. The [current contract](../SPECIFICATION.md#current-gitignore-behaviour) remains authoritative until the follow-on work ships.

## Goal

Tailor will update only sections that a user adopts. Project rules remain editable in the root `.gitignore`, with no Git configuration or generated source file.

The design reserves `tailor:ignore:` and these exact marker pairs:

```gitignore
# tailor:ignore:base:start
# tailor:ignore:base:end
# tailor:ignore:go:start
# tailor:ignore:go:end
# tailor:ignore:pages:start
# tailor:ignore:pages:end
```

The only names are `base`, `go`, and `pages`. Markers must be standalone lines with the exact spelling and spacing. A final end marker can omit its newline.

No marker means that the root is unadopted. Tailor will not infer ownership from matching patterns, a language, a generator, or the whole-file marker used by managed development files.

## Ownership and validation

Tailor owns only bytes between one valid marker pair. Marker bytes and every byte outside a section remain unchanged. This includes comments, blank lines, rule order, duplicate rules, mixed line endings, and the final-newline state.

When Tailor changes a body, each generated body line uses the newline from that section's start marker. An empty body leaves the two consent markers. Tailor does not sort, move, or deduplicate rules.

The parser must reject all malformed uses of the reserved `tailor:ignore:` prefix before any mutation:

| Input | Result |
| --- | --- |
| One exact pair for a supported name | Adopt that section. |
| No reserved marker | Treat the root as unadopted. |
| Start or end without its partner | Reject as partial. |
| Two starts, two ends, or two pairs with one name | Reject as duplicate. |
| End before start | Reject as reversed. |
| A marker inside another section | Reject as nested. |
| Marker text before or after the exact line | Reject as inline. |
| Unsupported name | Reject as unknown. |
| Extra or missing whitespace, or changed case | Reject as modified. |

Tailor limits both the input snapshot and planned output to 1 MiB (1,048,576 bytes). It rejects an unreadable or oversized input. It also rejects a directory, special file, unsafe parent, or final symlink. A final symlink stays unchanged, and Tailor requests manual adoption in the symlink target or replacement with a regular file.

## Order and adoption

A new root uses `base`, `go`, `pages` order. Tailor includes `base` and each explicitly enabled optional section. New sections precede project rules.

An existing root keeps every section at its current position. Tailor warns when project rules occur before a managed section because later matching rules can change the result. Tailor never relocates an adopted section.

Tailor inserts no missing section into an existing root. The user must add the marker pair manually. This rule applies when another supported section is already adopted.

```gitignore
# tailor:ignore:base:start
# Tailor writes fixed embedded base rules here.
# tailor:ignore:base:end

# Project-owned rules remain here.
!important.log
logs/
!logs/keep.log
```

The last matching rule wins. The `!important.log` rule can override an earlier managed rule. Git cannot re-include `logs/keep.log` while an ancestor directory remains excluded. The user must also re-include the ancestor, for example `!logs/` before `!logs/keep.log`.

Historical rules outside a managed section remain effective. A duplicate project rule can keep a path ignored after Tailor removes the same rule from a managed body.

## Decision matrices

### Root scope

The effective `.gitignore` swatch entry is the result after config default merging. An omitted entry can return when default merging restores it. Set `alteration: never` for a durable opt-out.

| Effective swatch scope | Missing root | Existing root | Inspection |
| --- | --- | --- | --- |
| `always` | Create the planned root. | Apply only adopted-section actions. | Inspect and validate. |
| `first-fit` | Create the planned root. | Apply only adopted-section actions. | Inspect and validate. |
| `always` or `first-fit` with `--recut` | Same as normal mode. | Same ownership limits as normal mode. | Inspect and validate. |
| Omitted | Do not create the root. | Reconcile existing adopted sections only. | Inspect only an existing root. |
| `never` | Do nothing. | Do nothing. | Do not inspect. |

An omitted entry limits root creation, not consent that existing markers already grant. `always`, `first-fit`, and `--recut` never grant section ownership.

### Section declarations

| Section | Declaration | Missing permitted root | Adopted section | Missing section in existing root |
| --- | --- | --- | --- | --- |
| `base` | Active root scope | Include fixed embedded base rules. | Replace the body with fixed embedded base rules. | Preserve the file and suggest the exact pair. |
| `base` | Omitted root scope | Do not create the root. | Replace the body with fixed embedded base rules. | Preserve the file and suggest the exact pair. |
| `base` | `never` | No inspection or write. | No inspection or write. | No inspection or write. |
| `go` | `languages.go: true` | Include only `*.test`. | Replace the body with only `*.test`. | Preserve the file and suggest the exact pair. |
| `go` | `languages.go: false` | Omit the section. | Empty the body and retain both markers. | Preserve the file. |
| `go` | `languages.go` absent | Omit the section. | Preserve the body. | Preserve the file. |
| `pages` | Enabled Hugo or Jekyll, and Pages available | Include the escaped output rule. | Replace the body and preview removed exclusions. | Preserve the file and suggest the exact pair. |
| `pages` | False, absent, static, or unavailable | Omit the section. | Preserve the body. | Preserve the file. |

The initial Go body contains exactly one rule, `*.test`. It contains no executable-name or coverage-profile patterns.

`never` overrides every declaration row. Tailor never infers `languages.go` from source files or code scanning settings.

### Pages transition

The current implementation appends a Hugo or Jekyll rule through [`processPagesIgnore`](../../internal/alter/pages_ignore.go). It preserves existing regular-file text, but it can replace a final symlink. The acceptance test preserves `old-rule/` during recut in [`pages_acceptance_test.go`](../../internal/alter/pages_acceptance_test.go).

The future contract replaces that append step:

| Pages state | Adopted `pages` section | Unadopted existing root | Missing permitted root |
| --- | --- | --- | --- |
| Enabled Hugo | Reconcile to the escaped `<path>/public/` rule. | Preserve all bytes and show an adoption snippet. | Create `base`, then `pages`. |
| Enabled Jekyll | Reconcile to the escaped `<path>/_site/` rule. | Preserve all bytes and show an adoption snippet. | Create `base`, then `pages`. |
| Enabled static | No change. | No change. | Create `base` only. |
| Disabled or absent | No change. | No change. | Follow root scope without a Pages section. |
| Unavailable or skipped | No change. | No change. | Make no Pages-section change. |

This migration does not remove historical appended rules. Users can move a reviewed rule into the adopted section manually. Preview must show exclusions that a changed adopted body removes.

## Planning and publication

One ignore writer replaces ordinary `.gitignore` dispatch and the later Pages append writer. Tailor builds one rooted snapshot and complete plan before any local or remote mutation. The plan includes section output, snippets, warnings, and conflicts.

Before publication, Tailor rechecks the destination identity, type, and bytes against the snapshot. Drift is a conflict. Tailor does not silently read the new file and make a new plan.

For a changed regular file, Tailor creates an exclusive sibling temporary file, writes and syncs it, closes it, renames it over the destination, then syncs the directory. For a missing root, publication uses no-clobber creation. The existing rooted writer in [`managed_write.go`](../../internal/alter/managed_write.go) is the implementation precedent.

These checks use compare-and-swap discipline, but the filesystem operation is not an atomic compare-and-swap. An unrelated writer can change the destination between the final check and rename. Atomic rename prevents partial file content only. Atomicity does not cover the complete run or remote operations.

If drift occurs before publication, preserve the changed file, report a conflict, run `tailor baste` again, review the new plan, then retry `tailor alter`. If a rename race occurs, recover the project-owned version from Git or backup, review the Tailor result, and retry after one writer has control.

## Conflict recovery

| Conflict | Recovery |
| --- | --- |
| Malformed reserved marker | Restore one exact, non-nested pair for each adopted name, or remove all reserved marker text. Run `tailor baste` again. |
| Unknown reserved name | Rename it to `base`, `go`, or `pages`, or remove the reserved prefix. |
| Final symlink | Edit its target manually, or replace the symlink with a reviewed regular root. Tailor will not follow or replace it. |
| Linked parent, directory, or special file | Move valuable content aside, restore a real parent and regular root, then preview again. |
| Unreadable or oversized root | Restore read access or reduce the file to at most 1,048,576 bytes. |
| Snapshot drift | Keep the external edit, run `tailor baste` on the new bytes, and retry after review. |
| Check-to-rename race | Recover the intended project file from Git or backup, stop the competing writer, and retry. |

## Rejected alternatives

| Alternative | Reason |
| --- | --- |
| Replace the complete root | It removes project ownership and user exceptions. |
| Match known rule text as consent | Matching text does not grant ownership. |
| Append Pages rules to unadopted roots | A new trailing rule can override user negations. |
| Use a separate generated ignore source | Git has no portable tracked include, and users must edit another file. |
| Use nested `.gitignore` files | Their rules apply only to their subtrees. |
| Configure `.git/info/exclude`, `core.excludesFile`, or config includes | These sources are local or do not import ignore fragments for every clone. |
| Move or deduplicate project rules | Rule position and duplicates can change Git results. |
| Promise an atomic filesystem compare-and-swap | The check-to-rename race remains. |

## Acceptance matrix

| Area | Required cases |
| --- | --- |
| Parser | Exact `base`, `go`, and `pages` pairs. Partial, duplicate, reversed, nested, inline, unknown, and whitespace-modified markers fail. |
| Bytes | LF, CRLF, mixed line endings, final end marker without newline, final newline present or absent, comments, duplicates, and external bytes stay exact. Input and output reject more than 1 MiB. |
| Declarations | `go` true replaces, false empties, and absent preserves. Base uses embedded rules. Pages covers Hugo, Jekyll, static, false, absent, and unavailable. |
| Modes | `always`, `first-fit`, and recut have equal ownership limits. Omitted scope blocks missing-root creation. `never` performs no inspection. Default merging can restore omission. |
| Adoption | Missing root bootstraps sections in fixed order. Existing unmarked and partly adopted roots get snippets only. Existing sections keep their positions. |
| Git semantics | User negation, ancestor reinclusion, nested overrides, ordered duplicates, and historical duplicates use isolated `git check-ignore --no-index -v` fixtures. |
| Destinations | Regular, missing, final symlink, linked parent, directory, special file, unreadable file, and oversized file. |
| Concurrency | Byte drift and type or identity drift stop before rename. A destination that appears during missing-root publication is not replaced. Inject a change at the check-to-rename boundary and document recovery. |
| Integration | One writer produces one root result. Ordinary dispatch and Pages do not make a second write. Preview is read-only and lists removed Pages exclusions. |
| Regression | WW-287 root preservation and WW-286 `lint` output remain unchanged until production migration ships. |

## Follow-on work

These scopes are proposals, not tracked issues. Capacity does not permit issue creation in this phase. Complete them in order.

1. **Parser and planner (M).** Add `internal/alter/ignore_sections.go` and `internal/alter/ignore_sections_test.go`. Add `parseIgnoreSections`, `planIgnoreRoot`, `renderIgnoreSection`, and byte-preservation tests. Change no dispatch path.
2. **Integration and concurrency safeguards (M).** Add `internal/alter/ignore_apply.go` and `internal/alter/ignore_apply_test.go`. Connect one `planIgnoreRoot` call in `internal/alter/alter.go`. Replace `processPagesIgnore` in `internal/alter/pages_ignore.go` and ordinary `.gitignore` dispatch with `applyIgnorePlan`. Reuse rooted parent checks, add snapshot recheck hooks, exclusive temporary files, sync, rename, directory sync, and missing-root no-clobber publication.
3. **End-to-end migration and documentation (M).** Add cases to `internal/alter/pages_acceptance_test.go` and `internal/alter/alter_acceptance_test.go`. Reuse the existing `internal/alter/ignore_git_semantics_test.go` fixtures and add only implementation-driven cases. Update the specification and wiki from future to current only after all acceptance rows pass.

## References

- [Git ignore pattern rules](https://git-scm.com/docs/gitignore)
- [`git check-ignore` fixture command](https://git-scm.com/docs/git-check-ignore)
- [Current Pages ignore writer](../../internal/alter/pages_ignore.go)
- [Current rooted file writer](../../internal/alter/managed_write.go)
- [Current managed marker parser](../../internal/alter/managed_plan.go)
