package alter

import (
	"errors"
	"strings"
)

const skipDependabotInactive SwatchReason = "not configured or mode never"

func dependabotResult(plan dependabotPlan) SwatchResult {
	result := SwatchResult{Path: dependabotPath}
	switch plan.Outcome {
	case dependabotOutcomeSkip:
		result.Category = Skipped
		result.Reason = skipDependabotInactive
	case dependabotOutcomePreserve:
		result.Category = Skipped
		result.Reason = ManagedNotOwned
	case dependabotOutcomeUnchanged:
		result.Category = NoChange
	case dependabotOutcomeCreate:
		result.Category = WouldCopy
	case dependabotOutcomeReplace:
		result.Category = WouldOverwrite
	default:
		result.Category = ManagedConflict
		result.Reason = "the Dependabot plan has an invalid outcome"
	}
	return result
}

func dependabotConflictResult(err error) (SwatchResult, bool) {
	var conflict *dependabotConflictError
	if !errors.As(err, &conflict) {
		return SwatchResult{}, false
	}
	return SwatchResult{Path: dependabotPath, Category: ManagedConflict, Reason: SwatchReason(conflict.Reason)}, true
}

func appendDependabotReporting(report *Report, plan dependabotPlan, results []SwatchResult) {
	var result *SwatchResult
	for i := range results {
		if results[i].Path == dependabotPath {
			result = &results[i]
			break
		}
	}
	if result == nil {
		return
	}

	var guidance []string
	if plan.GoKnown && (result.Category == WouldCopy || result.Category == WouldOverwrite || result.Category == NoChange) {
		ecosystems := "`github-actions` and `nix`"
		if plan.GoEnabled {
			ecosystems = "`github-actions`, `gomod`, and `nix`"
		}
		guidance = append(guidance, "The selected Dependabot ecosystems are "+ecosystems+".")
	}
	if result.Category == WouldOverwrite {
		guidance = append(guidance, "Tailor replaces the complete Dependabot file. Custom schedules, groups, registries, comments, and other custom fields are not retained.")
	}
	if result.Category == Skipped && result.Reason == ManagedNotOwned {
		guidance = append(guidance,
			"Back up `.github/dependabot.yml` in Git or another user-controlled location before adoption.",
			"Review both supported Dependabot templates and every custom field that complete-file replacement will discard.",
			"Keep the file unmarked if any custom field must remain. For adoption, set `languages.go` explicitly to `true` or `false`, then add the exact Tailor marker.",
			"Resolve any `.github/dependabot.yaml` entry before adoption.",
			"Run `tailor baste`, review the complete preview, and run `tailor alter` only after approval.",
		)
	}
	if result.Category == ManagedConflict {
		reason := string(result.Reason)
		if strings.Contains(reason, "alternate .github/dependabot.yaml") {
			guidance = append(guidance, "Resolve `.github/dependabot.yaml`, then run `tailor baste` again.")
		}
		if strings.Contains(reason, "does not identify one canonical Go state") {
			guidance = append(guidance, "Set `languages.go` explicitly to `true` or `false`, or remove the Tailor marker, then run `tailor baste` again.")
		}
	}
	if len(guidance) != 0 {
		appendGuidance(report, strings.Join(guidance, "\n")+"\n")
	}
}
