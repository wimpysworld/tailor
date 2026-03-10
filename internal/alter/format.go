package alter

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// defaultLabelWidth is the minimum column width for status labels in formatted
// output. Sized to accommodate "would skip (insufficient scope): " (37 characters).
// Annotations on triggered swatches may widen this dynamically.
const defaultLabelWidth = 37

// FormatOutput produces the alter command output from repo settings results,
// label results, and swatch results (including licence).
func FormatOutput(repoResults []RepoSettingResult, labelResults []LabelResult, swatchResults []SwatchResult) string {
	if len(repoResults) == 0 && len(labelResults) == 0 && len(swatchResults) == 0 {
		return ""
	}

	sortedSwatches := sortSwatchResults(swatchResults)
	width := labelWidth(repoResults, labelResults, sortedSwatches)

	var b strings.Builder

	for _, r := range sortRepoResults(repoResults) {
		label := repoLabel(r)
		switch r.Category {
		case WouldSet:
			fmt.Fprintf(&b, "%-*srepository.%s = %s\n", width, label, r.Field, r.Value)
		case RepoNoChange:
			fmt.Fprintf(&b, "%-*srepository.%s (already %s)\n", width, label, r.Field, r.Value)
		case WouldSkipScope, WouldSkipRole:
			fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Field)
		}
	}

	for _, r := range sortLabelResults(labelResults) {
		label := labelResultLabel(r)
		switch r.Category {
		case WouldCreate, WouldUpdate:
			fmt.Fprintf(&b, "%-*slabel.%s = %s\n", width, label, r.Name, r.Value)
		case LabelNoChange:
			fmt.Fprintf(&b, "%-*slabel.%s (already %s)\n", width, label, r.Name, r.Value)
		case LabelSkipScope, LabelSkipRole:
			fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Name)
		}
	}

	for _, r := range sortedSwatches {
		label := swatchLabel(r)
		fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Path)
	}

	return b.String()
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

// repoLabel returns the formatted label for a repo setting result.
func repoLabel(r RepoSettingResult) string {
	isSkip := r.Category == WouldSkipScope || r.Category == WouldSkipRole
	return formatAnnotatedLabel(string(r.Category), r.Annotation, isSkip)
}

// labelResultLabel returns the formatted label for a label result.
func labelResultLabel(r LabelResult) string {
	isSkip := r.Category == LabelSkipScope || r.Category == LabelSkipRole
	return formatAnnotatedLabel(string(r.Category), r.Annotation, isSkip)
}

// swatchLabel returns the formatted label for a swatch result, including any
// trigger annotation. For example: "would deploy (triggered: allow_auto_merge):".
func swatchLabel(r SwatchResult) string {
	if r.Annotation != "" {
		return string(r.Category) + " (" + r.Annotation + "):"
	}
	return string(r.Category) + ":"
}

// labelWidth computes the column width needed to accommodate all labels. It
// returns at least defaultLabelWidth, widening if any annotated label exceeds
// that.
func labelWidth(repos []RepoSettingResult, labels []LabelResult, swatches []SwatchResult) int {
	width := defaultLabelWidth
	for _, r := range repos {
		if w := len(repoLabel(r)) + 1; w > width {
			width = w
		}
	}
	for _, r := range labels {
		if w := len(labelResultLabel(r)) + 1; w > width {
			width = w
		}
	}
	for _, r := range swatches {
		if w := len(swatchLabel(r)) + 1; w > width {
			width = w
		}
	}
	return width
}

// sortRepoResults returns a sorted copy: actionable (WouldSet) before
// informational (RepoNoChange), lexicographic by field within each group.
func sortRepoResults(results []RepoSettingResult) []RepoSettingResult {
	if len(results) == 0 {
		return nil
	}
	sorted := make([]RepoSettingResult, len(results))
	copy(sorted, results)
	slices.SortStableFunc(sorted, func(a, b RepoSettingResult) int {
		if c := cmp.Compare(repoOrder(a.Category), repoOrder(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.Field, b.Field)
	})
	return sorted
}

// repoOrder returns the sort priority for a RepoSettingCategory.
func repoOrder(c RepoSettingCategory) int {
	switch c {
	case WouldSet:
		return 0
	case RepoNoChange:
		return 1
	case WouldSkipScope, WouldSkipRole:
		return 2
	default:
		return 3
	}
}

// sortSwatchResults returns a sorted copy: actionable (WouldCopy, WouldOverwrite)
// before informational (NoChange, SkippedFirstFit), lexicographic by path within
// each group.
func sortSwatchResults(results []SwatchResult) []SwatchResult {
	if len(results) == 0 {
		return nil
	}
	sorted := make([]SwatchResult, len(results))
	copy(sorted, results)
	slices.SortStableFunc(sorted, func(a, b SwatchResult) int {
		if c := cmp.Compare(swatchOrder(a.Category), swatchOrder(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.Path, b.Path)
	})
	return sorted
}

// sortLabelResults returns a sorted copy: actionable (WouldCreate, WouldUpdate)
// before informational (LabelNoChange), lexicographic by name within each group.
func sortLabelResults(results []LabelResult) []LabelResult {
	if len(results) == 0 {
		return nil
	}
	sorted := make([]LabelResult, len(results))
	copy(sorted, results)
	slices.SortStableFunc(sorted, func(a, b LabelResult) int {
		if c := cmp.Compare(labelOrder(a.Category), labelOrder(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return sorted
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
	case LabelSkipScope, LabelSkipRole:
		return 3
	default:
		return 4
	}
}

// swatchOrder returns the sort priority for a SwatchCategory.
// Actionable categories sort before informational: deploy, overwrite, remove,
// then no-change, skipped, ignored.
func swatchOrder(c SwatchCategory) int {
	switch c {
	case WouldCopy:
		return 0
	case WouldOverwrite:
		return 1
	case WouldDeploy:
		return 2
	case WouldRemove:
		return 3
	case Removed:
		return 4
	case NoChange:
		return 5
	case SkippedFirstFit:
		return 6
	case SkippedNever:
		return 7
	default:
		return 8
	}
}
