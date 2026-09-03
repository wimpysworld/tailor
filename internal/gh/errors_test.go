package gh

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestErrInsufficientScope_Error(t *testing.T) {
	err := &ErrInsufficientScope{
		StatusCode: 403,
		HaveScopes: []string{"public_repo"},
		NeedScopes: []string{"repo"},
		Message:    "Must have admin rights to Repository.",
		Operation:  SecurityFeatureOp(true, OpSetVulnerabilityAlerts),
	}

	want := "enable vulnerability alerts: insufficient scope (have: [public_repo], need: [repo]): Must have admin rights to Repository."
	if got := err.Error(); got != want {
		t.Errorf("Error() =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestErrInsufficientScope_ErrorEmptyScopes(t *testing.T) {
	err := &ErrInsufficientScope{
		StatusCode: 403,
		HaveScopes: nil,
		NeedScopes: nil,
		Message:    "Resource not accessible by integration",
		Operation:  SecurityFeatureOp(true, OpSetVulnerabilityAlerts),
	}

	want := "enable vulnerability alerts: insufficient scope (have: [], need: []): Resource not accessible by integration"
	if got := err.Error(); got != want {
		t.Errorf("Error() =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestErrInsufficientScope_ErrorPatchRepoSettings(t *testing.T) {
	err := &ErrInsufficientScope{
		StatusCode: 403,
		HaveScopes: []string{"public_repo"},
		NeedScopes: []string{"repo"},
		Message:    "Forbidden",
		Operation:  Op(OpPatchRepoSettings),
	}

	want := "patch repo settings: insufficient scope (have: [public_repo], need: [repo]): Forbidden"
	if got := err.Error(); got != want {
		t.Errorf("Error() =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestErrInsufficientScope_SatisfiesErrorInterface(t *testing.T) {
	var err error = &ErrInsufficientScope{
		StatusCode: 403,
		Operation:  Op(OpSetTopics),
		Message:    "test message",
	}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestErrInsufficientScope_ErrorsAs(t *testing.T) {
	original := &ErrInsufficientScope{
		StatusCode: 403,
		HaveScopes: []string{"public_repo"},
		NeedScopes: []string{"repo"},
		Message:    "forbidden",
		Operation:  SecurityFeatureOp(true, OpSetVulnerabilityAlerts),
	}

	wrapped := fmt.Errorf("applying settings: %w", original)

	var target *ErrInsufficientScope
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to unwrap ErrInsufficientScope")
	}
	if target.Operation.String() != "enable vulnerability alerts" {
		t.Errorf("Operation = %q, want %q", target.Operation.String(), "enable vulnerability alerts")
	}
	if target.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", target.StatusCode, http.StatusForbidden)
	}
}

func newHTTPError(statusCode int, message string, headers http.Header) *api.HTTPError {
	return &api.HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Headers:    headers,
		RequestURL: &url.URL{Scheme: "https", Host: "api.github.com", Path: "/repos/o/r"},
	}
}

func TestClassifyHTTPError_NilError(t *testing.T) {
	if err := classifyHTTPError(nil, Op(OpPatchRepoSettings)); err != nil {
		t.Errorf("classifyHTTPError(nil) = %v, want nil", err)
	}
}

func TestClassifyHTTPError_NonHTTPError(t *testing.T) {
	original := fmt.Errorf("network timeout")
	got := classifyHTTPError(original, Op(OpPatchRepoSettings))
	if !errors.Is(got, original) {
		t.Errorf("classifyHTTPError returned %v, want original error %v", got, original)
	}
}

func TestClassifyHTTPError_Non403Non404(t *testing.T) {
	httpErr := newHTTPError(http.StatusInternalServerError, "Internal Server Error", http.Header{})
	got := classifyHTTPError(httpErr, Op(OpPatchRepoSettings))

	var target *api.HTTPError
	if !errors.As(got, &target) {
		t.Fatal("expected *api.HTTPError passthrough for 500")
	}
	if target.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", target.StatusCode, http.StatusInternalServerError)
	}
}

func TestClassifyHTTPError_403ScopeError(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-OAuth-Scopes", "public_repo, read:org")
	headers.Set("X-Accepted-OAuth-Scopes", "repo")
	httpErr := newHTTPError(http.StatusForbidden, "Must have admin rights to Repository.", headers)

	got := classifyHTTPError(httpErr, Op(OpPatchRepoSettings))

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope, got %T: %v", got, got)
	}
	if scopeErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", scopeErr.StatusCode, http.StatusForbidden)
	}
	if scopeErr.Operation != Op(OpPatchRepoSettings) {
		t.Errorf("Operation = %v, want %v", scopeErr.Operation, Op(OpPatchRepoSettings))
	}
	if len(scopeErr.HaveScopes) != 2 || scopeErr.HaveScopes[0] != "public_repo" || scopeErr.HaveScopes[1] != "read:org" {
		t.Errorf("HaveScopes = %v, want [public_repo read:org]", scopeErr.HaveScopes)
	}
	if len(scopeErr.NeedScopes) != 1 || scopeErr.NeedScopes[0] != "repo" {
		t.Errorf("NeedScopes = %v, want [repo]", scopeErr.NeedScopes)
	}
	if scopeErr.Message != "Must have admin rights to Repository." {
		t.Errorf("Message = %q, want %q", scopeErr.Message, "Must have admin rights to Repository.")
	}
}

func TestClassifyHTTPError_404ClassifiedAsScopeError(t *testing.T) {
	httpErr := newHTTPError(http.StatusNotFound, "Not Found", http.Header{})

	got := classifyHTTPError(httpErr, Op(OpPatchRepoSettings))

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope for 404, got %T: %v", got, got)
	}
	if scopeErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", scopeErr.StatusCode, http.StatusNotFound)
	}
}

func TestClassifyHTTPError_UpdateLabel404PassesThrough(t *testing.T) {
	httpErr := newHTTPError(http.StatusNotFound, "Not Found", http.Header{})

	got := classifyHTTPError(httpErr, UpdateLabelOp("bug"))

	if !errors.Is(got, httpErr) {
		t.Errorf("classifyHTTPError() = %v, want original error", got)
	}
	if isAccessError(got) {
		t.Errorf("classifyHTTPError() returned an access error: %v", got)
	}
}

func TestClassifyHTTPError_WrappedHTTPError(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Accepted-OAuth-Scopes", "repo")
	httpErr := newHTTPError(http.StatusForbidden, "Forbidden", headers)
	wrapped := fmt.Errorf("applying settings: %w", httpErr)

	got := classifyHTTPError(wrapped, Op(OpPatchRepoSettings))

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope from wrapped error, got %T: %v", got, got)
	}
	if scopeErr.Operation != Op(OpPatchRepoSettings) {
		t.Errorf("Operation = %v, want %v", scopeErr.Operation, Op(OpPatchRepoSettings))
	}
}

func TestClassifyHTTPError_EmptyScopeHeaders(t *testing.T) {
	httpErr := newHTTPError(http.StatusForbidden, "Resource not accessible by integration", http.Header{})

	got := classifyHTTPError(httpErr, Op(OpPatchRepoSettings))

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope, got %T: %v", got, got)
	}
	if scopeErr.HaveScopes != nil {
		t.Errorf("HaveScopes = %v, want nil", scopeErr.HaveScopes)
	}
	if scopeErr.NeedScopes != nil {
		t.Errorf("NeedScopes = %v, want nil", scopeErr.NeedScopes)
	}
}

func TestParseCSVScopes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []string
		wantNi bool // want nil
	}{
		{"empty string", "", nil, true},
		{"single scope", "repo", []string{"repo"}, false},
		{"multiple scopes", "repo, read:org, user", []string{"repo", "read:org", "user"}, false},
		{"extra whitespace", "  repo , read:org  ", []string{"repo", "read:org"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCSVScopes(tt.input)
			if tt.wantNi {
				if got != nil {
					t.Errorf("parseCSVScopes(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("parseCSVScopes(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBoundedHTTPErrorKeepsNormalisedValidationDetail(t *testing.T) {
	// go-gh normalises a validation error with a non-custom code into a
	// Message line and leaves the item Message empty.
	httpErr := &api.HTTPError{
		Message:    "Validation Failed\nLabel.name already exists",
		RequestURL: &url.URL{Scheme: "https", Host: "api.github.com", Path: "/repos/acme/widget/labels"},
		StatusCode: http.StatusUnprocessableEntity,
		Errors: []api.HTTPErrorItem{
			{Resource: "Label", Field: "name", Code: "already_exists"},
		},
	}
	rendered := boundedHTTPError(httpErr).Error()
	if !strings.Contains(rendered, "Label.name already exists") {
		t.Errorf("error is missing the validation detail: %q", rendered)
	}
}
