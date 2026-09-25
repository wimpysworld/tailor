# Configuration

Use `.tailor.yml` to choose the files and GitHub settings that Tailor manages.

[Swatches](#swatches) · [Managed development files](#managed-development-files) · [Go support](#go-support) · [MCP support](#mcp-support) · [Licences](#licences) · [Default set](#default-swatch-set) · [Modes](#alteration-modes) · [Default merging](#default-merging) · [Config example](#config-file)

## Swatches

Swatches are complete template files embedded in the tailor binary. Tailor copies most swatches unchanged. Tailor replaces tokens in three files at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | Authenticated user from `GET /user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | GitHub repository context |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | GitHub repository context |

Without repository context, Tailor leaves `{{ADVISORY_URL}}` and `{{SUPPORT_URL}}` unchanged. After you add a GitHub remote, check the selected repository with `tailor docket`.

With default modes, the next `tailor alter` repairs `SECURITY.md`, but preserves the existing first-fit `.github/ISSUE_TEMPLATE/config.yml`. Replace its support token manually with `https://github.com/<owner>/<repo>/blob/HEAD/SUPPORT.md`.

Alternatively, use `tailor alter --recut` to regenerate the issue configuration. This overwrites all eligible first-fit swatches, not just that file.

## Managed development files

Tailor manages Just and Nix loaders separately from ordinary swatches. The ordinary default set remains 29 entries. The `justfile` and `flake.nix` entries only control creation when those roots are missing.

Tailor preserves an existing `justfile` or `flake.nix` for every mode and when its swatch entry is omitted. Tailor reports that root as preserved and gives loader adoption guidance. A missing root with mode `never` or an omitted entry stays missing and produces no root guidance. Connect an existing root only when the exact loader line is absent:

```just
import 'just/loader.just'
```

Add this expression to the existing Nix package list:

```nix
++ import ./nix/loader.nix { inherit pkgs; }
```

The Nix loader adds package lists only. It can use packages from the existing `pkgs` set, but it does not add flake inputs or change outputs. Review and add new `.nix` files to Git, because Nix flakes exclude untracked files. Tailor does not inspect or stage the Git index.

Tailor always reconciles `just/loader.just`, `nix/loader.nix`, and `just/tailor.just`. The fixed loader does not read `.tailor.yml` when Just runs. The generated `build`, `test`, and `lint` dependencies come from the last successful Tailor reconciliation. Go, Pages, and the Playwright package fragment use their Boolean declarations:

| Declaration | Managed fragment action |
| --- | --- |
| `true` | Create the fragment or replace its owned content. |
| `false` | Remove only an owned fragment. |
| Absent | Do not inspect or change the fragment. |

Owned files start with `# Managed by Tailor: <registered path>`. Tailor stops on an unmarked file at a managed destination. A disabled fragment with a missing destination causes no change.

The generated root requires Just 1.23.0 or later and owns the sole `default` recipe. The core provides exactly three quality commands: `build`, `test`, and `lint`. It also keeps the operational `setup` and `release` commands. Call `tailor alter`, `tailor baste`, and `tailor measure` directly, without Just wrappers. Pages owns `pages`. `just/go.just` is an empty compatibility import with no recipes.

Typed registry metadata selects embedded shell bodies for explicitly enabled capabilities. The renderer places those bodies directly in the core in lexical capability order. Preflight checks each selected body in the core. False and absent Go declarations exclude Go bodies. Go is the only supported quality ecosystem. Empty build and test commands print guidance, then run an optional project script. Workflow lint runs once when a YAML workflow exists, otherwise it prints a skip message. Missing tools and command failures remain errors.

A `never` mode for `justfile` or `.golangci.yml` does not disable Go bodies selected by `languages.go: true`. Remove old quality recipes and aliases from preserved roots and project fragments before loader adoption. Move project-specific quality work into the optional scripts below. Retained marked Go fragments with legacy quality recipes require explicit Go enablement for migration, or manual removal of those recipes. Do not rename legacy helpers into another quality API. Release remains independent of language declarations and keeps its existing local tag flow.

If you manually delete a managed Just fragment, Just can fail while it parses every recipe. Run `tailor baste` directly. Review the full plan. After approval, run `tailor alter`. The alteration can change other declared files and repository settings.

Keep the capability declaration explicitly true so that Tailor recreates the fragment. If the declaration is absent, enable it or restore the deleted fragment from Git. An older retained `just/go.just` can still define `build` or `test`, which conflicts with the new core aggregate. Tailor stops before writes. Set `languages.go: true` to migrate the fragment, or remove or rename its legacy `build` or `test` recipe. Do not edit generated dependencies manually. Do not use `--recut` for this recovery.

### Nix shell cache

New `.envrc` starters watch `nix`, `nix/hooks`, and their `.nix` files before `use flake`. File watches detect edits. Directory watches detect added or removed fragments, including hook fragments. Missing directories are watched too, so their creation triggers a reload. Watches stay within these two directories, not the whole repository.

Existing `.envrc` files use `first-fit`, so normal alterations preserve them. Manually add this block before your existing `use flake` line:

```bash
watch_file nix nix/hooks
for file in nix/*.nix nix/hooks/*.nix; do
  if [ -e "$file" ]; then
    watch_file "$file"
  fi
done
```

Keep `nix_direnv_manual_reload` if you want manual rebuilds. Review the `.envrc` change, then run `direnv allow` when direnv requests approval. Run `nix-direnv-reload` to rebuild the cache, then return to the shell prompt. Manual mode can retain the old environment until that rebuild. A reload alone cannot repair missing watches in nix-direnv 3.1.2. Tailor does not grant approval or rebuild the cache automatically.

Custom imports outside these directories need their own watches before `use flake`. Review and add new Nix files to Git separately, because watches do not change flake source inclusion.

### Nix shell hooks

`nix/loader.nix` still returns a package list. The separate `nix/hooks.nix` returns a shell string from registered hooks only. Missing optional fragments are safe during partial upgrades. Tailor always owns `nix/hooks.nix` and `nix/hooks/core.nix`. Go selects `nix/hooks/go.nix`. The FFmpeg component selects `nix/hooks/go-ffmpeg-statigo.nix`.

New flake starters import both loaders. For a preserved root, manually compose the hook without replacing custom content:

```nix
shellHook = (import ./nix/hooks.nix { inherit pkgs; }) + "\n" + existingHook;
```

Replace `existingHook` with your existing hook expression, or omit the concatenation when no hook exists. Pass the same `pkgs` set, including any Go override, to both loaders. Tailor never rewrites existing target roots.

The helper functions run first, component hooks run next, and the core host hook runs last. On NixOS, `LD_LIBRARY_PATH` puts `/run/opengl-driver/lib` first, component libraries next, and existing entries last. The path helper removes duplicate and empty entries and tolerates repeated shell entry and unset variables.

Graphics hooks run only when runtime `/etc/os-release` contains `ID=nixos`. Tailor parses this file without sourcing it. Set `TAILOR_NIXOS_DRIVERS=0` to disable all graphics changes. Pure Nix evaluation cannot identify the host distribution, so package selection uses Linux rather than NixOS. Other Linux hosts receive the packages but no graphics environment changes. Darwin receives neither.

The FFmpeg component adds `vulkan-loader`, `intel-media-driver`, and `vpl-gpu-rt` on Linux. Its library order is oneVPL, Intel media, then Vulkan. It defaults `ONEVPL_SEARCH_PATH` to the oneVPL library directory. It defaults `LIBVA_DRIVERS_PATH` to system DRI when present, otherwise Intel DRI. It defaults `VK_DRIVER_FILES` to sorted, readable system ICD manifests, including files under linked driver directories. These defaults apply only to unset variables. Explicit empty values and user choices remain unchanged. Tailor neither detects GPU vendors nor forces `LIBVA_DRIVER_NAME`.

## Go support

Set `languages.go: true` to add Go development and release files. The Boolean form remains supported. The mapping form requires `enabled`:

```yaml
languages:
  go:
    enabled: true
    cgo: true
    ffmpeg-statigo: true
```

Both optional keys accept only Booleans. Unknown keys, null values, and a mapping without `enabled` are invalid. The component requires enabled Go and rejects explicit `cgo: false`. With the component enabled, omitted `cgo` resolves to true without changing authored YAML. Otherwise omitted `cgo` leaves the caller environment unchanged. The always-managed `just/tailor.just` exports the effective `CGO_ENABLED` value once: `"0"` for false or `"1"` for true. This export overrides the caller environment for all managed, imported, and root recipes and project scripts, including setup and source commands. No per-ecosystem export is needed. Disabled Go or absent policy emits no export. The Go shell hook applies the same policy to direct shell commands, independently of the graphics guard and opt-out. The rendered builder workflow also receives the policy. Regenerate managed files after a policy change. Reload the development shell for direct shell commands. Just needs no shell reload.

Component true creates owned Just, Nix package, and Nix hook fragments. Component false removes only owned fragments. An absent component preserves them, unless Go is explicitly false. Go false removes owned Go and component fragments. Go absent preserves all of them. Default merging preserves the authored Boolean or mapping form and does not insert optional keys.

The component is for consumer repositories with `third_party/ffmpeg-statigo`, not the ffmpeg-statigo source root. It never runs setup automatically. See [explicit setup effects](Commands#ffmpeg-statigo-setup). Release generation and source discovery still use `CGO_ENABLED=0`. This component does not provide cross-platform FFmpeg releases.

Minimal Boolean form:

```yaml
languages:
  go: true
```

Go is opt-in. New configs use `false`. Existing configs without this setting remain unchanged, including during default merging. Tailor accepts only the `go` key and Boolean values. Null values and unknown language keys are errors. CodeQL language settings do not enable these swatches.

| Swatch | Purpose | Default mode |
|---|---|---|
| `.golangci.yml` | Go lint configuration | `first-fit` |
| `.goreleaser.yaml` | Executable builds, archives, native packages, container images, and GitHub Releases | `first-fit` |
| `.github/workflows/build-go.yml` | Tests, lint checks, pull request snapshots, and releases from version tags | `first-fit` |
| `Dockerfile` | Non-root container image for each executable | `first-fit` |

The release configuration disables CGO and produces these outputs by default:

| Output | Platforms | Contents or destination |
|---|---|---|
| Archives | Linux and macOS, amd64 and arm64 | All discovered executables, attached to GitHub Releases with checksums |
| Native packages | Linux, amd64 and arm64 | `deb`, `rpm`, and `apk` packages with all discovered executables, attached to GitHub Releases |
| Container images | Linux, amd64 and arm64 | One image per executable, published to GHCR |

Pull requests and default-branch pushes produce downloadable archives, native packages, and checksums without publication. Snapshot jobs also build container images locally, but do not upload or push those images. Tags that match `v*.*.*` publish releases and versioned container images. Only stable releases update the container `latest` tag.

The release job uses the existing GitHub token with `packages: write`. No extra secret, Nix environment, GoReleaser Pro licence, or signing setup is required.

Native packages use `GITHUB_REPOSITORY_OWNER` as their default maintainer. Before publication, replace `nfpms[].maintainer` in `.goreleaser.yaml` with your project's maintainer name and email address.

A single executable uses `ghcr.io/<owner>/<repo>`, in lowercase. Tailor replaces each `_` and `.` in the repository name with a hyphen. Tailor adds `image` before a leading hyphen and after a trailing hyphen. For example, `Owner/my_app` becomes `ghcr.io/owner/my-app`, and `Owner/.app` becomes `ghcr.io/owner/image-app`.

Multiple executables append a hyphen and a normalised binary name. Tailor lowercases binary names, converts non-alphanumeric runs to hyphens, and trims leading and trailing hyphens. Numeric suffixes resolve collisions.

The Dockerfile uses the same digest-pinned Chainguard static base as Tailor's root Dockerfile. It runs as a non-root user. The `BINARY` build argument selects the executable, which the image installs at a fixed entrypoint.

The digest fixes the base image for reproducible builds. Tailor maintainers review newer upstream digests and update the root Dockerfile and swatch together when they accept a refresh. The generated Dependabot configuration does not update Docker images. For an existing project, review a newer upstream digest and update the `FROM` line manually. Normal `first-fit` alterations preserve your Dockerfile, even after a Tailor upgrade.

Use GoReleaser v2.18.0 or later for local snapshots. Local snapshots require Docker with a running daemon and Docker Buildx. Replace `owner/repo` and `owner` with your repository's values. Run this command from the project root:

```bash
GITHUB_REPOSITORY=owner/repo GITHUB_REPOSITORY_OWNER=owner goreleaser release --snapshot --clean
```

`--clean` removes the previous `dist` directory before the build. GitHub Actions supplies both environment variables automatically.

Tailor discovers executable packages from local source files in the root Go module when it needs a new release configuration. It supports one or several executables. Discovery skips symlinks, vendor directories, testdata, nested modules, and test files. It also skips files and directories with a dot or underscore prefix. It never runs project code or `go list`.

If Tailor finds no supported executable, or finds unsupported build constraints, the required release configuration fails preflight before writes. An existing first-fit `.goreleaser.yaml` or a `never` entry needs no discovery. For a library-only project, set `.goreleaser.yaml`, `.github/workflows/build-go.yml`, and `Dockerfile` to `never`. Keep `.golangci.yml` if needed.

Before Tailor creates or replaces the release configuration, `Dockerfile` must be a regular file, or Tailor must plan to create it. Tailor preserves an existing first-fit Dockerfile during normal alterations. A custom Dockerfile must accept the generated release configuration's `BINARY` argument and platform-specific binary layout.

Before Tailor creates or replaces the builder workflow, it checks the lint and release configurations. Both must already be regular files, or Tailor must plan to create them. Tailor does not check the contents of customised configurations.

When Go is true, Tailor places Go bodies in `just/tailor.just` and updates the empty `just/go.just` compatibility import. The managed build validates all packages before publication to `bin/`. Library-only modules succeed without `bin/`. Duplicate names, symlinked `bin`, and non-regular outputs fail. Existing regular executables can be replaced. The managed body writes no root binary, but a project script can add one. The Nix fragment retains the existing Go development tools.

### Shared Go policy

With Go enabled, a missing `.golangci.yml` receives the shared default as an ordinary first-fit swatch. Customise `.golangci.yml` directly. Normal alterations preserve an existing first-fit file. `always` and `--recut` replace eligible files with the shared default. `never` preserves the file. No ownership marker or `.tailor.yml` lint section is required.

All shared lint and format commands select `.golangci.yml` explicitly. Tailor does not migrate an alternate `.golangci.yaml` automatically. Back up any custom policy, then review and move it to `.golangci.yml` manually before using the commands.

The initial default uses Nyala's reviewed policy. It retains `G306` and three scoped gosec exclusions with their reason comments. It also retains the reviewed linter and formatter path exclusions. These security exceptions are shared default policy, not generated per-project decisions. Review their scope for your project.

| Command | Behaviour |
|---|---|
| `just build` | Validate Go packages and publish executable packages to `bin/` |
| `just test` | Run full `go vet ./...` once, then `go test -cover -covermode=atomic ./...` with console coverage |
| `just lint` | Read-only checks, identical to `just lint check` |
| `just lint correct` | Run `go mod tidy` and the configured formatter, then repeat all checks |

For Go, lint checks run sequentially: `go mod tidy -diff`, `golangci-lint fmt --config .golangci.yml --diff`, workflow lint, `golangci-lint run --config .golangci.yml`, `govulncheck ./...`, then diagnostic `gocyclo`. Each failure stops later commands. Lint does not run vet or tests. Test coverage stays in the console and creates no `coverage.out`. Lint rejects modes other than `check` and `correct` before any work.

Optional project-owned scripts at `just/project/build.sh`, `just/project/test.sh`, and `just/project/lint.sh` run with Bash after their managed commands succeed. Scripts run from the project root. The lint script receives `check` or `correct` as its first argument. Keep its check mode read-only. Missing scripts are skipped. Symlinks and non-regular scripts, or linked or non-directory parents, fail visibly. Tailor does not generate or overwrite these scripts. Do not expose alternative quality recipes through wrappers.

Go commands can download modules and write caches. Vulnerability checks can use the network. Setup and release remain separate operational commands. Benchmark and profile commands remain future work.

Every generated Dependabot file contains GitHub Actions and Nix entries. Go true adds `gomod`; Go false omits it. For a missing file, an absent Go setting selects the Go-enabled variant for legacy compatibility.

Dependabot uses [whole-file ownership](https://github.com/wimpysworld/tailor/blob/main/docs/design/dependabot-ownership.md). The exact first line is:

```text
# Managed by Tailor: .github/dependabot.yml
```

The marker must end with LF or CRLF. It gives Tailor consent to replace the complete file. Back up a custom file before adding the marker because reconciliation removes custom schedules, groups, registries, comments, commit-message settings, and other custom fields. Keep the file unmarked if any custom field must remain.

Both `first-fit` and `always` reconcile an owned file, including with `--recut`. All active modes preserve an unmarked regular file byte-for-byte. A Dependabot entry that is absent from the effective config, or set to `never`, skips `.github/dependabot.yml` and `.github/dependabot.yaml` without inspection. Default merging can restore an omitted Dependabot entry as `first-fit` when the `.tailor.yml` entry uses `always`, or uses `first-fit` with `--recut`. Set the Dependabot entry to `never` for a durable opt-out.

For an owned file, explicit true or false selects the complete current variant. With an absent Go setting, Tailor retains a state from its finite set of current and historical canonical bodies. An unknown or customised owned body needs an explicit Boolean or marker removal. Tailor recognises LF and whole-body CRLF canonical content, but always writes LF output.

Active management reports a conflict for `.github/dependabot.yaml`, unsafe parents or destination types, unreadable input, and input or output above 1 MiB. `tailor baste` previews these results without creating files or directories.

Existing first-fit ordinary swatches stay unchanged after you enable Go. `tailor alter --recut` replaces all eligible first-fit ordinary swatches, not only Go files. `never` always preserves an ordinary swatch.

Set Go to false to remove only the owned managed fragments and stop ordinary Go swatch processing. Remove the setting to leave managed fragments untouched. In both cases, Tailor preserves existing ordinary Go swatches, even with `--recut`. Existing builder workflows still run on GitHub.

## MCP support

Enable a headless, isolated Playwright MCP browser:

```yaml
mcp:
  playwright: true
```

Playwright is independent of Pages. You can enable Playwright when `pages` is absent or `pages.enabled` is false. The MCP server launches and owns its browser. The optional `just pages` command only starts a separate Pages preview server.

A true declaration adds `nix/playwright.nix`, which supplies `playwright-mcp` with Chromium from the existing Nix packages. Playwright is the only production MCP server. Tailor renders one file per client from its fixed server registry, and creates each file only when it is missing:

| Client | Starter file | Playwright entry |
|---|---|---|
| Claude | `.mcp.json` | `mcpServers.playwright` |
| Codex | `.codex/config.toml` | `mcp_servers.playwright` |
| OpenCode | `opencode.json` | `mcp.playwright` |
| Pi | `.pi/mcp.json` | `mcpServers.playwright` |

Each server has definitions for Claude, Codex, OpenCode, and Pi. Tailor combines selected definitions in lexical server order. The four embedded starters remain byte-parity references and manual-copy sources, not runtime composition sources. The current OpenCode starter format remains unchanged.

Tailor does not parse or merge an existing client file. It preserves regular files and final symlinks, then gives the entry name for manual adoption. Review the starter before you add it to Git.

All starters run `playwright-mcp` with `--headless --isolated`. They do not use a manual stdio process, CDP endpoint, dynamic port, runtime download, or Playwright Just fragment. Your client applies its normal MCP tool approval controls.

The Pi starter alone passes a non-empty `HTTPS_PROXY` value to `--proxy-server`. An empty or unset value disables this mapping. Do not put proxy credentials in command output or documentation. Tailor does not claim that the Claude, Codex, or OpenCode starters use this proxy setting.

Set `mcp.playwright: false` to remove only an owned `nix/playwright.nix`. Package removal uses the existing Nix fragment lifecycle. Tailor preserves all client files and warns that their server entries can refer to a missing `playwright-mcp` executable. Remove or disable those entries manually. An absent setting does not inspect or change any of the five paths. Tailor does not support shared-file adoption.

A [validated future contract](https://github.com/wimpysworld/tailor/blob/main/docs/design/mcp-adoption.md) defines explicit entry adoption. The proposed adoption and release flags are not implemented, so current create-missing-only behaviour remains unchanged.

Tailor accepts only the `playwright` key and a Boolean value. Null sections, null values, duplicate or unknown keys, strings, numbers, lists, and nested values are errors. An empty `mcp: {}` mapping keeps the section but leaves Playwright undeclared.

New configs set `languages.go`, `pages.enabled`, and `mcp.playwright` to false. Existing configs preserve an absent MCP section, an empty mapping, and explicit true or false values across default merging and later writes. Follow the [activation steps](Commands#activate-playwright-mcp) after you change the setting.

## Licences

Licences are not swatches. Choose an identifier supported by the [GitHub licences API](https://docs.github.com/en/rest/licenses/licenses#get-a-license), or `none` to skip licence creation.

`fit` records the identifier without checking it. `baste` does not fetch the licence body, so a successful preview does not confirm licence availability.

When `LICENSE` is absent, `alter` fetches `GET /licenses/{id}` and writes the returned text verbatim. Fill in copyright names, years, and other placeholders manually. Tailor does not substitute licence tokens.

An existing regular `LICENSE` remains unchanged, even after you change `license:` or use `--recut`. Edit or replace that file yourself when you change licences.

If the licence fetch fails, check the identifier and API access. Earlier changes remain. Correct `.tailor.yml`, then rerun `tailor baste` and `tailor alter`.

## Default swatch set

Tailor embeds 29 ordinary default swatches. Managed loaders and fragments are a separate class and do not increase this count:

| Swatch | Mode |
|--------|------|
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `always` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `always` |
| `.github/pull_request_template.md` | `never` |
| `SECURITY.md` | `always` |
| `CODE_OF_CONDUCT.md` | `always` |
| `CONTRIBUTING.md` | `always` |
| `SUPPORT.md` | `always` |
| `.github/dependabot.yml` | `first-fit` |
| `.github/FUNDING.yml` | `first-fit` |
| `.github/ISSUE_TEMPLATE/config.yml` | `first-fit` |
| `justfile` | `first-fit` |
| `.golangci.yml` | `first-fit` (only when `languages.go: true`) |
| `.goreleaser.yaml` | `first-fit` (only when `languages.go: true`) |
| `.github/workflows/build-go.yml` | `first-fit` (only when `languages.go: true`) |
| `Dockerfile` | `first-fit` (only when `languages.go: true`) |
| `flake.nix` | `first-fit` |
| `.gitignore` | `first-fit` |
| `.envrc` | `first-fit` |
| `cubic.yaml` | `first-fit` |
| `.tailor.yml` | `always` |
| `.github/workflows/tailor-pages.yml` | `always` (only when Pages is enabled) |
| `pages/index.html` | `first-fit` (static starter only) |
| `pages/style.css` | `first-fit` (static starter only) |
| `pages/theme.js` | `first-fit` (static starter only) |
| `pages/icon.svg` | `first-fit` (static starter only) |
| `wiki/Home.md` | `first-fit` (only when `repository.has_wiki: true`) |
| `wiki/_Sidebar.md` | `first-fit` (only when `repository.has_wiki: true`) |
| `wiki/_Footer.md` | `first-fit` (only when `repository.has_wiki: true`) |
| `.github/workflows/tailor-wiki.yml` | `always` (only when `repository.has_wiki: true`) |

## Alteration modes

- **`always`** - Overwrites ordinary swatches when their embedded content differs from the file on disk. Local edits are not preserved.
- **`first-fit`** - Copies a missing file and preserves an existing file, unless `--recut` applies. Use this for files that you customise.
- **`never`** - Skips the file entirely. Use this to keep a swatch visible in the config without managing its destination.

`.gitignore` is an exception during ordinary swatch processing. Tailor preserves an existing regular file or final symlink byte-for-byte under `always`, `first-fit`, and `--recut`. It does not read or hash the file, or follow the symlink. If the configured root is missing, `always` and `first-fit` create it atomically without replacing a destination that appears during the write. A directory or special file stops the command before repository checks, authentication, or writes. `never` skips inspection and writing.

Enabled Hugo or Jekyll Pages uses a later additive exception. Tailor can replace a final `.gitignore` symlink without following its target, then appends the generated output rule. Existing regular-file text stays unchanged. If the same rule already exists before a later negation, Tailor does not move it, so review the pattern order manually.

### Future manual adoption

This section describes a future contract, not current Tailor behaviour. Future versions will update only adopted `base`, `go`, and `pages` sections. Existing unmarked roots will remain byte-for-byte unchanged and receive snippets for manual adoption.

Add exact marker pairs where Tailor can own rules:

```gitignore
# tailor:ignore:base:start
# Tailor base rules will go here.
# tailor:ignore:base:end

# Project-owned rules remain editable here.
!important.log
logs/
!logs/
!logs/keep.log
```

A later project rule can override an earlier managed rule. To re-include `logs/keep.log`, re-include its excluded `logs/` ancestor first. Tailor will preserve duplicate and historical project rules, so an old rule can keep a path ignored.

| Declaration | Adopted section action |
| --- | --- |
| `languages.go: true` | Replace the `go` body with only `*.test`. |
| `languages.go: false` | Empty the `go` body but keep its markers. |
| `languages.go` absent | Preserve the `go` body. |
| Enabled Hugo or Jekyll | Replace an adopted `pages` body. |
| Pages false, absent, static, unavailable, or skipped | Preserve the `pages` body. |

The `.gitignore` mode `never` will skip all inspection and writes. Active modes and `--recut` will not grant ownership. An omitted swatch entry will prevent missing-root creation but will not revoke existing marker consent. Default merging can restore an omitted entry, so use `never` for a durable opt-out.

Tailor will create sections in `base`, `go`, `pages` order only for a permitted missing root. It will not insert a missing section into an existing root. See the [approved design and recovery guide](https://github.com/wimpysworld/tailor/blob/main/docs/design/ignore-sections.md).

Config merging differs from ordinary file replacement. Protected `justfile`, `flake.nix`, and `.gitignore` roots, existing wiki pages, and static Pages starter files also have [recut exceptions](Commands#alter).

The [Pages workflow](GitHub-Pages#workflow) and [wiki workflow](GitHub-wiki#workflow) have their own compatibility and mode checks.

## Default merging

Tailor merges missing defaults into `.tailor.yml` when its swatch entry uses `always`, or `first-fit` with `--recut`. `baste` previews the normal merge without writes.

The merge appends missing swatch entries and fills missing supported settings. It preserves existing swatch modes and explicit settings, including `false`, empty strings, and empty lists. Labels have a separate empty-list rule below.

| Config content | Merge behaviour |
|---|---|
| `repository` | Adds missing defaults except project-specific `description`, `homepage`, and `topics`. |
| `actions` | Adds missing defaults, subject to the [selected-actions policy](GitHub-Actions#policy-changes). |
| `code_scanning` | Restores an absent section with `state: configured` and fills missing fields. |
| `code_quality` | Restores an absent section with `state: not-configured` and fills missing fields. |
| `ruleset` | Restores an absent default section with `enforcement: active` and fills missing fields at each level. Explicit lists remain whole. |
| `labels` | Restores default labels when absent or empty. A non-empty list remains unchanged. |
| `pages` | Preserves an absent section and an absent `enabled` key. In an existing mapping, adds missing `generator` and `path`, but not `enabled` or personal `links`. |
| `languages` | Preserves absent and explicit settings. Does not add a missing section or `go` key. |
| `mcp` | Preserves an absent section, an empty mapping, and an absent or explicit `playwright` value. |
| `license`, `immutable_releases`, `variables` | Does not add defaults. |

Omitting a section can stop management only when its merge rule preserves absence. Set `.tailor.yml` to `never`, or omit its swatch entry, to disable all default merging. The merge never restores its own `.tailor.yml` entry.

[Retired-entry cleanup](Commands#retired-workflow-cleanup) and [security prerequisite normalisation](Repository-settings#repository-fields) still apply when default merging is disabled. Security normalisation can change explicit values and reports a warning.

## Config file

All state lives in `.tailor.yml`. Its thirteen sections are `license`, `repository`, `immutable_releases`, `actions`, `code_scanning`, `code_quality`, `ruleset`, `labels`, `variables`, `pages`, `languages`, `mcp`, and `swatches`.

Tailor opens `.tailor.yml` relative to the project root. The config must be a regular file no larger than 1 MiB.

This abbreviated example uses custom topics and two swatches, and omits the default labels.

```yaml
# Initially fitted by tailor on 2026-03-04
license: BlueOak-1.0.0

languages:
  go: false

mcp:
  playwright: false

repository:
  topics:
    - automation
    - developer-tools
    - golang
  has_wiki: false
  has_discussions: false
  allow_squash_merge: true
  delete_branch_on_merge: true
  allow_auto_merge: true
  private_vulnerability_reporting_enabled: true
  vulnerability_alerts_enabled: true
  automated_security_fixes_enabled: true
  default_workflow_permissions: read
  can_approve_pull_request_reviews: false
  secret_scanning: enabled
  secret_scanning_push_protection: enabled
  secret_scanning_non_provider_patterns: enabled

immutable_releases:
  enabled: false

actions:
  enabled: true
  allowed_actions: all
  sha_pinning_required: false
  fork_pr_contributor_approval:
    approval_policy: first_time_contributors

code_scanning:
  state: configured
  query_suite: default
  threat_model: remote
  # An empty list means GitHub detects the languages. Valid values:
  # actions, c-cpp, csharp, go, java-kotlin, javascript-typescript, python, ruby, swift
  languages: []

code_quality:
  state: not-configured
  # An empty list means GitHub detects the languages. Valid values:
  # csharp, go, java-kotlin, javascript-typescript, python, ruby
  languages: []

ruleset:
  # Tailor manages one ruleset named "Tailor" and owns it entirely.
  # active enforces the rules. disabled keeps the ruleset on GitHub but
  # GitHub ignores it, so a hand-made ruleset can govern instead.
  enforcement: active
  bypass_actors:
    # actor_type: RepositoryRole, Team, User, Integration, DeployKey
    # RepositoryRole actor_id: 2 maintain, 4 write, 5 admin
    # bypass_mode: always, pull_request, exempt
    - actor_id: 5
      actor_type: RepositoryRole
      bypass_mode: always
  conditions:
    ref_name:
      # Branch names or fnmatch patterns in refs/heads/<name> form.
      # include also accepts ~DEFAULT_BRANCH and ~ALL.
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    creation: false
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: true
      parameters:
        required_approving_review_count: 1
        dismiss_stale_reviews_on_push: true
        require_code_owner_review: false
        require_last_push_approval: false
        required_review_thread_resolution: true
        require_extra_approval_for_unattributed_changes: true
        # Any combination of merge, squash, rebase. At least one.
        allowed_merge_methods:
          - squash
          - rebase
    required_status_checks:
      enabled: false
      parameters:
        # Require branches to be up to date before merging.
        strict_required_status_checks_policy: false
        # Do not require status checks on creation.
        do_not_enforce_on_create: false
        # context is the check name as shown on a pull request. For a GitHub
        # Actions job that is the job's name. integration_id is optional and
        # restricts the check to one app; 15368 is GitHub Actions.
        required_status_checks: []
    code_scanning:
      enabled: false
      parameters:
        # tool is the tool name as GitHub shows it, for example CodeQL.
        # alerts_threshold: none, errors, errors_and_warnings, all
        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all
        code_scanning_tools:
          - tool: CodeQL
            alerts_threshold: errors
            security_alerts_threshold: high_or_higher

# Repository Actions variables are non-secret values.
# Tailor creates or updates only declared variables and leaves others unchanged.
# variables:
#   - name: DEPLOY_REGION
#     value: "eu-west-2"
#   - name: RELEASE_SUFFIX
#     value: ""

swatches:
  - path: SECURITY.md
    alteration: always

  - path: justfile
    alteration: first-fit
```

Each swatch entry has two fields:

| Field | Description |
|-------|-------------|
| `path` | Registered path from the default swatch set, relative to the project root. Not a custom destination mapping. |
| `alteration` | `always`, `first-fit`, or `never` |

Set `alteration: never` to stop tailor managing a file. The entry stays visible in `.tailor.yml` and prevents `alter --recut` from re-adding it.

Every swatch path must be unique, including entries with mode `never`. Tailor rejects unknown paths and duplicate entries before writes, after it removes retired entries.

## Configuration and destination errors

For malformed YAML or unsupported fields, correct the reported entry in `.tailor.yml`. Keep one entry per registered swatch path, then rerun `tailor baste`.

For ordinary swatches that Tailor manages, parent paths must be real directories, not symlinks. Destinations cannot be directories or other non-regular files. Tailor replaces a destination symlink without following it, even in `first-fit` mode. The protected ordinary `.gitignore` root is the exception described above.

If a destination is unsafe, move valuable content aside before you correct the path. Use `never` for a file that Tailor must leave alone. Pages and wiki files have additional checks in their setting references below.

## Setting references

| Section | Reference |
|---|---|
| `repository`, `immutable_releases`, `labels` | [Repository settings](Repository-settings) |
| `actions`, `variables` | [GitHub Actions](GitHub-Actions) |
| `code_scanning`, `code_quality` | [Code scanning and quality](Code-scanning-and-quality) |
| `ruleset` | [Ruleset](Ruleset) |
| `pages` | [GitHub Pages](GitHub-Pages) |
| `languages` | [Go support](#go-support) |
| `mcp` | [MCP support](#mcp-support) |
| Wiki files and `repository.has_wiki` | [GitHub wiki](GitHub-wiki) |

See [Commands](Commands) for licence flags, file checks, and the shipped [justfile recipes](Commands#justfile-recipes).
