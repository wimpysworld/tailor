# Tailor

[![Go Report Card](https://goreportcard.com/badge/github.com/wimpysworld/tailor)](https://goreportcard.com/report/github.com/wimpysworld/tailor)

Ready-to-wear project templates for GitHub repositories. Tailor is a local CLI that fits projects with community health files, security policy, dev tooling, labels, and repository settings that meet GitHub's community standards.

If you manage multiple projects across different GitHub organisations and find that configurations drift out of sync, Tailor fixes that. It is opinionated by design - built for solo devs and small teams who want consistent, well-maintained repositories without hosted automation.

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

Tailor needs a GitHub authentication token for `fit`, `alter`, and `baste`. Set `GH_TOKEN` or `GITHUB_TOKEN`, or run `gh auth login` locally.

`measure` and `docket` do not require authentication.

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

Edit `.tailor.yml` to add swatches or change alteration modes, then run `alter`. Set `alteration: never` on any swatch you want Tailor to skip.

## Core Concepts

### Swatches

Swatches are complete template files embedded in the tailor binary. Most are copied verbatim. Four have tokens substituted at `alter` time:

| File | Token | Resolved from |
|------|-------|---------------|
| `.github/FUNDING.yml` | `{{GITHUB_USERNAME}}` | `gh api user` |
| `SECURITY.md` | `{{ADVISORY_URL}}` | `gh repo view` |
| `.github/ISSUE_TEMPLATE/config.yml` | `{{SUPPORT_URL}}` | `gh repo view` |
| `.tailor.yml` | `{{HOMEPAGE_URL}}` | `.tailor.yml` |

Licences are not swatches. They are fetched from the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

### Default swatch set

| Swatch | Mode |
|--------|------|
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `always` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `always` |
| `.github/pull_request_template.md` | `always` |
| `SECURITY.md` | `always` |
| `CODE_OF_CONDUCT.md` | `always` |
| `CONTRIBUTING.md` | `always` |
| `SUPPORT.md` | `always` |
| `.tailor.yml` | `always` |
| `.github/dependabot.yml` | `first-fit` |
| `.github/FUNDING.yml` | `first-fit` |
| `.github/ISSUE_TEMPLATE/config.yml` | `first-fit` |
| `justfile` | `first-fit` |
| `flake.nix` | `first-fit` |
| `.gitignore` | `first-fit` |
| `.envrc` | `first-fit` |
| `cubic.yaml` | `first-fit` |

### Alteration modes

- **`always`** - overwrites the file whenever the embedded swatch content differs from what is on disk. Local edits are not preserved.
- **`first-fit`** - copies the file only if it does not already exist. Never overwrites. Use this for files you intend to customise after initial delivery.
- **`never`** - skips the file entirely.

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

swatches:
  - path: SECURITY.md
    alteration: always

  - path: justfile
    alteration: first-fit
```

Each swatch entry has two fields:

| Field | Description |
|-------|-------------|
| `path` | File path relative to the project root, matching a swatch name in the binary |
| `alteration` | `always`, `first-fit`, or `never` |

Set `alteration: never` to stop Tailor managing a file. The entry stays visible in `.tailor.yml` and prevents `alter --recut` from re-adding it.

## Repository Settings

The `repository` section manages GitHub repository settings declaratively. Field names match the [GitHub REST API](https://docs.github.com/en/rest/repos/repos#update-a-repository) where possible. Settings are applied on every `alter` run.

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
| `web_commit_signoff_required` | bool | Require sign-off on web commits |
| `topics` | string[] | Repository topics for discoverability |

Omit the `repository` section entirely to skip settings management.

## Labels

The `labels` section manages GitHub issue labels declaratively. Tailor ships 12 default labels with colours from the [Catppuccin Latte](https://catppuccin.com/palette/) palette.

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

Labels are reconciled with create-and-update-only semantics. Tailor creates missing labels and updates labels whose colour or description differs, but never deletes labels from the repository.

Omit the `labels` section to skip label management.

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

`--recut` overwrites all files including `first-fit` swatches. `LICENSE` is exempt because it is fetched content, not an embedded swatch. For `.tailor.yml`, `--recut` appends missing default swatch entries but never modifies existing entries.

### `baste`

Previews what `alter` would do without making changes.

```bash
tailor baste
```

```text
     would set: repository.has_wiki = false
    would copy: LICENSE
 would overwrite: SECURITY.md
skipped (first-fit, exists): justfile
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

```text
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
| `warning` | Health diagnostic needing attention, such as missing `README.md` or unresolved licence placeholders |
| `present` | Health file exists on disk |
| `not-configured` | Default swatch not in `.tailor.yml` |
| `config-only` | Swatch in `.tailor.yml` not in the built-in default set |
| `mode-differs` | Alteration mode differs from the default |

The `not-configured`, `config-only`, and `mode-differs` statuses appear only when `.tailor.yml` is present.
