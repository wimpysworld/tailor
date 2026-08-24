package alter

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// ProcessActions compares the declared Actions policy against GitHub and
// applies only endpoint groups that differ.
func ProcessActions(cfg *config.Config, mode ApplyMode, client *api.RESTClient, owner, name string, hasRepo bool) ([]RepoSettingResult, error) {
	if cfg.Actions == nil || !actionsConfigured(cfg.Actions) {
		return nil, nil
	}
	if !hasRepo {
		return nil, nil
	}

	selected := cfg.Actions.GitHubOwnedAllowed != nil || cfg.Actions.VerifiedAllowed != nil || cfg.Actions.PatternsAllowed != nil
	live, warnings, err := gh.ReadActionsPolicy(client, owner, name, selected)
	if err != nil {
		return nil, err
	}
	results := compareActions(cfg.Actions, live)
	results = suppressActionsReadWarnings(results, warnings, cfg.Actions, live)

	coreChanged, selectedChanged := actionsChanges(results)
	if mode.ShouldWrite() && (coreChanged || selectedChanged) {
		applied, err := gh.ApplyActionsPolicy(client, owner, name, cfg.Actions, live, coreChanged, selectedChanged)
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
	return a.Enabled != nil || a.AllowedActions != nil || a.SHAPinningRequired != nil ||
		a.GitHubOwnedAllowed != nil || a.VerifiedAllowed != nil || a.PatternsAllowed != nil
}

func compareActions(declared, live *model.ActionsSettings) []RepoSettingResult {
	var results []RepoSettingResult
	add := func(field, display string, equal bool) {
		category := WouldSet
		if equal {
			category = RepoNoChange
		}
		results = append(results, RepoSettingResult{Section: "actions", Field: field, Category: category, Value: display})
	}
	if declared.Enabled != nil {
		add("enabled", fmt.Sprint(*declared.Enabled), live.Enabled != nil && *declared.Enabled == *live.Enabled)
	}
	if declared.AllowedActions != nil {
		add("allowed_actions", *declared.AllowedActions, live.AllowedActions != nil && *declared.AllowedActions == *live.AllowedActions)
	}
	if declared.SHAPinningRequired != nil {
		add("sha_pinning_required", fmt.Sprint(*declared.SHAPinningRequired), live.SHAPinningRequired != nil && *declared.SHAPinningRequired == *live.SHAPinningRequired)
	}
	if declared.GitHubOwnedAllowed != nil {
		add("github_owned_allowed", fmt.Sprint(*declared.GitHubOwnedAllowed), live.GitHubOwnedAllowed != nil && *declared.GitHubOwnedAllowed == *live.GitHubOwnedAllowed)
	}
	if declared.VerifiedAllowed != nil {
		add("verified_allowed", fmt.Sprint(*declared.VerifiedAllowed), live.VerifiedAllowed != nil && *declared.VerifiedAllowed == *live.VerifiedAllowed)
	}
	if declared.PatternsAllowed != nil {
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
		add("patterns_allowed", display, equal)
	}
	return results
}

func suppressActionsReadWarnings(results []RepoSettingResult, warnings []error, declared, live *model.ActionsSettings) []RepoSettingResult {
	for _, warning := range warnings {
		var scopeErr *gh.ErrInsufficientScope
		if !errors.As(warning, &scopeErr) {
			continue
		}
		fields := []string{"enabled", "allowed_actions", "sha_pinning_required"}
		switch scopeErr.Operation {
		case "fetch actions permissions":
			fields = []string{"enabled", "allowed_actions", "sha_pinning_required", "github_owned_allowed", "verified_allowed", "patterns_allowed"}
		case "fetch selected actions permissions":
			fields = []string{"github_owned_allowed", "verified_allowed", "patterns_allowed"}
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
			results = append(results, RepoSettingResult{Section: "actions", Field: field, Category: WouldSkipScope, Value: warning.Error(), Annotation: skipAnnotation(warning)})
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
		switch result.Field {
		case "enabled", "allowed_actions", "sha_pinning_required":
			core = true
		case "github_owned_allowed", "verified_allowed", "patterns_allowed":
			selected = true
		}
	}
	return core, selected
}
