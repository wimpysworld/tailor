package alter

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// rulesetSection is the Section of every ruleset result.
const rulesetSection = "ruleset"

// ProcessRuleset compares the declared Tailor ruleset against GitHub and
// writes the complete ruleset when any managed field differs. A repository
// without rulesets reports the declared fields as skipped. A token that can
// read but not write the ruleset reports them as scope skips.
func ProcessRuleset(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.Ruleset == nil || !target.HasRepo {
		return nil, nil
	}
	var id int64
	return processSetup(compareRuleset(cfg.Ruleset, nil), mode,
		func() ([]RepoSettingResult, error) {
			live, liveID, err := gh.ReadTailorRuleset(target.Client, target.Owner, target.Name)
			if err != nil {
				return nil, err
			}
			id = liveID
			return compareRuleset(cfg.Ruleset, live), nil
		},
		func([]RepoSettingResult) error {
			return gh.ApplyRuleset(target.Client, target.Owner, target.Name, id, cfg.Ruleset)
		})
}

// rulesetComparer collects one result per declared managed field. A nil
// live ruleset means the ruleset is absent, so every field would be set.
type rulesetComparer struct {
	results []RepoSettingResult
}

func (c *rulesetComparer) add(field, value string, equal bool) {
	category := WouldSet
	if equal {
		category = RepoNoChange
	}
	c.results = append(c.results, RepoSettingResult{Section: rulesetSection, Field: field, Category: category, Value: value})
}

func (c *rulesetComparer) str(field string, declared, live *string) {
	if declared != nil {
		c.add(field, *declared, live != nil && *live == *declared)
	}
}

func (c *rulesetComparer) boolean(field string, declared, live *bool) {
	if declared != nil {
		c.add(field, strconv.FormatBool(*declared), live != nil && *live == *declared)
	}
}

func (c *rulesetComparer) enabled(field string, declared, live *bool) {
	if declared != nil {
		c.add(field, enabledText(*declared), live != nil && *live == *declared)
	}
}

func (c *rulesetComparer) count(field string, declared, live *int) {
	if declared != nil {
		c.add(field, strconv.Itoa(*declared), live != nil && *live == *declared)
	}
}

// set compares two lists as sets and renders the declared list joined by
// ", ", or "(none)" when empty.
func (c *rulesetComparer) set(field string, declared, live *[]string) {
	if declared != nil {
		c.add(field, listText(*declared), live != nil && equalStringSets(*declared, *live))
	}
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func listText(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

// compareRuleset returns one result per declared managed field.
func compareRuleset(declared, live *model.RulesetSettings) []RepoSettingResult {
	if live == nil {
		live = &model.RulesetSettings{}
	}
	c := &rulesetComparer{}
	c.str("enforcement", declared.Enforcement, live.Enforcement)
	if declared.BypassActors != nil {
		c.set("bypass_actors", actorTexts(declared.BypassActors), actorTexts(live.BypassActors))
	}
	compareRefName(c, refNameOf(declared), refNameOf(live))
	if declared.Rules != nil {
		liveRules := live.Rules
		if liveRules == nil {
			liveRules = &model.RulesetRules{}
		}
		compareRules(c, declared.Rules, liveRules)
	}
	return c.results
}

func refNameOf(r *model.RulesetSettings) *model.RulesetRefName {
	if r.Conditions == nil || r.Conditions.RefName == nil {
		return nil
	}
	return r.Conditions.RefName
}

func compareRefName(c *rulesetComparer, declared, live *model.RulesetRefName) {
	if declared == nil {
		return
	}
	if live == nil {
		live = &model.RulesetRefName{}
	}
	c.set("conditions.ref_name.include", declared.Include, live.Include)
	c.set("conditions.ref_name.exclude", declared.Exclude, live.Exclude)
}

func compareRules(c *rulesetComparer, declared, live *model.RulesetRules) {
	c.boolean("rules.creation", declared.Creation, live.Creation)
	c.boolean("rules.update", declared.Update, live.Update)
	c.boolean("rules.deletion", declared.Deletion, live.Deletion)
	c.boolean("rules.required_linear_history", declared.RequiredLinearHistory, live.RequiredLinearHistory)
	c.boolean("rules.required_signatures", declared.RequiredSignatures, live.RequiredSignatures)
	c.boolean("rules.non_fast_forward", declared.NonFastForward, live.NonFastForward)
	comparePullRequest(c, declared.PullRequest, live.PullRequest)
	compareStatusChecks(c, declared.RequiredStatusChecks, live.RequiredStatusChecks)
	compareRulesetCodeScanning(c, declared.CodeScanning, live.CodeScanning)
}

// comparePullRequest compares the rule presence and, when the declared rule
// is enabled, its seven parameters. A disabled rule sends no parameters, so
// they are not compared.
func comparePullRequest(c *rulesetComparer, declared, live *model.RulesetPullRequest) {
	if declared == nil {
		return
	}
	if live == nil {
		live = &model.RulesetPullRequest{}
	}
	const field = "rules.pull_request"
	c.enabled(field, declared.Enabled, live.Enabled)
	if !isTrue(declared.Enabled) || declared.Parameters == nil {
		return
	}
	d := declared.Parameters
	l := live.Parameters
	if l == nil {
		l = &model.RulesetPullRequestParameters{}
	}
	const prefix = field + ".parameters."
	c.count(prefix+"required_approving_review_count", d.RequiredApprovingReviewCount, l.RequiredApprovingReviewCount)
	c.boolean(prefix+"dismiss_stale_reviews_on_push", d.DismissStaleReviewsOnPush, l.DismissStaleReviewsOnPush)
	c.boolean(prefix+"require_code_owner_review", d.RequireCodeOwnerReview, l.RequireCodeOwnerReview)
	c.boolean(prefix+"require_last_push_approval", d.RequireLastPushApproval, l.RequireLastPushApproval)
	c.boolean(prefix+"required_review_thread_resolution", d.RequiredReviewThreadResolution, l.RequiredReviewThreadResolution)
	c.boolean(prefix+"require_extra_approval_for_unattributed_changes", d.RequireExtraApprovalForUnattributedChanges, l.RequireExtraApprovalForUnattributedChanges)
	c.set(prefix+"allowed_merge_methods", d.AllowedMergeMethods, l.AllowedMergeMethods)
}

// compareStatusChecks compares the rule presence and, when the declared
// rule is enabled, its two policy flags and the check list as a set.
func compareStatusChecks(c *rulesetComparer, declared, live *model.RulesetStatusChecks) {
	if declared == nil {
		return
	}
	if live == nil {
		live = &model.RulesetStatusChecks{}
	}
	const field = "rules.required_status_checks"
	c.enabled(field, declared.Enabled, live.Enabled)
	if !isTrue(declared.Enabled) || declared.Parameters == nil {
		return
	}
	d := declared.Parameters
	l := live.Parameters
	if l == nil {
		l = &model.RulesetStatusChecksParameters{}
	}
	const prefix = field + ".parameters."
	c.boolean(prefix+"strict_required_status_checks_policy", d.StrictRequiredStatusChecksPolicy, l.StrictRequiredStatusChecksPolicy)
	c.boolean(prefix+"do_not_enforce_on_create", d.DoNotEnforceOnCreate, l.DoNotEnforceOnCreate)
	if d.RequiredStatusChecks != nil {
		c.set(prefix+"required_status_checks", checkTexts(d.RequiredStatusChecks), checkTexts(l.RequiredStatusChecks))
	}
}

// compareRulesetCodeScanning compares the rule presence and, when the
// declared rule is enabled, the tool list as a set.
func compareRulesetCodeScanning(c *rulesetComparer, declared, live *model.RulesetCodeScanning) {
	if declared == nil {
		return
	}
	if live == nil {
		live = &model.RulesetCodeScanning{}
	}
	const field = "rules.code_scanning"
	c.enabled(field, declared.Enabled, live.Enabled)
	if !isTrue(declared.Enabled) || declared.Parameters == nil || declared.Parameters.CodeScanningTools == nil {
		return
	}
	l := live.Parameters
	if l == nil {
		l = &model.RulesetCodeScanningParameters{}
	}
	c.set(field+".parameters.code_scanning_tools", toolTexts(declared.Parameters.CodeScanningTools), toolTexts(l.CodeScanningTools))
}

func isTrue(p *bool) bool {
	return p != nil && *p
}

// texts renders every item of a list with render, in the declared order,
// so the list compares as a set and displays as declared. A nil list stays
// nil, which means the list is absent.
func texts[T any](items *[]T, render func(T) string) *[]string {
	if items == nil {
		return nil
	}
	out := make([]string, 0, len(*items))
	for _, item := range *items {
		out = append(out, render(item))
	}
	return &out
}

// toolTexts renders code scanning tools as
// "tool (alerts_threshold, security_alerts_threshold)" strings.
func toolTexts(tools *[]model.RulesetCodeScanningTool) *[]string {
	return texts(tools, func(tool model.RulesetCodeScanningTool) string {
		return fmt.Sprintf("%s (%s, %s)", tool.Tool, tool.AlertsThreshold, tool.SecurityAlertsThreshold)
	})
}

// actorTexts renders bypass actors as "Type id (mode)" strings.
func actorTexts(actors *[]model.RulesetBypassActor) *[]string {
	return texts(actors, func(actor model.RulesetBypassActor) string {
		var parts []string
		if actor.ActorType != nil {
			parts = append(parts, *actor.ActorType)
		}
		if actor.ActorID != nil {
			parts = append(parts, strconv.Itoa(*actor.ActorID))
		}
		if actor.BypassMode != nil {
			parts = append(parts, fmt.Sprintf("(%s)", *actor.BypassMode))
		}
		return strings.Join(parts, " ")
	})
}

// checkTexts renders required status checks as "context" or
// "context (integration_id)" strings.
func checkTexts(checks *[]model.RulesetStatusCheck) *[]string {
	return texts(checks, func(check model.RulesetStatusCheck) string {
		if check.IntegrationID != nil {
			return fmt.Sprintf("%s (%d)", check.Context, *check.IntegrationID)
		}
		return check.Context
	})
}

// rulesetFieldOrder keeps ruleset results in config order in the output,
// where other sections sort fields by name.
var rulesetFieldOrder = []string{
	"enforcement",
	"bypass_actors",
	"conditions.ref_name.include",
	"conditions.ref_name.exclude",
	"rules.creation",
	"rules.update",
	"rules.deletion",
	"rules.required_linear_history",
	"rules.required_signatures",
	"rules.non_fast_forward",
	"rules.pull_request",
	"rules.pull_request.parameters.required_approving_review_count",
	"rules.pull_request.parameters.dismiss_stale_reviews_on_push",
	"rules.pull_request.parameters.require_code_owner_review",
	"rules.pull_request.parameters.require_last_push_approval",
	"rules.pull_request.parameters.required_review_thread_resolution",
	"rules.pull_request.parameters.require_extra_approval_for_unattributed_changes",
	"rules.pull_request.parameters.allowed_merge_methods",
	"rules.required_status_checks",
	"rules.required_status_checks.parameters.strict_required_status_checks_policy",
	"rules.required_status_checks.parameters.do_not_enforce_on_create",
	"rules.required_status_checks.parameters.required_status_checks",
	"rules.code_scanning",
	"rules.code_scanning.parameters.code_scanning_tools",
}

// rulesetSortKey returns a zero-padded config position so ruleset fields
// keep config order within their category.
func rulesetSortKey(field string) string {
	index := slices.Index(rulesetFieldOrder, field)
	if index == -1 {
		index = len(rulesetFieldOrder)
	}
	return fmt.Sprintf("%03d %s", index, field)
}
