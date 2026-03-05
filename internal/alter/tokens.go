package alter

import (
	"bytes"
	"fmt"
)

// TokenContext holds resolved values for template substitution.
type TokenContext struct {
	GitHubUsername string // from GET /user
	Owner          string // from repo context; empty if no context
	Name           string // from repo context; empty if no context
}

// HasRepoContext reports whether owner and name are set.
func (tc *TokenContext) HasRepoContext() bool {
	return tc.Owner != "" && tc.Name != ""
}

// AdvisoryURL returns the constructed advisory URL, or the raw token if no repo context.
func (tc *TokenContext) AdvisoryURL() string {
	if !tc.HasRepoContext() {
		return "{{ADVISORY_URL}}"
	}
	return fmt.Sprintf("https://github.com/%s/%s/security/advisories/new", tc.Owner, tc.Name)
}

// SupportURL returns the constructed support URL, or the raw token if no repo context.
func (tc *TokenContext) SupportURL() string {
	if !tc.HasRepoContext() {
		return "{{SUPPORT_URL}}"
	}
	return fmt.Sprintf("https://github.com/%s/%s/blob/HEAD/SUPPORT.md", tc.Owner, tc.Name)
}

// HasSubstitution reports whether the given source contains token placeholders.
func (tc *TokenContext) HasSubstitution(source string) bool {
	switch source {
	case ".github/FUNDING.yml", "SECURITY.md", ".github/ISSUE_TEMPLATE/config.yml":
		return true
	default:
		return false
	}
}

// Substitute replaces tokens in content based on the swatch source.
func (tc *TokenContext) Substitute(content []byte, source string) []byte {
	switch source {
	case ".github/FUNDING.yml":
		return bytes.ReplaceAll(content, []byte("{{GITHUB_USERNAME}}"), []byte(tc.GitHubUsername))
	case "SECURITY.md":
		return bytes.ReplaceAll(content, []byte("{{ADVISORY_URL}}"), []byte(tc.AdvisoryURL()))
	case ".github/ISSUE_TEMPLATE/config.yml":
		return bytes.ReplaceAll(content, []byte("{{SUPPORT_URL}}"), []byte(tc.SupportURL()))
	default:
		return content
	}
}
