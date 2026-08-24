package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"
)

// yamlSpecial lists characters that require quoting in YAML values.
const yamlSpecial = ":{}[]#&*!|>'\"%@`\n"

// templateFuncs provides helpers for the config template.
var templateFuncs = template.FuncMap{
	"yamlVal": func(v string) string {
		if strings.ContainsAny(v, yamlSpecial) || v != strings.TrimSpace(v) || v == "" {
			return fmt.Sprintf("%q", v)
		}
		return v
	},
}

// configTemplate renders .tailor.yml in the exact format specified. It uses
// text/template rather than yaml.Marshal to control key order, blank lines
// between swatch entries, and omission of nil pointer fields.
var configTemplate = template.Must(template.New("config").Funcs(templateFuncs).Parse(
	`# {{ .Verb }} by tailor on {{ .Date }}
license: {{ .License }}

{{- if .Repository }}

repository:
{{- if .Repository.Description }}
  description: {{ yamlVal .Repository.Description }}
{{- end }}
{{- if .Repository.Homepage }}
  homepage: {{ .Repository.Homepage }}
{{- end }}
{{- if .Repository.HasWiki }}
  has_wiki: {{ .Repository.HasWiki }}
{{- end }}
{{- if .Repository.HasDiscussions }}
  has_discussions: {{ .Repository.HasDiscussions }}
{{- end }}
{{- if .Repository.HasProjects }}
  has_projects: {{ .Repository.HasProjects }}
{{- end }}
{{- if .Repository.HasIssues }}
  has_issues: {{ .Repository.HasIssues }}
{{- end }}
{{- if .Repository.AllowMergeCommit }}
  allow_merge_commit: {{ .Repository.AllowMergeCommit }}
{{- end }}
{{- if .Repository.AllowSquashMerge }}
  allow_squash_merge: {{ .Repository.AllowSquashMerge }}
{{- end }}
{{- if .Repository.AllowRebaseMerge }}
  allow_rebase_merge: {{ .Repository.AllowRebaseMerge }}
{{- end }}
{{- if .Repository.SquashMergeCommitTitle }}
  squash_merge_commit_title: {{ yamlVal .Repository.SquashMergeCommitTitle }}
{{- end }}
{{- if .Repository.SquashMergeCommitMessage }}
  squash_merge_commit_message: {{ yamlVal .Repository.SquashMergeCommitMessage }}
{{- end }}
{{- if .Repository.MergeCommitTitle }}
  merge_commit_title: {{ yamlVal .Repository.MergeCommitTitle }}
{{- end }}
{{- if .Repository.MergeCommitMessage }}
  merge_commit_message: {{ yamlVal .Repository.MergeCommitMessage }}
{{- end }}
{{- if .Repository.DeleteBranchOnMerge }}
  delete_branch_on_merge: {{ .Repository.DeleteBranchOnMerge }}
{{- end }}
{{- if .Repository.AllowUpdateBranch }}
  allow_update_branch: {{ .Repository.AllowUpdateBranch }}
{{- end }}
{{- if .Repository.AllowAutoMerge }}
  allow_auto_merge: {{ .Repository.AllowAutoMerge }}
{{- end }}
{{- if .Repository.WebCommitSignoffRequired }}
  web_commit_signoff_required: {{ .Repository.WebCommitSignoffRequired }}
{{- end }}
{{- if .Repository.PrivateVulnerabilityReportEnabled }}
  private_vulnerability_reporting_enabled: {{ .Repository.PrivateVulnerabilityReportEnabled }}
{{- end }}
{{- if .Repository.VulnerabilityAlertsEnabled }}
  vulnerability_alerts_enabled: {{ .Repository.VulnerabilityAlertsEnabled }}
{{- end }}
{{- if .Repository.AutomatedSecurityFixesEnabled }}
  automated_security_fixes_enabled: {{ .Repository.AutomatedSecurityFixesEnabled }}
{{- end }}
{{- if .Repository.DefaultWorkflowPermissions }}
  default_workflow_permissions: {{ .Repository.DefaultWorkflowPermissions }}
{{- end }}
{{- if .Repository.CanApprovePullRequestReviews }}
  can_approve_pull_request_reviews: {{ .Repository.CanApprovePullRequestReviews }}
{{- end }}
{{- if .Repository.Topics }}
  topics:{{ if eq (len .Repository.Topics) 0 }} []{{ else }}
{{- range .Repository.Topics }}
    - {{ yamlVal . }}
{{- end }}{{ end }}
{{- end }}
{{- end }}
{{- if .Actions }}

actions:
{{- if .Actions.Enabled }}
  enabled: {{ .Actions.Enabled }}
{{- end }}
{{- if .Actions.AllowedActions }}
  allowed_actions: {{ .Actions.AllowedActions }}
{{- end }}
{{- if .Actions.SHAPinningRequired }}
  sha_pinning_required: {{ .Actions.SHAPinningRequired }}
{{- end }}
{{- if .Actions.GitHubOwnedAllowed }}
  github_owned_allowed: {{ .Actions.GitHubOwnedAllowed }}
{{- end }}
{{- if .Actions.VerifiedAllowed }}
  verified_allowed: {{ .Actions.VerifiedAllowed }}
{{- end }}
{{- if .Actions.PatternsAllowed }}
  patterns_allowed:{{ if eq (len .Actions.PatternsAllowed) 0 }} []{{ else }}
{{- range .Actions.PatternsAllowed }}
    - {{ yamlVal . }}
{{- end }}{{ end }}
{{- end }}
{{- end }}
{{- if .Labels }}

labels:
{{- range $i, $l := .Labels }}
{{ if $i }}
{{ end }}  - name: {{ yamlVal $l.Name }}
    color: {{ yamlVal $l.Color }}
    description: {{ yamlVal $l.Description }}
{{- end }}
{{- end }}

swatches:
{{- range $i, $s := .Swatches }}
{{ if $i }}
{{ end }}  - path: {{ $s.Path }}
    alteration: {{ $s.Alteration }}
{{- end }}
`))

// Write renders cfg to <dir>/.tailor.yml with the given header date and verb.
func Write(dir string, cfg *Config, date string, verb string) error {
	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, struct {
		Date string
		Verb string
		*Config
	}{
		Date:   date,
		Verb:   verb,
		Config: cfg,
	}); err != nil {
		return fmt.Errorf("rendering config template: %w", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("opening project root %q: %w", dir, err)
	}
	defer root.Close()

	if err := root.WriteFile(configPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
