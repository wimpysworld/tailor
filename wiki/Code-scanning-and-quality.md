# Code scanning and quality

[Default merging](Configuration#default-merging) restores absent `code_scanning` and `code_quality` sections and fills missing fields without changing explicit values. Restored sections apply in the same run.

To disable either feature, set its `state: not-configured`. To stop Tailor management without changing the live feature, omit its section and disable default merging.

## Code scanning

The top-level `code_scanning` section manages CodeQL default setup. Generated configs enable default setup with the default query suite, the remote threat model, and GitHub language detection.

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | `configured` or `not-configured` |
| `query_suite` | string | `default` or `extended` |
| `threat_model` | string | `remote` or `remote_and_local` |
| `languages` | string[] | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `actions`, `c-cpp`, `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby`, `swift` |

An empty `languages` list sends no `languages` field, so GitHub detects the languages on enable and keeps the current set afterwards, unlike `topics`, where an empty list clears all topics. Tailor sends only the fields it manages, so `runner_type` and `runner_label` keep the value set in the GitHub UI, and Tailor exposes no setting that needs a paid plan, an Advanced Security licence, or a self-hosted runner. A validation run in progress reports `would skip (setup in progress)`, and an unavailable feature reports `would skip (not available)`. Default setup does not conflict with workflows that upload SARIF results, such as Scorecard.

## Code Quality

The top-level `code_quality` section manages GitHub Code Quality. Generated configs leave Code Quality not configured with GitHub language detection.

| Field | Type | Description |
|-------|------|-------------|
| `state` | string | `configured` or `not-configured` |
| `languages` | string[] | Complete set of languages to analyse. An empty list means GitHub detects them. Accepts `csharp`, `go`, `java-kotlin`, `javascript-typescript`, `python`, `ruby` |

An empty `languages` list sends no `languages` field, so GitHub detects the languages and keeps the current set afterwards. Tailor sends only the fields it manages, so `ai_findings_option`, `runner_type`, and `runner_label` keep the value set in the GitHub UI, and Tailor never spends AI credit on a public repository. The skip results match code scanning.
