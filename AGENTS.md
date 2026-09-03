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
- Run linters: `just lint` (or `gocyclo -over 30 -top 20 -avg -ignore '_test\.go$' . && golangci-lint run && actionlint`)
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
- `measure` emits `warning` results for three local health diagnostics: missing `README.md` (not managed by tailor), `LICENSE` files containing unresolved placeholder tokens (e.g. `[year]`, `[fullname]`), and `LICENSE` files that exist but were not inspected (over 1 MiB or unreadable)
- `README.md` is checked by exact path at the project root; it is a local diagnostic, not a swatch or config-diff item

## Key implementation details

- Swatches are embedded at build time via `//go:embed swatches/*`
- Five commands: `fit` (bootstrap), `alter` (apply), `baste` (preview), `measure` (inspect), `docket` (inspect)
- `fit`, `alter`, and `baste` require a valid GitHub auth token at startup; `measure` and `docket` do not
- Run `alter` stages in this order: config migration and update, retired workflow cleanup, repository settings, Actions policy, code scanning, Code Quality, ruleset, labels, licence, then swatches
- Before strict path and mode validation, prune `.github/workflows/tailor-automerge.yml` and `.github/workflows/tailor.yml` from config
- Accept legacy `triggered` only for these retired entries during migration
- Always check both fixed retired paths, regardless of config or mode
- `baste` reports `would remove` without writes
- `alter` and `alter --recut` report `removed` only after deletion succeeds
- Apply SHA-256 comparison only to `always` swatches other than `.tailor.yml`; hash token-bearing content after substitution
- Keep `.tailor.yml` updates append-only: add missing defaults, but never modify existing entries; `--recut` enables this merge for `first-fit`
- Substitute `{{GITHUB_USERNAME}}`, `{{ADVISORY_URL}}`, and `{{SUPPORT_URL}}` only in their registered embedded swatches
- Licences fetched via GitHub REST API (`GET /licenses/{id}`), not embedded
- Several repository settings use separate API endpoints rather than the main repo PATCH: `topics`, `default_workflow_permissions`, and `can_approve_pull_request_reviews`; see `internal/gh/settings.go` for implementation
- `labels` is a top-level config section with its own API layer (`internal/gh/labels.go`) and alter layer (`internal/alter/labels.go`), separate from repository settings
- `code_scanning` is a top-level config section for CodeQL default setup with its own API layer (`internal/gh/codescanning.go`) and alter layer (`internal/alter/codescanning.go`); it uses `GET`/`PATCH /repos/{owner}/{repo}/code-scanning/default-setup`
- `code_quality` is a top-level config section for GitHub Code Quality with its own API layer (`internal/gh/codequality.go`) and alter layer (`internal/alter/codequality.go`); it uses `GET`/`PATCH /repos/{owner}/{repo}/code-quality/setup`
- `secret_scanning`, `secret_scanning_push_protection`, and `secret_scanning_non_provider_patterns` are `repository` fields that travel in the `security_and_analysis` object of the repository PATCH body; send only the declared keys
- Push protection and non-provider patterns (the GitHub UI calls them generic patterns) require secret scanning: normalise `secret_scanning` to `enabled` with a warning, on the same write path as the automated security fixes prerequisite
- Free features only: expose no setting that needs a paid plan, an Advanced Security or Secret Protection licence, a self-hosted runner, or AI credit spend; never manage `runner_type`, `runner_label`, `ai_findings_option`, or `security_and_analysis` keys other than the three managed keys
- An empty `languages` list in `code_scanning` or `code_quality` sends no `languages` field, so GitHub detects the languages; a non-empty list is the complete set, compared as a set (unlike `topics`, where an empty list clears all topics)
- Default setup writes return `202`; report `409` as `would skip (setup in progress)` and `403` as `would skip (not available)`, and continue
- `ruleset` is a top-level config section that manages one branch ruleset named `Tailor` with its own API layer (`internal/gh/ruleset.go`) and alter layer (`internal/alter/ruleset.go`); it uses list, get, `POST`, and `PUT` on `/repos/{owner}/{repo}/rulesets`
- Tailor owns the `Tailor` ruleset entirely: every write sends the complete ruleset, and Tailor never deletes it or touches any other ruleset
- The `rules` map keys are API rule types; `enabled` on `pull_request`, `required_status_checks`, and `code_scanning` is Tailor's own key, because these three rules carry parameters that stay in the config while the rule is off
- The ruleset `code_scanning` rule is off by default with one `CodeQL` entry at `errors` and `high_or_higher`, because the rule blocks a merge until the tool has results for both the pull request commit and the base branch, and do not cross-check it against the top-level `code_scanning.state`
- `enforcement` accepts `active` or `disabled` only; `evaluate` is Enterprise only and is rejected
- When `allowed_merge_methods` names a method whose `repository` setting is `false`, emit `warning: ruleset allows <method> merging but repository.<field> is false` and continue without changing either value
- Ruleset writes return `201` (`POST`) or `200` (`PUT`); report `403` as `would skip (not available)` and a read without `bypass_actors` as `would skip (insufficient scope)`, and continue; stop on `422`
- `validate.go` includes enum validation for `default_workflow_permissions` ("read"|"write"), topic format validation (lowercase alphanumeric start, max 50 chars, lowercase alphanumerics and hyphens only), and label validation (name length, hex colour, description length, duplicate detection)
- Dry-run output uses dynamically computed label width for `baste` and fixed 16 chars for `measure`
- `measure` output order: `missing`, `warning`, `present`, then config-diff categories (`not-configured`, `config-only`, `mode-differs`)
- Classic branch protection rules are out of scope; rulesets outside the `Tailor` ruleset are out of scope

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
- Never implement classic branch protection rules, and never read, write, or delete a ruleset other than `Tailor`
- Never log or store GitHub tokens
- Field names in `repository` config must match GitHub REST API names exactly; never rename or alias them
- Swatch-to-path mappings are hardcoded in source, not configurable at runtime
- `docs/SPECIFICATION.md` is authoritative; implementation decisions must align with it
