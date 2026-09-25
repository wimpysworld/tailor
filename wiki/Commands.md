# Commands

Run Tailor from your project directory. Use `baste` to preview changes before `alter`.

## FFmpeg-statigo setup

`just setup` is the only setup recipe. It runs inline shell bodies for explicitly enabled components in lexical order, each in a separate Bash subshell. A failed component stops setup. With no component enabled, setup succeeds with guidance. Setup never invokes project quality scripts.

Run setup only after review. It sets local `git config pull.ff only` and `git config submodule.recurse true`. It can initialise recursive submodules, fetch and check out the latest stable release, remove old `third_party/ffmpeg-statigo/lib/*/libffmpeg.a` archives, stage the submodule change, and download libraries. It requires network access. It rejects the ffmpeg-statigo source root. Tailor commands, shell entry, build, test, and lint never invoke setup automatically.

The effective CGO policy generates one shared export in `just/tailor.just`. Explicit false sets `CGO_ENABLED=0`, and explicit true or enabled ffmpeg-statigo sets `CGO_ENABLED=1`. All managed, imported, and root recipes and project scripts inherit this value instead of the caller value, even outside the development shell. Disabled Go or an unspecified policy generates no export and preserves the caller environment. After a policy change, regenerate managed files. Reload the development shell for direct shell commands, which use the Go hook. Just reads the export on each invocation without a shell reload.

The optional development CGO policy does not change the default `CGO_ENABLED=0` release pipeline. Configure FFmpeg release builds separately.

For a preserved flake, adopt both the package loader and the [hook companion](Configuration#nix-shell-hooks) manually. Keep your existing hook last for overrides. Use `TAILOR_NIXOS_DRIVERS=0` to disable graphics environment changes without disabling CGO policy.

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

`fit` uses the embedded `swatches/.tailor.yml` defaults for all managed settings, including for existing projects. It copies only repository `description` and `homepage` from GitHub, exactly, including empty strings. An explicit `--description` takes precedence. Without a GitHub remote, the description defaults to the project directory name and the homepage stays omitted.

Review `.tailor.yml` and run `tailor baste` before `tailor alter`. The defaults can disable an existing wiki, Code Quality and immutable releases, and replace the `Tailor` ruleset. Topics stay unmanaged because the embedded defaults omit them.

`fit` exits with an error if `.tailor.yml` already exists. Existing configs retain their declared values, including values copied from GitHub by an earlier `fit`. [Default merging](Configuration#default-merging) stays append-only, including with `--recut`.

## `alter`

Reads `.tailor.yml` in the current directory. It applies repository settings, immutable releases, Actions policy, code scanning, Code Quality, the ruleset, labels, variables, Pages, wiki files, and the licence. The swatch stage then publishes Dependabot, managed development files, and ordinary swatches. Pages files follow.

Wiki setup has one earlier write: after local safety checks, Tailor enables a declared wiki before its readiness check. If readiness fails, no other changes follow. See [GitHub wiki](GitHub-wiki) for setup steps.

```bash
tailor alter            # Apply changes
tailor alter --recut    # Overwrite eligible always and first-fit swatches
```

`--recut` overrides `first-fit` for ordinary swatches, but it still skips `never`. Existing `justfile`, `flake.nix`, and `.gitignore` roots, wiki starter pages, [static Pages starter files](GitHub-Pages#static-pages-starter), and regular `LICENSE` files are exempt. Ordinary `.gitignore` processing preserves an existing regular file or final symlink byte-for-byte under every mode. It does not read or hash the file, or follow the symlink.

For an active mode other than `never`, Tailor creates a missing `.gitignore` atomically without clobbering a destination that appears during the write. It rejects a directory or special file before repository checks, authentication, or writes. `never` skips that inspection and write. Enabled Hugo or Jekyll Pages is a later additive exception that can replace a final symlink and append its output rule. Review the rule order when an existing matching rule precedes a later negation. For `.tailor.yml`, see [default merging](Configuration#default-merging).

Dependabot uses [whole-file ownership](https://github.com/wimpysworld/tailor/blob/main/docs/design/dependabot-ownership.md). The exact first-line marker is `# Managed by Tailor: .github/dependabot.yml`. Back up a custom file before adding it because the next reconciliation can remove every custom field.

`first-fit` and `always` reconcile owned content, with or without `--recut`. They preserve an unmarked regular file byte-for-byte. An omitted entry or `never` skips both `.yml` and `.yaml` names without inspection.

Active Dependabot preflight runs before repository context, authentication, wiki enablement, or other writes. It rejects alternate `.yaml` entries, unsafe paths, unreadable input, and input or output above 1 MiB. Publication waits until the swatch stage after the licence stage.

Before publication, Tailor rechecks both names, the parent identity, the destination identity and type, the marker, and exact bytes. Creation never replaces a destination that appears after preflight. Replacement uses file sync, close, atomic rename, and directory sync.

When Pages or wiki creates a missing `.github` parent in the same run, Dependabot accepts only the captured private directory identity. Linux and macOS support protected no-replace parent publication. On other platforms, a missing parent returns `errors.ErrUnsupported`; runtime acceptance is not claimed there.

The final check-to-rename race remains, and the complete `alter` run is not a transaction. If publication succeeds before a later sync, cleanup, close, or stage error, Tailor reports the confirmed change and returns the error.

The following workflow is for a future release. Tailor will first build one rooted `.gitignore` snapshot and plan before local or remote mutation. One writer will handle ordinary, Go, and Pages rules. `baste` will show marker snippets for an unadopted existing root, without changing it.

Before publication, Tailor will recheck the file identity, type, and bytes. If they changed, Tailor will report a conflict instead of making a new plan. Run `tailor baste` again, review the new plan, then retry `tailor alter`.

Publication will use an exclusive sibling temporary file, file sync, close, rename, and directory sync. Missing-root publication will not replace a destination that appears. These checks are not an atomic filesystem compare-and-swap because another writer can change the destination between the final check and rename. If that race occurs, recover the project-owned file from Git or backup before retrying with one writer.

Future `.gitignore` atomicity will cover one file only. It will not cover remote operations or the complete `alter` run. Final symlinks, linked parents, directories, special files, unreadable files, and files above 1 MiB will stop future reconciliation. See the [future contract](https://github.com/wimpysworld/tailor/blob/main/docs/design/ignore-sections.md).

If a later step fails, completed local and repository changes remain. Tailor does not roll them back. Fix the reported error, then run `tailor baste` before you retry `tailor alter`.

`alter` and `alter --recut` report a completed label after each successful change. Labels are `set`, `created`, `updated`, `removed`, `copied`, and `overwritten`.

A default merge, retired-entry cleanup, or security prerequisite normalisation in `.tailor.yml` reports `updated`. Security normalisation also shows a warning. A retired workflow file cleanup reports `removed`.

## `baste`

`baste` previews the changes that `alter` will make. It makes no changes.

For Dependabot, `baste` distinguishes creation, complete-file replacement, unchanged owned content, preserved unmarked content, skipped management, and conflicts. It creates no file, directory, temporary file, or ownership record.

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

The generated files require Just 1.23.0 or later. The protected root imports managed fragments and owns the sole `default` recipe.

| Command | Behaviour |
|---|---|
| `just build` | Validate Go packages and publish executable packages to `bin/` |
| `just test` | Run full `go vet ./...` once, then `go test -cover -covermode=atomic ./...` with console coverage |
| `just lint` | Read-only checks, identical to `just lint check` |
| `just lint correct` | Run `go mod tidy` and the configured formatter, then repeat all checks |

For Go, lint checks run sequentially: `go mod tidy -diff`, `golangci-lint fmt --config .golangci.yml --diff`, workflow lint, `golangci-lint run --config .golangci.yml`, `govulncheck -show verbose ./...`, then diagnostic `gocyclo`. Each failure stops later commands. Lint does not run vet or tests. Test coverage stays in the console and creates no `coverage.out`. Lint rejects modes other than `check` and `correct` before any work.

Optional project-owned scripts at `just/project/build.sh`, `just/project/test.sh`, and `just/project/lint.sh` run with Bash after their managed commands succeed. Scripts run from the project root. The lint script receives `check` or `correct` as its first argument. Keep its check mode read-only. Missing scripts are skipped. Symlinks and non-regular scripts, or linked or non-directory parents, fail visibly. Tailor does not generate or overwrite these scripts. Do not expose alternative quality recipes through wrappers.

Go commands can download modules and write caches. Vulnerability checks can use the network. Setup and release remain separate operational commands. Benchmark and profile commands remain future work.

Operational commands remain separate:

| Command | Behaviour |
|---|---|
| `just` | List recipes |
| `just setup` | Run explicitly enabled component setup |
| `just release x.y.z` | Validate the clean tree and create a local version tag |
| `just pages` | Preview enabled Pages at `http://127.0.0.1:18473` |

Call `tailor alter`, `tailor baste`, and `tailor measure` directly. From Tailor's source checkout, use `go run ./cmd/tailor alter`, `go run ./cmd/tailor baste`, or `go run ./cmd/tailor measure`.

Tailor's project fragment retains `snapshot`. Snapshot runs `goreleaser release --snapshot --clean --skip=sign`. Tailor's project build script adds `go build -ldflags "-s -w" -o tailor ./cmd/tailor` after the managed build. No quality aliases remain.

### Activate Playwright MCP

After you set `mcp.playwright: true`, preview and apply the five managed files:

```bash
tailor baste
tailor alter
git status --short
```

Review each new Nix and client file, then stage the approved paths with `git add`. Nix flakes ignore untracked files. Tailor does not stage files.

When Tailor preserves `flake.nix`, add the [Nix loader expression](Configuration#managed-development-files) to its package list. Do the same for the Just loader only when you need other managed recipes. Playwright has no Just fragment or recipe.

Enter the project shell with `nix develop`, or reload direnv after `direnv allow`. Start a new client instance from that environment, or use the client's configuration reload command. Then approve Playwright MCP tool calls through the client's normal approval prompt.

The configured MCP server starts its own headless, isolated browser. Do not start a manual stdio server or CDP endpoint. Start `just pages` only when you also want the separate Pages preview server.

A [validated future contract](https://github.com/wimpysworld/tailor/blob/main/docs/design/mcp-adoption.md) defines the proposed `--adopt-mcp` and `--release-mcp` flags. Tailor does not implement these flags and does not infer consent. Future adoption will use a local ownership ledger bound to the project. Entry edits will preserve non-target client bytes, and release will preserve all client file bytes. Release is blocked while a transaction is pending. Reviewed recovery comes first. Client-version acceptance and adoption implementation remain separate follow-on work.

`tailor measure` checks local health without authentication. It does not include the authenticated `tailor baste` preview that the removed `just measure` wrapper ran first. Run `tailor baste` separately when you need that preview.

Tailor preserves an existing root and gives loader adoption guidance, including for `never`, `--recut`, or an omitted swatch entry. Before you add the exact Just loader import from [managed development files](Configuration#managed-development-files), remove or rename each user recipe that duplicates a managed recipe, including `build` and `test`. Keep custom checks under distinct wrapper names. Tailor does not rewrite user recipes or change production recipe names.

A partial `tailor alter` can leave the imported Just files temporarily invalid. Fix the reported error, then retry `tailor alter` before you run a recipe. The generated dispatch is consistent, but installed tool versions and configuration can produce different diagnostics. This lint aggregation change adds no tool pin or CI policy change.

## Retired workflow cleanup

Run `tailor baste` to preview the upgrade.

Run `tailor alter` to apply it. Tailor cleans up both retired workflows automatically:

- `.github/workflows/tailor-automerge.yml`
- `.github/workflows/tailor.yml`

`baste` changes no files. It reports `would update` when `.tailor.yml` contains retired entries and `would remove` for each retired workflow file on disk.

`alter` and `alter --recut` write `.tailor.yml` once as `updated` when the config contains retired entries. They then delete each present workflow file as `removed`.

Tailor accepts the historical `triggered` mode only for retired entries during this migration. Any other unrecognised swatch path stops validation before Tailor changes any files.
