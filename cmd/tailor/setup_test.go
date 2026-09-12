package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestFitWritesDefaultsWithLiveMetadata(t *testing.T) {
	tests := []struct {
		name        string
		metadata    map[string]any
		args        []string
		description string
		homepage    string
	}{
		{name: "live metadata", metadata: map[string]any{"description": "live description", "homepage": "https://example.com"}, description: "live description", homepage: "https://example.com"},
		{name: "empty metadata", metadata: map[string]any{"description": "", "homepage": ""}},
		{name: "null metadata", metadata: map[string]any{"description": nil, "homepage": nil}},
		{name: "absent metadata", metadata: map[string]any{}},
		{name: "description override", metadata: map[string]any{"description": "live description", "homepage": "https://example.com"}, args: []string{"--description=flag description"}, description: "flag description", homepage: "https://example.com"},
		{name: "empty description override", metadata: map[string]any{"description": "live description", "homepage": ""}, args: []string{"--description="}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ghfake.FakeAuth(t, "gho_test")
			ghfake.FakeRepo(t, "octocat", "my-project")
			want, err := config.DefaultConfig("BlueOak-1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			live := map[string]any{}
			for _, field := range model.RepositorySettingFields(want.Repository) {
				if !field.Set {
					continue
				}
				value := field.Value.Elem()
				switch value.Kind() {
				case reflect.Bool:
					live[field.YAMLKey] = !value.Bool()
				case reflect.String:
					live[field.YAMLKey] = "opposite-default"
				}
			}
			live["topics"] = []string{"existing-topic"}
			live["security_and_analysis"] = map[string]any{
				"secret_scanning":                       map[string]string{"status": "disabled"},
				"secret_scanning_push_protection":       map[string]string{"status": "disabled"},
				"secret_scanning_non_provider_patterns": map[string]string{"status": "disabled"},
			}
			maps.Copy(live, tt.metadata)
			managed := map[string]string{
				"/immutable-releases":              `{"enabled":false,"enforced_by_owner":false}`,
				"/code-scanning/default-setup":     `{"state":"not-configured","query_suite":"extended","threat_model":"remote_and_local","languages":["go"]}`,
				"/code-quality/setup":              `{"state":"configured","languages":["go"]}`,
				"/actions/permissions/workflow":    `{"default_workflow_permissions":"read","can_approve_pull_request_reviews":false}`,
				"/actions/permissions":             `{"enabled":false,"allowed_actions":"selected"}`,
				"/private-vulnerability-reporting": `{"enabled":false}`,
				"/automated-security-fixes":        `{"enabled":false}`,
				"/rulesets":                        `[{"id":1,"name":"Tailor","target":"branch","enforcement":"disabled","source_type":"Repository"}]`,
				"/rulesets/1":                      `{"id":1,"name":"Tailor","target":"branch","enforcement":"disabled","bypass_actors":[],"conditions":{"ref_name":{"include":["~ALL"],"exclude":[]}},"rules":[]}`,
				"/labels":                          `[{"name":"live-label","color":"abcdef","description":"live label"}]`,
				"/actions/variables":               `{"total_count":1,"variables":[{"name":"LIVE_VARIABLE","value":"live"}]}`,
				"/pages":                           `{"build_type":"workflow","html_url":"https://octocat.github.io/my-project/"}`,
			}
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.Method+" "+r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/user":
					fmt.Fprint(w, `{"login":"octocat"}`)
				case "/repos/octocat/my-project":
					if err := json.NewEncoder(w).Encode(live); err != nil {
						t.Error(err)
					}
				default:
					t.Errorf("fit requested managed settings: %s %s", r.Method, r.URL.Path)
					if body, ok := managed[strings.TrimPrefix(r.URL.Path, "/repos/octocat/my-project")]; ok {
						fmt.Fprint(w, body)
						return
					}
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"forbidden"}`)
				}
			}))
			t.Cleanup(srv.Close)
			restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
				return testutil.NewTestClient(t, srv), nil
			})
			t.Cleanup(restore)
			dir := t.TempDir()
			var stdout, stderr strings.Builder
			args := append([]string{"fit", dir}, tt.args...)
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("fit = %d, stderr: %s", code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if !reflect.DeepEqual(requests, []string{"GET /user", "GET /repos/octocat/my-project"}) {
				t.Errorf("requests = %v, want only auth and repository metadata", requests)
			}
			got, err := config.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			want.Repository.Description = new(tt.description)
			want.Repository.Homepage = new(tt.homepage)
			want.InferredHomepage = tt.homepage
			expectedDir := t.TempDir()
			if err := config.Write(expectedDir, want, "2026-09-12", "Initially fitted"); err != nil {
				t.Fatal(err)
			}
			want, err = config.Load(expectedDir)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("generated config differs from embedded defaults plus metadata:\ngot: %+v\nwant: %+v", got, want)
			}
			if tt.homepage != "" && got.HomepageDeclared() {
				t.Error("imported homepage lost its inferred provenance")
			}
			if got.Repository.Topics != nil {
				t.Error("fit imported topics")
			}
		})
	}
}

func TestFitMetadataReadFailureStopsCommand(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			ghfake.FakeAuth(t, "gho_test")
			ghfake.FakeRepo(t, "octocat", "my-project")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/user" {
					fmt.Fprint(w, `{"login":"octocat"}`)
					return
				}
				w.WriteHeader(status)
				fmt.Fprint(w, `{"message":"metadata unavailable"}`)
			}))
			t.Cleanup(srv.Close)
			restore := gh.SetNewRESTClientFunc(func(string) (*api.RESTClient, error) {
				return testutil.NewTestClient(t, srv), nil
			})
			t.Cleanup(restore)
			dir := t.TempDir()
			var stdout, stderr strings.Builder
			if code := run([]string{"fit", dir}, &stdout, &stderr); code == 0 {
				t.Fatal("fit succeeded despite metadata read failure")
			}
			if !strings.Contains(stderr.String(), "fetching repo metadata") {
				t.Errorf("stderr = %q", stderr.String())
			}
			if _, err := os.Stat(filepath.Join(dir, ".tailor.yml")); !os.IsNotExist(err) {
				t.Errorf("fit wrote config after metadata failure: %v", err)
			}
		})
	}
}
