package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

const customPagesEnvironment = `{"name":"github-pages","deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true},"protection_rules":[{"type":"wait_timer","wait_timer":30},{"type":"required_reviewers","reviewers":[{"id":1}]}]}`

func TestReadPagesEnvironmentAccess(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		proof   bool
		missing bool
	}{
		{"proven absence", 404, true, true},
		{"ambiguous absence", 404, false, false},
		{"denied", 403, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := statusServer(t, tt.status, `{"message":"not available"}`)
			state, err := ReadPagesEnvironment(newTestClient(t, server), "owner", "repo", "main", tt.proof)
			if tt.missing {
				if err != nil || state == nil || !state.Missing || !state.AddBranch {
					t.Fatalf("state=%+v error=%v", state, err)
				}
			} else {
				var scope *ErrInsufficientScope
				if state != nil || !errors.As(err, &scope) {
					t.Fatalf("state=%+v error=%v", state, err)
				}
			}
		})
	}
}

func TestReadPagesEnvironmentRateLimit(t *testing.T) {
	server := statusServer(t, http.StatusTooManyRequests, `{"message":"rate limit exceeded"}`)
	state, err := ReadPagesEnvironment(newTestClient(t, server), "owner", "repo", "main", true)
	var limited *ErrRateLimited
	if state != nil || !errors.As(err, &limited) {
		t.Fatalf("state=%+v error=%v", state, err)
	}
}

func TestPagesEnvironmentCustomPolicyWithoutAdminAccess(t *testing.T) {
	for _, branch := range []string{"main", "release"} {
		t.Run(branch, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					t.Errorf("unexpected write: %s", r.Method)
				}
				if strings.HasSuffix(r.URL.Path, "/deployment-branch-policies") {
					fmt.Fprintf(w, `{"total_count":1,"branch_policies":[{"id":1,"name":%q,"type":"branch"}]}`, branch)
					return
				}
				fmt.Fprint(w, customPagesEnvironment)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			state, err := ReadPagesEnvironment(client, "owner", "repo", "main", false)
			if branch != "main" {
				var scope *ErrInsufficientScope
				if state != nil || !errors.As(err, &scope) || scope.Operation.Kind != OpPostPagesEnvironmentPolicy {
					t.Fatalf("state=%+v error=%v", state, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
				if err != nil || len(result.Applied) != 0 {
					t.Fatalf("result=%+v error=%v", result, err)
				}
			}
			if writes != 0 {
				t.Fatalf("writes=%d", writes)
			}
		})
	}
}

func TestApplyPagesEnvironmentRequiresAdminAccess(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			}))
			t.Cleanup(server.Close)
			state := &PagesEnvironmentState{Missing: missing, AddBranch: true, Branch: "main", verified: true}
			result, err := ApplyPagesEnvironment(newTestClient(t, server), "owner", "repo", state)
			var scope *ErrInsufficientScope
			if !errors.As(err, &scope) || len(result.Applied) != 0 || requests != 0 {
				t.Fatalf("result=%+v error=%v requests=%d", result, err, requests)
			}
		})
	}
}

func TestPagesEnvironmentPreservesUnrestricted(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write: %s", r.Method)
		}
		fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null,"protection_rules":[{"type":"wait_timer","wait_timer":30}]}`)
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server)
	state, err := ReadPagesEnvironment(client, "owner", "repo", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
	if err != nil || len(result.Applied) != 0 || requests != 1 {
		t.Fatalf("result=%+v error=%v requests=%d", result, err, requests)
	}
}

func TestReadPagesEnvironmentCompletePolicies(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/deployment-branch-policies") {
			fmt.Fprint(w, customPagesEnvironment)
			return
		}
		pages++
		if pages == 1 {
			w.Header().Set("Link", `<https://api.github.com/example?page=2>; rel="next"`)
			fmt.Fprint(w, `{"total_count":2,"branch_policies":[{"id":1,"name":"release/*","type":"branch"}]}`)
		} else {
			fmt.Fprint(w, `{"total_count":2,"branch_policies":[{"id":2,"name":"main","type":"branch"}]}`)
		}
	}))
	t.Cleanup(server.Close)
	state, err := ReadPagesEnvironment(newTestClient(t, server), "owner", "repo", "main", true)
	if err != nil || state.AddBranch || pages != 2 {
		t.Fatalf("state=%+v error=%v pages=%d", state, err, pages)
	}
}

func TestReadPagesEnvironmentProtectedBranch(t *testing.T) {
	for _, protected := range []bool{true, false} {
		t.Run(fmt.Sprint(protected), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/owner/repo/environments/github-pages":
					fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":{"protected_branches":true,"custom_branch_policies":false}}`)
				case "/repos/owner/repo/branches/release/site":
					fmt.Fprintf(w, `{"protected":%t}`, protected)
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			t.Cleanup(server.Close)
			state, err := ReadPagesEnvironment(newTestClient(t, server), "owner", "repo", "release/site", true)
			if protected && (err != nil || state.AddBranch) {
				t.Fatalf("state=%+v error=%v", state, err)
			}
			if !protected && (err == nil || state != nil) {
				t.Fatalf("state=%+v error=%v", state, err)
			}
		})
	}
}

func TestApplyPagesEnvironmentWrites(t *testing.T) {
	for _, tt := range []struct {
		name    string
		missing bool
		failure bool
	}{
		{"create", true, false}, {"custom preservation", false, false}, {"partial failure", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var writes []recordedRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if strings.HasSuffix(r.URL.Path, "/deployment-branch-policies") {
						fmt.Fprint(w, `{"total_count":1,"branch_policies":[{"id":1,"name":"release/*","type":"branch"}]}`)
						return
					}
					if tt.missing {
						w.WriteHeader(http.StatusNotFound)
						fmt.Fprint(w, `{"message":"not found"}`)
						return
					}
					fmt.Fprint(w, customPagesEnvironment)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				writes = append(writes, recordedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
				if r.Method == http.MethodPost && tt.failure {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, `{"message":"failed"}`)
					return
				}
				w.WriteHeader(http.StatusCreated)
				fmt.Fprint(w, `{}`)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			state, err := ReadPagesEnvironment(client, "owner", "repo", "main", true)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
			if (err != nil) != tt.failure {
				t.Fatalf("error=%v", err)
			}
			wantApplied := 1
			if tt.missing && !tt.failure {
				wantApplied = 2
			}
			if len(result.Applied) != wantApplied {
				t.Fatalf("applied=%+v", result.Applied)
			}
			if tt.missing {
				want := map[string]any{"deployment_branch_policy": map[string]any{"protected_branches": false, "custom_branch_policies": true}}
				if writes[0].Method != http.MethodPut || !reflect.DeepEqual(writes[0].Body, want) {
					t.Fatalf("create=%+v", writes[0])
				}
			} else if len(writes) != 1 {
				t.Fatalf("writes=%+v", writes)
			}
			last := writes[len(writes)-1]
			if last.Method != http.MethodPost || !reflect.DeepEqual(last.Body, map[string]any{"name": "main", "type": "branch"}) {
				t.Fatalf("policy=%+v", last)
			}
		})
	}
}

func TestPagesEnvironmentPolicyRace(t *testing.T) {
	for _, resolved := range []bool{true, false} {
		t.Run(fmt.Sprint(resolved), func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					writes++
					w.WriteHeader(http.StatusUnprocessableEntity)
					fmt.Fprint(w, `{"message":"already exists"}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/deployment-branch-policies") {
					if writes > 0 && resolved {
						fmt.Fprint(w, `{"total_count":1,"branch_policies":[{"id":1,"name":"main","type":"branch"}]}`)
					} else {
						fmt.Fprint(w, `{"total_count":0,"branch_policies":[]}`)
					}
					return
				}
				fmt.Fprint(w, customPagesEnvironment)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			state, err := ReadPagesEnvironment(client, "owner", "repo", "main", true)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
			if (err == nil) != resolved || writes != 1 || len(result.Applied) != 0 {
				t.Fatalf("result=%+v error=%v writes=%d", result, err, writes)
			}
		})
	}
}

func TestReadPagesEnvironmentMalformedPolicies(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"total_count":1,"branch_policies":[]}`,
		`{"total_count":2,"branch_policies":[{"id":1,"name":"main","type":"branch"},{"id":1,"name":"main","type":"branch"}]}`,
		`{"total_count":1,"branch_policies":[{"id":1,"name":"main"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/deployment-branch-policies") {
					fmt.Fprint(w, body)
					return
				}
				fmt.Fprint(w, customPagesEnvironment)
			}))
			t.Cleanup(server.Close)
			state, err := ReadPagesEnvironment(newTestClient(t, server), "owner", "repo", "main", true)
			if err == nil || state != nil {
				t.Fatalf("state=%+v error=%v", state, err)
			}
		})
	}
}

func TestPagesEnvironmentCreationRace(t *testing.T) {
	for _, status := range []int{0, http.StatusConflict, http.StatusUnprocessableEntity} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					writes++
					if r.Method != http.MethodPut || writes > 1 {
						t.Errorf("unexpected write %s", r.Method)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if status == http.StatusUnprocessableEntity {
						fmt.Fprint(w, `{"message":"environment already exists"}`)
					} else {
						fmt.Fprint(w, `{"message":"conflict"}`)
					}
					return
				}
				reads++
				if reads == 1 || (status != 0 && writes == 0) {
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprint(w, `{"message":"not found"}`)
					return
				}
				fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null}`)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			state, err := ReadPagesEnvironment(client, "owner", "repo", "main", true)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
			wantWrites := 1
			if status == 0 {
				wantWrites = 0
			}
			if err != nil || len(result.Applied) != 0 || writes != wantWrites {
				t.Fatalf("result=%+v error=%v writes=%d", result, err, writes)
			}
		})
	}
}

func TestPagesEnvironmentCreationValidationError(t *testing.T) {
	reads, writes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			reads++
			if writes > 0 {
				fmt.Fprint(w, `{"name":"github-pages","deployment_branch_policy":null}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"not found"}`)
			return
		}
		writes++
		if r.Method != http.MethodPut {
			t.Errorf("unexpected write %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message":"invalid deployment branch policy"}`)
	}))
	t.Cleanup(server.Close)
	client := newTestClient(t, server)
	state, err := ReadPagesEnvironment(client, "owner", "repo", "main", true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyPagesEnvironment(client, "owner", "repo", state)
	if err == nil || !strings.Contains(err.Error(), "invalid deployment branch policy") || len(result.Applied) != 0 || reads != 2 || writes != 1 {
		t.Fatalf("result=%+v error=%v reads=%d writes=%d", result, err, reads, writes)
	}
}
