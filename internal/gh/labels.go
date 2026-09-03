package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

// labelResponse holds the subset of GitHub label fields read from the API.
type labelResponse struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// ReadLabels fetches all labels from a repository using paginated GET requests.
// Returns an empty slice (not nil) when the repository has no labels.
func ReadLabels(client *api.RESTClient, owner, repo string) ([]model.LabelEntry, error) {
	var all []model.LabelEntry

	for page := 1; ; page++ {
		path := fmt.Sprintf("repos/%s/%s/labels?per_page=100&page=%d", owner, repo, page)
		resp, err := client.RequestWithContext(context.Background(), http.MethodGet, path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetching labels page %d: %w", page, boundedHTTPError(err))
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
			all = append(all, model.LabelEntry{
				Name:        l.Name,
				Color:       l.Color,
				Description: l.Description,
			})
		}

		if !hasNextPage(resp.Header.Get("Link")) {
			break
		}
	}

	if all == nil {
		all = []model.LabelEntry{}
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
//
// Access errors (insufficient scope or role) on individual labels are collected
// in the returned ApplyResult rather than aborting, so a 403 on one label does
// not prevent others from being applied. Rate-limit errors abort immediately
// with a partial-completion error, so the loop does not burn the remaining
// API budget.
func ApplyLabels(client *api.RESTClient, owner, repo string, desired, current []model.LabelEntry) (*ApplyResult, error) {
	result := &ApplyResult{}

	currentMap := make(map[string]model.LabelEntry, len(current))
	for _, l := range current {
		currentMap[strings.ToLower(l.Name)] = l
	}

	applied := 0
	for i, d := range desired {
		key := strings.ToLower(d.Name)
		existing, found := currentMap[key]

		if !found {
			if err := createLabel(client, owner, repo, d); err != nil {
				skip, writeErr := labelWriteError(result, CreateLabelOp(d.Name), err, applied, remainingLabelChanges(desired[i:], currentMap))
				if skip {
					continue
				}
				return nil, writeErr
			}
			applied++
			continue
		}

		if model.LabelNeedsUpdate(existing, d) {
			if err := updateLabel(client, owner, repo, existing.Name, d); err != nil {
				skip, writeErr := labelWriteError(result, UpdateLabelOp(d.Name), err, applied, remainingLabelChanges(desired[i:], currentMap))
				if skip {
					continue
				}
				return nil, writeErr
			}
			applied++
		}
	}

	return result, nil
}

func remainingLabelChanges(desired []model.LabelEntry, current map[string]model.LabelEntry) int {
	remaining := 0
	for _, label := range desired {
		existing, found := current[strings.ToLower(label.Name)]
		if !found || model.LabelNeedsUpdate(existing, label) {
			remaining++
		}
	}
	return remaining
}

// labelWriteError handles a failed label create or update. A rate-limit error
// is returned wrapped with the partial-completion report. An access error is
// recorded in result and skip is true, so the caller moves to the next label.
// Every other error is returned unchanged with skip false.
func labelWriteError(result *ApplyResult, operation Operation, err error, applied, remaining int) (skip bool, writeErr error) {
	if limitErr := labelRateLimitError(err, operation, applied, remaining); limitErr != nil {
		return false, limitErr
	}
	if recordAccessError(result, operation, err) {
		return true, nil
	}
	return false, err
}

// labelRateLimitError classifies err and, when it is a rate-limit error,
// wraps it with a partial-completion report. Returns nil for every other
// error, leaving the caller's access-error and hard-error handling intact.
func labelRateLimitError(err error, operation Operation, applied, remaining int) error {
	classified := classifyHTTPError(err, operation)
	if !isRateLimitError(classified) {
		return nil
	}
	return fmt.Errorf("rate limited while applying labels: %d applied, %d remaining: %w", applied, remaining, classified)
}

// createLabel sends a POST to create a new label.
func createLabel(client *api.RESTClient, owner, repo string, label model.LabelEntry) error {
	body := labelResponse{
		Name:        label.Name,
		Color:       label.Color,
		Description: label.Description,
	}
	path := fmt.Sprintf("repos/%s/%s/labels", owner, repo)
	if err := sendJSON(client, http.MethodPost, path, body); err != nil {
		return fmt.Errorf("creating label %q: %w", label.Name, err)
	}
	return nil
}

// updateLabel sends a PATCH to update an existing label's colour or description.
// The name parameter is the current name on GitHub (used in the URL path).
func updateLabel(client *api.RESTClient, owner, repo, name string, label model.LabelEntry) error {
	body := map[string]string{
		"new_name":    label.Name,
		"color":       label.Color,
		"description": label.Description,
	}
	path := fmt.Sprintf("repos/%s/%s/labels/%s", owner, repo, url.PathEscape(name))
	if err := sendJSON(client, http.MethodPatch, path, body); err != nil {
		return fmt.Errorf("updating label %q: %w", name, err)
	}
	return nil
}
