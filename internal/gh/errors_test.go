package gh

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func TestErrInsufficientScope_Error(t *testing.T) {
	err := &ErrInsufficientScope{
		StatusCode:  403,
		HaveScopes:  []string{"public_repo"},
		NeedScopes:  []string{"repo"},
		Message:     "Must have admin rights to Repository.",
		DocumentURL: "https://docs.github.com/rest/repos/repos#update-a-repository",
		Operation:   "enable vulnerability alerts",
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
		Operation:  "enable vulnerability alerts",
	}

	want := "enable vulnerability alerts: insufficient scope (have: [], need: []): Resource not accessible by integration"
	if got := err.Error(); got != want {
		t.Errorf("Error() =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestErrInsufficientRole_Error(t *testing.T) {
	err := &ErrInsufficientRole{
		StatusCode:   403,
		Message:      "Must have admin rights to Repository.",
		DocumentURL:  "https://docs.github.com/rest/repos/repos#update-a-repository",
		Operation:    "enable vulnerability alerts",
		RequiredRole: "admin",
	}

	want := "enable vulnerability alerts: insufficient role (need: admin): Must have admin rights to Repository."
	if got := err.Error(); got != want {
		t.Errorf("Error() =\n  %q\nwant:\n  %q", got, want)
	}
}

func TestErrInsufficientScope_SatisfiesErrorInterface(t *testing.T) {
	var err error = &ErrInsufficientScope{
		StatusCode: 403,
		Operation:  "test",
		Message:    "test message",
	}
	if err.Error() == "" {
		t.Error("expected non-empty error string")
	}
}

func TestErrInsufficientRole_SatisfiesErrorInterface(t *testing.T) {
	var err error = &ErrInsufficientRole{
		StatusCode:   403,
		Operation:    "test",
		RequiredRole: "admin",
		Message:      "test message",
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
		Operation:  "enable vulnerability alerts",
	}

	wrapped := fmt.Errorf("applying settings: %w", original)

	var target *ErrInsufficientScope
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to unwrap ErrInsufficientScope")
	}
	if target.Operation != "enable vulnerability alerts" {
		t.Errorf("Operation = %q, want %q", target.Operation, "enable vulnerability alerts")
	}
	if target.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", target.StatusCode, http.StatusForbidden)
	}
}

func TestErrInsufficientRole_ErrorsAs(t *testing.T) {
	original := &ErrInsufficientRole{
		StatusCode:   403,
		Message:      "Must have admin rights to Repository.",
		Operation:    "enable vulnerability alerts",
		RequiredRole: "admin",
	}

	wrapped := fmt.Errorf("applying settings: %w", original)

	var target *ErrInsufficientRole
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to unwrap ErrInsufficientRole")
	}
	if target.Operation != "enable vulnerability alerts" {
		t.Errorf("Operation = %q, want %q", target.Operation, "enable vulnerability alerts")
	}
	if target.RequiredRole != "admin" {
		t.Errorf("RequiredRole = %q, want %q", target.RequiredRole, "admin")
	}
}

// --- Task 1.3: admin-role registry tests ---

func TestRequiredRole_AdminOperations(t *testing.T) {
	tests := []struct {
		operation string
		wantRole  string
	}{
		{"enable vulnerability alerts", "admin"},
		{"disable vulnerability alerts", "admin"},
		{"enable automated security fixes", "admin"},
		{"disable automated security fixes", "admin"},
		{"enable private vulnerability reporting", "admin or security manager"},
		{"disable private vulnerability reporting", "admin or security manager"},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			role, ok := requiredRole(tt.operation)
			if !ok {
				t.Fatalf("requiredRole(%q) returned ok=false, want true", tt.operation)
			}
			if role != tt.wantRole {
				t.Errorf("requiredRole(%q) = %q, want %q", tt.operation, role, tt.wantRole)
			}
		})
	}
}

func TestRequiredRole_NonAdminOperation(t *testing.T) {
	_, ok := requiredRole("update repository settings")
	if ok {
		t.Error("requiredRole(\"update repository settings\") returned ok=true, want false")
	}
}

// --- Task 1.2: classifyHTTPError tests ---

func newHTTPError(statusCode int, message string, headers http.Header) *api.HTTPError {
	return &api.HTTPError{
		StatusCode: statusCode,
		Message:    message,
		Headers:    headers,
		RequestURL: &url.URL{Scheme: "https", Host: "api.github.com", Path: "/repos/o/r"},
	}
}

func TestClassifyHTTPError_NilError(t *testing.T) {
	if err := classifyHTTPError(nil, "test"); err != nil {
		t.Errorf("classifyHTTPError(nil) = %v, want nil", err)
	}
}

func TestClassifyHTTPError_NonHTTPError(t *testing.T) {
	original := fmt.Errorf("network timeout")
	got := classifyHTTPError(original, "test")
	if !errors.Is(got, original) {
		t.Errorf("classifyHTTPError returned %v, want original error %v", got, original)
	}
}

func TestClassifyHTTPError_Non403Non404(t *testing.T) {
	httpErr := newHTTPError(http.StatusInternalServerError, "Internal Server Error", http.Header{})
	got := classifyHTTPError(httpErr, "test")

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

	got := classifyHTTPError(httpErr, "update repository settings")

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope, got %T: %v", got, got)
	}
	if scopeErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", scopeErr.StatusCode, http.StatusForbidden)
	}
	if scopeErr.Operation != "update repository settings" {
		t.Errorf("Operation = %q, want %q", scopeErr.Operation, "update repository settings")
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

func TestClassifyHTTPError_403RoleError_VA(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-OAuth-Scopes", "repo")
	headers.Set("X-Accepted-OAuth-Scopes", "repo")
	httpErr := newHTTPError(http.StatusForbidden, "Must have admin rights to Repository.", headers)

	got := classifyHTTPError(httpErr, "enable vulnerability alerts")

	var roleErr *ErrInsufficientRole
	if !errors.As(got, &roleErr) {
		t.Fatalf("expected *ErrInsufficientRole, got %T: %v", got, got)
	}
	if roleErr.StatusCode != http.StatusForbidden {
		t.Errorf("StatusCode = %d, want %d", roleErr.StatusCode, http.StatusForbidden)
	}
	if roleErr.Operation != "enable vulnerability alerts" {
		t.Errorf("Operation = %q, want %q", roleErr.Operation, "enable vulnerability alerts")
	}
	if roleErr.RequiredRole != "admin" {
		t.Errorf("RequiredRole = %q, want %q", roleErr.RequiredRole, "admin")
	}
	if roleErr.Message != "Must have admin rights to Repository." {
		t.Errorf("Message = %q, want %q", roleErr.Message, "Must have admin rights to Repository.")
	}
}

func TestClassifyHTTPError_403RoleError_PVR(t *testing.T) {
	httpErr := newHTTPError(http.StatusForbidden, "Resource not accessible by integration", http.Header{})

	got := classifyHTTPError(httpErr, "enable private vulnerability reporting")

	var roleErr *ErrInsufficientRole
	if !errors.As(got, &roleErr) {
		t.Fatalf("expected *ErrInsufficientRole, got %T: %v", got, got)
	}
	if roleErr.RequiredRole != "admin or security manager" {
		t.Errorf("RequiredRole = %q, want %q", roleErr.RequiredRole, "admin or security manager")
	}
}

func TestClassifyHTTPError_403RoleError_ASF(t *testing.T) {
	httpErr := newHTTPError(http.StatusForbidden, "Resource not accessible by integration", http.Header{})

	got := classifyHTTPError(httpErr, "disable automated security fixes")

	var roleErr *ErrInsufficientRole
	if !errors.As(got, &roleErr) {
		t.Fatalf("expected *ErrInsufficientRole, got %T: %v", got, got)
	}
	if roleErr.RequiredRole != "admin" {
		t.Errorf("RequiredRole = %q, want %q", roleErr.RequiredRole, "admin")
	}
}

func TestClassifyHTTPError_404ClassifiedAsScopeError(t *testing.T) {
	httpErr := newHTTPError(http.StatusNotFound, "Not Found", http.Header{})

	got := classifyHTTPError(httpErr, "update repository settings")

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope for 404, got %T: %v", got, got)
	}
	if scopeErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", scopeErr.StatusCode, http.StatusNotFound)
	}
}

func TestClassifyHTTPError_404AdminRoleOperation(t *testing.T) {
	httpErr := newHTTPError(http.StatusNotFound, "Not Found", http.Header{})

	got := classifyHTTPError(httpErr, "enable vulnerability alerts")

	var roleErr *ErrInsufficientRole
	if !errors.As(got, &roleErr) {
		t.Fatalf("expected *ErrInsufficientRole for 404 on admin-role op, got %T: %v", got, got)
	}
	if roleErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want %d", roleErr.StatusCode, http.StatusNotFound)
	}
}

func TestClassifyHTTPError_WrappedHTTPError(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-Accepted-OAuth-Scopes", "repo")
	httpErr := newHTTPError(http.StatusForbidden, "Forbidden", headers)
	wrapped := fmt.Errorf("applying settings: %w", httpErr)

	got := classifyHTTPError(wrapped, "update repository settings")

	var scopeErr *ErrInsufficientScope
	if !errors.As(got, &scopeErr) {
		t.Fatalf("expected *ErrInsufficientScope from wrapped error, got %T: %v", got, got)
	}
	if scopeErr.Operation != "update repository settings" {
		t.Errorf("Operation = %q, want %q", scopeErr.Operation, "update repository settings")
	}
}

func TestClassifyHTTPError_EmptyScopeHeaders(t *testing.T) {
	httpErr := newHTTPError(http.StatusForbidden, "Resource not accessible by integration", http.Header{})

	got := classifyHTTPError(httpErr, "update repository settings")

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
			if len(got) != len(tt.want) {
				t.Fatalf("parseCSVScopes(%q) len = %d, want %d", tt.input, len(got), len(tt.want))
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseCSVScopes(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}
