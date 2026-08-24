package alter

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// defaultLabelWidth is the minimum column width for status labels in formatted
// output. It fits the longest fixed label, "would skip (insufficient scope):",
// with padding.
const defaultLabelWidth = 37

// FormatOutput produces the alter command output from repo settings results,
// label results, and swatch results (including licence).
func FormatOutput(repoResults []RepoSettingResult, labelResults []LabelResult, swatchResults []SwatchResult, mode ApplyMode) string {
	if len(repoResults) == 0 && len(labelResults) == 0 && len(swatchResults) == 0 {
		return ""
	}
	if mode.ShouldWrite() {
		repoResults = removeSkippedRepoResults(repoResults)
		labelResults = removeSkippedLabelResults(labelResults)
	}

	sortedSwatches := sortSwatchResults(swatchResults)
	width := labelWidth(repoResults, labelResults, sortedSwatches, mode)

	var b strings.Builder

	for _, r := range sortRepoResults(repoResults) {
		label := repoLabel(r, mode)
		section := r.Section
		if section == "" {
			section = "repository"
		}
		switch r.Category {
		case WouldSet:
			fmt.Fprintf(&b, "%-*s%s.%s = %s\n", width, label, section, r.Field, r.Value)
		case RepoNoChange:
			fmt.Fprintf(&b, "%-*s%s.%s (already %s)\n", width, label, section, r.Field, r.Value)
		case WouldSkipScope:
			if r.Section == "actions" && isActionsPolicyField(r.Field) {
				fmt.Fprintf(&b, "%-*sactions.%s\n", width, label, r.Field)
			} else {
				fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Field)
			}
		}
	}

	for _, r := range sortLabelResults(labelResults) {
		label := labelResultLabel(r, mode)
		switch r.Category {
		case WouldCreate, WouldUpdate:
			fmt.Fprintf(&b, "%-*slabel.%s = %s\n", width, label, r.Name, r.Value)
		case LabelNoChange:
			fmt.Fprintf(&b, "%-*slabel.%s (already %s)\n", width, label, r.Name, r.Value)
		case LabelSkipScope:
			fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Name)
		}
	}

	for _, r := range sortedSwatches {
		label := swatchLabel(r, mode)
		fmt.Fprintf(&b, "%-*s%s%s\n", width, label, r.Path, swatchReason(r))
	}

	return b.String()
}

func isActionsPolicyField(field string) bool {
	switch field {
	case "enabled", "allowed_actions", "sha_pinning_required", "github_owned_allowed", "verified_allowed", "patterns_allowed":
		return true
	default:
		return false
	}
}

func removeSkippedRepoResults(results []RepoSettingResult) []RepoSettingResult {
	skipped := make(map[string]bool)
	for _, result := range results {
		if result.Category == WouldSkipScope {
			skipped[result.Field] = true
		}
	}
	if len(skipped) == 0 {
		return results
	}

	filtered := make([]RepoSettingResult, 0, len(results))
	for _, result := range results {
		if result.Category != WouldSet || !skipped[repoSettingOperation(result)] {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func repoSettingOperation(result RepoSettingResult) string {
	if result.Section == "actions" {
		switch result.Field {
		case "enabled", "allowed_actions", "sha_pinning_required":
			return "set actions permissions"
		case "github_owned_allowed", "verified_allowed", "patterns_allowed":
			return "set selected actions permissions"
		}
	}
	switch result.Field {
	case "private_vulnerability_reporting_enabled":
		return operationForValue(result.Value, "private vulnerability reporting")
	case "vulnerability_alerts_enabled":
		return operationForValue(result.Value, "vulnerability alerts")
	case "automated_security_fixes_enabled":
		return operationForValue(result.Value, "automated security fixes")
	case "topics":
		return "set topics"
	case "default_workflow_permissions", "can_approve_pull_request_reviews":
		return "set workflow permissions"
	default:
		return "patch repo settings"
	}
}

func operationForValue(value, feature string) string {
	if value == "true" {
		return "enable " + feature
	}
	return "disable " + feature
}

func removeSkippedLabelResults(results []LabelResult) []LabelResult {
	skipped := make(map[string]bool)
	for _, result := range results {
		if result.Category == LabelSkipScope {
			skipped[result.Name] = true
		}
	}
	if len(skipped) == 0 {
		return results
	}

	filtered := make([]LabelResult, 0, len(results))
	for _, result := range results {
		operation := ""
		switch result.Category {
		case WouldCreate:
			operation = fmt.Sprintf("create label %q", result.Name)
		case WouldUpdate:
			operation = fmt.Sprintf("update label %q", result.Name)
		}
		if !skipped[operation] {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// formatAnnotatedLabel embeds an annotation into a skip-category label when
// isSkip is true and annotation is non-empty. For example:
// "would skip (insufficient scope: token missing required scope):".
func formatAnnotatedLabel(category, annotation string, isSkip bool) string {
	if annotation != "" && isSkip {
		base := strings.TrimSuffix(category, ")")
		return base + ": " + annotation + "):"
	}
	return category + ":"
}

func outputCategory(category string, mode ApplyMode) string {
	if !mode.ShouldWrite() {
		return category
	}

	switch category {
	case "would set":
		return "set"
	case "would create":
		return "created"
	case "would update":
		return "updated"
	case "would copy":
		return "copied"
	case "would overwrite":
		return "overwritten"
	case "would remove":
		return "removed"
	default:
		return category
	}
}

// repoLabel returns the formatted label for a repo setting result.
func repoLabel(r RepoSettingResult, mode ApplyMode) string {
	isSkip := r.Category == WouldSkipScope
	return formatAnnotatedLabel(outputCategory(string(r.Category), mode), r.Annotation, isSkip)
}

// labelResultLabel returns the formatted label for a label result.
func labelResultLabel(r LabelResult, mode ApplyMode) string {
	isSkip := r.Category == LabelSkipScope
	return formatAnnotatedLabel(outputCategory(string(r.Category), mode), r.Annotation, isSkip)
}

func swatchLabel(r SwatchResult, mode ApplyMode) string {
	return outputCategory(string(r.Category), mode) + ":"
}

func swatchReason(r SwatchResult) string {
	if r.Reason == "" {
		return ""
	}
	return " (" + string(r.Reason) + ")"
}

// labelWidth computes the column width needed to accommodate all labels. It
// returns at least defaultLabelWidth, widening if any annotated label exceeds
// that.
func labelWidth(repos []RepoSettingResult, labels []LabelResult, swatches []SwatchResult, mode ApplyMode) int {
	width := defaultLabelWidth
	for _, r := range repos {
		if w := len(repoLabel(r, mode)) + 1; w > width {
			width = w
		}
	}
	for _, r := range labels {
		if w := len(labelResultLabel(r, mode)) + 1; w > width {
			width = w
		}
	}
	for _, r := range swatches {
		if w := len(swatchLabel(r, mode)) + 1; w > width {
			width = w
		}
	}
	return width
}

// sortRepoResults returns a sorted copy: actionable (WouldSet) before
// informational (RepoNoChange), lexicographic by field within each group.
func sortRepoResults(results []RepoSettingResult) []RepoSettingResult {
	return slices.SortedStableFunc(slices.Values(results), func(a, b RepoSettingResult) int {
		if c := cmp.Compare(repoOrder(a.Category), repoOrder(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.Field, b.Field)
	})
}

// repoOrder returns the sort priority for a RepoSettingCategory.
func repoOrder(c RepoSettingCategory) int {
	switch c {
	case WouldSet:
		return 0
	case RepoNoChange:
		return 1
	case WouldSkipScope:
		return 2
	default:
		return 3
	}
}

// sortSwatchResults returns a sorted copy with actionable results before
// informational results and paths sorted within each category.
func sortSwatchResults(results []SwatchResult) []SwatchResult {
	return slices.SortedStableFunc(slices.Values(results), func(a, b SwatchResult) int {
		if c := cmp.Compare(swatchOrder(a), swatchOrder(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})
}

// sortLabelResults returns a sorted copy: actionable (WouldCreate, WouldUpdate)
// before informational (LabelNoChange), lexicographic by name within each group.
func sortLabelResults(results []LabelResult) []LabelResult {
	return slices.SortedStableFunc(slices.Values(results), func(a, b LabelResult) int {
		if c := cmp.Compare(labelOrder(a.Category), labelOrder(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

// labelOrder returns the sort priority for a LabelCategory.
func labelOrder(c LabelCategory) int {
	switch c {
	case WouldCreate:
		return 0
	case WouldUpdate:
		return 1
	case LabelNoChange:
		return 2
	case LabelSkipScope:
		return 3
	default:
		return 4
	}
}

// swatchOrder returns the sort priority for a SwatchResult.
// Actionable categories sort before informational categories.
func swatchOrder(result SwatchResult) int {
	switch result.Category {
	case WouldUpdateConfig:
		return 0
	case WouldRemove:
		return 1
	case WouldCopy:
		return 2
	case WouldOverwrite:
		return 3
	case NoChange:
		return 4
	case Skipped:
		if result.Reason == SkipFirstFitExists {
			return 5
		}
		return 6
	default:
		return 7
	}
}
