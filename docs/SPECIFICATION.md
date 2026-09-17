# Tailor Specification v0.3

## Overview

Tailor is a local terminal CLI for managing project templates across GitHub repositories. It provides bespoke fitting for new projects and alterations for existing projects. Running `tailor` with no arguments displays help.

## Prerequisites

Tailor requires a valid GitHub authentication token. This can be provided in two ways:

1. **Environment variable**: Set `GH_TOKEN` or `GITHUB_TOKEN`. This works without the `gh` binary installed.
2. **GitHub CLI**: Install and authenticate the [GitHub CLI](https://cli.github.com/) (`gh`). Run `gh auth login` to authenticate. The `gh` binary is also used as a fallback for keyring-based token access when no environment variable is set.

The `fit`, `alter`, and `baste` commands require a valid authentication token: a token must exist and the effective host must accept it (`GET /user`). `fit` verifies the token at startup. `alter` and `baste` check token presence at startup and perform the `GET /user` verification after config parsing, before any write. If the token is missing or rejected, the command exits with an error before any local file change.

`measure` and `docket` are exempt from the authentication requirement. `measure` performs purely local file inspection and needs no network access or authentication. `docket` can report unauthenticated state without erroring - it displays the auth state rather than requiring it.

## Intended Workflow

### New project

`fit` creates the project directory and writes `.tailor.yml` with the full default swatch set in one command, with a `license: BlueOak-1.0.0` default. Use `--license=<id>` to select a different licence or `--license=none` to opt out. Change into `<path>`, then run `alter` to copy the swatch files and apply repository settings.

### Existing project

`measure` checks which community health files are present or missing - run it first to see what a project needs. If no `.tailor.yml` exists, run `tailor fit .` to create one (the directory already exists, so `fit` proceeds without error), or create `.tailor.yml` manually. Edit `.tailor.yml` directly to add or remove swatches or change alteration modes, then run `alter` to bring the project into sync with the current swatches. Re-run the CLI after upgrading Tailor to apply updated `always` swatches.

## Core Concepts

**Swatches**: Template files stored in `swatches/`. Most files are copied verbatim. `.github/FUNDING.yml` uses `{{GITHUB_USERNAME}}`. `SECURITY.md` uses `{{ADVISORY_URL}}`. `.github/ISSUE_TEMPLATE/config.yml` uses `{{SUPPORT_URL}}`. Go, Pages, and wiki support also render content from project settings.

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
| `.golangci.yml` | `.golangci.yml` |
| `.goreleaser.yaml` | `.goreleaser.yaml` |
| `.github/workflows/build-go.yml` | `.github/workflows/build-go.yml` |
| `Dockerfile` | `Dockerfile` |
| `cubic.yaml` | `cubic.yaml` |
| `.github/FUNDING.yml` | `.github/FUNDING.yml` |
| `.github/dependabot.yml` | `.github/dependabot.yml` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `.github/ISSUE_TEMPLATE/bug_report.yml` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `.github/ISSUE_TEMPLATE/feature_request.yml` |
| `.github/ISSUE_TEMPLATE/config.yml` | `.github/ISSUE_TEMPLATE/config.yml` |
| `.github/pull_request_template.md` | `.github/pull_request_template.md` |
| `pages/index.html` | `<pages.path>/index.html` |
| `pages/style.css` | `<pages.path>/style.css` |
| `pages/theme.js` | `<pages.path>/theme.js` |
| `pages/icon.svg` | `<pages.path>/icon.svg` |
| `wiki/Home.md` | `wiki/Home.md` |
| `wiki/_Sidebar.md` | `wiki/_Sidebar.md` |
| `wiki/_Footer.md` | `wiki/_Footer.md` |
| `.github/workflows/tailor-wiki.yml` | `.github/workflows/tailor-wiki.yml` |
| `.tailor.yml` | `.tailor.yml` |

Swatch-to-path mappings are hardcoded in the source. Licences are not swatches - they are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

**Repository Settings**: Tailor can manage GitHub repository settings declaratively via the `repository` section in `.tailor.yml`. Field names match the GitHub REST API field names exactly (snake_case). Settings are applied via `PATCH /repos/{owner}/{repo}` as a single API call, with additional fields applied via their own separate API endpoints. Repository settings are always applied idempotently on every `alter` run - there is no `first-fit` concept for API settings. If the `repository` section remains absent after the merge decision, repository settings are skipped entirely. A default merge for `always`, or `first-fit` with `--recut`, restores an absent section and manages it in the same run.

**Actions policy**: Tailor manages repository GitHub Actions policy through the top-level `actions` section. The built-in defaults enable Actions, use `allowed_actions: all`, and disable SHA pinning. The default config omits the three selected-action fields. Default merging adds the complete section when it is absent.

**Labels**: Tailor can manage GitHub issue labels declaratively via the `labels` section in `.tailor.yml`. Labels are a top-level config key alongside `repository:` and `swatches:`, not a field within `repository:`. The reconciliation strategy is create and update only - labels present on GitHub but absent from config are left untouched. No pruning. Label name matching is case-insensitive. When a label's name differs only in casing from the config, tailor updates the casing to match. The default config includes 12 labels (9 GitHub defaults plus `dependencies`, `github_actions`, and `hacktoberfest-accepted`) with colours from the Catppuccin Latte accent palette. If the `labels` section remains absent after the merge decision, label management is skipped entirely. A default merge for `always`, or `first-fit` with `--recut`, restores an absent or empty section and manages all default labels in the same run.

**Immutable releases**: The top-level `immutable_releases` section manages `enabled` (Boolean), which defaults to `false` in every config that `fit` creates. The section uses the endpoint response field name, not an invented repository PATCH field. Tailor runs this stage after repository settings and before Actions policy. An absent section or `enabled` key stays unmanaged, including during default merging and `--recut`. Explicit `true` and `false` values stay unchanged in config. `fit` uses the embedded default without reading the live value.

Tailor reads `GET /repos/{owner}/{repo}/immutable-releases`, which returns required Boolean fields `enabled` and `enforced_by_owner`. A `404` means disabled only after repository admin access and token Administration-read access are confirmed through repository and workflow permission reads. Otherwise, the value stays unknown. Access errors produce `would skip (insufficient scope)` in both `baste` and `alter`. A disabled declaration with `enforced_by_owner: true` produces `would skip (enforced by owner)` in both commands, without a write.

When the declared value differs, Tailor enables with `PUT` or disables with `DELETE` on the same endpoint, without a request body. Successful writes return `204`. A `409` is a conflict, not a setup-in-progress response, and stops the command with the API error. Other non-access errors also stop the command. Reads require Administration-read permission, and writes require Administration-write permission. `baste` performs no writes. Tailor never edits releases, tags, assets, or owner policy.

Enabling immutability protects future releases only. Disabling does not unlock existing immutable releases. Before enabling it, change release CI to create a draft, upload every asset, then publish. Workflows that upload or replace assets after publication will fail. See the [GitHub REST contract](https://docs.github.com/en/rest/repos/repos#check-if-immutable-releases-are-enabled-for-a-repository).

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
| `vulnerability_alerts_enabled` | bool | Enable Dependabot vulnerability alerts and the dependency graph |
| `automated_security_fixes_enabled` | bool | Enable Dependabot automated security fix pull requests |
| `topics` | string array | Repository topics for discoverability (replace-all semantics) |
| `default_workflow_permissions` | string | Default GITHUB_TOKEN permissions (`read` or `write`) |
| `can_approve_pull_request_reviews` | bool | Allow `GITHUB_TOKEN` workflows to create pull requests and submit approval reviews |
| `secret_scanning` | string | Secret scanning alerts (`enabled` or `disabled`) |
| `secret_scanning_push_protection` | string | Secret scanning push protection (`enabled` or `disabled`) |
| `secret_scanning_non_provider_patterns` | string | Secret scanning generic patterns (`enabled` or `disabled`) |

Several fields use separate API endpoints rather than the repository PATCH call. Tailor handles this transparently - they appear in `.tailor.yml` alongside other repository settings but are applied via their own API calls:

| Field | Read | Write |
|---|---|---|
| `private_vulnerability_reporting_enabled` | `GET /repos/{owner}/{repo}/private-vulnerability-reporting` | `PUT`/`DELETE /repos/{owner}/{repo}/private-vulnerability-reporting` |
| `vulnerability_alerts_enabled` | `GET /repos/{owner}/{repo}/vulnerability-alerts` (`204` means enabled) | `PUT`/`DELETE /repos/{owner}/{repo}/vulnerability-alerts` |
| `automated_security_fixes_enabled` | `GET /repos/{owner}/{repo}/automated-security-fixes` (`200` JSON contains `enabled` and `paused`) | `PUT`/`DELETE /repos/{owner}/{repo}/automated-security-fixes` |
| `topics` | Read from `GET /repos/{owner}/{repo}` response (no extra call) | `PUT /repos/{owner}/{repo}/topics` with `{"names": [...]}` |
| `default_workflow_permissions`, `can_approve_pull_request_reviews` | `GET /repos/{owner}/{repo}/actions/permissions/workflow` | `PUT /repos/{owner}/{repo}/actions/permissions/workflow` (both fields atomically) |
| `secret_scanning`, `secret_scanning_push_protection`, `secret_scanning_non_provider_patterns` | Read from the `security_and_analysis` block of `GET /repos/{owner}/{repo}` (no extra call) | `PATCH /repos/{owner}/{repo}` with `security_and_analysis` carrying only the declared keys |

**Topics**: The PUT endpoint replaces the entire topics list. The config declares the complete desired set; omitted topics are removed on apply. Topics are project-specific and not included in the default config template. Topic names must start with a lowercase letter or number, contain only lowercase alphanumerics and hyphens, and be 50 characters or fewer. The `topics` field uses `*[]string` semantics: nil (absent) means skip, empty list means clear all topics.

**Security settings**: The built-in defaults set the three Boolean security settings to `true` and the three secret scanning string settings to `enabled`, so generated configs expose all six security settings. Each Boolean field uses `*bool` semantics: nil means unmanaged before default merging, and a Boolean value declares the required state. Before strict validation and default merging, Tailor normalises the automated security fixes prerequisite. When `automated_security_fixes_enabled` is `true` and `vulnerability_alerts_enabled` is absent or `false`, Tailor sets `vulnerability_alerts_enabled` to `true` in memory and emits `warning: set vulnerability_alerts_enabled to true because automated_security_fixes_enabled requires vulnerability alerts`. `alter` and `alter --recut` write the corrected `.tailor.yml` before security API changes. `baste` reports `would update` and writes nothing. `measure` and `docket` accept the pair and write nothing. Default merging then appends other missing security settings and never changes an unrelated explicit value. Tailor applies only security endpoint settings whose live values differ, except for vulnerability alerts declared `true`.

When `vulnerability_alerts_enabled` is `true`, `alter` and `alter --recut` repeat `PUT /repos/{owner}/{repo}/vulnerability-alerts`, even when GET returns `204` (alerts enabled). This includes values set by prerequisite normalisation. GitHub documents that the [enable endpoint](https://docs.github.com/en/rest/repos/repos#enable-vulnerability-alerts) enables both vulnerability alerts and the dependency graph. Tailor has no independent read of dependency graph state and adds no separate setting. A successful request confirms the endpoint response, not independent verification of the graph.

For an already-enabled alerts value, the result retains `Before: true` and carries the annotation `reapply enable request for Dependency Graph`. `baste` reports the planned request without writes. A matching `false` remains `no change`. A field that remains absent after normalisation and default merging stays unmanaged. Unknown reads and access errors retain the skip behaviour below.

GitHub can use `404` for a disabled feature or denied access. A private vulnerability reporting `404` always leaves the value unknown and produces an access warning. A vulnerability alerts or automated fixes `404` means disabled only when the repository response confirms `permissions.admin: true` and the Actions workflow permission read confirms Administration-read access. Otherwise, Tailor leaves the value unknown and produces an access warning. The automated fixes GET uses the `enabled` value from the official `200` JSON response and does not treat `204` as a GET result.

When Tailor enables alerts and automated fixes together, it enables alerts first. If the alerts read is unknown, or the write fails or is skipped, Tailor does not enable automated fixes. When Tailor disables both, it disables automated fixes first. If the automated fixes read is unknown, or the write fails or is skipped, Tailor does not disable alerts. Tailor warns about the prerequisite only when automated fixes are required and alerts will remain disabled. Other access errors produce skip results, and other API errors stop the command.

**Secret scanning**: `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns` accept `enabled` or `disabled`. The key names and values match the `security_and_analysis` object of the repository PATCH body. Tailor sends only the declared keys, so keys outside Tailor policy keep the value set in the GitHub UI. The built-in defaults set all three to `enabled`. Push protection requires secret scanning. When `secret_scanning_push_protection` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` in memory and emits `warning: set secret_scanning to enabled because secret_scanning_push_protection requires secret scanning`. The write path for this warning matches the automated security fixes prerequisite. The `security_and_analysis` block is absent from the repository response when the token lacks admin access. Tailor then leaves all three values unknown and produces an access warning.

**Generic patterns**: `secret_scanning_non_provider_patterns` turns on the generic patterns that GitHub maintains, such as private keys, database connection strings, and HTTP authorisation headers. The GitHub UI calls the feature "Generic patterns", and the API key keeps the older name "non-provider patterns". There is nothing to configure beyond the toggle. Custom patterns need a Secret Protection licence and stay out of scope. Alerts from generic patterns have lower confidence than alerts from provider patterns. Push protection does not block them, so they raise alerts only. The feature is free on a public repository with secret scanning enabled. Generic patterns require secret scanning. When `secret_scanning_non_provider_patterns` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` in memory and emits `warning: set secret_scanning to enabled because secret_scanning_non_provider_patterns requires secret scanning`. The write path for this warning matches the push protection prerequisite.

**Actions workflow permissions**: `default_workflow_permissions` accepts `read` or `write`. The PUT endpoint sends both `default_workflow_permissions` and `can_approve_pull_request_reviews` atomically. GitHub labels the latter setting “Allow GitHub Actions to create and approve pull requests”. Tailor keeps the REST API field name because repository config keys map directly to API fields. Enabling it permits the repository `GITHUB_TOKEN` to create pull requests and submit approval reviews when the workflow has `pull-requests: write`. The setting does not permit merges, bypass branch rules, or affect personal access tokens or separate GitHub App tokens. The tailor defaults (`read` and `false`) follow the principle of least privilege. If the API rejects the read or write, Tailor reports `would skip (insufficient scope)` in `baste` and skips the operation in `alter`. Use a token with the required repository permissions.

Supported settings in the top-level `actions` section:

| Field | Type | Description |
|---|---|---|
| `enabled` | bool | Enable GitHub Actions for the repository |
| `allowed_actions` | string | Allowed policy: `all`, `local_only`, or `selected` |
| `sha_pinning_required` | bool | Require full-length commit SHAs for actions. Defaults to `false` |
| `github_owned_allowed` | bool | Allow GitHub-owned actions under the selected policy |
| `verified_allowed` | bool | Allow actions from verified creators under the selected policy |
| `patterns_allowed` | string array | Complete set of allowed action and reusable workflow patterns |
| `artifact_and_log_retention.days` | integer | Opt-in retention for new artifacts and logs, from 1 to 90 days, within the live owner cap |
| `fork_pr_contributor_approval.approval_policy` | string | Contributors whose fork pull request workflows require approval. Defaults to `first_time_contributors` |

Tailor reads and writes `enabled`, `allowed_actions`, and `sha_pinning_required` through `/repos/{owner}/{repo}/actions/permissions`. Tailor uses `/repos/{owner}/{repo}/actions/permissions/selected-actions` for `github_owned_allowed`, `verified_allowed`, and `patterns_allowed`. The three selected-action fields are valid only with `allowed_actions: selected`. The selected endpoint replaces `patterns_allowed`; comparison sorts both lists because GitHub order has no policy meaning.

Each Actions policy field uses pointer semantics, so default merging preserves explicit Boolean values, an explicit custom list, and an explicit empty list. When the section is absent, Tailor adds the complete default policy. When an existing config declares `selected`, Tailor appends each missing selected-action field. A missing `patterns_allowed` field receives the six compatibility defaults: `freerangebytes/setup-actionlint@*`, `golang/govulncheck-action@*`, `golangci/golangci-lint-action@*`, `nick-fields/retry@*`, `robherley/go-test-action@*`, and `softprops/action-gh-release@*`. Missing `github_owned_allowed` and `verified_allowed` fields receive `true`. After default merging, a selected policy must include `github_owned_allowed`, `verified_allowed`, and `patterns_allowed`. To switch an existing config to `all`, the user must remove `github_owned_allowed`, `verified_allowed`, and `patterns_allowed`. SHA pinning is a separate choice. Default merging never replaces an explicit policy or SHA pinning value, including under `--recut`. For `all` or `local_only`, Tailor appends missing core fields and the approval default, but leaves selected-action fields absent.

Fork pull request approval controls which external contributors need approval before their workflows run. The optional string `actions.fork_pr_contributor_approval.approval_policy` accepts exactly three values:

| Policy | Approval required for |
|---|---|
| `first_time_contributors_new_to_github` | First-time contributors who are new to GitHub |
| `first_time_contributors` | All first-time contributors (the Tailor default) |
| `all_external_contributors` | All external contributors |

`fit` writes `first_time_contributors` without reading the live approval policy. Default merging adds it when `actions`, the approval object, or `approval_policy` is absent or null. An empty approval object also receives the default. Explicit policies remain unchanged, including under `--recut`, regardless of the core Actions policy. Local validation rejects empty strings, other policy values, wrong YAML types, and unknown approval keys before any mutation.

For existing `.tailor.yml` files, default merging runs with `alteration: always`, or `first-fit` with `--recut`. It does not run for `never` or an absent `.tailor.yml` swatch entry. Without a merge, an absent or null approval policy stays unmanaged and causes no approval API calls. A restored default becomes managed in the same run. Approval alone requires no core, selected-action, or retention fields.

Tailor reads `GET /repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval` and writes `PUT` to the same endpoint with only `approval_policy`. Approval never travels through the core, selected-actions, or retention endpoints. It does not change workflow token permissions or private-fork permissions.

`baste` reports a difference as `actions.fork_pr_contributor_approval.approval_policy = first_time_contributors` with the `would set` label, without writes. Apply sends one approval PUT for a difference and none for a match. A successful write returns `204`. A second apply makes no approval write when the live policy still matches. Approval writes follow core, selected-action, and retention writes. A hard failure in an earlier stage prevents later writes.

Before reading approval, Tailor reads repository visibility through `GET /repos/{owner}/{repo}`. A confirmed private repository produces `would skip (not available)`, without an approval GET or PUT. Tailor continues other settings. Default merging remains unchanged, including the approval default on private repositories. Approval API errors, including HTTP `422`, remain fatal unless existing access-skip rules apply.

An approval read with a missing, null, empty, or unrecognised live policy stays unknown and produces `would skip (insufficient scope)`, without a write. Access-denied or unavailable reads and writes also produce skips. Approval skips do not suppress unrelated settings, and skips on other endpoints do not suppress approval. Rate limits and other API errors stop the command.

Artifact and log retention is opt-in. A retention-only declaration requires no core or selected-action fields:

```yaml
actions:
  artifact_and_log_retention:
    days: 30
```

`days` is an optional integer. Local validation rejects values outside 1 to 90 before any mutation. An absent retention object or absent or null `days` leaves retention unmanaged and causes no retention calls. Bootstrap, default merging, and `--recut` never insert active retention defaults. Default merging preserves explicit days and applies the core, selected-action, and approval defaults independently.

Tailor reads `GET /repos/{owner}/{repo}/actions/permissions/artifact-and-log-retention` to obtain `days` and `maximum_allowed_days`. Both live fields must be present and positive. A declared value above `maximum_allowed_days` stops the command before the retention write, with the allowed maximum in the error. The cap is read-only, not a config key. Denied or unavailable reads stay unknown, produce `would skip (insufficient scope)`, and cannot cause a retention write. Missing or invalid live fields stop the command.

`baste` reports a difference as `actions.artifact_and_log_retention.days = 30` with the `would set` label, without writes. Apply sends one `PUT` to the same retention endpoint for a difference and none for a match. The PUT body contains only `days`, and success returns `204`. Retention never travels through the core or selected-actions endpoint. Retention writes follow core and selected-action writes without changing their order. Access failures produce skip results, and other API failures stop the command.

Retention changes affect only new artifacts and logs. Tailor does not change existing artifacts, logs, caches, or owner policy. See the [GitHub retention REST contract](https://docs.github.com/en/rest/actions/permissions#set-artifact-and-log-retention-settings-for-a-repository).

The table summarises valid core and selected-action write orders. `core` means `/repos/{owner}/{repo}/actions/permissions`, and `selected` means the matching `/selected-actions` endpoint. Tailor writes only changed endpoint groups, except for temporary `core` writes that keep a partial failure fail-closed.

| Current and requested state | Write order |
|---|---|
| Enabled `all` to enabled `selected` | 1. `core`: select `selected` and preserve active SHA pinning when the request relaxes it<br>2. `selected`: apply requested restrictions<br>3. `core`: relax SHA pinning when requested |
| `all` or `local_only` to `selected`, except for the case above | 1. `core`: disable Actions and select `selected`<br>2. `selected`: apply requested restrictions<br>3. `core`: apply the requested final policy |
| Existing `selected`, both endpoint groups change, Actions are enabled, and selected restrictions broaden or the final policy disables Actions | 1. `core`: disable Actions<br>2. `selected`: apply requested restrictions<br>3. `core`: apply the requested final policy |
| Existing `selected`, both endpoint groups change, and no pre-disable is needed | 1. `selected`: apply requested restrictions<br>2. `core`: apply the requested final policy |
| Any other valid update | Write each changed endpoint group. If both groups change, `core` precedes `selected`. |

When Tailor changes an enabled `all` policy to an enabled `selected` policy, it first writes `allowed_actions: selected`. If the requested policy disables active SHA pinning, this write preserves `sha_pinning_required: true`. Tailor then writes the complete selected restrictions and writes `sha_pinning_required: false` in a final core request. If SHA pinning stays the same or becomes stricter, the first request uses the requested value and Tailor omits the final request. A hard first-request failure leaves `all` active. A selected-policy failure leaves the narrower `selected` policy active and preserves SHA pinning. A final SHA request failure leaves the selected restrictions and SHA pinning active. Tailor returns an explicit error for each hard failure.

For other transitions from `all` or `local_only` to `selected`, Tailor disables Actions before it writes the complete selected restrictions and the requested final core policy. For an existing selected policy, Tailor writes changed selected restrictions before a core write that broadens access, including disabling SHA pinning. When an enabled policy combines selected broadening with core tightening, Tailor first disables Actions. Selected broadening means newly allowing GitHub-owned actions, verified actions, or patterns. Core tightening means enabling SHA pinning or disabling Actions. Tailor then writes the selected restrictions and final core policy. Tailor also disables Actions before any selected update whose final policy disables Actions. If either later write fails, Tailor returns an explicit error and leaves Actions disabled. If the selected-policy read fails while the effective policy stays selected, Tailor skips dependent core broadening. Organisation policy can restrict repository choices. Other access errors produce clear skip results, and hard errors stop the command.

**Code scanning**: Tailor manages CodeQL default setup through the top-level `code_scanning` section. Tailor reads `GET /repos/{owner}/{repo}/code-scanning/default-setup` and writes `PATCH /repos/{owner}/{repo}/code-scanning/default-setup` with only the declared fields.

| Field | Type | Description |
|---|---|---|
| `state` | string | `configured` or `not-configured` |
| `query_suite` | string | `default` or `extended` |
| `threat_model` | string | `remote` or `remote_and_local` |
| `languages` | string array | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `actions`, `c-cpp`, `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby`, `swift` |

The built-in defaults set `state: configured`, `query_suite: default`, `threat_model: remote`, and `languages: []`. An empty `languages` list means Tailor sends no `languages` field, so GitHub detects the languages on enable and keeps the current set afterwards. A non-empty list is the complete set, compared as a set and sent on a difference. This differs from `topics`, where an empty list clears all topics. `fit` always writes `languages: []`, because GitHub does not report whether a live list was detected or chosen. A successful write returns `202` and starts a validation run. Tailor reports the field as set. A `409` means a validation run is in progress. Tailor reports `would skip (setup in progress)` and continues. A `403` means the feature is not available to the repository. Tailor reports `would skip (not available)` and continues. `runner_type` and `runner_label` are not managed. Default setup does not conflict with workflows that upload SARIF results, such as Scorecard. Default merging adds the complete section when it is absent and otherwise fills missing fields without changing explicit values. A default merge for `always`, or `first-fit` with `--recut`, restores an absent section and manages it in the same run.

**Code Quality**: Tailor manages GitHub Code Quality through the top-level `code_quality` section. Tailor reads `GET /repos/{owner}/{repo}/code-quality/setup` and writes `PATCH /repos/{owner}/{repo}/code-quality/setup` with only the declared fields.

| Field | Type | Description |
|---|---|---|
| `state` | string | `configured` or `not-configured` |
| `languages` | string array | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby` |

The built-in default sets `state: not-configured` and `languages: []`. The `languages` rules and the write responses follow the code scanning rules. `ai_findings_option`, `runner_type`, and `runner_label` are not managed, so AI findings stay as the GitHub UI set them. Default merging follows the code scanning rule: it adds the complete section when it is absent, otherwise fills missing fields without changing explicit values, and restores an absent section for `always`, or `first-fit` with `--recut`, in the same run.

**Ruleset**: Tailor manages one branch ruleset named `Tailor` through the top-level `ruleset` section. The name is fixed in source. Tailor lists `GET /repos/{owner}/{repo}/rulesets`, finds the ruleset by name, and reads it with `GET /repos/{owner}/{repo}/rulesets/{ruleset_id}`. When the ruleset is absent, Tailor creates it with `POST /repos/{owner}/{repo}/rulesets`. When a managed field differs, Tailor replaces it with `PUT /repos/{owner}/{repo}/rulesets/{ruleset_id}`. Every write sends the complete ruleset: `name`, `target: branch`, `enforcement`, `bypass_actors`, `conditions`, and `rules`. Before any write, validation requires `enforcement`, `bypass_actors`, both `ref_name` lists, the six Boolean rule keys, `enabled` on `pull_request`, `required_status_checks`, and `code_scanning`, and every parameter of an enabled rule, because an absent field would clear the live value without a report line. Tailor owns the `Tailor` ruleset entirely. A rule, bypass actor, or condition added by hand to that ruleset is removed on the next write. Hand-made rules belong in a separate ruleset, which Tailor never reads or writes. Tailor never deletes the `Tailor` ruleset. A default merge for `always`, or `first-fit` with `--recut`, restores an absent section and manages it in the same run.

| Field | Type | Description |
|---|---|---|
| `enforcement` | string | `active` or `disabled`. `disabled` keeps the ruleset on GitHub, and GitHub ignores it. `evaluate` is Enterprise only and is rejected |
| `bypass_actors` | list | Complete set of bypass actors. Each entry has `actor_id` (integer), `actor_type` (`RepositoryRole`, `Team`, `User`, `Integration`, or `DeployKey`), and `bypass_mode` (`always`, `pull_request`, or `exempt`). `actor_id` is null for `DeployKey`. `RepositoryRole` ids are `2` maintain, `4` write, `5` admin. An empty list means no bypass |
| `conditions.ref_name.include` | string array | Branch names, `refs/heads/` fnmatch patterns, `~DEFAULT_BRANCH`, or `~ALL`. At least one entry |
| `conditions.ref_name.exclude` | string array | Branch names or `refs/heads/` fnmatch patterns. `~DEFAULT_BRANCH` and `~ALL` are rejected |
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

The `rules` map is Tailor's form of the API `rules` list. Each key is an API rule `type`. A Boolean key sends `{"type": "<key>"}` when `true` and nothing when `false`. The `enabled` key on `pull_request`, `required_status_checks`, and `code_scanning` is Tailor's own, because these three rules carry parameters that stay in the config while the rule is off. The `parameters` keys match the API exactly. The built-in defaults reproduce the GitHub UI default ruleset: `deletion`, `non_fast_forward`, and `pull_request` on, with one required approval, stale review dismissal, thread resolution, the extra approval for unattributed changes, and squash or rebase merging, bypassed by the repository admin role, and targeting the default branch. `required_status_checks` is off with an empty list, because a required check that never reports blocks every merge, and no other repository has Tailor's CI job names. Enable it per repository and name an aggregating job, for example one that depends on every other job and fails when any of them failed.

The `code_scanning` rule is the free route to the "Check runs failure threshold" merge gate on the Advanced Security settings page. That page setting has no repository API. The rule blocks a merge until the tool has results for both the pull request commit and the base branch, so a new repository has no results yet. `code_scanning` is therefore off with one entry, `CodeQL` at `errors` and `high_or_higher`, which matches the GitHub UI defaults "Only errors" and "High or higher". Turning the gate on is a one-line change to `enabled`. Tailor does not cross-check the rule against the top-level `code_scanning.state`, because advanced setup also reports as the `CodeQL` tool while default setup stays `not-configured`.

Tailor compares only managed fields: `enforcement`, `bypass_actors` as a set, `include` and `exclude` as sets, the presence of each rule, the managed `pull_request` parameters, the managed `required_status_checks` parameters with the check list as a set, and `enabled` on `code_scanning` with the tool list as a set. Parameters that GitHub fills in on read and Tailor does not manage, such as `dismissal_restriction` and `required_reviewers`, never cause a write. GitHub blocks a merge when the ruleset allows a method that the repository disables. When `allowed_merge_methods` names a method whose `repository` setting is `false` in the same config, validation emits `warning: ruleset allows <method> merging but repository.<field> is false`, and continues. Tailor does not rewrite either value.

`POST` returns `201` and `PUT` returns `200`. Tailor reports the fields as set. A `403` on read or write means rulesets are not available to the repository. Tailor reports `would skip (not available)` and continues. A read that omits `bypass_actors` means the token lacks write access to the ruleset. Tailor reports `would skip (insufficient scope)` for the section and continues. A `422` means GitHub rejected the ruleset body, and Tailor stops with the API error, because the cause is the config. Other errors stop the command.

**Free features only**: Tailor exposes no setting that needs a paid GitHub plan, a GitHub Advanced Security or Secret Protection licence, a self-hosted runner, or AI credit spend on a public repository. Every request body carries only the fields that Tailor manages, so a setting outside Tailor policy keeps the value set in the GitHub UI. This rule excludes the `security_and_analysis` keys other than `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns`, the `runner_type` and `runner_label` fields of default setup endpoints, and `ai_findings_option`.

Settings deliberately excluded due to risk or org-level scope: `visibility`, `default_branch`, `name`, `archived`, `is_template`, `allow_forking`. Additional API areas considered and deferred: autolinks, general deployment environments, custom properties (org-level), and Dependabot secrets. Pages manages only the `github-pages` environment. Classic branch protection rules are out of scope. Rulesets replace them, and Tailor manages one ruleset through the `ruleset` section. Rulesets outside the `Tailor` ruleset are out of scope.

### Go ecosystem support

The top-level `languages` section selects language-specific development swatches. Go is the only supported language key:

```yaml
languages:
  go: true
```

`languages` must be a map. Its only accepted key is `go`, with a Boolean value. Reject null sections, null values, unknown keys, and non-Boolean values. An empty map leaves Go unspecified. Internally, `Languages *LanguageSettings` and `Go *bool` preserve absent, false, and true states.

New configurations contain `languages.go: false`. Default merging preserves an absent `languages` section or absent `go` key in existing configurations. It never infers language selection from source files or CodeQL settings. `code_scanning.languages` and `code_quality.languages` remain independent.

Only explicit `languages.go: true` activates `.golangci.yml`, `.goreleaser.yaml`, `.github/workflows/build-go.yml`, and `Dockerfile`. All four are development swatches with `first-fit` defaults. The registry contains these entries even when Go is inactive. Default merging appends missing entries only when the config swatch mode permits merging, without changing existing modes.

False or absent Go selection skips the four destinations without deletion or replacement, including with `--recut`. Existing workflows continue to run on GitHub. Disabling Go in Tailor does not disable a workflow. Active Go swatches use the ordinary `always`, `first-fit`, `never`, and `--recut` rules. Existing customised first-fit files remain unchanged during normal alterations.

Go selection also controls two existing swatches when Tailor renders them:

| Swatch | Go behaviour |
|---|---|
| `justfile` | Explicit true adds `build` (`go build ./...`) and `test` (`go test ./...`). The `lint` recipe adds golangci-lint and retains actionlint. Existing recipes remain in the template. |
| `.github/dependabot.yml` | Explicit true includes `gomod`. Explicit false omits `gomod`. An absent selection preserves the legacy `gomod` entry. GitHub Actions and Nix entries remain. |

These variants retain their existing alteration modes. A language change alone never replaces an existing first-fit file.

#### Go release discovery and preflight

When `.goreleaser.yaml` needs rendering, Tailor discovers executable packages through local Go syntax trees in the root module. It requires a root `go.mod` and real `package main` declarations with a `main` function. It creates one build per executable, with unique build and binary names. It never executes project code, calls `go list`, or accesses the network for discovery.

Discovery skips symlinks, vendor directories, testdata, nested modules, and `*_test.go` files. It also skips files and directories with a dot or underscore prefix. The root `go.mod` must be a readable regular file with a module declaration.

Discovery limits are 100,000 entries, 1 MiB per file, and 64 MiB of source. The targets are Linux and Darwin, each on amd64 and arm64, with `CGO_ENABLED=0`. Tailor checks file constraints with `go/build.MatchFile` for each target.

Tailor selects directories with a `package main` function that matches at least one release target. It excludes directories whose main declarations match no release target, including ignored generators. Each selected executable needs one eligible `main` function per target, without a receiver, arguments, results, or type parameters. Tailor checks executable signatures only in eligible `package main` files. Tailor rejects eligible C imports, mixed packages, and missing or duplicate entry points. Tailor does not resolve or type-check dependencies.

Root executables use the module basename without a final `/vN` segment. Other executables use their directory basename. Names contain ASCII letters, digits, hyphens, and underscores. Stable numeric suffixes resolve collisions.

An existing first-fit release configuration does not require discovery. A `never` entry, an absent swatch entry, or inactive Go support also skips discovery. Required discovery completes before token verification and all writes. If a required release configuration has no supported executable, Tailor stops before writes. `baste` performs these checks without writes.

Before Tailor creates or overwrites the builder workflow, it checks `.golangci.yml` and `.goreleaser.yaml`. Each dependency must already be a regular file, or Tailor must plan to create it. Existing dependency files cannot be symlinks. Tailor does not parse their contents. An existing first-fit builder workflow or a `never` entry skips these checks.

Before Tailor creates or overwrites `.goreleaser.yaml`, it checks `Dockerfile` with the same regular-file or planned-creation requirement. An existing first-fit release configuration or a `never` entry skips this check. Tailor does not parse custom Dockerfiles or check their compatibility with the generated release configuration.

Library-only projects can retain `.golangci.yml` and set `.goreleaser.yaml`, `.github/workflows/build-go.yml`, and `Dockerfile` to `never`.

#### Go release outputs

The default release configuration uses free GoReleaser features and disables CGO. It retains archives for Linux and macOS, each on amd64 and arm64.

The nFPM configuration produces `deb`, `rpm`, and `apk` packages for Linux on amd64 and arm64. Each package includes all discovered executables. GitHub Releases contain the archives, native packages, and checksums.

The default package maintainer is `{{ .Env.GITHUB_REPOSITORY_OWNER }}`. Projects can replace `nfpms[].maintainer` with their maintainer's name and email address before publication.

Each executable also produces one multi-platform GHCR image for Linux on amd64 and arm64. A single executable uses `ghcr.io/<owner>/<repo>`, in lowercase. Tailor replaces each `_` and `.` in the repository name with a hyphen. Tailor adds `image` before a leading hyphen and after a trailing hyphen. Other repository names remain unchanged apart from case.

Multiple executables append a hyphen and a normalised binary name. Tailor lowercases binary names, collapses non-alphanumeric runs to hyphens, and trims leading and trailing hyphens. Stable numeric suffixes resolve collisions. Version tags apply to all releases. Only stable releases update `latest`.

The Dockerfile swatch uses a digest-pinned Chainguard static base. It accepts a `BINARY` argument and copies the selected platform's executable to a fixed application entrypoint. The image runs as a non-root user. Existing custom first-fit Dockerfiles remain unchanged during normal alterations, and `never` always preserves them. Tailor's root Dockerfile uses a digest-pinned Chainguard Git base, which supplies Git and its runtime dependencies for repository detection and wiki reads. Both bases support `linux/amd64` and `linux/arm64`. Mounted checkouts require a matching user ID so that Git accepts their ownership.

The digests fix the base images for reproducible builds. Tailor maintainers review newer upstream digests and update each Dockerfile when they accept a refresh. The generated Dependabot configuration does not manage Docker images. Existing projects must review and update their Dockerfile digest explicitly because normal `first-fit` alterations preserve it.

The generated release configuration requires `GITHUB_REPOSITORY` and `GITHUB_REPOSITORY_OWNER` at runtime. GitHub Actions supplies both variables. Local GoReleaser runs must set them explicitly. Use GoReleaser v2.18.0 or later for local snapshots. Local snapshots require Docker with a running daemon and Docker Buildx.

#### Go builder workflow

The builder workflow runs tests, coverage, lint checks, and govulncheck. Pull requests and default-branch pushes build GoReleaser snapshots and upload archives, native packages, and checksums without publication. Snapshot jobs build container images locally, but do not push images or include them in downloadable artifacts. Tags that match `v*.*.*` publish GitHub Releases and GHCR images. Default-branch handling must use the repository's default branch, not a fixed `main` or `master` branch.

The Go release templates use GitHub-hosted runners and do not require Nix or GoReleaser Pro. The release job authenticates to GHCR with the existing GitHub token and grants `packages: write`. No extra secrets are required. The templates do not configure signing or external package repositories. Go support does not change Tailor's independent CodeQL default setup.

### GitHub wiki

The wiki publication job uses `ubuntu-slim` with an explicit 15-minute timeout, which matches the runner's hard limit. The runner provides Git and Python 3 for publication.

`repository.has_wiki: true` activates four wiki swatches on public repositories. No separate config section or command exists. Every config that `fit` creates uses `has_wiki: false`, including for repositories with an enabled wiki. An omitted setting leaves wiki files unmanaged, even when default merging adds `false` during that run.

The fixed sources are `wiki/Home.md`, `wiki/_Sidebar.md` and `wiki/_Footer.md`, with `first-fit` defaults. Existing starter destinations remain unchanged, including recut and an explicit `always` mode. `never` skips creation. `.github/workflows/tailor-wiki.yml` defaults to `always`, starts with `# Managed by Tailor: wiki`, and uses resolved-content comparison. A protected `first-fit` or `never` workflow must match the generated YAML semantics. An unmarked workflow blocks enabled setup before writes, including recut. The four paths are development swatches, excluded from generic processing and local health checks but included in config comparison. The registry contains 29 swatches.

Local safety preflight runs before writes. It rejects source symlinks, Git metadata, non-regular files and unsafe workflow destinations or parents through rooted filesystem access. Pages preflight also completes before wiki enablement. Tailor reads repository privacy, the current default branch and `has_wiki`. Known private repositories and projects without repository context skip wiki files. Unavailable or incomplete repository metadata blocks readiness.

For a declared public wiki, `alter` enables a disabled wiki through a repository PATCH containing only `has_wiki: true`. This is the sole write before readiness. Tailor then checks the remote wiki and local adoption. A readiness blocker exits nonzero before config updates, retired workflow removal or any other local or remote write. Confirmed enablement remains in effect if readiness fails. Tailor reports that change. An enablement failure stops the command.

Inspect the public wiki through bounded, anonymous Git reads in an isolated temporary clone. Never import into the project, push commits or change remote wiki history. A confirmed empty remote requires the user to create and save the first page at the repository's `/wiki` URL. The user then manually imports all files except `.git` from a separate wiki clone into `wiki/`, after reviewing local conflicts. After import, the user records the imported HEAD in `wiki/.tailor-wiki-base`. The user runs `tailor baste` to recheck readiness. When the wiki checks pass, the user runs `tailor alter`.

Failed Git access remains unknown. Report access or network errors separately, with conditional first-page guidance rather than a claim that the wiki is missing.

`baste` reports readiness blockers and continues the full preview without writes. After all preview results, it appends a `Next steps:` section for the blocker. The section states that `would copy` and other pending changes wait until wiki readiness passes. Readiness blockers alone do not change the preview's successful exit status.

A disabled wiki requires enablement before remote readiness can be checked. Its next steps start with `tailor alter` to enable the wiki through the API. They include conditional first-page creation at the repository's exact `/wiki` URL and manual import instructions if adoption is required. Include the separate clone command, conflict review, import of all files except `.git`, and the command to record imported HEAD in `wiki/.tailor-wiki-base`. Do not claim that a disabled wiki has no pages. Finish with `tailor baste` to recheck readiness, then `tailor alter` to apply the remaining changes.

Confirmed access and network blockers give check-and-retry instructions, without page creation or import instructions. If Tailor cannot distinguish absence from denied access, direct the user to check access at the exact `/wiki` URL. Only after access works does that guidance suggest first-page creation, if no page exists. It gives no import instructions for this unknown state.

Reconcile ready wiki files after Pages and before the licence. Pages and its source directory remain independent. `measure` and `docket` add no wiki requests.

The generated workflow runs for default-branch changes to `wiki/**` or itself, and manual dispatch on that branch. It checks current wiki availability, public visibility and the current default branch before publishing. The workflow uses a GitHub-hosted runner and built-in `GITHUB_TOKEN` with `contents: write`. Upstream implementation evidence supports token access, but Tailor's end-to-end publication is not live-tested. Tailor does not provision secrets or personal access tokens.

Before initial adoption, the user reviews local conflicts and manually imports every file from a separate wiki clone, except `.git`, into `wiki/`. Only after import does the user record the imported HEAD in `wiki/.tailor-wiki-base`, as one full 40-character lowercase hexadecimal commit ID. The user runs `tailor baste` to recheck readiness, then `tailor alter` before committing the imported files, baseline and workflow. A missing, malformed or stale baseline blocks initial setup. Starter pages alone never authorise replacement.

Readiness accepts initial adoption or explicit reimport when the baseline matches remote HEAD and every remote file path exists as a regular local source file. Local edits remain permitted. Otherwise, readiness requires exactly one valid `Tailor-Wiki-Source` trailer and a source commit that is an ancestor of local HEAD. Remote file modes and Git object IDs must match that commit's wiki tree, excluding the baseline. Independent remote edits or unknown source history block readiness and require reviewed reimport. These checks do not prove successful publication. The user checks the generated workflow run after the push.

After adoption, publishing replaces the wiki tree with the source subtree, excluding the baseline, and preserves commit history through a normal fast-forward push. No force-push is permitted. Each publication records the source commit in a `Tailor-Wiki-Source` trailer. Subsequent runs require one valid trailer, source ancestry and an exact match between the remote tree and the previous published source. Independent wiki edits require reimport and a new baseline. Stale source runs and concurrent remote writes fail without overwriting newer work.

Explicit `repository.has_wiki: false` retains all source files and removes only the marked workflow, regardless of recut. An unmarked workflow stays unchanged with manual-removal guidance. Commit and push the workflow removal to stop future runs. No remote wiki content or history is deleted. An omitted declaration does not remove the workflow.

### GitHub Pages

Static upload and all deployment jobs use `ubuntu-slim` with explicit 15-minute timeouts, which match the runner's hard limit. Hugo and Jekyll build jobs retain `ubuntu-24.04` for Go modules, Ruby dependencies and longer builds.

Pages is opt-in and supports free public repositories only. These settings control publication and optional static-site links:

```yaml
pages:
  enabled: false
  generator: static
  path: pages
  # branch: main
  # cname: www.example.com
  # Static only. Omit links to leave the page unchanged; {} clears its marked links.
  # Use full HTTPS URLs, or a bare address for email. Empty values add no icon.
  # links:
  #   website: https://example.com
  #   email: hello@example.com
```

An omitted section or disabled setting makes no Pages-related changes to settings, workflows, environments, ignores or homepages. Bootstrap, default merging and recut never activate Pages. Missing fields take disabled, `static` and `pages` defaults. Explicit values survive merging. Unknown keys, wrong types and unknown generators are errors.

`path` is an existing project-relative source directory. Reject absolute paths, traversal, all source and parent symlinks, and unsafe workflow interpolation. Config loading checks syntax only. Before any remote write, validate the source, dependency declarations and workflow ownership. An omitted `branch` resolves to the current repository default branch on each run. Explicit branches must be valid Git branch names. Escape workflow filter metacharacters so the branch matches literally. Neither path nor branch is a legacy Pages API source setting.

Static requires `index.html` in an existing site, or creates the starter in a missing or empty directory. It uploads authored URLs unchanged. The site must already support its deployment base path. Hugo requires recognised configuration and local themes or pinned module/submodule declarations, and builds to `<path>/public`. Modules require pinned `go.mod` requirements and matching `go.sum` entries. Module replacements are unsupported. Unpopulated theme submodules require a `.gitmodules` declaration and a pinned Git index entry. Jekyll requires `_config.yml`, `Gemfile` and `Gemfile.lock`, including Jekyll 4.4.1 and dependencies, and builds to `<path>/_site`. Git dependencies require pinned commits. Path dependencies must exist inside the source directory. Inspected input files must be regular files no larger than 1 MiB. Use Hugo Extended 0.165.0 and Ruby 3.3.12. Dependency installation failures fail CI. No implicit Node, Sass or arbitrary build commands are supported. Only static Pages has a starter.

One registered development swatch, `.github/workflows/tailor-pages.yml`, defaults to `always` and selects an embedded static, Hugo or Jekyll variant. General swatch processing excludes this destination. Generated content starts with `# Managed by Tailor: pages`. An existing unmarked destination is an ownership conflict, including under recut. `always` compares resolved content. `first-fit` creates a missing workflow and preserves compatible content unless recut applies. `never` never writes and requires an existing workflow. Protected workflows must match the generated YAML semantics, including execution and permissions. Comments and formatting can differ. An incompatible protected workflow blocks Pages setup before writes. Generator changes replace the same marked destination. Other workflows remain untouched.

Workflows run only on selected-branch pushes, never pull requests or untrusted refs. Use separate build and deployment jobs with `needs`, GitHub-hosted Ubuntu and Pages concurrency. Build permissions are `contents: read`. Deployment permissions are only `pages: write` and `id-token: write`. Configure-pages metadata supplies Hugo's full base URL and Jekyll's base path and origin, including project prefixes and custom domains. The environment URL comes from deploy-pages output. Pin verified full commit SHAs with release comments for checkout 7.0.1, configure-pages 6.0.0, upload-pages-artifact 5.0.0, deploy-pages 5.0.1 and setup-ruby 1.321.0.

Read `/repos/{owner}/{repo}/pages`. Create confirmed absence with POST and exactly `{"build_type":"workflow"}`. Migrate legacy publishing with PUT. Never send `source`, local path or branch. Creation returns 201, updates return 204. Never delete Pages. A 404 alone does not prove absence. Prove access through an accessible public repository, classic `repo` scope in `X-OAuth-Scopes`, and an admin or maintain role. Environment creation and branch-policy writes require admin access. Fine-grained token grants remain unknown and skip Pages with insufficient scope. Skip private, unavailable and denied reads. Re-read creation conflicts once. Ordinary validation errors stop, and failed writes never report success.

Preflight availability and environment compatibility before dependent Pages changes. Create only `github-pages` when absent, with a custom deployment policy for the selected branch. Preserve secrets, reviewers, timers and unrelated patterns. Add an absent branch to an existing custom policy. If protected-branch-only rules exclude it, report the conflict without weakening protection. Never change repository-wide workflow permissions or bypass Actions policy. Report restrictions that block the required actions.

An omitted `cname` preserves the remote domain. An empty string clears it through API `cname: null`. A non-empty value is a DNS domain without scheme or path. No `CNAME` file is required. Set the domain through a subsequent update before DNS guidance. Enforce HTTPS when the certificate covers the effective domain and GitHub accepts enforcement. Pending DNS, domain verification and certificate issuance are pending states, not failed deployments. Re-read and retry on a later `alter`. Do not suppress unrelated 422 errors.

Show only remaining manual work. A subdomain uses CNAME target `<owner>.github.io`, without a repository path. Apex domains link GitHub's published DNS instructions. Ownership verification links account or organisation Pages settings and explains the TXT step. Account-level verification and DNS administration remain manual. A pending certificate tells the user to wait and rerun `tailor alter`.

Explicit `repository.homepage`, including an empty value, takes precedence. Otherwise replace the live homepage only when it equals this repository's GitHub URL. Use the effective custom-domain URL or GitHub's reported Pages URL. Preserve other values, including an empty live homepage. Track an inferred homepage separately from its value through config merging and round trips. The value-bound inline comment `# tailor: inferred homepage <URL>` marks an inferred value. Removing the comment or changing the URL makes the value explicit. Apply homepage changes only after successful Pages reconciliation, and never restore an inferred repository URL on later runs.

When enabled, append the escaped, anchored generator output rule to `.gitignore`: `/pages/public/` for Hugo or `/pages/_site/` for Jekyll with the default path. Static adds nothing. Preserve text and prior rules, avoid duplicates, and never untrack files. An explicit `.gitignore` mode of `never` skips the addition with a result. Add the rule after ordinary swatches so recut cannot remove it.

Insert Pages reconciliation after variables and before the licence, preserving other stage order. Return confirmed partial progress after failures. `baste` previews settings, workflow, environment, ignore and homepage changes without writes. `measure` remains local and excludes the development workflow from health checks. Its config comparison includes the registered Pages path, even when Pages is disabled. Authenticated `docket` already verifies the token with `GET /user`. Pages adds no requests to either inspection command. Disabled Pages never deletes sites, workflows or environments. Private Pages, paid features, self-hosted runners and general environment management remain out of scope.

**Repository Actions variables**: The optional top-level `variables` section is a sequence of `name` and `value` entries. These values are non-secret. Tailor creates missing declared variables and updates changed declared values. It leaves undeclared variables unchanged. Tailor never deletes or renames variables.

Names match `[A-Za-z_][A-Za-z0-9_]*`. The `GITHUB_` prefix is reserved, without case sensitivity. Tailor rejects duplicate names without case sensitivity. It adds no maximum name length and accepts the `RUNNER_` prefix. A difference in name casing alone causes no write.

Both fields must be YAML string scalars. Tailor rejects missing or null values, numbers, Booleans, nested values, unknown entry keys, malformed entries and a map instead of a sequence. Quoted strings, explicitly string-tagged scalars and block strings are valid. An explicit `""` is a valid empty value.

Tailor compares values byte-for-byte without trimming, case conversion or token substitution. Config output always double-quotes declared values. Write/load round trips preserve whitespace, newlines, quotes, backslashes and Unicode.

Tailor accepts at most 500 declarations and 48 × 1024 UTF-8 bytes per value. The byte limit is Tailor's binary interpretation of GitHub's documented 48 KB limit. GitHub enforces total repository capacity and concurrent changes. GitHub also limits organisation and repository variables to 256 KB combined per workflow run. Tailor does not query organisation variables to enforce that limit.

An omitted section or `variables: []` makes no variable requests. `fit` writes only a commented non-secret example with quoted values and an explicit empty value. It never reads live variables. Default merging and `--recut` preserve declarations without adding defaults. Secrets, organisation variables and environment variables are outside this feature.

Tailor reads every page of `GET /repos/{owner}/{repo}/actions/variables?per_page=30&page=N` before any variable write. The response must contain `total_count` and `variables`. A failed page or malformed response leaves no usable collection. Tailor never treats an incomplete list as proof that a variable is absent.

Missing variables use `POST /repos/{owner}/{repo}/actions/variables` with exactly `name` and `value`, including empty values. Changed variables use `PATCH /repos/{owner}/{repo}/actions/variables/{name}` with only `value` and the escaped live name. Successful responses are `201` for creates and `204` for updates. Repeated application makes no writes when declared values already match.

Variables run after labels and before the licence and swatches. Without repository context, Tailor skips variables with a warning. `measure` stays local and makes no variable requests.

Variable results follow labels and precede file results. They use the `variable.` prefix. `baste` previews creates and old/new value differences with quoted, escaped strings and makes no writes. `alter` reports each successful create or update. Skipped or failed writes never appear as successful.

List access failures skip variable management without writes. Individual write access failures skip the affected variable and continue. Rate limits, `422`, transport errors and other hard failures stop the command. A PATCH `404` is a hard error, not a reason to create the variable. After partial writes, errors include applied and remaining counts. Tailor does not retry writes automatically.

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
| `.github/workflows/tailor-pages.yml` | `always` |
| `pages/index.html` | `first-fit` |
| `pages/style.css` | `first-fit` |
| `pages/theme.js` | `first-fit` |
| `pages/icon.svg` | `first-fit` |
| `wiki/Home.md` | `first-fit` |
| `wiki/_Sidebar.md` | `first-fit` |
| `wiki/_Footer.md` | `first-fit` |
| `.github/workflows/tailor-wiki.yml` | `always` |
| `justfile` | `first-fit` |
| `cubic.yaml` | `first-fit` |
| `flake.nix` | `first-fit` |
| `.golangci.yml` (only when `languages.go: true`) | `first-fit` |
| `.goreleaser.yaml` (only when `languages.go: true`) | `first-fit` |
| `.github/workflows/build-go.yml` (only when `languages.go: true`) | `first-fit` |
| `Dockerfile` (only when `languages.go: true`) | `first-fit` |
| `.tailor.yml` | `always` |

**Swatch Categories**: Each swatch is designated either `health` or `development`. This designation is an internal attribute used by `measure` to scope its file presence checks.

**Health swatches** (community health files tracked by GitHub):

- `LICENSE` (fetched via the GitHub REST API `GET /licenses/{id}`, not an embedded swatch)
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

- `.github/workflows/tailor-pages.yml` (only when Pages is enabled)
- `wiki/Home.md` (only when wiki publishing is enabled)
- `wiki/_Sidebar.md` (only when wiki publishing is enabled)
- `wiki/_Footer.md` (only when wiki publishing is enabled)
- `.github/workflows/tailor-wiki.yml` (only when wiki publishing is enabled)
- `.gitignore`
- `.envrc`
- `flake.nix`
- `justfile`
- `.golangci.yml`
- `.goreleaser.yaml`
- `.github/workflows/build-go.yml`
- `Dockerfile`
- `cubic.yaml`
- `.tailor.yml`

#### Static Pages links

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

#### Static Pages starter

When Pages is enabled with `generator: static`, Tailor creates a starter in a missing or empty `pages.path` (default `pages`). The four embedded sources are `pages/index.html`, `pages/style.css`, `pages/theme.js` and `pages/icon.svg`. Their destination names stay fixed beneath `pages.path`.

The starter uses the default azure theme from µCSS 1.4.9 (`@digicreon/mucss@1.4.9/dist/mu.css`), Work Sans and Fira Code, with pinned CDN dependencies. Custom presentation uses µCSS theme variables for light, dark and system modes. It needs no build step. Edit the HTML for your introduction, features and installation instructions. Replace `icon.svg` to use your project icon. One icon supplies the header, footer and favicon.

The starter includes optional examples for a three-slide CSS scroll-snap gallery, a screenshot with a caption, a lazy YouTube embed and a native HTML FAQ. The gallery uses keyboard-accessible links without automatic rotation. Video playback is user-initiated. Users edit or remove these sections directly in HTML, with no additional configuration fields.

Store examples use publisher-hosted graphics for App Store, Google Play, Mac App Store, Microsoft Store, Snap Store, Flathub, Steam and itch.io. They preserve the original colours and proportions and contain no placeholder product links. Users add their own product URLs. These remote graphics are not embedded swatches or pinned dependencies.

Tailor substitutes the repository name, configured description and repository URLs only when it creates the starter. The copyright year is dynamic. Edit the copyright holder in the footer. No author identity is inferred from the repository owner.

The starter files default to `first-fit`. Tailor never replaces an existing site with starter content, including with `--recut` or `always`. It updates only the opted-in marked sections. A non-empty directory without `index.html` remains an error. Missing assets in an existing site are not added. If any required starter file uses `never`, creation stops before writes. Supply an existing site to use that mode.

`baste` reports all four proposed files without writing them. Disabled or skipped Pages creates no files. Hugo and Jekyll do not use this starter. The starter sources are excluded from generic swatch processing and use the Pages stage instead.

#### Static Pages navigation

Static sites can opt into repository navigation with one pair of standalone `<!-- tailor:navigation:start -->` and `<!-- tailor:navigation:end -->` markers. These enclose list items inside the header navigation list. No markers leaves navigation unmanaged. Partial, repeated, reversed or inline markers stop preflight before writes.

Render the README link first, using `?tab=readme-ov-file`. When `repository.has_wiki` is true, label the README `Overview` and append `Documentation` pointing to `/wiki`. Otherwise, label the README `Documentation`. When `repository.has_discussions` is true, append `Discussions` pointing to `/discussions` after both documentation links. Always append `Download` pointing to `/releases` in the header. Omitted or false flags omit the corresponding feature link. Build URLs from the repository that Tailor manages.

Navigation is independent of `pages.links` and needs no new configuration field. Both marked sections share one page result and write. Preserve all bytes outside the sections, enforce the existing input and output limits, and keep previews read-only. Disabled or skipped Pages and Hugo or Jekyll sites receive no navigation changes.

The footer uses `<!-- tailor:footer-navigation:start -->` and `<!-- tailor:footer-navigation:end -->` inside its Resources list. It follows the same README, Wiki and Discussions order. Append `Support` linking to `/blob/HEAD/SUPPORT.md` when either Wiki or Discussions is disabled or omitted. Hide Support only when both are enabled. Releases stays in the separate Project list.

The optional `<!-- tailor:license:start -->` and `<!-- tailor:license:end -->` markers enclose the footer licence list item. Render its label as `License` and its URL as the managed repository URL plus `?tab=<license>-1-ov-file`. URL-encode the configured identifier. An empty `license` or `none` clears this section. Unmarked links stay unchanged. Apply the same preflight, preview and file-preservation rules as navigation, with one shared page write.

## Commands

Commands divide into three categories: bootstrap commands, which create the project and initial configuration; apply commands, which read `.tailor.yml` and modify project files; and inspection commands, which check the project without modifying anything.

**Bootstrap commands**: `fit`
**Apply commands**: `alter`
**Inspection commands**: `baste`, `measure`, `docket`

### Output formats

Tailor has two display contracts. `--format=auto` is the default. It uses rich output when standard output is a terminal. It uses plain output for redirected output, piped output, `TERM=dumb`, and `--format=plain`.

Plain output is the stable text contract that each command documents below. Its bytes, order, escaping, streams, partial-write reports, and exit codes do not change. Plain output has no ANSI or cursor controls. Help, version, parse errors, warnings, and fatal errors keep their existing streams and formats.

Rich output uses typed results before flat formatting. Each result records the command, context, domain, category, outcome, action, name, before value, after value, reason, and provenance. Notices, ordered guidance, and summary totals are also typed. The renderer does not infer status from display text.

Rich reports use this order: command and context summary, all planned or applied changes, all items that need attention, policy-preserved items, already-matching items, notices, then ordered guidance. Domain is the second grouping level. The default report collapses policy-preserved items by reason and unchanged items by domain. `--verbose` expands every item inside its outcome and domain groups. The default report never hides a mutation or an actionable blocker.

Each group is a bordered card. At 72 columns or more, a row keeps its highlighted value beside its name and category summaries use count grids. Below 72 columns, values move below names and category counts stack. Borders fit the detected width. An ASCII terminal uses `+`, `-`, and `|` borders, `*` for active work, `ok` for completion, `~` for a change, and `!` for attention.

Colour does not carry meaning. Words, symbols, and counts identify every outcome. The Tailor heading uses violet. Planned alterations use cyan, applied and matching results use green, attention uses amber, and errors use red. Policy preservation uses neutral violet, not warning colour. `--color=auto|always|never` controls colour independently from layout. `NO_COLOR` disables automatic colour. Tailor escapes control characters in untrusted presentation fields before styling them.

`--quiet` prints one final summary while warnings and fatal errors remain on standard error. `--no-progress` disables live progress without changing the final rich report. `--format=plain` disables progress regardless of the other flags.

### Progress

`fit`, `baste`, `alter`, `measure`, and `docket` publish typed stage events around meaningful local and GitHub operations. A rich command writes one inline Bubble Tea display to standard error and never enters the alternate screen. Stage text appears promptly. The spinner starts only after the active stage lasts about 300 milliseconds. Tailor uses one indeterminate spinner because the GitHub request count can change. Tailor does not show a percentage unless planning supplies a fixed total.

The live display contains at most three lines and stops before final standard output. Tailor suspends the display around warnings and errors, then redraws the active stage. Non-terminal standard error, `TERM=dumb`, `--format=plain`, and `--no-progress` emit no animation or cursor controls. With `--verbose`, stage diagnostics can use append-only records. Durable command results use standard output. Warnings and fatal errors use standard error.

### `fit <path>`

Creates a project directory and writes `.tailor.yml` from the embedded `swatches/.tailor.yml` defaults, including for existing projects. Only repository `description` and `homepage` come from GitHub. `fit` does not copy swatch files or apply settings. After `fit`, change into `<path>`, review `.tailor.yml`, and run `baste` before `alter`.

The defaults can disable an existing wiki, Code Quality and immutable releases, and replace an existing `Tailor` ruleset when `alter` applies them. Edit the generated config before `alter` to preserve different settings. Topics stay unmanaged because the embedded defaults omit them.

The default swatch set contains 29 registered destinations:

- `.github/workflows/tailor-pages.yml`
- `pages/index.html`
- `pages/style.css`
- `pages/theme.js`
- `pages/icon.svg`
- `wiki/Home.md`
- `wiki/_Sidebar.md`
- `wiki/_Footer.md`
- `.github/workflows/tailor-wiki.yml`
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
- `.golangci.yml`
- `.goreleaser.yaml`
- `.github/workflows/build-go.yml`
- `Dockerfile`
- `flake.nix`
- `.gitignore`
- `.envrc`
- `cubic.yaml`
- `.tailor.yml`

A `license` key is included in `.tailor.yml` by default (`license: BlueOak-1.0.0`). Use `--license=<id>` to select a different licence or `--license=none` to opt out entirely.

`--license=<id>` records the licence identifier in `.tailor.yml`. Defaults to `BlueOak-1.0.0` if not specified. `--license=none` records `license: none`, opting out of licence creation. The identifier is used to fetch licence text via the GitHub REST API (`GET /licenses/{id}`) at `alter` time; any licence supported by the GitHub API is valid. `fit` does not validate the identifier - validation is deferred to `alter`.

`--description=<text>` sets the `description` field in the `repository` section of `.tailor.yml`, overriding any value from GitHub. `fit` does not apply the description - it is applied at `alter` time.

**Repository settings resolution at `fit` time**: `fit` detects repository context by querying GitHub remotes in `<path>`. If a GitHub remote exists, the project has repository context. If no remote is found, no repository context exists. Repository context detection reads git remotes (via `go-gh`), so `git` must be present when a GitHub remote exists - which is always the case in practice, since the remote implies a git repository.

When several remotes exist, Tailor prefers `origin`, then `github`, then `upstream`, then any other remote. This deliberately reverses the gh CLI order: Tailor manages the repository the user administers and pushes to, so in a fork clone the fork (`origin`) wins over the parent (`upstream`). A default repository recorded by `gh repo set-default` (the `remote.<name>.gh-resolved` git config) overrides this priority. This selection applies to every command that uses repository context (`fit`, `alter`, `baste`, `docket`).

When repository context exists, `fit` reads `GET /repos/{owner}/{repo}` and copies only `description` and `homepage`, exactly, including empty strings. An explicit `--description` overrides the GitHub description. Empty metadata does not trigger a fallback to the repository name or URL. Non-empty imported homepages retain the inferred marker for the Pages safeguards described above. An imported empty homepage stays explicit and unchanged by Pages.

All other managed settings come from the embedded defaults. `fit` makes no import-only reads for security features, Actions workflow permissions, immutable releases, CodeQL, Code Quality, or the `Tailor` ruleset. When no repository context exists, `description` defaults to the project directory name unless `--description` is explicit, and `homepage` stays omitted. `alter` applies declared settings regardless of their source.

This policy applies only when `fit` creates a config. Existing configs retain their declared values, including values that an earlier `fit` copied from GitHub. Default merging remains append-only, including with `--recut`. Edit an existing config directly to change those values.

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
- `.tailor.yml` at `<path>/.tailor.yml`, with embedded defaults for all managed settings and each active default swatch at its default alteration mode. The licence flag and metadata rules above provide the only value overrides. Both `code_scanning` and `code_quality` use `languages: []`. The config starts with `# Initially fitted by tailor on <DATE>` (YYYY-MM-DD, no time).

### `alter`

Applies swatch alterations to the local project.

`alter` checks at startup that an authentication token is present for the effective host and exits with an error if the token is missing. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed. The API verification of the token (`GET /user`) happens after config parsing and normalisation, before the config write and any other local file change; if the effective host rejects the token, `alter` exits with the API error. Implementation Note 9 gives the full execution order.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Apply and override first-fit protection
```

Behaviour:

- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- **Retired workflow migration**: before strict path and mode validation, `alter` removes both retired paths from the in-memory config. The paths are `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. This migration accepts the historical `triggered` mode only on these retired entries. The migration ignores the entry mode and the mode of the `.tailor.yml` swatch.
- **Config update**: after migration, Tailor normalises the security prerequisites (automated security fixes, secret scanning push protection, and secret scanning non-provider patterns) and emits their warnings before validation. `alter` then writes a changed config once. The write uses a `# Refitted by tailor on <DATE>` header comment (YYYY-MM-DD). It combines security prerequisite normalisation and all retired-entry removals with built-in defaults merged in the same run. The write occurs before repository API changes, except the early wiki enablement described above. Wiki readiness must pass first. If the config did not change, `alter` does not write it. The default merge runs when `.tailor.yml` has `alteration: always`. The `alteration: first-fit` mode skips the merge. Sections restored by the merge are managed in the same run, so omission alone does not disable them. Security prerequisite normalisation is independent of the config swatch mode. See "Header comment" below for the comment format. The merge rules are:
  - **Swatches**: appends each missing default swatch with the default alteration mode. The merge does not modify active entries.
  - **Repository settings**: fills nil fields only from built-in defaults; never overwrites non-nil fields. This appends missing security settings with the built-in defaults (`true` for the Boolean settings and `enabled` for the secret scanning settings) and preserves explicit `false` or `disabled` values except for the automated security fixes, secret scanning push protection, and secret scanning non-provider patterns prerequisites described above. `Description`, `Homepage`, and `Topics` are excluded from this merge because they are project-specific.
  - **Actions policy**: adds the complete default section when absent. Otherwise, it fills missing core fields and missing or null approval policies without changing explicit values. It fills missing selected-action fields only when the effective policy is `selected`.
  - **Code scanning**: adds the complete default section when absent. Otherwise, it fills missing fields without changing explicit values.
  - **Code Quality**: adds the complete default section when absent. Otherwise, it fills missing fields without changing explicit values.
  - **Ruleset**: adds the complete default section when absent. Otherwise, it fills missing fields without changing explicit values.
  - **Labels**: populated only when the labels section is entirely absent or empty (all-or-nothing). If the config already has any labels defined, no defaults are merged.
  - **Variables**: no defaults. Merging preserves existing declarations and empty values.
  - **Languages**: preserves an absent section, an absent `go` key, and explicit values. Go swatch entries follow the swatch merge rule.
- For repository settings: if a `repository` section is present in `.tailor.yml`, reads the current repository settings via `GET /repos/{owner}/{repo}` and additional endpoints, compares each declared field against the live value, and applies changes via `PATCH /repos/{owner}/{repo}` plus separate API calls for fields with dedicated endpoints. Repository settings are the first API stage after local migration cleanup. If no GitHub repository context exists (no remote), repository settings are skipped with a warning. `--recut` has no special effect on repository settings - they are always applied declaratively.
- For Actions policy: after any default merge, if an `actions` section is present, reads the current policy, compares each declared field, and applies only endpoint groups that differ. The Actions policy runs after repository settings and before code scanning. If the section remains absent because default merging is disabled, Tailor makes no Actions policy calls. `--recut` has no other special effect.
- For code scanning: if a `code_scanning` section is present, reads the current default setup via `GET /repos/{owner}/{repo}/code-scanning/default-setup`, compares each declared field, and writes only the declared fields that differ via `PATCH /repos/{owner}/{repo}/code-scanning/default-setup`. An empty `languages` list sends no `languages` field. Code scanning runs after the Actions policy and before Code Quality. A `409` or `403` response produces a skip result and does not stop the command.
- For Code Quality: if a `code_quality` section is present, reads the current setup via `GET /repos/{owner}/{repo}/code-quality/setup`, compares each declared field, and writes only the declared fields that differ via `PATCH /repos/{owner}/{repo}/code-quality/setup`. The `languages` and response rules follow code scanning. Code Quality runs after code scanning and before the ruleset.
- For the ruleset: if a `ruleset` section is present, lists the repository rulesets via `GET /repos/{owner}/{repo}/rulesets`, finds the ruleset named `Tailor`, and reads it via `GET /repos/{owner}/{repo}/rulesets/{ruleset_id}`. When the ruleset is absent, creates it via `POST /repos/{owner}/{repo}/rulesets`. When a managed field differs, replaces the complete ruleset via `PUT /repos/{owner}/{repo}/rulesets/{ruleset_id}`. The ruleset runs after Code Quality and before labels. A `403` response, or a read that omits `bypass_actors`, produces a skip result and does not stop the command. A `422` response stops the command.
- For labels: if a `labels` section is present in `.tailor.yml`, reads the current labels via paginated `GET /repos/{owner}/{repo}/labels`, diffs desired vs current using case-insensitive name matching, creates missing labels via `POST`, and updates changed labels (colour or description differs) via `PATCH`. Labels present on GitHub but absent from config are left untouched. Labels are applied after the ruleset and before variables, licences and swatches. If no GitHub repository context exists (no remote), labels are skipped with a warning.
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

`baste` checks at startup that an authentication token is present for the effective host and exits with an error if the token is missing. It then reads `.tailor.yml` in the current working directory. No upward traversal is performed. The API verification of the token (`GET /user`) happens after config parsing and normalisation, in the same order as `alter`; if the effective host rejects the token, `baste` exits with the API error. `baste` writes nothing in any case.

```bash
tailor baste
```

Behaviour:

- If `.tailor.yml` is missing or malformed, exits immediately with the error described in Error Handling.
- Before strict validation, `baste` applies the same in-memory retired workflow migration and security prerequisite normalisation as `alter`. It accepts historical `triggered` entries only for the two retired paths. It emits the normalisation warning and reports `would update: .tailor.yml` without writing.
- `baste` performs the same comparison and file-safety checks as `alter` but writes and removes nothing. It reports what `alter` would do.

Output contract - repository settings, Actions policy settings, code scanning settings, Code Quality settings, and ruleset settings are shown first, then labels, then variables, then file results. Actions policy fields use the `actions.` prefix. Code scanning fields use the `code_scanning.` prefix, and Code Quality fields use the `code_quality.` prefix, for example `code_scanning.state = configured` and `code_quality.state (already not-configured)`. Ruleset fields use the `ruleset.` prefix, for example `ruleset.enforcement = active`, `ruleset.rules.pull_request (already enabled)`, `ruleset.rules.required_status_checks.parameters.required_status_checks = Sentinel 👁️`, `ruleset.rules.code_scanning (already disabled)`, and `ruleset.rules.code_scanning.parameters.code_scanning_tools = CodeQL (errors, high_or_higher)`. File results include the licence, the `.tailor.yml` default merge, and swatches. `baste` uses planned labels. `alter` and `alter --recut` use completed labels, and report each label only after the change succeeds. Informational and access-warning labels are the same for all three commands.

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
- `warning` - health diagnostic that requires attention but is not a missing swatch. Three cases are recognised: `LICENSE` exists but contains known unresolved placeholder tokens (e.g. `[year]`, `[fullname]`, `{project}`), `LICENSE` exists but was not inspected because it exceeds 1 MiB or could not be read (annotated `(not inspected: exceeds 1 MiB)` or `(not inspected: read failed)`), and `README.md` is absent from the project root. A warned path appears once in the output and does not also appear as `present`
- `present` - health file exists on disk
- `not-configured` - default swatch whose destination is not covered by any entry in `.tailor.yml`; the default swatch will not be applied until added
- `config-only` - swatch in `.tailor.yml` whose destination is not covered by any entry in the built-in default set. This arises when a swatch is removed from the built-in defaults in a newer tailor release but the project's `.tailor.yml` still references it. `alter` rejects unrecognised paths, except that it automatically migrates the two fixed retired workflow paths
- `mode-differs` - swatch whose destination appears in both `.tailor.yml` and the default set, but with a different alteration mode; the inline annotation shows both values

Output order: `missing`, `warning`, `present`, `not-configured`, `config-only`, `mode-differs`. Within each category, entries are sorted lexicographically by destination path. The category label is padded to a fixed width of 16 characters (the length of `not-configured:`) for consistent column alignment. For `warning` entries, the detail annotation (e.g. `(contains unresolved placeholders)`) is separated from the path by a single space, following the same annotation style as `mode-differs`. For `mode-differs` entries, the annotation (e.g. `(config: first-fit, default: always)`) is separated from the destination path by a single space; no additional fixed column alignment is applied to the annotation. Health file checks are always performed and reported regardless of whether `.tailor.yml` is present; config-diff categories (`not-configured`, `config-only`, `mode-differs`) are shown only when `.tailor.yml` is present.

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
- `auth` displays `authenticated` or `not authenticated` based on whether a valid token can be resolved for the host of the detected repository, or for the `go-gh` default host (`GH_HOST`, falling back to `github.com`) when no repository context exists.
- Does not read `.tailor.yml` and does not require it to be present.

## Error Handling

**Unrecognised swatch `path` in `.tailor.yml`**: `alter` and `baste` remove the two retired paths in memory before strict validation. Any other path must match an embedded swatch. The error identifies the unrecognised name and lists all valid paths. `alter` validates all remaining paths before a write or removal. An unknown non-retired path therefore causes no disk changes.

**Invalid alteration mode**: `always`, `first-fit`, and `never` are the only active modes. The historical `triggered` value is valid only on a retired workflow entry. This exception lets `alter` or `baste` remove that entry. Any other use causes a validation error before disk changes.

**Licence fetch failed**: if `GET /licenses/{id}` returns an error during `alter` (e.g. unrecognised licence identifier), tailor exits with the API error.

**Destination path not writable**: tailor exits with an error showing the full path that could not be written.

**Retired workflow path is unsafe**: if a parent component is a symlink, Tailor exits without following the path. If the destination is a directory, Tailor exits without removing content. If the destination is a symlink, Tailor removes only the link. A removal error stops later operations. Tailor reports `removed` only after a successful removal.

**`.tailor.yml` malformed or missing**: if `alter` or `baste` reads a missing or malformed `.tailor.yml`, it exits with a clear message directing the user to run `fit` to create a valid configuration, or edit `.tailor.yml` directly to correct it.

**`.tailor.yml` is not a valid config file**: Tailor rejects `.tailor.yml` if it is not a regular file or exceeds 1 MiB. The command exits before YAML parsing.

**`always` swatch modified locally**: for embedded swatches other than `.tailor.yml`, Tailor treats the file as changed whenever the SHA-256 of the resolved swatch content differs from the on-disk file. `alter` overwrites it unconditionally. Tailor does not preserve local edits to these `always` swatches; use `first-fit` alteration mode if local modifications must be retained after the initial fit. `--recut` overrides `first-fit` protection but still skips `never` swatches. Wiki and static Pages starter files are exceptions: existing destinations remain unchanged. The licence file is never overwritten regardless of flags. `.tailor.yml` uses no content hash: `always` removes retired entries and appends missing defaults. Other existing entries are never modified or overwritten.

**Duplicate path in `.tailor.yml`**: `alter` and `baste` remove retired entries before duplicate validation. If active entries share a path, the command identifies the conflict and exits before disk changes.

**Not authenticated**: if no authentication token can be resolved for the host of the detected repository, or for the `go-gh` default host (`GH_HOST`, falling back to `github.com`) when no repository context exists (neither `GH_TOKEN`/`GITHUB_TOKEN` environment variable, `gh` config file, nor `gh` keyring), `fit`, `alter`, and `baste` exit with: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run `gh auth login`."

**Token rejected**: if a token is present but the effective host rejects the `GET /user` verification request, `fit`, `alter`, and `baste` exit with the API error before any local file change.

**`{{GITHUB_USERNAME}}` resolution failed**: `{{GITHUB_USERNAME}}` is resolved via `GET /user`. If this call fails, `alter` exits with the API error. Unlike repo-context tokens, `{{GITHUB_USERNAME}}` depends on the authenticated user, not the repository, so it cannot be deferred.

**Repo-context tokens unresolved**: `{{ADVISORY_URL}}` and `{{SUPPORT_URL}}` require a GitHub repository context. If the project has no GitHub remote (e.g. a brand-new project not yet pushed), these tokens are left unsubstituted silently. For `always` swatches (e.g. `SECURITY.md`), `alter` will resolve and substitute them on a subsequent run once the repository has a remote. For `first-fit` swatches (e.g. `.github/ISSUE_TEMPLATE/config.yml`), delete the file and re-run `alter`, or use `--recut`.

**Repository settings without repo context**: if `.tailor.yml` contains a `repository` section but the project has no GitHub remote (no repository context found), repository settings are skipped with a warning: "No GitHub repository context found. Repository settings will be applied once a remote is configured." Warning only; does not block swatch or licence processing.

**Repository settings API failure**: if any API call to apply repository settings fails (PATCH, PUT, or DELETE), `alter` exits with the API error. In the main settings stage, failure prevents immutable releases, Actions policy, code scanning, Code Quality, ruleset, labels, variables, Pages, wiki, licence, and swatch operations. Local config migration and retired file cleanup already occurred. Early wiki enablement is separate. Its failure prevents config writes and retired file cleanup too. If licence fetch fails after repository settings and labels have been applied, those changes are not reverted.

**Repository settings with insufficient scope**: When GitHub rejects a repository-setting read or write with an access error, Tailor skips the affected fields rather than exiting. `baste` reports `would skip (insufficient scope: token missing required scope)` and `alter` skips the operation. Other repository settings continue to be applied. Use a token with the required repository permissions. The `code_scanning` and `code_quality` fields follow the same skip rules. Their output uses the section prefix, in the form `code_scanning.state = configured` and `code_quality.state (already not-configured)`. A `409` produces `would skip (setup in progress)`, and a `403` produces `would skip (not available)`. The `ruleset` section follows the same skip rules with the `ruleset.` prefix. A `403` on a ruleset read or write produces `would skip (not available)`. A ruleset read that omits `bypass_actors` produces `would skip (insufficient scope)` for the section. A `422` on a ruleset write stops the command with the API error, because the cause is the config.

**Actions policy failure**: Access errors from Actions policy reads or writes produce `would skip (insufficient scope)` results for the affected fields or operation, unless a prior write changed the failure state. During the enabled `all` to enabled `selected` restriction, a hard first core failure leaves `all` active. A later selected-policy failure leaves `selected` active and preserves existing SHA pinning. A final SHA write failure leaves the selected restrictions and SHA pinning active. Each hard failure stops the command. After Tailor disables Actions for another transition to `selected`, a selected-policy or final core write failure stops the command and leaves Actions disabled. Other errors stop the command. An invalid `allowed_actions` value or selected-action combination stops validation before any write.

**Unrecognised repository setting**: if `.tailor.yml` contains a field in the `repository` section that is not in the supported settings list, `alter` exits with an error identifying the unrecognised field and listing all valid repository setting field names.

**`fit` repository settings query failed**: if `fit` detects a GitHub remote but the subsequent API call to read repository settings fails (e.g. insufficient permissions, network error), `fit` exits with the API error. The user can re-run `fit` after resolving the issue, or create `.tailor.yml` manually.

## Configuration

### `.tailor.yml`

`.tailor.yml` has twelve top-level sections: `license`, `repository`, `immutable_releases`, `actions`, `code_scanning`, `code_quality`, `ruleset`, `labels`, `variables`, `pages`, `languages`, and `swatches`. The `actions` section is a map of repository Actions policy settings. The `code_scanning` section is a map of CodeQL default setup settings, and the `code_quality` section is a map of GitHub Code Quality settings. The `ruleset` section is a map of settings for the branch ruleset named `Tailor`. `path` values use the full path relative to `swatches/`, including the file extension where one exists. Extensionless files (e.g. `justfile`) are referenced as-is. The `repository`, `immutable_releases`, `actions`, `code_scanning`, `code_quality`, `ruleset`, `labels`, `variables`, and `pages` sections can be absent in a hand-written config. Default merging adds missing Actions defaults before policy management.

Tailor opens `.tailor.yml` relative to the project root. It does not search parent directories. The config must be a regular file no larger than 1 MiB (1,048,576 bytes).

The active configuration has 29 swatches and three alteration modes: `always`, `first-fit`, and `never`. Two paths are retired migration entries: `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. `alter` and `baste` remove every matching entry before strict path, duplicate-path, and mode validation. The historical `triggered` mode is accepted only on these removed entries. Retired paths are not active swatches. Tailor never adds them to a generated or refitted config.

Default (with `--license=BlueOak-1.0.0`). The `license` key varies by flag (`MIT`, `Apache-2.0`, `none`, etc.) - the rest of the generated file is identical regardless of licence choice:

```yaml
# Initially fitted by tailor on 2026-03-02
languages:
  go: false

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

# Pages is opt-in. Omission or enabled: false leaves Pages unmanaged.
# generator: static (default) creates a starter in an empty path. Hugo and Jekyll need an existing site.
# path: project-relative source directory, default pages.
# branch: omit to use the current repository default branch.
# cname: omit to preserve the domain, use "" to clear it, or set a domain.
pages:
  enabled: false
  generator: static
  path: pages
  links:
    website: https://wimpys.world/
    forum: ""
    x: ""
    bluesky: https://bsky.app/profile/wimpys.world
    mastodon: https://wimpysworld.social/@martin
    linkedin: https://linkedin.com/in/martinwimpress
    slack: ""
    discord: https://discord.com/invite/vUsydfP
    matrix: https://matrix.to/#/@wimpress:matrix.org
    youtube: https://youtube.com/WimpysWorld
    twitch: https://twitch.tv/WimpysWorld
    peertube: ""
    tiktok: ""
    podcast: https://linuxmatters.sh
    instagram: ""
    pixelfed: ""
    steam: https://steamcommunity.com/id/wimpress/
    itchio: https://wimpress.itch.io/
    patreon: ""
    github_sponsors: https://github.com/sponsors/flexiondotorg
    kofi: ""
    email: ""
    feed: ""
  # branch: main
  # cname: www.example.com
  # Static only. Omit links to leave the page unchanged; {} clears its marked links.
  # Use full HTTPS URLs, or a bare address for email. Empty values add no icon.
  # links:
  #   website: https://example.com
  #   email: hello@example.com

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

# Repository Actions variables are non-secret values.
# Tailor creates or updates only declared variables and leaves others unchanged.
# variables:
#   - name: DEPLOY_REGION
#     value: "eu-west-2"
#   - name: RELEASE_SUFFIX
#     value: ""

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

  - path: .golangci.yml
    alteration: first-fit

  - path: .goreleaser.yaml
    alteration: first-fit

  - path: .github/workflows/build-go.yml
    alteration: first-fit

  - path: Dockerfile
    alteration: first-fit

  - path: flake.nix
    alteration: first-fit

  - path: .gitignore
    alteration: first-fit

  - path: .envrc
    alteration: first-fit

  - path: cubic.yaml
    alteration: first-fit

  - path: .github/workflows/tailor-pages.yml
    alteration: always

  - path: pages/index.html
    alteration: first-fit

  - path: pages/style.css
    alteration: first-fit

  - path: pages/theme.js
    alteration: first-fit

  - path: pages/icon.svg
    alteration: first-fit

  - path: wiki/Home.md
    alteration: first-fit

  - path: wiki/_Sidebar.md
    alteration: first-fit

  - path: wiki/_Footer.md
    alteration: first-fit

  - path: .github/workflows/tailor-wiki.yml
    alteration: always

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
├── .golangci.yml
├── .goreleaser.yaml
├── Dockerfile
├── cubic.yaml
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── SECURITY.md
├── SUPPORT.md
├── flake.nix
├── justfile
├── go/
│   ├── justfile
│   └── dependabot-disabled.yml
├── .github/
│   ├── dependabot.yml  # Go modules follow languages.go
│   ├── workflows/
│   │   └── build-go.yml
│   ├── FUNDING.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   ├── config.yml
│   │   └── feature_request.yml
│   └── pull_request_template.md
├── pages/
│   ├── index.html
│   ├── style.css
│   ├── theme.js
│   ├── icon.svg
│   ├── static.yml
│   ├── hugo.yml
│   └── jekyll.yml
└── .tailor.yml
```

`.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically. `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically. If no GitHub repository context exists at `alter` time, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically. Resolution follows the same mechanism as `{{ADVISORY_URL}}` and constructs `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. `.github/dependabot.yml` covers `github-actions` and `nix`. Its `gomod` entry follows the Go selection rules above.

Licences are not embedded - they are fetched at `alter` time via the GitHub REST API (`GET /licenses/{id}`) and written verbatim to `LICENSE`.

## Retired Workflows

> [!IMPORTANT]
> After an upgrade, run `tailor baste` to preview cleanup, then run `tailor alter`. Tailor automatically removes both legacy config entries and files.

The retired paths are `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml`. Cleanup is unconditional. It applies to entries with any active mode and to historical `triggered` entries. Cleanup also removes a retired file when the config has no matching entry. `baste` reports the migration but changes nothing. `alter` and `alter --recut` write the cleaned config once, then remove each retired file that is present.

## Justfile Integration

The `justfile` swatch provides Tailor operations and workflow linting with `actionlint`. Its `first-fit` mode preserves local recipes during normal alterations. Projects can extend the file. `--recut` replaces it unless its mode is `never`.

With `languages.go: true`, the Go variant adds `build` and `test` recipes. It also adds golangci-lint to `lint`. See [Go ecosystem support](#go-ecosystem-support).

```makefile
# List available recipes
default:
    @just --list

# Alter tailor swatches
alter:
    @tailor alter

# Run linters
lint:
    @actionlint

# Check what tailor would change and measure
measure:
    @tailor baste
    @tailor measure
```

## Implementation Notes

1. **Overwrite detection**: SHA-256 hash comparison between the embedded swatch content (from the tailor binary) and the on-disk target file. SHA-256 comparison applies only to `always` swatches; `first-fit` swatches are skipped entirely if the destination exists, with no comparison performed. The on-disk file is overwritten only when this comparison shows a difference. For a token-bearing swatch configured as `always`, Tailor resolves the token before the hash comparison. `.tailor.yml` uses append-only config merging instead of a content hash. `--recut` bypasses the hash comparison for ordinary `always` and `first-fit` swatches, but still skips `never` swatches. Existing wiki and static Pages starter files remain unchanged.
2. **Interpolation (FUNDING.yml, SECURITY.md, and issue template config)**: These three swatches use token substitution. `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted at `alter` time from `GET /user`. `SECURITY.md` has `{{ADVISORY_URL}}` constructed from the repository context (owner/name). If no GitHub repository context exists, the token is left unsubstituted and resolved on a subsequent run. `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` constructed from the repository context, which produces `https://github.com/<owner>/<name>/blob/HEAD/SUPPORT.md`. If no GitHub repository context exists, the token is left unsubstituted. No per-swatch configuration is required. Licences are fetched via `GET /licenses/{id}` and written verbatim. Licences do not use token substitution.
3. **No versioning**: No swatch versions, always uses swatches from current tailor binary. Upgrading tailor will cause all `always` swatches to be re-evaluated against the new embedded content; files whose swatch content has changed will be overwritten on the next `alter` run.
4. **No global state**: All state is per-project in `.tailor.yml`
5. **No project registry**: Tailor has no awareness of its consumers. Projects pull from tailor, tailor does not track projects.
6. **Authentication via `go-gh`**: All project metadata, user metadata, licence content, and repository settings are resolved via `go-gh` (`github.com/cli/go-gh/v2`), the official Go library for GitHub CLI extensions. Token resolution follows the `go-gh` precedence order: `GH_TOKEN` environment variable, `GITHUB_TOKEN` environment variable, `gh` config file, `gh` keyring (via the `gh` binary). When `GH_TOKEN` or `GITHUB_TOKEN` is set, the `gh` binary is not required. The `gh` binary is needed only for `gh auth login` (establishing credentials) and as a fallback for keyring-based token access when no environment variable is set. Repository context detection reads git remotes via `go-gh`, so `git` must be present when a GitHub remote exists - but any directory with a GitHub remote already has `git` installed. If no token can be resolved, or the effective host rejects the token, `fit`, `alter`, and `baste` exit immediately with an error.
7. **CLI parsing**: [Kong](https://github.com/alecthomas/kong) is used as the command line parser.
8. **Repository settings via API**: Repository settings are applied via `PATCH /repos/{owner}/{repo}` with a JSON body constructed from the `repository` section of `.tailor.yml`, plus separate API calls for security features, topics, and Actions workflow permissions. The `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns` fields travel in the `security_and_analysis` object of the same PATCH body. The top-level `actions` section uses separate endpoints for core permissions, selected actions, artifact and log retention, and fork pull request contributor approval. The top-level `code_scanning` and `code_quality` sections use the code scanning default setup and Code Quality setup endpoints. The top-level `ruleset` section uses the repository rulesets endpoints (list, get, `POST`, and `PUT` on `/repos/{owner}/{repo}/rulesets`). Field names map directly to the GitHub REST API without translation, except for the `rules` map and its `enabled` keys, which are Tailor's form of the API `rules` list. Current settings are read via `GET /repos/{owner}/{repo}` and the relevant separate endpoints for `baste` comparison. All API calls use `go-gh`'s pre-authenticated REST client.
9. **Execution order**: after authentication and config parsing, `alter` removes retired entries in memory. It then normalises the security prerequisites (automated security fixes, secret scanning push protection, and secret scanning non-provider patterns) and emits their warnings before validation. Next, it completes required Go discovery and builder dependency checks before token verification with `GET /user`. It completes Pages and local wiki safety preflight and renders any required Go builder workflow before wiki enablement. For a declared public wiki, it enables `has_wiki` through the API if needed, then checks remote readiness and local adoption. A blocker stops the command before other writes. After readiness passes, it writes the changed config once and removes present retired workflow files. The same `GET /user` response resolves `{{GITHUB_USERNAME}}`, so verification adds no extra API call. It then applies repository settings, immutable releases, Actions policy, code scanning, Code Quality, the ruleset, labels, variables, Pages, wiki files, the licence, and active swatches in that order. `baste` uses `DryRun`. It reports wiki readiness blockers and continues the full preview, but writes and removes nothing. `alter` uses `Apply`, and `alter --recut` uses `Recut`.
