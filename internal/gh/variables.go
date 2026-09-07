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

type variableResponse struct {
	Name  *string `json:"name"`
	Value *string `json:"value"`
}

// ReadVariables returns a complete collection, or nil with an error.
func ReadVariables(client *api.RESTClient, owner, repo string) ([]model.VariableEntry, error) {
	all := []model.VariableEntry{}
	seen := make(map[string]bool)
	for page := 1; ; page++ {
		variables, total, next, err := readVariablesPage(client, owner, repo, page)
		if err != nil {
			return nil, fmt.Errorf("fetching variables page %d: %w", page, classifyHTTPError(err, Op(OpFetchVariables)))
		}
		for _, variable := range variables {
			key := strings.ToLower(variable.Name)
			if seen[key] {
				return nil, fmt.Errorf("decoding variables: duplicate name %q", variable.Name)
			}
			seen[key] = true
			all = append(all, variable)
		}
		if !next {
			if len(all) != total {
				return nil, fmt.Errorf("fetching variables: received %d of %d entries", len(all), total)
			}
			return all, nil
		}
		if len(variables) == 0 {
			return nil, fmt.Errorf("fetching variables: empty page with next link")
		}
	}
}

func readVariablesPage(client *api.RESTClient, owner, repo string, page int) ([]model.VariableEntry, int, bool, error) {
	path := fmt.Sprintf("repos/%s/%s/actions/variables?per_page=30&page=%d", owner, repo, page)
	resp, err := client.RequestWithContext(context.Background(), http.MethodGet, path, nil)
	if err != nil {
		return nil, 0, false, boundedHTTPError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading variables response: %w", err)
	}
	var response struct {
		TotalCount *int                `json:"total_count"`
		Variables  *[]variableResponse `json:"variables"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, 0, false, fmt.Errorf("decoding variables: %w", err)
	}
	if response.TotalCount == nil || *response.TotalCount < 0 || response.Variables == nil {
		return nil, 0, false, fmt.Errorf("decoding variables: missing or invalid total_count or variables")
	}
	variables := make([]model.VariableEntry, 0, len(*response.Variables))
	for _, variable := range *response.Variables {
		if variable.Name == nil || *variable.Name == "" || variable.Value == nil {
			return nil, 0, false, fmt.Errorf("decoding variables: missing or invalid name or value")
		}
		variables = append(variables, model.VariableEntry{Name: *variable.Name, Value: variable.Value})
	}
	return variables, *response.TotalCount, hasNextPage(resp.Header.Get("Link")), nil
}

// ApplyVariables creates or updates declared variables without renaming or deleting.
// The current collection must come from a successful ReadVariables call.
func ApplyVariables(client *api.RESTClient, owner, repo string, desired, current []model.VariableEntry) (*ApplyResult, error) {
	result := &ApplyResult{}
	if len(desired) == 0 {
		return result, nil
	}
	if current == nil {
		return result, fmt.Errorf("applying variables: current collection is unavailable")
	}
	currentMap := make(map[string]model.VariableEntry, len(current))
	for _, variable := range current {
		if variable.Value == nil {
			return result, fmt.Errorf("applying variables: current value is missing for %q", variable.Name)
		}
		currentMap[strings.ToLower(variable.Name)] = variable
	}
	for _, variable := range desired {
		if variable.Value == nil {
			return result, fmt.Errorf("applying variables: desired value is missing for %q", variable.Name)
		}
	}
	for i, variable := range desired {
		existing, found := currentMap[strings.ToLower(variable.Name)]
		if found && *existing.Value == *variable.Value {
			continue
		}
		operation := CreateVariableOp(variable.Name)
		if found {
			operation = UpdateVariableOp(variable.Name)
		}
		if err := writeVariable(client, owner, repo, variable, existing, found); err != nil {
			if recordAccessError(result, operation, err) {
				continue
			}
			classified := classifyHTTPError(err, operation)
			return result, fmt.Errorf("applying variables: %d applied, %d remaining: %w", len(result.Applied),
				remainingVariableChanges(desired[i:], currentMap), classified)
		}
		result.Applied = append(result.Applied, operation)
	}
	return result, nil
}

func remainingVariableChanges(desired []model.VariableEntry, current map[string]model.VariableEntry) int {
	remaining := 0
	for _, variable := range desired {
		existing, found := current[strings.ToLower(variable.Name)]
		if !found || *existing.Value != *variable.Value {
			remaining++
		}
	}
	return remaining
}

func writeVariable(client *api.RESTClient, owner, repo string, variable, existing model.VariableEntry, found bool) error {
	path := fmt.Sprintf("repos/%s/%s/actions/variables", owner, repo)
	method := http.MethodPost
	body := map[string]string{"name": variable.Name, "value": *variable.Value}
	if found {
		method = http.MethodPatch
		path += "/" + url.PathEscape(existing.Name)
		delete(body, "name")
	}
	if err := sendJSON(client, method, path, body); err != nil {
		return fmt.Errorf("writing variable %q: %w", variable.Name, err)
	}
	return nil
}
