package alter

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/termtext"
)

// defaultLabelWidth is the minimum column width for status labels in formatted
// output. It fits the longest fixed label, "would skip (insufficient scope):",
// with padding.
const defaultLabelWidth = 37

// outputLine pairs a status label with the text that follows it.
type outputLine struct {
	label string
	text  string
}

// FormatOutput produces the alter command output from repo settings results,
// label results, and swatch results (including licence).
func FormatOutput(repoResults []RepoSettingResult, labelResults []LabelResult, swatchResults []SwatchResult, mode ApplyMode) string {
	if len(repoResults) == 0 && len(labelResults) == 0 && len(swatchResults) == 0 {
		return ""
	}
	if mode.ShouldWrite() {
		repoResults = removeSkipped(repoResults, repoSkippedKind, repoActionKind)
		labelResults = removeSkipped(labelResults, labelSkippedName, labelActionName)
	}

	lines := slices.Concat(repoLines(repoResults, mode), labelLines(labelResults, mode), swatchLines(swatchResults, mode))

	for i := range lines {
		lines[i].label = termtext.EscapeControlText(lines[i].label)
		lines[i].text = termtext.EscapeControlText(lines[i].text)
	}

	width := defaultLabelWidth
	for _, line := range lines {
		if w := len(line.label) + 1; w > width {
			width = w
		}
	}

	var b strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&b, "%-*s%s\n", width, line.label, line.text)
	}
	return b.String()
}

// repoLines renders repo setting results as sorted output lines.
func repoLines(results []RepoSettingResult, mode ApplyMode) []outputLine {
	order := func(r RepoSettingResult) int { return repoOrder(r.Category) }
	lines := make([]outputLine, 0, len(results))
	for _, r := range sortResults(results, order, repoSortKey) {
		section := cmp.Or(r.Section, "repository")
		var text string
		switch r.Category {
		case WouldSet:
			text = fmt.Sprintf("%s.%s = %s", section, r.Field, r.Value)
		case RepoNoChange:
			text = fmt.Sprintf("%s.%s (already %s)", section, r.Field, r.Value)
		case WouldSkipScope:
			switch {
			case r.Field == "":
				text = r.Operation.String()
			case r.Section == "actions" && isActionsPolicyField(r.Field):
				text = "actions." + r.Field
			case r.Section == rulesetSection:
				text = rulesetSection + "." + r.Field
			default:
				text = r.Field
			}
		case WouldSkipSetup:
			text = section + "." + r.Field
		default:
			continue
		}
		label := resultLabel(string(r.Category), r.Annotation, r.Category == WouldSkipScope || r.Category == WouldSkipSetup, mode)
		lines = append(lines, outputLine{label, text})
	}
	return lines
}

// labelLines renders label results as sorted output lines.
func labelLines(results []LabelResult, mode ApplyMode) []outputLine {
	order := func(r LabelResult) int { return labelOrder(r.Category) }
	lines := make([]outputLine, 0, len(results))
	for _, r := range sortResults(results, order, labelSortKey) {
		var text string
		switch r.Category {
		case WouldCreate, WouldUpdate:
			text = fmt.Sprintf("label.%s = %s", r.Name, r.Value)
		case LabelNoChange:
			text = fmt.Sprintf("label.%s (already %s)", r.Name, r.Value)
		case LabelSkipScope:
			text = r.Operation.String()
		default:
			continue
		}
		label := resultLabel(string(r.Category), r.Annotation, r.Category == LabelSkipScope, mode)
		lines = append(lines, outputLine{label, text})
	}
	return lines
}

// swatchLines renders swatch results as sorted output lines.
func swatchLines(results []SwatchResult, mode ApplyMode) []outputLine {
	key := func(r SwatchResult) string { return r.Path }
	lines := make([]outputLine, 0, len(results))
	for _, r := range sortResults(results, swatchOrder, key) {
		label := resultLabel(string(r.Category), "", false, mode)
		lines = append(lines, outputLine{label, r.Path + swatchReason(r)})
	}
	return lines
}

func isActionsPolicyField(field string) bool {
	_, ok := actionsFieldGroupFor(field)
	return ok
}

// removeSkipped drops actionable results whose write operation was reported
// as skipped. skipKey identifies the skipped operation a scope-skip result
// records; actionKey identifies the operation an actionable result would
// perform. Each returns false when the result is not of its kind.
func removeSkipped[T any, K comparable](results []T, skipKey, actionKey func(T) (K, bool)) []T {
	skipped := make(map[K]bool)
	for _, result := range results {
		if key, ok := skipKey(result); ok {
			skipped[key] = true
		}
	}
	if len(skipped) == 0 {
		return results
	}

	filtered := make([]T, 0, len(results))
	for _, result := range results {
		if key, ok := actionKey(result); !ok || !skipped[key] {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// repoSkippedKind returns the operation kind a scope-skip result records.
func repoSkippedKind(r RepoSettingResult) (gh.OperationKind, bool) {
	return r.Operation.Kind, r.Category == WouldSkipScope && r.Operation.Kind != gh.OpNone
}

// repoActionKind returns the kind of the write operation that applies an
// actionable result's field.
func repoActionKind(r RepoSettingResult) (gh.OperationKind, bool) {
	if r.Category != WouldSet {
		return gh.OpNone, false
	}
	if r.Section == "actions" {
		if group, ok := actionsFieldGroupFor(r.Field); ok {
			return group.writeOperation(), true
		}
	}
	switch r.Section {
	case "code_scanning":
		return gh.OpSetCodeScanningSetup, true
	case "code_quality":
		return gh.OpSetCodeQualitySetup, true
	case rulesetSection:
		return gh.OpSetRuleset, true
	}
	switch r.Field {
	case "private_vulnerability_reporting_enabled":
		return gh.OpSetPrivateVulnerabilityReporting, true
	case "vulnerability_alerts_enabled":
		return gh.OpSetVulnerabilityAlerts, true
	case "automated_security_fixes_enabled":
		return gh.OpSetAutomatedSecurityFixes, true
	case "topics":
		return gh.OpSetTopics, true
	case "default_workflow_permissions", "can_approve_pull_request_reviews":
		return gh.OpSetWorkflowPermissions, true
	default:
		return gh.OpPatchRepoSettings, true
	}
}

// labelSkippedName returns the label name a scope-skip result records.
func labelSkippedName(r LabelResult) (string, bool) {
	return r.Operation.Label, r.Category == LabelSkipScope
}

// labelActionName returns the label name an actionable result would write.
func labelActionName(r LabelResult) (string, bool) {
	return r.Name, r.Category == WouldCreate || r.Category == WouldUpdate
}

// resultLabel formats a status label, translating dry-run categories to
// write-mode wording and embedding a skip annotation when present. For
// example: "would skip (insufficient scope: token missing required scope):"
// or "would skip (not available):".
func resultLabel(category, annotation string, isSkip bool, mode ApplyMode) string {
	category = outputCategory(category, mode)
	if annotation != "" && isSkip {
		if base, ok := strings.CutSuffix(category, ")"); ok {
			return base + ": " + annotation + "):"
		}
		return category + " (" + annotation + "):"
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

func swatchReason(r SwatchResult) string {
	if r.Reason == "" {
		return ""
	}
	return " (" + string(r.Reason) + ")"
}

// sortResults returns a sorted copy: actionable categories before
// informational, lexicographic by sort key within each priority.
func sortResults[T any](results []T, order func(T) int, key func(T) string) []T {
	return slices.SortedStableFunc(slices.Values(results), func(a, b T) int {
		if c := cmp.Compare(order(a), order(b)); c != 0 {
			return c
		}
		return cmp.Compare(key(a), key(b))
	})
}

// repoSortKey returns the field name, or the skipped operation text for
// write-skip results, which have no field name. Ruleset results share the
// empty key: the stable sort keeps their emission order, which is config
// order, and the empty key sorts them before every other section inside a
// category.
func repoSortKey(r RepoSettingResult) string {
	if r.Section == rulesetSection {
		return ""
	}
	if r.Field != "" {
		return r.Field
	}
	return r.Operation.String()
}

// repoOrder returns the sort priority for a RepoSettingCategory.
func repoOrder(c RepoSettingCategory) int {
	switch c {
	case WouldSet:
		return 0
	case RepoNoChange:
		return 1
	case WouldSkipScope, WouldSkipSetup:
		return 2
	default:
		return 3
	}
}

// labelSortKey returns the label name, or the skipped operation text for
// skip results, which have no label name.
func labelSortKey(r LabelResult) string {
	if r.Name != "" {
		return r.Name
	}
	return r.Operation.String()
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
