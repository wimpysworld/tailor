package gh

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

const (
	tailorRulesetListJSON = `[{"id":7,"name":"Other","target":"branch","enforcement":"active","source_type":"Repository"},` +
		`{"id":42,"name":"Tailor","target":"branch","enforcement":"active","source_type":"Repository"}]`
	tailorRulesetJSON = `{"id":42,"name":"Tailor","target":"branch","source_type":"Repository","source":"acme/widget",` +
		`"bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"},{"actor_id":null,"actor_type":"DeployKey","bypass_mode":"exempt"}],` +
		`"conditions":{"ref_name":{"exclude":["refs/heads/wip/*"],"include":["~DEFAULT_BRANCH"]}},"enforcement":"disabled",` +
		`"rules":[{"parameters":{"allowed_merge_methods":["squash","rebase"],"dismiss_stale_reviews_on_push":true,` +
		`"dismissal_restriction":{"allowed_actors":[],"enabled":false},"require_code_owner_review":false,` +
		`"require_extra_approval_for_unattributed_changes":true,"require_last_push_approval":false,"required_approving_review_count":1,` +
		`"required_review_thread_resolution":true,"required_reviewers":[]},"type":"pull_request"},{"type":"non_fast_forward"},{"type":"deletion"},` +
		`{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,` +
		`"required_status_checks":[{"context":"Sentinel 👁️"},{"context":"lint","integration_id":15368}]}},` +
		`{"type":"commit_message_pattern","parameters":{"operator":"starts_with","pattern":"feat"}}]}`
)

func defaultRulesetStub() *testutil.RulesetStub {
	return testutil.NewRulesetStub(tailorRulesetListJSON, tailorRulesetJSON)
}

func TestReadTailorRuleset(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*testutil.RulesetStub)
		wantID     int64
		wantAbsent bool
		wantReason SetupSkipReason
		wantScope  bool
		wantErr    string
	}{
		{name: "found", configure: func(*testutil.RulesetStub) {}, wantID: 42},
		{name: "not in list", configure: func(s *testutil.RulesetStub) { s.ListBody = `[{"id":7,"name":"Other"}]` }, wantAbsent: true},
		{name: "organisation ruleset is absent", configure: func(s *testutil.RulesetStub) {
			s.ListBody = `[{"id":9,"name":"Tailor","source_type":"Organization","target":"branch"}]`
		}, wantAbsent: true},
		{name: "tag ruleset is absent", configure: func(s *testutil.RulesetStub) {
			s.ListBody = `[{"id":9,"name":"Tailor","source_type":"Repository","target":"tag"}]`
		}, wantAbsent: true},
		{name: "empty list", configure: func(s *testutil.RulesetStub) { s.ListBody = `[]` }, wantAbsent: true},
		{name: "list forbidden", configure: func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusForbidden
			s.ListBody = `{"message":"Forbidden"}`
		}, wantReason: SetupNotAvailable},
		{name: "list not found", configure: func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusNotFound
			s.ListBody = `{"message":"Not Found"}`
		}, wantReason: SetupNotAvailable},
		{name: "list server error", configure: func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusInternalServerError
			s.ListBody = `{"message":"boom"}`
		}, wantErr: "list rulesets"},
		{name: "list rate limited", configure: func(s *testutil.RulesetStub) {
			s.ListStatus = http.StatusTooManyRequests
			s.ListBody = `{"message":"rate limit exceeded"}`
		}, wantErr: "rate limited"},
		{name: "read forbidden", configure: func(s *testutil.RulesetStub) {
			s.ReadStatus = http.StatusForbidden
			s.ReadBody = `{"message":"Forbidden"}`
		}, wantReason: SetupNotAvailable},
		{name: "read not found is absent", configure: func(s *testutil.RulesetStub) {
			s.ReadStatus = http.StatusNotFound
			s.ReadBody = `{"message":"Not Found"}`
		}, wantAbsent: true},
		{name: "read server error", configure: func(s *testutil.RulesetStub) {
			s.ReadStatus = http.StatusInternalServerError
			s.ReadBody = `{"message":"boom"}`
		}, wantErr: "fetch ruleset"},
		{name: "bypass actors omitted", configure: func(s *testutil.RulesetStub) {
			s.ReadBody = `{"id":42,"name":"Tailor","enforcement":"active","conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[]}`
		}, wantScope: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := defaultRulesetStub()
			tt.configure(stub)
			live, id, err := ReadTailorRuleset(testutil.NewTestClient(t, stub.Server(t)), "acme", "widget")
			var scope *ErrInsufficientScope
			switch {
			case tt.wantScope:
				if !errors.As(err, &scope) || scope.Operation.Kind != OpFetchRuleset || scope.StatusCode != http.StatusOK {
					t.Fatalf("error = %v, want insufficient scope on fetch", err)
				}
				if !strings.Contains(err.Error(), "fetch ruleset: response omitted bypass_actors") {
					t.Errorf("error text = %q", err)
				}
				return
			case tt.wantReason != "" || tt.wantErr != "":
				kind := OpListRulesets
				if strings.HasPrefix(tt.name, "read") {
					kind = OpFetchRuleset
				}
				assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, kind)
				return
			case err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantAbsent:
				if live != nil || id != 0 {
					t.Fatalf("ReadTailorRuleset() = %v, %d, want absent", live, id)
				}
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %d, want %d", id, tt.wantID)
			}
			if stub.ListQuery != "per_page=100&includes_parents=false&targets=branch" {
				t.Errorf("list query = %q, want parents excluded and branch target", stub.ListQuery)
			}
			if stub.ReadQuery != "includes_parents=false" {
				t.Errorf("read query = %q, want parents excluded", stub.ReadQuery)
			}
			testutil.AssertPtr(t, live.Enforcement, false, "disabled", "enforcement")
			wantActors := []model.RulesetBypassActor{
				{ActorID: new(5), ActorType: new("RepositoryRole"), BypassMode: new("always")},
				{ActorType: new("DeployKey"), BypassMode: new("exempt")},
			}
			if !reflect.DeepEqual(*live.BypassActors, wantActors) {
				t.Errorf("bypass_actors = %+v, want %+v", *live.BypassActors, wantActors)
			}
			if !reflect.DeepEqual(*live.Conditions.RefName.Include, []string{"~DEFAULT_BRANCH"}) || !reflect.DeepEqual(*live.Conditions.RefName.Exclude, []string{"refs/heads/wip/*"}) {
				t.Errorf("conditions = %+v", *live.Conditions.RefName)
			}
			rules := live.Rules
			testutil.AssertPtr(t, rules.Creation, false, false, "rules.creation")
			testutil.AssertPtr(t, rules.Deletion, false, true, "rules.deletion")
			testutil.AssertPtr(t, rules.NonFastForward, false, true, "rules.non_fast_forward")
			testutil.AssertPtr(t, rules.PullRequest.Enabled, false, true, "rules.pull_request.enabled")
			p := rules.PullRequest.Parameters
			testutil.AssertPtr(t, p.RequiredApprovingReviewCount, false, 1, "required_approving_review_count")
			testutil.AssertPtr(t, p.DismissStaleReviewsOnPush, false, true, "dismiss_stale_reviews_on_push")
			testutil.AssertPtr(t, p.RequireCodeOwnerReview, false, false, "require_code_owner_review")
			testutil.AssertPtr(t, p.RequireLastPushApproval, false, false, "require_last_push_approval")
			testutil.AssertPtr(t, p.RequiredReviewThreadResolution, false, true, "required_review_thread_resolution")
			testutil.AssertPtr(t, p.RequireExtraApprovalForUnattributedChanges, false, true, "require_extra_approval_for_unattributed_changes")
			if !reflect.DeepEqual(*p.AllowedMergeMethods, []string{"squash", "rebase"}) {
				t.Errorf("allowed_merge_methods = %v", *p.AllowedMergeMethods)
			}
			testutil.AssertPtr(t, rules.RequiredStatusChecks.Enabled, false, true, "rules.required_status_checks.enabled")
			checks := rules.RequiredStatusChecks.Parameters
			testutil.AssertPtr(t, checks.StrictRequiredStatusChecksPolicy, false, true, "strict_required_status_checks_policy")
			testutil.AssertPtr(t, checks.DoNotEnforceOnCreate, false, false, "do_not_enforce_on_create")
			wantChecks := []model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}}
			if !reflect.DeepEqual(*checks.RequiredStatusChecks, wantChecks) {
				t.Errorf("required_status_checks = %+v, want %+v", *checks.RequiredStatusChecks, wantChecks)
			}
		})
	}
}

func TestReadTailorRulesetAbsentRulesAreDisabled(t *testing.T) {
	stub := defaultRulesetStub()
	stub.ReadBody = `{"id":42,"name":"Tailor","enforcement":"active","bypass_actors":[],"conditions":{"ref_name":{"include":[],"exclude":[]}},"rules":[]}`
	live, _, err := ReadTailorRuleset(testutil.NewTestClient(t, stub.Server(t)), "acme", "widget")
	if err != nil {
		t.Fatalf("ReadTailorRuleset() error: %v", err)
	}
	for name, value := range map[string]*bool{
		"creation":                live.Rules.Creation,
		"update":                  live.Rules.Update,
		"deletion":                live.Rules.Deletion,
		"required_linear_history": live.Rules.RequiredLinearHistory,
		"required_signatures":     live.Rules.RequiredSignatures,
		"non_fast_forward":        live.Rules.NonFastForward,
		"pull_request":            live.Rules.PullRequest.Enabled,
		"required_status_checks":  live.Rules.RequiredStatusChecks.Enabled,
	} {
		testutil.AssertPtr(t, value, false, false, name)
	}
	if live.Rules.PullRequest.Parameters != nil || live.Rules.RequiredStatusChecks.Parameters != nil {
		t.Error("absent rules carry parameters")
	}
	if len(*live.BypassActors) != 0 || len(*live.Conditions.RefName.Include) != 0 {
		t.Errorf("empty lists were not preserved: %+v", live)
	}
}

func desiredRuleset() *model.RulesetSettings {
	return &model.RulesetSettings{
		Enforcement:  new("active"),
		BypassActors: &[]model.RulesetBypassActor{{ActorID: new(5), ActorType: new("RepositoryRole"), BypassMode: new("always")}, {ActorType: new("DeployKey"), BypassMode: new("always")}},
		Conditions:   &model.RulesetConditions{RefName: &model.RulesetRefName{Include: &[]string{"~DEFAULT_BRANCH"}, Exclude: &[]string{}}},
		Rules: &model.RulesetRules{
			Creation: new(false), Update: new(true), Deletion: new(true), RequiredLinearHistory: new(false), RequiredSignatures: new(false), NonFastForward: new(true),
			PullRequest: &model.RulesetPullRequest{Enabled: new(true), Parameters: &model.RulesetPullRequestParameters{
				RequiredApprovingReviewCount: new(1), DismissStaleReviewsOnPush: new(true), RequireCodeOwnerReview: new(false), RequireLastPushApproval: new(false),
				RequiredReviewThreadResolution: new(true), RequireExtraApprovalForUnattributedChanges: new(true), AllowedMergeMethods: &[]string{"squash", "rebase"},
			}},
			RequiredStatusChecks: &model.RulesetStatusChecks{Enabled: new(true), Parameters: &model.RulesetStatusChecksParameters{
				StrictRequiredStatusChecksPolicy: new(true), DoNotEnforceOnCreate: new(false),
				RequiredStatusChecks: &[]model.RulesetStatusCheck{{Context: "Sentinel 👁️"}, {Context: "lint", IntegrationID: new(15368)}},
			}},
		},
	}
}

const desiredRulesetBodyJSON = `{"bypass_actors":[{"actor_id":5,"actor_type":"RepositoryRole","bypass_mode":"always"},{"actor_id":null,"actor_type":"DeployKey","bypass_mode":"always"}],` +
	`"conditions":{"ref_name":{"exclude":[],"include":["~DEFAULT_BRANCH"]}},"enforcement":"active","name":"Tailor",` +
	`"rules":[{"type":"update"},{"type":"deletion"},{"type":"non_fast_forward"},` +
	`{"parameters":{"allowed_merge_methods":["squash","rebase"],"dismiss_stale_reviews_on_push":true,"require_code_owner_review":false,` +
	`"require_extra_approval_for_unattributed_changes":true,"require_last_push_approval":false,"required_approving_review_count":1,"required_review_thread_resolution":true},"type":"pull_request"},` +
	`{"parameters":{"do_not_enforce_on_create":false,"required_status_checks":[{"context":"Sentinel 👁️"},{"context":"lint","integration_id":15368}],"strict_required_status_checks_policy":true},"type":"required_status_checks"}],` +
	`"target":"branch"}`

func TestRulesetBody(t *testing.T) {
	tests := []struct {
		name    string
		desired *model.RulesetSettings
		want    string
	}{
		{name: "complete", desired: desiredRuleset(), want: desiredRulesetBodyJSON},
		{
			name:    "empty section sends the frame",
			desired: &model.RulesetSettings{},
			want:    `{"bypass_actors":[],"conditions":{"ref_name":{"exclude":[],"include":[]}},"enforcement":"","name":"Tailor","rules":[],"target":"branch"}`,
		},
		{
			name: "disabled rules send no parameters",
			desired: &model.RulesetSettings{Enforcement: new("disabled"), Rules: &model.RulesetRules{
				Deletion:             new(false),
				PullRequest:          &model.RulesetPullRequest{Enabled: new(false), Parameters: &model.RulesetPullRequestParameters{RequiredApprovingReviewCount: new(2)}},
				RequiredStatusChecks: &model.RulesetStatusChecks{Enabled: new(false), Parameters: &model.RulesetStatusChecksParameters{StrictRequiredStatusChecksPolicy: new(true)}},
			}},
			want: `{"bypass_actors":[],"conditions":{"ref_name":{"exclude":[],"include":[]}},"enforcement":"disabled","name":"Tailor","rules":[],"target":"branch"}`,
		},
		{
			name: "enabled rules without parameters send empty parameters",
			desired: &model.RulesetSettings{Rules: &model.RulesetRules{
				PullRequest:          &model.RulesetPullRequest{Enabled: new(true)},
				RequiredStatusChecks: &model.RulesetStatusChecks{Enabled: new(true)},
			}},
			want: `{"bypass_actors":[],"conditions":{"ref_name":{"exclude":[],"include":[]}},"enforcement":"","name":"Tailor",` +
				`"rules":[{"parameters":{},"type":"pull_request"},{"parameters":{"required_status_checks":[]},"type":"required_status_checks"}],"target":"branch"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(rulesetBody(tt.desired))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("body =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestRulesetRulesRoundTrip(t *testing.T) {
	desired := desiredRuleset()
	list := rulesToList(desired.Rules)
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	var parsed []rulesetRuleJSON
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if got := rulesFromList(parsed); !reflect.DeepEqual(got, desired.Rules) {
		t.Errorf("rules after round trip = %+v, want %+v", got, desired.Rules)
	}
}

func TestApplyRuleset(t *testing.T) {
	tests := []struct {
		name       string
		id         int64
		status     int
		wantWrite  string
		wantReason SetupSkipReason
		wantErr    string
	}{
		{name: "create", id: 0, status: http.StatusOK, wantWrite: "POST /repos/acme/widget/rulesets"},
		{name: "update", id: 42, status: http.StatusOK, wantWrite: "PUT /repos/acme/widget/rulesets/42"},
		{name: "create forbidden", id: 0, status: http.StatusForbidden, wantWrite: "POST /repos/acme/widget/rulesets", wantReason: SetupNotAvailable},
		{name: "update forbidden", id: 42, status: http.StatusForbidden, wantWrite: "PUT /repos/acme/widget/rulesets/42", wantReason: SetupNotAvailable},
		{name: "validation failed", id: 42, status: http.StatusUnprocessableEntity, wantWrite: "PUT /repos/acme/widget/rulesets/42", wantErr: "set ruleset: HTTP 422"},
		{name: "not found", id: 42, status: http.StatusNotFound, wantWrite: "PUT /repos/acme/widget/rulesets/42", wantErr: "set ruleset: HTTP 404"},
		{name: "server error", id: 0, status: http.StatusInternalServerError, wantWrite: "POST /repos/acme/widget/rulesets", wantErr: "set ruleset: HTTP 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := defaultRulesetStub()
			stub.WriteStatus = tt.status
			err := ApplyRuleset(testutil.NewTestClient(t, stub.Server(t)), "acme", "widget", tt.id, desiredRuleset())
			assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, OpSetRuleset)
			if strings.Join(stub.Writes, ",") != tt.wantWrite {
				t.Errorf("writes = %v, want %q", stub.Writes, tt.wantWrite)
			}
			if got, _ := json.Marshal(stub.LastBody); string(got) != desiredRulesetBodyJSON {
				t.Errorf("write body =\n%s\nwant:\n%s", got, desiredRulesetBodyJSON)
			}
		})
	}
}

func TestApplyRulesetRateLimited(t *testing.T) {
	stub := defaultRulesetStub()
	stub.WriteStatus = http.StatusTooManyRequests
	err := ApplyRuleset(testutil.NewTestClient(t, stub.Server(t)), "acme", "widget", 42, desiredRuleset())
	if !isRateLimitError(err) {
		t.Fatalf("error = %v, want rate limited", err)
	}
}

func TestRulesetOperationText(t *testing.T) {
	for kind, want := range map[OperationKind]string{
		OpListRulesets: "list rulesets",
		OpFetchRuleset: "fetch ruleset",
		OpSetRuleset:   "set ruleset",
	} {
		if got := Op(kind).String(); got != want {
			t.Errorf("Op(%d).String() = %q, want %q", kind, got, want)
		}
	}
}
