package alter

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestSetupWriteHardErrorReport(t *testing.T) {
	tests := []struct {
		section   string
		path      string
		live      string
		cfg       *config.Config
		process   func(*config.Config, ApplyMode, RepoTarget) ([]RepoSettingResult, error)
		unchanged string
	}{
		{
			section: "code_scanning", path: "/code-scanning/default-setup",
			live: `{"state":"not-configured","query_suite":"default"}`,
			cfg: &config.Config{CodeScanning: &model.CodeScanningSettings{
				State: new("configured"), QuerySuite: new("default"),
			}},
			process: ProcessCodeScanning, unchanged: "query_suite",
		},
		{
			section: "code_quality", path: "/code-quality/setup",
			live: `{"state":"not-configured","languages":["go"]}`,
			cfg: &config.Config{CodeQuality: &model.CodeQualitySettings{
				State: new("configured"), Languages: &[]string{"go"},
			}},
			process: ProcessCodeQuality, unchanged: "languages",
		},
		{
			section: "ruleset", path: "/rulesets/42",
			live: `{"id":42,"name":"Tailor","enforcement":"active","bypass_actors":[],"rules":[{"type":"deletion"}]}`,
			cfg: &config.Config{Ruleset: &model.RulesetSettings{
				Enforcement: new("disabled"), Rules: &model.RulesetRules{Deletion: new(true)},
			}},
			process: ProcessRuleset, unchanged: "rules.deletion",
		},
	}
	for _, tt := range tests {
		for _, status := range []int{http.StatusUnprocessableEntity, http.StatusInternalServerError} {
			for _, mode := range []ApplyMode{Apply, Recut} {
				t.Run(fmt.Sprintf("%s/%s/%d", tt.section, http.StatusText(status), mode), func(t *testing.T) {
					writes := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if r.URL.Path == "/repos/acme/widget/rulesets" && r.Method == http.MethodGet {
							_, _ = io.WriteString(w, `[{"id":42,"name":"Tailor","target":"branch","source_type":"Repository"}]`)
							return
						}
						if r.URL.Path != "/repos/acme/widget"+tt.path {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							http.NotFound(w, r)
							return
						}
						if r.Method == http.MethodGet {
							_, _ = io.WriteString(w, tt.live)
							return
						}
						writes++
						w.WriteHeader(status)
						_, _ = io.WriteString(w, `{"message":"write rejected"}`)
					}))
					defer server.Close()
					target := RepoTarget{Client: testutil.NewTestClient(t, server), Owner: "acme", Name: "widget", HasRepo: true}
					results, err := tt.process(tt.cfg, mode, target)
					if err == nil || !strings.Contains(err.Error(), "write rejected") {
						t.Fatalf("error = %v, want rejected write", err)
					}
					if writes != 1 {
						t.Fatalf("writes = %d, want 1", writes)
					}
					if len(results) != 1 || results[0].Category != RepoNoChange || results[0].Field != tt.unchanged {
						t.Fatalf("results = %#v, want only unchanged %s", results, tt.unchanged)
					}
					report := buildReport("alter", "acme/widget", results, nil, nil, nil, mode)
					if len(report.Document.Items) != 1 || report.Document.Items[0].Outcome != output.Unchanged {
						t.Errorf("report items = %#v, want only unchanged result", report.Document.Items)
					}
					if strings.Contains(report.Plain, "set:") || !strings.Contains(report.Plain, tt.section+"."+tt.unchanged+" (already ") {
						t.Errorf("plain report = %q, want only unchanged result", report.Plain)
					}
				})
			}
		}
	}
}

func TestApplySetupHardErrorWithoutUnchangedResults(t *testing.T) {
	wantErr := errors.New("connection lost")
	results, err := applySetup([]RepoSettingResult{
		{Section: "code_quality", Field: "state", Category: WouldSet, Value: "configured"},
		{Section: "code_quality", Field: "languages", Category: WouldSet, Value: "go"},
	}, func() error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
	if len(results) != 0 {
		t.Errorf("results = %#v, want no unconfirmed changes", results)
	}
	if report := buildReport("alter", "acme/widget", results, nil, nil, nil, Apply); len(report.Document.Items) != 0 || report.Plain != "" {
		t.Errorf("report = %#v, want no unconfirmed changes", report)
	}
}
