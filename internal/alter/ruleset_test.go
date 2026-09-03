package alter_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// liveTailorRulesetJSON is the live form of the built-in default ruleset,
// including parameters that Tailor does not manage.
const liveTailorRulesetJSON = `{"id":42,"name":"Tailor","target":"branch","source_type":"Repository",` +
	`"bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"}],` +
	`"conditions":{"ref_name":{"exclude":[],"include":["~DEFAULT_BRANCH"]}},"enforcement":"active",` +
	`"rules":[{"parameters":{"allowed_merge_methods":["rebase","squash"],"dismiss_stale_reviews_on_push":true,` +
	`"dismissal_restriction":{"allowed_actors":[],"enabled":false},"require_code_owner_review":false,` +
	`"require_extra_approval_for_unattributed_changes":true,"require_last_push_approval":false,"required_approving_review_count":1,` +
	`"required_review_thread_resolution":true,"required_reviewers":[]},"type":"pull_request"},{"type":"non_fast_forward"},{"type":"deletion"}]}`

func defaultRulesetStub() *testutil.RulesetStub {
	return testutil.NewRulesetStub(`[{"id":42,"name":"Tailor","target":"branch","enforcement":"active","source_type":"Repository"}]`, liveTailorRulesetJSON)
}

func defaultRulesetConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.DefaultConfig("none")
	if err != nil {
		t.Fatalf("DefaultConfig() error: %v", err)
	}
	return &config.Config{Ruleset: cfg.Ruleset}
}

// defaultRulesetNoChange lists the results for the built-in ruleset when
// GitHub already carries it.
var defaultRulesetNoChange = []string{
	"ruleset.enforcement no change active ",
	"ruleset.bypass_actors no change RepositoryRole 5 (always) ",
	"ruleset.conditions.ref_name.include no change ~DEFAULT_BRANCH ",
	"ruleset.conditions.ref_name.exclude no change (none) ",
	"ruleset.rules.creation no change false ",
	"ruleset.rules.update no change false ",
	"ruleset.rules.deletion no change true ",
	"ruleset.rules.required_linear_history no change false ",
	"ruleset.rules.required_signatures no change false ",
	"ruleset.rules.non_fast_forward no change true ",
	"ruleset.rules.pull_request no change enabled ",
	"ruleset.rules.pull_request.parameters.required_approving_review_count no change 1 ",
	"ruleset.rules.pull_request.parameters.dismiss_stale_reviews_on_push no change true ",
	"ruleset.rules.pull_request.parameters.require_code_owner_review no change false ",
	"ruleset.rules.pull_request.parameters.require_last_push_approval no change false ",
	"ruleset.rules.pull_request.parameters.required_review_thread_resolution no change true ",
	"ruleset.rules.pull_request.parameters.require_extra_approval_for_unattributed_changes no change true ",
	"ruleset.rules.pull_request.parameters.allowed_merge_methods no change squash, rebase ",
	"ruleset.rules.required_status_checks no change disabled ",
	"ruleset.rules.code_scanning no change disabled ",
}

const (
	codeScanningNoChange = "ruleset.rules.code_scanning no change disabled "
	// liveCodeScanningRuleJSON is the code scanning rule in the live form.
	liveCodeScanningRuleJSON = `{"type":"code_scanning","parameters":{"code_scanning_tools":[{"tool":"Sentinel 👁️","alerts_threshold":"all","security_alerts_threshold":"none"},` +
		`{"tool":"CodeQL","alerts_threshold":"errors","security_alerts_threshold":"high_or_higher"}]}}`
)

func codeScanningTools() *[]model.RulesetCodeScanningTool {
	return &[]model.RulesetCodeScanningTool{
		{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "high_or_higher"},
		{Tool: "Sentinel 👁️", AlertsThreshold: "all", SecurityAlertsThreshold: "none"},
	}
}

func wouldSetAll(results []string) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		out = append(out, strings.Replace(result, " no change ", " would set ", 1))
	}
	return out
}

func TestProcessRulesetAbsentMakesNoCalls(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"nil section":   {},
		"empty section": {Ruleset: &model.RulesetSettings{}},
	} {
		t.Run(name, func(t *testing.T) {
			results, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(nil, "acme", "widget", true))
			if err != nil || results != nil {
				t.Fatalf("ProcessRuleset() = %v, %v", results, err)
			}
		})
	}
}

func TestProcessRulesetNoRepoContext(t *testing.T) {
	results, err := alter.ProcessRuleset(defaultRulesetConfig(t), alter.Apply, repoTarget(nil, "", "", false))
	if err != nil || results != nil {
		t.Fatalf("ProcessRuleset() = %v, %v", results, err)
	}
}

func TestProcessRulesetCompare(t *testing.T) {
	tests := []struct {
		name     string
		readBody string
		declare  func(*model.RulesetSettings)
		want     []string
	}{
		{name: "defaults match live", readBody: liveTailorRulesetJSON, want: defaultRulesetNoChange},
		{
			name:     "enforcement and a rule differ",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.Enforcement = new("disabled")
				r.Rules.Creation = new(true)
				r.Rules.Deletion = new(false)
			},
			want: func() []string {
				want := append([]string(nil), defaultRulesetNoChange...)
				want[0] = "ruleset.enforcement would set disabled "
				want[4] = "ruleset.rules.creation would set true "
				want[6] = "ruleset.rules.deletion would set false "
				return want
			}(),
		},
		{
			name:     "lists compare as sets",
			readBody: strings.Replace(liveTailorRulesetJSON, `"include":["~DEFAULT_BRANCH"]`, `"include":["release/*","~DEFAULT_BRANCH"]`, 1),
			declare: func(r *model.RulesetSettings) {
				r.Conditions.RefName.Include = &[]string{"~DEFAULT_BRANCH", "release/*"}
				r.Rules.PullRequest.Parameters.AllowedMergeMethods = &[]string{"rebase", "squash"}
			},
			want: func() []string {
				want := append([]string(nil), defaultRulesetNoChange...)
				want[2] = "ruleset.conditions.ref_name.include no change ~DEFAULT_BRANCH, release/* "
				want[17] = "ruleset.rules.pull_request.parameters.allowed_merge_methods no change rebase, squash "
				return want
			}(),
		},
		{
			name:     "bypass actors differ",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.BypassActors = &[]model.RulesetBypassActor{
					{ActorID: new(5), ActorType: new("RepositoryRole"), BypassMode: new("always")},
					{ActorType: new("DeployKey"), BypassMode: new("exempt")},
				}
			},
			want: func() []string {
				want := append([]string(nil), defaultRulesetNoChange...)
				want[1] = "ruleset.bypass_actors would set RepositoryRole 5 (always), DeployKey (exempt) "
				return want
			}(),
		},
		{
			name:     "pull request parameters differ",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.Rules.PullRequest.Parameters.RequiredApprovingReviewCount = new(2)
				r.Rules.PullRequest.Parameters.RequireCodeOwnerReview = new(true)
				r.Rules.PullRequest.Parameters.AllowedMergeMethods = &[]string{"merge"}
			},
			want: func() []string {
				want := append([]string(nil), defaultRulesetNoChange...)
				want[11] = "ruleset.rules.pull_request.parameters.required_approving_review_count would set 2 "
				want[13] = "ruleset.rules.pull_request.parameters.require_code_owner_review would set true "
				want[17] = "ruleset.rules.pull_request.parameters.allowed_merge_methods would set merge "
				return want
			}(),
		},
		{
			name:     "disabled pull request compares no parameters",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.Rules.PullRequest.Enabled = new(false)
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:10]...),
				"ruleset.rules.pull_request would set disabled ",
				"ruleset.rules.required_status_checks no change disabled ",
				codeScanningNoChange,
			),
		},
		{
			name:     "status checks enabled against absent rule",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.Rules.RequiredStatusChecks.Enabled = new(true)
				r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks = &[]model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}}
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:18]...),
				"ruleset.rules.required_status_checks would set enabled ",
				"ruleset.rules.required_status_checks.parameters.strict_required_status_checks_policy would set false ",
				"ruleset.rules.required_status_checks.parameters.do_not_enforce_on_create would set false ",
				"ruleset.rules.required_status_checks.parameters.required_status_checks would set Sentinel 👁️, lint (15368) ",
				codeScanningNoChange,
			),
		},
		{
			name: "status checks match as a set",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`,
				`{"type":"deletion"},{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,`+
					`"required_status_checks":[{"context":"lint","integration_id":15368},{"context":"Sentinel 👁️"}]}}`, 1),
			declare: func(r *model.RulesetSettings) {
				r.Rules.RequiredStatusChecks.Enabled = new(true)
				r.Rules.RequiredStatusChecks.Parameters.StrictRequiredStatusChecksPolicy = new(true)
				r.Rules.RequiredStatusChecks.Parameters.RequiredStatusChecks = &[]model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}}
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:18]...),
				"ruleset.rules.required_status_checks no change enabled ",
				"ruleset.rules.required_status_checks.parameters.strict_required_status_checks_policy no change true ",
				"ruleset.rules.required_status_checks.parameters.do_not_enforce_on_create no change false ",
				"ruleset.rules.required_status_checks.parameters.required_status_checks no change Sentinel 👁️, lint (15368) ",
				codeScanningNoChange,
			),
		},
		{
			name:     "code scanning enabled against absent rule",
			readBody: liveTailorRulesetJSON,
			declare: func(r *model.RulesetSettings) {
				r.Rules.CodeScanning.Enabled = new(true)
				r.Rules.CodeScanning.Parameters.CodeScanningTools = codeScanningTools()
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning would set enabled ",
				"ruleset.rules.code_scanning.parameters.code_scanning_tools would set CodeQL (errors, high_or_higher), Sentinel 👁️ (all, none) ",
			),
		},
		{
			name:     "code scanning tools match as a set",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`, `{"type":"deletion"},`+liveCodeScanningRuleJSON, 1),
			declare: func(r *model.RulesetSettings) {
				r.Rules.CodeScanning.Enabled = new(true)
				r.Rules.CodeScanning.Parameters.CodeScanningTools = codeScanningTools()
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning no change enabled ",
				"ruleset.rules.code_scanning.parameters.code_scanning_tools no change CodeQL (errors, high_or_higher), Sentinel 👁️ (all, none) ",
			),
		},
		{
			name:     "code scanning threshold differs",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`, `{"type":"deletion"},`+liveCodeScanningRuleJSON, 1),
			declare: func(r *model.RulesetSettings) {
				r.Rules.CodeScanning.Enabled = new(true)
				r.Rules.CodeScanning.Parameters.CodeScanningTools = &[]model.RulesetCodeScanningTool{{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "critical"}}
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning no change enabled ",
				"ruleset.rules.code_scanning.parameters.code_scanning_tools would set CodeQL (errors, critical) ",
			),
		},
		{
			name:     "code scanning rule without parameters",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`, `{"type":"deletion"},{"type":"code_scanning"}`, 1),
			declare: func(r *model.RulesetSettings) {
				r.Rules.CodeScanning.Enabled = new(true)
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning no change enabled ",
				"ruleset.rules.code_scanning.parameters.code_scanning_tools would set CodeQL (errors, high_or_higher) ",
			),
		},
		{
			name:     "code scanning rule with malformed parameters",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`, `{"type":"deletion"},{"type":"code_scanning","parameters":{"code_scanning_tools":"CodeQL"}}`, 1),
			declare: func(r *model.RulesetSettings) {
				r.Rules.CodeScanning.Enabled = new(true)
			},
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning no change enabled ",
				"ruleset.rules.code_scanning.parameters.code_scanning_tools would set CodeQL (errors, high_or_higher) ",
			),
		},
		{
			name:     "disabled code scanning compares no tools",
			readBody: strings.Replace(liveTailorRulesetJSON, `{"type":"deletion"}`, `{"type":"deletion"},`+liveCodeScanningRuleJSON, 1),
			want: append(append([]string(nil), defaultRulesetNoChange[:19]...),
				"ruleset.rules.code_scanning would set disabled ",
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := defaultRulesetStub()
			stub.ReadBody = tt.readBody
			client := testutil.NewTestClient(t, stub.Server(t))
			cfg := defaultRulesetConfig(t)
			if tt.declare != nil {
				tt.declare(cfg.Ruleset)
			}
			results, err := alter.ProcessRuleset(cfg, alter.DryRun, repoTarget(client, "acme", "widget", true))
			if err != nil {
				t.Fatal(err)
			}
			assertResults(t, results, tt.want)
			if len(stub.Writes) != 0 {
				t.Fatalf("dry run made writes: %v", stub.Writes)
			}
		})
	}
}

func TestProcessRulesetAbsentWouldSetEverything(t *testing.T) {
	stub := defaultRulesetStub()
	stub.ListBody = `[]`
	client := testutil.NewTestClient(t, stub.Server(t))
	results, err := alter.ProcessRuleset(defaultRulesetConfig(t), alter.DryRun, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, wouldSetAll(defaultRulesetNoChange))
}

func TestProcessRulesetApplyCreates(t *testing.T) {
	stub := defaultRulesetStub()
	stub.ListBody = `[]`
	client := testutil.NewTestClient(t, stub.Server(t))
	results, err := alter.ProcessRuleset(defaultRulesetConfig(t), alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, wouldSetAll(defaultRulesetNoChange))
	if strings.Join(stub.Writes, ",") != "POST /repos/acme/widget/rulesets" {
		t.Fatalf("writes = %v, want one POST", stub.Writes)
	}
	if stub.LastBody["name"] != "Tailor" || stub.LastBody["target"] != "branch" || stub.LastBody["enforcement"] != "active" {
		t.Errorf("POST body = %v", stub.LastBody)
	}
	rules, _ := json.Marshal(stub.LastBody["rules"])
	want := `[{"type":"deletion"},{"type":"non_fast_forward"},{"parameters":{"allowed_merge_methods":["squash","rebase"],"dismiss_stale_reviews_on_push":true,` +
		`"require_code_owner_review":false,"require_extra_approval_for_unattributed_changes":true,"require_last_push_approval":false,` +
		`"required_approving_review_count":1,"required_review_thread_resolution":true},"type":"pull_request"}]`
	if string(rules) != want {
		t.Errorf("POST rules =\n%s\nwant:\n%s", rules, want)
	}
}

func TestProcessRulesetApplyUpdatesWholeRuleset(t *testing.T) {
	stub := defaultRulesetStub()
	client := testutil.NewTestClient(t, stub.Server(t))
	cfg := defaultRulesetConfig(t)
	cfg.Ruleset.Enforcement = new("disabled")
	results, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string(nil), defaultRulesetNoChange...)
	want[0] = "ruleset.enforcement would set disabled "
	assertResults(t, results, want)
	if strings.Join(stub.Writes, ",") != "PUT /repos/acme/widget/rulesets/42" {
		t.Fatalf("writes = %v, want one PUT", stub.Writes)
	}
	// The whole ruleset travels, not only the changed field.
	for _, key := range []string{"name", "target", "enforcement", "bypass_actors", "conditions", "rules"} {
		if _, ok := stub.LastBody[key]; !ok {
			t.Errorf("PUT body lacks %q: %v", key, stub.LastBody)
		}
	}
}

func TestProcessRulesetApplyNoChangeSendsNothing(t *testing.T) {
	stub := defaultRulesetStub()
	client := testutil.NewTestClient(t, stub.Server(t))
	results, err := alter.ProcessRuleset(defaultRulesetConfig(t), alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, defaultRulesetNoChange)
	if len(stub.Writes) != 0 {
		t.Fatalf("writes = %v, want none", stub.Writes)
	}
}

func TestProcessRulesetReadNotAvailable(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*testutil.RulesetStub)
	}{
		{name: "list forbidden", configure: func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusForbidden
			s.ListBody = `{"message":"Forbidden"}`
		}},
		{name: "read forbidden", configure: func(s *testutil.RulesetStub) {
			s.ReadStatus = http.StatusForbidden
			s.ReadBody = `{"message":"Forbidden"}`
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := defaultRulesetStub()
			tt.configure(stub)
			client := testutil.NewTestClient(t, stub.Server(t))
			cfg := &config.Config{Ruleset: &model.RulesetSettings{Enforcement: new("active"), Rules: &model.RulesetRules{Deletion: new(true)}}}
			results, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
			if err != nil {
				t.Fatal(err)
			}
			assertResults(t, results, []string{
				"ruleset.enforcement would skip  not available",
				"ruleset.rules.deletion would skip  not available",
			})
			if len(stub.Writes) != 0 {
				t.Fatalf("writes = %v, want none", stub.Writes)
			}
			output := alter.FormatOutput(results, nil, nil, alter.Apply)
			want := "would skip (not available):          ruleset.enforcement\n" +
				"would skip (not available):          ruleset.rules.deletion\n"
			if output != want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", output, want)
			}
		})
	}
}

func TestProcessRulesetMissingBypassActorsSkipsScope(t *testing.T) {
	stub := defaultRulesetStub()
	stub.ReadBody = `{"id":42,"name":"Tailor","enforcement":"active","conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"deletion"}]}`
	client := testutil.NewTestClient(t, stub.Server(t))
	cfg := &config.Config{Ruleset: &model.RulesetSettings{Enforcement: new("active"), Rules: &model.RulesetRules{Deletion: new(true)}}}
	results, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{
		"ruleset.enforcement would skip (insufficient scope)  token missing required scope",
		"ruleset.rules.deletion would skip (insufficient scope)  token missing required scope",
	})
	if len(stub.Writes) != 0 {
		t.Fatalf("writes = %v, want none", stub.Writes)
	}
	output := alter.FormatOutput(results, nil, nil, alter.DryRun)
	want := "would skip (insufficient scope: token missing required scope): ruleset.enforcement\n" +
		"would skip (insufficient scope: token missing required scope): ruleset.rules.deletion\n"
	if output != want {
		t.Errorf("FormatOutput() =\n%s\nwant:\n%s", output, want)
	}
}

func TestProcessRulesetReadHardError(t *testing.T) {
	for name, configure := range map[string]func(*testutil.RulesetStub){
		"list server error": func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusInternalServerError
			s.ListBody = `{"message":"boom"}`
		},
		"read server error": func(s *testutil.RulesetStub) {
			s.ReadStatus = http.StatusInternalServerError
			s.ReadBody = `{"message":"boom"}`
		},
	} {
		t.Run(name, func(t *testing.T) {
			stub := defaultRulesetStub()
			configure(stub)
			client := testutil.NewTestClient(t, stub.Server(t))
			if _, err := alter.ProcessRuleset(defaultRulesetConfig(t), alter.Apply, repoTarget(client, "acme", "widget", true)); err == nil {
				t.Fatal("ProcessRuleset() expected error for 500")
			}
		})
	}
}

func TestProcessRulesetWriteSkipped(t *testing.T) {
	stub := defaultRulesetStub()
	stub.WriteStatus = http.StatusForbidden
	client := testutil.NewTestClient(t, stub.Server(t))
	cfg := defaultRulesetConfig(t)
	cfg.Ruleset.Enforcement = new("disabled")
	results, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string(nil), defaultRulesetNoChange...)
	want[0] = "ruleset.enforcement would skip  not available"
	assertResults(t, results, want)
}

func TestProcessRulesetWriteValidationError(t *testing.T) {
	for name, status := range map[string]int{"validation failed": http.StatusUnprocessableEntity, "server error": http.StatusInternalServerError} {
		t.Run(name, func(t *testing.T) {
			stub := defaultRulesetStub()
			stub.WriteStatus = status
			client := testutil.NewTestClient(t, stub.Server(t))
			cfg := defaultRulesetConfig(t)
			cfg.Ruleset.Enforcement = new("disabled")
			_, err := alter.ProcessRuleset(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
			if err == nil || !strings.Contains(err.Error(), "set ruleset") {
				t.Fatalf("ProcessRuleset() error = %v, want set ruleset failure", err)
			}
		})
	}
}

func TestFormatOutputRulesetKeepsConfigOrder(t *testing.T) {
	results := []alter.RepoSettingResult{
		{Section: "ruleset", Field: "rules.pull_request.parameters.required_approving_review_count", Category: alter.RepoNoChange, Value: "1"},
		{Section: "ruleset", Field: "rules.pull_request", Category: alter.RepoNoChange, Value: "enabled"},
		{Section: "ruleset", Field: "rules.required_status_checks.parameters.required_status_checks", Category: alter.WouldSet, Value: "Sentinel 👁️"},
		{Section: "ruleset", Field: "enforcement", Category: alter.WouldSet, Value: "active"},
		{Section: "ruleset", Field: "conditions.ref_name.exclude", Category: alter.RepoNoChange, Value: "(none)"},
	}
	got := alter.FormatOutput(results, nil, nil, alter.DryRun)
	want := "would set:                           ruleset.enforcement = active\n" +
		"would set:                           ruleset.rules.required_status_checks.parameters.required_status_checks = Sentinel 👁️\n" +
		"no change:                           ruleset.conditions.ref_name.exclude (already (none))\n" +
		"no change:                           ruleset.rules.pull_request (already enabled)\n" +
		"no change:                           ruleset.rules.pull_request.parameters.required_approving_review_count (already 1)\n"
	if got != want {
		t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, want)
	}
}

func TestAlterRunRulesetMergeMethodWarning(t *testing.T) {
	configYAML := `license: none
repository:
  allow_merge_commit: false
  allow_squash_merge: true
ruleset:
  enforcement: active
  bypass_actors: []
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    creation: false
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: true
      parameters:
        required_approving_review_count: 1
        dismiss_stale_reviews_on_push: true
        require_code_owner_review: false
        require_last_push_approval: false
        required_review_thread_resolution: true
        require_extra_approval_for_unattributed_changes: true
        allowed_merge_methods:
          - merge
          - squash
    required_status_checks:
      enabled: false
    code_scanning:
      enabled: false
swatches: []
`
	tc := setupAlterTest(t, configYAML, withRuleset(liveTailorRulesetJSON))
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	cfg := loadTestConfig(t, tc.Dir)

	var stdout, stderr strings.Builder
	if err := alter.Run(cfg, tc.Dir, alter.DryRun, tc.Client, &stdout, &stderr); err != nil {
		t.Fatalf("alter.Run() error: %v", err)
	}
	want := "warning: ruleset allows merge merging but repository.allow_merge_commit is false\n"
	if stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if !strings.Contains(stdout.String(), "ruleset.rules.pull_request.parameters.allowed_merge_methods = merge, squash") {
		t.Errorf("stdout = %q, want the merge methods would set", stdout.String())
	}
	if calls := tc.MutatingCalls(); len(calls) != 0 {
		t.Errorf("dry run made mutating calls: %v", calls)
	}
}

func TestAlterRunRulesetIncompleteConfigFails(t *testing.T) {
	configYAML := `license: none
ruleset:
  enforcement: active
  bypass_actors: []
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
swatches: []
`
	tc := setupAlterTest(t, configYAML)
	cfg := loadTestConfig(t, tc.Dir)
	err := alter.Run(cfg, tc.Dir, alter.DryRun, tc.Client, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "ruleset requires rules") {
		t.Fatalf("alter.Run() error = %v, want incomplete ruleset error", err)
	}
}

func TestAlterRunRulesetMergedDefaultsCreateRuleset(t *testing.T) {
	configYAML := `license: none
swatches:
  - path: .tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML)
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, output, "ruleset.enforcement = active")
	requireContains(t, output, "ruleset.rules.pull_request = enabled")

	var posts []apiCall
	for _, call := range tc.Calls() {
		if call.Method == http.MethodPost && call.Path == "/repos/testowner/testrepo/rulesets" {
			posts = append(posts, call)
		}
	}
	if len(posts) != 1 {
		t.Fatalf("ruleset POSTs = %v, want one", posts)
	}
	if !strings.Contains(posts[0].Body, `"name":"Tailor"`) || !strings.Contains(posts[0].Body, `"target":"branch"`) {
		t.Errorf("POST body = %s", posts[0].Body)
	}
	persisted := loadTestConfig(t, tc.Dir)
	if persisted.Ruleset == nil || persisted.Ruleset.Enforcement == nil {
		t.Fatal("merged config lacks the ruleset section")
	}
}

func TestAlterRunRulesetUpgradeAddsCodeScanning(t *testing.T) {
	// A config written by a release before the code scanning rule carries a
	// complete ruleset section without rules.code_scanning. An always config
	// swatch merges the built-in block before validation.
	configYAML := `license: none
ruleset:
  enforcement: active
  bypass_actors:
    - actor_id: 5
      actor_type: RepositoryRole
      bypass_mode: always
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    creation: false
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: true
      parameters:
        required_approving_review_count: 1
        dismiss_stale_reviews_on_push: true
        require_code_owner_review: false
        require_last_push_approval: false
        required_review_thread_resolution: true
        require_extra_approval_for_unattributed_changes: true
        allowed_merge_methods:
          - squash
          - rebase
    required_status_checks:
      enabled: false
      parameters:
        strict_required_status_checks_policy: false
        do_not_enforce_on_create: false
        required_status_checks: []
swatches:
  - path: .tailor.yml
    alteration: always
`
	tc := setupAlterTest(t, configYAML, withRuleset(liveTailorRulesetJSON))
	writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
	cfg := loadTestConfig(t, tc.Dir)
	output := captureAlterRun(t, cfg, tc.Dir, alter.Apply, tc.Client)
	requireContains(t, output, "ruleset.rules.code_scanning (already disabled)")
	if strings.Contains(output, "ruleset.rules.code_scanning = ") {
		t.Errorf("output reports a code scanning change:\n%s", output)
	}
	for _, call := range tc.Calls() {
		if call.Method != http.MethodGet && strings.Contains(call.Path, "/rulesets") {
			t.Errorf("unchanged ruleset reached a write: %s %s", call.Method, call.Path)
		}
	}

	persisted := loadTestConfig(t, tc.Dir)
	rule := persisted.Ruleset.Rules.CodeScanning
	if rule == nil || rule.Enabled == nil || *rule.Enabled {
		t.Fatalf("persisted rules.code_scanning = %+v, want enabled false", rule)
	}
	want := []model.RulesetCodeScanningTool{{Tool: "CodeQL", AlertsThreshold: "errors", SecurityAlertsThreshold: "high_or_higher"}}
	if rule.Parameters == nil || rule.Parameters.CodeScanningTools == nil || !reflect.DeepEqual(*rule.Parameters.CodeScanningTools, want) {
		t.Errorf("persisted code_scanning_tools = %+v, want %+v", rule.Parameters, want)
	}
}

func TestAlterRunRulesetPartialSectionStopsBeforeWrite(t *testing.T) {
	// A first-fit config swatch without --recut never merges defaults, so
	// the partial section must fail validation before any API write.
	tests := []struct {
		name    string
		section string
		wantErr string
	}{
		{
			name: "missing bypass actors",
			section: `  enforcement: active
  rules:
    deletion: true
    pull_request:
      enabled: false
    required_status_checks:
      enabled: false
    code_scanning:
      enabled: false
`,
			wantErr: "ruleset requires bypass_actors",
		},
		{
			// An omitted Boolean key would send no rule of that type and
			// remove a live rule without a report line.
			name: "missing boolean rule",
			section: `  enforcement: active
  bypass_actors: []
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: false
    required_status_checks:
      enabled: false
    code_scanning:
      enabled: false
`,
			wantErr: "ruleset.rules requires creation",
		},
		{
			// An omitted code scanning rule would remove a live rule
			// without a report line.
			name: "missing code scanning rule",
			section: `  enforcement: active
  bypass_actors: []
  conditions:
    ref_name:
      include:
        - ~DEFAULT_BRANCH
      exclude: []
  rules:
    creation: false
    update: false
    deletion: true
    required_linear_history: false
    required_signatures: false
    non_fast_forward: true
    pull_request:
      enabled: false
    required_status_checks:
      enabled: false
`,
			wantErr: "ruleset.rules.code_scanning requires enabled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configYAML := "license: none\nruleset:\n" + tt.section + "swatches:\n  - path: .tailor.yml\n    alteration: first-fit\n"
			tc := setupAlterTest(t, configYAML, withRuleset(liveTailorRulesetJSON))
			writeOnDisk(t, tc.Dir, "LICENSE", []byte("existing"))
			cfg := loadTestConfig(t, tc.Dir)
			err := alter.Run(cfg, tc.Dir, alter.Apply, tc.Client, io.Discard, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("alter.Run() error = %v, want %q", err, tt.wantErr)
			}
			for _, call := range tc.Calls() {
				if call.Method != http.MethodGet {
					t.Errorf("partial ruleset reached a write: %s %s", call.Method, call.Path)
				}
			}
		})
	}
}
