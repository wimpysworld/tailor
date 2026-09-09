# Tailor 👔

Ready-to-wear project templates for GitHub repositories. Tailor is a local terminal CLI that fits projects with community health files, security policy, dev tooling, and repository settings.

If you manage multiple projects across different GitHub organisations and find that configurations keep drifting out of sync, Tailor fixes that. It is opinionated by design - built for solo devs and small teams who want consistent, well-maintained repositories without the overhead.

## Install

### bin

```bash
bin install github.com/wimpysworld/tailor
bin update tailor
```

Requires [`bin`](https://github.com/marcosnils/bin). Tailor releases publish bare executables, no archive extraction needed.

### Homebrew

```bash
brew install wimpysworld/tap/tailor
```

### Nix

```bash
nix run github:wimpysworld/nix-packages#tailor -- --version
nix profile install github:wimpysworld/nix-packages#tailor
```

To use tailor in a flake configuration, add `nix-packages` as an input:

```nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    wimpysworld-nix-packages = {
      url = "github:wimpysworld/nix-packages";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

Then reference tailor in your packages:

```nix
environment.systemPackages = [
  inputs.wimpysworld-nix-packages.packages.${system}.tailor
];
```

Available for `x86_64-linux`, `aarch64-linux`, and `aarch64-darwin`.

### Docker

```bash
docker run --rm ghcr.io/wimpysworld/tailor --version
```

Images are published to GHCR for `linux/amd64` and `linux/arm64`. Mount your project directory and pass a GitHub token:

```bash
docker run --rm \
  -v "$PWD":/work -w /work \
  -e GH_TOKEN \
  ghcr.io/wimpysworld/tailor alter
```

### Native packages

Releases include `.deb`, `.rpm`, `.apk`, and Arch Linux packages. Download the appropriate file from the [latest release](https://github.com/wimpysworld/tailor/releases/latest) and install with your system package manager. The AUR package is [`tailor-bin`](https://aur.archlinux.org/packages/tailor-bin).

### Authentication

Tailor needs a valid GitHub authentication token for `fit`, `alter`, and `baste`. Set `GH_TOKEN` or `GITHUB_TOKEN` to use Tailor without the `gh` binary.

Alternatively, install the [GitHub CLI](https://cli.github.com/) and run `gh auth login`. Tailor can then read the token from the `gh` config file or keyring. The `measure` and `docket` commands do not require authentication.

## Quick Start

### New project

```bash
tailor fit ./my-project
cd my-project
tailor alter
```

`fit` creates the directory and writes `.tailor.yml` with the full default swatch set. `alter` copies the files and applies repository settings. The default licence is BlueOak-1.0.0.

```bash
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

### Existing project

```bash
cd existing-project
tailor measure                # See what's missing
tailor fit .                  # Create .tailor.yml
tailor alter                  # Apply swatches and settings
```

`measure` checks which community health files are present, missing, or need attention. It warns when `README.md` is absent or when `LICENSE` contains a known unresolved placeholder. The licence check recognises `year`, `yyyy`, `fullname`, `name of copyright owner`, `name of copyright holder`, `software name`, `project`, `projecturl`, and `email` inside square or curly braces. Matching ignores case, allows ASCII whitespace beside the delimiters, and treats each internal sequence of ASCII whitespace as one space. Other bracketed licence text and complete Markdown inline links do not cause a warning. `fit .` works in an existing directory without error. If the project has a GitHub remote, `fit` reads the live repository settings so it preserves anything already configured.

Edit `.tailor.yml` to add swatches or change alteration modes, then run `alter`. Set `alteration: never` on any swatch you want tailor to skip.

## Core Concepts

### Swatches

Swatches are complete template files embedded in the tailor binary. Most are copied verbatim. Three have tokens substituted at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | Authenticated user from `GET /user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | GitHub repository context |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | GitHub repository context |

Licences are not swatches. They are fetched from the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

### Default swatch set

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

### Alteration modes

- **`always`** - Overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - Copies the file only if it does not already exist. Never overwrites. Use this for files you intend to customise after initial delivery.
- **`never`** - Skips the file entirely. Use this to keep a swatch visible in the config without managing its destination.

### Configuration

All state lives in `.tailor.yml`. Its eleven sections are `license`, `repository`, `immutable_releases`, `actions`, `code_scanning`, `code_quality`, `ruleset`, `labels`, `variables`, `pages`, and `swatches`.

Release immutability defaults to `immutable_releases.enabled: false`. Before enabling it, change release CI to upload every asset to a draft, then publish. Workflows that upload or replace assets after publication will fail. Enabling protects future releases only. Disabling does not unlock existing immutable releases. Tailor skips disabling when the repository owner enforces immutability. An omitted section or `enabled` key stays unmanaged, including during default merging. For an existing repository, `fit` preserves the live setting.

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
| `path` | File path relative to the project root (also matches the swatch name in the binary) |
| `alteration` | `always`, `first-fit`, or `never` |

Set `alteration: never` to stop tailor managing a file. The entry stays visible in `.tailor.yml` and prevents `alter --recut` from re-adding it.

## Repository Settings

The `repository` section manages GitHub repository settings declaratively. Field names match the [GitHub REST API](https://docs.github.com/en/rest/repos/repos#update-a-repository) exactly (snake_case). Tailor uses the repository endpoint and separate feature endpoints on every `alter` run.

| Field | Type | Description |
|-------|------|-------------|
| `description` | string | Repository description |
| `homepage` | string | Repository homepage URL |
| `has_wiki` | bool | Enable wiki |
| `has_discussions` | bool | Enable discussions |
| `has_projects` | bool | Enable projects |
| `has_issues` | bool | Enable issues |
| `allow_merge_commit` | bool | Allow merge commits |
| `allow_squash_merge` | bool | Allow squash merging |
| `allow_rebase_merge` | bool | Allow rebase merging |
| `squash_merge_commit_title` | string | `PR_TITLE` or `COMMIT_OR_PR_TITLE` |
| `squash_merge_commit_message` | string | `PR_BODY`, `COMMIT_MESSAGES`, or `BLANK` |
| `merge_commit_title` | string | `PR_TITLE` or `MERGE_MESSAGE` |
| `merge_commit_message` | string | `PR_TITLE`, `PR_BODY`, or `BLANK` |
| `delete_branch_on_merge` | bool | Delete branch on merge |
| `allow_update_branch` | bool | Allow updating PR branches |
| `allow_auto_merge` | bool | Allow auto-merge |
| `web_commit_signoff_required` | bool | Require sign-off on web commits |
| `private_vulnerability_reporting_enabled` | bool | Enable private vulnerability reporting |
| `vulnerability_alerts_enabled` | bool | Enable Dependabot vulnerability alerts |
| `automated_security_fixes_enabled` | bool | Enable Dependabot security update pull requests |
| `topics` | string[] | Repository topics for discoverability |
| `default_workflow_permissions` | string | `GITHUB_TOKEN` default permissions: `read` or `write` |
| `can_approve_pull_request_reviews` | bool | Allow `GITHUB_TOKEN` workflows to create pull requests and submit approval reviews |
| `secret_scanning` | string | Secret scanning alerts (`enabled` or `disabled`) |
| `secret_scanning_push_protection` | string | Secret scanning push protection (`enabled` or `disabled`) |
| `secret_scanning_non_provider_patterns` | string | Secret scanning generic patterns (`enabled` or `disabled`) |

To skip settings management, omit the `repository` section and set the `.tailor.yml` swatch to `alteration: never`. With `always`, `alter` restores the section from built-in defaults and applies it in the same run. With `first-fit`, `alter --recut` restores and applies the section.

Generated configs expose all six security settings, three Boolean and three string. The built-in values are `true` for the Boolean settings and `enabled` for `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns`. When a GitHub remote exists, `fit` uses live values. Default merging appends missing security settings without changing explicit values. The security prerequisite normalisations below are the exception: they can change an explicit `vulnerability_alerts_enabled: false` to `true`, and an explicit `secret_scanning: disabled` to `enabled`.

GitHub labels `can_approve_pull_request_reviews` as “Allow GitHub Actions to create and approve pull requests”. Tailor keeps the REST API field name because repository config keys match the API. Enabling the setting permits the repository `GITHUB_TOKEN` to create pull requests and submit approval reviews when the workflow has `pull-requests: write`. The setting does not permit merges, bypass branch rules, or affect personal access tokens or separate GitHub App tokens.

GitHub requires vulnerability alerts before automated security fixes. When automated fixes are enabled and alerts are absent or false, Tailor sets `vulnerability_alerts_enabled` to `true` and shows a warning. `alter` and `alter --recut` save the corrected `.tailor.yml` before repository API calls. `baste` previews the config update without writing. Tailor enables alerts first and disables automated fixes before alerts. If a prerequisite read is unknown, or its write fails or is skipped, Tailor skips the dependent write. A security `404` stays unknown unless Tailor can distinguish a disabled feature from denied access. Access failures produce warnings. Other API failures stop the command.

Push protection requires secret scanning. When `secret_scanning_push_protection` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` and shows a warning. The write path matches the automated security fixes prerequisite. Tailor sends only the declared `security_and_analysis` keys, so other keys keep the value set in the GitHub UI. When the token lacks admin access, GitHub omits the `security_and_analysis` block, and Tailor leaves all three values unknown with an access warning.

`secret_scanning_non_provider_patterns` turns on the generic patterns that GitHub maintains, such as private keys, database connection strings, and HTTP authorisation headers. The GitHub UI calls the feature "Generic patterns", and the API key keeps the older name "non-provider patterns". There is nothing to configure beyond the toggle. Custom patterns need a Secret Protection licence and stay out of scope. Alerts from generic patterns have lower confidence than alerts from provider patterns. Push protection does not block them, so they raise alerts only. The feature is free on a public repository with secret scanning enabled. Generic patterns require secret scanning. When `secret_scanning_non_provider_patterns` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` and shows a warning, on the same write path as push protection.

## Actions Policy

The top-level `actions` section manages the repository Actions policy. Generated configs enable Actions, allow all actions and reusable workflows, and disable SHA pinning. No allow-list maintenance is required.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable GitHub Actions for the repository |
| `allowed_actions` | string | Allow `all`, `local_only`, or `selected` actions and reusable workflows |
| `sha_pinning_required` | bool | Require actions to use full-length commit SHAs |
| `github_owned_allowed` | bool | Allow GitHub-owned actions when `allowed_actions` is `selected` |
| `verified_allowed` | bool | Allow actions from verified creators when `allowed_actions` is `selected` |
| `patterns_allowed` | string[] | Complete set of allowed action and reusable workflow patterns |
| `artifact_and_log_retention.days` | integer | Opt-in retention for new artifacts and logs, from 1 to 90 days, within the live owner cap |
| `fork_pr_contributor_approval.approval_policy` | string | Contributors whose fork pull request workflows require approval. Defaults to `first_time_contributors` |

The selected-action fields require `allowed_actions: selected`. A selected policy must include `github_owned_allowed`, `verified_allowed`, and `patterns_allowed` after default merging. The `patterns_allowed` field replaces the full GitHub list. Tailor ignores list order during comparison. Default merging adds a missing `actions` section and fills missing fields without changing explicit values. For `all` and `local_only`, Tailor does not add selected-only fields. For existing `selected` configs, Tailor keeps the policy and adds each missing selected-only field. A missing `patterns_allowed` field receives the six compatibility defaults listed in the [specification](docs/SPECIFICATION.md). Tailor preserves an explicit custom list or `patterns_allowed: []`.

To switch an existing config, set `actions.allowed_actions: all`. Remove `github_owned_allowed`, `verified_allowed`, and `patterns_allowed` from the `actions` section, then run `tailor alter`. SHA pinning is a separate choice. Tailor does not change an explicit `selected` policy automatically.

The default for `sha_pinning_required` is `false`. Default merging preserves explicit `true` and `false` values, including under `--recut`. To enable SHA pinning in an existing config, set `actions.sha_pinning_required: true`, then run `tailor alter`.

Fork pull request approval controls which external contributors need approval before their workflows run:

| Policy | Approval required for |
|---|---|
| `first_time_contributors_new_to_github` | First-time contributors who are new to GitHub |
| `first_time_contributors` | All first-time contributors (the Tailor default) |
| `all_external_contributors` | All external contributors |

`fit` writes the active `first_time_contributors` default. Default merging adds it when the approval object or policy is absent or null. An empty approval object also receives the default. Explicit policies remain unchanged, including under `--recut`. This default applies with `all`, `local_only`, and `selected`.

For existing `.tailor.yml` files, default merging runs with `alteration: always`, or `first-fit` with `--recut`. It does not run for `never` or an absent `.tailor.yml` swatch entry. Without a merge, an absent or null approval policy stays unmanaged and causes no approval API calls. Omission alone does not disable management when a merge restores the default.

`baste` reports the requested policy without writes. `alter` updates a different live policy and makes no approval write when it already matches. Approval uses the separate `/repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval` GET/PUT endpoint. The PUT body contains only `approval_policy`. Approval alone requires no other Actions settings. It does not change workflow token permissions or private-fork permissions.

Denied or unavailable approval reads, and unknown live policies, produce `would skip (insufficient scope)` and no approval write. Access failures affect only their endpoint group. Other API errors stop the command.

To manage artifact and log retention, add this field to your `actions` section:

```yaml
actions:
  artifact_and_log_retention:
    days: 30
```

Retention alone requires no core or selected-action fields. Tailor never adds a retention default during `fit`, default merging, or `alter --recut`. An omitted `days` field causes no retention API calls. Other Actions defaults still follow the merge rules above.

Tailor validates the integer range before any changes. It then checks GitHub's live `maximum_allowed_days` cap and rejects a higher value with the allowed maximum in the error. Denied or unavailable reads stay unknown and cannot cause a retention write. Missing or invalid live values stop the command.

`baste` reports the declared retention value when it differs, without writes. `alter` sends one update for a difference and none for a match. Tailor uses the separate [retention GET/PUT endpoint](https://docs.github.com/en/rest/actions/permissions#set-artifact-and-log-retention-settings-for-a-repository) and sends only `days` in the PUT body. Changes affect only new artifacts and logs, not existing artifacts, logs, caches, or owner policy.

For an enabled transition from `all` to an enabled `selected` policy, Tailor first changes `allowed_actions` to `selected`. This write keeps SHA pinning enabled when the final policy disables it. Tailor then applies the complete selected rules and disables SHA pinning in a final core write. When the final policy keeps or enables SHA pinning, the first core write applies that value and Tailor omits the final write. A hard first-write failure leaves `all` active. A selected-rule failure leaves the narrower `selected` policy active and preserves SHA pinning. A final SHA write failure leaves the selected rules and SHA pinning active. Each hard failure stops the command.

For other transitions from `all` or `local_only`, Tailor disables Actions before it applies the complete selected rules and the final core policy. For an existing selected policy, Tailor applies changed selected rules before any core broadening, including disabling SHA pinning. When an enabled policy combines selected broadening with SHA pinning or disabling Actions, Tailor disables Actions before both updates. Broadening means newly allowing GitHub-owned actions, verified actions, or patterns. Tailor also disables Actions before any selected update whose final policy disables Actions. A later update failure leaves Actions disabled and stops the command. If Tailor cannot read an active selected policy, it does not enable Actions or disable SHA pinning. Organisation policy can restrict repository values. Other access failures produce skip results, and other API failures stop the command.

## Code Scanning

The top-level `code_scanning` section manages CodeQL default setup. Generated configs enable default setup with the default query suite, the remote threat model, and GitHub language detection.

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | `configured` or `not-configured` |
| `query_suite` | string | `default` or `extended` |
| `threat_model` | string | `remote` or `remote_and_local` |
| `languages` | string[] | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `actions`, `c-cpp`, `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby`, `swift` |

An empty `languages` list sends no `languages` field, so GitHub detects the languages on enable and keeps the current set afterwards, unlike `topics`, where an empty list clears all topics. Tailor sends only the fields it manages, so `runner_type` and `runner_label` keep the value set in the GitHub UI, and Tailor exposes no setting that needs a paid plan, an Advanced Security licence, or a self-hosted runner. A validation run in progress reports `would skip (setup in progress)`, and an unavailable feature reports `would skip (not available)`. Default setup does not conflict with workflows that upload SARIF results, such as Scorecard.

## Code Quality

The top-level `code_quality` section manages GitHub Code Quality. Generated configs leave Code Quality not configured with GitHub language detection.

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | `configured` or `not-configured` |
| `languages` | string[] | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby` |

An empty `languages` list sends no `languages` field, so GitHub detects the languages and keeps the current set afterwards. Tailor sends only the fields it manages, so `ai_findings_option`, `runner_type`, and `runner_label` keep the value set in the GitHub UI, and Tailor never spends AI credit on a public repository. The skip results match code scanning.

## Ruleset

The top-level `ruleset` section manages one branch ruleset named `Tailor`. Tailor owns that ruleset entirely: every write sends the complete ruleset, so a rule, bypass actor, or condition added by hand to the `Tailor` ruleset is removed on the next `alter` run. Tailor never deletes the ruleset and never reads or writes any other ruleset. Set `enforcement: disabled` to keep the `Tailor` ruleset on GitHub while a ruleset made in the GitHub UI governs the repository instead. Generated configs reproduce the GitHub UI default ruleset: restrict deletions, block force pushes, and require a pull request with one approval on the default branch, bypassed by the repository admin role.

| Field | Type | Description |
|-------|------|-------------|
| `enforcement` | string | `active` or `disabled`. `disabled` keeps the ruleset on GitHub, and GitHub ignores it. `evaluate` is Enterprise only and is rejected |
| `bypass_actors` | list | Complete set of bypass actors. Each entry has `actor_id` (integer), `actor_type` (`RepositoryRole`, `Team`, `User`, `Integration`, or `DeployKey`), and `bypass_mode` (`always`, `pull_request`, or `exempt`). `actor_id` is null for `DeployKey`. `RepositoryRole` ids are `2` maintain, `4` write, `5` admin. An empty list means no bypass |
| `conditions.ref_name.include` | string[] | Branch names, `refs/heads/` fnmatch patterns, `~DEFAULT_BRANCH`, or `~ALL`. At least one entry |
| `conditions.ref_name.exclude` | string[] | Branch names or `refs/heads/` fnmatch patterns. `~DEFAULT_BRANCH` and `~ALL` are rejected |
| `rules.creation` | bool | Restrict creations |
| `rules.update` | bool | Restrict updates. The `update_allows_fetch_and_merge` parameter is not managed |
| `rules.deletion` | bool | Restrict deletions |
| `rules.required_linear_history` | bool | Require linear history. GitHub requires squash or rebase merging to be allowed, so validation rejects `true` with an enabled pull request rule whose `allowed_merge_methods` is `merge` only |
| `rules.required_signatures` | bool | Require signed commits |
| `rules.non_fast_forward` | bool | Block force pushes |
| `rules.pull_request.enabled` | bool | Require a pull request before merging |
| `rules.pull_request.parameters` | object | `required_approving_review_count` (0 to 10), `dismiss_stale_reviews_on_push`, `require_code_owner_review`, `require_last_push_approval`, `required_review_thread_resolution`, `require_extra_approval_for_unattributed_changes` (bools), and `allowed_merge_methods` (any combination of `merge`, `squash`, `rebase`, at least one) |
| `rules.required_status_checks.enabled` | bool | Require status checks to pass |
| `rules.required_status_checks.parameters` | object | `strict_required_status_checks_policy` and `do_not_enforce_on_create` (bools), and `required_status_checks`, a list of `context` (check name, not empty) and optional `integration_id` (positive integer) |
| `rules.code_scanning.enabled` | bool | Require code scanning results before merging. At least one tool when `true` |
| `rules.code_scanning.parameters` | object | `code_scanning_tools`, a list of `tool` (tool name as GitHub shows it, for example `CodeQL`, not empty, unique), `alerts_threshold` (`none`, `errors`, `errors_and_warnings`, or `all`), and `security_alerts_threshold` (`none`, `critical`, `high_or_higher`, `medium_or_higher`, or `all`). All three keys are required per entry |

Each key in the `rules` map is a GitHub rule type. The `enabled` key on `pull_request`, `required_status_checks`, and `code_scanning` is Tailor's own, because these three rules carry parameters that stay in the config while the rule is off. `required_status_checks` is off by default with an empty list, because a required check that never reports blocks every merge. Enable it per repository and name an aggregating job, for example one that depends on every other job and fails when any of them failed.

The `code_scanning` rule is the free route to the "Check runs failure threshold" merge gate on the Advanced Security settings page. That page setting has no repository API. The rule blocks a merge until the tool has results for both the pull request commit and the base branch, so a new repository has no results yet. `code_scanning` is therefore off by default with one entry, `CodeQL` at `errors` and `high_or_higher`, which matches the GitHub UI defaults "Only errors" and "High or higher". Turning the gate on is a one-line change to `enabled`. Tailor does not cross-check the rule against the top-level `code_scanning.state`, because advanced setup also reports as the `CodeQL` tool while default setup stays `not-configured`.

GitHub blocks a merge when the ruleset allows a method that the repository disables. When `allowed_merge_methods` names a method whose `repository` setting is `false` in the same config, Tailor shows `warning: ruleset allows <method> merging but repository.<field> is false` and continues without changing either value. When rulesets are not available to the repository, Tailor reports `would skip (not available)`. When the token lacks write access to the ruleset, Tailor reports `would skip (insufficient scope)`. When GitHub rejects the ruleset body, Tailor stops with the API error.

## Actions variables

The optional `variables` section manages non-secret variables in the repository. Tailor creates or updates only declared variables. It leaves other variables unchanged.

```yaml
variables:
  - name: DEPLOY_REGION
    value: "eu-west-2"
  - name: RELEASE_SUFFIX
    value: ""
```

Names accept ASCII letters, digits and underscores. A name cannot start with a digit or `GITHUB_`. Names match without case sensitivity. Tailor rejects duplicate names. A difference in name casing alone causes no update.

Each entry requires a string `value`. Quote values such as `"true"` and `"123"` to keep them strings. An explicit `""` sets an empty value. Tailor preserves whitespace, newlines and Unicode without substitution.

Tailor accepts up to 500 declarations and 48 × 1024 UTF-8 bytes per value. GitHub enforces total repository capacity. GitHub also limits organisation and repository variables to 256 KB combined per workflow run.

Omit `variables` or use `variables: []` to make no variable requests. `fit` writes only a commented example. It never reads live variables. Default merging and `alter --recut` preserve declarations without adding variables.

Tailor does not manage secrets, organisation variables or environment variables. `measure` stays local.

`baste` shows `variable.<name>` with quoted, escaped values and makes no writes. `alter` reports each successful create or update. Access failures skip the affected variables. Rate limits and other hard errors stop the command. After partial writes, the error includes applied and remaining counts.

## GitHub Pages

Tailor configures GitHub Pages for public repositories, with a deployment workflow and the `github-pages` environment. Pages is disabled by default.

For static Pages, Tailor can create a starter site. Hugo and Jekyll need existing source files. Edit these fields in `.tailor.yml`:

```yaml
pages:
  enabled: true           # Default: false. Omission leaves Pages unmanaged.
  generator: static       # static, hugo, or jekyll. Default: static.
  path: pages             # Existing source directory, relative to the project.
  # branch: main          # Omit to follow the current repository default branch.
  # cname: www.example.com # Omit to preserve the domain. "" clears it.
```

Run `tailor baste` to preview, then `tailor alter` to apply. Commit the source and generated workflow to the selected branch to deploy.

| Generator | Required source | Build |
|---|---|---|
| `static` | `index.html` | Uploads the source without changing URLs. Use URLs that support the deployment base path. |
| `hugo` | Recognised Hugo configuration and local themes or pinned modules/submodules | Hugo Extended 0.165.0 writes `<path>/public`. Modules require `go.mod` and matching `go.sum` entries. |
| `jekyll` | `_config.yml`, `Gemfile`, and a complete `Gemfile.lock` with Jekyll 4.4.1 | Ruby 3.3.12 builds `<path>/_site`. |

The default source path is `pages`. Symlinks in the source or its parents are rejected. Hugo module replacements are unsupported. Jekyll path dependencies must stay inside the source, and Git dependencies require pinned commits. Tailor does not create a site or add Node, Sass, or custom build commands.

The workflow uses pinned actions and GitHub-hosted runners. Pushes deploy only the selected literal branch, never pull requests. Hugo and Jekyll use GitHub's Pages metadata for project prefixes and custom domains. The effective Actions policy must allow the required actions, including `ruby/setup-ruby` for Jekyll. Tailor reports blocked actions without bypassing the policy or changing repository-wide workflow permissions.

> [!IMPORTANT]
> Pages currently requires a classic token with `repo` scope. Use a repository administrator account to permit environment creation or branch-policy updates. Tailor skips Pages with `insufficient scope` when it cannot prove permissions, including with fine-grained tokens.

The workflow path is `.github/workflows/tailor-pages.yml`. Tailor refuses an existing file without the first-line marker `# Managed by Tailor: pages`, even with `--recut`.

| Workflow mode | Behaviour |
|---|---|
| `always` | Creates or updates the marked workflow for the selected generator, path and branch. |
| `first-fit` | Creates a missing workflow. Preserves an existing compatible workflow, unless `--recut` applies. |
| `never` | Requires an existing compatible workflow and never writes it. |

A protected workflow must match the generated YAML semantics. Comments and formatting can differ, but changes to execution or permissions block setup.

Set `cname` to a domain without a scheme or path. No `CNAME` file is required. DNS and account-level domain verification remain manual. For subdomains, point the DNS CNAME to `<owner>.github.io`, without a repository path. For apex domains, follow [GitHub's DNS instructions](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site). Complete any required TXT verification in account or organisation Pages settings. Tailor enables HTTPS when the certificate is ready. If DNS, verification or the certificate is pending, complete the reported step and rerun `tailor alter`.

An explicit `repository.homepage`, including `""`, wins. Otherwise, Tailor replaces only a live homepage that points to this repository's GitHub URL. The inline comment `# tailor: inferred homepage <URL>` identifies an inferred config value. Remove that comment, or edit `repository.homepage`, to make the value explicit. Tailor updates the homepage only after successful Pages setup.

For Hugo and Jekyll, Tailor appends the output directory to `.gitignore`, unless its swatch mode is `never`. Existing text stays unchanged, and tracked files stay tracked. Static adds no ignore rule.

Omitting `pages` or setting `enabled: false` stops Pages management without deleting the site, workflow or environment. Default merging and `--recut` never enable Pages.

### Static Pages links

`pages.links` manages optional connection icons for `generator: static` only. Non-empty values with Hugo or Jekyll are validation errors. An omitted generator means static.

```yaml
pages:
  enabled: true
  generator: static
  links:
    website: https://example.com
    mastodon: https://fosstodon.org/@example
    email: hello@example.com
    feed: https://example.com/feed.xml
```

The supported keys are `website`, `x`, `bluesky`, `mastodon`, `discord`, `matrix`, `slack`, `linkedin`, `youtube`, `twitch`, `podcast`, `instagram`, `pixelfed`, `tiktok`, `peertube`, `steam`, `itchio`, `patreon`, `github_sponsors`, `kofi`, `forum`, `email`, and `feed`. Use full HTTPS URLs without credentials. `email` takes a bare address without mail headers. Icons follow the key order in `pages.links`. Tailor preserves this order when it writes the config. Empty strings add no icon. Unknown keys, nulls and non-string values are rejected.

Put these markers on separate lines inside the footer of `<pages.path>/index.html`:

```html
<!-- tailor:links:start -->
<!-- tailor:links:end -->
```

Tailor owns only the content between the markers. It replaces that content with labelled icon links and preserves all other bytes, including under `--recut`. Missing, repeated, reversed or inline markers stop preflight before local or remote writes. The input must be a regular file no larger than 1 MiB, with no symlinked parents. The output also stays within that limit.

Omit `links` to leave the page untouched. Use `links: {}` or all-empty values to clear the marked section. New configs include Martin Wimpress’s published links from <https://wimpysworld.link/>. Services without a matching personal URL stay empty. Default merging never adds these links to existing configs and preserves explicit declarations, including an empty mapping. Disabled Pages leaves links unmanaged. A skipped Pages setup also skips the links update. `baste` reports `would overwrite` for a changed page without writing; `alter` reports `overwritten`. Repeated application makes no change. An empty mapping with Hugo or Jekyll has no effect.

The generated section includes its own minimal layout and CSS masks, so it needs no JavaScript or project-specific icon classes. It uses pinned Simple Icons 16.30.0 and Octicons 19.36.0 CDN URLs. Slack and LinkedIn use generic organisation and briefcase icons. Other service links use brand icons; website, podcast, forum, email and feed use generic icons. GitHub navigation stays separate from these optional connections.

### Static Pages starter

When Pages is enabled with `generator: static`, Tailor creates a starter in a missing or empty `pages.path` (default `pages`). The four embedded sources are `pages/index.html`, `pages/style.css`, `pages/theme.js` and `pages/icon.svg`. Their destination names stay fixed beneath `pages.path`.

The starter uses Pico CSS, Catppuccin Latte and Mocha, Work Sans and Fira Code, with pinned CDN dependencies. It needs no build step. Edit the HTML for your introduction, features and installation instructions. Replace `icon.svg` to use your project icon. One icon supplies the header, footer and favicon.

Optional examples include a three-slide gallery, a screenshot with a caption, a YouTube video, store graphics and a native HTML FAQ. Edit or remove each example before publication. The gallery supports scrolling, swiping and keyboard links without automatic rotation. The video loads lazily and starts only when the visitor plays it.

The store examples load official graphics from their publishers without download links. Keep the badges for your stores and wrap each image in a link to your product listing. Keep the original colours and proportions. The examples cover App Store, Google Play, Mac App Store, Microsoft Store, Snap Store, Flathub, Steam and itch.io. Unlike the pinned CSS and font dependencies, these publisher-hosted assets can change upstream.

Tailor substitutes the repository name, configured description and repository URLs only when it creates the starter. The copyright year is dynamic. Edit the copyright holder in the footer. No author identity is inferred from the repository owner.

The starter files default to `first-fit`. Tailor never replaces an existing site with starter content, including with `--recut` or `always`. It updates only the opted-in marked sections. A non-empty directory without `index.html` remains an error. Missing assets in an existing site are not added. If any required starter file uses `never`, creation stops before writes. Supply an existing site to use that mode.

`baste` reports all four proposed files without writing them. Disabled or skipped Pages creates no files. Hugo and Jekyll do not use this starter. The starter sources are excluded from generic swatch processing and use the Pages stage instead.

### Static Pages navigation

Put these markers on separate lines inside the header navigation list to let Tailor manage its repository links:

```html
<!-- tailor:navigation:start -->
<!-- tailor:navigation:end -->
```

The README link comes first. When `repository.has_wiki: true`, its label is `Overview`, followed by a `Documentation` link to the wiki. Otherwise, the README link is `Documentation`. When `repository.has_discussions: true`, a `Discussions` link follows. A `Download` link to `/releases` always comes last in the header. Omitted or false flags omit the corresponding link.

Tailor uses the repository that it manages to build these URLs. It preserves content outside the markers, including custom header links. No markers means no navigation changes. Malformed markers stop preflight before writes. This works only for enabled static Pages, independently of `pages.links`. Disabled or skipped Pages and Hugo or Jekyll sites stay unchanged. Run `tailor baste` to preview and `tailor alter` to update the links.

The footer uses `<!-- tailor:footer-navigation:start -->` and `<!-- tailor:footer-navigation:end -->` inside its Resources list. It follows the same README, Wiki and Discussions order. Append `Support` linking to `/blob/HEAD/SUPPORT.md` when either Wiki or Discussions is disabled or omitted. Hide Support only when both are enabled. Releases stays in the separate Project list.

The footer License link can also follow `license:`. Put these markers around its list item:

```html
<!-- tailor:license:start -->
<!-- tailor:license:end -->
```

Tailor keeps the label `License` and builds `?tab=<license>-1-ov-file` from the configured identifier. An empty value or `none` removes the marked link. Unmarked licence links stay unchanged. The same static Pages, preview and marker rules apply.

## GitHub wiki

Set `repository.has_wiki: true`, then run `tailor alter` to add wiki starter pages and `.github/workflows/tailor-wiki.yml`. Wiki publishing supports public repositories only. The source directory is `wiki/`, independent of Pages and `pages/`. Existing-project `fit` preserves the live `has_wiki` setting. New configs default to `false`.

Tailor preserves existing wiki starter pages, including under `alter --recut`. The generated workflow publishes the `wiki/` directory when its files or the workflow change on the default branch. You can also run it manually. An unmarked workflow at the same destination blocks setup before writes.

Before the first publication:

1. If the GitHub wiki has no pages, create its first page through the repository's Wiki tab.
2. Clone `https://github.com/OWNER/REPO.wiki.git` into a separate directory.
3. Import all wiki files into `wiki/`, except `.git`. Review conflicts with existing local files.
4. Record the imported wiki commit's full ID in `wiki/.tailor-wiki-base` using `git rev-parse HEAD` in that clone.
5. Commit the imported files, baseline and workflow, then push to the default branch.

The baseline records explicit adoption of the imported wiki. Without it, the first publication refuses to write. After adoption, the source directory controls the published tree, including deletions. The publisher preserves wiki history and never force-pushes. Independent edits through the Wiki tab stop publication. Import those edits and update the baseline before publishing again. Missing wikis, unknown access and concurrent changes stop publication without replacing content.

The workflow uses GitHub's built-in `GITHUB_TOKEN` with `contents: write`. Token support is verified against upstream implementation evidence, not a live Tailor publication. Tailor does not create a personal access token or configure a secret.

Set `repository.has_wiki: false` and run `tailor alter` to disable the wiki and remove only Tailor's marked workflow. Commit and push the removal to stop future workflow runs. Local source files and remote wiki history remain intact. An omitted setting leaves wiki files unmanaged. An unmarked workflow remains untouched and needs manual removal.

## Labels

The `labels` section manages GitHub issue labels declaratively. Tailor ships 12 default labels (the 9 GitHub defaults plus `dependencies`, `github_actions`, and `hacktoberfest-accepted`) with colours from the [Catppuccin Latte](https://catppuccin.com/palette/) palette.

```yaml
labels:
  - name: bug
    color: d20f39
    description: "Something isn't working"
  - name: enhancement
    color: 1e66f5
    description: "New feature request"
  - name: dependencies
    color: fe640b
    description: "Dependency update"
```

Labels are reconciled with create-and-update-only semantics: tailor creates missing labels and updates labels whose colour or description differs, but never deletes labels from the repository. This avoids removing labels already applied to issues.

To skip label management, omit the `labels` section and set the `.tailor.yml` swatch to `alteration: never`. With `always`, `alter` restores all default labels and manages them in the same run. With `first-fit`, `alter --recut` restores and manages them.

## Sponsorships

Tailor places `.github/FUNDING.yml` as a `first-fit` swatch, but the GitHub API does not expose the "Sponsorships" checkbox. After running `alter`, tick **Settings > General > Features > Sponsorships** manually to display the Sponsor button on the repository.

### Retired workflow cleanup

Run `tailor baste` to preview the upgrade, then run `tailor alter` to apply it. Tailor cleans up both retired workflows automatically:

- `.github/workflows/tailor-automerge.yml`
- `.github/workflows/tailor.yml`

`baste` changes no files. It reports `would update` when `.tailor.yml` contains retired entries and `would remove` for each retired workflow file on disk.

`alter` and `alter --recut` write `.tailor.yml` once as `updated` when the config contains retired entries. They then delete each present workflow file as `removed`.

Tailor accepts the historical `triggered` mode only for retired entries during this migration. Any other unrecognised swatch path stops validation before Tailor changes any files.

## Commands

### `fit <path>`

Creates a project directory and writes `.tailor.yml` with the full default swatch set. Does not copy files or apply settings.

```bash
tailor fit ./my-project
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

When a GitHub remote exists, `fit` queries the live repository configuration for the `repository`, `code_scanning`, `code_quality`, and `ruleset` sections. Otherwise, built-in defaults are used. When a default setup read returns an access error or `404`, `fit` warns and writes the built-in section. `fit` always writes `languages: []`. `fit` reads the `Tailor` ruleset when it exists and writes its live values. When the live `enforcement` is `evaluate`, `fit` warns and writes `active`. When the ruleset does not exist, or the read returns an access error or `403`, `fit` writes the built-in `ruleset` section. Exits with an error if `.tailor.yml` already exists.

### `alter`

Reads `.tailor.yml` in the current directory. It applies repository settings, immutable releases, Actions policy, code scanning, Code Quality, the ruleset, labels, variables, Pages, wiki files, licence, and swatches in that order.

```bash
tailor alter            # Apply changes
tailor alter --recut    # Overwrite always and first-fit swatches
```

`--recut` overrides `first-fit` and overwrites those swatches, but it still skips `never` swatches. Existing wiki starter pages and `LICENSE` are exempt. For `.tailor.yml`, `--recut` appends missing default swatch entries but never modifies existing entries.

`alter` and `alter --recut` report a completed label after each successful change. Labels are `set`, `created`, `updated`, `removed`, `copied`, and `overwritten`.

A default merge, retired-entry cleanup, or security prerequisite normalisation in `.tailor.yml` reports `updated`. Security normalisation also shows a warning. A retired workflow file cleanup reports `removed`.

### `baste`

Previews what `alter` would do without making changes.

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

### `docket`

Displays the current GitHub authentication state and repository context.

When authenticated, `docket` verifies the token with `GET /user`. Pages adds no requests to this command.

```bash
tailor docket
```

### `measure`

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
| `warning` | Health diagnostic needing attention (missing `README.md` or known unresolved licence placeholders) |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `.tailor.yml` |
| `config-only` | Swatch in `.tailor.yml` not in the built-in default set |
| `mode-differs` | Alteration mode differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor.yml` is present.
