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
