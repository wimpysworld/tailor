# AGENTS.md

## Project overview

Tailor is a Go CLI tool for managing project templates (swatches) across GitHub repositories. It fits new projects with community health files, dev tooling, and repository settings, then keeps them current via automated alterations.

The authoritative specification is `docs/SPECIFICATION.md`. All implementation decisions must align with it.

## Tech stack

- **Language**: Go (1.25+)
- **CLI parser**: [Kong](https://github.com/alecthomas/kong)
- **GitHub auth**: `GH_TOKEN`/`GITHUB_TOKEN` env var, or `gh` (GitHub CLI) for keyring-based token access
- **Swatch embedding**: Go `embed` directive (`swatches/` directory)
- **Dev environment**: Nix flake with `gh`, `go`, `golangci-lint`, `just`

## Project structure

```
tailor/
├── .github/workflows/  # CI workflows
├── cmd/tailor/         # CLI entrypoint
├── internal/           # Internal packages (config, swatch, gh wrappers)
├── swatches/           # Embedded template files (17 swatches)
├── docs/               # Specification
└── AGENTS.md
```

## Build and test commands

- Build: `just build` (or `go build -ldflags "-s -w" -o tailor ./cmd/tailor`)
- Run tests: `just test` (or `go test ./...`)
- Run linters: `just lint` (or `go vet ./... && golangci-lint run`)
- Enter dev shell: `nix develop` or `direnv allow`
- Task runner: `just` (lists available recipes)
- Create release: `just release 0.1.0`

## Code style

- Follow standard Go conventions: `gofmt`, `go vet`
- Package names are short, lowercase, single-word
- Internal packages go in `internal/`; no `pkg/` directory
- Error messages are lowercase, no trailing punctuation
- Use `fmt.Errorf` with `%w` for error wrapping
- Swatch-to-path mappings and default alteration modes are hardcoded in source, not configurable
- Field names in the `repository` config section match GitHub REST API names exactly (snake_case)
- Four alteration modes: `always`, `first-fit`, `triggered`, `never`
- `never` beats `triggered` - a user can suppress a triggered swatch by setting `alteration: never`
- Triggered swatches use a lookup table in `internal/swatch/trigger.go` mapping source paths to config field conditions
- `EvaluateTrigger(source string, repo any)` uses reflection to match yaml tags on `RepositorySettings`; `repo` is `any` (not `*config.RepositorySettings`) to avoid a circular import
- Adding a new triggered swatch requires: an entry in `triggerConditions` (trigger.go), a registry entry (registry.go), and inclusion in `swatches/.tailor.yml`

## Testing

- Table-driven tests following Go conventions
- Test files sit alongside the code they test (`*_test.go`)
- Test swatch embedding and config parsing without network access
- Commands that call `gh` should have their external calls abstracted behind interfaces for testability
- `measure` is purely local and needs no mocking
- `measure` emits `warning` results for two local health diagnostics: missing `README.md` (not managed by tailor) and `LICENSE` files containing unresolved placeholder tokens (e.g. `[year]`, `[fullname]`)
- `README.md` is checked by exact path at the project root; it is a local diagnostic, not a swatch or config-diff item

## Key implementation details

- Swatches are embedded at build time via `//go:embed swatches/*`
- Five commands: `fit` (bootstrap), `alter` (apply), `baste` (preview), `measure` (inspect), `docket` (inspect)
- `fit`, `alter`, and `baste` require a valid GitHub auth token at startup; `measure` and `docket` do not
- `alter` execution order: repository settings, then labels, then licence, then swatches
- SHA-256 comparison for `always` and `triggered` swatches; substituted swatches (`.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor.yml`, `.github/workflows/tailor-automerge.yml`) compare the resolved content hash against the on-disk file
- `triggered` swatches deploy when their condition is met (overwrite like `always`), remove the file when the condition becomes false, and skip when the file is absent and condition is false
- `--recut` overwrites everything except `LICENSE`; for `.tailor.yml`, recut overrides `first-fit` to `always` (append-only: missing default entries added, existing entries never modified)
- Token substitution: `{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, `{{HOMEPAGE_URL}}`, `{{MERGE_STRATEGY}}`
- Licences fetched via GitHub REST API (`GET /licenses/{id}`), not embedded
- Several repository settings use separate API endpoints rather than the main repo PATCH:
  - `private_vulnerability_reporting_enabled`: `PUT`/`DELETE` toggle, 204/404 status code read
  - `vulnerability_alerts_enabled`: `PUT`/`DELETE` toggle, 204/404 status code read
  - `automated_security_fixes_enabled`: `PUT`/`DELETE` toggle, JSON read
  - `topics`: `PUT` replace-all
  - `default_workflow_permissions` and `can_approve_pull_request_reviews`: `GET`/`PUT` via actions/permissions/workflow endpoint
- `labels` is a top-level config section with its own API layer (`internal/gh/labels.go`) and alter layer (`internal/alter/labels.go`), separate from repository settings
- `validate.go` includes enum validation for `default_workflow_permissions` ("read"|"write"), topic format validation (lowercase alphanumeric start, max 50 chars, lowercase alphanumerics and hyphens only), and label validation (name length, hex colour, description length, duplicate detection)
- Dry-run output uses dynamically computed label width for `baste` (accommodates trigger annotations) and fixed 16 chars for `measure`
- `measure` output order: `missing`, `warning`, `present`, then config-diff categories (`not-configured`, `config-only`, `mode-differs`)
- Triggered swatch output includes annotation, e.g. `would deploy (triggered: allow_auto_merge):`
- Branch protection (classic rules and rulesets) is out of scope: it requires `Administration: write`, which `GITHUB_TOKEN` cannot hold, and branch protection rarely drifts for the solo-dev and small-team audience Tailor targets

## Commit guidelines

- [Conventional Commits](https://www.conventionalcommits.org/) specification
- Common prefixes: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`

## CI token requirements

`GITHUB_TOKEN` covers all Tailor operations on the workflow's own repository except three settings that require admin role on the repository:

- `vulnerability_alerts_enabled`
- `automated_security_fixes_enabled`
- `private_vulnerability_reporting_enabled`

`GITHUB_TOKEN` never holds `administration` permission regardless of `permissions:` in the workflow - this is a GitHub platform constraint. When `GITHUB_TOKEN` is used and these settings appear in `.tailor.yml`, Tailor skips them with a warning and continues without failing. To manage them from CI, set `GH_TOKEN: ${{ secrets.TAILOR_PAT }}` on the workflow step, where `TAILOR_PAT` is a classic PAT with `repo` scope (or fine-grained with `Administration: write`, `Contents: write`, `Issues: write`, `Metadata: read`, `Actions: write`) stored as a repository secret.

### GitHub API permission model

#### Endpoint matrix

| API category | Endpoint(s) | Required scope | `repo` alone sufficient | `GITHUB_TOKEN` in CI sufficient | Notes |
|---|---|---|---|---|---|
| Repo settings PATCH | `PATCH /repos/{owner}/{repo}` | `repo` | Yes | Yes (contents:write) | Caller must have admin role on the repo |
| Topics | `PUT /repos/{owner}/{repo}/topics` | `repo` | Yes | Yes (contents:write) | Fine-grained: Metadata write |
| Labels | `GET/POST/PATCH/DELETE /repos/{owner}/{repo}/labels` | `repo` | Yes | Yes (issues:write for writes) | Labels sit under issues permissions in fine-grained |
| Private vulnerability reporting | `PUT/DELETE /repos/{owner}/{repo}/private-vulnerability-reporting` | `repo` | Yes | No | Requires repo admin or security manager role |
| Vulnerability alerts | `PUT/DELETE /repos/{owner}/{repo}/vulnerability-alerts` | `repo` | Yes | No | Requires admin role; fine-grained needs security-events:write |
| Automated security fixes | `PUT/DELETE /repos/{owner}/{repo}/automated-security-fixes` | `repo` | Yes | No | Same constraints as vulnerability alerts |
| Actions workflow permissions | `GET/PUT /repos/{owner}/{repo}/actions/permissions/workflow` | `repo` | Yes | Yes (actions:write) | Repo-level only; org-level requires admin:org |
| Licence fetch | `GET /licenses/{id}`, `GET /repos/{owner}/{repo}/license` | none (public) | Yes | Yes | Public endpoint; `repo` scope needed for private repos |
| File contents read/write | `GET/PUT /repos/{owner}/{repo}/contents/{path}` | `repo` (write) | Yes | Yes (contents:read/write) | Public read needs no auth; write always requires repo scope |
| Repository creation (user) | `POST /user/repos` | `repo` | Yes | No | `GITHUB_TOKEN` cannot create repos |
| Repository creation (org) | `POST /orgs/{org}/repos` | `repo`/`public_repo` | Yes | No | `GITHUB_TOKEN` cannot create repos in other orgs |

#### Scope header semantics

- `X-OAuth-Scopes`: comma-separated scopes the token holds. Present only on classic PAT responses; absent for `GITHUB_TOKEN` and fine-grained PATs.
- `X-Accepted-OAuth-Scopes`: minimum scopes the endpoint accepts. Present on all authenticated requests.
- `X-Accepted-GitHub-Permissions`: present on 403 responses for fine-grained PATs; contains the required permission, e.g. `pull_requests=write`.

| Code | Meaning |
|---|---|
| 401 | Invalid or expired token |
| 403 | Insufficient scope or insufficient role |
| 404 | Returned instead of 403 for private resources to avoid leaking existence |

Security feature endpoints (e.g. `GET /repos/{owner}/{repo}/vulnerability-alerts`) return 404 when the feature is disabled - not a scope error in the read path.

## Security considerations

- Never store or log GitHub tokens; rely on `go-gh` token resolution for authentication
- Validate swatch `path` values against the embedded set before writing files
- Validate `repository` setting field names against the allowed list before API calls
- Reject duplicate paths in config before making any changes
- Create intermediate directories safely; do not follow symlinks outside project root
