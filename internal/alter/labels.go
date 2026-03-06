package alter

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

// LabelCategory classifies the outcome of processing a single label entry.
type LabelCategory string

const (
	WouldCreate   LabelCategory = "would create"
	WouldUpdate   LabelCategory = "would update"
	LabelNoChange LabelCategory = "no change"
)

// LabelResult records the label name, category, and display value for one
// label entry.
type LabelResult struct {
	Name     string
	Category LabelCategory
	Value    string
}

// ProcessLabels compares declared labels against live labels and optionally
// applies them. Returns results for output formatting.
func ProcessLabels(cfg *config.Config, mode ApplyMode, client *api.RESTClient, owner, name string, hasRepo bool) ([]LabelResult, error) {
	if len(cfg.Labels) == 0 {
		return nil, nil
	}

	if !hasRepo {
		fmt.Fprintln(os.Stderr, "No GitHub repository context found. Labels will be applied once a remote is configured.")
		return nil, nil
	}

	current, err := gh.ReadLabels(client, owner, name)
	if err != nil {
		return nil, err
	}

	results := compareLabels(cfg.Labels, current)

	if mode.ShouldWrite() && hasLabelChanges(results) {
		if err := gh.ApplyLabels(client, owner, name, cfg.Labels, current); err != nil {
			return nil, err
		}
	}

	return results, nil
}

// compareLabels iterates desired labels and compares each against current
// labels. Name matching is case-insensitive per GitHub's label behaviour.
func compareLabels(desired, current []config.LabelEntry) []LabelResult {
	currentMap := make(map[string]config.LabelEntry, len(current))
	for _, l := range current {
		currentMap[strings.ToLower(l.Name)] = l
	}

	results := make([]LabelResult, 0, len(desired))

	for _, d := range desired {
		key := strings.ToLower(d.Name)
		existing, found := currentMap[key]

		display := formatLabelValue(d)

		if !found {
			results = append(results, LabelResult{
				Name:     d.Name,
				Category: WouldCreate,
				Value:    display,
			})
			continue
		}

		if config.LabelNeedsUpdate(existing, d) {
			results = append(results, LabelResult{
				Name:     d.Name,
				Category: WouldUpdate,
				Value:    display,
			})
			continue
		}

		results = append(results, LabelResult{
			Name:     d.Name,
			Category: LabelNoChange,
			Value:    display,
		})
	}

	return results
}

// formatLabelValue returns a display string for a label entry.
func formatLabelValue(l config.LabelEntry) string {
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
