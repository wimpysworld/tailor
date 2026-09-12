package alter

import (
	"fmt"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// LabelCategory classifies label and variable outcomes.
type LabelCategory string

const (
	// WouldCreate marks a missing label or variable.
	WouldCreate LabelCategory = "would create"
	// WouldUpdate marks an existing label or variable that differs.
	WouldUpdate LabelCategory = "would update"
	// LabelNoChange marks a label or variable that already matches.
	LabelNoChange LabelCategory = "no change"
	// LabelSkipScope marks an operation blocked by insufficient access.
	LabelSkipScope LabelCategory = "would skip (insufficient scope)"
)

// LabelResult records the label name, category, and display value for one
// label entry. Skip results leave Name empty and carry the skipped Operation
// instead. Annotation adds context to the displayed status label.
type LabelResult struct {
	Name       string
	Category   LabelCategory
	Value      string
	Before     string
	Annotation string
	Operation  gh.Operation
}

// ProcessLabels previews or applies declared labels without deleting undeclared labels.
func ProcessLabels(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]LabelResult, error) {
	if len(cfg.Labels) == 0 {
		return nil, nil
	}

	if target.missingRepo("Labels") {
		return nil, nil
	}

	current, err := gh.ReadLabels(target.Client, target.Owner, target.Name)
	if err != nil {
		return nil, err
	}

	results := compareLabels(cfg.Labels, current)

	if mode.ShouldWrite() && hasLabelChanges(results) {
		applyResult, err := gh.ApplyLabels(target.Client, target.Owner, target.Name, cfg.Labels, current)
		if err != nil {
			return nil, err
		}
		results = append(results, labelSkippedToResults(applyResult)...)
	}

	return results, nil
}

// labelSkippedToResults converts gh.ApplyResult skipped operations into
// LabelResult entries with LabelSkipScope categories.
func labelSkippedToResults(ar *gh.ApplyResult) []LabelResult {
	if ar == nil {
		return nil
	}
	var results []LabelResult
	for _, sk := range ar.Skipped {
		results = append(results, LabelResult{
			Operation:  sk.Operation,
			Category:   LabelSkipScope,
			Annotation: skipAnnotation,
		})
	}
	return results
}

// compareLabels iterates desired labels and compares each against current
// labels. Name matching is case-insensitive per GitHub's label behaviour.
func compareLabels(desired, current []model.LabelEntry) []LabelResult {
	currentMap := make(map[string]model.LabelEntry, len(current))
	for _, l := range current {
		currentMap[strings.ToLower(l.Name)] = l
	}

	results := make([]LabelResult, 0, len(desired))

	for _, d := range desired {
		key := strings.ToLower(d.Name)
		existing, found := currentMap[key]

		display := formatLabelValue(d)

		category := LabelNoChange
		before := ""
		if found {
			before = formatLabelValue(existing)
		}
		switch {
		case !found:
			category = WouldCreate
		case model.LabelNeedsUpdate(existing, d):
			category = WouldUpdate
		}
		results = append(results, LabelResult{
			Name:     d.Name,
			Category: category,
			Value:    display,
			Before:   before,
		})
	}

	return results
}

// formatLabelValue returns a display string for a label entry.
func formatLabelValue(l model.LabelEntry) string {
	if l.Description != "" {
		return fmt.Sprintf("#%s %q", l.Color, l.Description)
	}
	return "#" + l.Color
}

// hasLabelChanges returns true if any result is WouldCreate or WouldUpdate.
func hasLabelChanges(results []LabelResult) bool {
	for _, r := range results {
		if r.Category == WouldCreate || r.Category == WouldUpdate {
			return true
		}
	}
	return false
}
