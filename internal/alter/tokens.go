package alter

import (
	"bytes"
	"fmt"

	"github.com/wimpysworld/tailor/internal/config"
)

// TokenContext holds resolved values for template substitution.
type TokenContext struct {
	GitHubUsername string                     // from GET /user
	Owner          string                     // from repo context; empty if no context
	Name           string                     // from repo context; empty if no context
	Repository     *config.RepositorySettings // from config; nil if absent
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

// HomepageURL returns the constructed homepage URL, or the raw token if no repo context.
func (tc *TokenContext) HomepageURL() string {
	if !tc.HasRepoContext() {
		return "{{HOMEPAGE_URL}}"
	}
	return fmt.Sprintf("https://github.com/%s/%s", tc.Owner, tc.Name)
}

// MergeStrategy returns the highest-preference enabled method
// (squash > rebase > merge), defaulting to --squash.
func (tc *TokenContext) MergeStrategy() string {
	if tc.Repository == nil {
		return "--squash"
	}

	type method struct {
		enabled *bool
		flag    string
	}
	methods := []method{
		{tc.Repository.AllowSquashMerge, "--squash"},
		{tc.Repository.AllowRebaseMerge, "--rebase"},
		{tc.Repository.AllowMergeCommit, "--merge"},
	}

	for _, m := range methods {
		if m.enabled != nil && *m.enabled {
			return m.flag
		}
	}
	return "--squash"
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
	case ".tailor/config.yml":
		return bytes.ReplaceAll(content, []byte("{{HOMEPAGE_URL}}"), []byte(tc.HomepageURL()))
	case ".github/workflows/tailor-automerge.yml":
		return bytes.ReplaceAll(content, []byte("{{MERGE_STRATEGY}}"), []byte(tc.MergeStrategy()))
	default:
		return content
	}
}
