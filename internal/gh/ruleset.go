package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

// rulesetSummary holds the fields of one entry in the abbreviated ruleset
// list response that Tailor reads.
type rulesetSummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Target     string `json:"target"`
	SourceType string `json:"source_type"`
}

// rulesetSourceType is the source_type of a ruleset that the repository
// itself owns. Organisation and enterprise rulesets carry other values.
const rulesetSourceType = "Repository"

// rulesetListQuery limits the list to branch rulesets that the repository
// owns, so a parent ruleset or a tag ruleset named Tailor is never matched.
const rulesetListQuery = "?per_page=100&includes_parents=false&targets=" + model.RulesetTarget

// rulesetReadQuery keeps a parent ruleset out of the read by id.
const rulesetReadQuery = "?includes_parents=false"

// rulesetResponse holds the managed fields of a full ruleset response.
// BypassActors is a pointer so an omitted key, which GitHub sends when the
// token cannot write the ruleset, differs from an empty list.
type rulesetResponse struct {
	Enforcement  string              `json:"enforcement"`
	BypassActors *[]rulesetActorJSON `json:"bypass_actors"`
	Conditions   struct {
		RefName struct {
			Include []string `json:"include"`
			Exclude []string `json:"exclude"`
		} `json:"ref_name"`
	} `json:"conditions"`
	Rules []rulesetRuleJSON `json:"rules"`
}

type rulesetActorJSON struct {
	ActorID    *int   `json:"actor_id"`
	ActorType  string `json:"actor_type"`
	BypassMode string `json:"bypass_mode"`
}

// rulesetRuleJSON is one entry of the rules list. Parameters stay raw
// because their form depends on the rule type.
type rulesetRuleJSON struct {
	Type       string          `json:"type"`
	Parameters json.RawMessage `json:"parameters,omitempty"`
}

// rulesetCheckJSON is one required status check in the API form.
type rulesetCheckJSON struct {
	Context       string `json:"context"`
	IntegrationID *int   `json:"integration_id,omitempty"`
}

// pullRequestParametersJSON holds the seven managed parameters of the pull
// request rule. Parameters that Tailor does not manage are ignored on read
// and never sent.
type pullRequestParametersJSON struct {
	RequiredApprovingReviewCount               *int      `json:"required_approving_review_count"`
	DismissStaleReviewsOnPush                  *bool     `json:"dismiss_stale_reviews_on_push"`
	RequireCodeOwnerReview                     *bool     `json:"require_code_owner_review"`
	RequireLastPushApproval                    *bool     `json:"require_last_push_approval"`
	RequiredReviewThreadResolution             *bool     `json:"required_review_thread_resolution"`
	RequireExtraApprovalForUnattributedChanges *bool     `json:"require_extra_approval_for_unattributed_changes"`
	AllowedMergeMethods                        *[]string `json:"allowed_merge_methods"`
}

// statusChecksParametersJSON holds the managed parameters of the required
// status checks rule.
type statusChecksParametersJSON struct {
	StrictRequiredStatusChecksPolicy *bool               `json:"strict_required_status_checks_policy"`
	DoNotEnforceOnCreate             *bool               `json:"do_not_enforce_on_create"`
	RequiredStatusChecks             *[]rulesetCheckJSON `json:"required_status_checks"`
}

// rulesetToolJSON is one code scanning tool in the API form. The fields
// are in key order so a marshalled body matches a re-marshalled map.
type rulesetToolJSON struct {
	AlertsThreshold         string `json:"alerts_threshold"`
	SecurityAlertsThreshold string `json:"security_alerts_threshold"`
	Tool                    string `json:"tool"`
}

// codeScanningParametersJSON holds the managed parameters of the code
// scanning rule.
type codeScanningParametersJSON struct {
	CodeScanningTools *[]rulesetToolJSON `json:"code_scanning_tools"`
}

func rulesetsPath(owner, name string) string {
	return fmt.Sprintf("repos/%s/%s/rulesets", owner, name)
}

func rulesetPath(owner, name string, id int64) string {
	return fmt.Sprintf("repos/%s/%s/rulesets/%d", owner, name, id)
}

// ReadTailorRuleset finds the branch ruleset named Tailor that the
// repository owns and reads it. Parent rulesets and tag rulesets are
// ignored. It returns a nil ruleset and a zero id when the ruleset is
// absent. It returns an *ErrSetupSkipped when rulesets are not available to
// the token, and an *ErrInsufficientScope when the response omits the
// bypass actors, which means the token cannot write the ruleset.
func ReadTailorRuleset(client *api.RESTClient, owner, name string) (*model.RulesetSettings, int64, error) {
	var summaries []rulesetSummary
	if err := readSetup(client, rulesetsPath(owner, name)+rulesetListQuery, Op(OpListRulesets), &summaries); err != nil {
		return nil, 0, err
	}
	var id int64
	for _, summary := range summaries {
		if summary.Name == model.RulesetName && summary.SourceType == rulesetSourceType && summary.Target == model.RulesetTarget {
			id = summary.ID
			break
		}
	}
	if id == 0 {
		return nil, 0, nil
	}

	var response rulesetResponse
	err := boundedHTTPError(client.Get(rulesetPath(owner, name, id)+rulesetReadQuery, &response))
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		// The ruleset disappeared between the list and the read.
		return nil, 0, nil
	}
	if err := classifySetupError(err, Op(OpFetchRuleset), false); err != nil {
		return nil, 0, err
	}
	if response.BypassActors == nil {
		return nil, 0, &ErrInsufficientScope{
			StatusCode: http.StatusOK,
			Message:    "response omitted bypass_actors; the token cannot manage the ruleset",
			Operation:  Op(OpFetchRuleset),
		}
	}
	return rulesetFromResponse(&response), id, nil
}

// ApplyRuleset sends the complete desired ruleset. It creates the ruleset
// when id is zero and updates it otherwise. It returns an *ErrSetupSkipped
// when rulesets are not available to the token. Every other failure,
// including a 422 validation error, is returned as a hard error.
func ApplyRuleset(client *api.RESTClient, owner, name string, id int64, desired *model.RulesetSettings) error {
	operation := Op(OpSetRuleset)
	payload, err := json.Marshal(rulesetBody(desired))
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", operation, err)
	}
	if id == 0 {
		err = client.Post(rulesetsPath(owner, name), bytes.NewReader(payload), nil)
	} else {
		err = client.Put(rulesetPath(owner, name, id), bytes.NewReader(payload), nil)
	}
	return classifyRulesetWriteError(boundedHTTPError(err), operation)
}

// classifyRulesetWriteError converts a 403 into *ErrSetupSkipped and a rate
// limit response into *ErrRateLimited. Other errors carry the operation and
// stop the command.
func classifyRulesetWriteError(err error, operation Operation) error {
	if err == nil {
		return nil
	}
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if isRateLimitHTTPError(httpErr) {
		return classifyHTTPError(err, operation)
	}
	if httpErr.StatusCode == http.StatusForbidden {
		return &ErrSetupSkipped{StatusCode: httpErr.StatusCode, Reason: SetupNotAvailable, Operation: operation}
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// rulesetFromResponse converts a full ruleset response into the config
// model. A rule type absent from the list becomes false or disabled.
func rulesetFromResponse(response *rulesetResponse) *model.RulesetSettings {
	actors := make([]model.RulesetBypassActor, 0, len(*response.BypassActors))
	for _, actor := range *response.BypassActors {
		actors = append(actors, model.RulesetBypassActor{
			ActorID:    actor.ActorID,
			ActorType:  new(actor.ActorType),
			BypassMode: new(actor.BypassMode),
		})
	}
	include := response.Conditions.RefName.Include
	if include == nil {
		include = []string{}
	}
	exclude := response.Conditions.RefName.Exclude
	if exclude == nil {
		exclude = []string{}
	}
	return &model.RulesetSettings{
		Enforcement:  new(response.Enforcement),
		BypassActors: &actors,
		Conditions:   &model.RulesetConditions{RefName: &model.RulesetRefName{Include: &include, Exclude: &exclude}},
		Rules:        rulesFromList(response.Rules),
	}
}

// rulesFromList converts the API rules list into the config rules map.
func rulesFromList(list []rulesetRuleJSON) *model.RulesetRules {
	rules := &model.RulesetRules{
		Creation:              new(false),
		Update:                new(false),
		Deletion:              new(false),
		RequiredLinearHistory: new(false),
		RequiredSignatures:    new(false),
		NonFastForward:        new(false),
		PullRequest:           &model.RulesetPullRequest{Enabled: new(false)},
		RequiredStatusChecks:  &model.RulesetStatusChecks{Enabled: new(false)},
		CodeScanning:          &model.RulesetCodeScanning{Enabled: new(false)},
	}
	for _, rule := range list {
		switch rule.Type {
		case "creation":
			rules.Creation = new(true)
		case "update":
			rules.Update = new(true)
		case "deletion":
			rules.Deletion = new(true)
		case "required_linear_history":
			rules.RequiredLinearHistory = new(true)
		case "required_signatures":
			rules.RequiredSignatures = new(true)
		case "non_fast_forward":
			rules.NonFastForward = new(true)
		case "pull_request":
			rules.PullRequest = pullRequestFromJSON(rule.Parameters)
		case "required_status_checks":
			rules.RequiredStatusChecks = statusChecksFromJSON(rule.Parameters)
		case "code_scanning":
			rules.CodeScanning = codeScanningFromJSON(rule.Parameters)
		}
	}
	return rules
}

// pullRequestFromJSON decodes the managed pull request parameters. Unknown
// parameters are ignored. A parameter the response omits stays nil.
func pullRequestFromJSON(raw json.RawMessage) *model.RulesetPullRequest {
	rule := &model.RulesetPullRequest{Enabled: new(true)}
	var p pullRequestParametersJSON
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil {
		return rule
	}
	rule.Parameters = &model.RulesetPullRequestParameters{
		RequiredApprovingReviewCount:               p.RequiredApprovingReviewCount,
		DismissStaleReviewsOnPush:                  p.DismissStaleReviewsOnPush,
		RequireCodeOwnerReview:                     p.RequireCodeOwnerReview,
		RequireLastPushApproval:                    p.RequireLastPushApproval,
		RequiredReviewThreadResolution:             p.RequiredReviewThreadResolution,
		RequireExtraApprovalForUnattributedChanges: p.RequireExtraApprovalForUnattributedChanges,
		AllowedMergeMethods:                        p.AllowedMergeMethods,
	}
	return rule
}

// statusChecksFromJSON decodes the managed required status checks
// parameters. A parameter the response omits stays nil.
func statusChecksFromJSON(raw json.RawMessage) *model.RulesetStatusChecks {
	rule := &model.RulesetStatusChecks{Enabled: new(true)}
	var p statusChecksParametersJSON
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil {
		return rule
	}
	parameters := &model.RulesetStatusChecksParameters{
		StrictRequiredStatusChecksPolicy: p.StrictRequiredStatusChecksPolicy,
		DoNotEnforceOnCreate:             p.DoNotEnforceOnCreate,
	}
	if p.RequiredStatusChecks != nil {
		checks := make([]model.RulesetStatusCheck, 0, len(*p.RequiredStatusChecks))
		for _, check := range *p.RequiredStatusChecks {
			checks = append(checks, model.RulesetStatusCheck{Context: check.Context, IntegrationID: check.IntegrationID})
		}
		parameters.RequiredStatusChecks = &checks
	}
	rule.Parameters = parameters
	return rule
}

// codeScanningFromJSON decodes the managed code scanning parameters. A
// parameter the response omits stays nil.
func codeScanningFromJSON(raw json.RawMessage) *model.RulesetCodeScanning {
	rule := &model.RulesetCodeScanning{Enabled: new(true)}
	var p codeScanningParametersJSON
	if len(raw) == 0 || json.Unmarshal(raw, &p) != nil {
		return rule
	}
	parameters := &model.RulesetCodeScanningParameters{}
	if p.CodeScanningTools != nil {
		tools := make([]model.RulesetCodeScanningTool, 0, len(*p.CodeScanningTools))
		for _, tool := range *p.CodeScanningTools {
			tools = append(tools, model.RulesetCodeScanningTool{
				Tool:                    tool.Tool,
				AlertsThreshold:         tool.AlertsThreshold,
				SecurityAlertsThreshold: tool.SecurityAlertsThreshold,
			})
		}
		parameters.CodeScanningTools = &tools
	}
	rule.Parameters = parameters
	return rule
}

// rulesetBody builds the complete request body for a ruleset write. Every
// managed field is sent; nothing else is.
func rulesetBody(desired *model.RulesetSettings) map[string]any {
	body := map[string]any{
		"name":          model.RulesetName,
		"target":        model.RulesetTarget,
		"enforcement":   "",
		"bypass_actors": []any{},
		"conditions": map[string]any{
			"ref_name": map[string]any{"include": []string{}, "exclude": []string{}},
		},
		"rules": []any{},
	}
	if desired.Enforcement != nil {
		body["enforcement"] = *desired.Enforcement
	}
	if desired.BypassActors != nil {
		actors := make([]any, 0, len(*desired.BypassActors))
		for _, actor := range *desired.BypassActors {
			entry := map[string]any{"actor_id": nil}
			if actor.ActorID != nil {
				entry["actor_id"] = *actor.ActorID
			}
			if actor.ActorType != nil {
				entry["actor_type"] = *actor.ActorType
			}
			if actor.BypassMode != nil {
				entry["bypass_mode"] = *actor.BypassMode
			}
			actors = append(actors, entry)
		}
		body["bypass_actors"] = actors
	}
	if desired.Conditions != nil && desired.Conditions.RefName != nil {
		refName := body["conditions"].(map[string]any)["ref_name"].(map[string]any)
		if desired.Conditions.RefName.Include != nil {
			refName["include"] = *desired.Conditions.RefName.Include
		}
		if desired.Conditions.RefName.Exclude != nil {
			refName["exclude"] = *desired.Conditions.RefName.Exclude
		}
	}
	if desired.Rules != nil {
		body["rules"] = rulesToList(desired.Rules)
	}
	return body
}

// rulesToList converts the config rules map into the API rules list. A
// true Boolean adds a rule of that type. An enabled pull request, required
// status checks, or code scanning rule carries its managed parameters.
func rulesToList(rules *model.RulesetRules) []any {
	list := make([]any, 0, 9)
	for _, rule := range []struct {
		name    string
		enabled *bool
	}{
		{"creation", rules.Creation},
		{"update", rules.Update},
		{"deletion", rules.Deletion},
		{"required_linear_history", rules.RequiredLinearHistory},
		{"required_signatures", rules.RequiredSignatures},
		{"non_fast_forward", rules.NonFastForward},
	} {
		if isTrue(rule.enabled) {
			list = append(list, map[string]any{"type": rule.name})
		}
	}
	if pr := rules.PullRequest; pr != nil && isTrue(pr.Enabled) {
		list = append(list, map[string]any{"type": "pull_request", "parameters": pullRequestParametersBody(pr.Parameters)})
	}
	if checks := rules.RequiredStatusChecks; checks != nil && isTrue(checks.Enabled) {
		list = append(list, map[string]any{"type": "required_status_checks", "parameters": statusChecksParametersBody(checks.Parameters)})
	}
	if scanning := rules.CodeScanning; scanning != nil && isTrue(scanning.Enabled) {
		list = append(list, map[string]any{"type": "code_scanning", "parameters": codeScanningParametersBody(scanning.Parameters)})
	}
	return list
}

func pullRequestParametersBody(p *model.RulesetPullRequestParameters) map[string]any {
	body := make(map[string]any, 7)
	if p == nil {
		return body
	}
	if p.RequiredApprovingReviewCount != nil {
		body["required_approving_review_count"] = *p.RequiredApprovingReviewCount
	}
	for key, value := range map[string]*bool{
		"dismiss_stale_reviews_on_push":                   p.DismissStaleReviewsOnPush,
		"require_code_owner_review":                       p.RequireCodeOwnerReview,
		"require_last_push_approval":                      p.RequireLastPushApproval,
		"required_review_thread_resolution":               p.RequiredReviewThreadResolution,
		"require_extra_approval_for_unattributed_changes": p.RequireExtraApprovalForUnattributedChanges,
	} {
		if value != nil {
			body[key] = *value
		}
	}
	if p.AllowedMergeMethods != nil {
		body["allowed_merge_methods"] = *p.AllowedMergeMethods
	}
	return body
}

func statusChecksParametersBody(p *model.RulesetStatusChecksParameters) map[string]any {
	body := map[string]any{"required_status_checks": []any{}}
	if p == nil {
		return body
	}
	if p.StrictRequiredStatusChecksPolicy != nil {
		body["strict_required_status_checks_policy"] = *p.StrictRequiredStatusChecksPolicy
	}
	if p.DoNotEnforceOnCreate != nil {
		body["do_not_enforce_on_create"] = *p.DoNotEnforceOnCreate
	}
	if p.RequiredStatusChecks != nil {
		checks := make([]any, 0, len(*p.RequiredStatusChecks))
		for _, check := range *p.RequiredStatusChecks {
			checks = append(checks, rulesetCheckJSON{Context: check.Context, IntegrationID: check.IntegrationID})
		}
		body["required_status_checks"] = checks
	}
	return body
}

func codeScanningParametersBody(p *model.RulesetCodeScanningParameters) map[string]any {
	body := map[string]any{"code_scanning_tools": []any{}}
	if p == nil || p.CodeScanningTools == nil {
		return body
	}
	tools := make([]any, 0, len(*p.CodeScanningTools))
	for _, tool := range *p.CodeScanningTools {
		tools = append(tools, rulesetToolJSON{
			Tool:                    tool.Tool,
			AlertsThreshold:         tool.AlertsThreshold,
			SecurityAlertsThreshold: tool.SecurityAlertsThreshold,
		})
	}
	body["code_scanning_tools"] = tools
	return body
}
