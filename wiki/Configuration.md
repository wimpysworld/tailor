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

Tailor always reconciles `just/loader.just`, `nix/loader.nix`, and `just/tailor.just`. Go, Pages, and the Playwright package fragment use their Boolean declarations:

| Declaration | Managed fragment action |
| --- | --- |
| `true` | Create the fragment or replace its owned content. |
| `false` | Remove only an owned fragment. |
| Absent | Do not inspect or change the fragment. |

Owned files start with `# Managed by Tailor: <registered path>`. Tailor stops on an unmarked file at a managed destination. A disabled fragment with a missing destination causes no change.

The generated root requires Just 1.23.0 or later. Recipe names are unique: core owns `alter`, `measure`, `release`, and `lint`; Go owns `build`, `test`, and `lint-go`; Pages owns `pages`.

## Go support

Set `languages.go: true` to add Go development and release files:

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

When Go is true, Tailor creates or updates `just/go.just` and `nix/go.nix`. The Just fragment adds `build`, `test`, and `lint-go`. The Nix fragment adds Go, golangci-lint, and GoReleaser from the existing `pkgs` set.

Newly rendered Dependabot configuration includes `gomod` when Go is true and omits it when Go is false. An absent Go setting keeps the legacy `gomod` entry. GitHub Actions and Nix entries remain.

Existing first-fit ordinary swatches stay unchanged after you enable Go. `tailor alter --recut` replaces all eligible first-fit ordinary swatches, not only Go files. `never` always preserves an ordinary swatch.

Set Go to false to remove only the owned managed fragments and stop ordinary Go swatch processing. Remove the setting to leave managed fragments untouched. In both cases, Tailor preserves existing ordinary Go swatches, even with `--recut`. Existing builder workflows still run on GitHub.

## MCP support

Enable a headless, isolated Playwright MCP browser:

```yaml
mcp:
  playwright: true
```

Playwright is independent of Pages. You can enable Playwright when `pages` is absent or `pages.enabled` is false. The MCP server launches and owns its browser. The optional `just pages` command only starts a separate Pages preview server.

A true declaration adds `nix/playwright.nix`, which supplies `playwright-mcp` with Chromium from the existing Nix packages. It also creates these client files only when each file is missing:

| Client | Starter file | Playwright entry |
|---|---|---|
| Claude | `.mcp.json` | `mcpServers.playwright` |
| Codex | `.codex/config.toml` | `mcp_servers.playwright` |
| OpenCode | `opencode.json` | `mcp.playwright` |
| Pi | `.pi/mcp.json` | `mcpServers.playwright` |

Tailor does not parse or merge an existing client file. It preserves regular files and final symlinks, then gives the entry name for manual adoption. Review the starter before you add it to Git.

All starters run `playwright-mcp` with `--headless --isolated`. They do not use a manual stdio process, CDP endpoint, dynamic port, runtime download, or Playwright Just fragment. Your client applies its normal MCP tool approval controls.

The Pi starter alone passes a non-empty `HTTPS_PROXY` value to `--proxy-server`. An empty or unset value disables this mapping. Do not put proxy credentials in command output or documentation. Tailor does not claim that the Claude, Codex, or OpenCode starters use this proxy setting.

Set `mcp.playwright: false` to remove only an owned `nix/playwright.nix`. Tailor preserves all client files and warns that their server entries can refer to a missing `playwright-mcp` executable. Remove or disable those entries manually. An absent setting does not inspect or change any of the five paths.

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

Config merging differs from ordinary file replacement. Protected `justfile` and `flake.nix` roots, existing wiki pages, and static Pages starter files also have [recut exceptions](Commands#alter).

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

For ordinary swatches that Tailor manages, parent paths must be real directories, not symlinks. Destinations cannot be directories or other non-regular files. Tailor replaces a destination symlink without following it, even in `first-fit` mode.

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
