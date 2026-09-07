package alter

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// actionsFieldGroup identifies the endpoint group an Actions policy field
// belongs to: core fields write through the actions permissions endpoint,
// selected fields through selected-actions, and retention and fork approval
// through their own endpoints.
type actionsFieldGroup int

const (
	actionsCore actionsFieldGroup = iota
	actionsSelected
	actionsRetention
	actionsForkApproval
)

// writeOperation returns the gh write operation kind for the group.
func (g actionsFieldGroup) writeOperation() gh.OperationKind {
	if g == actionsForkApproval {
		return gh.OpSetForkPRContributorApproval
	}
	if g == actionsRetention {
		return gh.OpSetActionsRetention
	}
	if g == actionsSelected {
		return gh.OpSetSelectedActionsPermissions
	}
	return gh.OpSetActionsPermissions
}

// actionsFieldSpec ties one Actions policy field name to its endpoint group
// and the logic that reads the field from an ActionsSettings value.
type actionsFieldSpec struct {
	name    string
	group   actionsFieldGroup
	set     func(*model.ActionsSettings) bool
	compare func(declared, live *model.ActionsSettings) (display string, equal bool)
}

// actionsFieldTable is the single source of Actions field-to-group knowledge.
// Entry order sets the comparison and skip output order.
var actionsFieldTable = []actionsFieldSpec{
	{
		name:  "fork_pr_contributor_approval.approval_policy",
		group: actionsForkApproval,
		set: func(a *model.ActionsSettings) bool {
			return a.ForkPRContributorApproval != nil && a.ForkPRContributorApproval.ApprovalPolicy != nil
		},
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			policy := *declared.ForkPRContributorApproval.ApprovalPolicy
			return policy, live.ForkPRContributorApproval != nil && live.ForkPRContributorApproval.ApprovalPolicy != nil && policy == *live.ForkPRContributorApproval.ApprovalPolicy
		},
	},
	{
		name:  "artifact_and_log_retention.days",
		group: actionsRetention,
		set: func(a *model.ActionsSettings) bool {
			return a.ArtifactAndLogRetention != nil && a.ArtifactAndLogRetention.Days != nil
		},
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			days := *declared.ArtifactAndLogRetention.Days
			return fmt.Sprint(days), live.ArtifactAndLogRetention != nil && live.ArtifactAndLogRetention.Days != nil && days == *live.ArtifactAndLogRetention.Days
		},
	},
	{
		name:  "enabled",
		group: actionsCore,
		set:   func(a *model.ActionsSettings) bool { return a.Enabled != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			return fmt.Sprint(*declared.Enabled), live.Enabled != nil && *declared.Enabled == *live.Enabled
		},
	},
	{
		name:  "allowed_actions",
		group: actionsCore,
		set:   func(a *model.ActionsSettings) bool { return a.AllowedActions != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			return *declared.AllowedActions, live.AllowedActions != nil && *declared.AllowedActions == *live.AllowedActions
		},
	},
	{
		name:  "sha_pinning_required",
		group: actionsCore,
		set:   func(a *model.ActionsSettings) bool { return a.SHAPinningRequired != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			return fmt.Sprint(*declared.SHAPinningRequired), live.SHAPinningRequired != nil && *declared.SHAPinningRequired == *live.SHAPinningRequired
		},
	},
	{
		name:  "github_owned_allowed",
		group: actionsSelected,
		set:   func(a *model.ActionsSettings) bool { return a.GitHubOwnedAllowed != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			return fmt.Sprint(*declared.GitHubOwnedAllowed), live.GitHubOwnedAllowed != nil && *declared.GitHubOwnedAllowed == *live.GitHubOwnedAllowed
		},
	},
	{
		name:  "verified_allowed",
		group: actionsSelected,
		set:   func(a *model.ActionsSettings) bool { return a.VerifiedAllowed != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			return fmt.Sprint(*declared.VerifiedAllowed), live.VerifiedAllowed != nil && *declared.VerifiedAllowed == *live.VerifiedAllowed
		},
	},
	{
		name:  "patterns_allowed",
		group: actionsSelected,
		set:   func(a *model.ActionsSettings) bool { return a.PatternsAllowed != nil },
		compare: func(declared, live *model.ActionsSettings) (string, bool) {
			desired := slices.Clone(*declared.PatternsAllowed)
			slices.Sort(desired)
			equal := live.PatternsAllowed != nil && equalStringSets(desired, *live.PatternsAllowed)
			display := strings.Join(desired, ", ")
			if len(desired) == 0 {
				display = "[]"
			}
			return display, equal
		},
	},
}

// actionsFieldGroupFor looks up the endpoint group for an Actions field name.
func actionsFieldGroupFor(field string) (actionsFieldGroup, bool) {
	for _, spec := range actionsFieldTable {
		if spec.name == field {
			return spec.group, true
		}
	}
	return 0, false
}

// actionsFieldNames returns the field names in the given groups, in table order.
func actionsFieldNames(groups ...actionsFieldGroup) []string {
	var names []string
	for _, spec := range actionsFieldTable {
		if slices.Contains(groups, spec.group) {
			names = append(names, spec.name)
		}
	}
	return names
}

// actionsGroupSet reports whether any field in the group is declared.
func actionsGroupSet(a *model.ActionsSettings, group actionsFieldGroup) bool {
	for _, spec := range actionsFieldTable {
		if spec.group == group && spec.set(a) {
			return true
		}
	}
	return false
}

// ProcessActions compares the declared Actions policy against GitHub and
// applies only endpoint groups that differ.
func ProcessActions(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.Actions == nil || !actionsConfigured(cfg.Actions) {
		return nil, nil
	}
	if !target.HasRepo {
		return nil, nil
	}

	selected := actionsGroupSet(cfg.Actions, actionsSelected)
	live := &model.ActionsSettings{}
	var warnings []error
	if actionsGroupSet(cfg.Actions, actionsCore) || selected {
		var err error
		live, warnings, err = gh.ReadActionsPolicy(target.Client, target.Owner, target.Name, selected)
		if err != nil {
			return nil, err
		}
	}
	var retention *gh.ActionsRetentionResponse
	if actionsGroupSet(cfg.Actions, actionsRetention) {
		var retentionWarnings []error
		var err error
		retention, retentionWarnings, err = gh.ReadActionsRetention(target.Client, target.Owner, target.Name)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, retentionWarnings...)
		if retention != nil {
			if err := retention.ValidateDays(*cfg.Actions.ArtifactAndLogRetention.Days); err != nil {
				return nil, err
			}
			live.ArtifactAndLogRetention = &model.ArtifactAndLogRetentionSettings{Days: retention.Days}
		}
	}
	var approval *gh.ForkPRContributorApprovalResponse
	if actionsGroupSet(cfg.Actions, actionsForkApproval) {
		var approvalWarnings []error
		var err error
		approval, approvalWarnings, err = gh.ReadForkPRContributorApproval(target.Client, target.Owner, target.Name)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, approvalWarnings...)
		if approval != nil {
			live.ForkPRContributorApproval = &model.ForkPRContributorApprovalSettings{ApprovalPolicy: approval.ApprovalPolicy}
		}
	}
	results := compareActions(cfg.Actions, live)
	results = suppressActionsReadWarnings(results, warnings, cfg.Actions, live)

	coreChanged, selectedChanged, retentionChanged, approvalChanged := actionsChanges(results)
	if mode.ShouldWrite() && (coreChanged || selectedChanged) {
		applied, err := gh.ApplyActionsPolicy(target.Client, target.Owner, target.Name, cfg.Actions, live, coreChanged, selectedChanged)
		if err != nil {
			return nil, err
		}
		for _, result := range skippedToResults(applied) {
			result.Section = "actions"
			results = append(results, result)
		}
	}
	if mode.ShouldWrite() && retentionChanged {
		applied, err := gh.ApplyActionsRetention(target.Client, target.Owner, target.Name, *cfg.Actions.ArtifactAndLogRetention.Days, retention)
		if err != nil {
			return nil, err
		}
		for _, result := range skippedToResults(applied) {
			result.Section = "actions"
			results = append(results, result)
		}
	}
	if mode.ShouldWrite() && approvalChanged {
		applied, err := gh.ApplyForkPRContributorApproval(target.Client, target.Owner, target.Name, *cfg.Actions.ForkPRContributorApproval.ApprovalPolicy, approval)
		if err != nil {
			return nil, err
		}
		for _, result := range skippedToResults(applied) {
			result.Section = "actions"
			results = append(results, result)
		}
	}
	return results, nil
}

func actionsConfigured(a *model.ActionsSettings) bool {
	return actionsGroupSet(a, actionsCore) || actionsGroupSet(a, actionsSelected) || actionsGroupSet(a, actionsRetention) || actionsGroupSet(a, actionsForkApproval)
}

func compareActions(declared, live *model.ActionsSettings) []RepoSettingResult {
	var results []RepoSettingResult
	for _, spec := range actionsFieldTable {
		if !spec.set(declared) {
			continue
		}
		display, equal := spec.compare(declared, live)
		category := WouldSet
		if equal {
			category = RepoNoChange
		}
		results = append(results, RepoSettingResult{Section: "actions", Field: spec.name, Category: category, Value: display})
	}
	return results
}

func suppressActionsReadWarnings(results []RepoSettingResult, warnings []error, declared, live *model.ActionsSettings) []RepoSettingResult {
	for _, warning := range warnings {
		var scopeErr *gh.ErrInsufficientScope
		if !errors.As(warning, &scopeErr) {
			continue
		}
		fields := actionsFieldNames(actionsCore)
		switch scopeErr.Operation.Kind {
		case gh.OpFetchForkPRContributorApproval:
			fields = actionsFieldNames(actionsForkApproval)
		case gh.OpFetchActionsRetention:
			fields = actionsFieldNames(actionsRetention)
		case gh.OpFetchActionsPermissions:
			fields = actionsFieldNames(actionsCore, actionsSelected)
		case gh.OpFetchSelectedActionsPermissions:
			fields = actionsFieldNames(actionsSelected)
			for _, result := range results {
				if actionsCoreBroadening(result, declared, live) {
					fields = append(fields, result.Field)
				}
			}
		}
		for _, field := range fields {
			results = replaceWithScopeSkip(results, "actions", field)
		}
	}
	return results
}

func actionsCoreBroadening(result RepoSettingResult, declared, live *model.ActionsSettings) bool {
	if result.Category != WouldSet {
		return false
	}
	switch result.Field {
	case "enabled":
		return declared.Enabled != nil && *declared.Enabled && live.Enabled != nil && !*live.Enabled
	case "sha_pinning_required":
		return declared.SHAPinningRequired != nil && !*declared.SHAPinningRequired &&
			live.SHAPinningRequired != nil && *live.SHAPinningRequired
	default:
		return false
	}
}

func actionsChanges(results []RepoSettingResult) (core, selected, retention, approval bool) {
	for _, result := range results {
		if result.Category != WouldSet {
			continue
		}
		group, ok := actionsFieldGroupFor(result.Field)
		if !ok {
			continue
		}
		switch group {
		case actionsCore:
			core = true
		case actionsSelected:
			selected = true
		case actionsRetention:
			retention = true
		case actionsForkApproval:
			approval = true
		}
	}
	return core, selected, retention, approval
}
