package gh

import (
	"bytes"
	"encoding/json"
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
	StatusCode int
	HaveScopes []string // parsed from X-OAuth-Scopes (empty for fine-grained tokens)
	NeedScopes []string // parsed from X-Accepted-OAuth-Scopes
	Message    string   // from JSON body
	Operation  Operation
}

func (e *ErrInsufficientScope) Error() string {
	// A successful response that omits an admin-only block carries no scope
	// headers, so the message stands alone.
	if e.StatusCode < http.StatusBadRequest {
		return fmt.Sprintf("%s: %s", e.Operation, e.Message)
	}
	return fmt.Sprintf("%s: insufficient scope (have: %v, need: %v): %s",
		e.Operation, e.HaveScopes, e.NeedScopes, e.Message)
}

// isAccessError returns true when err is an *ErrInsufficientScope, indicating
// the token lacks permission for the operation.
func isAccessError(err error) bool {
	var scope *ErrInsufficientScope
	return errors.As(err, &scope)
}

// ErrRateLimited signals the API rejected the request because the caller
// exhausted a rate limit.
type ErrRateLimited struct {
	StatusCode int
	Message    string // from JSON body
	RetryAfter string // from Retry-After header, empty when absent
	Operation  Operation
}

func (e *ErrRateLimited) Error() string {
	msg := fmt.Sprintf("%s: rate limited (HTTP %d): %s", e.Operation, e.StatusCode, e.Message)
	if e.RetryAfter != "" {
		msg += fmt.Sprintf(" (retry after %s)", e.RetryAfter)
	}
	return msg
}

// isRateLimitError returns true when err is an *ErrRateLimited.
func isRateLimitError(err error) bool {
	var limited *ErrRateLimited
	return errors.As(err, &limited)
}

// isRateLimitHTTPError reports whether httpErr is a rate-limit response:
// any 429, or a 403 with X-RateLimit-Remaining: 0, a Retry-After header,
// or a rate-limit message.
func isRateLimitHTTPError(httpErr *api.HTTPError) bool {
	if httpErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if httpErr.StatusCode != http.StatusForbidden {
		return false
	}
	if httpErr.Headers.Get("X-RateLimit-Remaining") == "0" {
		return true
	}
	if httpErr.Headers.Get("Retry-After") != "" {
		return true
	}
	return strings.Contains(strings.ToLower(httpErr.Message), "rate limit")
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

// classifyHTTPError inspects err for a *api.HTTPError. Rate-limit responses
// (any 429, or a 403 that carries rate-limit evidence) return an
// *ErrRateLimited. Other 403 responses and most 404 responses return an
// *ErrInsufficientScope. An update-label 404 passes through because the label
// can disappear after Tailor reads it. Non-HTTP errors and other HTTP errors
// pass through unchanged.
func classifyHTTPError(err error, operation Operation) error {
	if err == nil {
		return nil
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}

	if isRateLimitHTTPError(httpErr) {
		return &ErrRateLimited{
			StatusCode: httpErr.StatusCode,
			Message:    httpErr.Message,
			RetryAfter: httpErr.Headers.Get("Retry-After"),
			Operation:  operation,
		}
	}

	if httpErr.StatusCode != http.StatusForbidden &&
		(httpErr.StatusCode != http.StatusNotFound || operation.Kind == OpUpdateLabel) {
		return err
	}

	haveScopes := parseCSVScopes(httpErr.Headers.Get("X-OAuth-Scopes"))
	needScopes := parseCSVScopes(httpErr.Headers.Get("X-Accepted-OAuth-Scopes"))

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
	// go-gh joins the top-level message and the normalised per-error lines
	// into Message. Validation errors with a non-custom code carry their
	// field detail only in those lines, so details come from Message, not
	// from the per-item Message fields.
	lines := strings.Split(httpErr.Message, "\n")
	bounded := &api.HTTPError{
		Headers:    httpErr.Headers.Clone(),
		Message:    boundedSanitisedText(lines[0], maxTextLength),
		RequestURL: httpErr.RequestURL,
		StatusCode: httpErr.StatusCode,
	}
	details := make([]string, 0, min(len(lines)-1, maxDetails))
	for _, line := range lines[1:] {
		detail := boundedSanitisedText(line, maxTextLength)
		if detail == "" {
			continue
		}
		details = append(details, detail)
		if len(details) == maxDetails {
			break
		}
	}
	for _, item := range httpErr.Errors {
		item.Message = boundedSanitisedText(item.Message, maxTextLength)
		bounded.Errors = append(bounded.Errors, item)
		if len(bounded.Errors) == maxDetails {
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

func boundedSanitisedText(input string, limit int) string {
	var output strings.Builder
	truncated := false
	for _, r := range input {
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

// sendJSON marshals body, sends it with the given method, and discards the
// response. The returned error is bounded by boundedHTTPError.
func sendJSON(client *api.RESTClient, method, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return boundedHTTPError(client.Do(method, path, bytes.NewReader(payload), nil))
}
