# Tailor Specification v0.3

## Overview

Tailor is a local terminal CLI for managing project templates across GitHub repositories. It provides bespoke fitting for new projects and alterations for existing projects. Running `tailor` with no arguments displays help.

## Prerequisites

Tailor requires a valid GitHub authentication token. This can be provided in two ways:

1. **Environment variable**: Set `GH_TOKEN` or `GITHUB_TOKEN`. This works without the `gh` binary installed.
2. **GitHub CLI**: Install and authenticate the [GitHub CLI](https://cli.github.com/) (`gh`). Run `gh auth login` to authenticate. The `gh` binary is also used as a fallback for keyring-based token access when no environment variable is set.

The `fit`, `alter`, and `baste` commands verify that a valid authentication token exists at startup and exit with an error if no token is available.

`measure` and `docket` are exempt from the authentication requirement. `measure` performs purely local file inspection and needs no network access or authentication. `docket` can report unauthenticated state without erroring - it displays the auth state rather than requiring it.

## Intended Workflow

### New project

`fit` creates the project directory and writes `.tailor.yml` with the full default swatch set in one command, with a `license: BlueOak-1.0.0` default. Use `--license=<id>` to select a different licence or `--license=none` to opt out. Change into `<path>`, then run `alter` to copy the swatch files and apply repository settings.

### Existing project

`measure` checks which community health files are present or missing - run it first to see what a project needs. If no `.tailor.yml` exists, run `tailor fit .` to create one (the directory already exists, so `fit` proceeds without error), or create `.tailor.yml` manually. Edit `.tailor.yml` directly to add or remove swatches or change alteration modes, then run `alter` to bring the project into sync with the current swatches. Re-run the CLI after upgrading Tailor to apply updated `always` swatches.

## Core Concepts

**Swatches**: Complete, ready-to-use template files stored in `swatches/`. Files are copied verbatim except for three substitutions. `.github/FUNDING.yml` uses `{{GITHUB_USERNAME}}`. `SECURITY.md` uses `{{ADVISORY_URL}}`. `.github/ISSUE_TEMPLATE/config.yml` uses `{{SUPPORT_URL}}`.

**Swatch names**: Swatch references use the full source path relative to `swatches/`, including the file extension where one exists. Extensionless files are referenced as-is. For example, `swatches/.github/dependabot.yml` is referenced as `.github/dependabot.yml`. `swatches/SECURITY.md` is referenced as `SECURITY.md`. `swatches/justfile` is referenced as `justfile` without an extension.

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
| `cubic.yaml` | `cubic.yaml` |
| `.github/FUNDING.yml` | `.github/FUNDING.yml` |
| `.github/dependabot.yml` | `.github/dependabot.yml` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `.github/ISSUE_TEMPLATE/bug_report.yml` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `.github/ISSUE_TEMPLATE/feature_request.yml` |
| `.github/ISSUE_TEMPLATE/config.yml` | `.github/ISSUE_TEMPLATE/config.yml` |
| `.github/pull_request_template.md` | `.github/pull_request_template.md` |
| `.tailor.yml` | `.tailor.yml` |

Swatch-to-path mappings are hardcoded in the source. Licences are not swatches - they are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

**Repository Settings**: Tailor can manage GitHub repository settings declaratively via the `repository` section in `.tailor.yml`. Field names match the GitHub REST API field names exactly (snake_case). Settings are applied via `PATCH /repos/{owner}/{repo}` as a single API call, with additional fields applied via their own separate API endpoints. Repository settings are always applied idempotently on every `alter` run - there is no `first-fit` concept for API settings. If the `repository` section is absent from `.tailor.yml`, repository settings are skipped entirely.

**Actions policy**: Tailor manages repository GitHub Actions policy through the top-level `actions` section. The built-in defaults enable Actions, use `allowed_actions: selected`, disable SHA pinning, allow GitHub-owned and verified actions, and allow `freerangebytes/setup-actionlint@*`, `golang/govulncheck-action@*`, `golangci/golangci-lint-action@*`, `nick-fields/retry@*`, `robherley/go-test-action@*`, and `softprops/action-gh-release@*`. Default merging adds the complete section when it is absent.

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
| `automated_security_fixes_enabled` | bool | Enable Dependabot automated security fix pull requests |
| `topics` | string array | Repository topics for discoverability (replace-all semantics) |
| `default_workflow_permissions` | string | Default GITHUB_TOKEN permissions (`read` or `write`) |
| `can_approve_pull_request_reviews` | bool | Allow `GITHUB_TOKEN` workflows to create pull requests and submit approval reviews |

Several fields use separate API endpoints rather than the repository PATCH call. Tailor handles this transparently - they appear in `.tailor.yml` alongside other repository settings but are applied via their own API calls:

| Field | Read | Write |
|---|---|---|
| `private_vulnerability_reporting_enabled` | `GET /repos/{owner}/{repo}/private-vulnerability-reporting` | `PUT`/`DELETE /repos/{owner}/{repo}/private-vulnerability-reporting` |
| `vulnerability_alerts_enabled` | `GET /repos/{owner}/{repo}/vulnerability-alerts` (`204` means enabled) | `PUT`/`DELETE /repos/{owner}/{repo}/vulnerability-alerts` |
| `automated_security_fixes_enabled` | `GET /repos/{owner}/{repo}/automated-security-fixes` (`200` JSON contains `enabled` and `paused`) | `PUT`/`DELETE /repos/{owner}/{repo}/automated-security-fixes` |
| `topics` | Read from `GET /repos/{owner}/{repo}` response (no extra call) | `PUT /repos/{owner}/{repo}/topics` with `{"names": [...]}` |
| `default_workflow_permissions`, `can_approve_pull_request_reviews` | `GET /repos/{owner}/{repo}/actions/permissions/workflow` | `PUT /repos/{owner}/{repo}/actions/permissions/workflow` (both fields atomically) |

**Topics**: The PUT endpoint replaces the entire topics list. The config declares the complete desired set; omitted topics are removed on apply. Topics are project-specific and not included in the default config template. Topic names must start with a lowercase letter or number, contain only lowercase alphanumerics and hyphens, and be 50 characters or fewer. The `topics` field uses `*[]string` semantics: nil (absent) means skip, empty list means clear all topics.

**Security settings**: The built-in defaults set all three security settings to `true`, so generated configs expose each control. Each field uses `*bool` semantics: nil means unmanaged before default merging, and a Boolean value declares the required state. Before strict validation and default merging, Tailor normalises the automated security fixes prerequisite. When `automated_security_fixes_enabled` is `true` and `vulnerability_alerts_enabled` is absent or `false`, Tailor sets `vulnerability_alerts_enabled` to `true` in memory and emits `warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts`. `alter` and `alter --recut` write the corrected `.tailor.yml` before repository API work. `baste` reports `would update` and writes nothing. `measure` and `docket` accept the pair and write nothing. Default merging then appends other missing security settings and never changes an unrelated explicit value. Tailor applies only security endpoint settings whose live values differ.

GitHub can use `404` for a disabled feature or denied access. A private vulnerability reporting `404` always leaves the value unknown and produces an access warning. A vulnerability alerts or automated fixes `404` means disabled only when the repository response confirms `permissions.admin: true` and the Actions workflow permission read confirms Administration-read access. Otherwise, Tailor leaves the value unknown and produces an access warning. The automated fixes GET uses the `enabled` value from the official `200` JSON response and does not treat `204` as a GET result.

When Tailor enables alerts and automated fixes together, it enables alerts first. If the alerts read is unknown, or the write fails or is skipped, Tailor does not enable automated fixes. When Tailor disables both, it disables automated fixes first. If the automated fixes read is unknown, or the write fails or is skipped, Tailor does not disable alerts. Tailor warns about the prerequisite only when automated fixes are required and alerts will remain disabled. Other access errors produce skip results, and other API errors stop the command.

**Actions workflow permissions**: `default_workflow_permissions` accepts `read` or `write`. The PUT endpoint sends both `default_workflow_permissions` and `can_approve_pull_request_reviews` atomically. GitHub labels the latter setting “Allow GitHub Actions to create and approve pull requests”. Tailor keeps the REST API field name because repository config keys map directly to API fields. Enabling it permits the repository `GITHUB_TOKEN` to create pull requests and submit approval reviews when the workflow has `pull-requests: write`. The setting does not permit merges, bypass branch rules, or affect personal access tokens or separate GitHub App tokens. The tailor defaults (`read` and `false`) follow the principle of least privilege. If the API rejects the read or write, Tailor reports `would skip (insufficient scope)` in `baste` and skips the operation in `alter`. Use a token with the required repository permissions.

Supported top-level Actions policy settings:

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable GitHub Actions for the repository |
| `allowed_actions` | string | Allowed policy: `all`, `local_only`, or `selected` |
| `sha_pinning_required` | bool | Require full-length commit SHAs for actions |
| `github_owned_allowed` | bool | Allow GitHub-owned actions under the selected policy |
| `verified_allowed` | bool | Allow actions from verified creators under the selected policy |
| `patterns_allowed` | string array | Complete set of allowed action and reusable workflow patterns |

Tailor reads and writes `enabled`, `allowed_actions`, and `sha_pinning_required` through `/repos/{owner}/{repo}/actions/permissions`. Tailor uses `/repos/{owner}/{repo}/actions/permissions/selected-actions` for the other fields. The three selected-action fields are valid only with `allowed_actions: selected`. The selected endpoint replaces `patterns_allowed`; comparison sorts both lists because GitHub order has no policy meaning.

Each Actions policy field uses pointer semantics, so default merging preserves explicit Boolean values, an explicit custom list, and an explicit empty list. When the section is absent, Tailor adds the complete default policy. When the effective policy is `selected`, Tailor appends each missing selected-action field. A missing `patterns_allowed` field receives the six approved defaults. After default merging, a selected policy must include `github_owned_allowed`, `verified_allowed`, and `patterns_allowed`. When the policy is `all` or `local_only`, Tailor appends only missing core fields and leaves selected-action fields absent so the result remains valid.

When Tailor changes an enabled `all` policy to an enabled `selected` policy, it first writes `allowed_actions: selected`. If the requested policy disables active SHA pinning, this write preserves `sha_pinning_required: true`. Tailor then writes the complete selected restrictions and writes `sha_pinning_required: false` in a final core request. If SHA pinning stays the same or becomes stricter, the first request uses the requested value and Tailor omits the final request. A hard first-request failure leaves `all` active. A selected-policy failure leaves the narrower `selected` policy active and preserves SHA pinning. A final SHA request failure leaves the selected restrictions and SHA pinning active. Tailor returns an explicit error for each hard failure.

For other transitions from `all` or `local_only` to `selected`, Tailor disables Actions before it writes the complete selected restrictions and the requested final core policy. For an existing selected policy, Tailor writes changed selected restrictions before a core write that broadens access, including disabling SHA pinning. When an enabled policy combines selected broadening with core tightening, Tailor first disables Actions. Selected broadening means newly allowing GitHub-owned actions, verified actions, or patterns. Core tightening means enabling SHA pinning or disabling Actions. Tailor then writes the selected restrictions and final core policy. Tailor also disables Actions before any selected update whose final policy disables Actions. If either later write fails, Tailor returns an explicit error and leaves Actions disabled. If the selected-policy read fails while the effective policy stays selected, Tailor skips dependent core broadening. Organisation policy can restrict repository choices. Other access errors produce clear skip results, and hard errors stop the command.

Settings deliberately excluded due to risk or org-level scope: `visibility`, `default_branch`, `name`, `archived`, `is_template`, `allow_forking`, `security_and_analysis`. Additional API areas considered and deferred: autolinks, Pages configuration, deployment environments, custom properties (org-level), and Dependabot secrets. Branch protection (both classic rules and rulesets) is explicitly out of scope. It requires `Administration: write`, which is the same permission level needed to delete a repository. Tailor does not request this high-risk permission. For Tailor's target audience of solo developers and small teams, branch protection is a one-time UI operation that does not drift over time. The `gh` CLI handles the setup in a single command.

**Alteration Modes**:
- `always`: Tailor compares the embedded swatch content against the on-disk file on every `alter` run and overwrites if they differ. For `.tailor.yml` specifically, `always` means "migrate retired entries and append missing defaults" rather than "overwrite content", because `.tailor.yml` content is user-managed. The config is rewritten only when migration or default merging changes it
- `first-fit`: Tailor copies this file only if it does not already exist; never overwrites
- `never`: Tailor skips this swatch entirely. Tailor does not write or compare the destination. Use this mode to keep a swatch visible in the config without managing its destination

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
| `.github/pull_request_template.md` | `never` |
| `.github/dependabot.yml` | `first-fit` |
| `justfile` | `first-fit` |
| `cubic.yaml` | `first-fit` |
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
- `cubic.yaml`
- `.tailor.yml`

## Commands

Commands divide into three categories: bootstrap commands, which create the project and initial configuration; apply commands, which read `.tailor.yml` and modify project files; and inspection commands, which check the project without modifying anything.

**Bootstrap commands**: `fit`
**Apply commands**: `alter`
**Inspection commands**: `baste`, `measure`, `docket`

### `fit <path>`

Creates a new project directory and writes `.tailor.yml` with the full default swatch set and the repository settings. When run against an existing project with a GitHub remote, `fit` queries the live repository configuration and uses those values for the `repository` section, preserving the project's current state. When no repository context exists, the built-in defaults are used. Does not copy any files or apply any settings. After `fit`, change into `<path>` before running `alter`.

The default swatch set contains 16 embedded swatches:

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
- `cubic.yaml`
- `.tailor.yml`

A `license` key is included in `.tailor.yml` by default (`license: BlueOak-1.0.0`). Use `--license=<id>` to select a different licence or `--license=none` to opt out entirely.

`--license=<id>` records the licence identifier in `.tailor.yml`. Defaults to `BlueOak-1.0.0` if not specified. `--license=none` records `license: none`, opting out of licence creation. The identifier is used to fetch licence text via the GitHub REST API (`GET /licenses/{id}`) at `alter` time; any licence supported by the GitHub API is valid. `fit` does not validate the identifier - validation is deferred to `alter`.

`--description=<text>` sets the `description` field in the `repository` section of `.tailor.yml`, overriding any value from GitHub. `fit` does not apply the description - it is applied at `alter` time.

**Repository settings resolution at `fit` time**: `fit` detects repository context by querying GitHub remotes in `<path>`. If a GitHub remote exists, the project has repository context. If no remote is found, no repository context exists. Repository context detection reads git remotes (via `go-gh`), so `git` must be present when a GitHub remote exists - which is always the case in practice, since the remote implies a git repository.

When repository context exists, `fit` queries the live repository configuration via `GET /repos/{owner}/{repo}` and the separate endpoints for security features and Actions workflow permissions. The live values populate the `repository` section. This prevents Tailor from changing features that the repository already configures. The `--description` flag takes precedence over the value from GitHub. `description` and `homepage` are omitted if empty. When no repository context exists, the built-in defaults from the embedded swatch are used. `DefaultConfig` normalises `description` and `homepage` to nil, so the generated config omits them.

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

If `<path>` already exists but does not contain `.tailor.yml`, `fit` proceeds without error and creates the configuration. If `<path>` already exists and contains `.tailor.yml`, `fit` exits with an error: `.tailor.yml already exists at <path>; edit it directly to change swatch configuration`. `fit` creates all intermediate directories in `<path>` as needed.

Generates:
- Project directory at `<path>`
- `.tailor.yml` at `<path>/.tailor.yml`, containing the `license` key, the `repository` section (populated from live GitHub settings when available, otherwise from built-in defaults), the default `actions` section, the `labels` section (12 default labels with Catppuccin Latte colours), and the full default swatch set, each entry at its default alteration mode, prefixed with a `# Initially fitted by tailor on <DATE>` header comment (YYYY-MM-DD, no time).

### `alter`

Applies swatch alterations to the local project.

`alter` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Apply and override first-fit protection
```

Behaviour:
- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- **Retired workflow migration**: before strict path and mode validation, `alter` removes both retired paths from the in-memory config. The paths are `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. This migration accepts the historical `triggered` mode only on these retired entries. The migration ignores the entry mode and the mode of the `.tailor.yml` swatch.
- **Config update**: after migration, Tailor normalises the security prerequisite and emits its warning before validation. `alter` then writes a changed config once. The write uses a `# Refitted by tailor on <DATE>` header comment (YYYY-MM-DD). It combines security prerequisite normalisation and all retired-entry removals with built-in defaults merged in the same run. The write occurs before repository API work. If the config did not change, `alter` does not write it. The default merge runs when `.tailor.yml` has `alteration: always`. The `alteration: first-fit` mode skips the merge. Security prerequisite normalisation is independent of the config swatch mode. See "Header comment" below for the comment format. The four merge rules are:
  - **Swatches**: appends each missing default swatch with the default alteration mode. The merge does not modify active entries.
  - **Repository settings**: fills nil fields only from built-in defaults; never overwrites non-nil fields. This appends missing security settings with the `true` defaults and preserves explicit `false` values except for the automated security fixes prerequisite described above. `Description`, `Homepage`, and `Topics` are excluded from this merge because they are project-specific.
  - **Actions policy**: adds the complete default section when absent. Otherwise, it fills missing core fields without changing explicit values. It fills missing selected-action fields only when the effective policy is `selected`.
  - **Labels**: populated only when the labels section is entirely absent or empty (all-or-nothing). If the config already has any labels defined, no defaults are merged.
- For repository settings: if a `repository` section is present in `.tailor.yml`, reads the current repository settings via `GET /repos/{owner}/{repo}` and additional endpoints, compares each declared field against the live value, and applies changes via `PATCH /repos/{owner}/{repo}` plus separate API calls for fields with dedicated endpoints. Repository settings are the first API stage after local migration cleanup. If no GitHub repository context exists (no remote), repository settings are skipped with a warning. `--recut` has no special effect on repository settings - they are always applied declaratively.
- For Actions policy: after any default merge, if an `actions` section is present, reads the current policy, compares each declared field, and applies only endpoint groups that differ. The Actions policy runs after repository settings and before labels. If the section remains absent because default merging is disabled, Tailor makes no Actions policy calls. `--recut` has no other special effect.
- For labels: if a `labels` section is present in `.tailor.yml`, reads the current labels via paginated `GET /repos/{owner}/{repo}/labels`, diffs desired vs current using case-insensitive name matching, creates missing labels via `POST`, and updates changed labels (colour or description differs) via `PATCH`. Labels present on GitHub but absent from config are left untouched. Labels are applied after repository settings and before licences and swatches. If no GitHub repository context exists (no remote), labels are skipped with a warning.
- For `always` swatches other than `.tailor.yml`: compares the SHA-256 of the embedded swatch content against the on-disk file. Tailor overwrites the file if the hashes differ. For a token-bearing swatch configured as `always`, Tailor resolves `{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, or `{{SUPPORT_URL}}` before the comparison. The resolved content can produce `no change` when it matches the on-disk file. The token-bearing swatches are `.github/FUNDING.yml`, `SECURITY.md`, and `.github/ISSUE_TEMPLATE/config.yml`.
- For `first-fit` swatches: copies only if the destination file does not exist; never overwrites. If the destination exists, the swatch is skipped entirely - no SHA-256 comparison is performed.
- For `never` swatches: skips entirely. No file is written or compared.
- For licences: if `.tailor.yml` contains a `license` key with a value other than `none`, and no `LICENSE` file exists on disk, fetches the licence text via the GitHub REST API (`GET /licenses/{id}`) and writes it to `LICENSE`. The text is written verbatim as returned by GitHub - no token substitution is performed. Always treated as `first-fit`; the on-disk `LICENSE` file is never overwritten. If the licence fetch fails (e.g. unrecognised licence identifier), `alter` exits with the API error.
- For `.github/FUNDING.yml`: substitutes `{{GITHUB_USERNAME}}` before writing. `{{GITHUB_USERNAME}}` is resolved through `GET /user`. Tailor returns the API error if the request fails. The Sponsorships checkbox under Settings > General > Features is not exposed via the GitHub API. After alter places `.github/FUNDING.yml`, enable sponsorships manually in repository settings.
- For `SECURITY.md`: substitutes `{{ADVISORY_URL}}` before writing. `{{ADVISORY_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/security/advisories/new` from the repository context (owner/name). If no GitHub repository context exists (e.g. a brand-new project with no remote), `{{ADVISORY_URL}}` is left unsubstituted in the written file. `alter` will resolve and substitute it on a subsequent run once the repository has a remote.
- For `.github/ISSUE_TEMPLATE/config.yml`: substitutes `{{SUPPORT_URL}}` before writing. `{{SUPPORT_URL}}` is constructed at `alter` time as `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md` from the repository context (owner/name). If no GitHub repository context exists, `{{SUPPORT_URL}}` is left unsubstituted in the written file.
- With `--recut`: overrides `first-fit` with `always` semantics, but still skips `never` swatches. It overwrites a `first-fit` swatch file even if the file exists and has local modifications. Use with care. The licence file is exempt from `--recut` and is never overwritten, because it is fetched content, not an embedded swatch. For `.tailor.yml`, `--recut` enables the default merge when its mode is `first-fit`. Retired entries are removed, and missing defaults are appended. Other existing entries are never modified or overwritten. When `--recut` writes a token-bearing swatch, it resolves the token before writing.
- If no `license` key is present in `.tailor.yml` (or its value is `none`) and no `LICENSE` file exists in the project root, emits a warning: "No licence file found and no licence configured. Add `license: BlueOak-1.0.0` (or another identifier) to `.tailor.yml` and run `tailor alter`." Warning only; does not block execution.
- Creates intermediate directories as needed before writing any swatch whose destination path requires directories that do not yet exist.
- After the config stage, removes each retired workflow file that exists. Cleanup uses the fixed paths even when `.tailor.yml` does not list them. Cleanup does not depend on an alteration mode. An absent retired file produces no result.
- Keeps retired workflow paths rooted in the project directory. Tailor does not follow a symlink in a parent directory. For a symlink destination, Tailor removes the link and does not touch its target. For a directory destination, Tailor returns an error and removes no content. Tailor does not remove empty parent directories.
- Never touches files not listed in `.tailor.yml`, except for the two fixed retired workflow paths
- Modifies files only; does not commit or push

### `baste`

Previews what `alter` would do without making any changes.

`baste` verifies that a valid authentication token exists at startup and exits with an error if no token is available. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed.

```bash
tailor baste
```

Behaviour:
- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- Before strict validation, `baste` applies the same in-memory retired workflow migration and security prerequisite normalisation as `alter`. It accepts historical `triggered` entries only for the two retired paths. It emits the normalisation warning and reports `would update: .tailor.yml` without writing.
- `baste` performs the same comparison and file-safety checks as `alter` but writes and removes nothing. It reports what `alter` would do.

Output contract - repository settings and Actions policy settings are shown first, then labels, then file results. Actions policy fields use the `actions.` prefix. File results include the licence, the `.tailor.yml` default merge, and swatches. `baste` uses planned labels. `alter` and `alter --recut` use completed labels, and report each label only after the change succeeds. Informational and access-warning labels are the same for all three commands.

| Result | `baste` | `alter` and `alter --recut` |
|---|---|---|
| Repository setting differs | `would set` | `set` |
| Label is absent | `would create` | `created` |
| Label differs | `would update` | `updated` |
| `.tailor.yml` gains built-in defaults, loses retired entries, or normalises the security prerequisite | `would update` | `updated` |
| Retired workflow file exists | `would remove` | `removed` |
| Licence or swatch destination is absent | `would copy` | `copied` |
| Swatch destination is replaced | `would overwrite` | `overwritten` |
| Licence exists, or a `first-fit` swatch exists without `--recut` | `skipped: <path> (first-fit, exists)` | `skipped: <path> (first-fit, exists)` |
| Swatch mode is `never` | `skipped: <path> (mode never)` | `skipped: <path> (mode never)` |

Repository settings output uses the following categories:

`baste`:

```
would set:                           repository.has_wiki = false
no change:                           repository.allow_squash_merge (already true)
```

`alter` and `alter --recut`:

```
set:                                 repository.has_wiki = false
no change:                           repository.allow_squash_merge (already true)
```

`would set` - declared value differs from the live repository setting in `baste`.
`set` - `alter` or `alter --recut` changed the declared value.
`no change` - declared value matches the live repository setting.

Repository setting entries are sorted by category, with `would set` or `set` first, `no change` second, and `would skip` variants last. Entries are sorted lexicographically by field name within each category.

Label output uses the following categories:

`baste`:

```
would create:                              label.bug = #d20f39 "Something isn't working"
would update:                              label.documentation = #04a5e5 "Documentation improvement"
no change:                                 label.enhancement (already #1e66f5 "New feature request")
would skip (insufficient scope: <detail>): create label "bug"
```

`alter` and `alter --recut`:

```
created:                             label.bug = #d20f39 "Something isn't working"
updated:                             label.documentation = #04a5e5 "Documentation improvement"
no change:                           label.enhancement (already #1e66f5 "New feature request")
```

`would create` - label does not exist on GitHub in `baste`.
`created` - `alter` or `alter --recut` created the label.
`would update` - label exists on GitHub but colour or description differs from config in `baste`.
`updated` - `alter` or `alter --recut` updated the label.
`no change` - label exists on GitHub and matches config.
`would skip (insufficient scope: <detail>)` - operation could not be applied because the token lacks the required scope or permission. Use a token with the required repository permissions.

Label entries are sorted by category: `would create` or `created` first, `would update` or `updated` second, `no change` third, then `would skip` variants. Entries are sorted lexicographically by label name within each category.

File output uses the following categories:

`baste`:

```
would update:                               .tailor.yml
would remove:                               .github/workflows/tailor-automerge.yml
would remove:                               .github/workflows/tailor.yml
would copy:                                 LICENSE
would overwrite:                            SECURITY.md
no change:                                  CODE_OF_CONDUCT.md
skipped:                                    .envrc (first-fit, exists)
skipped:                                    .github/pull_request_template.md (mode never)
```

`alter`:

```
updated:                                .tailor.yml
removed:                                .github/workflows/tailor-automerge.yml
removed:                                .github/workflows/tailor.yml
copied:                                 LICENSE
overwritten:                            SECURITY.md
skipped:                                .envrc (first-fit, exists)
skipped:                                .github/pull_request_template.md (mode never)
```

`alter --recut` overwrites existing `first-fit` swatches, but it uses the same skip format for an existing licence and a `never` swatch:

```
overwritten:                            .envrc
skipped:                                LICENSE (first-fit, exists)
skipped:                                .github/pull_request_template.md (mode never)
```

`would update` - `baste` found built-in defaults to merge, retired workflow entries to remove, or a security prerequisite to normalise. Security normalisation also emits its warning. Multiple config changes produce one `.tailor.yml` result.
`updated` - `alter` or `alter --recut` wrote the changed `.tailor.yml` once. The write combines default merges, retired-entry removals, and security prerequisite normalisation.
`would remove` - a retired workflow file exists and `baste` would remove it. `baste` does not change the file.
`removed` - `alter` or `alter --recut` removed a retired workflow file. Tailor reports the result only after removal succeeds.
`would copy` - destination does not exist and the swatch would be written. Applies regardless of whether the swatch is `always` or `first-fit`.
`copied` - `alter` or `alter --recut` copied the licence or swatch.
`would overwrite` - `always` swatch whose embedded content differs from the on-disk file.
`overwritten` - `alter` or `alter --recut` overwrote the swatch.
`no change` - `always` swatch other than `.tailor.yml` whose resolved embedded content matches the on-disk file. Existing `first-fit` swatches always produce `skipped: <path> (first-fit, exists)`, never `no change`. A token-bearing swatch configured as `always` participates in the normal SHA-256 comparison after token resolution.
`skipped: <path> (first-fit, exists)` - `first-fit` swatch whose destination already exists. Tailor does not compare the content.
`skipped: <path> (mode never)` - swatch with `alteration: never`. Tailor skips the swatch in all three commands.

File results put actionable categories first: update, remove, copy, and overwrite. Informational categories follow: `no change`, then `skipped`. First-fit skip results precede never-mode skip results. Entries are sorted lexicographically by path within each category and reason. The planned or completed tense does not change this order.

The category label width is computed dynamically from the longest label in the full output, with a minimum of 37 characters. Access-warning annotations can increase the width.

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
- `warning` - health diagnostic that requires attention but is not a missing swatch. Two cases are recognised: `LICENSE` exists but contains known unresolved placeholder tokens (e.g. `[year]`, `[fullname]`, `{project}`), and `README.md` is absent from the project root. A warned path appears once in the output and does not also appear as `present`
- `present` - health file exists on disk
- `not-configured` - default swatch whose destination is not covered by any entry in `.tailor.yml`; the default swatch will not be applied until added
- `config-only` - swatch in `.tailor.yml` whose destination is not covered by any entry in the built-in default set. This arises when a swatch is removed from the built-in defaults in a newer tailor release but the project's `.tailor.yml` still references it. `alter` rejects unrecognised paths, except that it automatically migrates the two fixed retired workflow paths
- `mode-differs` - swatch whose destination appears in both `.tailor.yml` and the default set, but with a different alteration mode; the inline annotation shows both values

Output order: `missing`, `warning`, `present`, `not-configured`, `config-only`, `mode-differs`. Within each category, entries are sorted lexicographically by destination path. The category label is padded to a fixed width of 16 characters (the length of `not-configured: `) for consistent column alignment. For `warning` entries, the detail annotation (e.g. `(contains unresolved placeholders)`) is separated from the path by a single space, following the same annotation style as `mode-differs`. For `mode-differs` entries, the annotation (e.g. `(config: first-fit, default: always)`) is separated from the destination path by a single space; no additional fixed column alignment is applied to the annotation. Health file checks are always performed and reported regardless of whether `.tailor.yml` is present; config-diff categories (`not-configured`, `config-only`, `mode-differs`) are shown only when `.tailor.yml` is present.

`README.md` is a local health diagnostic, not a swatch or config-diff item. It is checked by exact path at the project root only. `README`, `README.rst`, and other variants do not satisfy the check. The `README.md` warning is not emitted when the file exists. Licence placeholder detection recognises only these names inside square or curly braces: `year`, `yyyy`, `fullname`, `name of copyright owner`, `name of copyright holder`, `software name`, `project`, `projecturl`, and `email`. Matching ignores case, allows ASCII whitespace beside the delimiters, and normalises each internal sequence of ASCII whitespace to one space. Arbitrary bracketed text, complete Markdown inline links, and angle-bracket application examples do not cause a warning. The check runs only when `LICENSE` exists on disk; an absent `LICENSE` stays in the `missing` category.

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
- `user` is resolved via `GET /user` if authenticated. It displays `(none)` if not authenticated.
- `repository` displays the `owner/repo` derived from the GitHub remote in the current directory; displays `(none)` if no GitHub remote exists.
- `auth` displays `authenticated` or `not authenticated` based on whether a valid token can be resolved for `github.com`.
- Does not read `.tailor.yml` and does not require it to be present.

## Error Handling

**Unrecognised swatch `path` in `.tailor.yml`**: `alter` and `baste` remove the two retired paths in memory before strict validation. Any other path must match an embedded swatch. The error identifies the unrecognised name and lists all valid paths. `alter` validates all remaining paths before a write or removal. An unknown non-retired path therefore causes no disk changes.

**Invalid alteration mode**: `always`, `first-fit`, and `never` are the only active modes. The historical `triggered` value is valid only on a retired workflow entry. This exception lets `alter` or `baste` remove that entry. Any other use causes a validation error before disk changes.

**Licence fetch failed**: if `GET /licenses/{id}` returns an error during `alter` (e.g. unrecognised licence identifier), tailor exits with the API error.

**Destination path not writable**: tailor exits with an error showing the full path that could not be written.

**Retired workflow path is unsafe**: if a parent component is a symlink, Tailor exits without following the path. If the destination is a directory, Tailor exits without removing content. If the destination is a symlink, Tailor removes only the link. A removal error stops later operations. Tailor reports `removed` only after a successful removal.

**`.tailor.yml` malformed or missing**: if `alter` or `baste` reads a missing or malformed `.tailor.yml`, it exits with a clear message directing the user to run `fit` to create a valid configuration, or edit `.tailor.yml` directly to correct it.

**`.tailor.yml` is not a valid config file**: Tailor rejects `.tailor.yml` if it is not a regular file or exceeds 1 MiB. The command exits before YAML parsing.

**`always` swatch modified locally**: for embedded swatches other than `.tailor.yml`, Tailor treats the file as changed whenever the SHA-256 of the resolved swatch content differs from the on-disk file. `alter` overwrites it unconditionally. Tailor does not preserve local edits to these `always` swatches; use `first-fit` alteration mode if local modifications must be retained after the initial fit. `--recut` overrides `first-fit` protection but still skips `never` swatches. The licence file is never overwritten regardless of flags. `.tailor.yml` uses no content hash: `always` removes retired entries and appends missing defaults. Other existing entries are never modified or overwritten.

**Duplicate path in `.tailor.yml`**: `alter` and `baste` remove retired entries before duplicate validation. If active entries share a path, the command identifies the conflict and exits before disk changes.

**Not authenticated**: if no valid authentication token can be resolved for `github.com` (neither `GH_TOKEN`/`GITHUB_TOKEN` environment variable, `gh` config file, nor `gh` keyring), `fit`, `alter`, and `baste` exit with: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run `gh auth login`."

**`{{GITHUB_USERNAME}}` resolution failed**: `{{GITHUB_USERNAME}}` is resolved via `GET /user`. If this call fails, `alter` exits with the API error. Unlike repo-context tokens, `{{GITHUB_USERNAME}}` depends on the authenticated user, not the repository, so it cannot be deferred.

**Repo-context tokens unresolved**: `{{ADVISORY_URL}}` and `{{SUPPORT_URL}}` require a GitHub repository context. If the project has no GitHub remote (e.g. a brand-new project not yet pushed), these tokens are left unsubstituted silently. For `always` swatches (e.g. `SECURITY.md`), `alter` will resolve and substitute them on a subsequent run once the repository has a remote. For `first-fit` swatches (e.g. `.github/ISSUE_TEMPLATE/config.yml`), delete the file and re-run `alter`, or use `--recut`.

**Repository settings without repo context**: if `.tailor.yml` contains a `repository` section but the project has no GitHub remote (no repository context found), repository settings are skipped with a warning: "No GitHub repository context found. Repository settings will be applied once a remote is configured." Warning only; does not block swatch or licence processing.

**Repository settings API failure**: if any API call to apply repository settings fails (PATCH, PUT, or DELETE), `alter` exits with the API error. Repository settings are the first API stage, so labels, licence, and swatch operations are not attempted. Local config migration and retired file cleanup already occurred. If licence fetch fails after repository settings and labels have been applied, those changes are not reverted.

**Repository settings with insufficient scope**: When GitHub rejects a repository-setting read or write with an access error, Tailor skips the affected fields rather than exiting. `baste` reports `would skip (insufficient scope: token missing required scope)` and `alter` skips the operation. Other repository settings continue to be applied. Use a token with the required repository permissions.

**Actions policy failure**: Access errors from Actions policy reads or writes produce `would skip (insufficient scope)` results for the affected fields or operation, unless a prior write changed the failure state. During the enabled `all` to enabled `selected` restriction, a hard first core failure leaves `all` active. A later selected-policy failure leaves `selected` active and preserves existing SHA pinning. A final SHA write failure leaves the selected restrictions and SHA pinning active. Each hard failure stops the command. After Tailor disables Actions for another transition to `selected`, a selected-policy or final core write failure stops the command and leaves Actions disabled. Other errors stop the command. An invalid `allowed_actions` value or selected-action combination stops validation before any write.

**Unrecognised repository setting**: if `.tailor.yml` contains a field in the `repository` section that is not in the supported settings list, `alter` exits with an error identifying the unrecognised field and listing all valid repository setting field names.

**`fit` repository settings query failed**: if `fit` detects a GitHub remote but the subsequent API call to read repository settings fails (e.g. insufficient permissions, network error), `fit` exits with the API error. The user can re-run `fit` after resolving the issue, or create `.tailor.yml` manually.

## Configuration

### `.tailor.yml`

`.tailor.yml` has five top-level sections: `license`, `repository`, `actions`, `labels`, and `swatches`. The `actions` section is a map of repository Actions policy settings. `path` values use the full path relative to `swatches/`, including the file extension where one exists. Extensionless files (e.g. `justfile`) are referenced as-is. The `repository`, `actions`, and `labels` sections can be absent in a hand-written config. Default merging adds missing Actions defaults before policy management.

Tailor opens `.tailor.yml` relative to the project root. It does not search parent directories. The config must be a regular file no larger than 1 MiB (1,048,576 bytes).

The active configuration has 16 swatches and three alteration modes: `always`, `first-fit`, and `never`. Two paths are retired migration entries: `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. `alter` and `baste` remove every matching entry before strict path, duplicate-path, and mode validation. The historical `triggered` mode is accepted only on these removed entries. Retired paths are not active swatches. Tailor never adds them to a generated or refitted config.

Default (with `--license=BlueOak-1.0.0`). The `license` key varies by flag (`MIT`, `Apache-2.0`, `none`, etc.) - the rest of the generated file is identical regardless of licence choice:

```yaml
# Initially fitted by tailor on 2026-03-02
license: BlueOak-1.0.0

repository:
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

actions:
  enabled: true
  allowed_actions: selected
  sha_pinning_required: false
  github_owned_allowed: true
  verified_allowed: true
  patterns_allowed:
    - freerangebytes/setup-actionlint@*
    - golang/govulncheck-action@*
    - golangci/golangci-lint-action@*
    - nick-fields/retry@*
    - robherley/go-test-action@*
    - softprops/action-gh-release@*

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
    alteration: never

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

  - path: cubic.yaml
    alteration: first-fit

  - path: .tailor.yml
    alteration: always
```

### Header comment

The first line of `.tailor.yml` is a header comment indicating when the config was created or last updated by Tailor.

- `# Initially fitted by tailor on <DATE>` - written by `fit` when the config is first created.
- `# Refitted by tailor on <DATE>` - written by `alter` when it removes retired workflow entries, normalises the security prerequisite, or merges built-in defaults. The date is the current date (YYYY-MM-DD). If the config does not change, the header does not change.

The `config.Write` function accepts a date string and a header verb. The template uses the verb to select between "Initially fitted" and "Refitted".

### Registry

No global registry. Projects are configured by the presence of `.tailor.yml`.

## Swatch Storage

Swatches are embedded in the tailor binary at build time from `swatches/`:

```
swatches/
├── .envrc
├── .gitignore
├── cubic.yaml
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── SECURITY.md
├── SUPPORT.md
├── flake.nix
├── justfile
├── .github/
│   ├── dependabot.yml  # includes GitHub Actions and Nix ecosystem updates
│   ├── FUNDING.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   ├── config.yml
│   │   └── feature_request.yml
│   └── pull_request_template.md
└── .tailor.yml
```

`.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically. `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically. If no GitHub repository context exists at `alter` time, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically. Resolution follows the same mechanism as `{{ADVISORY_URL}}` and constructs `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. `.github/dependabot.yml` covers the `github-actions`, `gomod`, and `nix` package ecosystems for automated dependency updates.

Licences are not embedded - they are fetched at `alter` time via the GitHub REST API (`GET /licenses/{id}`) and written verbatim to `LICENSE`.

## Retired Workflows

> [!IMPORTANT]
> After an upgrade, run `tailor baste` to preview cleanup, then run `tailor alter`. Tailor automatically removes both legacy config entries and files.

The retired paths are `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. Cleanup is unconditional. It applies to entries with any active mode and to historical `triggered` entries. Cleanup also removes a retired file when the config has no matching entry. `baste` reports the migration but changes nothing. `alter` and `alter --recut` write the cleaned config once, then remove each retired file that is present.

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

1. **Overwrite detection**: SHA-256 hash comparison between the embedded swatch content (from the tailor binary) and the on-disk target file. SHA-256 comparison applies only to `always` swatches; `first-fit` swatches are skipped entirely if the destination exists, with no comparison performed. The on-disk file is overwritten only when this comparison shows a difference. For a token-bearing swatch configured as `always`, Tailor resolves the token before the hash comparison. `.tailor.yml` uses append-only config merging instead of a content hash. `--recut` bypasses the hash comparison for `always` and `first-fit` swatches, but still skips `never` swatches.
2. **Interpolation (FUNDING.yml, SECURITY.md, and issue template config)**: Swatches are complete verbatim files with three exceptions. `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted at `alter` time from `GET /user`. `SECURITY.md` has `{{ADVISORY_URL}}` constructed from the repository context (owner/name). If no GitHub repository context exists, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` constructed from the repository context, which produces `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. If no GitHub repository context exists, the token is left unsubstituted. No per-swatch configuration is required. Licences are fetched via `GET /licenses/{id}` and written verbatim. Licences do not use token substitution.
3. **No versioning**: No swatch versions, always uses swatches from current tailor binary. Upgrading tailor will cause all `always` swatches to be re-evaluated against the new embedded content; files whose swatch content has changed will be overwritten on the next `alter` run.
4. **No global state**: All state is per-project in `.tailor.yml`
5. **No project registry**: Tailor has no awareness of its consumers. Projects pull from tailor, tailor does not track projects.
6. **Authentication via `go-gh`**: All project metadata, user metadata, licence content, and repository settings are resolved via `go-gh` (`github.com/cli/go-gh/v2`), the official Go library for GitHub CLI extensions. Token resolution follows the `go-gh` precedence order: `GH_TOKEN` environment variable, `GITHUB_TOKEN` environment variable, `gh` config file, `gh` keyring (via the `gh` binary). When `GH_TOKEN` or `GITHUB_TOKEN` is set, the `gh` binary is not required. The `gh` binary is needed only for `gh auth login` (establishing credentials) and as a fallback for keyring-based token access when no environment variable is set. Repository context detection reads git remotes via `go-gh`, so `git` must be present when a GitHub remote exists - but any directory with a GitHub remote already has `git` installed. If no valid token can be resolved, `fit`, `alter`, and `baste` exit immediately with an error.
7. **CLI parsing**: [Kong](https://github.com/alecthomas/kong) is used as the command line parser.
8. **Repository settings via API**: Repository settings are applied via `PATCH /repos/{owner}/{repo}` with a JSON body constructed from the `repository` section of `.tailor.yml`, plus separate API calls for security features, topics, and Actions workflow permissions. The top-level `actions` section uses the Actions permissions and selected-actions endpoints. Field names map directly to the GitHub REST API without translation. Current settings are read via `GET /repos/{owner}/{repo}` and the relevant separate endpoints for `baste` comparison. All API calls use `go-gh`'s pre-authenticated REST client.
9. **Execution order**: after authentication and config parsing, `alter` removes retired entries in memory. It then normalises the security prerequisite and emits its warning before validation. Next, it writes the changed config once and removes present retired workflow files. It then applies repository settings, Actions policy, labels, the licence, and active swatches in that order. `baste` uses `DryRun`; it performs the same planning and validation but writes and removes nothing. `alter` uses `Apply`, and `alter --recut` uses `Recut`.
