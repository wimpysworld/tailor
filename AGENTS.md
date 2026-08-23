# AGENTS.md

## Project overview

Tailor is a local terminal CLI for managing project templates (swatches) across GitHub repositories. It fits new projects with community health files, dev tooling, and repository settings, then updates them through explicit CLI alterations.

The authoritative specification is `docs/SPECIFICATION.md`. All implementation decisions must align with it.

## Tech stack

- **Language**: Go (1.26+)
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
├── swatches/           # Embedded template files (16 swatches)
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
- Three alteration modes: `always`, `first-fit`, `never`
- Adding a new swatch requires: the file in `swatches/`, a registry entry (`registry.go`), an entry in `swatches/.tailor.yml`, updated count assertions in `registry_test.go` and any golden-string test fixtures, plus updates to `docs/SPECIFICATION.md` and `README.md`

## Testing

- Table-driven tests following Go conventions
- Test files sit alongside the code they test (`*_test.go`)
- Test swatch embedding and config parsing without network access
- Commands that call `gh` should have their external calls abstracted behind interfaces for testability
- Test authentication through local `GH_TOKEN`, `GITHUB_TOKEN`, and `gh` credential paths only
- `measure` is purely local and needs no mocking
- `measure` emits `warning` results for two local health diagnostics: missing `README.md` (not managed by tailor) and `LICENSE` files containing unresolved placeholder tokens (e.g. `[year]`, `[fullname]`)
- `README.md` is checked by exact path at the project root; it is a local diagnostic, not a swatch or config-diff item

## Key implementation details

- Swatches are embedded at build time via `//go:embed swatches/*`
- Five commands: `fit` (bootstrap), `alter` (apply), `baste` (preview), `measure` (inspect), `docket` (inspect)
- `fit`, `alter`, and `baste` require a valid GitHub auth token at startup; `measure` and `docket` do not
- Run `alter` stages in this order: config migration and update, retired workflow cleanup, repository settings, labels, licence, then swatches
- Before strict path and mode validation, prune `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml` from config
- Accept legacy `triggered` only for these retired entries during migration
- Always check both fixed retired paths, regardless of config or mode
- `baste` reports `would remove` without writes
- `alter` and `alter --recut` report `removed` only after deletion succeeds
- SHA-256 comparison applies to `always` swatches; substituted swatches (`.github/FUNDING.yml`, `SECURITY.md`, `.github/ISSUE_TEMPLATE/config.yml`, `.tailor.yml`) compare the resolved content hash against the on-disk file
- `--recut` overwrites everything except `LICENSE`; for `.tailor.yml`, recut overrides `first-fit` to `always` (append-only: missing default entries added, existing entries never modified)
- Token substitution: `{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, `{{SUPPORT_URL}}`, `{{HOMEPAGE_URL}}`
- Licences fetched via GitHub REST API (`GET /licenses/{id}`), not embedded
- Several repository settings use separate API endpoints rather than the main repo PATCH: `topics`, `default_workflow_permissions`, and `can_approve_pull_request_reviews`; see `internal/gh/settings.go` for implementation
- `labels` is a top-level config section with its own API layer (`internal/gh/labels.go`) and alter layer (`internal/alter/labels.go`), separate from repository settings
- `validate.go` includes enum validation for `default_workflow_permissions` ("read"|"write"), topic format validation (lowercase alphanumeric start, max 50 chars, lowercase alphanumerics and hyphens only), and label validation (name length, hex colour, description length, duplicate detection)
- Dry-run output uses dynamically computed label width for `baste` and fixed 16 chars for `measure`
- `measure` output order: `missing`, `warning`, `present`, then config-diff categories (`not-configured`, `config-only`, `mode-differs`)
- Branch protection (classic rules and rulesets) is out of scope

## Commit guidelines

- [Conventional Commits](https://www.conventionalcommits.org/) specification
- Common prefixes: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`

## Security considerations

- Never store or log GitHub tokens; rely on `go-gh` token resolution for authentication
- Validate swatch `path` values against the embedded set before writing files
- Validate `repository` setting field names against the allowed list before API calls
- Reject duplicate paths in config before making any changes
- Create intermediate directories safely; do not follow symlinks outside project root
- Use rooted filesystem access for retired workflow cleanup
- Reject symlinked parents and directories
- Remove a destination symlink without following it, and keep parent directories

## Constraints

- Never add a `pkg/` directory; internal packages go in `internal/`
- Never make alteration modes runtime-configurable; `always`, `first-fit`, `never` are compile-time constants
- Keep Tailor as a local terminal CLI; never add GitHub Actions runtime detection, execution fallbacks, or installation-token handling
- Keep Tailor's own CI, the Dependabot swatch, and repository workflow permission settings
- Never implement branch protection (classic rules or rulesets) - out of scope by design
- Never log or store GitHub tokens
- Field names in `repository` config must match GitHub REST API names exactly; never rename or alias them
- Swatch-to-path mappings are hardcoded in source, not configurable at runtime
- `docs/SPECIFICATION.md` is authoritative; implementation decisions must align with it
