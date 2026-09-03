package config

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

var (
	topicRegexp    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	labelHexRegexp = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
)

// maxLabels caps the label list so a misconfigured file cannot drive an
// unbounded number of API mutations.
const maxLabels = 1000

func validateTopLevelSettings(cfg *Config) error {
	valid := []string{"actions", "code_quality", "code_scanning", "labels", "license", "repository", "ruleset", "swatches"}
	return rejectExtra("top-level", cfg.Extra, valid)
}

// ValidatePaths checks that every swatch path in cfg matches a known embedded
// swatch. Returns an error listing the unrecognised path and all valid paths.
func ValidatePaths(cfg *Config) error {
	valid := swatch.Paths()
	for _, s := range cfg.Swatches {
		if !slices.Contains(valid, s.Path) {
			return fmt.Errorf("unrecognised swatch path %q in config; valid paths: %s",
				s.Path, strings.Join(valid, ", "))
		}
	}
	return nil
}

// ValidateDuplicatePaths checks that no two swatches share a path. Returns an
// error identifying the duplicate.
func ValidateDuplicatePaths(cfg *Config) error {
	seen := make(map[string]bool, len(cfg.Swatches))
	for _, s := range cfg.Swatches {
		if seen[s.Path] {
			return fmt.Errorf("duplicate swatch path %q in config", s.Path)
		}
		seen[s.Path] = true
	}
	return nil
}

// ValidateRepoSettings checks that every field name in cfg.Repository
// matches the supported settings list. Returns an error identifying the
// unrecognised field and listing all valid field names.
func ValidateRepoSettings(cfg *Config) error {
	if cfg.Repository == nil {
		return nil
	}

	return rejectExtra("repository", cfg.Repository.Extra, settingNames(model.RepositorySettingFields(nil)))
}

// ValidateWorkflowPermissions checks that default_workflow_permissions, if set,
// is either "read" or "write".
func ValidateWorkflowPermissions(cfg *Config) error {
	if cfg.Repository == nil || cfg.Repository.DefaultWorkflowPermissions == nil {
		return nil
	}
	return validateEnum("default_workflow_permissions", cfg.Repository.DefaultWorkflowPermissions, "read", "write")
}

// ValidateSecretScanning checks that secret_scanning,
// secret_scanning_push_protection, and secret_scanning_non_provider_patterns,
// if set, are either "enabled" or "disabled".
func ValidateSecretScanning(cfg *Config) error {
	if cfg.Repository == nil {
		return nil
	}
	for _, setting := range []struct {
		name  string
		value *string
	}{
		{"secret_scanning", cfg.Repository.SecretScanning},
		{"secret_scanning_push_protection", cfg.Repository.SecretScanningPushProtection},
		{"secret_scanning_non_provider_patterns", cfg.Repository.SecretScanningNonProviderPatterns},
	} {
		if err := validateEnum(setting.name, setting.value, "enabled", "disabled"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateCodeScanning checks the code scanning field names, enum values, and
// language list.
func ValidateCodeScanning(cfg *Config) error {
	if cfg.CodeScanning == nil {
		return nil
	}
	if err := rejectExtra("code_scanning", cfg.CodeScanning.Extra, settingNames(model.CodeScanningSettingFields(nil))); err != nil {
		return err
	}

	c := cfg.CodeScanning
	if err := validateEnum("code_scanning.state", c.State, "configured", "not-configured"); err != nil {
		return err
	}
	if err := validateEnum("code_scanning.query_suite", c.QuerySuite, "default", "extended"); err != nil {
		return err
	}
	if err := validateEnum("code_scanning.threat_model", c.ThreatModel, "remote", "remote_and_local"); err != nil {
		return err
	}
	return validateLanguages("code_scanning", c.Languages, model.CodeScanningLanguages)
}

// ValidateCodeQuality checks the Code Quality field names, enum values, and
// language list.
func ValidateCodeQuality(cfg *Config) error {
	if cfg.CodeQuality == nil {
		return nil
	}
	if err := rejectExtra("code_quality", cfg.CodeQuality.Extra, settingNames(model.CodeQualitySettingFields(nil))); err != nil {
		return err
	}

	c := cfg.CodeQuality
	if err := validateEnum("code_quality.state", c.State, "configured", "not-configured"); err != nil {
		return err
	}
	return validateLanguages("code_quality", c.Languages, model.CodeQualityLanguages)
}

// validateEnum checks that value, if set, is one of the allowed values. The
// error lists the allowed values as `"a" or "b"` for two values and as
// `"a", "b", or "c"` for three or more.
func validateEnum(name string, value *string, allowed ...string) error {
	if value == nil || slices.Contains(allowed, *value) {
		return nil
	}
	quoted := make([]string, len(allowed))
	for i, v := range allowed {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	separator := " or "
	if len(quoted) > 2 {
		separator = ", or "
	}
	list := strings.Join(quoted[:len(quoted)-1], ", ") + separator + quoted[len(quoted)-1]
	return fmt.Errorf("invalid %s %q; must be %s", name, *value, list)
}

// validateLanguages checks that every language, if set, is in the valid list
// and appears only once.
func validateLanguages(section string, languages *[]string, valid []string) error {
	if languages == nil {
		return nil
	}
	return validateMembers(section+".languages", "language", *languages, valid)
}

// validateMembers checks that every value in list is in the valid list and
// appears only once. The noun names one value in the error messages.
func validateMembers(list, noun string, values, valid []string) error {
	seen := make(map[string]bool, len(values))
	for i, value := range values {
		if !slices.Contains(valid, value) {
			return fmt.Errorf("%s[%d]: unrecognised %s %q; valid %ss: %s",
				list, i, noun, value, noun, strings.Join(valid, ", "))
		}
		if seen[value] {
			return fmt.Errorf("%s[%d]: duplicate %s %q", list, i, noun, value)
		}
		seen[value] = true
	}
	return nil
}

// ValidateActions checks the Actions policy field names, enum values, and
// selected-action field combinations, and rejects patterns_allowed entries
// containing control characters.
func ValidateActions(cfg *Config) error {
	if cfg.Actions == nil {
		return nil
	}
	if err := rejectExtra("actions", cfg.Actions.Extra, settingNames(model.ActionsSettingFields(nil))); err != nil {
		return err
	}

	a := cfg.Actions
	if err := validateEnum("allowed_actions", a.AllowedActions, "all", "local_only", "selected"); err != nil {
		return err
	}
	selectedFieldsSet := a.GitHubOwnedAllowed != nil || a.VerifiedAllowed != nil || a.PatternsAllowed != nil
	if selectedFieldsSet && (a.AllowedActions == nil || *a.AllowedActions != "selected") {
		return fmt.Errorf("github_owned_allowed, verified_allowed, and patterns_allowed require allowed_actions to be %q", "selected")
	}
	if a.PatternsAllowed != nil {
		for i, pattern := range *a.PatternsAllowed {
			if containsControl(pattern) {
				return fmt.Errorf("patterns_allowed[%d]: pattern %q contains control characters", i, pattern)
			}
		}
	}
	return nil
}

func unrecognisedSettingError(section string, extra map[string]any, valid []string) error {
	keys := slices.Sorted(maps.Keys(extra))
	return fmt.Errorf("unrecognised %s setting %q in config; valid settings: %s",
		section, keys[0], strings.Join(valid, ", "))
}

// ValidateCompleteActions checks that a selected Actions policy includes the
// complete selected-actions endpoint payload after default merging.
func ValidateCompleteActions(cfg *Config) error {
	if cfg.Actions == nil || cfg.Actions.AllowedActions == nil || *cfg.Actions.AllowedActions != "selected" {
		return nil
	}

	a := cfg.Actions
	if a.GitHubOwnedAllowed == nil || a.VerifiedAllowed == nil || a.PatternsAllowed == nil {
		return fmt.Errorf("allowed_actions %q requires github_owned_allowed, verified_allowed, and patterns_allowed", "selected")
	}
	return nil
}

// ValidateTopics checks that every topic, if set, starts with a lowercase
// letter or number, contains only lowercase alphanumerics and hyphens, does
// not exceed 50 characters, and appears only once.
func ValidateTopics(cfg *Config) error {
	if cfg.Repository == nil || cfg.Repository.Topics == nil {
		return nil
	}
	seen := make(map[string]bool, len(*cfg.Repository.Topics))
	for _, topic := range *cfg.Repository.Topics {
		if len(topic) > 50 {
			return fmt.Errorf("topic %q exceeds 50 characters", topic)
		}
		if !topicRegexp.MatchString(topic) {
			return fmt.Errorf("topic %q is invalid; must start with a lowercase letter or number and contain only lowercase alphanumerics and hyphens", topic)
		}
		if seen[topic] {
			return fmt.Errorf("duplicate topic %q in config", topic)
		}
		seen[topic] = true
	}
	return nil
}

// containsControl reports whether s contains any control character, including
// C0 controls, DEL, and C1 controls.
func containsControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// ValidateRepoStringSettings checks repository string enum values and rejects
// control characters, which could inject terminal control sequences into
// output. Topics carry their own stricter format validation in ValidateTopics.
func ValidateRepoStringSettings(cfg *Config) error {
	if cfg.Repository == nil {
		return nil
	}

	enumSettings := []struct {
		name    string
		value   *string
		allowed []string
	}{
		{"squash_merge_commit_title", cfg.Repository.SquashMergeCommitTitle, []string{"PR_TITLE", "COMMIT_OR_PR_TITLE"}},
		{"squash_merge_commit_message", cfg.Repository.SquashMergeCommitMessage, []string{"PR_BODY", "COMMIT_MESSAGES", "BLANK"}},
		{"merge_commit_title", cfg.Repository.MergeCommitTitle, []string{"PR_TITLE", "MERGE_MESSAGE"}},
		{"merge_commit_message", cfg.Repository.MergeCommitMessage, []string{"PR_TITLE", "PR_BODY", "BLANK"}},
	}
	for _, setting := range enumSettings {
		if err := validateEnum(setting.name, setting.value, setting.allowed...); err != nil {
			return err
		}
	}

	for _, field := range model.RepositorySettingFields(cfg.Repository) {
		if !field.Set || field.Value.Elem().Kind() != reflect.String {
			continue
		}
		if containsControl(field.Value.Elem().String()) {
			return fmt.Errorf("repository setting %s contains control characters", field.YAMLKey)
		}
	}
	return nil
}

// ValidateLabels checks that every label entry has valid name, color, and
// description fields. Rejects control characters in names and descriptions
// and duplicate names (case-insensitive).
func ValidateLabels(cfg *Config) error {
	if cfg.Labels == nil {
		return nil
	}
	if len(cfg.Labels) > maxLabels {
		return fmt.Errorf("labels: %d entries exceed the maximum of %d", len(cfg.Labels), maxLabels)
	}
	seen := make(map[string]bool, len(cfg.Labels))
	for i, l := range cfg.Labels {
		if l.Name == "" {
			return fmt.Errorf("label[%d]: name must not be empty", i)
		}
		if utf8.RuneCountInString(l.Name) > 50 {
			return fmt.Errorf("label[%d]: name %q exceeds 50 characters", i, l.Name)
		}
		if containsControl(l.Name) {
			return fmt.Errorf("label[%d]: name %q contains control characters", i, l.Name)
		}
		if l.Color == "" {
			return fmt.Errorf("label[%d]: color must not be empty", i)
		}
		if !labelHexRegexp.MatchString(l.Color) {
			return fmt.Errorf("label[%d]: color %q is not a valid 6-character hex colour (no # prefix)", i, l.Color)
		}
		if l.Description == "" {
			return fmt.Errorf("label[%d]: description must not be empty", i)
		}
		if utf8.RuneCountInString(l.Description) > 100 {
			return fmt.Errorf("label[%d]: description exceeds 100 characters", i)
		}
		if containsControl(l.Description) {
			return fmt.Errorf("label[%d]: description contains control characters", i)
		}
		key := strings.ToLower(l.Name)
		if seen[key] {
			return fmt.Errorf("label[%d]: duplicate label name %q (case-insensitive)", i, l.Name)
		}
		seen[key] = true
	}
	return nil
}

// settingNames returns the sorted yaml tag names for fields.
func settingNames(fields []model.SettingField) []string {
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		names = append(names, field.YAMLKey)
	}
	slices.Sort(names)
	return names
}

// rulesetEnterpriseEnforcement is the enforcement level that GitHub offers
// only on GitHub Enterprise plans.
const rulesetEnterpriseEnforcement = "evaluate"

// maxRulesetReviewCount is the largest required_approving_review_count that
// the rulesets API accepts.
const maxRulesetReviewCount = 10

// ValidateRuleset checks the ruleset field names, enum values, bypass
// actors, branch conditions, and rule parameters. Fields that default
// merging fills are checked only when set; ValidateCompleteRuleset checks
// for their presence after merging.
func ValidateRuleset(cfg *Config) error {
	r := cfg.Ruleset
	if r == nil {
		return nil
	}
	if err := rejectExtra("ruleset", r.Extra, model.RulesetSettingNames); err != nil {
		return err
	}
	if err := validateRulesetEnforcement(r.Enforcement); err != nil {
		return err
	}
	if err := validateBypassActors(r.BypassActors); err != nil {
		return err
	}
	if err := validateRulesetConditions(r.Conditions); err != nil {
		return err
	}
	return validateRulesetRules(r.Rules)
}

func validateRulesetEnforcement(enforcement *string) error {
	if enforcement != nil && *enforcement == rulesetEnterpriseEnforcement {
		return fmt.Errorf("invalid ruleset.enforcement %q; evaluate is available only on GitHub Enterprise", *enforcement)
	}
	return validateEnum("ruleset.enforcement", enforcement, model.RulesetEnforcements...)
}

// validateBypassActors checks every bypass actor and rejects duplicate
// actors, which share an actor_type and actor_id.
func validateBypassActors(actors *[]model.RulesetBypassActor) error {
	if actors == nil {
		return nil
	}
	seen := make(map[string]bool, len(*actors))
	for i, actor := range *actors {
		name := fmt.Sprintf("ruleset.bypass_actors[%d]", i)
		if err := rejectExtra(name, actor.Extra, model.RulesetBypassActorNames); err != nil {
			return err
		}
		if actor.ActorType == nil {
			return fmt.Errorf("%s: actor_type must not be empty", name)
		}
		if err := validateEnum(name+".actor_type", actor.ActorType, model.RulesetActorTypes...); err != nil {
			return err
		}
		if actor.BypassMode == nil {
			return fmt.Errorf("%s: bypass_mode must not be empty", name)
		}
		if err := validateEnum(name+".bypass_mode", actor.BypassMode, model.RulesetBypassModes...); err != nil {
			return err
		}
		key := *actor.ActorType
		if *actor.ActorType == "DeployKey" {
			if actor.ActorID != nil {
				return fmt.Errorf("%s: actor_id must be absent for a DeployKey actor", name)
			}
			if *actor.BypassMode == "pull_request" {
				return fmt.Errorf("%s: bypass_mode %q is not valid for a DeployKey actor", name, *actor.BypassMode)
			}
		} else {
			if actor.ActorID == nil {
				return fmt.Errorf("%s: actor_id is required for a %s actor", name, *actor.ActorType)
			}
			key = fmt.Sprintf("%s:%d", *actor.ActorType, *actor.ActorID)
		}
		if seen[key] {
			return fmt.Errorf("%s: duplicate bypass actor %s", name, key)
		}
		seen[key] = true
	}
	return nil
}

func validateRulesetConditions(conditions *model.RulesetConditions) error {
	if conditions == nil {
		return nil
	}
	if err := rejectExtra("ruleset.conditions", conditions.Extra, model.RulesetConditionsNames); err != nil {
		return err
	}
	refName := conditions.RefName
	if refName == nil {
		return nil
	}
	if err := rejectExtra("ruleset.conditions.ref_name", refName.Extra, model.RulesetRefNameNames); err != nil {
		return err
	}
	if refName.Include != nil && len(*refName.Include) == 0 {
		return fmt.Errorf("ruleset.conditions.ref_name.include must contain at least one entry")
	}
	if err := validateRefPatterns("ruleset.conditions.ref_name.include", refName.Include); err != nil {
		return err
	}
	if err := validateRefPatterns("ruleset.conditions.ref_name.exclude", refName.Exclude); err != nil {
		return err
	}
	return rejectIncludeTokens(refName.Exclude)
}

// rulesetIncludeTokens are the special values that GitHub accepts in the
// include list only.
var rulesetIncludeTokens = []string{"~DEFAULT_BRANCH", "~ALL"}

// rejectIncludeTokens rejects the include-only special values in the
// exclude list, where GitHub treats them as a plain pattern that matches
// no branch.
func rejectIncludeTokens(exclude *[]string) error {
	if exclude == nil {
		return nil
	}
	for i, pattern := range *exclude {
		if slices.Contains(rulesetIncludeTokens, pattern) {
			return fmt.Errorf("ruleset.conditions.ref_name.exclude[%d]: %s is valid in include only", i, pattern)
		}
	}
	return nil
}

// validateRefPatterns rejects empty, control-bearing, and duplicate
// patterns in a branch condition list.
func validateRefPatterns(name string, patterns *[]string) error {
	if patterns == nil {
		return nil
	}
	seen := make(map[string]bool, len(*patterns))
	for i, pattern := range *patterns {
		if pattern == "" {
			return fmt.Errorf("%s[%d]: pattern must not be empty", name, i)
		}
		if containsControl(pattern) {
			return fmt.Errorf("%s[%d]: pattern %q contains control characters", name, i, pattern)
		}
		if seen[pattern] {
			return fmt.Errorf("%s[%d]: duplicate pattern %q", name, i, pattern)
		}
		seen[pattern] = true
	}
	return nil
}

func validateRulesetRules(rules *model.RulesetRules) error {
	if rules == nil {
		return nil
	}
	if err := rejectExtra("ruleset.rules", rules.Extra, model.RulesetRulesNames); err != nil {
		return err
	}
	if err := validateRulesetPullRequest(rules.PullRequest); err != nil {
		return err
	}
	if err := validateLinearHistoryMergeMethods(rules); err != nil {
		return err
	}
	if err := validateRulesetStatusChecks(rules.RequiredStatusChecks); err != nil {
		return err
	}
	return validateRulesetCodeScanning(rules.CodeScanning)
}

// validateLinearHistoryMergeMethods rejects required_linear_history with an
// enabled pull request rule that allows merge commits only. GitHub requires
// squash or rebase merging when history must be linear, so that ruleset
// could never merge a pull request.
func validateLinearHistoryMergeMethods(rules *model.RulesetRules) error {
	pr := rules.PullRequest
	if rules.RequiredLinearHistory == nil || !*rules.RequiredLinearHistory ||
		pr == nil || pr.Enabled == nil || !*pr.Enabled || pr.Parameters == nil || pr.Parameters.AllowedMergeMethods == nil {
		return nil
	}
	methods := *pr.Parameters.AllowedMergeMethods
	if slices.Contains(methods, "squash") || slices.Contains(methods, "rebase") {
		return nil
	}
	return fmt.Errorf("ruleset.rules.required_linear_history requires pull_request.parameters.allowed_merge_methods to include squash or rebase")
}

func validateRulesetPullRequest(rule *model.RulesetPullRequest) error {
	if rule == nil {
		return nil
	}
	const name = "ruleset.rules.pull_request"
	if err := rejectExtra(name, rule.Extra, model.RulesetRuleNames); err != nil {
		return err
	}
	p := rule.Parameters
	if p == nil {
		return nil
	}
	if err := rejectExtra(name+".parameters", p.Extra, model.RulesetPullRequestParameterNames); err != nil {
		return err
	}
	if p.RequiredApprovingReviewCount != nil && (*p.RequiredApprovingReviewCount < 0 || *p.RequiredApprovingReviewCount > maxRulesetReviewCount) {
		return fmt.Errorf("%s.parameters.required_approving_review_count must be between 0 and %d, got %d", name, maxRulesetReviewCount, *p.RequiredApprovingReviewCount)
	}
	if p.AllowedMergeMethods == nil {
		return nil
	}
	if len(*p.AllowedMergeMethods) == 0 {
		return fmt.Errorf("%s.parameters.allowed_merge_methods must contain at least one method", name)
	}
	return validateMembers(name+".parameters.allowed_merge_methods", "method", *p.AllowedMergeMethods, model.RulesetMergeMethods)
}

func validateRulesetStatusChecks(rule *model.RulesetStatusChecks) error {
	if rule == nil {
		return nil
	}
	const name = "ruleset.rules.required_status_checks"
	if err := rejectExtra(name, rule.Extra, model.RulesetRuleNames); err != nil {
		return err
	}
	p := rule.Parameters
	if p == nil {
		return nil
	}
	if err := rejectExtra(name+".parameters", p.Extra, model.RulesetStatusChecksParameterNames); err != nil {
		return err
	}
	if p.RequiredStatusChecks == nil {
		return nil
	}
	seen := make(map[string]bool, len(*p.RequiredStatusChecks))
	for i, check := range *p.RequiredStatusChecks {
		entry := fmt.Sprintf("%s.parameters.required_status_checks[%d]", name, i)
		if check.IntegrationID != nil && *check.IntegrationID <= 0 {
			return fmt.Errorf("%s: integration_id must be positive, got %d", entry, *check.IntegrationID)
		}
		if err := validateListEntry(entry, check.Extra, model.RulesetStatusCheckNames, "context", check.Context, seen); err != nil {
			return err
		}
	}
	return nil
}

// validateListEntry checks one entry of a keyed list: no unrecognised
// setting, and a key value that is not empty, carries no control
// characters, and appears once. It records the value in seen.
func validateListEntry(entry string, extra map[string]any, valid []string, field, value string, seen map[string]bool) error {
	if err := rejectExtra(entry, extra, valid); err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("%s: %s must not be empty", entry, field)
	}
	if containsControl(value) {
		return fmt.Errorf("%s: %s %q contains control characters", entry, field, value)
	}
	if seen[value] {
		return fmt.Errorf("%s: duplicate %s %q", entry, field, value)
	}
	seen[value] = true
	return nil
}

func validateRulesetCodeScanning(rule *model.RulesetCodeScanning) error {
	if rule == nil {
		return nil
	}
	const name = "ruleset.rules.code_scanning"
	if err := rejectExtra(name, rule.Extra, model.RulesetRuleNames); err != nil {
		return err
	}
	p := rule.Parameters
	if p == nil {
		return nil
	}
	if err := rejectExtra(name+".parameters", p.Extra, model.RulesetCodeScanningParameterNames); err != nil {
		return err
	}
	if p.CodeScanningTools == nil {
		return nil
	}
	seen := make(map[string]bool, len(*p.CodeScanningTools))
	for i, tool := range *p.CodeScanningTools {
		entry := fmt.Sprintf("%s.parameters.code_scanning_tools[%d]", name, i)
		if err := validateEnum(entry+".alerts_threshold", &tool.AlertsThreshold, model.RulesetAlertsThresholds...); err != nil {
			return err
		}
		if err := validateEnum(entry+".security_alerts_threshold", &tool.SecurityAlertsThreshold, model.RulesetSecurityAlertsThresholds...); err != nil {
			return err
		}
		if err := validateListEntry(entry, tool.Extra, model.RulesetCodeScanningToolNames, "tool", tool.Tool, seen); err != nil {
			return err
		}
	}
	return nil
}

// ValidateCompleteRuleset checks that the ruleset carries every field the
// rulesets API requires after default merging: the enforcement level, the
// bypass actor list, both branch condition lists, the six Boolean rule
// keys, the pull request rule with its seven parameters, the required
// status checks rule with its two policy flags, and the code scanning rule
// with its tool list. A write sends the whole ruleset, so an absent list or
// Boolean would clear the live value without a report line.
func ValidateCompleteRuleset(cfg *Config) error {
	r := cfg.Ruleset
	if r == nil {
		return nil
	}
	if r.Enforcement == nil {
		return fmt.Errorf("ruleset requires enforcement")
	}
	if r.BypassActors == nil {
		return fmt.Errorf("ruleset requires bypass_actors")
	}
	if r.Conditions == nil || r.Conditions.RefName == nil || r.Conditions.RefName.Include == nil {
		return fmt.Errorf("ruleset requires conditions.ref_name.include")
	}
	if r.Conditions.RefName.Exclude == nil {
		return fmt.Errorf("ruleset requires conditions.ref_name.exclude")
	}
	if r.Rules == nil {
		return fmt.Errorf("ruleset requires rules")
	}
	if err := validateCompleteBooleanRules(r.Rules); err != nil {
		return err
	}
	if err := validateCompletePullRequest(r.Rules.PullRequest); err != nil {
		return err
	}
	if err := validateCompleteStatusChecks(r.Rules.RequiredStatusChecks); err != nil {
		return err
	}
	return validateCompleteCodeScanning(r.Rules.CodeScanning)
}

// validateCompleteBooleanRules requires the six Boolean rule keys. A write
// sends a rule only when its key is true, so an absent key would remove the
// live rule without a report line.
func validateCompleteBooleanRules(rules *model.RulesetRules) error {
	for _, rule := range []struct {
		key   string
		value *bool
	}{
		{"creation", rules.Creation},
		{"update", rules.Update},
		{"deletion", rules.Deletion},
		{"required_linear_history", rules.RequiredLinearHistory},
		{"required_signatures", rules.RequiredSignatures},
		{"non_fast_forward", rules.NonFastForward},
	} {
		if rule.value == nil {
			return fmt.Errorf("ruleset.rules requires %s", rule.key)
		}
	}
	return nil
}

func validateCompletePullRequest(rule *model.RulesetPullRequest) error {
	const name = "ruleset.rules.pull_request"
	if rule == nil || rule.Enabled == nil {
		return fmt.Errorf("%s requires enabled", name)
	}
	if !*rule.Enabled {
		return nil
	}
	p := rule.Parameters
	if p == nil || p.RequiredApprovingReviewCount == nil || p.DismissStaleReviewsOnPush == nil ||
		p.RequireCodeOwnerReview == nil || p.RequireLastPushApproval == nil ||
		p.RequiredReviewThreadResolution == nil || p.RequireExtraApprovalForUnattributedChanges == nil ||
		p.AllowedMergeMethods == nil {
		return fmt.Errorf("%s requires every parameter when enabled: %s", name, strings.Join(model.RulesetPullRequestParameterNames, ", "))
	}
	return nil
}

func validateCompleteStatusChecks(rule *model.RulesetStatusChecks) error {
	const name = "ruleset.rules.required_status_checks"
	if rule == nil || rule.Enabled == nil {
		return fmt.Errorf("%s requires enabled", name)
	}
	if !*rule.Enabled {
		return nil
	}
	p := rule.Parameters
	if p == nil || p.StrictRequiredStatusChecksPolicy == nil || p.DoNotEnforceOnCreate == nil || p.RequiredStatusChecks == nil {
		return fmt.Errorf("%s requires every parameter when enabled: %s", name, strings.Join(model.RulesetStatusChecksParameterNames, ", "))
	}
	return nil
}

// validateCompleteCodeScanning requires the tool list when the rule is
// enabled. GitHub rejects a code scanning rule with no tools, so an empty
// list would fail the write.
func validateCompleteCodeScanning(rule *model.RulesetCodeScanning) error {
	const name = "ruleset.rules.code_scanning"
	if rule == nil || rule.Enabled == nil {
		return fmt.Errorf("%s requires enabled", name)
	}
	if !*rule.Enabled {
		return nil
	}
	p := rule.Parameters
	if p == nil || p.CodeScanningTools == nil || len(*p.CodeScanningTools) == 0 {
		return fmt.Errorf("%s requires at least one entry in parameters.code_scanning_tools when enabled", name)
	}
	return nil
}

// RulesetMergeMethodWarnings reports every merge method that the ruleset
// allows but the repository settings in the same config disable. Neither
// value is changed; the warning tells the user that the two disagree.
func RulesetMergeMethodWarnings(cfg *Config) []string {
	if cfg.Ruleset == nil || cfg.Repository == nil || cfg.Ruleset.Rules == nil ||
		cfg.Ruleset.Rules.PullRequest == nil || cfg.Ruleset.Rules.PullRequest.Parameters == nil ||
		cfg.Ruleset.Rules.PullRequest.Parameters.AllowedMergeMethods == nil {
		return nil
	}
	repositoryFields := map[string]struct {
		field string
		value *bool
	}{
		"merge":  {"allow_merge_commit", cfg.Repository.AllowMergeCommit},
		"squash": {"allow_squash_merge", cfg.Repository.AllowSquashMerge},
		"rebase": {"allow_rebase_merge", cfg.Repository.AllowRebaseMerge},
	}
	var warnings []string
	for _, method := range *cfg.Ruleset.Rules.PullRequest.Parameters.AllowedMergeMethods {
		setting, ok := repositoryFields[method]
		if !ok || setting.value == nil || *setting.value {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("warning: ruleset allows %s merging but repository.%s is false", method, setting.field))
	}
	return warnings
}

// rejectExtra returns the unrecognised setting error when extra holds keys.
func rejectExtra(section string, extra map[string]any, valid []string) error {
	if len(extra) == 0 {
		return nil
	}
	return unrecognisedSettingError(section, extra, valid)
}
