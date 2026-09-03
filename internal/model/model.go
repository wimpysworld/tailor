package model

import (
	"reflect"
	"slices"
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
	SecretScanningNonProviderPatterns *string   `yaml:"secret_scanning_non_provider_patterns,omitempty"`

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

// RulesetName is the name of the one branch ruleset that Tailor manages.
const RulesetName = "Tailor"

// RulesetTarget is the target of the managed ruleset. Tailor manages branch
// rulesets only.
const RulesetTarget = "branch"

// RulesetEnforcements lists the enforcement levels that Tailor accepts.
// GitHub also accepts "evaluate", which is available only on GitHub
// Enterprise, so Tailor rejects it.
var RulesetEnforcements = []string{"active", "disabled"}

// RulesetActorTypes lists the bypass actor types in the order the config
// template documents them.
var RulesetActorTypes = []string{"RepositoryRole", "Team", "User", "Integration", "DeployKey"}

// RulesetBypassModes lists the bypass modes in the order the config template
// documents them.
var RulesetBypassModes = []string{"always", "pull_request", "exempt"}

// RulesetMergeMethods lists the merge methods a pull request rule accepts, in
// the order the config template documents them.
var RulesetMergeMethods = []string{"merge", "squash", "rebase"}

// RulesetAlertsThresholds lists the code scanning alert thresholds, in the
// order the config template documents them.
var RulesetAlertsThresholds = []string{"none", "errors", "errors_and_warnings", "all"}

// RulesetSecurityAlertsThresholds lists the code scanning security alert
// thresholds, in the order the config template documents them.
var RulesetSecurityAlertsThresholds = []string{"none", "critical", "high_or_higher", "medium_or_higher", "all"}

// RulesetRepositoryRole pairs a RepositoryRole actor_id with its name.
type RulesetRepositoryRole struct {
	ID   int
	Name string
}

// RulesetRepositoryRoles lists the built-in repository roles that a
// RepositoryRole bypass actor can name, in the order the config template
// documents them.
var RulesetRepositoryRoles = []RulesetRepositoryRole{{2, "maintain"}, {4, "write"}, {5, "admin"}}

// RulesetSettings holds the managed fields of the Tailor branch ruleset.
// Pointer types distinguish absent fields from zero values.
type RulesetSettings struct {
	Enforcement  *string               `yaml:"enforcement,omitempty"` // active | disabled
	BypassActors *[]RulesetBypassActor `yaml:"bypass_actors,omitempty"`
	Conditions   *RulesetConditions    `yaml:"conditions,omitempty"`
	Rules        *RulesetRules         `yaml:"rules,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	// ValidateRuleset uses this to reject unrecognised settings.
	Extra map[string]any `yaml:",inline"`
}

// RulesetBypassActor describes one actor that can bypass the ruleset.
// ActorID is nil for a DeployKey actor.
type RulesetBypassActor struct {
	ActorID    *int    `yaml:"actor_id,omitempty"`
	ActorType  *string `yaml:"actor_type,omitempty"`
	BypassMode *string `yaml:"bypass_mode,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetConditions holds the conditions that select the branches the
// ruleset governs.
type RulesetConditions struct {
	RefName *RulesetRefName `yaml:"ref_name,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetRefName holds the branch name patterns the ruleset includes and
// excludes.
type RulesetRefName struct {
	Include *[]string `yaml:"include,omitempty"`
	Exclude *[]string `yaml:"exclude,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetRules holds the rule types that Tailor manages. A true Boolean adds
// the rule of that type to the ruleset.
type RulesetRules struct {
	Creation              *bool                `yaml:"creation,omitempty"`
	Update                *bool                `yaml:"update,omitempty"`
	Deletion              *bool                `yaml:"deletion,omitempty"`
	RequiredLinearHistory *bool                `yaml:"required_linear_history,omitempty"`
	RequiredSignatures    *bool                `yaml:"required_signatures,omitempty"`
	NonFastForward        *bool                `yaml:"non_fast_forward,omitempty"`
	PullRequest           *RulesetPullRequest  `yaml:"pull_request,omitempty"`
	RequiredStatusChecks  *RulesetStatusChecks `yaml:"required_status_checks,omitempty"`
	CodeScanning          *RulesetCodeScanning `yaml:"code_scanning,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetPullRequest holds the pull request rule and its parameters.
type RulesetPullRequest struct {
	Enabled    *bool                         `yaml:"enabled,omitempty"`
	Parameters *RulesetPullRequestParameters `yaml:"parameters,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetPullRequestParameters holds the managed parameters of the pull
// request rule.
type RulesetPullRequestParameters struct {
	RequiredApprovingReviewCount               *int      `yaml:"required_approving_review_count,omitempty"`
	DismissStaleReviewsOnPush                  *bool     `yaml:"dismiss_stale_reviews_on_push,omitempty"`
	RequireCodeOwnerReview                     *bool     `yaml:"require_code_owner_review,omitempty"`
	RequireLastPushApproval                    *bool     `yaml:"require_last_push_approval,omitempty"`
	RequiredReviewThreadResolution             *bool     `yaml:"required_review_thread_resolution,omitempty"`
	RequireExtraApprovalForUnattributedChanges *bool     `yaml:"require_extra_approval_for_unattributed_changes,omitempty"`
	AllowedMergeMethods                        *[]string `yaml:"allowed_merge_methods,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetStatusChecks holds the required status checks rule and its
// parameters.
type RulesetStatusChecks struct {
	Enabled    *bool                          `yaml:"enabled,omitempty"`
	Parameters *RulesetStatusChecksParameters `yaml:"parameters,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetStatusChecksParameters holds the managed parameters of the required
// status checks rule.
type RulesetStatusChecksParameters struct {
	StrictRequiredStatusChecksPolicy *bool                 `yaml:"strict_required_status_checks_policy,omitempty"`
	DoNotEnforceOnCreate             *bool                 `yaml:"do_not_enforce_on_create,omitempty"`
	RequiredStatusChecks             *[]RulesetStatusCheck `yaml:"required_status_checks,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetStatusCheck names one required status check. IntegrationID is
// optional and restricts the check to one GitHub App.
type RulesetStatusCheck struct {
	Context       string `yaml:"context"`
	IntegrationID *int   `yaml:"integration_id,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetCodeScanning holds the code scanning rule and its parameters.
type RulesetCodeScanning struct {
	Enabled    *bool                          `yaml:"enabled,omitempty"`
	Parameters *RulesetCodeScanningParameters `yaml:"parameters,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetCodeScanningParameters holds the managed parameters of the code
// scanning rule.
type RulesetCodeScanningParameters struct {
	CodeScanningTools *[]RulesetCodeScanningTool `yaml:"code_scanning_tools,omitempty"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// RulesetCodeScanningTool names one code scanning tool and the alert
// thresholds that block a merge.
type RulesetCodeScanningTool struct {
	Tool                    string `yaml:"tool"`
	AlertsThreshold         string `yaml:"alerts_threshold"`
	SecurityAlertsThreshold string `yaml:"security_alerts_threshold"`

	// Extra captures any YAML keys not mapped to struct fields above.
	Extra map[string]any `yaml:",inline"`
}

// Sorted yaml key names for each ruleset level. Validation reports them
// when it rejects an unrecognised key.
var (
	RulesetSettingNames               = yamlKeys[RulesetSettings]()
	RulesetBypassActorNames           = yamlKeys[RulesetBypassActor]()
	RulesetConditionsNames            = yamlKeys[RulesetConditions]()
	RulesetRefNameNames               = yamlKeys[RulesetRefName]()
	RulesetRulesNames                 = yamlKeys[RulesetRules]()
	RulesetRuleNames                  = yamlKeys[RulesetPullRequest]()
	RulesetPullRequestParameterNames  = yamlKeys[RulesetPullRequestParameters]()
	RulesetStatusChecksParameterNames = yamlKeys[RulesetStatusChecksParameters]()
	RulesetStatusCheckNames           = yamlKeys[RulesetStatusCheck]()
	RulesetCodeScanningParameterNames = yamlKeys[RulesetCodeScanningParameters]()
	RulesetCodeScanningToolNames      = yamlKeys[RulesetCodeScanningTool]()
)

// yamlKeys returns the sorted yaml tag names of T, excluding the inline
// Extra field.
func yamlKeys[T any]() []string {
	fields := settingFields[T](nil)
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.YAMLKey)
	}
	slices.Sort(names)
	return names
}
