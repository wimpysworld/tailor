package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
)

const forkApprovalTestPath = "/repos/acme/widget/actions/permissions/fork-pr-contributor-approval"

func TestReadForkPRContributorApproval(t *testing.T) {
	type testCase struct {
		name           string
		status         int
		body           string
		policy         string
		skip           bool
		unavailable    bool
		private        bool
		metadata       string
		metadataStatus int
		hardErr        bool
	}
	tests := []testCase{
		{name: "missing", status: 200, body: `{}`, skip: true},
		{name: "null", status: 200, body: `{"approval_policy":null}`, skip: true},
		{name: "null response", status: 200, body: `null`, skip: true},
		{name: "empty", status: 200, body: `{"approval_policy":""}`, skip: true},
		{name: "unknown", status: 200, body: `{"approval_policy":"future_policy"}`, skip: true},
		{name: "forbidden", status: 403, body: `{"message":"Forbidden"}`, skip: true},
		{name: "unavailable", status: 404, body: `{"message":"Not Found"}`, skip: true},
		{name: "private repository", private: true, unavailable: true},
		{name: "visibility forbidden", metadataStatus: 403, skip: true},
		{name: "visibility unavailable", metadataStatus: 404, skip: true},
		{name: "visibility failure", metadataStatus: 500, hardErr: true},
		{name: "unknown visibility validation failure", metadata: `{}`, status: 422, body: `{"message":"Validation Failed","errors":"Fork PR approval is not allowed for private repositories."}`, hardErr: true},
		{name: "null visibility validation failure", metadata: `{"private":null}`, status: 422, body: `{"message":"Validation Failed"}`, hardErr: true},
		{name: "public repository live validation response", status: 422, body: `{"message":"Validation Failed","errors":"Fork PR approval is not allowed for private repositories.","documentation_url":"https://docs.github.com/rest/actions/permissions#get-fork-pr-contributor-approval-permissions-for-a-repository","status":"422"}`, hardErr: true},
		{name: "other validation error", status: 422, body: `{"message":"Validation Failed"}`, hardErr: true},
		{name: "private message with wrong status", status: 500, body: `{"message":"Fork PR approval is not allowed for private repositories."}`, hardErr: true},
		{name: "unauthorised", status: 401, body: `{"message":"Bad credentials"}`, hardErr: true},
		{name: "server failure", status: 500, body: `{"message":"Server error"}`, hardErr: true},
		{name: "rate limit", status: 403, body: `{"message":"API rate limit exceeded"}`, hardErr: true},
		{name: "malformed", status: 200, body: `{`, hardErr: true},
		{name: "wrong type", status: 200, body: `{"approval_policy":true}`, hardErr: true},
	}
	for _, policy := range model.ForkPRContributorApprovalPolicies {
		tests = append(tests, testCase{name: policy, status: 200, body: fmt.Sprintf(`{"approval_policy":%q}`, policy), policy: policy})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget" {
					if tt.metadataStatus != 0 {
						w.WriteHeader(tt.metadataStatus)
					}
					if tt.metadata != "" {
						fmt.Fprint(w, tt.metadata)
						return
					}
					fmt.Fprintf(w, `{"private":%t}`, tt.private)
					return
				}
				calls++
				if r.Method != http.MethodGet || r.URL.Path != forkApprovalTestPath {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, forkApprovalTestPath)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, server)
			current, warnings, err := ReadForkPRContributorApproval(client, "acme", "widget")
			if (err != nil) != tt.hardErr {
				t.Fatalf("read error = %v, want hard error %t", err, tt.hardErr)
			}
			switch {
			case tt.unavailable:
				var skipped *ErrSetupSkipped
				if current != nil || len(warnings) != 1 || !errors.As(warnings[0], &skipped) || skipped.Reason != SetupNotAvailable || skipped.Operation.Kind != OpFetchForkPRContributorApproval {
					t.Fatalf("read = %+v, warnings = %v, want unavailable approval read", current, warnings)
				}
			case tt.skip:
				if current != nil || len(warnings) != 1 {
					t.Fatalf("read = %+v, warnings = %v, want unknown with one warning", current, warnings)
				}
				var scope *ErrInsufficientScope
				if !errors.As(warnings[0], &scope) || scope.Operation.Kind != OpFetchForkPRContributorApproval {
					t.Fatalf("warning = %v, want approval read skip", warnings[0])
				}
				result, err := ApplyForkPRContributorApproval(client, "acme", "widget", "first_time_contributors", current)
				if err != nil || len(result.Skipped) != 1 || result.Skipped[0].Operation.Kind != OpSetForkPRContributorApproval {
					t.Fatalf("apply = %+v, %v, want approval write skip", result, err)
				}
			case !tt.hardErr:
				if current == nil || current.ApprovalPolicy == nil || *current.ApprovalPolicy != tt.policy || len(warnings) != 0 {
					t.Fatalf("read = %+v, warnings = %v, want %q", current, warnings, tt.policy)
				}
			}
			if (tt.private || tt.metadataStatus != 0) && calls != 0 {
				t.Fatalf("requests = %d, want no approval requests", calls)
			}
			if !tt.private && tt.metadataStatus == 0 && calls != 1 {
				t.Fatalf("requests = %d, want one GET and no PUT", calls)
			}
		})
	}
}

func TestApplyForkPRContributorApproval(t *testing.T) {
	for _, policy := range model.ForkPRContributorApprovalPolicies {
		for _, status := range []int{204, 403, 404, 409, 422, 500} {
			t.Run(fmt.Sprintf("%s/%d", policy, status), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != http.MethodPut || r.URL.Path != forkApprovalTestPath {
						t.Errorf("request = %s %s, want PUT %s", r.Method, r.URL.Path, forkApprovalTestPath)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode request: %v", err)
					}
					if !reflect.DeepEqual(body, map[string]any{"approval_policy": policy}) {
						t.Errorf("body = %v, want only approval_policy=%s", body, policy)
					}
					w.WriteHeader(status)
					if status != 204 {
						fmt.Fprint(w, `{"message":"request rejected"}`)
					}
				}))
				t.Cleanup(server.Close)
				currentPolicy := "first_time_contributors"
				if policy == currentPolicy {
					currentPolicy = "all_external_contributors"
				}
				result, err := ApplyForkPRContributorApproval(newTestClient(t, server), "acme", "widget", policy, &ForkPRContributorApprovalResponse{ApprovalPolicy: &currentPolicy})
				if (err != nil) != (status >= 409) {
					t.Fatalf("apply error = %v for status %d", err, status)
				}
				if status == 403 || status == 404 {
					if len(result.Skipped) != 1 || result.Skipped[0].Operation.Kind != OpSetForkPRContributorApproval {
						t.Fatalf("apply = %+v, want approval write skip", result)
					}
				} else if status == 204 && len(result.Skipped) != 0 {
					t.Fatalf("apply = %+v, want no skips", result)
				}
				if calls != 1 {
					t.Fatalf("requests = %d, want one PUT", calls)
				}
			})
		}
	}
}

func TestApplyForkPRContributorApprovalNoWrite(t *testing.T) {
	tests := []struct {
		name    string
		policy  string
		current *ForkPRContributorApprovalResponse
		skip    bool
		hardErr bool
	}{
		{name: "equal", policy: "first_time_contributors", current: &ForkPRContributorApprovalResponse{ApprovalPolicy: new("first_time_contributors")}},
		{name: "nil response", policy: "first_time_contributors", skip: true},
		{name: "nil policy", policy: "first_time_contributors", current: &ForkPRContributorApprovalResponse{}, skip: true},
		{name: "empty live policy", policy: "first_time_contributors", current: &ForkPRContributorApprovalResponse{ApprovalPolicy: new("")}, skip: true},
		{name: "unknown live policy", policy: "first_time_contributors", current: &ForkPRContributorApprovalResponse{ApprovalPolicy: new("unknown")}, skip: true},
		{name: "empty desired policy", hardErr: true},
		{name: "invalid desired policy", policy: "unknown", hardErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)
			result, err := ApplyForkPRContributorApproval(newTestClient(t, server), "acme", "widget", tt.policy, tt.current)
			if (err != nil) != tt.hardErr {
				t.Fatalf("apply error = %v, want hard error %t", err, tt.hardErr)
			}
			if !tt.hardErr && (len(result.Skipped) != 0) != tt.skip {
				t.Fatalf("apply = %+v, want skip %t", result, tt.skip)
			}
		})
	}
}
