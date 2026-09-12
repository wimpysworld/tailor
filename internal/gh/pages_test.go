package gh

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestReadPagesRepository(t *testing.T) {
	for _, tt := range []struct {
		name, scopes, permissions      string
		private, wantAccess, wantAdmin bool
	}{
		{"admin repo", "repo, user", `{"admin":true}`, false, true, true},
		{"maintainer repo", "repo", `{"maintain":true}`, false, true, false},
		{"public repo insufficient", "public_repo", `{"admin":true}`, false, false, false},
		{"fine grained unknown", "", `{"admin":true}`, false, false, false},
		{"reader repo", "repo", `{"pull":true}`, false, false, false},
		{"private repo", "repo", `{"admin":true}`, true, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/repos/acme/widget" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("X-OAuth-Scopes", tt.scopes)
				w.Header().Set("X-Accepted-OAuth-Scopes", "repo")
				w.Header().Set("X-Accepted-GitHub-Permissions", "pages=write;administration=write")
				_, _ = w.Write([]byte(`{"default_branch":"main","homepage":"https://github.com/acme/widget","private":` + map[bool]string{false: "false", true: "true"}[tt.private] + `,"permissions":` + tt.permissions + `}`))
			}))
			t.Cleanup(server.Close)
			state, err := ReadPagesRepository(testutil.NewTestClient(t, server), "acme", "widget")
			if (err == nil) != (tt.wantAccess && !tt.private) {
				t.Fatalf("err = %v", err)
			}
			if state == nil || state.WriteAccess != tt.wantAccess || state.AdminAccess != tt.wantAdmin {
				t.Fatalf("state = %+v", state)
			}
			if err != nil && !tt.private {
				if _, ok := errors.AsType[*ErrInsufficientScope](err); !ok {
					t.Fatalf("want scope error, got %v", err)
				}
			}
		})
	}
}

func TestReadPagesAbsenceAndErrors(t *testing.T) {
	for _, tt := range []struct {
		name         string
		status       int
		access, rate bool
		want         string
	}{
		{"proved absence", 404, true, false, "absent"},
		{"ambiguous absence", 404, false, false, "scope"},
		{"forbidden", 403, true, false, "scope"},
		{"rate limit", 403, true, true, "rate"},
		{"too many", 429, true, false, "rate"},
		{"server", 500, true, false, "hard"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.rate {
					w.Header().Set("Retry-After", "60")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"failed"}`))
			}))
			t.Cleanup(server.Close)
			state, err := ReadPages(testutil.NewTestClient(t, server), "acme", "widget", tt.access)
			switch tt.want {
			case "absent":
				if err != nil || state.Exists {
					t.Fatalf("state=%+v err=%v", state, err)
				}
			case "scope":
				if _, ok := errors.AsType[*ErrInsufficientScope](err); !ok {
					t.Fatalf("err=%v", err)
				}
			case "rate":
				if _, ok := errors.AsType[*ErrRateLimited](err); !ok {
					t.Fatalf("err=%v", err)
				}
			case "hard":
				if err == nil || isAccessError(err) {
					t.Fatalf("err=%v", err)
				}
			}
		})
	}
}

type pagesRequest struct {
	method, body string
	status       int
	response     string
}

func pagesScriptServer(t *testing.T, requests []pagesRequest) *httptest.Server {
	t.Helper()
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if index >= len(requests) {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		want := requests[index]
		index++
		if r.Method != want.method || r.URL.Path != "/repos/acme/widget/pages" {
			t.Errorf("request=%s %s, want %s pages", r.Method, r.URL.Path, want.method)
		}
		if want.body != "" {
			var got, expected any
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal([]byte(want.body), &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, expected) {
				t.Errorf("body=%v, want %v", got, expected)
			}
		}
		w.WriteHeader(want.status)
		if want.response != "" {
			_, _ = w.Write([]byte(want.response))
		}
	}))
	t.Cleanup(func() {
		server.Close()
		if index != len(requests) {
			t.Errorf("requests=%d, want %d", index, len(requests))
		}
	})
	return server
}

const pagesReadyJSON = `{"build_type":"workflow","html_url":"https://acme.github.io/widget/","https_enforced":true}`

func TestApplyPages(t *testing.T) {
	for _, tt := range []struct {
		name     string
		state    PagesState
		cname    *string
		requests []pagesRequest
		applied  []OperationKind
		pending  bool
		hard     bool
	}{
		{name: "create", requests: []pagesRequest{{"POST", `{"build_type":"workflow"}`, 201, `{}`}, {"GET", "", 200, pagesReadyJSON}}, applied: []OperationKind{OpCreatePages}},
		{name: "migrate", state: PagesState{Exists: true, BuildType: "legacy"}, requests: []pagesRequest{{"PUT", `{"build_type":"workflow"}`, 204, ""}, {"GET", "", 200, pagesReadyJSON}}, applied: []OperationKind{OpSetPagesBuildType}},
		{name: "matching domain omitted", state: PagesState{Exists: true, BuildType: "workflow", CNAME: "site.example.com"}, requests: []pagesRequest{{"GET", "", 200, `{"build_type":"workflow","cname":"site.example.com","https_enforced":true}`}}},
		{name: "clear domain", state: PagesState{Exists: true, BuildType: "workflow", CNAME: "site.example.com"}, cname: new(""), requests: []pagesRequest{{"PUT", `{"cname":null}`, 204, ""}, {"GET", "", 200, pagesReadyJSON}}, applied: []OperationKind{OpSetPagesDomain}},
		{name: "set domain pending", state: PagesState{Exists: true, BuildType: "workflow"}, cname: new("site.example.com"), requests: []pagesRequest{{"PUT", `{"cname":"site.example.com"}`, 204, ""}, {"GET", "", 200, `{"build_type":"workflow","cname":"site.example.com","protected_domain_state":"pending"}`}}, applied: []OperationKind{OpSetPagesDomain}, pending: true},
		{name: "certificate pending", state: PagesState{Exists: true, BuildType: "workflow"}, requests: []pagesRequest{{"GET", "", 200, `{"build_type":"workflow","cname":"site.example.com","https_certificate":{"state":"issued","domains":["site.example.com"]}}`}}, pending: true},
		{name: "certificate wrong domain", state: PagesState{Exists: true, BuildType: "workflow"}, requests: []pagesRequest{{"GET", "", 200, `{"build_type":"workflow","cname":"site.example.com","https_certificate":{"state":"approved","domains":["old.example.com"]}}`}}, pending: true},
		{name: "enforce certificate", state: PagesState{Exists: true, BuildType: "workflow"}, requests: []pagesRequest{{"GET", "", 200, `{"build_type":"workflow","cname":"site.example.com","https_certificate":{"state":"approved","domains":["site.example.com"]}}`}, {"PUT", `{"https_enforced":true}`, 204, ""}}, applied: []OperationKind{OpEnforcePagesHTTPS}},
		{name: "creation race", requests: []pagesRequest{{"POST", `{"build_type":"workflow"}`, 409, `{"message":"already exists"}`}, {"GET", "", 200, pagesReadyJSON}, {"GET", "", 200, pagesReadyJSON}}},
		{name: "creation validation race", requests: []pagesRequest{{"POST", `{"build_type":"workflow"}`, 422, `{"message":"A GitHub Pages site already exists."}`}, {"GET", "", 200, pagesReadyJSON}, {"GET", "", 200, pagesReadyJSON}}},
		{name: "creation validation error", requests: []pagesRequest{{"POST", `{"build_type":"workflow"}`, 422, `{"message":"invalid build_type"}`}}, hard: true},
		{name: "create partial domain failure", cname: new("site.example.com"), requests: []pagesRequest{{"POST", `{"build_type":"workflow"}`, 201, `{}`}, {"PUT", `{"cname":"site.example.com"}`, 422, `{"message":"invalid cname"}`}}, applied: []OperationKind{OpCreatePages}, hard: true},
		{name: "migration denied", state: PagesState{Exists: true, BuildType: "legacy"}, requests: []pagesRequest{{"PUT", `{"build_type":"workflow"}`, 403, `{"message":"forbidden"}`}}, hard: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := pagesScriptServer(t, tt.requests)
			result, _, err := ApplyPages(testutil.NewTestClient(t, server), "acme", "widget", &tt.state, tt.cname)
			var pending *ErrPagesPending
			if errors.As(err, &pending) != tt.pending || (err != nil) != (tt.pending || tt.hard) {
				t.Fatalf("err=%v", err)
			}
			var applied []OperationKind
			for _, operation := range result.Applied {
				applied = append(applied, operation.Kind)
			}
			if !reflect.DeepEqual(applied, tt.applied) {
				t.Errorf("applied=%v, want %v", applied, tt.applied)
			}
		})
	}
}

func TestPagesDomainError(t *testing.T) {
	for _, tt := range []struct {
		message string
		pending bool
	}{
		{"certificate is not yet available", true}, {"domain does not resolve", true}, {"domain is not verified", true}, {"invalid cname", false}, {"validation failed", false},
	} {
		t.Run(tt.message, func(t *testing.T) {
			server := pagesScriptServer(t, []pagesRequest{{"PUT", `{"cname":"site.example.com"}`, 422, `{"message":"` + tt.message + `"}`}})
			err := pagesDomainError(writePages(testutil.NewTestClient(t, server), "acme", "widget", "PUT", map[string]any{"cname": "site.example.com"}, 204, OpSetPagesDomain))
			var pending *ErrPagesPending
			if errors.As(err, &pending) != tt.pending {
				t.Fatalf("err=%v", err)
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
