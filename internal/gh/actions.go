package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
)

type actionsPermissionsResponse struct {
	Enabled            bool   `json:"enabled"`
	AllowedActions     string `json:"allowed_actions"`
	SHAPinningRequired bool   `json:"sha_pinning_required"`
}

type selectedActionsResponse struct {
	GitHubOwnedAllowed bool     `json:"github_owned_allowed"`
	VerifiedAllowed    bool     `json:"verified_allowed"`
	PatternsAllowed    []string `json:"patterns_allowed"`
}

// ReadActionsPolicy reads the configured repository Actions policy endpoints.
func ReadActionsPolicy(client *api.RESTClient, owner, name string, selected bool) (*model.ActionsSettings, []error, error) {
	base := fmt.Sprintf("repos/%s/%s/actions/permissions", owner, name)
	settings := &model.ActionsSettings{}
	var warnings []error

	var permissions actionsPermissionsResponse
	coreKnown := false
	if err := client.Get(base, &permissions); err != nil {
		classified := classifyHTTPError(err, "fetch actions permissions")
		if isAccessError(classified) {
			warnings = append(warnings, classified)
		} else {
			return nil, nil, fmt.Errorf("fetching actions permissions: %w", err)
		}
	} else {
		coreKnown = true
		settings.Enabled = ptr.Ptr(permissions.Enabled)
		settings.AllowedActions = ptr.Ptr(permissions.AllowedActions)
		settings.SHAPinningRequired = ptr.Ptr(permissions.SHAPinningRequired)
	}

	// GitHub rejects the selected-actions read while another policy is active.
	// Leave those live values unknown so apply can use the disabled transition.
	if selected && coreKnown && permissions.AllowedActions == "selected" {
		var policy selectedActionsResponse
		if err := client.Get(base+"/selected-actions", &policy); err != nil {
			classified := classifyHTTPError(err, "fetch selected actions permissions")
			if isAccessError(classified) {
				warnings = append(warnings, classified)
			} else {
				return nil, nil, fmt.Errorf("fetching selected actions permissions: %w", err)
			}
		} else {
			settings.GitHubOwnedAllowed = ptr.Ptr(policy.GitHubOwnedAllowed)
			settings.VerifiedAllowed = ptr.Ptr(policy.VerifiedAllowed)
			settings.PatternsAllowed = &policy.PatternsAllowed
		}
	}
	return settings, warnings, nil
}

// ApplyActionsPolicy writes only the Actions policy endpoint groups that differ.
func ApplyActionsPolicy(client *api.RESTClient, owner, name string, desired, current *model.ActionsSettings, core, selected bool) (*ApplyResult, error) {
	base := fmt.Sprintf("repos/%s/%s/actions/permissions", owner, name)
	result := &ApplyResult{}
	coreBody, err := actionsCoreBody(desired, current)
	if err != nil {
		return nil, err
	}
	selectedBody := actionsSelectedBody(desired)
	desiredSelected := desired.AllowedActions != nil && *desired.AllowedActions == "selected" ||
		desired.AllowedActions == nil && current.AllowedActions != nil && *current.AllowedActions == "selected"
	policyTransition := desiredSelected && selected && current.AllowedActions != nil && *current.AllowedActions != "selected"
	orderedUpdate := desiredSelected && selected && core
	if orderedUpdate {
		actionsDisabled := current.Enabled != nil && !*current.Enabled
		disableBeforeUpdate := current.Enabled != nil && *current.Enabled &&
			desired.Enabled != nil && !*desired.Enabled
		failClosedUpdate := current.Enabled != nil && *current.Enabled &&
			selectedPolicyBroadens(desired, current)
		if policyTransition {
			applied, err := applyActionsWrite(client, base, map[string]any{
				"enabled":         false,
				"allowed_actions": "selected",
			}, "disable actions for selected policy transition", result)
			if err != nil {
				return result, err
			}
			if !applied {
				appendSkippedDependency(result, "set selected actions permissions")
				appendSkippedDependency(result, "set actions permissions")
				return result, nil
			}
			actionsDisabled = true
		} else if disableBeforeUpdate || failClosedUpdate {
			applied, err := applyActionsWrite(client, base, map[string]any{"enabled": false}, "disable actions for selected policy update", result)
			if err != nil {
				return result, err
			}
			if !applied {
				appendSkippedDependency(result, "set selected actions permissions")
				appendSkippedDependency(result, "set actions permissions")
				return result, nil
			}
			actionsDisabled = true
		}
		if err := putActionsPolicy(client, base+"/selected-actions", selectedBody); err != nil {
			if actionsDisabled {
				return nil, fmt.Errorf("setting selected actions permissions failed while actions are disabled: %w", err)
			}
			classified := classifyHTTPError(err, "set selected actions permissions")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "set selected actions permissions", Err: classified})
				appendSkippedDependency(result, "set actions permissions")
				return result, nil
			}
			return nil, fmt.Errorf("setting selected actions permissions: %w", err)
		}
		if actionsDisabled {
			if err := putActionsPolicy(client, base, coreBody); err != nil {
				return nil, fmt.Errorf("setting final actions permissions failed while actions are disabled: %w", err)
			}
			return result, nil
		}
		_, err := applyActionsWrite(client, base, coreBody, "set actions permissions", result)
		return result, err
	}

	coreApplied := true
	if core {
		coreApplied, err = applyActionsWrite(client, base, coreBody, "set actions permissions", result)
		if err != nil {
			return nil, err
		}
		if !coreApplied && selected {
			appendSkippedDependency(result, "set selected actions permissions")
		}
	}
	if selected && coreApplied {
		if err := putActionsPolicy(client, base+"/selected-actions", selectedBody); err != nil {
			classified := classifyHTTPError(err, "set selected actions permissions")
			if isAccessError(classified) {
				result.Skipped = append(result.Skipped, SkippedOperation{Operation: "set selected actions permissions", Err: classified})
			} else {
				return nil, fmt.Errorf("setting selected actions permissions: %w", err)
			}
		}
	}
	return result, nil
}

func selectedPolicyBroadens(desired, current *model.ActionsSettings) bool {
	if desired.GitHubOwnedAllowed != nil && *desired.GitHubOwnedAllowed &&
		current.GitHubOwnedAllowed != nil && !*current.GitHubOwnedAllowed {
		return true
	}
	if desired.VerifiedAllowed != nil && *desired.VerifiedAllowed &&
		current.VerifiedAllowed != nil && !*current.VerifiedAllowed {
		return true
	}
	if desired.PatternsAllowed == nil || current.PatternsAllowed == nil {
		return false
	}

	desiredPatterns := make(map[string]bool, len(*desired.PatternsAllowed))
	for _, pattern := range *desired.PatternsAllowed {
		desiredPatterns[pattern] = true
	}
	currentPatterns := make(map[string]bool, len(*current.PatternsAllowed))
	for _, pattern := range *current.PatternsAllowed {
		currentPatterns[pattern] = true
	}
	for pattern := range desiredPatterns {
		if !strings.HasPrefix(pattern, "!") && !currentPatterns[pattern] {
			return true
		}
	}
	for pattern := range currentPatterns {
		if strings.HasPrefix(pattern, "!") && !desiredPatterns[pattern] {
			return true
		}
	}
	return false
}

func actionsCoreBody(desired, current *model.ActionsSettings) (map[string]any, error) {
	enabled := desired.Enabled
	if enabled == nil {
		enabled = current.Enabled
	}
	if enabled == nil {
		return nil, fmt.Errorf("current actions enabled state is unknown")
	}
	body := map[string]any{"enabled": *enabled}
	if desired.AllowedActions != nil {
		body["allowed_actions"] = *desired.AllowedActions
	}
	if desired.SHAPinningRequired != nil {
		body["sha_pinning_required"] = *desired.SHAPinningRequired
	}
	return body, nil
}

func actionsSelectedBody(desired *model.ActionsSettings) map[string]any {
	body := make(map[string]any)
	if desired.GitHubOwnedAllowed != nil {
		body["github_owned_allowed"] = *desired.GitHubOwnedAllowed
	}
	if desired.VerifiedAllowed != nil {
		body["verified_allowed"] = *desired.VerifiedAllowed
	}
	if desired.PatternsAllowed != nil {
		patterns := slices.Clone(*desired.PatternsAllowed)
		slices.Sort(patterns)
		body["patterns_allowed"] = patterns
	}
	return body
}

func applyActionsWrite(client *api.RESTClient, path string, body map[string]any, operation string, result *ApplyResult) (bool, error) {
	if err := putActionsPolicy(client, path, body); err != nil {
		classified := classifyHTTPError(err, operation)
		if isAccessError(classified) {
			result.Skipped = append(result.Skipped, SkippedOperation{Operation: operation, Err: classified})
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func putActionsPolicy(client *api.RESTClient, path string, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling actions permissions: %w", err)
	}
	return client.Put(path, bytes.NewReader(payload), nil)
}
