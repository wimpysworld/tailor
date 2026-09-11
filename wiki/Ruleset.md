# Ruleset

The top-level `ruleset` section manages one branch ruleset named `Tailor`. Tailor writes the complete ruleset whenever a managed field differs. Each write removes rules, bypass actors or conditions added by hand that the config does not declare. Put manual rules in a separate ruleset. Tailor never deletes the `Tailor` ruleset and never reads or writes any other ruleset.

Set `enforcement: disabled` to keep the ruleset on GitHub without enforcing its rules. To stop Tailor management without changing the live ruleset, omit `ruleset` and disable [default merging](Configuration#default-merging). Otherwise, a merge restores an absent section with active defaults and manages it in the same run.

Generated configs reproduce the GitHub UI default ruleset: restrict deletions, block force pushes, and require a pull request with one approval on the default branch, bypassed by the repository admin role.

## Fields

Start with the full `ruleset` declaration in the [config example](Configuration#config-file). After default merging, every declaration must contain:

- `enforcement`, `bypass_actors` and both `conditions.ref_name` lists.
- All six Boolean rule keys: `creation`, `update`, `deletion`, `required_linear_history`, `required_signatures` and `non_fast_forward`.
- `enabled` for `pull_request`, `required_status_checks` and `code_scanning`.
- Every parameter listed below for each enabled rule.

Default merging fills missing fields and preserves explicit values. When default merging is disabled, supply every required field yourself, including when `enforcement: disabled`.

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

## Required checks

Each key in the `rules` map is a GitHub rule type. The `enabled` key on `pull_request`, `required_status_checks`, and `code_scanning` is Tailor's own, because these three rules carry parameters that stay in the config while the rule is off. `required_status_checks` is off by default with an empty list, because a required check that never reports blocks every merge. Enable it per repository and name an aggregating job, for example one that depends on every other job and fails when any of them failed.

## Code scanning results

The `code_scanning` rule is the free route to the "Check runs failure threshold" merge gate on the Advanced Security settings page. That page setting has no repository API. The rule blocks a merge until the tool has results for both the pull request commit and the base branch, so a new repository has no results yet. `code_scanning` is therefore off by default with one entry, `CodeQL` at `errors` and `high_or_higher`, which matches the GitHub UI defaults "Only errors" and "High or higher".

Turning the gate on is a one-line change to `enabled`. Tailor does not cross-check the rule against the top-level `code_scanning.state`, because advanced setup also reports as the `CodeQL` tool while default setup stays `not-configured`.

## Conflicts and access errors

GitHub blocks a merge when the ruleset allows a method that the repository disables. When `allowed_merge_methods` names a method whose `repository` setting is `false` in the same config, Tailor shows `warning: ruleset allows <method> merging but repository.<field> is false` and continues without changing either value. When rulesets are not available to the repository, Tailor reports `would skip (not available)`. When the token lacks write access to the ruleset, Tailor reports `would skip (insufficient scope)`. When GitHub rejects the ruleset body, Tailor stops with the API error.
