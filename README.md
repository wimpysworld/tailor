# Tailor

[![Go Report Card](https://goreportcard.com/badge/github.com/wimpysworld/tailor)](https://goreportcard.com/report/github.com/wimpysworld/tailor)

Ready-to-wear project templates for GitHub repositories. Tailor fits projects with community health files, security policy, dev tooling, and repository settings that meet GitHub's community standards, then keeps them current with automated alterations. It also ships a Dependabot automerge workflow so patch and minor updates land without manual intervention.

If you manage multiple projects across different GitHub organisations and find that configurations keep drifting out of sync, Tailor fixes that. It is opinionated by design - built for solo devs and small teams who want consistent, well-maintained repositories without the overhead.

This README covers both the CLI and the [GitHub Action](#github-action).

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

Tailor needs a GitHub authentication token. Set `GH_TOKEN` or `GITHUB_TOKEN` for CI, or run `gh auth login` locally.

## GitHub Action

The `wimpysworld/tailor` action installs the tailor binary and optionally runs one or more commands. Pin to a major version tag to receive non-breaking updates automatically.

```yaml
- uses: wimpysworld/tailor@v0
  with:
    alter: true
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `version` | Tailor version to install (e.g. `0.2.0`). Defaults to the version matching the action release. | latest for the pinned major |
| `fit` | Run `tailor fit` to bootstrap a new project. Requires `GH_TOKEN` in the job env. | `false` |
| `alter` | Run `tailor alter` to apply swatches and repository settings. Requires `GH_TOKEN` in the job env. | `false` |
| `baste` | Run `tailor baste` to preview what alter would change. Requires `GH_TOKEN` in the job env. | `false` |
| `measure` | Run `tailor measure` to check community health files and configuration alignment. | `false` |
| `docket` | Run `tailor docket` to display authentication state and repository context. | `false` |

### Token requirements

`GITHUB_TOKEN` is sufficient for most tailor operations in CI. Two fields are the exception: `default_workflow_permissions` and `can_approve_pull_request_reviews` call the `PUT /repos/{owner}/{repo}/actions/permissions/workflow` endpoint, which requires repository administration access. No `permissions:` block grants `GITHUB_TOKEN` this scope - it is a GitHub platform constraint.

When `GITHUB_TOKEN` is used, tailor skips those fields and reports:

```
would skip (insufficient scope: token missing required scope): default_workflow_permissions
would skip (insufficient scope: token missing required scope): can_approve_pull_request_reviews
```

To manage these fields from CI, provide a PAT with the necessary access.

#### Personal repositories

Create one of the following:

- **Classic PAT** at <https://github.com/settings/tokens> - enable the `repo` scope
- **Fine-grained PAT** at <https://github.com/settings/personal-access-tokens/new> - set "Repository permissions > Administration" to "Read and write"

#### Organisation repositories

Use the same PAT creation steps above. The PAT must belong to a user with admin access to the repository. If the organisation enforces SSO, authorise the PAT for the org after creation via the token's "Configure SSO" link.

#### Storing and using the PAT

Add the PAT as a repository secret via **Settings > Secrets and variables > Actions**, then pass it as `GH_TOKEN` in the workflow:

```yaml
env:
  GH_TOKEN: ${{ secrets.TAILOR_TOKEN }}
```

### Supported platforms

Linux (amd64, arm64) and macOS (amd64, arm64).

### Version resolution

The `version` input takes precedence when set. Without it, the action resolves the version from its ref: a full tag such as `v0.2.0` pins exactly, while a major tag such as `v0` resolves to the latest stable `v0.x.x` release.

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

`measure` checks which community health files are present, missing, or need attention. It warns when `README.md` is absent or when `LICENSE` contains unresolved placeholders. `fit .` works in an existing directory without error. If the project has a GitHub remote, `fit` reads the live repository settings so it preserves anything already configured.

Edit `.tailor.yml` to add swatches or change alteration modes, then run `alter`. Set `alteration: never` on any swatch you want tailor to skip.

## Core Concepts

### Swatches

Swatches are complete template files embedded in the tailor binary. Most are copied verbatim. Five have tokens substituted at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | `gh api user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | `gh repo view` |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | `gh repo view` |
| `.tailor.yml` | `{{HOMEPAGE_URL}}` | `.tailor.yml` |
| `.github/workflows/tailor-automerge.yml` | `{{MERGE_STRATEGY}}` | Repository merge settings |

Licences are not swatches. They are fetched from the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

### Default swatch set

| Swatch | Mode |
|--------|------|
| `.github/workflows/tailor.yml` | `always` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `always` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `always` |
| `.github/pull_request_template.md` | `always` |
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
| `.github/workflows/tailor-automerge.yml` | `triggered` |

### Alteration modes

- **`always`** - Overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - Copies the file only if it does not already exist. Never overwrites. Use this for files you intend to customise after initial delivery.
- **`triggered`** - Deploys the file only when a condition in the repository settings is met. Overwrites when active, removes the file when the condition becomes false.
- **`never`** - Skips the file entirely. Use this to suppress a triggered swatch you do not want.

### Configuration

All state lives in `.tailor.yml` with four sections: `license`, `repository`, `labels`, and `swatches`.

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
  default_workflow_permissions: read
  can_approve_pull_request_reviews: false

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
| `alteration` | `always`, `first-fit`, `triggered`, or `never` |

Set `alteration: never` to stop tailor managing a file. The entry stays visible in `.tailor.yml` and prevents `alter --recut` from re-adding it.

## Repository Settings

The `repository` section manages GitHub repository settings declaratively. Field names match the [GitHub REST API](https://docs.github.com/en/rest/repos/repos#update-a-repository) exactly (snake_case). Settings are applied as a single API call on every `alter` run.

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
| `topics` | string[] | Repository topics for discoverability |
| `default_workflow_permissions` | string | `GITHUB_TOKEN` default permissions: `read` or `write` |
| `can_approve_pull_request_reviews` | bool | Allow workflows to approve PRs |

Omit the `repository` section entirely to skip settings management.

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

Omit the `labels` section to skip label management.

## Sponsorships

Tailor places `.github/FUNDING.yml` as a `first-fit` swatch, but the GitHub API does not expose the "Sponsorships" checkbox. After running `alter`, tick **Settings > General > Features > Sponsorships** manually to display the Sponsor button on the repository.

## Automated Maintenance

The `.github/workflows/tailor.yml` swatch delivers a GitHub Actions workflow that runs `tailor alter` weekly and opens a pull request when swatch content changes.

```yaml
name: Tailor 🪡
on:
  schedule:
    - cron: "0 9 * * 1"
  workflow_dispatch:

jobs:
  alter:
    runs-on: ubuntu-slim
    env:
      GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    steps:
      - uses: actions/checkout@v4

      - name: Setup tailor
        uses: wimpysworld/tailor@v0

      - name: Alter swatches
        run: tailor alter

      - name: Create PR
        uses: peter-evans/create-pull-request@v6
        with:
          branch: tailor-alter
          title: "chore: alter tailor swatches"
```

The workflow itself is an `always` swatch, so it stays current as tailor releases update the template. The `action.yml` at the repository root installs the binary into the runner.

### Branch protection

Branch protection rules and rulesets require `Administration: write`, which `GITHUB_TOKEN` cannot hold regardless of `permissions:` configuration - this is a GitHub platform constraint. Branch protection is out of scope for Tailor; configure it via the GitHub UI or `gh api`.

### Automerge

The `.github/workflows/tailor-automerge.yml` swatch auto-approves and merges Dependabot pull requests. It deploys automatically when `allow_auto_merge: true` is set in repository settings and removes itself when the setting is false.

| Ecosystem | Patch | Minor | Major |
|-----------|-------|-------|-------|
| GitHub Actions | Auto-merge | Auto-merge | Auto-merge |
| All others | Auto-merge | Auto-merge | Skip |

GitHub Actions use major version tags as their release convention, so Dependabot reports most action updates as major bumps - restricting to patch and minor would skip the majority. All other ecosystems follow semantic versioning where major indicates breaking changes, so those are left for manual review.

The workflow uses `gh pr merge --auto`, which waits for all branch protection rules to pass before completing.

> **Prerequisite:** Auto-merge requires [branch protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches) with at least one required status check on the default branch. Without this, `gh pr merge --auto` merges immediately with no CI gate.

**Opt-out:** set `alteration: never` on the automerge swatch entry in `.tailor.yml`.

**Manual catch-up:** the workflow supports `workflow_dispatch` for repositories with pre-existing open Dependabot PRs. Triggering it manually enables auto-merge on all open Dependabot PRs regardless of ecosystem.

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

Reads `.tailor.yml` in the current directory and applies repository settings, labels, licence, and swatches. Execution order: repository settings, then labels, then licence, then swatches.

```bash
tailor alter            # Apply changes
tailor alter --recut    # Overwrite regardless of mode
```

`--recut` overwrites all files including `first-fit` swatches. `LICENSE` is exempt (fetched content, not an embedded swatch). For `.tailor.yml`, `--recut` appends missing default swatch entries but never modifies existing entries.

### `baste`

Previews what `alter` would do without making changes.

```bash
tailor baste
```

```
     would set: repository.has_wiki = false
    would copy: LICENSE
 would overwrite: SECURITY.md
     no change: .github/workflows/tailor.yml
skipped (first-fit, exists): justfile
would deploy (triggered: allow_auto_merge): .github/workflows/tailor-automerge.yml
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
| `warning` | Health diagnostic needing attention (missing `README.md` or unresolved licence placeholders) |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `.tailor.yml` |
| `config-only` | Swatch in `.tailor.yml` not in the built-in default set |
| `mode-differs` | Alteration mode differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor.yml` is present.
