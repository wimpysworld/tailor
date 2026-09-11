# Configuration

Use `.tailor.yml` to choose the files and GitHub settings that Tailor manages.

[Swatches](#swatches) · [Licences](#licences) · [Default set](#default-swatch-set) · [Modes](#alteration-modes) · [Default merging](#default-merging) · [Config example](#config-file)

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

## Licences

Licences are not swatches. Choose an identifier supported by the [GitHub licences API](https://docs.github.com/en/rest/licenses/licenses#get-a-license), or `none` to skip licence creation.

`fit` records the identifier without checking it. `baste` does not fetch the licence body, so a successful preview does not confirm licence availability.

When `LICENSE` is absent, `alter` fetches `GET /licenses/{id}` and writes the returned text verbatim. Fill in copyright names, years, and other placeholders manually. Tailor does not substitute licence tokens.

An existing regular `LICENSE` remains unchanged, even after you change `license:` or use `--recut`. Edit or replace that file yourself when you change licences.

If the licence fetch fails, check the identifier and API access. Earlier changes remain. Correct `.tailor.yml`, then rerun `tailor baste` and `tailor alter`.

## Default swatch set

Tailor embeds 25 default swatches:

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

Config merging differs from ordinary file replacement. Existing wiki and static Pages starter files also have [recut exceptions](Commands#alter).

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
| `pages` | Adds missing defaults, with `enabled: false`. Does not add personal `links` defaults. |
| `license`, `immutable_releases`, `variables` | Does not add defaults. |

Omitting a managed section does not stop management when merging restores that section. Set `.tailor.yml` to `never`, or omit its swatch entry, to disable default merging. The merge never restores its own `.tailor.yml` entry.

[Retired-entry cleanup](Commands#retired-workflow-cleanup) and [security prerequisite normalisation](Repository-settings#repository-fields) still apply when default merging is disabled. Security normalisation can change explicit values and reports a warning.

## Config file

All state lives in `.tailor.yml`. Its eleven sections are `license`, `repository`, `immutable_releases`, `actions`, `code_scanning`, `code_quality`, `ruleset`, `labels`, `variables`, `pages`, and `swatches`.

Tailor opens `.tailor.yml` relative to the project root. The config must be a regular file no larger than 1 MiB.

```yaml
# Initially fitted by tailor on 2026-03-04
license: BlueOak-1.0.0

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
| Wiki files and `repository.has_wiki` | [GitHub wiki](GitHub-wiki) |

See [Commands](Commands) for licence flags, file checks, and the shipped [justfile recipes](Commands#justfile-recipes).
