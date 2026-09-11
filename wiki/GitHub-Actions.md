# GitHub Actions

[Policy](#actions-policy) · [Fork approval](#fork-pull-request-approval) · [Retention](#artifact-and-log-retention) · [Policy changes](#policy-changes) · [Variables](#actions-variables)

## Actions policy

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

The selected-action fields require `allowed_actions: selected`. A selected policy must include `github_owned_allowed`, `verified_allowed`, and `patterns_allowed` after default merging. The `patterns_allowed` field replaces the full GitHub list. Tailor ignores list order during comparison.

[Default merging](Configuration#default-merging) adds a missing `actions` section and fills missing fields without changing explicit values. For `all` and `local_only`, Tailor does not add selected-only fields. For existing `selected` configs, Tailor keeps the policy and adds each missing selected-only field. A missing `patterns_allowed` field receives the six compatibility defaults listed in the [specification](https://github.com/wimpysworld/tailor/blob/HEAD/docs/SPECIFICATION.md).

Tailor preserves an explicit custom list or `patterns_allowed: []`.

To switch an existing config, set `actions.allowed_actions: all`. Remove `github_owned_allowed`, `verified_allowed`, and `patterns_allowed` from the `actions` section, then run `tailor alter`. SHA pinning is a separate choice. Tailor does not change an explicit `selected` policy automatically.

The default for `sha_pinning_required` is `false`. Default merging preserves explicit `true` and `false` values, including under `--recut`. To enable SHA pinning in an existing config, set `actions.sha_pinning_required: true`, then run `tailor alter`.

## Fork pull request approval

Fork pull request approval controls which external contributors need approval before their workflows run:

| Policy | Approval required for |
|---|---|
| `first_time_contributors_new_to_github` | First-time contributors who are new to GitHub |
| `first_time_contributors` | All first-time contributors (the Tailor default) |
| `all_external_contributors` | All external contributors |

`fit` writes the active `first_time_contributors` default. Default merging adds it when the approval object or policy is absent or null. An empty approval object also receives the default. Explicit policies remain unchanged, including under `--recut`.

This default applies with `all`, `local_only`, and `selected`.

To stop approval management, omit the approval policy and disable [default merging](Configuration#default-merging). Without a merge, an absent or null policy stays unmanaged. Omission alone does not stop management when a merge restores the default.

`baste` reports the requested policy without writes. `alter` updates a different live policy and makes no approval write when it already matches. Approval uses the separate `/repos/{owner}/{repo}/actions/permissions/fork-pr-contributor-approval` GET/PUT endpoint. The PUT body contains only `approval_policy`.

Approval alone requires no other Actions settings. It does not change workflow token permissions or private-fork permissions.

Tailor checks repository visibility before reading the approval policy. For private repositories, it reports `would skip (not available)` without an approval read or write, then continues other settings. Private repositories need no config change or disabled default merging.

Denied or unavailable reads, and unknown live policies, produce `would skip (insufficient scope)` and no approval write. Access failures affect only their endpoint group. Other API errors, including `422` responses, stop the command.

## Artifact and log retention

To manage artifact and log retention, add this field to your `actions` section:

```yaml
actions:
  artifact_and_log_retention:
    days: 30
```

Retention alone requires no core or selected-action fields. Tailor never adds a retention default during `fit`, default merging, or `alter --recut`. An omitted `days` field causes no retention API calls. Other Actions defaults still follow the merge rules above.

Tailor validates the integer range before any changes. It then checks GitHub's live `maximum_allowed_days` cap and rejects a higher value with the allowed maximum in the error. Denied or unavailable reads stay unknown and cannot cause a retention write. Missing or invalid live values stop the command.

`baste` reports the declared retention value when it differs, without writes. `alter` sends one update for a difference and none for a match. Tailor uses the separate [retention GET/PUT endpoint](https://docs.github.com/en/rest/actions/permissions#set-artifact-and-log-retention-settings-for-a-repository) and sends only `days` in the PUT body. Changes affect only new artifacts and logs, not existing artifacts, logs, caches, or owner policy.

## Policy changes

For an enabled transition from `all` to an enabled `selected` policy, Tailor first changes `allowed_actions` to `selected`. This write keeps SHA pinning enabled when the final policy disables it. Tailor then applies the complete selected rules and disables SHA pinning in a final core write. When the final policy keeps or enables SHA pinning, the first core write applies that value and Tailor omits the final write.

A hard first-write failure leaves `all` active. A selected-rule failure leaves the narrower `selected` policy active and preserves SHA pinning. A final SHA write failure leaves the selected rules and SHA pinning active. Each hard failure stops the command.

For other transitions from `all` or `local_only`, Tailor disables Actions before it applies the complete selected rules and the final core policy. For an existing selected policy, Tailor applies changed selected rules before any core broadening, including disabling SHA pinning. When an enabled policy combines selected broadening with SHA pinning or disabling Actions, Tailor disables Actions before both updates. Broadening means newly allowing GitHub-owned actions, verified actions, or patterns.

Tailor also disables Actions before any selected update whose final policy disables Actions. A later update failure leaves Actions disabled and stops the command. If Tailor cannot read an active selected policy, it does not enable Actions or disable SHA pinning. Organisation policy can restrict repository values.

Other access failures produce skip results, and other API failures stop the command.

## Actions variables

The optional `variables` section manages non-secret variables in the repository. Tailor creates or updates only declared variables. It leaves other variables unchanged.

```yaml
variables:
  - name: DEPLOY_REGION
    value: "eu-west-2"
  - name: RELEASE_SUFFIX
    value: ""
```

Names accept ASCII letters, digits and underscores. A name cannot start with a digit or `GITHUB_`. Names match without case sensitivity. Tailor rejects duplicate names.

A difference in name casing alone causes no update.

Each entry requires a string `value`. Quote values such as `"true"` and `"123"` to keep them strings. An explicit `""` sets an empty value. Tailor preserves whitespace, newlines and Unicode without substitution.

Tailor accepts up to 500 declarations and 48 × 1024 UTF-8 bytes per value. GitHub enforces total repository capacity. GitHub also limits organisation and repository variables to 256 KB combined per workflow run.

Omit `variables` or use `variables: []` to make no variable requests. `fit` writes only a commented example. It never reads live variables. Default merging and `alter --recut` preserve declarations without adding variables.

Tailor does not manage secrets, organisation variables or environment variables. `measure` stays local.

`baste` shows `variable.<name>` with quoted, escaped values and makes no writes. `alter` reports each successful create or update. Access failures skip the affected variables. Rate limits and other hard errors stop the command. After partial writes, the error includes applied and remaining counts.
