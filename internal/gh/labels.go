package gh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
)

// labelResponse holds the subset of GitHub label fields we read.
type labelResponse struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// ReadLabels fetches all labels from a repository using paginated GET requests.
// Returns an empty slice (not nil) when the repository has no labels.
func ReadLabels(client *api.RESTClient, owner, repo string) ([]config.LabelEntry, error) {
	var all []config.LabelEntry

	for page := 1; ; page++ {
		path := fmt.Sprintf("repos/%s/%s/labels?per_page=100&page=%d", owner, repo, page)
		resp, err := client.RequestWithContext(context.Background(), http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching labels page %d: %w", page, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading labels response: %w", err)
		}

		var labels []labelResponse
		if err := json.Unmarshal(body, &labels); err != nil {
			return nil, fmt.Errorf("decoding labels: %w", err)
		}

		for _, l := range labels {
			all = append(all, config.LabelEntry{
				Name:        l.Name,
				Color:       l.Color,
				Description: l.Description,
			})
		}

		if len(labels) < 100 || !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}

	if all == nil {
		all = []config.LabelEntry{}
	}

	return all, nil
}

// hasNextPage checks whether a Link header contains a rel="next" link.
func hasNextPage(link string) bool {
	return strings.Contains(link, `rel="next"`)
}

// ApplyLabels diffs desired labels against current labels and reconciles the
// difference. Missing labels are created (POST), changed labels are updated
// (PATCH), and matched labels are skipped. Labels present on GitHub but absent
// from desired are left untouched (no delete/prune).
//
// Name matching is case-insensitive per GitHub's label behaviour.
func ApplyLabels(client *api.RESTClient, owner, repo string, desired, current []config.LabelEntry) error {
	currentMap := make(map[string]config.LabelEntry, len(current))
	for _, l := range current {
		currentMap[strings.ToLower(l.Name)] = l
	}

	for _, d := range desired {
		key := strings.ToLower(d.Name)
		existing, found := currentMap[key]

		if !found {
			if err := createLabel(client, owner, repo, d); err != nil {
				return err
			}
			continue
		}

		if config.LabelNeedsUpdate(existing, d) {
			if err := updateLabel(client, owner, repo, existing.Name, d); err != nil {
				return err
			}
		}
	}

	return nil
}

// createLabel sends a POST to create a new label.
func createLabel(client *api.RESTClient, owner, repo string, label config.LabelEntry) error {
	body := labelResponse{
		Name:        label.Name,
		Color:       label.Color,
		Description: label.Description,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling label %q: %w", label.Name, err)
	}

	path := fmt.Sprintf("repos/%s/%s/labels", owner, repo)
	if err := client.Post(path, bytes.NewReader(payload), nil); err != nil {
		return fmt.Errorf("creating label %q: %w", label.Name, err)
	}
	return nil
}

// updateLabel sends a PATCH to update an existing label's colour or description.
// The name parameter is the current name on GitHub (used in the URL path).
func updateLabel(client *api.RESTClient, owner, repo, name string, label config.LabelEntry) error {
	body := map[string]string{
		"new_name":    label.Name,
		"color":       label.Color,
		"description": label.Description,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling label update %q: %w", name, err)
	}

	path := fmt.Sprintf("repos/%s/%s/labels/%s", owner, repo, name)
	if err := client.Patch(path, bytes.NewReader(payload), nil); err != nil {
		return fmt.Errorf("updating label %q: %w", name, err)
	}
	return nil
}
