package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/cli/go-gh/v2/pkg/api"
)

// SetupSkipReason explains why a code scanning or Code Quality setup
// operation was skipped without stopping the command.
type SetupSkipReason string

const (
	// SetupNotAvailable means GitHub answered 403, or 404 on a read: the
	// feature is not available to the repository.
	SetupNotAvailable SetupSkipReason = "not available"
	// SetupInProgress means GitHub answered 409 on a write: a setup run is
	// in progress.
	SetupInProgress SetupSkipReason = "setup in progress"
)

// ErrSetupSkipped reports that a setup read or write did not complete for a
// reason that is not a hard failure. The caller reports the affected fields
// as skipped and continues.
type ErrSetupSkipped struct {
	StatusCode int
	Reason     SetupSkipReason
	Operation  Operation
}

func (e *ErrSetupSkipped) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Operation, e.Reason, e.StatusCode)
}

// classifySetupError converts the skip responses into *ErrSetupSkipped. Rate
// limit responses become *ErrRateLimited. Other errors carry the operation
// and stop the command.
func classifySetupError(err error, operation Operation, write bool) error {
	if err == nil {
		return nil
	}
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if isRateLimitHTTPError(httpErr) {
		return classifyHTTPError(err, operation)
	}
	skipped := &ErrSetupSkipped{StatusCode: httpErr.StatusCode, Operation: operation}
	switch {
	case httpErr.StatusCode == http.StatusForbidden, !write && httpErr.StatusCode == http.StatusNotFound:
		skipped.Reason = SetupNotAvailable
		return skipped
	case write && httpErr.StatusCode == http.StatusConflict:
		skipped.Reason = SetupInProgress
		return skipped
	}
	return fmt.Errorf("%s: %w", operation, err)
}

// readSetup reads one setup endpoint into response.
func readSetup(client *api.RESTClient, path string, operation Operation, response any) error {
	return classifySetupError(boundedHTTPError(client.Get(path, response)), operation, false)
}

// patchSetup sends body to one setup endpoint. GitHub answers 202 when it
// accepts the update and starts a setup run.
func patchSetup(client *api.RESTClient, path string, operation Operation, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", operation, err)
	}
	return classifySetupError(boundedHTTPError(client.Patch(path, bytes.NewReader(payload), nil)), operation, true)
}

// optionalString returns a pointer to value, or nil when the response omitted
// the key or sent an empty string, so fit never writes an empty enum value.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// sortedLanguages returns a sorted copy of a declared language list, or nil
// when the list is nil or empty, which means GitHub detects the languages.
func sortedLanguages(languages *[]string) []string {
	if languages == nil || len(*languages) == 0 {
		return nil
	}
	sorted := slices.Clone(*languages)
	slices.Sort(sorted)
	return sorted
}
