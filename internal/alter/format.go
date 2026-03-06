package alter

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// defaultLabelWidth is the minimum column width for status labels in formatted
// output. Sized to accommodate "skipped (first-fit, exists): " (29 characters).
// Annotations on triggered swatches may widen this dynamically.
const defaultLabelWidth = 29

// FormatOutput produces the alter command output from repo settings results,
// label results, and swatch results (including licence).
func FormatOutput(repoResults []RepoSettingResult, labelResults []LabelResult, swatchResults []SwatchResult) string {
	if len(repoResults) == 0 && len(labelResults) == 0 && len(swatchResults) == 0 {
		return ""
	}

	sortedSwatches := sortSwatchResults(swatchResults)
	width := labelWidth(sortedSwatches)

	var b strings.Builder

	for _, r := range sortRepoResults(repoResults) {
		label := string(r.Category) + ":"
		switch r.Category {
		case WouldSet:
			fmt.Fprintf(&b, "%-*srepository.%s = %s\n", width, label, r.Field, r.Value)
		case RepoNoChange:
			fmt.Fprintf(&b, "%-*srepository.%s (already %s)\n", width, label, r.Field, r.Value)
		}
	}

	for _, r := range sortLabelResults(labelResults) {
		label := string(r.Category) + ":"
		switch r.Category {
		case WouldCreate, WouldUpdate:
			fmt.Fprintf(&b, "%-*slabel.%s = %s\n", width, label, r.Name, r.Value)
		case LabelNoChange:
			fmt.Fprintf(&b, "%-*slabel.%s (already %s)\n", width, label, r.Name, r.Value)
		}
	}

	for _, r := range sortedSwatches {
		label := swatchLabel(r)
		fmt.Fprintf(&b, "%-*s%s\n", width, label, r.Destination)
	}

	return b.String()
}

// swatchLabel returns the formatted label for a swatch result, including any
// trigger annotation. For example: "would copy (triggered: allow_auto_merge):".
func swatchLabel(r SwatchResult) string {
	if r.Annotation != "" {
		return string(r.Category) + " (" + r.Annotation + "):"
	}
	return string(r.Category) + ":"
}

// labelWidth computes the column width needed to accommodate all labels. It
// returns at least defaultLabelWidth, widening if any annotated swatch label
// exceeds that.
func labelWidth(swatches []SwatchResult) int {
	width := defaultLabelWidth
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
		return strings.Compare(a.Field, b.Field)
	})
	return sorted
}

// repoOrder returns the sort priority for a RepoSettingCategory.
func repoOrder(c RepoSettingCategory) int {
	switch c {
	case WouldSet:
		return 0
	default:
		return 1
	}
}

// sortSwatchResults returns a sorted copy: actionable (WouldCopy, WouldOverwrite)
// before informational (NoChange, SkippedFirstFit), lexicographic by destination within
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
		return strings.Compare(a.Destination, b.Destination)
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
		return strings.Compare(a.Name, b.Name)
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
	default:
		return 2
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
	case WouldRemove:
		return 2
	case Removed:
		return 3
	case NoChange:
		return 4
	case SkippedFirstFit:
		return 5
	case SkippedNever:
		return 6
	default:
		return 7
	}
}
