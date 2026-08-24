package gh

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cli/go-gh/v2/pkg/api"
)

// ErrInsufficientScope signals the token lacks a required scope.
type ErrInsufficientScope struct {
	StatusCode  int
	HaveScopes  []string // parsed from X-OAuth-Scopes (empty for fine-grained tokens)
	NeedScopes  []string // parsed from X-Accepted-OAuth-Scopes
	Message     string   // from JSON body
	DocumentURL string   // from JSON body
	Operation   string   // e.g. "enable vulnerability alerts"
}

func (e *ErrInsufficientScope) Error() string {
	msg := fmt.Sprintf("%s: insufficient scope (have: %v, need: %v): %s",
		e.Operation, e.HaveScopes, e.NeedScopes, e.Message)
	if e.DocumentURL != "" {
		msg += fmt.Sprintf(" (see %s)", e.DocumentURL)
	}
	return msg
}

// isAccessError returns true when err is an *ErrInsufficientScope, indicating
// the token lacks permission for the operation.
func isAccessError(err error) bool {
	var scope *ErrInsufficientScope
	return errors.As(err, &scope)
}

// parseCSVScopes splits a comma-separated scope header value into a slice,
// trimming whitespace from each entry. Returns nil for an empty string.
func parseCSVScopes(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	scopes := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

// classifyHTTPError inspects err for a *api.HTTPError and, on 403 or 404,
// returns an *ErrInsufficientScope. Non-HTTP errors
// and non-403/404 HTTP errors pass through unchanged.
func classifyHTTPError(err error, operation string) error {
	if err == nil {
		return nil
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	if httpErr.StatusCode != http.StatusForbidden && httpErr.StatusCode != http.StatusNotFound {
		return err
	}

	haveScopes := parseCSVScopes(httpErr.Headers.Get("X-OAuth-Scopes"))
	needScopes := parseCSVScopes(httpErr.Headers.Get("X-Accepted-OAuth-Scopes"))

	// api.HTTPError (go-gh v2.13.0) does not expose the documentation_url
	// field from the JSON error body. DocumentURL is left empty until
	// upstream adds support or response-body parsing supplies it here.

	return &ErrInsufficientScope{
		StatusCode: httpErr.StatusCode,
		HaveScopes: haveScopes,
		NeedScopes: needScopes,
		Message:    httpErr.Message,
		Operation:  operation,
	}
}

// boundedHTTPError limits the text that Tailor can render. go-gh reads
// the complete response body before it returns, so this is not an allocation
// limit for the response body.
func boundedHTTPError(err error) error {
	if err == nil {
		return nil
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	const maxDetails = 3
	const maxTextLength = 256
	bounded := &api.HTTPError{
		Headers:    httpErr.Headers.Clone(),
		Message:    boundedSanitisedText(httpErr.Message, maxTextLength, true),
		RequestURL: httpErr.RequestURL,
		StatusCode: httpErr.StatusCode,
	}
	details := make([]string, 0, min(len(httpErr.Errors), maxDetails))
	for _, item := range httpErr.Errors {
		detail := boundedSanitisedText(item.Message, maxTextLength, false)
		if detail == "" {
			continue
		}
		details = append(details, detail)
		item.Message = detail
		bounded.Errors = append(bounded.Errors, item)
		if len(details) == maxDetails {
			break
		}
	}
	if len(details) != 0 {
		if bounded.Message == "" {
			bounded.Message = "GitHub API request failed"
		}
		bounded.Message += ": " + strings.Join(details, "; ")
	}
	return bounded
}

func boundedSanitisedText(input string, limit int, firstLineOnly bool) string {
	var output strings.Builder
	truncated := false
	for _, r := range input {
		if firstLineOnly && r == '\n' {
			break
		}
		if unicode.IsControl(r) {
			r = ' '
		}
		if output.Len()+utf8.RuneLen(r) > limit {
			truncated = true
			break
		}
		output.WriteRune(r)
	}
	text := strings.TrimSpace(output.String())
	if truncated {
		text += "..."
	}
	return text
}
