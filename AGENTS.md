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
- Adding a new triggered swatch requires: an entry in `triggerConditions` (trigger.go), a registry entry (registry.go), and inclusion in `swatches/.tailor/config.yml`

## Testing

- Table-driven tests following Go conventions
- Test files sit alongside the code they test (`*_test.go`)
- Test swatch embedding and config parsing without network access
- Commands that call `gh` should have their external calls abstracted behind interfaces for testability
- `measure` is purely local and needs no mocking

## Key implementation details

- Swatches are embedded at build time via `//go:embed swatches/*`
- Five commands: `fit` (bootstrap), `alter` (apply), `baste` (preview), `measure` (inspect), `docket` (inspect)
- `fit`, `alter`, and `baste` require a valid GitHub auth token at startup; `measure` and `docket` do not
- `alter` execution order: repository settings, then licence, then swatches
- SHA-256 comparison for `always` and `triggered` swatches; substituted swatches (`.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor/config.yml`, `.github/workflows/tailor-automerge.yml`) compare the resolved content hash against the on-disk file
- `triggered` swatches deploy when their condition is met (overwrite like `always`), remove the file when the condition becomes false, and skip when the file is absent and condition is false
- `--recut` overwrites everything except `LICENSE`; for `.tailor/config.yml`, recut overrides `first-fit` to `always` (append-only: missing default entries added, existing entries never modified)
- Token substitution: `{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, `{{HOMEPAGE_URL}}`, `{{MERGE_STRATEGY}}`
- Licences fetched via GitHub REST API (`GET /licenses/{id}`), not embedded
- `private_vulnerability_reporting_enabled` uses a separate API endpoint (`PUT`/`DELETE`)
- Dry-run output uses dynamically computed label width for `baste` (accommodates trigger annotations) and fixed 16 chars for `measure`
- Triggered swatch output includes annotation, e.g. `would deploy (triggered: allow_auto_merge):`

## Commit guidelines

- [Conventional Commits](https://www.conventionalcommits.org/) specification
- Common prefixes: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`

## Security considerations

- Never store or log GitHub tokens; rely on `go-gh` token resolution for authentication
- Validate swatch `source` values against the embedded set before writing files
- Validate `repository` setting field names against the allowed list before API calls
- Reject duplicate destinations in config before making any changes
- Create intermediate directories safely; do not follow symlinks outside project root
