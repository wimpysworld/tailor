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
// selected fields through the selected-actions endpoint.
type actionsFieldGroup int

const (
	actionsCore actionsFieldGroup = iota
	actionsSelected
)

// writeOperation returns the gh write operation kind for the group.
func (g actionsFieldGroup) writeOperation() gh.OperationKind {
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
			equal := false
			if live.PatternsAllowed != nil {
				actual := slices.Clone(*live.PatternsAllowed)
				slices.Sort(actual)
				equal = slices.Equal(desired, actual)
			}
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
	live, warnings, err := gh.ReadActionsPolicy(target.Client, target.Owner, target.Name, selected)
	if err != nil {
		return nil, err
	}
	results := compareActions(cfg.Actions, live)
	results = suppressActionsReadWarnings(results, warnings, cfg.Actions, live)

	coreChanged, selectedChanged := actionsChanges(results)
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
	return results, nil
}

func actionsConfigured(a *model.ActionsSettings) bool {
	return actionsGroupSet(a, actionsCore) || actionsGroupSet(a, actionsSelected)
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
			index := slices.IndexFunc(results, func(result RepoSettingResult) bool {
				return result.Field == field
			})
			if index == -1 {
				continue
			}
			results = slices.Delete(results, index, index+1)
			results = append(results, RepoSettingResult{Section: "actions", Field: field, Category: WouldSkipScope, Annotation: skipAnnotation})
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

func actionsChanges(results []RepoSettingResult) (core, selected bool) {
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
		}
	}
	return core, selected
}
