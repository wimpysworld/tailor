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
	Description                  *string   `yaml:"description,omitempty"`
	Homepage                     *string   `yaml:"homepage,omitempty"`
	HasWiki                      *bool     `yaml:"has_wiki,omitempty"`
	HasDiscussions               *bool     `yaml:"has_discussions,omitempty"`
	HasProjects                  *bool     `yaml:"has_projects,omitempty"`
	HasIssues                    *bool     `yaml:"has_issues,omitempty"`
	AllowMergeCommit             *bool     `yaml:"allow_merge_commit,omitempty"`
	AllowSquashMerge             *bool     `yaml:"allow_squash_merge,omitempty"`
	AllowRebaseMerge             *bool     `yaml:"allow_rebase_merge,omitempty"`
	SquashMergeCommitTitle       *string   `yaml:"squash_merge_commit_title,omitempty"`
	SquashMergeCommitMessage     *string   `yaml:"squash_merge_commit_message,omitempty"`
	MergeCommitTitle             *string   `yaml:"merge_commit_title,omitempty"`
	MergeCommitMessage           *string   `yaml:"merge_commit_message,omitempty"`
	DeleteBranchOnMerge          *bool     `yaml:"delete_branch_on_merge,omitempty"`
	AllowUpdateBranch            *bool     `yaml:"allow_update_branch,omitempty"`
	AllowAutoMerge               *bool     `yaml:"allow_auto_merge,omitempty"`
	WebCommitSignoffRequired     *bool     `yaml:"web_commit_signoff_required,omitempty"`
	Topics                       *[]string `yaml:"topics,omitempty" json:"topics,omitempty"`
	DefaultWorkflowPermissions   *string   `yaml:"default_workflow_permissions,omitempty"`
	CanApprovePullRequestReviews *bool     `yaml:"can_approve_pull_request_reviews,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateRepoSettings uses this to reject unrecognised settings.
	Extra map[string]interface{} `yaml:",inline"`
}

// RepositorySettingField describes one supported repository setting.
type RepositorySettingField struct {
	YAMLKey string
	GoName  string
	Index   int
	Value   reflect.Value
	Set     bool
}

// RepositorySettingFields returns supported repository settings in struct order.
// Extra is excluded because it stores unknown YAML keys.
func RepositorySettingFields(settings *RepositorySettings) []RepositorySettingField {
	t := reflect.TypeOf(RepositorySettings{})
	var v reflect.Value
	if settings != nil {
		v = reflect.ValueOf(settings).Elem()
	}

	fields := make([]RepositorySettingField, 0, t.NumField())
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

		field := RepositorySettingField{
			YAMLKey: key,
			GoName:  sf.Name,
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
