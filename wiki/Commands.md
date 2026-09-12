# Commands

Run Tailor from your project directory. Use `baste` to preview changes before `alter`.

## Repository selection

Tailor selects a GitHub fetch remote in this order: `origin`, `github`, `upstream`, then another remote. A default set through `gh repo set-default` overrides this order.

This selection applies to `fit`, `alter`, `baste`, and `docket`. `fit` checks its target directory. The other commands check the current directory.

In a fork clone, `origin` usually selects your fork. Check the repository with `tailor docket` before `tailor baste` and `tailor alter`.

## `fit <path>`

Creates a project directory and writes `.tailor.yml` with the full default swatch set. It does not copy files or apply settings. `fit .` also works in an existing directory.

The default licence is BlueOak-1.0.0.

```bash
tailor fit ./my-project
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

When a GitHub remote exists, `fit` queries the live repository configuration for the `repository`, `code_scanning`, `code_quality`, and `ruleset` sections. Otherwise, built-in defaults are used. When a default setup read returns an access error or `404`, `fit` warns and writes the built-in section. `fit` always writes `languages: []`.

`fit` reads the `Tailor` ruleset when it exists and writes its live values. When the live `enforcement` is `evaluate`, `fit` warns and writes `active`. When the ruleset does not exist, or the read returns an access error or `403`, `fit` writes the built-in `ruleset` section. `fit` exits with an error if `.tailor.yml` already exists.

## `alter`

Reads `.tailor.yml` in the current directory. It applies repository settings, immutable releases, Actions policy, code scanning, Code Quality, the ruleset, labels, variables, Pages, wiki files, licence, and swatches in that order.

Wiki setup has one earlier write: after local safety checks, Tailor enables a declared wiki before its readiness check. If readiness fails, no other changes follow. See [GitHub wiki](GitHub-wiki) for setup steps.

```bash
tailor alter            # Apply changes
tailor alter --recut    # Overwrite always and first-fit swatches
```

`--recut` overrides `first-fit` and overwrites those swatches, but it still skips `never` swatches. Existing wiki starter pages, [static Pages starter files](GitHub-Pages#static-pages-starter), and regular `LICENSE` files are exempt. For `.tailor.yml`, see [default merging](Configuration#default-merging).

If a later step fails, completed local and repository changes remain. Tailor does not roll them back. Fix the reported error, then run `tailor baste` before you retry `tailor alter`.

`alter` and `alter --recut` report a completed label after each successful change. Labels are `set`, `created`, `updated`, `removed`, `copied`, and `overwritten`.

A default merge, retired-entry cleanup, or security prerequisite normalisation in `.tailor.yml` reports `updated`. Security normalisation also shows a warning. A retired workflow file cleanup reports `removed`.

## `baste`

`baste` previews the changes that `alter` will make. It makes no changes.

```bash
tailor baste
```

`baste` uses planned labels. The write commands use the corresponding completed labels:

| `baste` | `alter` and `alter --recut` |
|---------|-----------------------------|
| `would set` | `set` |
| `would create` | `created` |
| `would update` | `updated` |
| `would remove` | `removed` |
| `would copy` | `copied` |
| `would overwrite` | `overwritten` |

`skipped` and `no change` use the same labels in all three commands. A skipped file shows its reason after the path.

A default merge, retired-entry cleanup, or security prerequisite normalisation in `.tailor.yml` reports `would update` in `baste`. Security normalisation also shows a warning. After a successful write, `alter` reports `updated`.

Each present retired workflow file reports `would remove` in `baste` and `removed` after deletion.

```
would set:                           repository.has_wiki = false
would update:                        .tailor.yml
would remove:                        .github/workflows/tailor-automerge.yml
would remove:                        .github/workflows/tailor.yml
would copy:                          LICENSE
would overwrite:                     SECURITY.md
no change:                           CODE_OF_CONDUCT.md
skipped:                             .envrc (first-fit, exists)
skipped:                             .github/pull_request_template.md (mode never)
```

## `docket`

Displays the current GitHub authentication state and repository context.

When authenticated, `docket` verifies the token with `GET /user`. Pages adds no requests to this command.

```bash
tailor docket
```

## `measure`

Checks community health files and configuration alignment. No network access, no authentication, no `.tailor.yml` required.

The Pages workflow is not a community health file. Its registered path remains part of the configuration comparison, even when Pages is disabled.

```bash
tailor measure
```

```
       missing: .github/FUNDING.yml
       warning: LICENSE (contains unresolved placeholders)
       warning: README.md (not managed by tailor)
       present: CODE_OF_CONDUCT.md
not-configured: .github/dependabot.yml
  mode-differs: SECURITY.md (config: first-fit, default: always)
```

| Status | Meaning |
|--------|--------|
| `missing` | Health file does not exist on disk |
| `warning` | Missing `README.md`, known unresolved licence placeholders, or an uninspected licence |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `.tailor.yml` |
| `config-only` | Swatch in `.tailor.yml` not in the built-in default set |
| `mode-differs` | Alteration mode differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor.yml` is present.

## Local health diagnostics

`measure` checks which community health files are present, missing, or need attention. It warns when `README.md` is absent or when `LICENSE` contains a known unresolved placeholder. The licence check recognises `year`, `yyyy`, `fullname`, `name of copyright owner`, `name of copyright holder`, `software name`, `project`, `projecturl`, and `email` inside square or curly braces. Matching ignores case, allows ASCII whitespace beside the delimiters, and treats each internal sequence of ASCII whitespace as one space.

Other bracketed licence text and complete Markdown inline links do not cause a warning.

| Licence warning | Action |
|---|---|
| `(contains unresolved placeholders)` | Fill in the licence placeholders manually. |
| `(not inspected: exceeds 1 MiB)` | Inspect the licence manually. Tailor does not scan files above this limit. |
| `(not inspected: read failed)` | Check that `LICENSE` is a readable regular file, fix read access, then rerun `tailor measure`. |

## Justfile recipes

The `justfile` swatch provides these recipes. Install `just` to use them and `actionlint` for the lint recipe.

| Command | Runs | Requirements |
|---|---|---|
| `just` | Lists recipes | `just` |
| `just alter` | `tailor alter` | Tailor, GitHub authentication, valid `.tailor.yml` |
| `just lint` | `actionlint` | `actionlint` |
| `just measure` | `tailor baste`, then `tailor measure` | Tailor, GitHub authentication, valid `.tailor.yml` |

With [Go support](Configuration#go-support) enabled, a newly rendered `justfile` also includes these recipes:

| Command | Runs | Requirements |
|---|---|---|
| `just build` | `go build ./...` | Go |
| `just test` | `go test ./...` | Go |
| `just lint` | `golangci-lint run`, then `actionlint` | Go, golangci-lint, actionlint |

Unlike `tailor measure`, `just measure` needs authentication because its first command is `tailor baste`. If that preview fails, the recipe stops before the local health check.

Extend the first-fit `justfile` with your project recipes. Normal `alter` runs preserve it. `alter --recut` overwrites it unless its mode is `never`.

## Retired workflow cleanup

Run `tailor baste` to preview the upgrade.

Run `tailor alter` to apply it. Tailor cleans up both retired workflows automatically:

- `.github/workflows/tailor-automerge.yml`
- `.github/workflows/tailor.yml`

`baste` changes no files. It reports `would update` when `.tailor.yml` contains retired entries and `would remove` for each retired workflow file on disk.

`alter` and `alter --recut` write `.tailor.yml` once as `updated` when the config contains retired entries. They then delete each present workflow file as `removed`.

Tailor accepts the historical `triggered` mode only for retired entries during this migration. Any other unrecognised swatch path stops validation before Tailor changes any files.
