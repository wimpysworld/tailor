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

`fit` creates the project directory and writes `.tailor.yml` with the full default swatch set in one command, with a `license: BlueOak-1.0.0` default. Use `--license=<id>` to select a different licence or `--license=none` to opt out. Change into `<path>`, then run `alter` to copy the swatch files, including the `.github/workflows/tailor.yml` workflow that handles weekly automated maintenance. The action opens a pull request whenever swatch content changes, keeping files current without manual intervention.

### Existing project

`measure` checks which community health files are present or missing - run it first to see what a project needs. If no `.tailor.yml` exists, run `tailor fit .` to create one (the directory already exists, so `fit` proceeds without error), or create `.tailor.yml` manually. Edit `.tailor.yml` directly to add or remove swatches or change alteration modes, then run `alter` to bring the project into sync with the current swatches; the `.github/workflows/tailor.yml` swatch handles ongoing maintenance once placed, opening pull requests whenever upstream swatch content changes.

## Core Concepts

**Swatches**: Complete, ready-to-use template files stored in `swatches/`. Files are copied verbatim, with five exceptions: `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically; `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically; `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically; `.tailor.yml` has `{{HOMEPAGE_URL}}` substituted automatically; `.github/workflows/tailor-automerge.yml` has `{{MERGE_STRATEGY}}` substituted automatically.

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
| `.tailor.yml` | `.tailor.yml` |

Swatch-to-path mappings are hardcoded in the source. Licences are not swatches - they are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

**Repository Settings**: Tailor can manage GitHub repository settings declaratively via the `repository` section in `.tailor.yml`. Field names match the GitHub REST API field names exactly (snake_case). Settings are applied via `PATCH /repos/{owner}/{repo}` as a single API call, with additional fields applied via their own separate API endpoints. Repository settings are always applied idempotently on every `alter` run - there is no `first-fit` concept for API settings. If the `repository` section is absent from `.tailor.yml`, repository settings are skipped entirely.

**Labels**: Tailor can manage GitHub issue labels declaratively via the `labels` section in `.tailor.yml`. Labels are a top-level config key alongside `repository:` and `swatches:`, not a field within `repository:`. The reconciliation strategy is create and update only - labels present on GitHub but absent from config are left untouched. No pruning. Label name matching is case-insensitive. When a label's name differs only in casing from the config, tailor updates the casing to match. The default config includes 12 labels (9 GitHub defaults plus `dependencies`, `github_actions`, and `hacktoberfest-accepted`) with colours from the Catppuccin Latte accent palette. If the `labels` section is absent from `.tailor.yml`, label management is skipped entirely.

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
| `vulnerability_alerts_enabled` | bool | Enable Dependabot vulnerability alerts |
| `automated_security_fixes_enabled` | bool | Enable Dependabot automated security fix PRs |
| `topics` | string array | Repository topics for discoverability (replace-all semantics) |
| `default_workflow_permissions` | string | Default GITHUB_TOKEN permissions (`read` or `write`) |
| `can_approve_pull_request_reviews` | bool | Allow GitHub Actions to approve pull requests |

Several fields use separate API endpoints rather than the repository PATCH call. Tailor handles this transparently - they appear in `.tailor.yml` alongside other repository settings but are applied via their own API calls:

| Field | Read | Write |
|---|---|---|
| `private_vulnerability_reporting_enabled` | `GET /repos/{owner}/{repo}/private-vulnerability-reporting` | `PUT`/`DELETE /repos/{owner}/{repo}/private-vulnerability-reporting` |
| `vulnerability_alerts_enabled` | `GET /repos/{owner}/{repo}/vulnerability-alerts` (204=enabled, 404=disabled) | `PUT`/`DELETE /repos/{owner}/{repo}/vulnerability-alerts` |
| `automated_security_fixes_enabled` | `GET /repos/{owner}/{repo}/automated-security-fixes` (JSON `{"enabled": bool}`) | `PUT`/`DELETE /repos/{owner}/{repo}/automated-security-fixes` |
| `topics` | Read from `GET /repos/{owner}/{repo}` response (no extra call) | `PUT /repos/{owner}/{repo}/topics` with `{"names": [...]}` |
| `default_workflow_permissions`, `can_approve_pull_request_reviews` | `GET /repos/{owner}/{repo}/actions/permissions/workflow` | `PUT /repos/{owner}/{repo}/actions/permissions/workflow` (both fields atomically) |

**Admin role required**: `vulnerability_alerts_enabled`, `automated_security_fixes_enabled`, and `private_vulnerability_reporting_enabled` require the caller to hold admin role (or security manager role for PVR) on the repository. `GITHUB_TOKEN` never holds `administration` permission regardless of `permissions:` in the workflow - this is a GitHub platform constraint. When `GITHUB_TOKEN` is used and these fields appear in `.tailor.yml`, Tailor skips them with a warning and continues. To manage them from CI, supply a PAT via `GH_TOKEN: ${{ secrets.TAILOR_PAT }}` - see README.md for the workaround.

**Ordering constraint**: `automated_security_fixes_enabled` requires `vulnerability_alerts_enabled` to be active. When enabling both, alerts are enabled first, then security fixes. When disabling both, security fixes are disabled first, then alerts. If `automated_security_fixes_enabled: true` is declared but alerts are disabled on GitHub, a warning is emitted.

**Topics**: The PUT endpoint replaces the entire topics list. The config declares the complete desired set; omitted topics are removed on apply. Topics are project-specific and not included in the default config template. Topic names must start with a lowercase letter or number, contain only lowercase alphanumerics and hyphens, and be 50 characters or fewer. The `topics` field uses `*[]string` semantics: nil (absent) means skip, empty list means clear all topics.

**Actions workflow permissions**: `default_workflow_permissions` accepts `read` or `write`. The PUT endpoint sends both `default_workflow_permissions` and `can_approve_pull_request_reviews` atomically. The tailor defaults (`read` and `false`) follow the principle of least privilege. GitHub defaults vary by context: personal repositories default to restricted `GITHUB_TOKEN` permissions with PR approval disabled, while organisation repositories inherit these settings from organisation-level Actions configuration.

Settings deliberately excluded due to risk or org-level scope: `visibility`, `default_branch`, `name`, `archived`, `is_template`, `allow_forking`, `security_and_analysis`. Additional API areas considered and deferred: Actions permissions policy (`enabled`, `allowed_actions`), autolinks, Pages configuration, deployment environments, custom properties (org-level), and Dependabot secrets. Branch protection (both classic rules and rulesets) is explicitly out of scope. It requires `Administration: write` - the same permission level needed to delete a repository - which `GITHUB_TOKEN` cannot hold at all; this is a deliberate GitHub security boundary preventing workflows from weakening the rules that govern their own repository. For Tailor's target audience of solo developers and small teams, branch protection is a one-time UI operation that does not drift over time, so the declarative consistency argument that justifies Tailor does not apply. Supporting it would roughly double the PAT privilege requirements for CI users for a setting they configure once, and `gh` CLI handles the setup in a single command, leaving no gap for Tailor to fill.

**Alteration Modes**:
- `always`: Tailor compares the embedded swatch content against the on-disk file on every `alter` run and overwrites if they differ. For `.tailor.yml` specifically, `always` means "append missing default swatch entries" rather than "overwrite content", because `.tailor.yml` content is user-managed. The config is rewritten only when entries are actually added
- `first-fit`: Tailor copies this file only if it does not already exist; never overwrites
- `triggered`: Tailor deploys this swatch only when a trigger condition elsewhere in the config is met. When the condition is met, behaves like `always` (overwrite when changed). When the condition is not met and the file exists on disk, Tailor removes it. Each triggered swatch has a trigger condition defined in a lookup table in the swatch package, mapping source path to a config field and expected value. Triggered swatches appear explicitly in `.tailor.yml` like any other swatch
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
| `.tailor.yml` | `always` |

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
- `.tailor.yml`

## Commands

Commands divide into three categories: bootstrap commands, which create the project and initial configuration; apply commands, which read `.tailor.yml` and modify project files; and inspection commands, which check the project without modifying anything.

**Bootstrap commands**: `fit`
**Apply commands**: `alter`
**Inspection commands**: `baste`, `measure`, `docket`

### `fit <path>`

Creates a new project directory and writes `.tailor.yml` with the full default swatch set and the repository settings. When run against an existing project with a GitHub remote, `fit` queries the live repository configuration and uses those values for the `repository` section, preserving the project's current state. When no repository context exists, the built-in defaults are used. Does not copy any files or apply any settings. After `fit`, change into `<path>` before running `alter`.

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
- `.tailor.yml`

A `license` key is included in `.tailor.yml` by default (`license: BlueOak-1.0.0`). Use `--license=<id>` to select a different licence or `--license=none` to opt out entirely.

`--license=<id>` records the licence identifier in `.tailor.yml`. Defaults to `BlueOak-1.0.0` if not specified. `--license=none` records `license: none`, opting out of licence creation. The identifier is used to fetch licence text via the GitHub REST API (`GET /licenses/{id}`) at `alter` time; any licence supported by the GitHub API is valid. `fit` does not validate the identifier - validation is deferred to `alter`.

`--description=<text>` sets the `description` field in the `repository` section of `.tailor.yml`, overriding any value from GitHub. `fit` does not apply the description - it is applied at `alter` time.

**Repository settings resolution at `fit` time**: `fit` detects repository context by querying GitHub remotes in `<path>`. If a GitHub remote exists, the project has repository context. If no remote is found, no repository context exists. Repository context detection reads git remotes (via `go-gh`), so `git` must be present when a GitHub remote exists - which is always the case in practice, since the remote implies a git repository.

When repository context exists, `fit` queries the live repository configuration via `GET /repos/{owner}/{repo}` and the separate endpoints for private vulnerability reporting, vulnerability alerts, automated security fixes, and Actions workflow permissions to populate the `repository` section with the project's current settings. This ensures that enabling tailor on an existing project does not inadvertently change features that are already configured (e.g. disabling wiki or discussions that are currently enabled). The `--description` flag takes precedence over the value from GitHub. `description` and `homepage` are omitted if empty. When no repository context exists (e.g. a brand-new project with no remote), the built-in defaults from the embedded swatch are used, with `description` and `homepage` normalised to nil by `DefaultConfig` so they are omitted from the generated config.

```bash
# Default licence (BlueOak-1.0.0)
tailor fit ./my-project

# Explicit licence selection
tailor fit ./my-project --license=Apache-2.0

# Opt out of licence entirely
tailor fit ./my-project --license=none

# Set description (overrides any value from GitHub)
tailor fit ./my-project --description="My awesome project"
```

If `<path>` already exists but does not contain `.tailor.yml`, `fit` proceeds without error and creates the configuration. If `<path>` already exists and contains `.tailor.yml`, `fit` exits with an error: "`.tailor.yml` already exists at `<path>`. Edit `.tailor.yml` directly to change the swatch configuration." `fit` creates all intermediate directories in `<path>` as needed.

Generates:
- Project directory at `<path>`
- `.tailor.yml` at `<path>/.tailor.yml`, containing the `license` key, the `repository` section (populated from live GitHub settings when available, otherwise from built-in defaults), the `labels` section (12 default labels with Catppuccin Latte colours), and the full default swatch set, each entry at its default alteration mode, prefixed with a `# Initially fitted by tailor on <DATE>` header comment (YYYY-MM-DD, no time).

### `alter`

Applies swatch alterations to the local project.

`alter` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Apply and overwrite regardless of mode or existence
```

Behaviour:
- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- **Config update** (when `.tailor.yml` has `alteration: always`): before processing swatches, `alter` merges built-in defaults into the loaded config across three sections. If any section was updated, the config file is rewritten to disk with a `# Refitted by tailor on <DATE>` header comment (YYYY-MM-DD). If nothing was added, the config file is not touched. When `.tailor.yml` has `alteration: first-fit`, this check is skipped entirely. See "Header comment" below for the comment format. The three merge rules are:
  - **Swatches**: for each default swatch whose path has no matching entry in the config, appends a new `SwatchEntry` with the default alteration mode. Existing entries are never modified - only missing entries are appended.
  - **Repository settings**: fills nil fields only from built-in defaults; never overwrites non-nil fields. `Description`, `Homepage`, and `Topics` are excluded from this merge because they are project-specific.
  - **Labels**: populated only when the labels section is entirely absent or empty (all-or-nothing). If the config already has any labels defined, no defaults are merged.
- For repository settings: if a `repository` section is present in `.tailor.yml`, reads the current repository settings via `GET /repos/{owner}/{repo}` and additional endpoints, compares each declared field against the live value, and applies changes via `PATCH /repos/{owner}/{repo}` plus separate API calls for fields with dedicated endpoints. Repository settings are applied first in the execution order. If no GitHub repository context exists (no remote), repository settings are skipped with a warning. `--recut` has no special effect on repository settings - they are always applied declaratively.
- For labels: if a `labels` section is present in `.tailor.yml`, reads the current labels via paginated `GET /repos/{owner}/{repo}/labels`, diffs desired vs current using case-insensitive name matching, creates missing labels via `POST`, and updates changed labels (colour or description differs) via `PATCH`. Labels present on GitHub but absent from config are left untouched. Labels are applied after repository settings and before licences and swatches. If no GitHub repository context exists (no remote), labels are skipped with a warning.
- For `always` swatches: compares the SHA-256 of the embedded swatch content against the on-disk file; overwrites if they differ. SHA-256 comparison applies only to `always` swatches. For swatches containing substitution tokens (`{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, `{{HOMEPAGE_URL}}`, or `{{MERGE_STRATEGY}}`), tokens are resolved before the SHA-256 comparison. The resolved content is hashed and compared against the on-disk file, so substituted swatches correctly produce `no change` when the resolved content matches. The set of substituted swatches is: `.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor.yml`, and `.github/workflows/tailor-automerge.yml`.
- For `first-fit` swatches: copies only if the destination file does not exist; never overwrites. If the destination exists, the swatch is skipped entirely - no SHA-256 comparison is performed.
- For `triggered` swatches: looks up the trigger condition for the swatch source in the trigger condition table. If the condition is met (e.g. `allow_auto_merge: true` in the `repository` section), behaves like `always` - deploys and overwrites when content differs. If the condition is not met and the file exists on disk, removes it. If the condition is not met and the file does not exist, skips silently. Triggered swatches are never overwritten by `--recut` when the trigger condition is false.
- For `never` swatches: skips entirely. No file is written, compared, or removed. This mode suppresses any swatch, including triggered swatches whose condition would otherwise be met.
- For licences: if `.tailor.yml` contains a `license` key with a value other than `none`, and no `LICENSE` file exists on disk, fetches the licence text via the GitHub REST API (`GET /licenses/{id}`) and writes it to `LICENSE`. The text is written verbatim as returned by GitHub - no token substitution is performed. Always treated as `first-fit`; the on-disk `LICENSE` file is never overwritten. If the licence fetch fails (e.g. unrecognised licence identifier), `alter` exits with the API error.
- For `.github/FUNDING.yml`: substitutes `{{GITHUB_USERNAME}}` before writing. `{{GITHUB_USERNAME}}` is resolved at `alter` time from `GET /user`. The Sponsorships checkbox under Settings > General > Features is not exposed via the GitHub API. After alter places `.github/FUNDING.yml`, enable sponsorships manually in repository settings.
- For `SECURITY.md`: substitutes `{{ADVISORY_URL}}` before writing. `{{ADVISORY_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/security/advisories/new` from the repository context (owner/name). If no GitHub repository context exists (e.g. a brand-new project with no remote), `{{ADVISORY_URL}}` is left unsubstituted in the written file. The unsubstituted token is intentionally detectable by a future `measure` run; `alter` will resolve and substitute it on a subsequent run once the repository has a remote.
- For `.github/ISSUE_TEMPLATE/config.yml`: substitutes `{{SUPPORT_URL}}` before writing. `{{SUPPORT_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md` from the repository context (owner/name). If no GitHub repository context exists, `{{SUPPORT_URL}}` is left unsubstituted in the written file.
- For `.tailor.yml`: substitutes `{{HOMEPAGE_URL}}` before writing. `{{HOMEPAGE_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>` from the repository context (owner/name). If no GitHub repository context exists, `{{HOMEPAGE_URL}}` is left unsubstituted in the written file.
- With `--recut`: overwrites regardless of mode or existence, including `first-fit` swatches - `--recut` will overwrite a `first-fit` swatch file even if it exists and has been locally modified. Use with care. The licence file is exempt from `--recut` and is never overwritten regardless, because it is fetched content not an embedded swatch. For `.tailor.yml`, `--recut` overrides `first-fit` to `always` semantics like any other swatch - this means missing default swatches are appended, but existing entries are never modified or overwritten, because `always` for `.tailor.yml` means "append missing entries". When `--recut` writes a substituted swatch (e.g. `.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor.yml`, `.github/workflows/tailor-automerge.yml`), the full token resolution pipeline runs and fresh values are substituted before writing.
- If no `license` key is present in `.tailor.yml` (or its value is `none`) and no `LICENSE` file exists in the project root, emits a warning: "No licence file found and no licence configured. Add `license: BlueOak-1.0.0` (or another identifier) to `.tailor.yml` and run `tailor alter`." Warning only; does not block execution.
- Creates intermediate directories as needed before writing any swatch whose destination path requires directories that do not yet exist.
- Never touches files not listed in `.tailor.yml`
- Modifies files only; does not commit or push

### `baste`

Previews what `alter` would do without making any changes.

`baste` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed.

```bash
tailor baste
```

Behaviour:
- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- `baste` performs the same comparison logic as `alter` but writes nothing. It reports what `alter` would do.

Output format - repository settings are shown first (if a `repository` section is present), then labels (if a `labels` section is present), then swatch entries.

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
`no change` - `always` or `triggered` swatch whose embedded content matches the on-disk file. `no change` only appears for `always` and active `triggered` swatches; `first-fit` swatches that exist always produce `skipped (first-fit, exists)`, never `no change`. Substituted swatches participate in the normal SHA-256 comparison after token resolution and can produce `no change` when the resolved content matches the on-disk file.
`skipped (first-fit, exists)` - `first-fit` swatch whose destination already exists; no comparison is performed.
`skip (never)` - swatch with `alteration: never`; skipped unconditionally.

Output order: actionable items first (`would set`, `would copy`, `would overwrite`, `would deploy`, `would remove`), then informational (`no change`, `skipped (first-fit, exists)`, `skip (never)`). Within each category, entries are sorted lexicographically by path or field name. The category label width is computed dynamically from the longest label for consistent column alignment.

### `measure`

Assesses a project's community health files and, when `.tailor.yml` is present, checks configuration alignment against the built-in defaults. Requires no git repository, no network access, and no tailor configuration; it can be run in any directory, including projects that have never used tailor. It is the recommended first step when assessing an unfamiliar project.

```bash
tailor measure
```

**Without `.tailor.yml`** (health file check only):

```
missing:        .github/FUNDING.yml
missing:        .github/ISSUE_TEMPLATE/bug_report.yml
missing:        .github/ISSUE_TEMPLATE/feature_request.yml
missing:        .github/dependabot.yml
missing:        .github/pull_request_template.md
missing:        CONTRIBUTING.md
missing:        SUPPORT.md
warning:        LICENSE (contains unresolved placeholders)
warning:        README.md (not managed by tailor)
present:        CODE_OF_CONDUCT.md
present:        SECURITY.md

No .tailor.yml found. Run `tailor fit <path>` to initialise, or create `.tailor.yml` manually to enable configuration alignment checks.
```

**With `.tailor.yml`** (health file check and configuration alignment):

```
missing:        CONTRIBUTING.md
warning:        LICENSE (contains unresolved placeholders)
present:        SECURITY.md
not-configured: .github/dependabot.yml
config-only:    some-custom-swatch.yml
mode-differs:   SECURITY.md          (config: first-fit, default: always)
```

Category definitions:
- `missing` - health file does not exist on disk
- `warning` - health diagnostic that requires attention but is not a missing swatch. Two cases are recognised: `LICENSE` exists but contains unresolved placeholder tokens (e.g. `[year]`, `[fullname]`, `{project}`), and `README.md` is absent from the project root. A warned path appears once in the output and does not also appear as `present`
- `present` - health file exists on disk
- `not-configured` - default swatch whose destination is not covered by any entry in `.tailor.yml`; the default swatch will not be applied until added
- `config-only` - swatch in `.tailor.yml` whose destination is not covered by any entry in the built-in default set. This arises when a swatch is removed from the built-in defaults in a newer tailor release but the project's `.tailor.yml` still references it. `alter` will reject unrecognised swatch paths, so this category serves as a diagnostic hint that `.tailor.yml` needs updating
- `mode-differs` - swatch whose destination appears in both `.tailor.yml` and the default set, but with a different alteration mode; the inline annotation shows both values

Output order: `missing`, `warning`, `present`, `not-configured`, `config-only`, `mode-differs`. Within each category, entries are sorted lexicographically by destination path. The category label is padded to a fixed width of 16 characters (the length of `not-configured: `) for consistent column alignment. For `warning` entries, the detail annotation (e.g. `(contains unresolved placeholders)`) is separated from the path by a single space, following the same annotation style as `mode-differs`. For `mode-differs` entries, the annotation (e.g. `(config: first-fit, default: always)`) is separated from the destination path by a single space; no additional fixed column alignment is applied to the annotation. Health file checks are always performed and reported regardless of whether `.tailor.yml` is present; config-diff categories (`not-configured`, `config-only`, `mode-differs`) are shown only when `.tailor.yml` is present.

`README.md` is a local health diagnostic, not a swatch or config-diff item. It is checked by exact path at the project root only. `README`, `README.rst`, and other variants do not satisfy the check. The `README.md` warning is not emitted when the file exists. Licence placeholder detection scans for `\[[^\]]+\]` and `\{[^}]+\}` patterns, covering GitHub licence template tokens such as `[year]`, `[fullname]`, `[yyyy]`, `[name of copyright owner]`, and `{project}`. The check runs only when `LICENSE` exists on disk; an absent `LICENSE` stays in the `missing` category.

The `present`/`missing`/`warning` check covers health swatches, `LICENSE`, and `README.md`. The config-diff check (`config-only`, `not-configured`, `mode-differs`) compares against the full default swatch set (both health and development swatches), since `.tailor.yml` covers all swatches.

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
- Does not read `.tailor.yml` and does not require it to be present.

## Error Handling

**Unrecognised swatch `path` in `.tailor.yml`**: if `alter` encounters a `path` value that does not match any embedded swatch, it exits with an error identifying the unrecognised name and listing all valid swatch paths embedded in the binary.

**Licence fetch failed**: if `GET /licenses/{id}` returns an error during `alter` (e.g. unrecognised licence identifier), tailor exits with the API error.

**Destination path not writable**: tailor exits with an error showing the full path that could not be written.

**`.tailor.yml` malformed or missing**: if `alter` or `baste` reads a missing or malformed `.tailor.yml`, it exits with a clear message directing the user to run `fit` to create a valid configuration, or edit `.tailor.yml` directly to correct it.

**`always` swatch modified locally**: tailor treats the file as changed whenever the SHA-256 of the embedded swatch content differs from the on-disk file. `alter` overwrites it unconditionally. Tailor does not preserve local edits to `always` swatches; use `first-fit` alteration mode if local modifications must be retained after the initial fit. `--recut` overrides `first-fit` protection for all swatches except the licence file, which is never overwritten regardless of flags. `.tailor.yml` uses `always` mode with append-only semantics - existing entries are never modified or overwritten, only missing default entries are appended.

**Duplicate path in `.tailor.yml`**: if `alter` detects that two or more swatches share a path, it exits with an error identifying the conflicting swatches before making any changes.

**Not authenticated**: if no valid authentication token can be resolved for `github.com` (neither `GH_TOKEN`/`GITHUB_TOKEN` environment variable, `gh` config file, nor `gh` keyring), `fit`, `alter`, and `baste` exit with: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run `gh auth login`."

**`{{GITHUB_USERNAME}}` resolution failed**: `{{GITHUB_USERNAME}}` is resolved via the GitHub REST API (`GET /user`). If this call fails (e.g. rate limits, network issues), `alter` exits with the API error. Unlike repo-context tokens, `{{GITHUB_USERNAME}}` depends on the authenticated user, not the repository, so it cannot be deferred.

**Repo-context tokens unresolved**: `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, and `{{HOMEPAGE_URL}}` require a GitHub repository context. If the project has no GitHub remote (e.g. a brand-new project not yet pushed), these tokens are left unsubstituted silently. For `always` swatches (e.g. `SECURITY.md`), `alter` will resolve and substitute them on a subsequent run once the repository has a remote. For `first-fit` swatches (e.g. `.github/ISSUE_TEMPLATE/config.yml`), delete the file and re-run `alter`, or use `--recut`.

**Repository settings without repo context**: if `.tailor.yml` contains a `repository` section but the project has no GitHub remote (no repository context found), repository settings are skipped with a warning: "No GitHub repository context found. Repository settings will be applied once a remote is configured." Warning only; does not block swatch or licence processing.

**Repository settings API failure**: if any API call to apply repository settings fails (PATCH, PUT, or DELETE), `alter` exits with the API error. Because repository settings are applied first in the execution order, labels, licence, and swatch operations are not attempted. If licence fetch fails after repository settings and labels have been applied, those changes are not reverted.

**Unrecognised repository setting**: if `.tailor.yml` contains a field in the `repository` section that is not in the supported settings list, `alter` exits with an error identifying the unrecognised field and listing all valid repository setting field names.

**`fit` repository settings query failed**: if `fit` detects a GitHub remote but the subsequent API call to read repository settings fails (e.g. insufficient permissions, network error), `fit` exits with the API error. The user can re-run `fit` after resolving the issue, or create `.tailor.yml` manually.

## Configuration

### `.tailor.yml`

`.tailor.yml` has four top-level sections: `license` (a string), `repository` (a map of GitHub repository settings), `labels` (a list of label entries with name, colour, and description), and `swatches` (a list of swatch entries). `path` values use the full path relative to `swatches/`, including the file extension where one exists. Extensionless files (e.g. `justfile`) are referenced as-is. The `repository` and `labels` sections are optional; if absent, their respective management is skipped.

Default (with `--license=BlueOak-1.0.0`). The `license` key varies by flag (`MIT`, `Apache-2.0`, `none`, etc.) - the rest of the generated file is identical regardless of licence choice:

```yaml
# Initially fitted by tailor on 2026-03-02
license: BlueOak-1.0.0

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
  vulnerability_alerts_enabled: true
  automated_security_fixes_enabled: true
  default_workflow_permissions: read
  can_approve_pull_request_reviews: false

labels:
  - name: bug
    color: d20f39
    description: "Something isn't working"

  - name: documentation
    color: 04a5e5
    description: "Documentation improvement"

  - name: duplicate
    color: 8839ef
    description: "Already exists"

  - name: enhancement
    color: 1e66f5
    description: "New feature request"

  - name: good first issue
    color: 40a02b
    description: "Good for newcomers"

  - name: help wanted
    color: "179299"
    description: "Extra attention needed"

  - name: invalid
    color: e64553
    description: "Not valid or relevant"

  - name: question
    color: 7287fd
    description: "Needs more information"

  - name: wontfix
    color: dc8a78
    description: "Will not be worked on"

  - name: dependencies
    color: fe640b
    description: "Dependency update"

  - name: github_actions
    color: ea76cb
    description: "GitHub Actions update"

  - name: hacktoberfest-accepted
    color: df8e1d
    description: "Hacktoberfest contribution"

swatches:
  - path: .github/workflows/tailor.yml
    alteration: always

  - path: .github/dependabot.yml
    alteration: first-fit

  - path: .github/FUNDING.yml
    alteration: first-fit

  - path: .github/ISSUE_TEMPLATE/bug_report.yml
    alteration: always

  - path: .github/ISSUE_TEMPLATE/feature_request.yml
    alteration: always

  - path: .github/ISSUE_TEMPLATE/config.yml
    alteration: first-fit

  - path: .github/pull_request_template.md
    alteration: always

  - path: SECURITY.md
    alteration: always

  - path: CODE_OF_CONDUCT.md
    alteration: always

  - path: CONTRIBUTING.md
    alteration: always

  - path: SUPPORT.md
    alteration: always

  - path: justfile
    alteration: first-fit

  - path: flake.nix
    alteration: first-fit

  - path: .gitignore
    alteration: first-fit

  - path: .envrc
    alteration: first-fit

  - path: .github/workflows/tailor-automerge.yml
    alteration: triggered

  - path: .tailor.yml
    alteration: always
```

### Header comment

The first line of `.tailor.yml` is a header comment indicating when the config was created or last updated by Tailor.

- `# Initially fitted by tailor on <DATE>` - written by `fit` when the config is first created.
- `# Refitted by tailor on <DATE>` - written by `alter` when missing default swatches are appended to the config. The date is the current date (YYYY-MM-DD). If `alter` finds no missing entries, the header is not changed.

The `config.Write` function accepts a date string and a header verb. The template uses the verb to select between "Initially fitted" and "Refitted".

### Registry

No global registry. Projects are configured by the presence of `.tailor.yml`.

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
└── .tailor.yml
```

`.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically. `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically; if no GitHub repository context exists at `alter` time, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically; resolution follows the same mechanism as `{{ADVISORY_URL}}`, constructing `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. `.tailor.yml` has `{{HOMEPAGE_URL}}` substituted automatically, constructing `https://github.com/<owner>/<name>` from the repository context; if no repository context exists, the token is left unsubstituted. `.github/dependabot.yml` covers the `github-actions` package ecosystem for automated dependency updates of GitHub Actions. `.github/workflows/tailor-automerge.yml` is a triggered swatch that auto-merges Dependabot pull requests; it is deployed only when `allow_auto_merge: true` is set in the `repository` section. `.github/workflows/tailor-automerge.yml` has `{{MERGE_STRATEGY}}` substituted automatically. `{{MERGE_STRATEGY}}` resolves to `--squash`, `--rebase`, or `--merge` based on the repository merge settings in `.tailor.yml`. Preference order: squash > rebase > merge. If no merge method is explicitly enabled, defaults to `--squash`.

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
- The action runs in a non-interactive shell. `GH_TOKEN` is set at the job level, providing the authentication token directly to `go-gh` via environment variable. The `gh` binary is not required for token resolution when `GH_TOKEN` is set. `first-fit` swatches (`.github/FUNDING.yml`, `.github/ISSUE_TEMPLATE/config.yml`, the licence file) are not overwritten after initial creation. `.tailor.yml` uses `always` mode but only appends missing swatch entries - existing content is never overwritten. `SECURITY.md` is `always` mode and is compared on every run, but it is rewritten only when the resolved content differs (see the substituted-swatch rule in the `alter` behaviour section). Because `{{ADVISORY_URL}}` usually resolves to the same URL for a given repository, this typically results in no diff and `create-pull-request` opens no PR. If a tailor upgrade changes a swatch template, the file will differ and a PR will be opened.
- Because `.github/workflows/tailor.yml` is itself an `always` swatch, the action workflow is kept current automatically: if the embedded swatch content changes in a new tailor release, the weekly run will update the workflow file and open a PR.

## Automerge Workflow

The `.github/workflows/tailor-automerge.yml` swatch delivers a GitHub Actions workflow that auto-merges Dependabot pull requests. It is a `triggered` swatch, deployed only when `allow_auto_merge: true` is set in the `repository` section of `.tailor.yml`. The file is namespaced with a `tailor-` prefix to avoid collisions with user-managed automerge workflows.

**Prerequisite**: Auto-merge requires branch protection with at least one required status check on the default branch. Without this, `gh pr merge --auto` merges immediately with no CI gate. See [GitHub's documentation on managing a branch protection rule](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/managing-a-branch-protection-rule) for guidance.

**Per-ecosystem merge policy**:

| Ecosystem | Patch | Minor | Major |
|-----------|-------|-------|-------|
| GitHub Actions | Auto-merge | Auto-merge | Auto-merge |
| All others | Auto-merge | Auto-merge | Skip |

GitHub Actions use major version tags (v1, v2, v3) as their release convention, so Dependabot reports nearly every action update as a major version bump. All action updates are auto-merged regardless of semver level. Major bumps in other ecosystems (Go modules, npm, pip) follow semantic versioning where major indicates breaking changes; these are left for manual review.

The workflow uses `gh pr merge --auto {{MERGE_STRATEGY}}` where `{{MERGE_STRATEGY}}` resolves to the appropriate merge strategy flag (`--squash`, `--rebase`, or `--merge`) based on the repository merge settings in `.tailor.yml`. Preference order: squash > rebase > merge. If no merge method is explicitly enabled, defaults to `--squash`. The merge only completes after all required status checks and branch protection rules pass.

**Manual catch-up**: The workflow supports `workflow_dispatch` for repositories with pre-existing open Dependabot PRs. When triggered manually, a separate `automerge-existing` job lists all open Dependabot PRs and enables auto-merge on each. The manual job does not apply per-ecosystem filtering; required status checks still gate every merge.

**Opt-out**: Users who have `allow_auto_merge: true` but use their own automerge solution can set `alteration: never` on the automerge swatch entry in `.tailor.yml` to suppress deployment while keeping the entry visible.

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

1. **Overwrite detection**: SHA-256 hash comparison between the embedded swatch content (from the tailor binary) and the on-disk target file. SHA-256 comparison applies only to `always` swatches; `first-fit` swatches are skipped entirely if the destination exists, with no comparison performed. The on-disk file is overwritten only when this comparison shows a difference. For swatches containing substitution tokens, tokens are resolved before the hash comparison, so the resolved content is compared against the on-disk file. Bypassed with `--recut`.
2. **Interpolation (FUNDING.yml, SECURITY.md, .tailor.yml, and tailor-automerge.yml)**: Swatches are complete verbatim files with five exceptions. `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted at `alter` time from `GET /user`. `SECURITY.md` has `{{ADVISORY_URL}}` constructed from the repository context (owner/name); if no repository context exists, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` constructed from the repository context, producing `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`; if no repository context exists, the token is left unsubstituted. `.tailor.yml` has `{{HOMEPAGE_URL}}` constructed from the repository context, producing `https://github.com/<owner>/<name>`; if no repository context exists, the token is left unsubstituted. `.github/workflows/tailor-automerge.yml` has `{{MERGE_STRATEGY}}` resolved to `--squash`, `--rebase`, or `--merge` based on the repository merge settings in `.tailor.yml`; preference order is squash > rebase > merge; defaults to `--squash` if no merge method is explicitly enabled. No per-swatch configuration is required. Licences are fetched via `GET /licenses/{id}` and written verbatim - no token substitution is involved.
3. **No versioning**: No swatch versions, always uses swatches from current tailor binary. Upgrading tailor will cause all `always` swatches to be re-evaluated against the new embedded content; files whose swatch content has changed will be overwritten on the next `alter` run.
4. **No global state**: All state is per-project in `.tailor.yml`
5. **No project registry**: Tailor has no awareness of its consumers. Projects pull from tailor, tailor does not track projects.
6. **Authentication via `go-gh`**: All project metadata, user metadata, licence content, and repository settings are resolved via `go-gh` (`github.com/cli/go-gh/v2`), the official Go library for GitHub CLI extensions. Token resolution follows the `go-gh` precedence order: `GH_TOKEN` environment variable, `GITHUB_TOKEN` environment variable, `gh` config file, `gh` keyring (via the `gh` binary). When `GH_TOKEN` or `GITHUB_TOKEN` is set, the `gh` binary is not required. The `gh` binary is needed only for `gh auth login` (establishing credentials) and as a fallback for keyring-based token access when no environment variable is set. Repository context detection reads git remotes via `go-gh`, so `git` must be present when a GitHub remote exists - but any directory with a GitHub remote already has `git` installed. If no valid token can be resolved, `fit`, `alter`, and `baste` exit immediately with an error.
7. **CLI parsing**: [Kong](https://github.com/alecthomas/kong) is used as the command line parser.
8. **Repository settings via API**: Repository settings are applied via `PATCH /repos/{owner}/{repo}` with a JSON body constructed from the `repository` section of `.tailor.yml`, plus separate API calls for fields with dedicated endpoints (private vulnerability reporting, vulnerability alerts, automated security fixes, topics, Actions workflow permissions). Field names map directly to the GitHub REST API without translation. Current settings are read via `GET /repos/{owner}/{repo}` and the relevant separate endpoints for `baste` comparison. All API calls use `go-gh`'s pre-authenticated REST client. The `alter` execution order is: repository settings, then labels, then licence, then swatches.
