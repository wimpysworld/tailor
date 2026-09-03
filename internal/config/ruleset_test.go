package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func defaultRuleset(t *testing.T) *model.RulesetSettings {
	t.Helper()
	cfg, err := DefaultConfig("none")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}
	if cfg.Ruleset == nil {
		t.Fatal("default ruleset section is nil")
	}
	return cfg.Ruleset
}

func actor(actorType string, id *int, mode string) model.RulesetBypassActor {
	return model.RulesetBypassActor{ActorID: id, ActorType: new(actorType), BypassMode: new(mode)}
}

func refName(include, exclude *[]string) *model.RulesetConditions {
	return &model.RulesetConditions{RefName: &model.RulesetRefName{Include: include, Exclude: exclude}}
}

func pullRequestRule(p *model.RulesetPullRequestParameters) *model.RulesetRules {
	return &model.RulesetRules{PullRequest: &model.RulesetPullRequest{Enabled: new(true), Parameters: p}}
}

// linearHistoryRule pairs required_linear_history with an enabled pull
// request rule that allows methods.
func linearHistoryRule(linear bool, methods *[]string) *model.RulesetRules {
	rules := pullRequestRule(&model.RulesetPullRequestParameters{AllowedMergeMethods: methods})
	rules.RequiredLinearHistory = new(linear)
	return rules
}

func statusChecksRule(p *model.RulesetStatusChecksParameters) *model.RulesetRules {
	return &model.RulesetRules{RequiredStatusChecks: &model.RulesetStatusChecks{Enabled: new(true), Parameters: p}}
}

func codeScanningRule(tools ...model.RulesetCodeScanningTool) *model.RulesetRules {
	return &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Enabled: new(true), Parameters: &model.RulesetCodeScanningParameters{CodeScanningTools: &tools}}}
}

func codeQLTool() model.RulesetCodeScanningTool {
	return model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "high_or_higher"}
}

func TestValidateRuleset(t *testing.T) {
	tests := []struct {
		name    string
		section *model.RulesetSettings
		wantErr string
	}{
		{name: "absent"},
		{name: "empty", section: &model.RulesetSettings{}},
		{name: "defaults", section: defaultRuleset(t)},
		{name: "evaluate is enterprise only", section: &model.RulesetSettings{Enforcement: new("evaluate")}, wantErr: `invalid ruleset.enforcement "evaluate"; evaluate is available only on GitHub Enterprise`},
		{name: "invalid enforcement", section: &model.RulesetSettings{Enforcement: new("on")}, wantErr: `invalid ruleset.enforcement "on"; must be "active" or "disabled"`},
		{name: "unknown key", section: &model.RulesetSettings{Extra: map[string]any{"target": "branch"}}, wantErr: `unrecognised ruleset setting "target" in config; valid settings: bypass_actors, conditions, enforcement, rules`},
		{name: "actor without type", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{{ActorID: new(5), BypassMode: new("always")}}}, wantErr: `ruleset.bypass_actors[0]: actor_type must not be empty`},
		{name: "actor invalid type", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("OrganizationAdmin", new(1), "always")}}, wantErr: `invalid ruleset.bypass_actors[0].actor_type "OrganizationAdmin"; must be "RepositoryRole" or "Team" or "User" or "Integration" or "DeployKey"`},
		{name: "actor without mode", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{{ActorID: new(5), ActorType: new("RepositoryRole")}}}, wantErr: `ruleset.bypass_actors[0]: bypass_mode must not be empty`},
		{name: "actor invalid mode", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("Team", new(7), "never")}}, wantErr: `invalid ruleset.bypass_actors[0].bypass_mode "never"; must be "always" or "pull_request" or "exempt"`},
		{name: "actor without id", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("Team", nil, "always")}}, wantErr: `ruleset.bypass_actors[0]: actor_id is required for a Team actor`},
		{name: "deploy key with id", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("DeployKey", new(1), "always")}}, wantErr: `ruleset.bypass_actors[0]: actor_id must be absent for a DeployKey actor`},
		{name: "deploy key pull request mode", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("DeployKey", nil, "pull_request")}}, wantErr: `ruleset.bypass_actors[0]: bypass_mode "pull_request" is not valid for a DeployKey actor`},
		{name: "deploy key exempt", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("DeployKey", nil, "exempt")}}},
		{name: "duplicate actors", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("RepositoryRole", new(5), "always"), actor("RepositoryRole", new(5), "exempt")}}, wantErr: `ruleset.bypass_actors[1]: duplicate bypass actor RepositoryRole:5`},
		{name: "duplicate deploy keys", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("DeployKey", nil, "always"), actor("DeployKey", nil, "always")}}, wantErr: `ruleset.bypass_actors[1]: duplicate bypass actor DeployKey`},
		{name: "same id different type", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{actor("RepositoryRole", new(5), "always"), actor("Team", new(5), "always")}}},
		{name: "actor unknown key", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{{Extra: map[string]any{"role": "admin"}}}}, wantErr: `unrecognised ruleset.bypass_actors[0] setting "role" in config; valid settings: actor_id, actor_type, bypass_mode`},
		{name: "conditions unknown key", section: &model.RulesetSettings{Conditions: &model.RulesetConditions{Extra: map[string]any{"repository_name": nil}}}, wantErr: `unrecognised ruleset.conditions setting "repository_name" in config; valid settings: ref_name`},
		{name: "ref_name unknown key", section: &model.RulesetSettings{Conditions: &model.RulesetConditions{RefName: &model.RulesetRefName{Extra: map[string]any{"branches": nil}}}}, wantErr: `unrecognised ruleset.conditions.ref_name setting "branches" in config; valid settings: exclude, include`},
		{name: "include empty list", section: &model.RulesetSettings{Conditions: refName(&[]string{}, nil)}, wantErr: `ruleset.conditions.ref_name.include must contain at least one entry`},
		{name: "include empty string", section: &model.RulesetSettings{Conditions: refName(&[]string{""}, nil)}, wantErr: `ruleset.conditions.ref_name.include[0]: pattern must not be empty`},
		{name: "include duplicate", section: &model.RulesetSettings{Conditions: refName(&[]string{"~ALL", "~ALL"}, nil)}, wantErr: `ruleset.conditions.ref_name.include[1]: duplicate pattern "~ALL"`},
		{name: "include control characters", section: &model.RulesetSettings{Conditions: refName(&[]string{"refs/heads/\x1b[31mmain"}, nil)}, wantErr: `ruleset.conditions.ref_name.include[0]: pattern "refs/heads/\x1b[31mmain" contains control characters`},
		{name: "exclude empty list", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{})}},
		{name: "exclude empty string", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{""})}, wantErr: `ruleset.conditions.ref_name.exclude[0]: pattern must not be empty`},
		{name: "exclude duplicate", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{"refs/heads/wip", "refs/heads/wip"})}, wantErr: `ruleset.conditions.ref_name.exclude[1]: duplicate pattern "refs/heads/wip"`},
		{name: "exclude default branch token", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{"refs/heads/wip", "~DEFAULT_BRANCH"})}, wantErr: `ruleset.conditions.ref_name.exclude[1]: ~DEFAULT_BRANCH is valid in include only`},
		{name: "exclude all token", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{"~ALL"})}, wantErr: `ruleset.conditions.ref_name.exclude[0]: ~ALL is valid in include only`},
		{name: "exclude pattern", section: &model.RulesetSettings{Conditions: refName(nil, &[]string{"refs/heads/wip/*"})}},
		{name: "rules unknown key", section: &model.RulesetSettings{Rules: &model.RulesetRules{Extra: map[string]any{"workflows": nil}}}, wantErr: `unrecognised ruleset.rules setting "workflows" in config; valid settings: code_scanning, creation, deletion, non_fast_forward, pull_request, required_linear_history, required_signatures, required_status_checks, update`},
		{name: "pull request unknown key", section: &model.RulesetSettings{Rules: &model.RulesetRules{PullRequest: &model.RulesetPullRequest{Extra: map[string]any{"type": "pull_request"}}}}, wantErr: `unrecognised ruleset.rules.pull_request setting "type" in config; valid settings: enabled, parameters`},
		{name: "pull request parameters unknown key", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{Extra: map[string]any{"required_reviewers": nil}})}, wantErr: `unrecognised ruleset.rules.pull_request.parameters setting "required_reviewers" in config; valid settings: allowed_merge_methods, dismiss_stale_reviews_on_push, require_code_owner_review, require_extra_approval_for_unattributed_changes, require_last_push_approval, required_approving_review_count, required_review_thread_resolution`},
		{name: "review count negative", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{RequiredApprovingReviewCount: new(-1)})}, wantErr: `ruleset.rules.pull_request.parameters.required_approving_review_count must be between 0 and 10, got -1`},
		{name: "review count too large", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{RequiredApprovingReviewCount: new(11)})}, wantErr: `ruleset.rules.pull_request.parameters.required_approving_review_count must be between 0 and 10, got 11`},
		{name: "review count bounds", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{RequiredApprovingReviewCount: new(10)})}},
		{name: "merge methods empty", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{AllowedMergeMethods: &[]string{}})}, wantErr: `ruleset.rules.pull_request.parameters.allowed_merge_methods must contain at least one method`},
		{name: "merge method unknown", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{AllowedMergeMethods: &[]string{"fast-forward"}})}, wantErr: `ruleset.rules.pull_request.parameters.allowed_merge_methods[0]: unrecognised method "fast-forward"; valid methods: merge, squash, rebase`},
		{name: "merge method duplicate", section: &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{AllowedMergeMethods: &[]string{"merge", "squash", "merge"}})}, wantErr: `ruleset.rules.pull_request.parameters.allowed_merge_methods[2]: duplicate method "merge"`},
		{name: "linear history with merge only", section: &model.RulesetSettings{Rules: linearHistoryRule(true, &[]string{"merge"})}, wantErr: `ruleset.rules.required_linear_history requires pull_request.parameters.allowed_merge_methods to include squash or rebase`},
		{name: "linear history with squash", section: &model.RulesetSettings{Rules: linearHistoryRule(true, &[]string{"merge", "squash"})}},
		{name: "linear history without methods", section: &model.RulesetSettings{Rules: linearHistoryRule(true, nil)}},
		{name: "merge only without linear history", section: &model.RulesetSettings{Rules: linearHistoryRule(false, &[]string{"merge"})}},
		{name: "linear history with pull request disabled", section: &model.RulesetSettings{Rules: &model.RulesetRules{RequiredLinearHistory: new(true), PullRequest: &model.RulesetPullRequest{Enabled: new(false), Parameters: &model.RulesetPullRequestParameters{AllowedMergeMethods: &[]string{"merge"}}}}}},
		{name: "status checks unknown key", section: &model.RulesetSettings{Rules: &model.RulesetRules{RequiredStatusChecks: &model.RulesetStatusChecks{Extra: map[string]any{"checks": nil}}}}, wantErr: `unrecognised ruleset.rules.required_status_checks setting "checks" in config; valid settings: enabled, parameters`},
		{name: "status checks parameters unknown key", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{Extra: map[string]any{"contexts": nil}})}, wantErr: `unrecognised ruleset.rules.required_status_checks.parameters setting "contexts" in config; valid settings: do_not_enforce_on_create, required_status_checks, strict_required_status_checks_policy`},
		{name: "status check unknown key", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "lint", Extra: map[string]any{"app_id": 1}}}})}, wantErr: `unrecognised ruleset.rules.required_status_checks.parameters.required_status_checks[0] setting "app_id" in config; valid settings: context, integration_id`},
		{name: "status check empty context", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: ""}}})}, wantErr: `ruleset.rules.required_status_checks.parameters.required_status_checks[0]: context must not be empty`},
		{name: "status check integration id zero", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "lint", IntegrationID: new(0)}}})}, wantErr: `ruleset.rules.required_status_checks.parameters.required_status_checks[0]: integration_id must be positive, got 0`},
		{name: "status check duplicate context", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "lint"}, {Context: "lint", IntegrationID: new(15368)}}})}, wantErr: `ruleset.rules.required_status_checks.parameters.required_status_checks[1]: duplicate context "lint"`},
		{name: "status checks valid", section: &model.RulesetSettings{Rules: statusChecksRule(&model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}}})}},
		{name: "code scanning unknown key", section: &model.RulesetSettings{Rules: &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Extra: map[string]any{"tools": nil}}}}, wantErr: `unrecognised ruleset.rules.code_scanning setting "tools" in config; valid settings: enabled, parameters`},
		{name: "code scanning parameters unknown key", section: &model.RulesetSettings{Rules: &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Enabled: new(true), Parameters: &model.RulesetCodeScanningParameters{Extra: map[string]any{"tools": nil}}}}}, wantErr: `unrecognised ruleset.rules.code_scanning.parameters setting "tools" in config; valid settings: code_scanning_tools`},
		{name: "code scanning tool unknown key", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "all", Extra: map[string]any{"threshold": "all"}})}, wantErr: `unrecognised ruleset.rules.code_scanning.parameters.code_scanning_tools[0] setting "threshold" in config; valid settings: alerts_threshold, security_alerts_threshold, tool`},
		{name: "code scanning empty tool", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{AlertsThreshold: "errors", SecurityAlertsThreshold: "all"})}, wantErr: `ruleset.rules.code_scanning.parameters.code_scanning_tools[0]: tool must not be empty`},
		{name: "code scanning tool control characters", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{Tool: "Code\x1bQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "all"})}, wantErr: `ruleset.rules.code_scanning.parameters.code_scanning_tools[0]: tool "Code\x1bQL" contains control characters`},
		{name: "code scanning invalid alerts threshold", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "warnings", SecurityAlertsThreshold: "all"})}, wantErr: `invalid ruleset.rules.code_scanning.parameters.code_scanning_tools[0].alerts_threshold "warnings"; must be "none" or "errors" or "errors_and_warnings" or "all"`},
		{name: "code scanning invalid security alerts threshold", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "high"})}, wantErr: `invalid ruleset.rules.code_scanning.parameters.code_scanning_tools[0].security_alerts_threshold "high"; must be "none" or "critical" or "high_or_higher" or "medium_or_higher" or "all"`},
		{name: "code scanning empty threshold", section: &model.RulesetSettings{Rules: codeScanningRule(model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "errors"})}, wantErr: `invalid ruleset.rules.code_scanning.parameters.code_scanning_tools[0].security_alerts_threshold ""`},
		{name: "code scanning duplicate tool", section: &model.RulesetSettings{Rules: codeScanningRule(codeQLTool(), model.RulesetCodeScanningTool{Tool: "CodeQL", AlertsThreshold: "all", SecurityAlertsThreshold: "all"})}, wantErr: `ruleset.rules.code_scanning.parameters.code_scanning_tools[1]: duplicate tool "CodeQL"`},
		{name: "code scanning tool names are case-sensitive", section: &model.RulesetSettings{Rules: codeScanningRule(codeQLTool(), model.RulesetCodeScanningTool{Tool: "codeql", AlertsThreshold: "all", SecurityAlertsThreshold: "all"})}},
		{name: "code scanning empty list", section: &model.RulesetSettings{Rules: codeScanningRule()}},
		{name: "code scanning valid", section: &model.RulesetSettings{Rules: codeScanningRule(codeQLTool(), model.RulesetCodeScanningTool{Tool: "Sentinel 👁️", AlertsThreshold: "none", SecurityAlertsThreshold: "medium_or_higher"})}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRuleset(&Config{Ruleset: tt.section})
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestValidateCompleteRuleset(t *testing.T) {
	// booleanRules returns the six Boolean rule keys, all false.
	booleanRules := func() *model.RulesetRules {
		return &model.RulesetRules{
			Creation: new(false), Update: new(false), Deletion: new(false),
			RequiredLinearHistory: new(false), RequiredSignatures: new(false), NonFastForward: new(false),
		}
	}
	disabledRules := func() *model.RulesetRules {
		rules := booleanRules()
		rules.PullRequest = &model.RulesetPullRequest{Enabled: new(false)}
		rules.RequiredStatusChecks = &model.RulesetStatusChecks{Enabled: new(false)}
		rules.CodeScanning = &model.RulesetCodeScanning{Enabled: new(false)}
		return rules
	}
	withPullRequest := func(rules *model.RulesetRules) *model.RulesetRules {
		complete := booleanRules()
		complete.PullRequest = rules.PullRequest
		complete.RequiredStatusChecks = rules.RequiredStatusChecks
		complete.CodeScanning = rules.CodeScanning
		return complete
	}
	// withCodeScanning pairs disabled pull request and status checks rules
	// with the given code scanning rule.
	withCodeScanning := func(rule *model.RulesetCodeScanning) *model.RulesetRules {
		rules := disabledRules()
		rules.CodeScanning = rule
		return rules
	}
	// complete wraps rules with the other required fields present.
	complete := func(rules *model.RulesetRules) *model.RulesetSettings {
		return &model.RulesetSettings{
			Enforcement:  new("active"),
			BypassActors: &[]model.RulesetBypassActor{},
			Conditions:   refName(&[]string{"~ALL"}, &[]string{}),
			Rules:        rules,
		}
	}
	tests := []struct {
		name    string
		section *model.RulesetSettings
		wantErr string
	}{
		{name: "absent"},
		{name: "defaults", section: defaultRuleset(t)},
		{name: "disabled rules need no parameters", section: complete(disabledRules())},
		{name: "missing enforcement", section: &model.RulesetSettings{BypassActors: &[]model.RulesetBypassActor{}, Conditions: refName(&[]string{"~ALL"}, &[]string{}), Rules: disabledRules()}, wantErr: "ruleset requires enforcement"},
		{name: "missing bypass actors", section: &model.RulesetSettings{Enforcement: new("active"), Conditions: refName(&[]string{"~ALL"}, &[]string{}), Rules: disabledRules()}, wantErr: "ruleset requires bypass_actors"},
		{name: "missing conditions", section: &model.RulesetSettings{Enforcement: new("active"), BypassActors: &[]model.RulesetBypassActor{}, Rules: disabledRules()}, wantErr: "ruleset requires conditions.ref_name.include"},
		{name: "missing include", section: &model.RulesetSettings{Enforcement: new("active"), BypassActors: &[]model.RulesetBypassActor{}, Conditions: refName(nil, &[]string{}), Rules: disabledRules()}, wantErr: "ruleset requires conditions.ref_name.include"},
		{name: "missing exclude", section: &model.RulesetSettings{Enforcement: new("active"), BypassActors: &[]model.RulesetBypassActor{}, Conditions: refName(&[]string{"~ALL"}, nil), Rules: disabledRules()}, wantErr: "ruleset requires conditions.ref_name.exclude"},
		{name: "missing rules", section: complete(nil), wantErr: "ruleset requires rules"},
		{name: "missing boolean rule", section: complete(&model.RulesetRules{}), wantErr: "ruleset.rules requires creation"},
		{name: "missing one boolean rule", section: complete(&model.RulesetRules{Creation: new(false), Update: new(false), Deletion: new(true), RequiredLinearHistory: new(false), RequiredSignatures: new(false)}), wantErr: "ruleset.rules requires non_fast_forward"},
		{name: "missing pull request", section: complete(booleanRules()), wantErr: "ruleset.rules.pull_request requires enabled"},
		{name: "pull request enabled without parameters", section: complete(withPullRequest(pullRequestRule(nil))), wantErr: "ruleset.rules.pull_request requires every parameter when enabled: allowed_merge_methods, dismiss_stale_reviews_on_push"},
		{name: "pull request missing one parameter", section: complete(withPullRequest(pullRequestRule(&model.RulesetPullRequestParameters{
			RequiredApprovingReviewCount: new(1), DismissStaleReviewsOnPush: new(true), RequireCodeOwnerReview: new(false),
			RequireLastPushApproval: new(false), RequiredReviewThreadResolution: new(true), AllowedMergeMethods: &[]string{"squash"},
		}))), wantErr: "ruleset.rules.pull_request requires every parameter when enabled"},
		{name: "missing status checks", section: complete(withPullRequest(&model.RulesetRules{PullRequest: &model.RulesetPullRequest{Enabled: new(false)}})), wantErr: "ruleset.rules.required_status_checks requires enabled"},
		{name: "status checks enabled without parameters", section: complete(withPullRequest(&model.RulesetRules{
			PullRequest:          &model.RulesetPullRequest{Enabled: new(false)},
			RequiredStatusChecks: &model.RulesetStatusChecks{Enabled: new(true), Parameters: &model.RulesetStatusChecksParameters{StrictRequiredStatusChecksPolicy: new(false)}},
		})), wantErr: "ruleset.rules.required_status_checks requires every parameter when enabled: do_not_enforce_on_create, required_status_checks, strict_required_status_checks_policy"},
		{name: "missing code scanning", section: complete(withCodeScanning(nil)), wantErr: "ruleset.rules.code_scanning requires enabled"},
		{name: "code scanning without enabled", section: complete(withCodeScanning(&model.RulesetCodeScanning{Parameters: &model.RulesetCodeScanningParameters{CodeScanningTools: &[]model.RulesetCodeScanningTool{codeQLTool()}}})), wantErr: "ruleset.rules.code_scanning requires enabled"},
		{name: "code scanning enabled without parameters", section: complete(withCodeScanning(&model.RulesetCodeScanning{Enabled: new(true)})), wantErr: "ruleset.rules.code_scanning requires at least one entry in parameters.code_scanning_tools when enabled"},
		{name: "code scanning enabled with empty tools", section: complete(withCodeScanning(codeScanningRule().CodeScanning)), wantErr: "ruleset.rules.code_scanning requires at least one entry in parameters.code_scanning_tools when enabled"},
		{name: "code scanning enabled with tools", section: complete(withCodeScanning(codeScanningRule(codeQLTool()).CodeScanning))},
		{name: "code scanning disabled with empty tools", section: complete(withCodeScanning(&model.RulesetCodeScanning{Enabled: new(false), Parameters: &model.RulesetCodeScanningParameters{CodeScanningTools: &[]model.RulesetCodeScanningTool{}}}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompleteRuleset(&Config{Ruleset: tt.section})
			assertErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestRulesetMergeMethodWarnings(t *testing.T) {
	tests := []struct {
		name       string
		repository *model.RepositorySettings
		methods    *[]string
		want       []string
	}{
		{name: "nil repository", methods: &[]string{"merge"}},
		{name: "nil methods", repository: &model.RepositorySettings{AllowMergeCommit: new(false)}},
		{name: "all allowed", repository: &model.RepositorySettings{AllowMergeCommit: new(true), AllowSquashMerge: new(true), AllowRebaseMerge: new(true)}, methods: &[]string{"merge", "squash", "rebase"}},
		{name: "unset repository field", repository: &model.RepositorySettings{}, methods: &[]string{"merge"}},
		{name: "disabled method not allowed by ruleset", repository: &model.RepositorySettings{AllowMergeCommit: new(false)}, methods: &[]string{"squash"}},
		{
			name:       "one conflict",
			repository: &model.RepositorySettings{AllowMergeCommit: new(false), AllowSquashMerge: new(false)},
			methods:    &[]string{"squash", "rebase"},
			want:       []string{"warning: ruleset allows squash merging but repository.allow_squash_merge is false"},
		},
		{
			name:       "conflicts in method order",
			repository: &model.RepositorySettings{AllowMergeCommit: new(false), AllowSquashMerge: new(true), AllowRebaseMerge: new(false)},
			methods:    &[]string{"rebase", "merge"},
			want: []string{
				"warning: ruleset allows rebase merging but repository.allow_rebase_merge is false",
				"warning: ruleset allows merge merging but repository.allow_merge_commit is false",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Repository: tt.repository}
			if tt.methods != nil {
				cfg.Ruleset = &model.RulesetSettings{Rules: pullRequestRule(&model.RulesetPullRequestParameters{AllowedMergeMethods: tt.methods})}
			}
			got := RulesetMergeMethodWarnings(cfg)
			if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("warnings = %q, want %q", got, tt.want)
			}
			if tt.repository != nil && tt.repository.AllowMergeCommit != nil && *tt.repository.AllowMergeCommit {
				return
			}
			if tt.methods != nil && !reflect.DeepEqual(*cfg.Ruleset.Rules.PullRequest.Parameters.AllowedMergeMethods, *tt.methods) {
				t.Error("warning changed allowed_merge_methods")
			}
		})
	}
}

func TestLoadRejectsInvalidRuleset(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{name: "enforcement", yaml: "license: none\nruleset:\n  enforcement: evaluate\nswatches: []\n", want: "evaluate is available only on GitHub Enterprise"},
		{name: "unknown nested key", yaml: "license: none\nruleset:\n  rules:\n    pull_request:\n      enabled: true\n      parameters:\n        required_reviewers: []\nswatches: []\n", want: `unrecognised ruleset.rules.pull_request.parameters setting "required_reviewers"`},
		{name: "deploy key null id", yaml: "license: none\nruleset:\n  bypass_actors:\n    - actor_id: null\n      actor_type: DeployKey\n      bypass_mode: always\nswatches: []\n"},
		{name: "review count string", yaml: "license: none\nruleset:\n  rules:\n    pull_request:\n      enabled: true\n      parameters:\n        required_approving_review_count: two\nswatches: []\n", want: "parsing config"},
		{name: "code scanning threshold", yaml: "license: none\nruleset:\n  rules:\n    code_scanning:\n      enabled: true\n      parameters:\n        code_scanning_tools:\n          - tool: CodeQL\n            alerts_threshold: errors\n            security_alerts_threshold: severe\nswatches: []\n", want: `security_alerts_threshold "severe"`},
		{name: "code scanning tool unknown key", yaml: "license: none\nruleset:\n  rules:\n    code_scanning:\n      enabled: true\n      parameters:\n        code_scanning_tools:\n          - tool: CodeQL\n            alerts_threshold: errors\n            security_alerts_threshold: all\n            threshold: all\nswatches: []\n", want: `unrecognised ruleset.rules.code_scanning.parameters.code_scanning_tools[0] setting "threshold"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.WriteConfig(t, dir, tt.yaml)
			_, err := Load(dir)
			assertErrorContains(t, err, tt.want)
		})
	}
}

func TestDefaultConfigRuleset(t *testing.T) {
	r := defaultRuleset(t)
	testutil.AssertPtr(t, r.Enforcement, false, "active", "enforcement")
	if r.BypassActors == nil || len(*r.BypassActors) != 1 {
		t.Fatalf("bypass_actors = %v, want one actor", r.BypassActors)
	}
	a := (*r.BypassActors)[0]
	testutil.AssertPtr(t, a.ActorID, false, 5, "actor_id")
	testutil.AssertPtr(t, a.ActorType, false, "RepositoryRole", "actor_type")
	testutil.AssertPtr(t, a.BypassMode, false, "always", "bypass_mode")
	if r.Conditions == nil || r.Conditions.RefName == nil || r.Conditions.RefName.Include == nil || !reflect.DeepEqual(*r.Conditions.RefName.Include, []string{"~DEFAULT_BRANCH"}) {
		t.Errorf("include = %v, want [~DEFAULT_BRANCH]", r.Conditions)
	}
	if r.Conditions.RefName.Exclude == nil || len(*r.Conditions.RefName.Exclude) != 0 {
		t.Errorf("exclude = %v, want empty list", r.Conditions.RefName.Exclude)
	}
	testutil.AssertPtr(t, r.Rules.Deletion, false, true, "rules.deletion")
	testutil.AssertPtr(t, r.Rules.NonFastForward, false, true, "rules.non_fast_forward")
	testutil.AssertPtr(t, r.Rules.Creation, false, false, "rules.creation")
	testutil.AssertPtr(t, r.Rules.PullRequest.Enabled, false, true, "rules.pull_request.enabled")
	testutil.AssertPtr(t, r.Rules.PullRequest.Parameters.RequiredApprovingReviewCount, false, 1, "required_approving_review_count")
	if !reflect.DeepEqual(*r.Rules.PullRequest.Parameters.AllowedMergeMethods, []string{"squash", "rebase"}) {
		t.Errorf("allowed_merge_methods = %v, want [squash rebase]", *r.Rules.PullRequest.Parameters.AllowedMergeMethods)
	}
	testutil.AssertPtr(t, r.Rules.RequiredStatusChecks.Enabled, false, false, "rules.required_status_checks.enabled")
	if r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks == nil || len(*r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks) != 0 {
		t.Errorf("required_status_checks = %v, want empty list", r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks)
	}
	testutil.AssertPtr(t, r.Rules.CodeScanning.Enabled, false, false, "rules.code_scanning.enabled")
	if !reflect.DeepEqual(*r.Rules.CodeScanning.Parameters.CodeScanningTools, []model.RulesetCodeScanningTool{codeQLTool()}) {
		t.Errorf("code_scanning_tools = %v, want [CodeQL errors high_or_higher]", *r.Rules.CodeScanning.Parameters.CodeScanningTools)
	}
	if err := ValidateCompleteRuleset(&Config{Ruleset: r}); err != nil {
		t.Errorf("default ruleset is incomplete: %v", err)
	}
}

func TestMergeDefaultsAddsRuleset(t *testing.T) {
	cfg := &Config{License: "none"}
	changed, err := MergeDefaults(cfg)
	if err != nil {
		t.Fatalf("MergeDefaults() error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for an empty config")
	}
	if !reflect.DeepEqual(cfg.Ruleset, defaultRuleset(t)) {
		t.Errorf("merged ruleset = %#v, want the default section", cfg.Ruleset)
	}
	changed, err = MergeDefaults(cfg)
	if err != nil {
		t.Fatalf("second MergeDefaults() error: %v", err)
	}
	if changed {
		t.Fatal("second MergeDefaults() changed a complete config")
	}
}

func TestMergeDefaultsFillsRulesetFields(t *testing.T) {
	cfg := &Config{
		License: "none",
		Ruleset: &model.RulesetSettings{
			Enforcement:  new("disabled"),
			BypassActors: &[]model.RulesetBypassActor{},
			Conditions:   refName(&[]string{"refs/heads/main", "refs/heads/release/*"}, nil),
			Rules: &model.RulesetRules{
				Deletion:    new(false),
				PullRequest: &model.RulesetPullRequest{Enabled: new(false)},
				RequiredStatusChecks: &model.RulesetStatusChecks{
					Enabled:    new(true),
					Parameters: &model.RulesetStatusChecksParameters{RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "lint"}}},
				},
			},
		},
	}
	if _, err := MergeDefaults(cfg); err != nil {
		t.Fatalf("MergeDefaults() error: %v", err)
	}
	r := cfg.Ruleset
	testutil.AssertPtr(t, r.Enforcement, false, "disabled", "enforcement")
	if len(*r.BypassActors) != 0 {
		t.Errorf("bypass_actors = %v, want the explicit empty list", *r.BypassActors)
	}
	if !reflect.DeepEqual(*r.Conditions.RefName.Include, []string{"refs/heads/main", "refs/heads/release/*"}) {
		t.Errorf("include = %v, want the explicit list", *r.Conditions.RefName.Include)
	}
	if r.Conditions.RefName.Exclude == nil || len(*r.Conditions.RefName.Exclude) != 0 {
		t.Errorf("exclude = %v, want the default empty list", r.Conditions.RefName.Exclude)
	}
	testutil.AssertPtr(t, r.Rules.Deletion, false, false, "rules.deletion")
	testutil.AssertPtr(t, r.Rules.NonFastForward, false, true, "rules.non_fast_forward")
	testutil.AssertPtr(t, r.Rules.PullRequest.Enabled, false, false, "rules.pull_request.enabled")
	testutil.AssertPtr(t, r.Rules.PullRequest.Parameters.RequiredApprovingReviewCount, false, 1, "required_approving_review_count")
	testutil.AssertPtr(t, r.Rules.RequiredStatusChecks.Enabled, false, true, "rules.required_status_checks.enabled")
	testutil.AssertPtr(t, r.Rules.RequiredStatusChecks.Parameters.StrictRequiredStatusChecksPolicy, false, false, "strict_required_status_checks_policy")
	if !reflect.DeepEqual(*r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks, []model.RulesetStatusCheck{{Context: "lint"}}) {
		t.Errorf("required_status_checks = %v, want the explicit list", *r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks)
	}
	testutil.AssertPtr(t, r.Rules.CodeScanning.Enabled, false, false, "rules.code_scanning.enabled")
	if !reflect.DeepEqual(*r.Rules.CodeScanning.Parameters.CodeScanningTools, []model.RulesetCodeScanningTool{codeQLTool()}) {
		t.Errorf("code_scanning_tools = %v, want the default list", *r.Rules.CodeScanning.Parameters.CodeScanningTools)
	}
	if err := ValidateCompleteRuleset(cfg); err != nil {
		t.Errorf("merged ruleset is incomplete: %v", err)
	}

	changed, err := MergeDefaults(cfg)
	if err != nil {
		t.Fatalf("second MergeDefaults() error: %v", err)
	}
	if changed {
		t.Fatal("second MergeDefaults() changed a complete config")
	}
}

func TestMergeRulesetSetup(t *testing.T) {
	cfg := &Config{Ruleset: defaultRuleset(t)}
	live := &model.RulesetSettings{
		Enforcement:  new("disabled"),
		BypassActors: &[]model.RulesetBypassActor{},
		Conditions:   refName(&[]string{"refs/heads/main"}, &[]string{"refs/heads/wip/*"}),
		Rules: &model.RulesetRules{
			Creation:    new(true),
			Deletion:    new(false),
			PullRequest: &model.RulesetPullRequest{Enabled: new(false)},
			RequiredStatusChecks: &model.RulesetStatusChecks{
				Enabled: new(true),
				Parameters: &model.RulesetStatusChecksParameters{
					StrictRequiredStatusChecksPolicy: new(true),
					DoNotEnforceOnCreate:             new(false),
					RequiredStatusChecks:             &[]model.RulesetStatusCheck{{Context: "lint", IntegrationID: new(15368)}},
				},
			},
			CodeScanning: &model.RulesetCodeScanning{
				Enabled: new(true),
				Parameters: &model.RulesetCodeScanningParameters{
					CodeScanningTools: &[]model.RulesetCodeScanningTool{{Tool: "CodeQL", AlertsThreshold: "all", SecurityAlertsThreshold: "critical"}},
				},
			},
		},
	}
	MergeRulesetSetup(cfg, live)
	r := cfg.Ruleset
	testutil.AssertPtr(t, r.Enforcement, false, "disabled", "enforcement")
	if len(*r.BypassActors) != 0 {
		t.Errorf("bypass_actors = %v, want the live empty list", *r.BypassActors)
	}
	if !reflect.DeepEqual(*r.Conditions.RefName.Exclude, []string{"refs/heads/wip/*"}) {
		t.Errorf("exclude = %v, want the live list", *r.Conditions.RefName.Exclude)
	}
	testutil.AssertPtr(t, r.Rules.Creation, false, true, "rules.creation")
	testutil.AssertPtr(t, r.Rules.Deletion, false, false, "rules.deletion")
	testutil.AssertPtr(t, r.Rules.NonFastForward, false, true, "rules.non_fast_forward")
	testutil.AssertPtr(t, r.Rules.PullRequest.Enabled, false, false, "rules.pull_request.enabled")
	// The live ruleset carries no pull request rule, so the built-in
	// parameters stay in the config for the day the rule is enabled.
	testutil.AssertPtr(t, r.Rules.PullRequest.Parameters.RequiredApprovingReviewCount, false, 1, "required_approving_review_count")
	testutil.AssertPtr(t, r.Rules.RequiredStatusChecks.Enabled, false, true, "rules.required_status_checks.enabled")
	testutil.AssertPtr(t, r.Rules.RequiredStatusChecks.Parameters.StrictRequiredStatusChecksPolicy, false, true, "strict_required_status_checks_policy")
	if !reflect.DeepEqual(*r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks, []model.RulesetStatusCheck{{Context: "lint", IntegrationID: new(15368)}}) {
		t.Errorf("required_status_checks = %v, want the live list", *r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks)
	}
	testutil.AssertPtr(t, r.Rules.CodeScanning.Enabled, false, true, "rules.code_scanning.enabled")
	wantTools := []model.RulesetCodeScanningTool{{Tool: "CodeQL", AlertsThreshold: "all", SecurityAlertsThreshold: "critical"}}
	if !reflect.DeepEqual(*r.Rules.CodeScanning.Parameters.CodeScanningTools, wantTools) {
		t.Errorf("code_scanning_tools = %v, want the live list", *r.Rules.CodeScanning.Parameters.CodeScanningTools)
	}

	MergeRulesetSetup(&Config{}, live)
}

func TestMergeRulesetSetupKeepsBuiltInCodeScanningTools(t *testing.T) {
	// The live ruleset carries no code scanning rule, so the built-in tool
	// list stays in the config for the day the rule is enabled.
	cfg := &Config{Ruleset: defaultRuleset(t)}
	MergeRulesetSetup(cfg, &model.RulesetSettings{Rules: &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Enabled: new(false)}}})
	testutil.AssertPtr(t, cfg.Ruleset.Rules.CodeScanning.Enabled, false, false, "rules.code_scanning.enabled")
	if !reflect.DeepEqual(*cfg.Ruleset.Rules.CodeScanning.Parameters.CodeScanningTools, []model.RulesetCodeScanningTool{codeQLTool()}) {
		t.Errorf("code_scanning_tools = %v, want the built-in list", *cfg.Ruleset.Rules.CodeScanning.Parameters.CodeScanningTools)
	}
}

func TestMergeRulesetSetupSkipsUnmanagedEnforcement(t *testing.T) {
	cfg := &Config{Ruleset: defaultRuleset(t)}
	live := &model.RulesetSettings{Enforcement: new("evaluate"), Rules: &model.RulesetRules{Creation: new(true)}}
	if !MergeRulesetSetup(cfg, live) {
		t.Error("MergeRulesetSetup() = false, want true for an evaluate enforcement")
	}
	testutil.AssertPtr(t, cfg.Ruleset.Enforcement, false, "active", "enforcement")
	testutil.AssertPtr(t, cfg.Ruleset.Rules.Creation, false, true, "rules.creation")
	testutil.AssertPtr(t, live.Enforcement, false, "evaluate", "live enforcement")
	if err := ValidateRuleset(cfg); err != nil {
		t.Errorf("ValidateRuleset() error after merge: %v", err)
	}

	if MergeRulesetSetup(cfg, &model.RulesetSettings{Enforcement: new("disabled")}) {
		t.Error("MergeRulesetSetup() = true, want false for a disabled enforcement")
	}
	testutil.AssertPtr(t, cfg.Ruleset.Enforcement, false, "disabled", "enforcement")
}

func TestWriteRulesetSection(t *testing.T) {
	tests := []struct {
		name        string
		ruleset     *model.RulesetSettings
		want        []string
		wantMissing []string
	}{
		{
			name: "empty lists and a deploy key actor",
			ruleset: &model.RulesetSettings{
				Enforcement:  new("disabled"),
				BypassActors: &[]model.RulesetBypassActor{actor("DeployKey", nil, "always"), actor("Team", new(7), "pull_request")},
				Conditions:   refName(&[]string{"refs/heads/main", "release/*"}, &[]string{}),
				Rules: &model.RulesetRules{
					Deletion: new(true),
					RequiredStatusChecks: &model.RulesetStatusChecks{
						Enabled: new(true),
						Parameters: &model.RulesetStatusChecksParameters{
							StrictRequiredStatusChecksPolicy: new(true),
							DoNotEnforceOnCreate:             new(false),
							RequiredStatusChecks:             &[]model.RulesetStatusCheck{{Context: "Sentinel"}, {Context: "lint", IntegrationID: new(15368)}},
						},
					},
					CodeScanning: codeScanningRule(codeQLTool(), model.RulesetCodeScanningTool{Tool: "Sentinel", AlertsThreshold: "all", SecurityAlertsThreshold: "none"}).CodeScanning,
				},
			},
			want: []string{
				"\nruleset:\n" +
					"  # Tailor manages one ruleset named \"Tailor\" and owns it entirely.\n" +
					"  # active enforces the rules. disabled keeps the ruleset on GitHub but\n" +
					"  # GitHub ignores it, so a hand-made ruleset can govern instead.\n" +
					"  enforcement: disabled\n" +
					"  bypass_actors:\n" +
					"    # actor_type: RepositoryRole, Team, User, Integration, DeployKey\n" +
					"    # RepositoryRole actor_id: 2 maintain, 4 write, 5 admin\n" +
					"    # bypass_mode: always, pull_request, exempt\n" +
					"    - actor_type: DeployKey\n" +
					"      bypass_mode: always\n" +
					"    - actor_id: 7\n" +
					"      actor_type: Team\n" +
					"      bypass_mode: pull_request\n" +
					"  conditions:\n" +
					"    ref_name:\n" +
					"      # Branch names or fnmatch patterns in refs/heads/<name> form.\n" +
					"      # include also accepts ~DEFAULT_BRANCH and ~ALL.\n" +
					"      include:\n" +
					"        - refs/heads/main\n" +
					"        - \"release/*\"\n" +
					"      exclude: []\n" +
					"  rules:\n" +
					"    deletion: true\n" +
					"    required_status_checks:\n" +
					"      enabled: true\n" +
					"      parameters:\n" +
					"        # Require branches to be up to date before merging.\n" +
					"        strict_required_status_checks_policy: true\n" +
					"        # Do not require status checks on creation.\n" +
					"        do_not_enforce_on_create: false\n" +
					"        # context is the check name as shown on a pull request. For a GitHub\n" +
					"        # Actions job that is the job's name. integration_id is optional and\n" +
					"        # restricts the check to one app; 15368 is GitHub Actions.\n" +
					"        required_status_checks:\n" +
					"          - context: Sentinel\n" +
					"          - context: lint\n" +
					"            integration_id: 15368\n" +
					"    code_scanning:\n" +
					"      enabled: true\n" +
					"      parameters:\n" +
					"        # tool is the tool name as GitHub shows it, for example CodeQL.\n" +
					"        # alerts_threshold: none, errors, errors_and_warnings, all\n" +
					"        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all\n" +
					"        code_scanning_tools:\n" +
					"          - tool: CodeQL\n" +
					"            alerts_threshold: errors\n" +
					"            security_alerts_threshold: high_or_higher\n" +
					"          - tool: Sentinel\n" +
					"            alerts_threshold: all\n" +
					"            security_alerts_threshold: none\n" +
					"\nswatches:\n",
			},
			wantMissing: []string{"pull_request:", "creation:"},
		},
		{
			name:    "code scanning with an empty tool list",
			ruleset: &model.RulesetSettings{Rules: &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Enabled: new(false), Parameters: &model.RulesetCodeScanningParameters{CodeScanningTools: &[]model.RulesetCodeScanningTool{}}}}},
			want: []string{
				"  rules:\n" +
					"    code_scanning:\n" +
					"      enabled: false\n" +
					"      parameters:\n" +
					"        # tool is the tool name as GitHub shows it, for example CodeQL.\n" +
					"        # alerts_threshold: none, errors, errors_and_warnings, all\n" +
					"        # security_alerts_threshold: none, critical, high_or_higher, medium_or_higher, all\n" +
					"        code_scanning_tools: []\n" +
					"\nswatches:\n",
			},
		},
		{
			name:        "code scanning without parameters",
			ruleset:     &model.RulesetSettings{Rules: &model.RulesetRules{CodeScanning: &model.RulesetCodeScanning{Enabled: new(false)}}},
			want:        []string{"  rules:\n    code_scanning:\n      enabled: false\n\nswatches:\n"},
			wantMissing: []string{"parameters:", "code_scanning_tools"},
		},
		{
			name:    "no bypass actors keeps the guidance",
			ruleset: &model.RulesetSettings{Enforcement: new("active"), BypassActors: &[]model.RulesetBypassActor{}},
			want: []string{
				"  enforcement: active\n" +
					"  # actor_type: RepositoryRole, Team, User, Integration, DeployKey\n" +
					"  # RepositoryRole actor_id: 2 maintain, 4 write, 5 admin\n" +
					"  # bypass_mode: always, pull_request, exempt\n" +
					"  bypass_actors: []\n" +
					"\nswatches:\n",
			},
			wantMissing: []string{"conditions:", "rules:"},
		},
		{
			name:        "absent section",
			wantMissing: []string{"ruleset:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := Write(dir, &Config{License: "none", Ruleset: tt.ruleset}, "2026-03-02", "Refitted"); err != nil {
				t.Fatalf("Write() error: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".tailor.yml"))
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			for _, want := range tt.want {
				if !strings.Contains(content, want) {
					t.Errorf("config missing %q:\n%s", want, content)
				}
			}
			for _, missing := range tt.wantMissing {
				if strings.Contains(content, missing) {
					t.Errorf("config contains %q:\n%s", missing, content)
				}
			}
		})
	}
}

func TestWriteRulesetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := defaultRuleset(t)
	(*want.BypassActors) = append(*want.BypassActors, actor("DeployKey", nil, "exempt"))
	*want.Conditions.RefName.Exclude = []string{"refs/heads/wip/*", "~release"}
	*want.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks = []model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}}
	*want.Rules.CodeScanning.Parameters.CodeScanningTools = []model.RulesetCodeScanningTool{codeQLTool(), {Tool: "Sentinel 👁️", AlertsThreshold: "all", SecurityAlertsThreshold: "none"}}
	if err := Write(dir, &Config{License: "none", Ruleset: want}, "2026-03-02", "Refitted"); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !reflect.DeepEqual(loaded.Ruleset, want) {
		t.Errorf("round-tripped ruleset = %#v, want %#v", loaded.Ruleset, want)
	}
}
