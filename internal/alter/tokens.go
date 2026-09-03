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

// Substitute replaces tokens in content based on the swatch path.
// The repository URL tokens stay unchanged when owner or name is empty.
func (tc *TokenContext) Substitute(content []byte, path string) []byte {
	switch path {
	case ".github/FUNDING.yml":
		return bytes.ReplaceAll(content, []byte("{{GITHUB_USERNAME}}"), []byte(tc.GitHubUsername))
	case "SECURITY.md":
		if tc.Owner == "" || tc.Name == "" {
			return content
		}
		url := fmt.Sprintf("https://github.com/%s/%s/security/advisories/new", tc.Owner, tc.Name)
		return bytes.ReplaceAll(content, []byte("{{ADVISORY_URL}}"), []byte(url))
	case ".github/ISSUE_TEMPLATE/config.yml":
		if tc.Owner == "" || tc.Name == "" {
			return content
		}
		url := fmt.Sprintf("https://github.com/%s/%s/blob/HEAD/SUPPORT.md", tc.Owner, tc.Name)
		return bytes.ReplaceAll(content, []byte("{{SUPPORT_URL}}"), []byte(url))
	default:
		return content
	}
}
