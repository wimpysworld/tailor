package model

import (
	"reflect"
	"strings"
)

// LabelEntry describes a single GitHub label in the config file.
type LabelEntry struct {
	Name        string `yaml:"name" json:"name"`
	Color       string `yaml:"color" json:"color"`
	Description string `yaml:"description" json:"description"`
}

// ActionsSettings holds repository GitHub Actions policy fields.
// Pointer types distinguish absent fields from zero values.
type ActionsSettings struct {
	Enabled            *bool     `yaml:"enabled,omitempty"`
	AllowedActions     *string   `yaml:"allowed_actions,omitempty"`
	SHAPinningRequired *bool     `yaml:"sha_pinning_required,omitempty"`
	GitHubOwnedAllowed *bool     `yaml:"github_owned_allowed,omitempty"`
	VerifiedAllowed    *bool     `yaml:"verified_allowed,omitempty"`
	PatternsAllowed    *[]string `yaml:"patterns_allowed,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateActions uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// LabelNeedsUpdate reports whether existing differs from desired in name casing,
// colour, or description. Colour comparison is case-insensitive to match GitHub
// behaviour. Name comparison is case-sensitive: the caller already matched these
// entries case-insensitively, so a difference here means a casing rename.
func LabelNeedsUpdate(existing, desired LabelEntry) bool {
	return existing.Name != desired.Name ||
		!strings.EqualFold(existing.Color, desired.Color) ||
		existing.Description != desired.Description
}

// RepositorySettings holds GitHub repository configuration fields.
// Pointer types distinguish absent fields from zero values.
type RepositorySettings struct {
	Description                       *string   `yaml:"description,omitempty"`
	Homepage                          *string   `yaml:"homepage,omitempty"`
	HasWiki                           *bool     `yaml:"has_wiki,omitempty"`
	HasDiscussions                    *bool     `yaml:"has_discussions,omitempty"`
	HasProjects                       *bool     `yaml:"has_projects,omitempty"`
	HasIssues                         *bool     `yaml:"has_issues,omitempty"`
	AllowMergeCommit                  *bool     `yaml:"allow_merge_commit,omitempty"`
	AllowSquashMerge                  *bool     `yaml:"allow_squash_merge,omitempty"`
	AllowRebaseMerge                  *bool     `yaml:"allow_rebase_merge,omitempty"`
	SquashMergeCommitTitle            *string   `yaml:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage          *string   `yaml:"squash_merge_commit_message,omitempty"`
	MergeCommitTitle                  *string   `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage                *string   `yaml:"merge_commit_message,omitempty"`
	DeleteBranchOnMerge               *bool     `yaml:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch                 *bool     `yaml:"allow_update_branch,omitempty"`
	AllowAutoMerge                    *bool     `yaml:"allow_auto_merge,omitempty"`
	WebCommitSignoffRequired          *bool     `yaml:"web_commit_signoff_required,omitempty"`
	PrivateVulnerabilityReportEnabled *bool     `yaml:"private_vulnerability_reporting_enabled,omitempty"`
	VulnerabilityAlertsEnabled        *bool     `yaml:"vulnerability_alerts_enabled,omitempty"`
	AutomatedSecurityFixesEnabled     *bool     `yaml:"automated_security_fixes_enabled,omitempty"`
	Topics                            *[]string `yaml:"topics,omitempty"`
	DefaultWorkflowPermissions        *string   `yaml:"default_workflow_permissions,omitempty"`
	CanApprovePullRequestReviews      *bool     `yaml:"can_approve_pull_request_reviews,omitempty"`
	SecretScanning                    *string   `yaml:"secret_scanning,omitempty"`
	SecretScanningPushProtection      *string   `yaml:"secret_scanning_push_protection,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateRepoSettings uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// CodeScanningLanguages lists the languages that code scanning default setup
// accepts, in the order the config template documents them.
var CodeScanningLanguages = []string{"actions", "c-cpp", "csharp", "go", "java-kotlin", "javascript-typescript", "python", "ruby", "swift"}

// CodeQualityLanguages lists the languages that Code Quality setup accepts,
// in the order the config template documents them.
var CodeQualityLanguages = []string{"csharp", "go", "java-kotlin", "javascript-typescript", "python", "ruby"}

// CodeScanningSettings holds the managed code scanning default setup fields.
// Pointer types distinguish absent fields from zero values. An empty
// Languages list means GitHub detects the languages.
type CodeScanningSettings struct {
	State       *string   `yaml:"state,omitempty"`        // configured | not-configured
	QuerySuite  *string   `yaml:"query_suite,omitempty"`  // default | extended
	ThreatModel *string   `yaml:"threat_model,omitempty"` // remote | remote_and_local
	Languages   *[]string `yaml:"languages,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateCodeScanning uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// CodeQualitySettings holds the managed Code Quality setup fields. Pointer
// types distinguish absent fields from zero values. An empty Languages list
// means GitHub detects the languages.
type CodeQualitySettings struct {
	State     *string   `yaml:"state,omitempty"` // configured | not-configured
	Languages *[]string `yaml:"languages,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateCodeQuality uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// SettingField describes one supported setting field, keyed by its yaml tag.
type SettingField struct {
	YAMLKey string
	Index   int
	Value   reflect.Value
	Set     bool
}

// RepositorySettingFields returns supported repository settings in struct order.
// Extra is excluded because it stores unknown YAML keys.
func RepositorySettingFields(settings *RepositorySettings) []SettingField {
	return settingFields(settings)
}

// ActionsSettingFields returns supported Actions settings in struct order.
// Extra is excluded because it stores unknown YAML keys.
func ActionsSettingFields(settings *ActionsSettings) []SettingField {
	return settingFields(settings)
}

// CodeScanningSettingFields returns supported code scanning settings in
// struct order. Extra is excluded because it stores unknown YAML keys.
func CodeScanningSettingFields(settings *CodeScanningSettings) []SettingField {
	return settingFields(settings)
}

// CodeQualitySettingFields returns supported Code Quality settings in struct
// order. Extra is excluded because it stores unknown YAML keys.
func CodeQualitySettingFields(settings *CodeQualitySettings) []SettingField {
	return settingFields(settings)
}

// settingFields walks the yaml-tagged pointer fields of a settings struct.
func settingFields[T any](settings *T) []SettingField {
	t := reflect.TypeFor[T]()
	var v reflect.Value
	if settings != nil {
		v = reflect.ValueOf(settings).Elem()
	}

	fields := make([]SettingField, 0, t.NumField())
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := sf.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")
		if key == "" {
			continue
		}

		field := SettingField{
			YAMLKey: key,
			Index:   i,
		}
		if v.IsValid() {
			field.Value = v.Field(i)
			field.Set = field.Value.Kind() == reflect.Pointer && !field.Value.IsNil()
		}
		fields = append(fields, field)
	}
	return fields
}
