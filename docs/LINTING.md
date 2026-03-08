# Go Linting Configuration

## Overview

Tailor's golangci-lint configuration targets three goals: catch real bugs (resource leaks, error handling, security), prevent duplicate words, hardcoded constants, stale idioms, and keep contributor friction low. With 14 explicitly enabled linters and selective govet/revive rules, the config sits in the moderate tier of the Go ecosystem, comparable to Prometheus in philosophy but leaner in total linter count. Projects at the strict end (Traefik, Gitea, Moby) enable 25-50 linters; projects at the lenient end (GitHub CLI, Kubernetes) enable 14-19 but disable many sub-checks.

## Enabled Linters

| Linter | Rationale |
|--------|-----------|
| bodyclose | Detects unclosed HTTP response bodies, a common source of resource leaks |
| copyloopvar | Flags loop variable copies that are unnecessary in modern Go |
| dupword | Catches duplicate words in comments and strings, particularly AI-generated repetition |
| errorlint | Enforces correct use of `errors.Is` and `errors.As` over direct comparison |
| gocritic | Default checker set covering a broad range of code improvement suggestions |
| gosec | Security-focused analysis (SQL injection, hardcoded credentials, weak crypto) |
| misspell | Catches typos in comments, strings, and identifiers |
| noctx | Flags HTTP requests made without an explicit context, enforcing cancellation support |
| revive | Configurable linter replacing golint, run with 18 explicit rules (see below) |
| staticcheck | The most widely adopted Go linter after govet, catches bugs govet misses |
| unconvert | Removes unnecessary type conversions |
| unparam | Detects unused function parameters |
| usestdlibvars | Flags hardcoded HTTP status codes and methods that should use `net/http` constants |
| wastedassign | Detects assignments to variables that are never subsequently used |

## Configuration Choices

### govet: enable-all minus fieldalignment and shadow

`govet` runs with all analysers enabled, then disables two. This matches Moby, Prometheus, and Traefik exactly. Every project that uses `enable-all` disables `fieldalignment` (struct padding optimisation creates churn and hurts readability). Most also disable `shadow` (variable shadowing reports are noisy in idiomatic Go, particularly with `err`).

### revive: 18 explicit rules

Rather than accepting revive's defaults (which change between versions), tailor specifies 18 rules explicitly: `blank-imports`, `context-as-argument`, `dot-imports`, `error-return`, `error-strings`, `error-naming`, `exported`, `increment-decrement`, `var-naming`, `range`, `receiver-naming`, `time-naming`, `unexported-return`, `indent-error-flow`, `errorf`, `empty-block`, `superfluous-else`, `unreachable-code`. This set closely overlaps with Prometheus (22 rules) and Gitea (17 rules), covering the consensus expectations for Go code style without venturing into subjective territory.

### gofumpt as formatter

`gofumpt` is a stricter superset of `gofmt`. Prometheus, Caddy, Gitea, and Traefik all use it. It eliminates formatting ambiguity that `gofmt` leaves (unnecessary blank lines, grouped var blocks, consistent case formatting).

### gosec: G306 exclusion

G306 flags file creation with permissions broader than 0600. This produces false positives for files that legitimately need group or world read access. Tailor's exclusion is conservative compared to the ecosystem: Moby excludes ten gosec rules, Caddy excludes four, and GitHub CLI disables gosec entirely.

### Exclusion presets

The presets `comments`, `common-false-positives`, `legacy`, and `std-error-handling` match Caddy and Gitea. These suppress known noisy patterns (comment linting on generated code, `fmt.Println` error ignoring, legacy Go patterns) without masking real issues.

## Ecosystem Survey

All configs examined were golangci-lint v2 format unless noted. "Linters enabled" counts explicitly enabled linters beyond whatever defaults each project uses.

| Project | Linters enabled | Approach | govet | gocritic | revive rules | Formatter | Strictness |
|---------|----------------|----------|-------|----------|-------------|-----------|------------|
| **Tailor** | 14 | Selective enable | enable-all (minus fieldalignment, shadow) | Default checks | 18 rules | gofumpt | Moderate |
| **Moby/Docker** | 28 | Selective enable | enable-all (minus fieldalignment) | enable-all (38 checks disabled) | 7 rules | gofmt, goimports | High |
| **Prometheus** | 19 | Selective enable | enable-all (minus shadow, fieldalignment) | enable-all (28 checks disabled) | 22 rules | gci, gofumpt, goimports | High |
| **Caddy** | 26 | default: none, selective | default | default | none | gci, gofmt, gofumpt, goimports | High |
| **Gitea** | 24 | default: none, selective | nilness, unusedwrite only | 1 enabled, 2 disabled | 17 rules (severity: error) | gofmt, gofumpt | High |
| **Traefik** | ~50 | **default: all**, selective disable | enable-all (minus shadow, fieldalignment) | default | 19 rules | gci, gofumpt | Very high |
| **Kubernetes** | 14 | default: none, selective | default (limited checks) | 2 enabled, 10 disabled | 1 rule (exported) | none specified | Moderate |
| **GitHub CLI** | 19 | default: none, selective | httpresponse only | disabled style tag | none | gofmt | Low-moderate |
| **Hugo** | 0 | No golangci-lint | N/A | N/A | N/A | N/A | Minimal |

### Alignment with the ecosystem

- **Selective enabling** is the dominant pattern. Only Traefik uses `default: all` with a long disable list, resulting in 300+ lines of exclusion rules.
- **govet enable-all** matches Moby, Prometheus, and Traefik. The `fieldalignment` and `shadow` exclusions are near-universal.
- **revive with explicit rules** is standard. Tailor's 18 rules closely overlap with Prometheus (22) and Gitea (17).
- **gosec with G306 excluded** is conservative relative to Moby (10 exclusions) and GitHub CLI (disabled entirely).
- **Exclusion presets** match Caddy and Gitea exactly.

## Linters Deliberately Excluded

| Linter | Reason for exclusion |
|--------|---------------------|
| prealloc | No other surveyed project enables it. Traefik explicitly disables it ("Too many false-positive"). Performance gains from pre-allocating slices are negligible outside hot paths. |
| depguard | Prevents use of deprecated or unwanted packages. Every large project uses it, but tailor's dependency surface is small enough that code review suffices. |
| exhaustive | Enum switch exhaustiveness. Only Moby and Caddy use it, both requiring configuration to avoid noise. |
| funlen, gocognit, cyclop | Function length and complexity linters create significant contributor friction. Even Traefik sets funlen to 120 statements. |
| wsl, nlreturn | Whitespace style linters. Traefik explicitly disables both ("Too strict"). |
| testpackage, paralleltest, tparallel | Test structure linters. Traefik disables them ("Not relevant"). |
| ireturn, wrapcheck, varnamelen | Traefik disables all three as too strict. |
| modernize | Suggests modern Go idioms. Useful but flags existing code that contributors did not write, creating churn in unrelated PRs. |
| errcheck | Most projects enable it with exclusions for common false positives. Tailor's govet and errorlint coverage catches the high-value error handling issues. |

## Dependency Review Security Gate

The `security` job in `.github/workflows/builder.yml` runs `actions/dependency-review-action@v4` on every pull request to `main`. It blocks merges that introduce a vulnerable or licence-incompatible dependency before code reaches the default branch.

### Trigger and CI graph position

The job runs only on `pull_request` events (`if: github.event_name == 'pull_request'`). On push, tag, and dispatch events it is skipped. `sentinel` depends on `security` alongside `lint-code`, `lint-actions`, `coverage`, and `build-test`; because `sentinel` checks only for `failure` or `cancelled` results, a skipped `security` job passes sentinel on non-PR events. Branch protection targets `sentinel` exclusively, so no additional required-status-check entries are needed.

### Policy

| Setting | Value | Rationale |
|---------|-------|-----------|
| `fail-on-severity` | `moderate` | Blocks moderate, high, and critical CVEs; low-only vulnerabilities pass. Matches GitHub's recommended starting point. |
| `deny-licenses` | `GPL-2.0, GPL-3.0, AGPL-3.0-only, AGPL-3.0-or-later, LGPL-2.1, LGPL-3.0` | Deny-list approach per GitHub best practice. Copyleft licences are incompatible with typical Go project distribution; tailor's own dependencies are MIT/BSD/Apache-2.0. |
| `fail-on-scopes` | `runtime` | Checks runtime dependencies only; test-only imports are excluded to reduce false positives. |
| `comment-summary-in-pr` | `on-failure` | Posts a summary comment on the PR when the job fails; no noise on clean PRs. |


## Workflow Linting (actionlint)

`actionlint` is a static analyser for GitHub Actions workflow files. It checks for undefined expressions, incorrect context references (`github.*`, `env.*`, `secrets.*`), missing `needs` dependencies, invalid runner labels, shell script errors in `run:` steps (via shellcheck), and type mismatches in action inputs.

### CI job: `lint-actions`

The `lint-actions` job runs on every push and pull request event using `devops-actions/actionlint@v0.1.11`, a versioned wrapper around the `rhysd/actionlint` binary. It uses an `ubuntu-slim` runner and requires only a checkout, with no Go toolchain dependency.

`sentinel` depends on `lint-actions` alongside `lint-code`, `coverage`, `build-test`, and `security`. Failure blocks merge through the same sentinel gating pattern as every other required check.

### PR annotations

On pull request events, `devops-actions/actionlint` posts inline review comments on the Files Changed tab via the GitHub API, pinning each error to the exact line that introduced it. This is more contributor-visible than check-run annotations in the Checks tab, which require navigating away from the diff.

### `pull-requests: write` permission

The job declares `pull-requests: write` at job scope to enable the inline annotation API calls. The action uses this permission solely to post comments; it has no write path to repository content or PR metadata (it cannot merge, label, or modify the PR). The permission is not declared at workflow scope, so no other job inherits it.

| Permission | Scope | Purpose |
|------------|-------|---------|
| `contents: read` | `lint-actions` job | Checkout |
| `pull-requests: write` | `lint-actions` job | Inline diff annotations via GitHub API |

The alternative, a problem matcher with `::add-matcher::` and a direct binary invocation, achieves Checks-tab annotations with `contents: read` only, but requires committing `actionlint-matcher.json` to the repository and managing the binary download separately. The inline diff surfacing justifies the narrowly scoped permission increase.

### Local usage

`actionlint` is available in the dev shell via `flake.nix`. The `just lint` recipe runs it alongside `golangci-lint`, so contributors get the same workflow validation locally before pushing.

### Risks

| Risk | Impact | Notes |
|------|--------|-------|
| `pull-requests: write` broadens job permissions | Slightly elevated trust for the linting job | Scoped to `lint-actions` only; `devops-actions/actionlint` uses it solely to post annotations. |
| `actionlint` flags unknown runner labels | False positives for custom or hosted runners | Pass `action-flags: -ignore 'label ".+" is unknown'` if the repo uses self-hosted runners with custom labels. |
| `devops-actions/actionlint` is a low-star wrapper (9 stars) | Third-party action risk | Mitigated by version-tag pinning; Dependabot tracks updates automatically; the action shells out to official `rhysd/actionlint` releases. |

## Source Configs

All configs were retrieved via the GitHub API on 6 March 2026.

| Project | Config location | Config format version |
|---------|----------------|----------------------|
| Moby/Docker | `.golangci.yml` (repo root) | v2 |
| Prometheus | `.golangci.yml` (repo root) | v2 |
| Caddy | `.golangci.yml` (repo root) | v2 |
| Gitea | `.golangci.yml` (repo root) | v2 |
| Traefik | `.golangci.yml` (repo root) | v2 |
| Kubernetes | `hack/golangci.yaml` | v2 |
| GitHub CLI | `.golangci.yml` (repo root) | v2 |
| Hugo | No golangci-lint config found | N/A |
