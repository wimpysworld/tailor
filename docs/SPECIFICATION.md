# Tailor Specification v0.3

## Overview

Tailor is a Go CLI tool for managing project templates across GitHub repositories. It provides bespoke fitting for new projects and alterations for existing projects. Running `tailor` with no arguments displays help.

## Prerequisites

Tailor requires a valid GitHub authentication token. This can be provided in two ways:

1. **Environment variable**: Set `GH_TOKEN` or `GITHUB_TOKEN`. This is the recommended approach for CI environments and works without the `gh` binary installed.
2. **GitHub CLI**: Install and authenticate the [GitHub CLI](https://cli.github.com/) (`gh`). Run `gh auth login` to authenticate. The `gh` binary is also used as a fallback for keyring-based token access when no environment variable is set.

The `fit`, `alter`, and `baste` commands verify that a valid authentication token exists at startup and exit with an error if no token is available.

`measure` and `docket` are exempt from the authentication requirement. `measure` performs purely local file inspection and needs no network access or authentication. `docket` can report unauthenticated state without erroring - it displays the auth state rather than requiring it.

## Intended Workflow

### New project

`fit` creates the project directory and writes `.tailor/config.yml` with the full default swatch set in one command, with a `license: MIT` default. Use `--license=<id>` to select a different licence or `--license=none` to opt out. Change into `<path>`, then run `alter` to copy the swatch files, including the `.github/workflows/tailor.yml` workflow that handles weekly automated maintenance. The action opens a pull request whenever swatch content changes, keeping files current without manual intervention.

### Existing project

`measure` checks which community health files are present or missing - run it first to see what a project needs. If no `.tailor/config.yml` exists, run `tailor fit .` to create one (the directory already exists, so `fit` proceeds without error), or create `.tailor/config.yml` manually. Edit `.tailor/config.yml` directly to add or remove swatches or change alteration modes, then run `alter` to bring the project into sync with the current swatches; the `.github/workflows/tailor.yml` swatch handles ongoing maintenance once placed, opening pull requests whenever upstream swatch content changes.

## Core Concepts

**Swatches**: Complete, ready-to-use template files stored in `swatches/`. Files are copied verbatim, with four exceptions: `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically; `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically; `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically; `.tailor/config.yml` has `{{HOMEPAGE_URL}}` substituted automatically.

**Swatch names**: Swatch references use the full source path relative to `swatches/`, including the file extension where one exists. Extensionless files are referenced as-is. For example, `swatches/.github/workflows/tailor.yml` is referenced as `.github/workflows/tailor.yml`; `swatches/SECURITY.md` as `SECURITY.md`; `swatches/justfile` as `justfile` (no extension).

**Swatch Mapping**: Each swatch has a defined source-to-destination mapping:

| Source | Destination |
|---|---|
| `.gitignore` | `.gitignore` |
| `.envrc` | `.envrc` |
| `SECURITY.md` | `SECURITY.md` |
| `CODE_OF_CONDUCT.md` | `CODE_OF_CONDUCT.md` |
| `CONTRIBUTING.md` | `CONTRIBUTING.md` |
| `SUPPORT.md` | `SUPPORT.md` |
| `flake.nix` | `flake.nix` |
| `justfile` | `justfile` |
| `.github/FUNDING.yml` | `.github/FUNDING.yml` |
| `.github/dependabot.yml` | `.github/dependabot.yml` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `.github/ISSUE_TEMPLATE/bug_report.yml` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `.github/ISSUE_TEMPLATE/feature_request.yml` |
| `.github/ISSUE_TEMPLATE/config.yml` | `.github/ISSUE_TEMPLATE/config.yml` |
| `.github/pull_request_template.md` | `.github/pull_request_template.md` |
| `.github/workflows/tailor.yml` | `.github/workflows/tailor.yml` |
| `.github/workflows/tailor-automerge.yml` | `.github/workflows/tailor-automerge.yml` |
| `.tailor/config.yml` | `.tailor/config.yml` |

Swatch-to-path mappings are hardcoded in the source. Licences are not swatches - they are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

**Repository Settings**: Tailor can manage GitHub repository settings declaratively via the `repository` section in `config.yml`. Field names match the GitHub REST API field names exactly (snake_case). Settings are applied via `PATCH /repos/{owner}/{repo}` as a single API call. Repository settings are always applied idempotently on every `alter` run - there is no `first-fit` concept for API settings. If the `repository` section is absent from `config.yml`, repository settings are skipped entirely.

Supported repository settings:

| Field | Type | Description |
|---|---|---|
| `description` | string | Repository description |
| `homepage` | string | Repository homepage URL |
| `has_wiki` | bool | Enable wiki |
| `has_discussions` | bool | Enable discussions |
| `has_projects` | bool | Enable projects |
| `has_issues` | bool | Enable issues |
| `allow_merge_commit` | bool | Allow merge commits |
| `allow_squash_merge` | bool | Allow squash merging |
| `allow_rebase_merge` | bool | Allow rebase merging |
| `squash_merge_commit_title` | string | Squash merge commit title (`PR_TITLE`, `COMMIT_OR_PR_TITLE`) |
| `squash_merge_commit_message` | string | Squash merge commit message (`PR_BODY`, `COMMIT_MESSAGES`, `BLANK`) |
| `merge_commit_title` | string | Merge commit title (`PR_TITLE`, `MERGE_MESSAGE`) |
| `merge_commit_message` | string | Merge commit message (`PR_TITLE`, `PR_BODY`, `BLANK`) - values match the GitHub REST API |
| `delete_branch_on_merge` | bool | Delete branch on merge |
| `allow_update_branch` | bool | Allow updating PR branches |
| `allow_auto_merge` | bool | Allow auto-merge |
| `web_commit_signoff_required` | bool | Require sign-off on web commits |
| `private_vulnerability_reporting_enabled` | bool | Allow users to privately report potential security vulnerabilities |

`private_vulnerability_reporting_enabled` uses a separate API endpoint (`PUT`/`DELETE /repos/{owner}/{repo}/private-vulnerability-reporting`) rather than the repository PATCH call. Tailor handles this transparently - it appears in `config.yml` alongside other repository settings but is applied via its own API call.

Settings deliberately excluded due to risk or org-level scope: `visibility`, `default_branch`, `topics`, `template`, `allow_forking`, `enable_advanced_security`, `enable_secret_scanning`, `enable_secret_scanning_push_protection`.

**Alteration Modes**:
- `always`: Tailor compares the embedded swatch content against the on-disk file on every `alter` run and overwrites if they differ
- `first-fit`: Tailor copies this file only if it does not already exist; never overwrites
- `triggered`: Tailor deploys this swatch only when a trigger condition elsewhere in the config is met. When the condition is met, behaves like `always` (overwrite when changed). When the condition is not met and the file exists on disk, Tailor removes it. Each triggered swatch has a trigger condition defined in a lookup table in the swatch package, mapping source path to a config field and expected value. Triggered swatches appear explicitly in `config.yml` like any other swatch
- `never`: Tailor skips this swatch entirely - no deployment, no comparison, no removal. Used to suppress a swatch (including a triggered swatch whose condition is met) while keeping it visible in the config. `never` takes precedence over `triggered`

**Default Alteration Modes**:

| Swatch | Default mode |
|---|---|
| `.gitignore` | `first-fit` |
| `.envrc` | `first-fit` |
| `SECURITY.md` | `always` |
| `CODE_OF_CONDUCT.md` | `always` |
| `CONTRIBUTING.md` | `always` |
| `SUPPORT.md` | `always` |
| `.github/FUNDING.yml` | `first-fit` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `always` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `always` |
| `.github/ISSUE_TEMPLATE/config.yml` | `first-fit` |
| `.github/pull_request_template.md` | `always` |
| `.github/workflows/tailor.yml` | `always` |
| `.github/workflows/tailor-automerge.yml` | `triggered` |
| `.github/dependabot.yml` | `first-fit` |
| `justfile` | `first-fit` |
| `flake.nix` | `first-fit` |
| `.tailor/config.yml` | `first-fit` |

**Swatch Categories**: Each swatch is designated either `health` or `development`. This designation is an internal attribute used by `measure` to scope its file presence checks.

**Health swatches** (community health files tracked by GitHub):
- `LICENSE` (fetched via `gh`, not an embedded swatch)
- `SECURITY.md`
- `CODE_OF_CONDUCT.md`
- `CONTRIBUTING.md`
- `SUPPORT.md`
- `.github/FUNDING.yml`
- `.github/ISSUE_TEMPLATE/bug_report.yml`
- `.github/ISSUE_TEMPLATE/feature_request.yml`
- `.github/ISSUE_TEMPLATE/config.yml`
- `.github/pull_request_template.md`
- `.github/dependabot.yml`

**Development swatches** (dev environment and project tooling):
- `.gitignore`
- `.envrc`
- `flake.nix`
- `justfile`
- `.github/workflows/tailor.yml`
- `.github/workflows/tailor-automerge.yml`
- `.tailor/config.yml`

## Commands

Commands divide into three categories: bootstrap commands, which create the project and initial configuration; apply commands, which read `config.yml` and modify project files; and inspection commands, which check the project without modifying anything.

**Bootstrap commands**: `fit`
**Apply commands**: `alter`
**Inspection commands**: `baste`, `measure`, `docket`

### `fit <path>`

Creates a new project directory and writes `.tailor/config.yml` with the full default swatch set and the repository settings. When run against an existing project with a GitHub remote, `fit` queries the live repository configuration and uses those values for the `repository` section, preserving the project's current state. When no repository context exists, the built-in defaults are used. Does not copy any files or apply any settings. After `fit`, change into `<path>` before running `alter`.

The default swatch set embedded in the binary is:

- `.github/workflows/tailor.yml`
- `.github/workflows/tailor-automerge.yml`
- `.github/dependabot.yml`
- `.github/FUNDING.yml`
- `.github/ISSUE_TEMPLATE/bug_report.yml`
- `.github/ISSUE_TEMPLATE/feature_request.yml`
- `.github/ISSUE_TEMPLATE/config.yml`
- `.github/pull_request_template.md`
- `SECURITY.md`
- `CODE_OF_CONDUCT.md`
- `CONTRIBUTING.md`
- `SUPPORT.md`
- `justfile`
- `flake.nix`
- `.gitignore`
- `.envrc`
- `.tailor/config.yml`

A `license` key is included in `config.yml` by default (`license: MIT`). Use `--license=<id>` to select a different licence or `--license=none` to opt out entirely.

`--license=<id>` records the licence identifier in `config.yml`. Defaults to `MIT` if not specified. `--license=none` records `license: none`, opting out of licence creation. The identifier is used to fetch licence text via the GitHub REST API (`GET /licenses/{id}`) at `alter` time; any licence supported by the GitHub API is valid. `fit` does not validate the identifier - validation is deferred to `alter`.

`--description=<text>` sets the `description` field in the `repository` section of `config.yml`, overriding any value from GitHub. `fit` does not apply the description - it is applied at `alter` time.

**Repository settings resolution at `fit` time**: `fit` detects repository context by querying GitHub remotes in `<path>`. If a GitHub remote exists, the project has repository context. If no remote is found, no repository context exists. Repository context detection reads git remotes (via `go-gh`), so `git` must be present when a GitHub remote exists - which is always the case in practice, since the remote implies a git repository.

When repository context exists, `fit` queries the live repository configuration via `GET /repos/{owner}/{repo}` and `GET /repos/{owner}/{repo}/private-vulnerability-reporting` to populate the `repository` section with the project's current settings. This ensures that enabling tailor on an existing project does not inadvertently change features that are already configured (e.g. disabling wiki or discussions that are currently enabled). The `--description` flag takes precedence over the value from GitHub. `description` and `homepage` are omitted if empty. When no repository context exists (e.g. a brand-new project with no remote), the built-in defaults from the embedded swatch are used, with `description` and `homepage` normalised to nil by `DefaultConfig` so they are omitted from the generated config.

```bash
# Default licence (MIT)
tailor fit ./my-project

# Explicit licence selection
tailor fit ./my-project --license=Apache-2.0

# Opt out of licence entirely
tailor fit ./my-project --license=none

# Set description (overrides any value from GitHub)
tailor fit ./my-project --description="My awesome project"
```

If `<path>` already exists but does not contain `.tailor/config.yml`, `fit` proceeds without error and creates the configuration. If `<path>` already exists and contains `.tailor/config.yml`, `fit` exits with an error: "`.tailor/config.yml` already exists at `<path>`. Edit `.tailor/config.yml` directly to change the swatch configuration." `fit` creates all intermediate directories in `<path>` as needed.

Generates:
- Project directory at `<path>`
- `.tailor/config.yml` at `<path>/.tailor/config.yml`, creating the `.tailor/` directory if it does not already exist, containing the `license` key, the `repository` section (populated from live GitHub settings when available, otherwise from built-in defaults), and the full default swatch set, each entry at its default alteration mode, prefixed with a `# Initially fitted by tailor on <DATE>` header comment (YYYY-MM-DD, no time).

### `alter`

Applies swatch alterations to the local project.

`alter` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor/config.yml` in the current working directory. No upward traversal is performed.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Apply and overwrite regardless of mode or existence
```

Behaviour:
- If `.tailor/config.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- For repository settings: if a `repository` section is present in `config.yml`, reads the current repository settings via `GET /repos/{owner}/{repo}`, compares each declared field against the live value, and sends a single `PATCH /repos/{owner}/{repo}` call with all declared fields. Repository settings are applied before licences and swatches. If no GitHub repository context exists (no remote), repository settings are skipped with a warning. `--recut` has no special effect on repository settings - they are always applied declaratively.
- For `always` swatches: compares the SHA-256 of the embedded swatch content against the on-disk file; overwrites if they differ. SHA-256 comparison applies only to `always` swatches. For any `always` swatch whose embedded content contains substitution tokens (`{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, or `{{HOMEPAGE_URL}}`), the SHA-256 comparison is skipped and the swatch is always overwritten on `alter`. This is because the on-disk file contains the resolved value while the embedded template contains the raw token, so they will always differ. The set of substituted swatches is determined by an explicit list in code: `.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, and `.tailor/config.yml`. If a user changes one of these swatches from `first-fit` to `always` in their config, the same skip-and-overwrite rule applies.
- For `first-fit` swatches: copies only if the destination file does not exist; never overwrites. If the destination exists, the swatch is skipped entirely - no SHA-256 comparison is performed.
- For `triggered` swatches: looks up the trigger condition for the swatch source in the trigger condition table. If the condition is met (e.g. `allow_auto_merge: true` in the `repository` section), behaves like `always` - deploys and overwrites when content differs. If the condition is not met and the file exists on disk, removes it. If the condition is not met and the file does not exist, skips silently. Triggered swatches are never overwritten by `--recut` when the trigger condition is false.
- For `never` swatches: skips entirely. No file is written, compared, or removed. This mode suppresses any swatch, including triggered swatches whose condition would otherwise be met.
- For licences: if `config.yml` contains a `license` key with a value other than `none`, and no `LICENSE` file exists on disk, fetches the licence text via the GitHub REST API (`GET /licenses/{id}`) and writes it to `LICENSE`. The text is written verbatim as returned by GitHub - no token substitution is performed. Always treated as `first-fit`; the on-disk `LICENSE` file is never overwritten. If the licence fetch fails (e.g. unrecognised licence identifier), `alter` exits with the API error.
- For `.github/FUNDING.yml`: substitutes `{{GITHUB_USERNAME}}` before writing. `{{GITHUB_USERNAME}}` is resolved at `alter` time from `GET /user`.
- For `SECURITY.md`: substitutes `{{ADVISORY_URL}}` before writing. `{{ADVISORY_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/security/advisories/new` from the repository context (owner/name). If no GitHub repository context exists (e.g. a brand-new project with no remote), `{{ADVISORY_URL}}` is left unsubstituted in the written file. The unsubstituted token is intentionally detectable by a future `measure` run; `alter` will resolve and substitute it on a subsequent run once the repository has a remote.
- For `.github/ISSUE_TEMPLATE/config.yml`: substitutes `{{SUPPORT_URL}}` before writing. `{{SUPPORT_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md` from the repository context (owner/name). If no GitHub repository context exists, `{{SUPPORT_URL}}` is left unsubstituted in the written file.
- For `.tailor/config.yml`: substitutes `{{HOMEPAGE_URL}}` before writing. `{{HOMEPAGE_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>` from the repository context (owner/name). If no GitHub repository context exists, `{{HOMEPAGE_URL}}` is left unsubstituted in the written file.
- With `--recut`: overwrites regardless of mode or existence, including `first-fit` swatches - `--recut` will overwrite a `first-fit` swatch file even if it exists and has been locally modified. Use with care. The licence file and `.tailor/config.yml` are exempt from `--recut` and are never overwritten regardless - the licence because it is fetched content not an embedded swatch, and `.tailor/config.yml` because overwriting it would destroy the project's configuration. When `--recut` writes a substituted swatch (e.g. `.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor/config.yml`), the full token resolution pipeline runs and fresh values are substituted before writing.
- If no `license` key is present in `config.yml` (or its value is `none`) and no `LICENSE` file exists in the project root, emits a warning: "No licence file found and no licence configured. Add `license: MIT` (or another identifier) to `.tailor/config.yml` and run `tailor alter`." Warning only; does not block execution.
- Creates intermediate directories as needed before writing any swatch whose destination path requires directories that do not yet exist.
- Never touches files not listed in `config.yml`
- Modifies files only; does not commit or push

### `baste`

Previews what `alter` would do without making any changes.

`baste` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor/config.yml` in the current working directory. No upward traversal is performed.

```bash
tailor baste
```

Behaviour:
- If `.tailor/config.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- `baste` performs the same comparison logic as `alter` but writes nothing. It reports what `alter` would do.

Output format - repository settings are shown first (if a `repository` section is present), followed by swatch entries.

Repository settings output uses two categories:

```
would set:                   repository.has_wiki = false
would set:                   repository.delete_branch_on_merge = true
no change:                   repository.allow_squash_merge (already true)
```

`would set` - declared value differs from the live repository setting.
`no change` - declared value matches the live repository setting.

Repository settings entries are sorted lexicographically by field name within each category, actionable (`would set`) before informational (`no change`).

Swatch output uses the following categories:

```
would copy:                                LICENSE
would overwrite:                           SECURITY.md
would deploy (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml
would remove (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml
no change:                                 .github/workflows/tailor.yml
skipped (first-fit, exists):               justfile
skip (never):                              .github/workflows/tailor-automerge.yml
```

`would copy` - destination does not exist and the swatch would be written. Applies regardless of whether the swatch is `always` or `first-fit`.
`would overwrite` - `always` swatch whose embedded content differs from the on-disk file.
`would deploy (triggered: <field>)` - triggered swatch whose condition is met; the annotation shows which config field activated it. Covers both copy (file absent) and overwrite (file exists, content differs) cases.
`would remove (triggered: <field>)` - triggered swatch whose condition is not met and the file exists on disk.
`no change` - `always` or `triggered` swatch whose embedded content matches the on-disk file. `no change` only appears for `always` and active `triggered` swatches; `first-fit` swatches that exist always produce `skipped (first-fit, exists)`, never `no change`. `always` swatches containing substitution tokens always produce `would overwrite`, never `no change` (see the substituted-swatch rule in the `alter` behaviour section above).
`skipped (first-fit, exists)` - `first-fit` swatch whose destination already exists; no comparison is performed.
`skip (never)` - swatch with `alteration: never`; skipped unconditionally.

Output order: actionable items first (`would set`, `would copy`, `would overwrite`, `would deploy`, `would remove`), then informational (`no change`, `skipped (first-fit, exists)`, `skip (never)`). Within each category, entries are sorted lexicographically by path or field name. The category label width is computed dynamically from the longest label for consistent column alignment.

### `measure`

Assesses a project's community health files and, when `.tailor/config.yml` is present, checks configuration alignment against the built-in defaults. Requires no git repository, no network access, and no tailor configuration; it can be run in any directory, including projects that have never used tailor. It is the recommended first step when assessing an unfamiliar project.

```bash
tailor measure
```

**Without `.tailor/config.yml`** (health file check only):

```
missing:        .github/FUNDING.yml
missing:        .github/ISSUE_TEMPLATE/bug_report.yml
missing:        .github/ISSUE_TEMPLATE/feature_request.yml
missing:        .github/dependabot.yml
missing:        .github/pull_request_template.md
missing:        CONTRIBUTING.md
missing:        SUPPORT.md
present:        CODE_OF_CONDUCT.md
present:        LICENSE
present:        SECURITY.md

No .tailor/config.yml found. Run `tailor fit <path>` to initialise, or create `.tailor/config.yml` manually to enable configuration alignment checks.
```

**With `.tailor/config.yml`** (health file check and configuration alignment):

```
missing:        CONTRIBUTING.md
present:        LICENSE
present:        SECURITY.md
not-configured: .github/dependabot.yml
config-only:    some-custom-swatch.yml
mode-differs:   SECURITY.md          (config: first-fit, default: always)
```

Category definitions:
- `missing` - health file does not exist on disk
- `present` - health file exists on disk
- `not-configured` - default swatch whose destination is not covered by any entry in `config.yml`; the default swatch will not be applied until added
- `config-only` - swatch in `config.yml` whose destination is not covered by any entry in the built-in default set. This arises when a swatch is removed from the built-in defaults in a newer tailor release but the project's `config.yml` still references it. `alter` will reject unrecognised swatch sources, so this category serves as a diagnostic hint that `config.yml` needs updating
- `mode-differs` - swatch whose destination appears in both `config.yml` and the default set, but with a different alteration mode; the inline annotation shows both values

Output order: `missing`, `present`, `not-configured`, `config-only`, `mode-differs`. Within each category, entries are sorted lexicographically by destination path. The category label is padded to a fixed width of 16 characters (the length of `not-configured: `) for consistent column alignment. For `mode-differs` entries, the annotation (e.g. `(config: first-fit, default: always)`) is separated from the destination path by a single space; no additional fixed column alignment is applied to the annotation. Health file checks are always performed and reported regardless of whether `.tailor/config.yml` is present; config-diff categories (`not-configured`, `config-only`, `mode-differs`) are shown only when `.tailor/config.yml` is present.

The `present`/`missing` check covers health swatches only. The config-diff check (`config-only`, `not-configured`, `mode-differs`) compares against the full default swatch set (both health and development swatches), since `config.yml` covers all swatches.

### `docket`

Displays the current GitHub authentication state and repository context. This is the answer to "whose job is this and who's doing it?" - it shows who is authenticated, what repository is in scope, and whether tailor can operate.

`docket` requires no arguments. It does not require authentication - it reports unauthenticated state instead of erroring.

```bash
tailor docket
```

**Authenticated, with repository context:**

```
user:           octocat
repository:     octocat/my-project
auth:           authenticated
```

**Authenticated, without repository context:**

```
user:           octocat
repository:     (none)
auth:           authenticated
```

**Not authenticated:**

```
user:           (none)
repository:     (none)
auth:           not authenticated
```

Behaviour:
- `user` is resolved via `GET /user` if authenticated; displays `(none)` if not authenticated.
- `repository` displays the `owner/repo` derived from the GitHub remote in the current directory; displays `(none)` if no GitHub remote exists.
- `auth` displays `authenticated` or `not authenticated` based on whether a valid token can be resolved for `github.com`.
- Does not read `.tailor/config.yml` and does not require it to be present.

## Error Handling

**Unrecognised swatch `source` in `config.yml`**: if `alter` encounters a `source` value that does not match any embedded swatch, it exits with an error identifying the unrecognised name and listing all valid swatch source names embedded in the binary.

**Licence fetch failed**: if `GET /licenses/{id}` returns an error during `alter` (e.g. unrecognised licence identifier), tailor exits with the API error.

**Destination path not writable**: tailor exits with an error showing the full path that could not be written.

**`.tailor/config.yml` malformed or missing**: if `alter` or `baste` reads a missing or malformed `.tailor/config.yml`, it exits with a clear message directing the user to run `fit` to create a valid configuration, or edit `.tailor/config.yml` directly to correct it.

**`always` swatch modified locally**: tailor treats the file as changed whenever the SHA-256 of the embedded swatch content differs from the on-disk file. `alter` overwrites it unconditionally. Tailor does not preserve local edits to `always` swatches; use `first-fit` alteration mode if local modifications must be retained after the initial fit. `--recut` overrides `first-fit` protection for all swatches except the licence file and `.tailor/config.yml`, which are never overwritten regardless of flags.

**Duplicate destination in `config.yml`**: if `alter` detects that two or more swatches share a destination, it exits with an error identifying the conflicting swatches before making any changes.

**Not authenticated**: if no valid authentication token can be resolved for `github.com` (neither `GH_TOKEN`/`GITHUB_TOKEN` environment variable, `gh` config file, nor `gh` keyring), `fit`, `alter`, and `baste` exit with: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run `gh auth login`."

**`{{GITHUB_USERNAME}}` resolution failed**: `{{GITHUB_USERNAME}}` is resolved via the GitHub REST API (`GET /user`). If this call fails (e.g. rate limits, network issues), `alter` exits with the API error. Unlike repo-context tokens, `{{GITHUB_USERNAME}}` depends on the authenticated user, not the repository, so it cannot be deferred.

**Repo-context tokens unresolved**: `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, and `{{HOMEPAGE_URL}}` require a GitHub repository context. If the project has no GitHub remote (e.g. a brand-new project not yet pushed), these tokens are left unsubstituted silently. For `always` swatches (e.g. `SECURITY.md`), `alter` will resolve and substitute them on a subsequent run once the repository has a remote. For `first-fit` swatches (e.g. `.github/ISSUE_TEMPLATE/config.yml`), delete the file and re-run `alter`, or use `--recut`.

**Repository settings without repo context**: if `config.yml` contains a `repository` section but the project has no GitHub remote (no repository context found), repository settings are skipped with a warning: "No GitHub repository context found. Repository settings will be applied once a remote is configured." Warning only; does not block swatch or licence processing.

**Repository settings API failure**: if the `PATCH /repos/{owner}/{repo}` call to apply repository settings fails, `alter` exits with the API error. Because repository settings are applied first in the execution order, licence and swatch operations are not attempted. If licence fetch fails after repository settings have been applied, the settings are not reverted.

**Unrecognised repository setting**: if `config.yml` contains a field in the `repository` section that is not in the supported settings list, `alter` exits with an error identifying the unrecognised field and listing all valid repository setting field names.

**`fit` repository settings query failed**: if `fit` detects a GitHub remote but the subsequent API call to read repository settings fails (e.g. insufficient permissions, network error), `fit` exits with the API error. The user can re-run `fit` after resolving the issue, or create `.tailor/config.yml` manually.

## Configuration

### `.tailor/config.yml`

`config.yml` has three top-level sections: `license` (a string), `repository` (a map of GitHub repository settings), and `swatches` (a list of swatch entries). `source` values use the full source path relative to `swatches/`, including the file extension where one exists. Extensionless files (e.g. `justfile`) are referenced as-is. The `repository` section is optional; if absent, repository settings are not managed.

Default (with `--license=MIT`). The `license` key varies by flag (`MIT`, `Apache-2.0`, `none`, etc.) - the rest of the generated file is identical regardless of licence choice:

```yaml
# Initially fitted by tailor on 2026-03-02
license: MIT

repository:
  description: ""
  homepage: "{{HOMEPAGE_URL}}"
  has_wiki: false
  has_discussions: false
  has_projects: false
  has_issues: true
  allow_merge_commit: false
  allow_squash_merge: true
  allow_rebase_merge: true
  squash_merge_commit_title: PR_TITLE
  squash_merge_commit_message: PR_BODY
  delete_branch_on_merge: true
  allow_update_branch: true
  allow_auto_merge: true
  web_commit_signoff_required: false
  private_vulnerability_reporting_enabled: true

swatches:
  - source: .github/workflows/tailor.yml
    destination: .github/workflows/tailor.yml
    alteration: always

  - source: .github/dependabot.yml
    destination: .github/dependabot.yml
    alteration: first-fit

  - source: .github/FUNDING.yml
    destination: .github/FUNDING.yml
    alteration: first-fit

  - source: .github/ISSUE_TEMPLATE/bug_report.yml
    destination: .github/ISSUE_TEMPLATE/bug_report.yml
    alteration: always

  - source: .github/ISSUE_TEMPLATE/feature_request.yml
    destination: .github/ISSUE_TEMPLATE/feature_request.yml
    alteration: always

  - source: .github/ISSUE_TEMPLATE/config.yml
    destination: .github/ISSUE_TEMPLATE/config.yml
    alteration: first-fit

  - source: .github/pull_request_template.md
    destination: .github/pull_request_template.md
    alteration: always

  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always

  - source: CODE_OF_CONDUCT.md
    destination: CODE_OF_CONDUCT.md
    alteration: always

  - source: CONTRIBUTING.md
    destination: CONTRIBUTING.md
    alteration: always

  - source: SUPPORT.md
    destination: SUPPORT.md
    alteration: always

  - source: justfile
    destination: justfile
    alteration: first-fit

  - source: flake.nix
    destination: flake.nix
    alteration: first-fit

  - source: .gitignore
    destination: .gitignore
    alteration: first-fit

  - source: .envrc
    destination: .envrc
    alteration: first-fit

  - source: .github/workflows/tailor-automerge.yml
    destination: .github/workflows/tailor-automerge.yml
    alteration: triggered

  - source: .tailor/config.yml
    destination: .tailor/config.yml
    alteration: first-fit
```

### Registry

No global registry. Projects are configured by the presence of `.tailor/config.yml`.

## Swatch Storage

Swatches are embedded in the tailor binary at build time from `swatches/`:

```
swatches/
├── .envrc
├── .gitignore
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── SECURITY.md
├── SUPPORT.md
├── flake.nix
├── justfile
├── .github/
│   ├── dependabot.yml
│   ├── FUNDING.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   ├── config.yml
│   │   └── feature_request.yml
│   ├── pull_request_template.md
│   └── workflows/
│       ├── tailor.yml
│       └── tailor-automerge.yml
└── .tailor/
    └── config.yml
```

`.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically. `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically; if no GitHub repository context exists at `alter` time, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically; resolution follows the same mechanism as `{{ADVISORY_URL}}`, constructing `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. `.tailor/config.yml` has `{{HOMEPAGE_URL}}` substituted automatically, constructing `https://github.com/<owner>/<name>` from the repository context; if no repository context exists, the token is left unsubstituted. `.github/dependabot.yml` covers the `github-actions` package ecosystem for automated dependency updates of GitHub Actions. `.github/workflows/tailor-automerge.yml` is a triggered swatch that auto-merges Dependabot pull requests; it is deployed only when `allow_auto_merge: true` is set in the `repository` section.

Licences are not embedded - they are fetched at `alter` time via the GitHub REST API (`GET /licenses/{id}`) and written verbatim to `LICENSE`.

## GitHub Action

The `.github/workflows/tailor.yml` swatch delivers a GitHub Actions workflow that runs `tailor alter` on a weekly schedule and opens a pull request whenever swatch content has changed. The workflow is placed by `alter` like any other swatch; no manual setup is required beyond including it in the swatch list.

`wimpysworld/tailor-action@v1` is a separate GitHub Actions action maintained alongside tailor that installs the tailor binary into the workflow runner. It is a separate deliverable from the tailor CLI itself.

The swatch content:

```yaml
name: Tailor 🪡
on:
  schedule:
    - cron: "0 9 * * 1" # Weekly
  workflow_dispatch:

jobs:
  alter:
    runs-on: ubuntu-slim
    env:
      GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    steps:
      - uses: actions/checkout@v4

      - name: Setup tailor
        uses: wimpysworld/tailor-action@v1

      - name: Alter swatches
        run: tailor alter

      - name: Create PR
        uses: peter-evans/create-pull-request@v6
        with:
          branch: tailor-alter
          title: "chore: alter tailor swatches"
```

Action behaviour:
- `tailor alter` writes changes to the working tree; `create-pull-request` opens a PR. The PR body is auto-generated by `peter-evans/create-pull-request` from its diff detection - no `body` or `body-path` is set.
- Committing and pushing are handled by `peter-evans/create-pull-request`, not by tailor. Tailor only modifies files in the working tree.
- The action runs in a non-interactive shell. `GH_TOKEN` is set at the job level, providing the authentication token directly to `go-gh` via environment variable. The `gh` binary is not required for token resolution when `GH_TOKEN` is set. `first-fit` swatches (`.github/FUNDING.yml`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor/config.yml`, the licence file) are not overwritten after initial creation. `SECURITY.md` is `always` mode and is rewritten on every run (see the substituted-swatch rule in the `alter` behaviour section). Although the file is rewritten each time, `{{ADVISORY_URL}}` resolves to the same URL for a given repository, so the substituted content is identical across runs - git detects no diff and `create-pull-request` opens no PR. If a tailor upgrade changes a swatch template, the file will differ and a PR will be opened.
- Because `.github/workflows/tailor.yml` is itself an `always` swatch, the action workflow is kept current automatically: if the embedded swatch content changes in a new tailor release, the weekly run will update the workflow file and open a PR.

## Automerge Workflow

The `.github/workflows/tailor-automerge.yml` swatch delivers a GitHub Actions workflow that auto-merges Dependabot pull requests. It is a `triggered` swatch, deployed only when `allow_auto_merge: true` is set in the `repository` section of `config.yml`. The file is namespaced with a `tailor-` prefix to avoid collisions with user-managed automerge workflows.

**Prerequisite**: Auto-merge requires branch protection with at least one required status check on the default branch. Without this, `gh pr merge --auto` merges immediately with no CI gate. See [GitHub's documentation on managing a branch protection rule](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/managing-a-branch-protection-rule) for guidance.

**Per-ecosystem merge policy**:

| Ecosystem | Patch | Minor | Major |
|-----------|-------|-------|-------|
| GitHub Actions | Auto-merge | Auto-merge | Auto-merge |
| All others | Auto-merge | Auto-merge | Skip |

GitHub Actions use major version tags (v1, v2, v3) as their release convention, so Dependabot reports nearly every action update as a major version bump. All action updates are auto-merged regardless of semver level. Major bumps in other ecosystems (Go modules, npm, pip) follow semantic versioning where major indicates breaking changes; these are left for manual review.

The workflow uses `gh pr merge --auto --merge` which enables GitHub's auto-merge feature on the PR. The merge only completes after all required status checks and branch protection rules pass.

**Manual catch-up**: The workflow supports `workflow_dispatch` for repositories with pre-existing open Dependabot PRs. When triggered manually, a separate `automerge-existing` job lists all open Dependabot PRs and enables auto-merge on each. The manual job does not apply per-ecosystem filtering; required status checks still gate every merge.

**Opt-out**: Users who have `allow_auto_merge: true` but use their own automerge solution can set `alteration: never` on the automerge swatch entry in `config.yml` to suppress deployment while keeping the entry visible.

## Justfile Integration

The `justfile` swatch is a minimal bootstrap scaffold covering tailor operations only. It is placed as `first-fit` and is not updated after initial delivery; projects are expected to extend it with their own recipes.

```makefile
# List available recipes
default:
    @just --list

# Alter tailor swatches
alter:
    @tailor alter

# Check what tailor would change and measure
measure:
    @tailor baste
    @tailor measure
```

## Implementation Notes

1. **Overwrite detection**: SHA-256 hash comparison between the embedded swatch content (from the tailor binary) and the on-disk target file. SHA-256 comparison applies only to `always` swatches; `first-fit` swatches are skipped entirely if the destination exists, with no comparison performed. The on-disk file is overwritten only when this comparison shows a difference. For `always` swatches containing substitution tokens, the SHA-256 comparison is skipped entirely and the swatch is always overwritten (see the authoritative rule in the `alter` behaviour section). Bypassed with `--recut`.
2. **Interpolation (FUNDING.yml, SECURITY.md, config.yml, and .tailor/config.yml)**: Swatches are complete verbatim files with four exceptions. `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted at `alter` time from `GET /user`. `SECURITY.md` has `{{ADVISORY_URL}}` constructed from the repository context (owner/name); if no repository context exists, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` constructed from the repository context, producing `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`; if no repository context exists, the token is left unsubstituted. `.tailor/config.yml` has `{{HOMEPAGE_URL}}` constructed from the repository context, producing `https://github.com/<owner>/<name>`; if no repository context exists, the token is left unsubstituted. No per-swatch configuration is required. Licences are fetched via `GET /licenses/{id}` and written verbatim - no token substitution is involved.
3. **No versioning**: No swatch versions, always uses swatches from current tailor binary. Upgrading tailor will cause all `always` swatches to be re-evaluated against the new embedded content; files whose swatch content has changed will be overwritten on the next `alter` run.
4. **No global state**: All state is per-project in `.tailor/config.yml`
5. **No project registry**: Tailor has no awareness of its consumers. Projects pull from tailor, tailor does not track projects.
6. **Authentication via `go-gh`**: All project metadata, user metadata, licence content, and repository settings are resolved via `go-gh` (`github.com/cli/go-gh/v2`), the official Go library for GitHub CLI extensions. Token resolution follows the `go-gh` precedence order: `GH_TOKEN` environment variable, `GITHUB_TOKEN` environment variable, `gh` config file, `gh` keyring (via the `gh` binary). When `GH_TOKEN` or `GITHUB_TOKEN` is set, the `gh` binary is not required. The `gh` binary is needed only for `gh auth login` (establishing credentials) and as a fallback for keyring-based token access when no environment variable is set. Repository context detection reads git remotes via `go-gh`, so `git` must be present when a GitHub remote exists - but any directory with a GitHub remote already has `git` installed. If no valid token can be resolved, `fit`, `alter`, and `baste` exit immediately with an error.
7. **CLI parsing**: [Kong](https://github.com/alecthomas/kong) is used as the command line parser.
8. **Repository settings via API**: Repository settings are applied via `PATCH /repos/{owner}/{repo}` with a JSON body constructed from the `repository` section of `config.yml`. Field names map directly to the GitHub REST API without translation. Current settings are read via `GET /repos/{owner}/{repo}` for `baste` comparison. All API calls use `go-gh`'s pre-authenticated REST client. The `alter` execution order is: repository settings, then licence, then swatches.
