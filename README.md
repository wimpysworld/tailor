# Tailor 👔

Ready-to-wear project templates for GitHub repositories. Tailor is a local terminal CLI that fits projects with community health files, security policy, dev tooling, and repository settings that meet GitHub's community standards.

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

Tailor embeds 16 default swatches:

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

### Alteration modes

- **`always`** - Overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - Copies the file only if it does not already exist. Never overwrites. Use this for files you intend to customise after initial delivery.
- **`never`** - Skips the file entirely. Use this to keep a swatch visible in the config without managing its destination.

### Configuration

All state lives in `.tailor.yml` with five sections: `license`, `repository`, `actions`, `labels`, and `swatches`.

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

To skip settings management, omit the `repository` section and set the `.tailor.yml` swatch to `alteration: never`. With `always`, `alter` restores the section from built-in defaults and applies it in the same run. With `first-fit`, `alter --recut` restores and applies the section.

Generated configs expose all three security settings. Their built-in values are `true`. When a GitHub remote exists, `fit` uses live values. Default merging appends missing security settings without changing explicit Boolean values. The security prerequisite normalisation below is the exception: it can change an explicit `vulnerability_alerts_enabled: false` to `true`.

GitHub labels `can_approve_pull_request_reviews` as “Allow GitHub Actions to create and approve pull requests”. Tailor keeps the REST API field name because repository config keys match the API. Enabling the setting permits the repository `GITHUB_TOKEN` to create pull requests and submit approval reviews when the workflow has `pull-requests: write`. The setting does not permit merges, bypass branch rules, or affect personal access tokens or separate GitHub App tokens.

GitHub requires vulnerability alerts before automated security fixes. When automated fixes are enabled and alerts are absent or false, Tailor sets `vulnerability_alerts_enabled` to `true` and shows a warning. `alter` and `alter --recut` save the corrected `.tailor.yml` before repository API calls. `baste` previews the config update without writing. Tailor enables alerts first and disables automated fixes before alerts. If a prerequisite read is unknown, or its write fails or is skipped, Tailor skips the dependent write. A security `404` stays unknown unless Tailor can distinguish a disabled feature from denied access. Access failures produce warnings. Other API failures stop the command.

## Actions Policy

The top-level `actions` section manages the repository Actions policy. Generated configs enable Actions, select restricted actions, allow GitHub-owned and verified actions, disable SHA pinning, and allow the six patterns in the configuration example.

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable GitHub Actions for the repository |
| `allowed_actions` | string | Allow `all`, `local_only`, or `selected` actions and reusable workflows |
| `sha_pinning_required` | bool | Require actions to use full-length commit SHAs |
| `github_owned_allowed` | bool | Allow GitHub-owned actions when `allowed_actions` is `selected` |
| `verified_allowed` | bool | Allow actions from verified creators when `allowed_actions` is `selected` |
| `patterns_allowed` | string[] | Complete set of allowed action and reusable workflow patterns |

The selected-action fields require `allowed_actions: selected`. A selected policy must include `github_owned_allowed`, `verified_allowed`, and `patterns_allowed` after default merging. The `patterns_allowed` field replaces the full GitHub list. Tailor ignores list order during comparison. Default merging adds a missing `actions` section and fills missing fields without changing explicit values. For `all` and `local_only`, Tailor does not add selected-only fields. For `selected`, Tailor adds each missing selected-only field. A missing `patterns_allowed` field receives the six approved defaults. Tailor preserves an explicit custom list or `patterns_allowed: []`.

For an enabled transition from `all` to an enabled `selected` policy, Tailor first changes `allowed_actions` to `selected`. This write keeps SHA pinning enabled when the final policy disables it. Tailor then applies the complete selected rules and disables SHA pinning in a final core write. When the final policy keeps or enables SHA pinning, the first core write applies that value and Tailor omits the final write. A hard first-write failure leaves `all` active. A selected-rule failure leaves the narrower `selected` policy active and preserves SHA pinning. A final SHA write failure leaves the selected rules and SHA pinning active. Each hard failure stops the command.

For other transitions from `all` or `local_only`, Tailor disables Actions before it applies the complete selected rules and the final core policy. For an existing selected policy, Tailor applies changed selected rules before any core broadening, including disabling SHA pinning. When an enabled policy combines selected broadening with SHA pinning or disabling Actions, Tailor disables Actions before both updates. Broadening means newly allowing GitHub-owned actions, verified actions, or patterns. Tailor also disables Actions before any selected update whose final policy disables Actions. A later update failure leaves Actions disabled and stops the command. If Tailor cannot read an active selected policy, it does not enable Actions or disable SHA pinning. Organisation policy can restrict repository values. Other access failures produce skip results, and other API failures stop the command.

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

### Branch protection

Branch protection rules and rulesets require `Administration: write`. Tailor does not manage them. Configure branch protection through the GitHub UI or `gh api`.

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

When a GitHub remote exists, `fit` queries the live repository configuration for the `repository` section. Otherwise, built-in defaults are used. Exits with an error if `.tailor.yml` already exists.

### `alter`

Reads `.tailor.yml` in the current directory and applies repository settings, Actions policy, labels, licence, and swatches in that order.

```bash
tailor alter            # Apply changes
tailor alter --recut    # Overwrite always and first-fit swatches
```

`--recut` overrides `first-fit` and overwrites those swatches, but it still skips `never` swatches. `LICENSE` is exempt (fetched content, not an embedded swatch). For `.tailor.yml`, `--recut` appends missing default swatch entries but never modifies existing entries.

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

```bash
tailor docket
```

### `measure`

Checks community health files and configuration alignment. No network access, no authentication, no `.tailor.yml` required.

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
