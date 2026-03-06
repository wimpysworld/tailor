# Tailor

Ready-to-wear project templates for GitHub repositories. Tailor fits projects with community health files, dev tooling, and repository settings, then keeps them current with automated alterations.

If you manage multiple projects across different GitHub organisations and find that configurations keep drifting out of sync, Tailor fixes that. It is opinionated by design - built for solo devs and small teams who want consistent, well-maintained repositories without the overhead.

## Install

```bash
bin install github.com/wimpysworld/tailor
bin update tailor
```

Requires [`bin`](https://github.com/marcosnils/bin). Tailor releases publish bare executables, no archive extraction needed.

Tailor needs a GitHub authentication token. Set `GH_TOKEN` or `GITHUB_TOKEN` for CI, or run `gh auth login` locally.

## Quick Start

### New project

```bash
tailor fit ./my-project
cd my-project
tailor alter
```

`fit` creates the directory and writes `.tailor/config.yml` with the full default swatch set. `alter` copies the files and applies repository settings. The default licence is MIT.

```bash
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

### Existing project

```bash
cd existing-project
tailor measure                # See what's missing
tailor fit .                  # Create .tailor/config.yml
tailor alter                  # Apply swatches and settings
```

`measure` checks which community health files are present or missing. `fit .` works in an existing directory without error. If the project has a GitHub remote, `fit` reads the live repository settings so it preserves anything already configured.

Edit `.tailor/config.yml` to add swatches or change alteration modes, then run `alter`. Set `alteration: never` on any swatch you want tailor to skip.

## Core Concepts

### Swatches

Swatches are complete template files embedded in the tailor binary. Most are copied verbatim. Five have tokens substituted at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | `gh api user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | `gh repo view` |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | `gh repo view` |
| `.tailor/config.yml` | `{{HOMEPAGE_URL}}` | `.tailor/config.yml` |
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
| `.tailor/config.yml` | `always` |
| `.github/workflows/tailor-automerge.yml` | `triggered` |

### Alteration modes

- **`always`** - Overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - Copies the file only if it does not already exist. Never overwrites. Use this for files you intend to customise after initial delivery.
- **`triggered`** - Deploys the file only when a condition in the repository settings is met. Overwrites when active, removes the file when the condition becomes false.
- **`never`** - Skips the file entirely. Use this to suppress a triggered swatch you do not want.

### Configuration

All state lives in `.tailor/config.yml` with three sections: `license`, `repository`, and `swatches`.

```yaml
# Initially fitted by tailor on 2026-03-04
license: MIT

repository:
  has_wiki: false
  has_discussions: false
  allow_squash_merge: true
  delete_branch_on_merge: true
  allow_auto_merge: true

swatches:
  - source: SECURITY.md
    destination: SECURITY.md
    alteration: always

  - source: justfile
    destination: justfile
    alteration: first-fit
```

Each swatch entry has three fields:

| Field | Description |
|-------|-------------|
| `source` | Swatch name, matching the path relative to `swatches/` in the binary |
| `destination` | Output path relative to the project root |
| `alteration` | `always`, `first-fit`, `triggered`, or `never` |

Set `alteration: never` to stop tailor managing a file. The entry stays visible in `config.yml` and prevents `alter --recut` from re-adding it. Add entries to include additional swatches.

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
| `private_vulnerability_reporting_enabled` | bool | Enable private vulnerability reporting |

Omit the `repository` section entirely to skip settings management.

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
        uses: wimpysworld/tailor-action@v1

      - name: Alter swatches
        run: tailor alter

      - name: Create PR
        uses: peter-evans/create-pull-request@v6
        with:
          branch: tailor-alter
          title: "chore: alter tailor swatches"
```

The workflow itself is an `always` swatch, so it stays current as tailor releases update the template. [`wimpysworld/tailor-action`](https://github.com/wimpysworld/tailor-action) installs the binary into the runner.

### Automerge

The `.github/workflows/tailor-automerge.yml` swatch auto-approves and merges Dependabot pull requests. It deploys automatically when `allow_auto_merge: true` is set in repository settings and removes itself when the setting is false.

| Ecosystem | Patch | Minor | Major |
|-----------|-------|-------|-------|
| GitHub Actions | Auto-merge | Auto-merge | Auto-merge |
| All others | Auto-merge | Auto-merge | Skip |

GitHub Actions use major version tags as their release convention, so Dependabot reports most action updates as major bumps - restricting to patch and minor would skip the majority. All other ecosystems follow semantic versioning where major indicates breaking changes, so those are left for manual review.

The workflow uses `gh pr merge --auto`, which waits for all branch protection rules to pass before completing.

> **Prerequisite:** Auto-merge requires [branch protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches) with at least one required status check on the default branch. Without this, `gh pr merge --auto` merges immediately with no CI gate.

**Opt-out:** set `alteration: never` on the automerge swatch entry in `.tailor/config.yml`.

**Manual catch-up:** the workflow supports `workflow_dispatch` for repositories with pre-existing open Dependabot PRs. Triggering it manually enables auto-merge on all open Dependabot PRs regardless of ecosystem.

## Commands

### `fit <path>`

Creates a project directory and writes `.tailor/config.yml` with the full default swatch set. Does not copy files or apply settings.

```bash
tailor fit ./my-project
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

When a GitHub remote exists, `fit` queries the live repository configuration for the `repository` section. Otherwise, built-in defaults are used. Exits with an error if `.tailor/config.yml` already exists.

### `alter`

Reads `.tailor/config.yml` in the current directory and applies swatches, licence, and repository settings. Execution order: repository settings, then licence, then swatches.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Overwrite regardless of mode
```

`--recut` overwrites all files including `first-fit` swatches. `LICENSE` is exempt (fetched content, not an embedded swatch). For `.tailor/config.yml`, `--recut` appends missing default swatch entries but never modifies existing entries.

### `baste`

Previews what `alter` would do without making changes.

```bash
tailor baste
```

```
would set:                                      repository.has_wiki = false
would copy:                                     LICENSE
would overwrite:                                SECURITY.md
no change:                                      .github/workflows/tailor.yml
skipped (first-fit, exists):                    justfile
would deploy (triggered: allow_auto_merge):     .github/workflows/tailor-automerge.yml
```

### `docket`

Displays the current GitHub authentication state and repository context.

```bash
tailor docket
```

### `measure`

Checks community health files and configuration alignment. No network access, no authentication, no `.tailor/config.yml` required.

```bash
tailor measure
```

```
missing:        .github/FUNDING.yml
present:        CODE_OF_CONDUCT.md
not-configured: .github/dependabot.yml
mode-differs:   SECURITY.md          (config: first-fit, default: always)
```

| Status | Meaning |
|--------|---------|
| `missing` | Health file does not exist on disk |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `config.yml` |
| `config-only` | Swatch in `config.yml` not in the built-in default set |
| `mode-differs` | Alteration mode differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor/config.yml` is present.
