# Tailor Specification v0.3

## Overview

Tailor is a Go CLI tool for managing project templates across GitHub repositories. It provides bespoke fitting for new projects and alterations for existing projects. Tailor is intended to run locally from a terminal.

Running `tailor` with no arguments displays help.

## Prerequisites

Tailor requires a valid GitHub authentication token for commands that read or change GitHub state:

1. **Environment variable**: set `GH_TOKEN` or `GITHUB_TOKEN`.
2. **GitHub CLI**: install and authenticate the [GitHub CLI](https://cli.github.com/) with `gh auth login`.

The `fit`, `alter`, and `baste` commands verify that a valid authentication token exists at startup and exit with an error if no token is available.

`measure` and `docket` are exempt from the authentication requirement. `measure` performs purely local file inspection. `docket` reports authentication state without requiring it.

## Intended Workflow

### New project

`fit` creates the project directory and writes `.tailor.yml` with the full default swatch set in one command, with a `license: BlueOak-1.0.0` default. Use `--license=<id>` to select a different licence or `--license=none` to opt out. Change into `<path>`, then run `alter` to copy the swatch files and apply repository settings.

### Existing project

`measure` checks which community health files are present or missing. If no `.tailor.yml` exists, run `tailor fit .` to create one, or create `.tailor.yml` manually. Edit `.tailor.yml` directly to add or remove swatches or change alteration modes, then run `alter` to bring the project into sync with the current swatches.

## Core Concepts

**Swatches**: complete, ready-to-use template files stored in `swatches/`. Files are copied verbatim, with four exceptions: `.github/FUNDING.yml` has `{{GITHUB_USERNAME}}` substituted automatically; `SECURITY.md` has `{{ADVISORY_URL}}` substituted automatically; `.github/ISSUE_TEMPLATE/config.yml` has `{{SUPPORT_URL}}` substituted automatically; `.tailor.yml` has `{{HOMEPAGE_URL}}` substituted automatically.

**Swatch names**: swatch references use the full source path relative to `swatches/`, including the file extension where one exists. Extensionless files are referenced as-is.

**Swatch mapping**:

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
| `cubic.yaml` | `cubic.yaml` |
| `.github/FUNDING.yml` | `.github/FUNDING.yml` |
| `.github/dependabot.yml` | `.github/dependabot.yml` |
| `.github/ISSUE_TEMPLATE/bug_report.yml` | `.github/ISSUE_TEMPLATE/bug_report.yml` |
| `.github/ISSUE_TEMPLATE/feature_request.yml` | `.github/ISSUE_TEMPLATE/feature_request.yml` |
| `.github/ISSUE_TEMPLATE/config.yml` | `.github/ISSUE_TEMPLATE/config.yml` |
| `.github/pull_request_template.md` | `.github/pull_request_template.md` |
| `.tailor.yml` | `.tailor.yml` |

Licences are not swatches. They are fetched via the GitHub REST API (`GET /licenses/{id}`) at `alter` time and written to `LICENSE`.

**Repository settings**: Tailor can manage GitHub repository settings declaratively via the `repository` section in `.tailor.yml`. Field names match the GitHub REST API field names where possible. Repository settings are always applied idempotently on every `alter` run. If the `repository` section is absent from `.tailor.yml`, repository settings are skipped entirely.

**Labels**: Tailor can manage GitHub issue labels declaratively via the `labels` section in `.tailor.yml`. Labels are a top-level config key alongside `repository:` and `swatches:`. The reconciliation strategy is create and update only. Labels present on GitHub but absent from config are left untouched.

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
| `merge_commit_message` | string | Merge commit message (`PR_TITLE`, `PR_BODY`, `BLANK`) |
| `delete_branch_on_merge` | bool | Delete branch on merge |
| `allow_update_branch` | bool | Allow updating PR branches |
| `web_commit_signoff_required` | bool | Require sign-off on web commits |
| `topics` | string array | Repository topics for discoverability |

`topics` uses a separate API endpoint rather than the repository PATCH call:

| Field | Read | Write |
|---|---|---|
| `topics` | Read from `GET /repos/{owner}/{repo}` response | `PUT /repos/{owner}/{repo}/topics` with `{"names": [...]}` |

**Topics**: the PUT endpoint replaces the entire topics list. The config declares the complete desired set. Omitted topics are removed on apply. Topics are project-specific and not included in the default config template.

Settings deliberately excluded due to risk or org-level scope: `visibility`, `default_branch`, `name`, `archived`, `is_template`, `allow_forking`, `security_and_analysis`, branch protection, rulesets, Actions permissions policy, deployment environments, custom properties, Dependabot secrets, and GitHub workflow permissions.

**Alteration modes**:

- `always`: Tailor compares the embedded swatch content against the on-disk file on every `alter` run and overwrites if they differ. For `.tailor.yml`, `always` means append missing default swatch entries rather than overwrite content.
- `first-fit`: Tailor copies this file only if it does not already exist. It never overwrites.
- `never`: Tailor skips this swatch entirely.

**Default alteration modes**:

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
| `.github/pull_request_template.md` | `always` |
| `.github/dependabot.yml` | `first-fit` |
| `justfile` | `first-fit` |
| `cubic.yaml` | `first-fit` |
| `flake.nix` | `first-fit` |
| `.tailor.yml` | `always` |

**Swatch categories**: each swatch is designated either `health` or `development`. This designation is an internal attribute used by `measure` to scope its file presence checks.

**Health swatches**:

- `LICENSE` (fetched via GitHub, not an embedded swatch)
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

**Development swatches**:

- `.gitignore`
- `.envrc`
- `flake.nix`
- `justfile`
- `cubic.yaml`
- `.tailor.yml`

## Commands

Commands divide into three categories: bootstrap commands, apply commands, and inspection commands.

- **Bootstrap commands**: `fit`
- **Apply commands**: `alter`
- **Inspection commands**: `baste`, `measure`, `docket`

### `fit <path>`

Creates a new project directory and writes `.tailor.yml` with the full default swatch set and repository settings. When run against an existing project with a GitHub remote, `fit` queries the live repository configuration and uses those values for the `repository` section. When no repository context exists, the built-in defaults are used. It does not copy any files or apply any settings.

The default swatch set embedded in the binary is:

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
- `flake.nix`
- `.gitignore`
- `.envrc`
- `cubic.yaml`
- `.tailor.yml`

A `license` key is included in `.tailor.yml` by default (`license: BlueOak-1.0.0`). Use `--license=<id>` to select a different licence or `--license=none` to opt out entirely.

### `alter`

Applies swatch alterations to the local project.

```bash
tailor alter
tailor alter --recut
```

Behaviour:

- If `.tailor.yml` is missing or malformed, exits immediately.
- Before processing swatches, merges built-in defaults into the loaded config when `.tailor.yml` has `alteration: always`.
- Repository settings are applied first when a `repository` section is present.
- Labels are applied after repository settings when a `labels` section is present.
- Licences are fetched only when needed and never overwritten.
- `always` swatches are overwritten when the embedded content differs from the on-disk file.
- `first-fit` swatches are copied only when the destination file does not exist.
- `never` swatches are skipped entirely.
- `--recut` overwrites swatches regardless of mode or existence, except `LICENSE`.
- Intermediate directories are created as needed.
- Files not listed in `.tailor.yml` are never touched.
- Tailor modifies files only. It does not commit, push, open pull requests, run in GitHub Actions, or merge pull requests.

### `baste`

Previews what `alter` would do without making any changes.

```bash
tailor baste
```

Repository settings output:

```text
would set: repository.has_wiki = false
no change: repository.allow_squash_merge (already true)
```

Swatch output:

```text
would copy:                  LICENSE
would overwrite:             SECURITY.md
skipped (first-fit, exists): justfile
skip (never):                SUPPORT.md
```

### `measure`

Assesses a project's community health files and, when `.tailor.yml` is present, checks configuration alignment against the built-in defaults. Requires no git repository, no network access, and no Tailor configuration.

```bash
tailor measure
```

### `docket`

Displays the current GitHub authentication state and repository context.

```bash
tailor docket
```

Behaviour:

- `user` is resolved via `GET /user` if authenticated.
- `repository` displays the `owner/repo` derived from the GitHub remote in the current directory.
- `auth` displays `authenticated` or `not authenticated`.
- Does not read `.tailor.yml` and does not require it to be present.

## Error Handling

**Unrecognised swatch `path` in `.tailor.yml`**: `alter` exits with an error identifying the unrecognised name and listing all valid swatch paths embedded in the binary.

**Licence fetch failed**: `alter` exits with the API error.

**Destination path not writable**: Tailor exits with an error showing the full path that could not be written.

**`.tailor.yml` malformed or missing**: `alter` or `baste` exits with a clear message directing the user to run `fit` or edit `.tailor.yml`.

**Duplicate path in `.tailor.yml`**: `alter` exits with an error identifying the conflicting swatches before making changes.

**Not authenticated**: if no valid authentication token can be resolved for `github.com`, `fit`, `alter`, and `baste` exit with: "tailor requires GitHub authentication. Set the GH_TOKEN or GITHUB_TOKEN environment variable, or run `gh auth login`."

**Repo-context tokens unresolved**: `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, and `{{HOMEPAGE_URL}}` require a GitHub repository context. If the project has no GitHub remote, these tokens are left unsubstituted silently.

**Repository settings without repo context**: if `.tailor.yml` contains a `repository` section but the project has no GitHub remote, repository settings are skipped with a warning.

**Repository settings API failure**: if any API call to apply repository settings fails, `alter` exits with the API error.

**Unrecognised repository setting**: if `.tailor.yml` contains a field in the `repository` section that is not in the supported settings list, `alter` exits with an error identifying the unrecognised field and listing all valid repository setting field names.

**Legacy GitHub automation swatches in older `.tailor.yml` files**: the removed hosted-automation paths `.github/workflows/tailor.yml` and `.github/workflows/tailor-automerge.yml` are still accepted for backwards compatibility, but `alter` and `baste` treat them as skipped legacy entries and never recreate those workflows.

## Configuration

Default `.tailor.yml` with `--license=BlueOak-1.0.0`:

```yaml
# Initially fitted by tailor on 2026-03-02
license: BlueOak-1.0.0

repository:
  description: ""
  homepage: "{{HOMEPAGE_URL}}"
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
  web_commit_signoff_required: false

labels:
  - name: bug
    color: d20f39
    description: "Something isn't working"

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
    alteration: always

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

  - path: flake.nix
    alteration: first-fit

  - path: .gitignore
    alteration: first-fit

  - path: .envrc
    alteration: first-fit

  - path: cubic.yaml
    alteration: first-fit

  - path: .tailor.yml
    alteration: always
```

## Swatch Storage

Swatches are embedded in the Tailor binary at build time from `swatches/`:

```text
swatches/
├── .envrc
├── .gitignore
├── cubic.yaml
├── CODE_OF_CONDUCT.md
├── CONTRIBUTING.md
├── SECURITY.md
├── SUPPORT.md
├── flake.nix
├── justfile
├── .github/
│   ├── dependabot.yml
│   ├── FUNDING.yml
│   ├── ISSUE_TEMPLATE/
│   │   ├── bug_report.yml
│   │   ├── config.yml
│   │   └── feature_request.yml
│   └── pull_request_template.md
└── .tailor.yml
```

## Implementation Notes

1. **Overwrite detection**: SHA-256 hash comparison between the embedded swatch content and the on-disk target file.
2. **Interpolation**: `.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, and `.tailor.yml` support token substitution.
3. **No hosted automation**: Tailor does not ship GitHub Action metadata, GitHub workflow swatches, CI self-update workflows, or automerge workflows.
4. **Legacy config compatibility**: older `.tailor.yml` files that still list the removed `tailor.yml` or `tailor-automerge.yml` workflow swatches remain parseable, but those entries are ignored rather than applied.
5. **No versioning**: no swatch versions, always uses swatches from the current Tailor binary.
6. **No global state**: all state is per-project in `.tailor.yml`.
7. **No project registry**: Tailor has no awareness of its consumers.
8. **Authentication via `go-gh`**: token resolution follows the `go-gh` precedence order.
9. **CLI parsing**: [Kong](https://github.com/alecthomas/kong) is used as the command line parser.
10. **Repository settings via API**: repository settings are applied via GitHub REST APIs. The execution order is repository settings, labels, licence, swatches.
