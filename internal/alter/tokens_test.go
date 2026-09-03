package alter_test

import (
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestSubstituteRepoContext(t *testing.T) {
	advisory := []byte("Report: {{ADVISORY_URL}}\n")
	support := []byte("url: \"{{SUPPORT_URL}}\"\n")
	tests := []struct {
		name         string
		tc           alter.TokenContext
		wantAdvisory []byte
		wantSupport  []byte
	}{
		{
			"both set",
			alter.TokenContext{Owner: "org", Name: "repo"},
			[]byte("Report: https://github.com/org/repo/security/advisories/new\n"),
			[]byte("url: \"https://github.com/org/repo/blob/HEAD/SUPPORT.md\"\n"),
		},
		{"owner empty", alter.TokenContext{Owner: "", Name: "repo"}, advisory, support},
		{"name empty", alter.TokenContext{Owner: "org", Name: ""}, advisory, support},
		{"both empty", alter.TokenContext{}, advisory, support},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tc.Substitute(advisory, "SECURITY.md"); !bytes.Equal(got, tt.wantAdvisory) {
				t.Errorf("SECURITY.md: got %q, want %q", got, tt.wantAdvisory)
			}
			if got := tt.tc.Substitute(support, ".github/ISSUE_TEMPLATE/config.yml"); !bytes.Equal(got, tt.wantSupport) {
				t.Errorf("config.yml: got %q, want %q", got, tt.wantSupport)
			}
		})
	}
}

func TestSubstituteFundingYml(t *testing.T) {
	tc := &alter.TokenContext{GitHubUsername: "octocat"}
	input := []byte("github: \"{{GITHUB_USERNAME}}\"\n")
	got := tc.Substitute(input, ".github/FUNDING.yml")
	want := []byte("github: \"octocat\"\n")
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
	input := []byte("url: \"{{SUPPORT_URL}}\"\n")
	got := tc.Substitute(input, ".github/ISSUE_TEMPLATE/config.yml")
	want := []byte("url: \"https://github.com/org/repo/blob/HEAD/SUPPORT.md\"\n")
	if !bytes.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSubstituteConfigYmlWithoutRepoContext(t *testing.T) {
	tc := &alter.TokenContext{}
	input := []byte("url: \"{{SUPPORT_URL}}\"\n")
	got := tc.Substitute(input, ".github/ISSUE_TEMPLATE/config.yml")
	if !bytes.Equal(got, input) {
		t.Errorf("expected no substitution, got %q", got)
	}
}

func TestTokenCountsInEmbeddedSwatches(t *testing.T) {
	tests := []struct {
		path  string
		token string
		count int
	}{
		{".github/FUNDING.yml", "{{GITHUB_USERNAME}}", 1},
		{".github/ISSUE_TEMPLATE/config.yml", "{{SUPPORT_URL}}", 1},
		{"SECURITY.md", "{{ADVISORY_URL}}", 1},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			source, err := swatch.Content(tt.path)
			if err != nil {
				t.Fatalf("swatch.Content(%q) error: %v", tt.path, err)
			}
			if got := bytes.Count(source, []byte(tt.token)); got != tt.count {
				t.Errorf("%s occurs %d times in %s, want %d", tt.token, got, tt.path, tt.count)
			}
		})
	}
}

func TestSubstitutedYamlSwatchesParseAsStrings(t *testing.T) {
	tc := &alter.TokenContext{GitHubUsername: "octocat", Owner: "org", Name: "repo"}

	t.Run(".github/FUNDING.yml", func(t *testing.T) {
		source, err := swatch.Content(".github/FUNDING.yml")
		if err != nil {
			t.Fatalf("swatch.Content error: %v", err)
		}
		got := tc.Substitute(source, ".github/FUNDING.yml")
		var doc map[string]any
		if err := yaml.Unmarshal(got, &doc); err != nil {
			t.Fatalf("substituted FUNDING.yml is not valid YAML: %v", err)
		}
		github, ok := doc["github"].(string)
		if !ok {
			t.Fatalf("github key is %T, want string", doc["github"])
		}
		if github != "octocat" {
			t.Errorf("github = %q, want %q", github, "octocat")
		}
	})

	t.Run(".github/ISSUE_TEMPLATE/config.yml", func(t *testing.T) {
		source, err := swatch.Content(".github/ISSUE_TEMPLATE/config.yml")
		if err != nil {
			t.Fatalf("swatch.Content error: %v", err)
		}
		got := tc.Substitute(source, ".github/ISSUE_TEMPLATE/config.yml")
		var doc struct {
			ContactLinks []map[string]any `yaml:"contact_links"`
		}
		if err := yaml.Unmarshal(got, &doc); err != nil {
			t.Fatalf("substituted config.yml is not valid YAML: %v", err)
		}
		if len(doc.ContactLinks) != 1 {
			t.Fatalf("contact_links has %d entries, want 1", len(doc.ContactLinks))
		}
		url, ok := doc.ContactLinks[0]["url"].(string)
		if !ok {
			t.Fatalf("url key is %T, want string", doc.ContactLinks[0]["url"])
		}
		want := "https://github.com/org/repo/blob/HEAD/SUPPORT.md"
		if url != want {
			t.Errorf("url = %q, want %q", url, want)
		}
	})
}

func TestSubstitutePassthroughOtherSources(t *testing.T) {
	tc := &alter.TokenContext{GitHubUsername: "octocat", Owner: "org", Name: "repo"}
	input := []byte("some content with {{GITHUB_USERNAME}} and {{ADVISORY_URL}}")
	got := tc.Substitute(input, "CODE_OF_CONDUCT.md")
	if !bytes.Equal(got, input) {
		t.Errorf("expected passthrough, got %q", got)
	}
}
