package alter_test

import (
	"bytes"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
)

func TestHasRepoContext(t *testing.T) {
	tests := []struct {
		name string
		tc   alter.TokenContext
		want bool
	}{
		{"both set", alter.TokenContext{Owner: "org", Name: "repo"}, true},
		{"owner empty", alter.TokenContext{Owner: "", Name: "repo"}, false},
		{"name empty", alter.TokenContext{Owner: "org", Name: ""}, false},
		{"both empty", alter.TokenContext{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tc.HasRepoContext(); got != tt.want {
				t.Errorf("HasRepoContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubstituteFundingYml(t *testing.T) {
	tc := &alter.TokenContext{GitHubUsername: "octocat"}
	input := []byte("github: {{GITHUB_USERNAME}}\n")
	got := tc.Substitute(input, ".github/FUNDING.yml")
	want := []byte("github: octocat\n")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteSecurityMdWithRepoContext(t *testing.T) {
	tc := &alter.TokenContext{Owner: "org", Name: "repo"}
	input := []byte("Report: {{ADVISORY_URL}}\n")
	got := tc.Substitute(input, "SECURITY.md")
	want := []byte("Report: https://github.com/org/repo/security/advisories/new\n")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteSecurityMdWithoutRepoContext(t *testing.T) {
	tc := &alter.TokenContext{}
	input := []byte("Report: {{ADVISORY_URL}}\n")
	got := tc.Substitute(input, "SECURITY.md")
	if !bytes.Equal(got, input) {
		t.Errorf("expected no substitution, got %q", got)
	}
}

func TestSubstituteConfigYmlWithRepoContext(t *testing.T) {
	tc := &alter.TokenContext{Owner: "org", Name: "repo"}
	input := []byte("url: {{SUPPORT_URL}}\n")
	got := tc.Substitute(input, ".github/ISSUE_TEMPLATE/config.yml")
	want := []byte("url: https://github.com/org/repo/blob/HEAD/SUPPORT.md\n")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteConfigYmlWithoutRepoContext(t *testing.T) {
	tc := &alter.TokenContext{}
	input := []byte("url: {{SUPPORT_URL}}\n")
	got := tc.Substitute(input, ".github/ISSUE_TEMPLATE/config.yml")
	if !bytes.Equal(got, input) {
		t.Errorf("expected no substitution, got %q", got)
	}
}

func TestHomepageURLWithRepoContext(t *testing.T) {
	tc := &alter.TokenContext{Owner: "org", Name: "repo"}
	got := tc.HomepageURL()
	want := "https://github.com/org/repo"
	if got != want {
		t.Errorf("HomepageURL() = %q, want %q", got, want)
	}
}

func TestHomepageURLWithoutRepoContext(t *testing.T) {
	tc := &alter.TokenContext{}
	got := tc.HomepageURL()
	want := "{{HOMEPAGE_URL}}"
	if got != want {
		t.Errorf("HomepageURL() = %q, want %q", got, want)
	}
}

func TestSubstituteTailorConfigYmlWithRepoContext(t *testing.T) {
	tc := &alter.TokenContext{Owner: "org", Name: "repo"}
	input := []byte("homepage: \"{{HOMEPAGE_URL}}\"\n")
	got := tc.Substitute(input, ".tailor/config.yml")
	want := []byte("homepage: \"https://github.com/org/repo\"\n")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteTailorConfigYmlWithoutRepoContext(t *testing.T) {
	tc := &alter.TokenContext{}
	input := []byte("homepage: \"{{HOMEPAGE_URL}}\"\n")
	got := tc.Substitute(input, ".tailor/config.yml")
	if !bytes.Equal(got, input) {
		t.Errorf("expected no substitution, got %q", got)
	}
}

func TestHasSubstitutionTailorConfigYml(t *testing.T) {
	tc := &alter.TokenContext{}
	if !tc.HasSubstitution(".tailor/config.yml") {
		t.Error("HasSubstitution(.tailor/config.yml) = false, want true")
	}
}

func TestMergeStrategy(t *testing.T) {
	tests := []struct {
		name string
		repo *config.RepositorySettings
		want string
	}{
		{
			name: "nil repository defaults to squash",
			repo: nil,
			want: "--squash",
		},
		{
			name: "no methods explicitly set defaults to squash",
			repo: &config.RepositorySettings{},
			want: "--squash",
		},
		{
			name: "only squash enabled",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(true),
				AllowRebaseMerge: boolPtr(false),
				AllowMergeCommit: boolPtr(false),
			},
			want: "--squash",
		},
		{
			name: "only rebase enabled",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(false),
				AllowRebaseMerge: boolPtr(true),
				AllowMergeCommit: boolPtr(false),
			},
			want: "--rebase",
		},
		{
			name: "only merge enabled",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(false),
				AllowRebaseMerge: boolPtr(false),
				AllowMergeCommit: boolPtr(true),
			},
			want: "--merge",
		},
		{
			name: "squash and rebase enabled prefers squash",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(true),
				AllowRebaseMerge: boolPtr(true),
				AllowMergeCommit: boolPtr(false),
			},
			want: "--squash",
		},
		{
			name: "rebase and merge enabled prefers rebase",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(false),
				AllowRebaseMerge: boolPtr(true),
				AllowMergeCommit: boolPtr(true),
			},
			want: "--rebase",
		},
		{
			name: "all enabled prefers squash",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(true),
				AllowRebaseMerge: boolPtr(true),
				AllowMergeCommit: boolPtr(true),
			},
			want: "--squash",
		},
		{
			name: "all explicitly disabled defaults to squash",
			repo: &config.RepositorySettings{
				AllowSquashMerge: boolPtr(false),
				AllowRebaseMerge: boolPtr(false),
				AllowMergeCommit: boolPtr(false),
			},
			want: "--squash",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := &alter.TokenContext{Repository: tt.repo}
			if got := tc.MergeStrategy(); got != tt.want {
				t.Errorf("MergeStrategy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSubstituteAutomergeWorkflow(t *testing.T) {
	tc := &alter.TokenContext{
		Repository: &config.RepositorySettings{
			AllowRebaseMerge: boolPtr(true),
			AllowSquashMerge: boolPtr(false),
		},
	}
	input := []byte("gh pr merge --auto {{MERGE_STRATEGY}} \"$PR_URL\"")
	got := tc.Substitute(input, ".github/workflows/tailor-automerge.yml")
	want := []byte("gh pr merge --auto --rebase \"$PR_URL\"")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHasSubstitutionAutomergeWorkflow(t *testing.T) {
	tc := &alter.TokenContext{}
	if !tc.HasSubstitution(".github/workflows/tailor-automerge.yml") {
		t.Error("HasSubstitution(.github/workflows/tailor-automerge.yml) = false, want true")
	}
}

func TestSubstitutePassthroughOtherSources(t *testing.T) {
	tc := &alter.TokenContext{GitHubUsername: "octocat", Owner: "org", Name: "repo"}
	input := []byte("some content with {{GITHUB_USERNAME}} and {{ADVISORY_URL}}")
	got := tc.Substitute(input, "CODE_OF_CONDUCT.md")
	if !bytes.Equal(got, input) {
		t.Errorf("expected passthrough, got %q", got)
	}
}
