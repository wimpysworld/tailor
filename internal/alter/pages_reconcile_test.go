package alter

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestPagesPreflightSkipsEnvironmentWritesWithoutAdminAccess(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".tailor.yml")
	configContent := []byte("pages:\n  enabled: true\n")
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatal(err)
	}
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
			t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/repos/owner/repo":
			w.Header().Set("X-OAuth-Scopes", "repo")
			fmt.Fprint(w, `{"private":false,"default_branch":"main","permissions":{"maintain":true,"admin":false}}`)
		case "/repos/owner/repo/pages":
			fmt.Fprint(w, `{"build_type":"workflow","https_enforced":true}`)
		case "/repos/owner/repo/environments/github-pages":
			fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true}}`)
		case "/repos/owner/repo/environments/github-pages/deployment-branch-policies":
			fmt.Fprint(w, `{"total_count":0,"branch_policies":[]}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	target := RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true}
	cfg := &config.Config{Pages: &model.PagesSettings{Enabled: new(true)}}
	prepared := &pagesPreparation{Generator: "static", Path: "pages", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: swatch.Always}}
	p, err := preflightPages(cfg, dir, Apply, target, prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.skipped) != 1 || p.skipped[0].Category != WouldSkipScope || p.environment != nil {
		t.Fatalf("preflight=%+v", p)
	}
	results, workflow, err := processPages(cfg, dir, Apply, target, p)
	if err != nil || workflow != nil || len(results) != 1 || results[0].Category != WouldSkipScope || writes != 0 {
		t.Fatalf("results=%+v workflow=%+v error=%v writes=%d", results, workflow, err, writes)
	}
	content, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(content, configContent) {
		t.Fatalf("config changed during preflight: %q, error=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(dir, swatch.PagesDestination)); !os.IsNotExist(err) {
		t.Fatalf("workflow exists after skipped preflight: %v", err)
	}
}

func TestPagesPreflightSkipsPrivateRepositoryWorkflowConflicts(t *testing.T) {
	for _, tt := range []struct {
		name       string
		alteration swatch.AlterationMode
		content    string
	}{
		{name: "unowned workflow", alteration: swatch.Always, content: "name: custom\n"},
		{name: "missing never workflow", alteration: swatch.Never},
	} {
		for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
			t.Run(fmt.Sprintf("%s/%v", tt.name, mode), func(t *testing.T) {
				dir := t.TempDir()
				workflowPath := filepath.Join(dir, swatch.PagesDestination)
				if tt.content != "" {
					if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(workflowPath, []byte(tt.content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodGet || r.URL.Path != "/repos/owner/repo" {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
						return
					}
					w.Header().Set("X-OAuth-Scopes", "repo")
					fmt.Fprint(w, `{"private":true,"default_branch":"main","permissions":{"admin":true}}`)
				}))
				defer server.Close()
				target := RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true}
				cfg := &config.Config{Pages: &model.PagesSettings{Enabled: new(true)}}
				prepared := &pagesPreparation{Generator: "static", Path: "pages", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: tt.alteration}}
				p, err := preflightPages(cfg, dir, mode, target, prepared)
				if err != nil {
					t.Fatal(err)
				}
				results, workflow, err := processPages(cfg, dir, mode, target, p)
				if err != nil || workflow != nil || len(results) != 1 || results[0].Category != WouldSkipSetup || results[0].Annotation != "not available" {
					t.Fatalf("results=%+v workflow=%+v error=%v", results, workflow, err)
				}
				content, err := os.ReadFile(workflowPath)
				if tt.content == "" {
					if !os.IsNotExist(err) {
						t.Fatalf("workflow exists after skipped preflight: %v", err)
					}
				} else if err != nil || string(content) != tt.content {
					t.Fatalf("workflow changed: %q, error=%v", content, err)
				}
			})
		}
	}
}

func TestPagesPartialReconciliation(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) {
			dir := t.TempDir()
			created, homepageWrites := false, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/repos/owner/repo/actions/permissions":
					fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all"}`)
				case strings.HasSuffix(r.URL.Path, "/environments/github-pages"):
					fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null}`)
				case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/pages":
					created = true
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pages":
					if pending {
						fmt.Fprint(w, `{"build_type":"workflow","html_url":"https://owner.github.io/repo/","https_enforced":false}`)
					} else {
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprint(w, `{"message":"failed"}`)
					}
				case r.Method == http.MethodPatch:
					homepageWrites++
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			var stderr bytes.Buffer
			target := RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true, Stderr: &stderr}
			environment, err := gh.ReadPagesEnvironment(target.Client, "owner", "repo", "main", true)
			if err != nil {
				t.Fatal(err)
			}
			prepared := &pagesPreparation{Generator: "static", Path: "pages", Branch: "main", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: swatch.Always}}
			if err := preparePagesWorkflow(prepared, dir, Apply, "main"); err != nil {
				t.Fatal(err)
			}
			p := &pagesRun{prepared: prepared, repository: &gh.PagesRepositoryState{Homepage: "https://github.com/owner/repo"}, site: &gh.PagesState{}, environment: environment}
			cfg := &config.Config{Pages: &model.PagesSettings{Enabled: new(true)}}
			results, workflow, err := processPages(cfg, dir, Apply, target, p)
			if (err == nil) != pending {
				t.Fatalf("pending=%v, error=%v", pending, err)
			}
			if !created || homepageWrites != 0 {
				t.Fatalf("created=%v homepage writes=%d", created, homepageWrites)
			}
			output := FormatOutput(results, nil, nil, nil, Apply)
			if !strings.Contains(output, "pages.build_type = workflow") || strings.Contains(output, "pages.https_enforced = true") {
				t.Fatal(output)
			}
			_, fileErr := os.Stat(filepath.Join(dir, swatch.PagesDestination))
			if pending {
				if workflow == nil || fileErr != nil || !strings.Contains(stderr.String(), "wait for the HTTPS certificate") {
					t.Fatalf("workflow=%v file error=%v guidance=%s", workflow, fileErr, stderr.String())
				}
			} else if workflow != nil || !os.IsNotExist(fileErr) {
				t.Fatalf("workflow written after hard error: %v %v", workflow, fileErr)
			}
		})
	}
}

func TestPagesWorkflowRecheckPreservesCurrentFile(t *testing.T) {
	for _, tt := range []struct {
		name       string
		alteration swatch.AlterationMode
		exists     bool
		category   SwatchCategory
		reason     SwatchReason
	}{
		{name: "first-fit created", alteration: swatch.FirstFit, category: Skipped, reason: SkipFirstFitExists},
		{name: "always created", alteration: swatch.Always, category: NoChange},
		{name: "always updated", alteration: swatch.Always, exists: true, category: NoChange},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			workflowPath := filepath.Join(dir, swatch.PagesDestination)
			if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.exists {
				content, err := swatch.PagesContent("static", "pages", "old")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(workflowPath, content, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
				}
				switch r.URL.Path {
				case "/repos/owner/repo":
					w.Header().Set("X-OAuth-Scopes", "repo")
					fmt.Fprint(w, `{"private":false,"default_branch":"main","permissions":{"admin":true}}`)
				case "/repos/owner/repo/pages":
					fmt.Fprint(w, `{"build_type":"workflow","https_enforced":true}`)
				case "/repos/owner/repo/environments/github-pages":
					fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null}`)
				case "/repos/owner/repo/actions/permissions":
					fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all"}`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			target := RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "owner", Name: "repo", HasRepo: true}
			cfg := &config.Config{Pages: &model.PagesSettings{Enabled: new(true)}}
			prepared := &pagesPreparation{Generator: "static", Path: "pages", Entry: config.SwatchEntry{Path: swatch.PagesDestination, Alteration: tt.alteration}}
			p, err := preflightPages(cfg, dir, Apply, target, prepared)
			if err != nil {
				t.Fatal(err)
			}
			wantInitial := WouldCopy
			if tt.exists {
				wantInitial = WouldOverwrite
			}
			if prepared.Result.Category != wantInitial {
				t.Fatalf("initial result = %+v, want %s", prepared.Result, wantInitial)
			}
			content := bytes.Clone(prepared.Content)
			if tt.alteration == swatch.FirstFit {
				content = append(content, []byte("\n# Keep this local comment.\n")...)
			}
			if err := os.WriteFile(workflowPath, content, 0o644); err != nil {
				t.Fatal(err)
			}
			modified := time.Unix(1000000000, 0)
			if err := os.Chtimes(workflowPath, modified, modified); err != nil {
				t.Fatal(err)
			}
			_, workflow, err := processPages(cfg, dir, Apply, target, p)
			if err != nil {
				t.Fatal(err)
			}
			if workflow == nil || workflow.Category != tt.category || workflow.Reason != tt.reason {
				t.Errorf("workflow = %+v, want %s (%s)", workflow, tt.category, tt.reason)
			}
			got, err := os.ReadFile(workflowPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, content) {
				t.Error("workflow changed after the recheck")
			}
			info, err := os.Stat(workflowPath)
			if err != nil {
				t.Fatal(err)
			}
			if !info.ModTime().Equal(modified) {
				t.Error("workflow was rewritten after the recheck")
			}
		})
	}
}

func TestPagesHomepagePrecedence(t *testing.T) {
	for _, tt := range []struct {
		name, live, declared, inferred, cname, want string
		explicit                                    bool
	}{
		{name: "repository URL", live: "https://github.com/owner/repo", want: "https://owner.github.io/repo/"},
		{name: "custom domain", live: "https://github.com/owner/repo", cname: "docs.example.com", want: "https://docs.example.com/"},
		{name: "inferred URL", live: "https://github.com/owner/repo", declared: "https://github.com/owner/repo", inferred: "https://github.com/owner/repo", want: "https://owner.github.io/repo/"},
		{name: "explicit empty", live: "https://github.com/owner/repo", explicit: true},
		{name: "explicit URL", live: "https://github.com/owner/repo", explicit: true, declared: "https://example.com"},
		{name: "empty live"},
		{name: "unrelated live", live: "https://example.com"},
		{name: "repeated run", live: "https://owner.github.io/repo/"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{InferredHomepage: tt.inferred}
			if tt.explicit || tt.declared != "" {
				cfg.Repository = &model.RepositorySettings{Homepage: &tt.declared}
			}
			results, err := processPagesHomepage(cfg, DryRun, RepoTarget{Owner: "owner", Name: "repo"}, &gh.PagesRepositoryState{Homepage: tt.live}, &gh.PagesState{Exists: true, HTMLURL: "https://owner.github.io/repo/", CNAME: tt.cname})
			if err != nil {
				t.Fatal(err)
			}
			if tt.want == "" {
				if len(results) != 0 {
					t.Fatalf("unexpected results: %+v", results)
				}
				return
			}
			if len(results) != 1 || results[0].Value != tt.want {
				t.Fatalf("results=%+v want=%s", results, tt.want)
			}
		})
	}
}

func TestPagesActionRestrictions(t *testing.T) {
	for _, tt := range []struct {
		name, action string
		owned        bool
		patterns     []string
		want         bool
	}{
		{name: "GitHub action", action: "actions/checkout@abc", owned: true, want: true},
		{name: "third party blocked", action: "ruby/setup-ruby@abc", owned: true},
		{name: "third party pattern", action: "ruby/setup-ruby@abc", patterns: []string{"ruby/*"}, want: true},
		{name: "excluded SHA", action: "actions/checkout@abc", owned: true, patterns: []string{"!actions/checkout@abc"}},
		{name: "exclusion beats allow", action: "ruby/setup-ruby@abc", patterns: []string{"ruby/*", "!ruby/setup-ruby@*"}},
		{name: "different ref blocked", action: "ruby/setup-ruby@abc", patterns: []string{"ruby/setup-ruby@v1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policy := &model.ActionsSettings{AllowedActions: new("selected"), GitHubOwnedAllowed: &tt.owned, PatternsAllowed: &tt.patterns}
			if got := pagesActionAllowed(policy, tt.action); got != tt.want {
				t.Fatalf("allowed=%v want=%v", got, tt.want)
			}
		})
	}
}
