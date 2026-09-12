package alter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestPagesBranchBeforeFromReader(t *testing.T) {
	for _, tt := range []struct {
		name       string
		missing    bool
		policy     string
		policies   string
		wantBefore string
		wantAdd    bool
	}{
		{name: "missing environment", missing: true, wantBefore: "(none)", wantAdd: true},
		{name: "missing branch", policy: `{"protected_branches":false,"custom_branch_policies":true}`, policies: `{"total_count":1,"branch_policies":[{"id":1,"name":"release","type":"branch"}]}`, wantBefore: "(none)", wantAdd: true},
		{name: "existing branch", policy: `{"protected_branches":false,"custom_branch_policies":true}`, policies: `{"total_count":1,"branch_policies":[{"id":1,"name":"main","type":"branch"}]}`, wantBefore: "main"},
		{name: "unrestricted environment", policy: `null`, wantBefore: "main"},
		{name: "protected branch", policy: `{"protected_branches":true,"custom_branch_policies":false}`, wantBefore: "main"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
				}
				switch r.URL.Path {
				case "/repos/owner/repo/environments/github-pages":
					if tt.missing {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"message":"Not Found"}`)
						return
					}
					fmt.Fprintf(w, `{"name":"github-pages","deployment_branch_policy":%s}`, tt.policy)
				case "/repos/owner/repo/environments/github-pages/deployment-branch-policies":
					fmt.Fprint(w, tt.policies)
				case "/repos/owner/repo/branches/main":
					fmt.Fprint(w, `{"protected":true}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			environment, err := gh.ReadPagesEnvironment(testutil.NewTestClient(t, server), "owner", "repo", "main", true)
			if err != nil {
				t.Fatal(err)
			}
			if environment.Branch != "main" || environment.AddBranch != tt.wantAdd {
				t.Fatalf("reader state = %+v", environment)
			}
			results := comparePages(&config.Config{Pages: &model.PagesSettings{}}, &pagesRun{
				prepared: &pagesPreparation{Branch: "main"}, environment: environment,
				site: &gh.PagesState{Exists: true, BuildType: "workflow", HTTPSEnforced: true},
			})
			var branch RepoSettingResult
			for _, result := range results {
				if result.Field == "branch" {
					branch = result
				}
			}
			wantCategory := RepoNoChange
			if tt.wantAdd {
				wantCategory = WouldSet
			}
			if branch.Before != tt.wantBefore || branch.Value != "main" || branch.Category != wantCategory {
				t.Fatalf("branch result = %+v", branch)
			}
			for _, mode := range []ApplyMode{DryRun, Apply, Recut} {
				report := buildReport("baste", "", []RepoSettingResult{branch}, nil, nil, nil, mode)
				item := report.Document.Items[0]
				wantOutcome := output.Unchanged
				label, text := "no change:", "pages.branch (already main)"
				if tt.wantAdd {
					wantOutcome = output.Alteration
					label, text = "would set:", "pages.branch = main"
					if mode.ShouldWrite() {
						wantOutcome, label = output.Applied, "set:"
					}
				}
				if item.Before != tt.wantBefore || item.After != "main" || item.Outcome != wantOutcome {
					t.Errorf("mode %v: report item = %+v", mode, item)
				}
				if want := fmt.Sprintf("%-37s%s\n", label, text); report.Plain != want {
					t.Errorf("mode %v: plain = %q, want %q", mode, report.Plain, want)
				}
			}
		})
	}
}
