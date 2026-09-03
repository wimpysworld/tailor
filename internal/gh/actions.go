package gh

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
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
	if err := boundedHTTPError(client.Get(base, &permissions)); err != nil {
		if err := collectAccessWarning(err, Op(OpFetchActionsPermissions), "fetching actions permissions", &warnings); err != nil {
			return nil, nil, err
		}
	} else {
		coreKnown = true
		settings.Enabled = new(permissions.Enabled)
		settings.AllowedActions = new(permissions.AllowedActions)
		settings.SHAPinningRequired = new(permissions.SHAPinningRequired)
	}

	// GitHub rejects the selected-actions read while another policy is active.
	// Leave the live selected values unknown so ApplyActionsPolicy takes the
	// policy transition path.
	if selected && coreKnown && permissions.AllowedActions == "selected" {
		var policy selectedActionsResponse
		if err := boundedHTTPError(client.Get(base+"/selected-actions", &policy)); err != nil {
			if err := collectAccessWarning(err, Op(OpFetchSelectedActionsPermissions), "fetching selected actions permissions", &warnings); err != nil {
				return nil, nil, err
			}
		} else {
			settings.GitHubOwnedAllowed = new(policy.GitHubOwnedAllowed)
			settings.VerifiedAllowed = new(policy.VerifiedAllowed)
			settings.PatternsAllowed = &policy.PatternsAllowed
		}
	}
	return settings, warnings, nil
}

// actionsWriteOrder names the write-ordering strategy for one Actions policy
// update. The fail-closed orderings keep a partial write from widening the
// policy while Actions can run.
type actionsWriteOrder int

const (
	// coreThenSelected writes the core policy, then the selected policy, with
	// no fail-closed ordering.
	coreThenSelected actionsWriteOrder = iota
	// restrictAllThenSelected narrows an enabled "all" policy to "selected"
	// through the core endpoint before the selected policy exists.
	restrictAllThenSelected
	// selectedThenCore writes the selected policy before the final core
	// policy, disabling Actions first when the update could widen access.
	selectedThenCore
)

// ApplyActionsPolicy writes only the Actions policy endpoint groups that differ.
func ApplyActionsPolicy(client *api.RESTClient, owner, name string, desired, current *model.ActionsSettings, core, selected bool) (*ApplyResult, error) {
	base := fmt.Sprintf("repos/%s/%s/actions/permissions", owner, name)
	result := &ApplyResult{}
	coreBody, err := actionsCoreBody(desired, current)
	if err != nil {
		return nil, err
	}
	selectedBody := actionsSelectedBody(desired)
	switch planActionsWriteOrder(desired, current, core, selected, coreBody) {
	case restrictAllThenSelected:
		return applyRestrictAllThenSelected(client, base, desired, current, coreBody, selectedBody, result)
	case selectedThenCore:
		return applySelectedThenCore(client, base, desired, current, coreBody, selectedBody, result)
	default:
		return applyCoreThenSelected(client, base, coreBody, selectedBody, core, selected, result)
	}
}

// planActionsWriteOrder picks the write ordering. Fail-closed orderings apply
// only when both endpoint groups are written and the target policy, desired or
// carried over from current, is "selected".
func planActionsWriteOrder(desired, current *model.ActionsSettings, core, selected bool, coreBody map[string]any) actionsWriteOrder {
	targetsSelected := strEq(desired.AllowedActions, "selected") ||
		desired.AllowedActions == nil && strEq(current.AllowedActions, "selected")
	if !targetsSelected || !core || !selected {
		return coreThenSelected
	}
	enabledAllPolicy := isTrue(current.Enabled) && strEq(current.AllowedActions, "all")
	if enabledAllPolicy && coreBody["enabled"] == true {
		return restrictAllThenSelected
	}
	return selectedThenCore
}

// applyRestrictAllThenSelected narrows an enabled "all" policy: the core write
// switches to "selected" first, then the selected policy lands. When the
// update also relaxes SHA pinning, the initial core write keeps pinning
// required and a final core write relaxes it after the selected policy exists.
func applyRestrictAllThenSelected(client *api.RESTClient, base string, desired, current *model.ActionsSettings, coreBody, selectedBody map[string]any, result *ApplyResult) (*ApplyResult, error) {
	initialCoreBody := coreBody
	relaxesSHAPinning := isTrue(current.SHAPinningRequired) && isFalse(desired.SHAPinningRequired)
	if relaxesSHAPinning {
		initialCoreBody = maps.Clone(coreBody)
		initialCoreBody["sha_pinning_required"] = true
	}
	applied, err := applyActionsWrite(client, base, initialCoreBody, Op(OpSetActionsPermissions), result)
	if err != nil {
		return nil, err
	}
	if !applied {
		appendSkippedDependency(result, Op(OpSetSelectedActionsPermissions))
		return result, nil
	}
	if err := putActionsPolicy(client, base+"/selected-actions", selectedBody); err != nil {
		return nil, fmt.Errorf("setting selected actions permissions failed after actions were restricted to selected: %w", err)
	}
	if relaxesSHAPinning {
		if err := putActionsPolicy(client, base, coreBody); err != nil {
			return nil, fmt.Errorf("relaxing SHA pinning failed after selected actions restrictions were applied: %w", err)
		}
	}
	return result, nil
}

// disableActionsStep returns the pre-update disable write and its operation,
// or nil when the selected policy can change while Actions keep their
// current state.
func disableActionsStep(desired, current *model.ActionsSettings) (map[string]any, Operation) {
	if current.AllowedActions != nil && *current.AllowedActions != "selected" {
		return map[string]any{
			"enabled":         false,
			"allowed_actions": "selected",
		}, Op(OpDisableActionsForPolicyTransition)
	}
	if isTrue(current.Enabled) && (isFalse(desired.Enabled) || selectedPolicyBroadens(desired, current)) {
		return map[string]any{"enabled": false}, Op(OpDisableActionsForPolicyUpdate)
	}
	return nil, Operation{}
}

// applySelectedThenCore writes the selected policy before the final core
// policy. When the update could widen access, a disable write runs first so a
// partial failure leaves Actions disabled rather than over-permitted.
func applySelectedThenCore(client *api.RESTClient, base string, desired, current *model.ActionsSettings, coreBody, selectedBody map[string]any, result *ApplyResult) (*ApplyResult, error) {
	actionsDisabled := isFalse(current.Enabled)
	disabledByTailor := false
	if disableBody, disableOp := disableActionsStep(desired, current); disableBody != nil {
		applied, err := applyActionsWrite(client, base, disableBody, disableOp, result)
		if err != nil {
			return result, err
		}
		if !applied {
			appendSkippedDependency(result, Op(OpSetSelectedActionsPermissions))
			appendSkippedDependency(result, Op(OpSetActionsPermissions))
			return result, nil
		}
		actionsDisabled = true
		disabledByTailor = true
	}
	if err := putActionsPolicy(client, base+"/selected-actions", selectedBody); err != nil {
		if disabledByTailor {
			return nil, fmt.Errorf("setting selected actions permissions failed while actions are disabled: %w", err)
		}
		if recordAccessError(result, Op(OpSetSelectedActionsPermissions), err) {
			appendSkippedDependency(result, Op(OpSetActionsPermissions))
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
	_, err := applyActionsWrite(client, base, coreBody, Op(OpSetActionsPermissions), result)
	return result, err
}

// applyCoreThenSelected writes the requested endpoint groups in the plain
// order: core first, then the selected policy if the core write applied.
func applyCoreThenSelected(client *api.RESTClient, base string, coreBody, selectedBody map[string]any, core, selected bool, result *ApplyResult) (*ApplyResult, error) {
	coreApplied := true
	if core {
		var err error
		coreApplied, err = applyActionsWrite(client, base, coreBody, Op(OpSetActionsPermissions), result)
		if err != nil {
			return nil, err
		}
		if !coreApplied && selected {
			appendSkippedDependency(result, Op(OpSetSelectedActionsPermissions))
		}
	}
	if selected && coreApplied {
		if err := putActionsPolicy(client, base+"/selected-actions", selectedBody); err != nil {
			if !recordAccessError(result, Op(OpSetSelectedActionsPermissions), err) {
				return nil, fmt.Errorf("setting selected actions permissions: %w", err)
			}
		}
	}
	return result, nil
}

// selectedPolicyBroadens reports whether the desired selected-actions policy
// allows more than the current one, including via a removed or narrowed
// exclusion pattern. ApplyActionsPolicy disables Actions before a broadening
// update so a partial write cannot widen the policy while Actions run.
func selectedPolicyBroadens(desired, current *model.ActionsSettings) bool {
	if isTrue(desired.GitHubOwnedAllowed) && isFalse(current.GitHubOwnedAllowed) {
		return true
	}
	if isTrue(desired.VerifiedAllowed) && isFalse(current.VerifiedAllowed) {
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
		if strings.HasPrefix(pattern, "!") && !exclusionCovered(pattern, desiredPatterns) {
			return true
		}
	}
	return false
}

// exclusionCovered reports whether a current exclusion pattern stays excluded
// under desired, either exactly or through a broader "!prefix*" pattern.
func exclusionCovered(current string, desired map[string]bool) bool {
	if desired[current] {
		return true
	}
	for pattern := range desired {
		if strings.HasPrefix(pattern, "!") && strings.HasSuffix(pattern, "*") &&
			strings.HasPrefix(strings.TrimPrefix(current, "!"), strings.TrimSuffix(strings.TrimPrefix(pattern, "!"), "*")) {
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

func applyActionsWrite(client *api.RESTClient, path string, body map[string]any, operation Operation, result *ApplyResult) (bool, error) {
	if err := putActionsPolicy(client, path, body); err != nil {
		if recordAccessError(result, operation, err) {
			return false, nil
		}
		return false, fmt.Errorf("%s: %w", operation, err)
	}
	return true, nil
}

func putActionsPolicy(client *api.RESTClient, path string, body map[string]any) error {
	return sendJSON(client, http.MethodPut, path, body)
}
