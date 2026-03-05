# Tailor

Bespoke project templates for GitHub repositories. Tailor fits new projects with community health files, dev tooling, and repository settings, then keeps them current with automated weekly alterations.

```bash
# Fit a new project
tailor fit ./my-project
cd my-project
tailor alter
```

## Prerequisites

Tailor requires a valid GitHub authentication token. Set `GH_TOKEN` or `GITHUB_TOKEN` for CI environments, or run `gh auth login` for local development.

## Quick Start

### New project

```bash
tailor fit ./my-project
cd my-project
tailor alter
```

`fit` creates the project directory with a `.tailor/config.yml` containing the full default swatch set. `alter` copies the swatch files and applies repository settings. The default licence is MIT.

```bash
# Choose a different licence
tailor fit ./my-project --license=Apache-2.0

# Opt out of licence entirely
tailor fit ./my-project --license=none

# Set repository description
tailor fit ./my-project --description="My awesome project"
```

### Existing project

```bash
cd existing-project
tailor measure                # See what's missing
tailor fit .                  # Create .tailor/config.yml
tailor alter                  # Apply swatches and settings
```

`measure` checks which community health files are present or missing. `fit .` works in an existing directory and creates the configuration without error. If the project has a GitHub remote, `fit` reads the live repository settings so it does not change anything already configured.

Edit `.tailor/config.yml` directly to add or remove swatches or change alteration modes, then run `alter`.

## Swatches

Swatches are complete template files embedded in the tailor binary. They are copied verbatim to your project, with four exceptions where tokens are substituted at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | `gh api user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | `gh repo view` |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | `gh repo view` |
| `.tailor/config.yml` | `{{HOMEPAGE_URL}}` | `.tailor/config.yml` |

Licences are not swatches. They are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

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
| `.tailor/config.yml` | `first-fit` |

### Alteration modes

- **`always`** - Overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - Copies the file only if it does not already exist. Never overwrites. Use this mode for files you intend to customise after initial delivery.

## Configuration

All state lives in `.tailor/config.yml` at the project root. The file has three sections: `license`, `repository`, and `swatches`.

```yaml
# Initially fitted by tailor on 2026-03-04
license: MIT

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
| `source` | Swatch name, matching the path relative to `swatches/` in the tailor binary |
| `destination` | Output path relative to the project root |
| `alteration` | `always` or `first-fit` |

Remove a swatch entry from `config.yml` to stop tailor managing that file. Add entries to include additional swatches. Change `alteration` to control update behaviour.

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

Omit the `repository` section entirely to skip repository settings management.

## Automated Maintenance

The `.github/workflows/tailor.yml` swatch delivers a GitHub Actions workflow that runs `tailor alter` weekly and opens a pull request when swatch content changes.

```yaml
name: Tailor
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

No manual setup beyond including `.github/workflows/tailor.yml` in your swatch list. Because the workflow itself is an `always` swatch, it stays current when tailor releases update the template.

[`wimpysworld/tailor-action`](https://github.com/wimpysworld/tailor-action) installs the tailor binary into the workflow runner.

## Commands

### `fit <path>`

Creates a project directory and writes `.tailor/config.yml` with the full default swatch set. Does not copy files or apply settings.

```bash
tailor fit ./my-project
tailor fit ./my-project --license=Apache-2.0
tailor fit ./my-project --license=none
tailor fit ./my-project --description="Short description"
```

When run against an existing directory with a GitHub remote, `fit` queries the live repository configuration and uses those values for the `repository` section. When no remote exists, the built-in defaults are used.

Exits with an error if `.tailor/config.yml` already exists at `<path>`.

### `alter`

Reads `.tailor/config.yml` in the current directory and applies swatches, licence, and repository settings.

```bash
tailor alter              # Apply changes
tailor alter --recut      # Apply and overwrite regardless of mode
```

Execution order: repository settings, then licence, then swatches.

`--recut` overwrites all files including `first-fit` swatches. Two files are exempt: `LICENSE` (fetched content, not an embedded swatch) and `.tailor/config.yml` (overwriting it would destroy the project configuration).

### `baste`

Previews what `alter` would do without making any changes.

```bash
tailor baste
```

```
would set:                   repository.has_wiki = false
would copy:                  LICENSE
would overwrite:             SECURITY.md
no change:                   .github/workflows/tailor.yml
skipped (first-fit, exists): justfile
```

### `docket`

Displays the current GitHub authentication state and repository context.

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

### `measure`

Checks community health files and configuration alignment. Requires no network access, no authentication, and no `.tailor/config.yml`. Run it in any directory.

```bash
tailor measure
```

```
missing:        .github/FUNDING.yml
missing:        CONTRIBUTING.md
present:        CODE_OF_CONDUCT.md
present:        LICENSE
not-configured: .github/dependabot.yml
mode-differs:   SECURITY.md          (config: first-fit, default: always)
```

| Status | Meaning |
|--------|---------|
| `missing` | Health file does not exist on disk |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `config.yml` |
| `config-only` | Swatch in `config.yml` not in the built-in default set |
| `mode-differs` | Alteration mode in `config.yml` differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor/config.yml` is present.
