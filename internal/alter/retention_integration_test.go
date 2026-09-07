package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestRunRetentionOmittedAfterDefaultMerge(t *testing.T) {
	for _, tc := range []struct {
		name       string
		alteration string
		mode       alter.ApplyMode
	}{
		{"apply defaults", "always", alter.Apply},
		{"preview defaults", "always", alter.DryRun},
		{"recut first-fit", "first-fit", alter.Recut},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := setupAlterTest(t, "license: none\nswatches:\n  - path: .tailor.yml\n    alteration: "+tc.alteration+"\n")
			cfg := loadTestConfig(t, ctx.Dir)
			output := captureAlterRun(t, cfg, ctx.Dir, tc.mode, ctx.Client)
			if cfg.Actions == nil || cfg.Actions.AllowedActions == nil {
				t.Fatal("Run did not merge Actions defaults")
			}
			if cfg.Actions.ArtifactAndLogRetention != nil {
				t.Fatal("Run added retention to the merged config")
			}
			for _, call := range ctx.Calls() {
				if strings.Contains(call.Path, "artifact-and-log-retention") {
					t.Errorf("omitted retention caused a request: %+v", call)
				}
			}
			written, err := os.ReadFile(filepath.Join(ctx.Dir, ".tailor.yml"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(written), "artifact_and_log_retention") || strings.Contains(output, "artifact_and_log_retention") {
				t.Fatal("omitted retention appeared in config or output")
			}
		})
	}
}

func TestRunInvalidRetentionBeforeMutations(t *testing.T) {
	for _, days := range []int{-1, 0, 91} {
		for _, mode := range []alter.ApplyMode{alter.DryRun, alter.Apply, alter.Recut} {
			t.Run(fmt.Sprintf("days=%d/mode=%d", days, mode), func(t *testing.T) {
				original := fmt.Sprintf("license: none\nrepository:\n  has_wiki: false\nactions:\n  artifact_and_log_retention:\n    days: %d\nswatches:\n  - path: .tailor.yml\n    alteration: always\n", days)
				ctx := setupAlterTest(t, original)
				writeOnDisk(t, ctx.Dir, ".github/workflows/tailor.yml", []byte("keep retired workflow"))
				var cfg config.Config
				if err := yaml.Unmarshal([]byte(original), &cfg); err != nil {
					t.Fatal(err)
				}
				err := alter.Run(&cfg, ctx.Dir, mode, ctx.Client, io.Discard, io.Discard)
				if err == nil || !strings.Contains(err.Error(), "between 1 and 90") {
					t.Fatalf("Run error = %v, want retention range error", err)
				}
				if calls := ctx.Calls(); len(calls) != 0 {
					t.Fatalf("invalid config caused API calls: %+v", calls)
				}
				for path, want := range map[string]string{".tailor.yml": original, ".github/workflows/tailor.yml": "keep retired workflow"} {
					got, err := os.ReadFile(filepath.Join(ctx.Dir, path))
					if err != nil || string(got) != want {
						t.Errorf("invalid config changed %s: content %q, error %v", path, got, err)
					}
				}
			})
		}
	}
}

func TestRunRetentionPreservesSelectedTransition(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         alter.ApplyMode
		failSelected bool
	}{
		{"apply", alter.Apply, false},
		{"preview", alter.DryRun, false},
		{"selected write denied", alter.Apply, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := setupAlterTest(t, "license: none\nactions:\n  allowed_actions: selected\n  github_owned_allowed: true\n  verified_allowed: false\n  patterns_allowed: [testowner/*]\n  artifact_and_log_retention:\n    days: 14\n")
			var writes []string
			enabled := true
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := strings.TrimPrefix(r.URL.Path, "/repos/testowner/testrepo/actions/permissions")
				if r.Method == http.MethodGet {
					switch path {
					case "/user":
						fmt.Fprint(w, `{"login":"testuser"}`)
					case "":
						fmt.Fprint(w, `{"enabled":true,"allowed_actions":"local_only","sha_pinning_required":false}`)
					case "/artifact-and-log-retention":
						fmt.Fprint(w, `{"days":30,"maximum_allowed_days":90}`)
					default:
						t.Errorf("unexpected GET %s", r.URL.Path)
						http.NotFound(w, r)
					}
					return
				}
				writes = append(writes, path)
				switch path {
				case "":
					var body struct {
						Enabled bool `json:"enabled"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					enabled = body.Enabled
				case "/selected-actions":
					if enabled {
						t.Error("selected policy changed while Actions remained enabled")
					}
					if tc.failSelected {
						w.WriteHeader(http.StatusForbidden)
						fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
						return
					}
				case "/artifact-and-log-retention":
					var body map[string]int
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 1 || body["days"] != 14 {
						t.Errorf("retention body = %v, error = %v", body, err)
					}
				default:
					t.Errorf("unexpected mutation %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			cfg := loadTestConfig(t, ctx.Dir)
			err := alter.Run(cfg, ctx.Dir, tc.mode, testutil.NewTestClient(t, server), io.Discard, io.Discard)
			if tc.failSelected {
				if err == nil || !strings.Contains(err.Error(), "while actions are disabled") || enabled {
					t.Fatalf("failed transition: error = %v, enabled = %t", err, enabled)
				}
				if !slices.Equal(writes, []string{"", "/selected-actions"}) {
					t.Fatalf("writes after failure = %v", writes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.mode == alter.DryRun {
				if len(writes) != 0 {
					t.Fatalf("preview writes = %v", writes)
				}
			} else if !enabled || !slices.Equal(writes, []string{"", "/selected-actions", "", "/artifact-and-log-retention"}) {
				t.Fatalf("transition writes = %v, enabled = %t", writes, enabled)
			}
		})
	}
}
