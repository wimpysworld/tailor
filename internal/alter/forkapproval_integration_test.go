package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

const forkApprovalPath = "/repos/testowner/testrepo/actions/permissions/fork-pr-contributor-approval"

func TestRunForkApprovalSecondApplyNoChange(t *testing.T) {
	ctx := setupAlterTest(t, "license: none\nactions:\n  fork_pr_contributor_approval:\n    approval_policy: first_time_contributors\n")
	var mu sync.Mutex
	policy := "all_external_contributors"
	var reads, writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/user":
			fmt.Fprint(w, `{"login":"testuser"}`)
		case r.Method == http.MethodGet && r.URL.Path == forkApprovalPath:
			reads++
			fmt.Fprintf(w, `{"approval_policy":%q}`, policy)
		case r.Method == http.MethodPut && r.URL.Path == forkApprovalPath:
			writes++
			var body struct {
				ApprovalPolicy string `json:"approval_policy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			policy = body.ApprovalPolicy
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	first := captureAlterRun(t, loadTestConfig(t, ctx.Dir), ctx.Dir, alter.Apply, client)
	requireContains(t, first, "set:")
	requireContains(t, first, "actions.fork_pr_contributor_approval.approval_policy = first_time_contributors")
	before := forkApprovalFiles(t, ctx.Dir)
	second := captureAlterRun(t, loadTestConfig(t, ctx.Dir), ctx.Dir, alter.Apply, client)
	requireContains(t, second, "no change:")
	requireContains(t, second, "actions.fork_pr_contributor_approval.approval_policy")
	requireNotContains(t, second, "set:")
	if !reflect.DeepEqual(before, forkApprovalFiles(t, ctx.Dir)) {
		t.Error("second apply changed project files")
	}
	mu.Lock()
	defer mu.Unlock()
	if reads != 2 || writes != 1 || policy != "first_time_contributors" {
		t.Errorf("reads = %d, writes = %d, policy = %q, want 2, 1, first_time_contributors", reads, writes, policy)
	}
}

func TestRunForkApprovalDefaultMerge(t *testing.T) {
	for _, tc := range []struct {
		name       string
		alteration string
		mode       alter.ApplyMode
	}{
		{"apply", "always", alter.Apply},
		{"preview", "always", alter.DryRun},
		{"recut", "first-fit", alter.Recut},
	} {
		for _, actions := range []string{"", "actions: {}\n", "actions:\n  fork_pr_contributor_approval: {}\n", "actions:\n  fork_pr_contributor_approval:\n    approval_policy: null\n"} {
			t.Run(tc.name+"/"+actions, func(t *testing.T) {
				original := "license: none\n" + actions + "swatches:\n  - path: .tailor.yml\n    alteration: " + tc.alteration + "\n"
				ctx := setupAlterTest(t, original)
				writeOnDisk(t, ctx.Dir, ".github/workflows/tailor.yml", []byte("retired workflow"))
				before := forkApprovalFiles(t, ctx.Dir)
				cfg := loadTestConfig(t, ctx.Dir)
				output := captureAlterRun(t, cfg, ctx.Dir, tc.mode, ctx.Client)
				assertForkApprovalPolicy(t, cfg, "first_time_contributors")
				requireContains(t, output, "actions.fork_pr_contributor_approval.approval_policy = first_time_contributors")
				var reads, writes int
				for _, call := range ctx.Calls() {
					if call.Path == forkApprovalPath {
						switch call.Method {
						case http.MethodGet:
							reads++
						case http.MethodPut:
							writes++
							if call.Body != `{"approval_policy":"first_time_contributors"}` {
								t.Errorf("approval payload = %s", call.Body)
							}
						default:
							t.Errorf("unexpected approval method %s", call.Method)
						}
					}
				}
				if reads != 1 {
					t.Errorf("approval reads = %d, want 1", reads)
				}
				if tc.mode == alter.DryRun {
					if !reflect.DeepEqual(before, forkApprovalFiles(t, ctx.Dir)) {
						t.Error("preview changed project files")
					}
					for _, call := range ctx.Calls() {
						if call.Method != http.MethodGet {
							t.Errorf("preview mutation: %+v", call)
						}
					}
				} else {
					if writes != 1 {
						t.Errorf("approval writes = %d, want 1", writes)
					}
					assertForkApprovalPolicy(t, loadTestConfig(t, ctx.Dir), "first_time_contributors")
				}
			})
		}
	}
}

func TestRunForkApprovalUnmanagedWithoutMerge(t *testing.T) {
	for _, alteration := range []string{"absent", "never", "first-fit"} {
		for _, actions := range []string{"", "actions: {}\n", "actions:\n  fork_pr_contributor_approval: null\n", "actions:\n  fork_pr_contributor_approval: {}\n", "actions:\n  fork_pr_contributor_approval:\n    approval_policy: null\n"} {
			for _, mode := range []alter.ApplyMode{alter.Apply, alter.DryRun, alter.Recut} {
				if alteration == "first-fit" && mode == alter.Recut {
					continue
				}
				t.Run(fmt.Sprintf("%s/%s/%d", alteration, actions, mode), func(t *testing.T) {
					original := "license: none\n" + actions
					if alteration != "absent" {
						original += "swatches:\n  - path: .tailor.yml\n    alteration: " + alteration + "\n"
					}
					ctx := setupAlterTest(t, original)
					cfg := loadTestConfig(t, ctx.Dir)
					output := captureAlterRun(t, cfg, ctx.Dir, mode, ctx.Client)
					requireNotContains(t, output, "fork_pr_contributor_approval")
					for _, call := range ctx.Calls() {
						if call.Path == forkApprovalPath {
							t.Errorf("unmanaged approval request: %+v", call)
						}
					}
					if cfg.Actions != nil && cfg.Actions.ForkPRContributorApproval != nil && cfg.Actions.ForkPRContributorApproval.ApprovalPolicy != nil {
						t.Error("Run added unmanaged approval")
					}
				})
			}
		}
	}
}

func TestRunForkApprovalPreservesExplicitPolicies(t *testing.T) {
	for _, policy := range model.ForkPRContributorApprovalPolicies {
		for _, mode := range []alter.ApplyMode{alter.Apply, alter.DryRun, alter.Recut} {
			t.Run(fmt.Sprintf("%s/%d", policy, mode), func(t *testing.T) {
				ctx := setupAlterTest(t, "license: none\nactions:\n  fork_pr_contributor_approval:\n    approval_policy: "+policy+"\nswatches:\n  - path: .tailor.yml\n    alteration: always\n")
				cfg := loadTestConfig(t, ctx.Dir)
				captureAlterRun(t, cfg, ctx.Dir, mode, ctx.Client)
				assertForkApprovalPolicy(t, cfg, policy)
				assertForkApprovalPolicy(t, loadTestConfig(t, ctx.Dir), policy)
				for _, call := range ctx.Calls() {
					if call.Path == forkApprovalPath && call.Method == http.MethodPut && call.Body != fmt.Sprintf(`{"approval_policy":%q}`, policy) {
						t.Errorf("explicit policy changed in payload: %s", call.Body)
					}
				}
			})
		}
	}
}

func TestRunInvalidForkApprovalBeforeMutations(t *testing.T) {
	for _, policy := range []string{"", "unknown"} {
		for _, mode := range []alter.ApplyMode{alter.Apply, alter.DryRun, alter.Recut} {
			t.Run(fmt.Sprintf("%q/%d", policy, mode), func(t *testing.T) {
				original := fmt.Sprintf("license: none\nrepository:\n  has_wiki: false\nactions:\n  fork_pr_contributor_approval:\n    approval_policy: %q\nswatches:\n  - path: .tailor.yml\n    alteration: always\n", policy)
				ctx := setupAlterTest(t, original)
				writeOnDisk(t, ctx.Dir, ".github/workflows/tailor.yml", []byte("keep retired workflow"))
				before := forkApprovalFiles(t, ctx.Dir)
				var cfg config.Config
				if err := yaml.Unmarshal([]byte(original), &cfg); err != nil {
					t.Fatal(err)
				}
				err := alter.Run(&cfg, ctx.Dir, mode, ctx.Client, io.Discard, io.Discard)
				if err == nil || !strings.Contains(err.Error(), "approval_policy") {
					t.Fatalf("Run error = %v, want approval policy error", err)
				}
				if calls := ctx.Calls(); len(calls) != 0 {
					t.Errorf("invalid config caused API calls: %+v", calls)
				}
				if !reflect.DeepEqual(before, forkApprovalFiles(t, ctx.Dir)) {
					t.Error("invalid config changed project files")
				}
			})
		}
	}
}

func TestRunForkApprovalAfterSelectedAndRetention(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         alter.ApplyMode
		failSelected bool
	}{
		{"apply", alter.Apply, false},
		{"preview", alter.DryRun, false},
		{"failed transition", alter.Apply, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := setupAlterTest(t, "license: none\nactions:\n  allowed_actions: selected\n  github_owned_allowed: true\n  verified_allowed: false\n  patterns_allowed: [testowner/*]\n  artifact_and_log_retention:\n    days: 14\n  fork_pr_contributor_approval:\n    approval_policy: first_time_contributors\n")
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
					case "/fork-pr-contributor-approval":
						fmt.Fprint(w, `{"approval_policy":"all_external_contributors"}`)
					default:
						t.Errorf("unexpected GET %s", r.URL.Path)
						http.NotFound(w, r)
					}
					return
				}
				if r.Method != http.MethodPut {
					t.Errorf("unexpected mutation method %s", r.Method)
				}
				writes = append(writes, path)
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if path != "/fork-pr-contributor-approval" {
					if _, ok := body["approval_policy"]; ok {
						t.Errorf("approval leaked into %s", path)
					}
				}
				switch path {
				case "":
					enabled, _ = body["enabled"].(bool)
				case "/selected-actions":
					if enabled {
						t.Error("selected policy changed while Actions remained enabled")
					}
					if tc.failSelected {
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprint(w, `{"message":"boom"}`)
						return
					}
				case "/artifact-and-log-retention":
					if len(body) != 1 || body["days"] != float64(14) {
						t.Errorf("retention payload = %v", body)
					}
				case "/fork-pr-contributor-approval":
					if len(body) != 1 || body["approval_policy"] != "first_time_contributors" {
						t.Errorf("approval payload = %v, want only approval_policy", body)
					}
				default:
					t.Errorf("unexpected mutation %s", r.URL.Path)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			err := alter.Run(loadTestConfig(t, ctx.Dir), ctx.Dir, tc.mode, testutil.NewTestClient(t, server), io.Discard, io.Discard)
			if tc.failSelected {
				if err == nil || !strings.Contains(err.Error(), "while actions are disabled") || enabled {
					t.Fatalf("failed transition: error = %v, enabled = %t", err, enabled)
				}
				if !slices.Equal(writes, []string{"", "/selected-actions"}) {
					t.Errorf("writes after failure = %v", writes)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.mode == alter.DryRun {
				if len(writes) != 0 {
					t.Errorf("preview writes = %v", writes)
				}
			} else if !enabled || !slices.Equal(writes, []string{"", "/selected-actions", "", "/artifact-and-log-retention", "/fork-pr-contributor-approval"}) {
				t.Errorf("writes = %v, enabled = %t", writes, enabled)
			}
		})
	}
}

func assertForkApprovalPolicy(t *testing.T, cfg *config.Config, want string) {
	t.Helper()
	if cfg.Actions == nil || cfg.Actions.ForkPRContributorApproval == nil || cfg.Actions.ForkPRContributorApproval.ApprovalPolicy == nil {
		t.Fatal("missing contributor approval policy")
	}
	if got := *cfg.Actions.ForkPRContributorApproval.ApprovalPolicy; got != want {
		t.Errorf("approval_policy = %q, want %q", got, want)
	}
}

func forkApprovalFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	files := make(map[string]string)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[path] = "directory"
			return nil
		}
		data, err := root.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
