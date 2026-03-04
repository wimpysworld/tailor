package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

// templateFuncs provides helpers for the config template.
var templateFuncs = template.FuncMap{
	"deref": func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	},
	"derefBool": func(p *bool) string {
		if p == nil {
			return ""
		}
		if *p {
			return "true"
		}
		return "false"
	},
	"set": func(p any) bool {
		switch v := p.(type) {
		case *string:
			return v != nil
		case *bool:
			return v != nil
		default:
			return false
		}
	},
}

// configTemplate renders .tailor/config.yml in the exact format specified.
// It uses text/template rather than yaml.Marshal to control key order,
// blank lines between swatch entries, and omission of nil pointer fields.
var configTemplate = template.Must(template.New("config").Funcs(templateFuncs).Parse(
	`# Initially fitted by tailor on {{ .Date }}
license: {{ .License }}

repository:
{{- if set .Repository.Description }}
  description: {{ deref .Repository.Description }}
{{- end }}
{{- if set .Repository.Homepage }}
  homepage: {{ deref .Repository.Homepage }}
{{- end }}
{{- if set .Repository.HasWiki }}
  has_wiki: {{ derefBool .Repository.HasWiki }}
{{- end }}
{{- if set .Repository.HasDiscussions }}
  has_discussions: {{ derefBool .Repository.HasDiscussions }}
{{- end }}
{{- if set .Repository.HasProjects }}
  has_projects: {{ derefBool .Repository.HasProjects }}
{{- end }}
{{- if set .Repository.HasIssues }}
  has_issues: {{ derefBool .Repository.HasIssues }}
{{- end }}
{{- if set .Repository.AllowMergeCommit }}
  allow_merge_commit: {{ derefBool .Repository.AllowMergeCommit }}
{{- end }}
{{- if set .Repository.AllowSquashMerge }}
  allow_squash_merge: {{ derefBool .Repository.AllowSquashMerge }}
{{- end }}
{{- if set .Repository.AllowRebaseMerge }}
  allow_rebase_merge: {{ derefBool .Repository.AllowRebaseMerge }}
{{- end }}
{{- if set .Repository.SquashMergeCommitTitle }}
  squash_merge_commit_title: {{ deref .Repository.SquashMergeCommitTitle }}
{{- end }}
{{- if set .Repository.SquashMergeCommitMessage }}
  squash_merge_commit_message: {{ deref .Repository.SquashMergeCommitMessage }}
{{- end }}
{{- if set .Repository.MergeCommitTitle }}
  merge_commit_title: {{ deref .Repository.MergeCommitTitle }}
{{- end }}
{{- if set .Repository.MergeCommitMessage }}
  merge_commit_message: {{ deref .Repository.MergeCommitMessage }}
{{- end }}
{{- if set .Repository.DeleteBranchOnMerge }}
  delete_branch_on_merge: {{ derefBool .Repository.DeleteBranchOnMerge }}
{{- end }}
{{- if set .Repository.AllowUpdateBranch }}
  allow_update_branch: {{ derefBool .Repository.AllowUpdateBranch }}
{{- end }}
{{- if set .Repository.AllowAutoMerge }}
  allow_auto_merge: {{ derefBool .Repository.AllowAutoMerge }}
{{- end }}
{{- if set .Repository.WebCommitSignoffRequired }}
  web_commit_signoff_required: {{ derefBool .Repository.WebCommitSignoffRequired }}
{{- end }}
{{- if set .Repository.PrivateVulnerabilityReportEnabled }}
  private_vulnerability_reporting_enabled: {{ derefBool .Repository.PrivateVulnerabilityReportEnabled }}
{{- end }}

swatches:
{{- range $i, $s := .Swatches }}
{{ if $i }}
{{ end }}  - source: {{ $s.Source }}
    destination: {{ $s.Destination }}
    alteration: {{ $s.Alteration }}
{{- end }}
`))

// Write renders cfg to <dir>/.tailor/config.yml with the given header date.
func Write(dir string, cfg *Config, date string) error {
	tailorDir := filepath.Join(dir, ".tailor")
	if err := os.MkdirAll(tailorDir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, struct {
		Date string
		*Config
	}{
		Date:   date,
		Config: cfg,
	}); err != nil {
		return fmt.Errorf("rendering config template: %w", err)
	}

	path := filepath.Join(tailorDir, "config.yml")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
