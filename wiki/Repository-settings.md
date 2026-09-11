# Repository settings

## Repository fields

The `repository` section manages GitHub repository settings from `.tailor.yml`. Field names match the [GitHub REST API](https://docs.github.com/en/rest/repos/repos#update-a-repository) exactly (snake_case). Tailor uses the repository endpoint and separate feature endpoints on every `alter` run.

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

To stop repository settings management, omit the `repository` section and disable [default merging](Configuration#default-merging). An omitted field stays unmanaged only if default merging and the security prerequisites below leave it absent.

Generated configs expose all six security settings, three Boolean and three string. The built-in values are `true` for the Boolean settings and `enabled` for `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns`. When a GitHub remote exists, `fit` uses live values. Default merging appends missing security settings without changing explicit values.

The security prerequisite normalisations below are the exception: they can change an explicit `vulnerability_alerts_enabled: false` to `true`, and an explicit `secret_scanning: disabled` to `enabled`.

GitHub labels `can_approve_pull_request_reviews` as “Allow GitHub Actions to create and approve pull requests”. Tailor keeps the REST API field name because repository config keys match the API. Enabling the setting permits the repository `GITHUB_TOKEN` to create pull requests and submit approval reviews when the workflow has `pull-requests: write`. The setting does not permit merges, bypass branch rules, or affect personal access tokens or separate GitHub App tokens.

GitHub requires vulnerability alerts before automated security fixes. When automated fixes are enabled and alerts are absent or false, Tailor sets `vulnerability_alerts_enabled` to `true` and shows a warning. `alter` and `alter --recut` save the corrected `.tailor.yml` before security API changes. `baste` previews the config update without writing.

Tailor enables alerts first and disables automated fixes before alerts. If a prerequisite read is unknown, or its write fails or is skipped, Tailor skips the dependent write. A security `404` stays unknown unless Tailor can distinguish a disabled feature from denied access. Access failures produce warnings.

Other API failures stop the command.

Push protection requires secret scanning. When `secret_scanning_push_protection` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` and shows a warning. The write path matches the automated security fixes prerequisite. Tailor sends only the declared `security_and_analysis` keys, so other keys keep the value set in the GitHub UI.

When the token lacks admin access, GitHub omits the `security_and_analysis` block, and Tailor leaves all three values unknown with an access warning.

`secret_scanning_non_provider_patterns` turns on the generic patterns that GitHub maintains, such as private keys, database connection strings, and HTTP authorisation headers. The GitHub UI calls the feature "Generic patterns", and the API key keeps the older name "non-provider patterns". There is nothing to configure beyond the toggle. Custom patterns need a Secret Protection licence and stay out of scope.

Alerts from generic patterns have lower confidence than alerts from provider patterns. Push protection does not block them, so they raise alerts only. The feature is free on a public repository with secret scanning enabled. Generic patterns require secret scanning.

When `secret_scanning_non_provider_patterns` is `enabled` and `secret_scanning` is absent or `disabled`, Tailor sets `secret_scanning` to `enabled` and shows a warning, on the same write path as push protection.

## Topics

Declare the complete topic list in `repository.topics`. Tailor replaces all live topics with that list, so topics absent from the declaration are removed.

Omit `topics` to leave live topics unchanged. Set `topics: []` to clear them all. Default merging never adds topics.

Names must be unique and at most 50 characters long. Each name must start with a lowercase letter or digit and contain only lowercase letters, digits and hyphens.

## Immutable releases

Release immutability defaults to `immutable_releases.enabled: false`. Before enabling it, change release CI to upload every asset to a draft, then publish. Workflows that upload or replace assets after publication will fail. Enabling protects future releases only.

Disabling does not unlock existing immutable releases. Tailor skips disabling when the repository owner enforces immutability. An omitted section or `enabled` key stays unmanaged, including during default merging. For an existing repository, `fit` preserves the live setting.

The token needs Administration-read permission to inspect immutability and Administration-write permission to change it. Access failures report `would skip (insufficient scope)`. A conflict or other non-access error stops the command.

## Labels

The `labels` section manages GitHub issue labels from `.tailor.yml`. Tailor ships 12 default labels (the 9 GitHub defaults plus `dependencies`, `github_actions`, and `hacktoberfest-accepted`) with colours from the [Catppuccin Latte](https://catppuccin.com/palette/) palette.

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

Every label requires these fields:

| Field | Constraints |
|-------|-------------|
| `name` | Non-empty, at most 50 characters, no control characters |
| `color` | Six hexadecimal characters without `#` |
| `description` | Non-empty, at most 100 characters, no control characters |

Tailor accepts at most 1,000 labels and rejects duplicate names without case sensitivity.

Tailor matches live names without case sensitivity, but updates their casing to match the declaration. Colour comparison ignores hexadecimal letter case. Tailor creates missing labels and updates changed names, colours or descriptions. It never deletes repository labels.

[Default merging](Configuration#default-merging) restores all default labels when `labels` is absent or empty, then manages them in the same run. Any non-empty list prevents all default-label additions, so one custom label is enough to manage only that label.

To stop label management, omit `labels` or use `labels: []`, and disable default merging.

## Sponsorships

Tailor places `.github/FUNDING.yml` as a `first-fit` swatch, but the GitHub API does not expose the "Sponsorships" checkbox. After running `alter`, tick **Settings > General > Features > Sponsorships** manually to display the Sponsor button on the repository.
