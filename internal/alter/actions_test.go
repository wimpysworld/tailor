package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func actionsServer(t *testing.T, writes *atomic.Int32, forbidden bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if forbidden {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":false,"patterns_allowed":["z/*","a/*"]}`)
		case r.Method == http.MethodPut:
			writes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestProcessActionsAbsentMakesNoCalls(t *testing.T) {
	results, err := alter.ProcessActions(&config.Config{}, alter.Apply, repoTarget(nil, "acme", "widget", true))
	if err != nil || results != nil {
		t.Fatalf("ProcessActions() = %v, %v", results, err)
	}
}

func TestProcessActionsForkApproval(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         alter.ApplyMode
		body         string
		readStatus   int
		writeStatus  int
		wantWrites   int32
		wantCategory alter.RepoSettingCategory
		wantError    bool
	}{
		{name: "preview", mode: alter.DryRun, body: `{"approval_policy":"all_external_contributors"}`, wantCategory: alter.WouldSet},
		{name: "apply", mode: alter.Apply, body: `{"approval_policy":"all_external_contributors"}`, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "recut", mode: alter.Recut, body: `{"approval_policy":"all_external_contributors"}`, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "match", mode: alter.Apply, body: `{"approval_policy":"first_time_contributors"}`, wantCategory: alter.RepoNoChange},
		{name: "missing policy", mode: alter.Apply, body: `{}`, wantCategory: alter.WouldSkipScope},
		{name: "null policy", mode: alter.Apply, body: `{"approval_policy":null}`, wantCategory: alter.WouldSkipScope},
		{name: "empty policy", mode: alter.Apply, body: `{"approval_policy":""}`, wantCategory: alter.WouldSkipScope},
		{name: "unknown policy", mode: alter.Apply, body: `{"approval_policy":"future_policy"}`, wantCategory: alter.WouldSkipScope},
		{name: "forbidden read", mode: alter.Apply, readStatus: 403, wantCategory: alter.WouldSkipScope},
		{name: "unavailable read", mode: alter.Apply, readStatus: 404, wantCategory: alter.WouldSkipScope},
		{name: "failed read", mode: alter.Apply, readStatus: 500, wantError: true},
		{name: "malformed read", mode: alter.Apply, body: `{`, wantError: true},
		{name: "forbidden write", mode: alter.Apply, body: `{"approval_policy":"all_external_contributors"}`, writeStatus: 403, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "unavailable write", mode: alter.Apply, body: `{"approval_policy":"all_external_contributors"}`, writeStatus: 404, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "rejected write", mode: alter.Apply, body: `{"approval_policy":"all_external_contributors"}`, writeStatus: 422, wantWrites: 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads, writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget" {
					fmt.Fprint(w, `{"private":false}`)
					return
				}
				if r.URL.Path != "/repos/acme/widget/actions/permissions/fork-pr-contributor-approval" {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				switch r.Method {
				case http.MethodGet:
					reads.Add(1)
					if tc.readStatus != 0 {
						w.WriteHeader(tc.readStatus)
						fmt.Fprint(w, `{"message":"unavailable"}`)
						return
					}
					fmt.Fprint(w, tc.body)
				case http.MethodPut:
					writes.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || strings.TrimSpace(string(body)) != `{"approval_policy":"first_time_contributors"}` {
						t.Errorf("body = %s, error = %v", body, err)
					}
					if tc.writeStatus != 0 {
						w.WriteHeader(tc.writeStatus)
						fmt.Fprint(w, `{"message":"rejected"}`)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Actions: &model.ActionsSettings{ForkPRContributorApproval: &model.ForkPRContributorApprovalSettings{ApprovalPolicy: new("first_time_contributors")}}}
			results, err := alter.ProcessActions(cfg, tc.mode, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
			if reads.Load() != 1 || writes.Load() != tc.wantWrites {
				t.Fatalf("reads = %d, writes = %d, want 1, %d", reads.Load(), writes.Load(), tc.wantWrites)
			}
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || len(results) == 0 || results[0].Category != tc.wantCategory || results[0].Field != "fork_pr_contributor_approval.approval_policy" {
				t.Fatalf("results = %+v, error = %v", results, err)
			}
			output := alter.FormatOutput(results, nil, nil, nil, tc.mode)
			if tc.writeStatus != 0 {
				if strings.Contains(output, " = first_time_contributors") || !strings.Contains(output, "would skip (insufficient scope") {
					t.Fatalf("output = %q", output)
				}
			} else if tc.wantCategory == alter.WouldSet {
				label := "set:"
				if tc.mode == alter.DryRun {
					label = "would set:"
				}
				want := fmt.Sprintf("%-37sactions.fork_pr_contributor_approval.approval_policy = first_time_contributors\n", label)
				if output != want {
					t.Fatalf("output = %q, want %q", output, want)
				}
			}
		})
	}
}

func TestProcessActionsForkApprovalOmittedMakesNoCalls(t *testing.T) {
	for _, approval := range []*model.ForkPRContributorApprovalSettings{nil, {}} {
		cfg := &config.Config{Actions: &model.ActionsSettings{ForkPRContributorApproval: approval}}
		results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(nil, "acme", "widget", true))
		if err != nil || len(results) != 0 {
			t.Fatalf("results = %+v, error = %v", results, err)
		}
	}
}

func TestProcessActionsForkApprovalReadIsolation(t *testing.T) {
	const corePath = "/repos/acme/widget/actions/permissions"
	const retentionPath = corePath + "/artifact-and-log-retention"
	const approvalPath = corePath + "/fork-pr-contributor-approval"
	for _, tc := range []struct {
		name      string
		denied    string
		wantPuts  []string
		skipField string
	}{
		{"core denied", corePath, []string{retentionPath, approvalPath}, "enabled"},
		{"retention denied", retentionPath, []string{corePath, approvalPath}, "artifact_and_log_retention.days"},
		{"approval denied", approvalPath, []string{corePath, retentionPath}, "fork_pr_contributor_approval.approval_policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var puts []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == tc.denied {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `{"message":"unavailable"}`)
					return
				}
				if r.Method == http.MethodPut {
					puts = append(puts, r.URL.Path)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				switch r.URL.Path {
				case "/repos/acme/widget":
					fmt.Fprint(w, `{"private":false}`)
				case corePath:
					fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
				case retentionPath:
					fmt.Fprint(w, `{"days":30,"maximum_allowed_days":90}`)
				case approvalPath:
					fmt.Fprint(w, `{"approval_policy":"all_external_contributors"}`)
				default:
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Actions: &model.ActionsSettings{
				Enabled:                   new(false),
				ArtifactAndLogRetention:   &model.ArtifactAndLogRetentionSettings{Days: new(14)},
				ForkPRContributorApproval: &model.ForkPRContributorApprovalSettings{ApprovalPolicy: new("first_time_contributors")},
			}}
			results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
			if err != nil || !slices.Equal(puts, tc.wantPuts) {
				t.Fatalf("error = %v, PUTs = %v, want %v", err, puts, tc.wantPuts)
			}
			if len(results) != 3 {
				t.Fatalf("results = %+v, want three fields", results)
			}
			for _, result := range results {
				want := alter.WouldSet
				if result.Field == tc.skipField {
					want = alter.WouldSkipScope
				}
				if result.Category != want {
					t.Errorf("result = %+v, want %s", result, want)
				}
			}
		})
	}
}

func TestProcessActionsRetention(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mode         alter.ApplyMode
		readStatus   int
		writeStatus  int
		body         string
		wantWrites   int32
		wantCategory alter.RepoSettingCategory
		wantError    string
	}{
		{name: "preview", mode: alter.DryRun, body: `{"days":30,"maximum_allowed_days":90}`, wantCategory: alter.WouldSet},
		{name: "apply", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":90}`, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "recut", mode: alter.Recut, body: `{"days":30,"maximum_allowed_days":90}`, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "match", mode: alter.Apply, body: `{"days":14,"maximum_allowed_days":90}`, wantCategory: alter.RepoNoChange},
		{name: "forbidden read", mode: alter.Apply, readStatus: 403, wantCategory: alter.WouldSkipScope},
		{name: "unavailable read", mode: alter.Apply, readStatus: 404, wantCategory: alter.WouldSkipScope},
		{name: "failed read", mode: alter.Apply, readStatus: 500, wantError: "fetching actions"},
		{name: "cap apply", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":7}`, wantError: "maximum of 7 days"},
		{name: "cap preview", mode: alter.DryRun, body: `{"days":30,"maximum_allowed_days":7}`, wantError: "maximum of 7 days"},
		{name: "missing cap", mode: alter.Apply, body: `{"days":30}`, wantError: "maximum_allowed_days"},
		{name: "zero cap", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":0}`, wantError: "maximum_allowed_days"},
		{name: "unknown days", mode: alter.Apply, body: `{"maximum_allowed_days":90}`, wantError: "days is unknown"},
		{name: "forbidden write", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":90}`, writeStatus: 403, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "unavailable write", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":90}`, writeStatus: 404, wantWrites: 1, wantCategory: alter.WouldSet},
		{name: "rejected write", mode: alter.Apply, body: `{"days":30,"maximum_allowed_days":90}`, writeStatus: 422, wantWrites: 1, wantError: "set actions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads, writes atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/repos/acme/widget/actions/permissions/artifact-and-log-retention" {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				switch r.Method {
				case http.MethodGet:
					reads.Add(1)
					if tc.readStatus != 0 {
						w.WriteHeader(tc.readStatus)
						fmt.Fprint(w, `{"message":"unavailable"}`)
						return
					}
					fmt.Fprint(w, tc.body)
				case http.MethodPut:
					writes.Add(1)
					body, err := io.ReadAll(r.Body)
					if err != nil || strings.TrimSpace(string(body)) != `{"days":14}` {
						t.Errorf("body = %s, error = %v", body, err)
					}
					if tc.writeStatus != 0 {
						w.WriteHeader(tc.writeStatus)
						fmt.Fprint(w, `{"message":"rejected"}`)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Actions: &model.ActionsSettings{ArtifactAndLogRetention: &model.ArtifactAndLogRetentionSettings{Days: new(14)}}}
			results, err := alter.ProcessActions(cfg, tc.mode, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
			if reads.Load() != 1 || writes.Load() != tc.wantWrites {
				t.Fatalf("reads = %d, writes = %d, want 1, %d", reads.Load(), writes.Load(), tc.wantWrites)
			}
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || len(results) == 0 || results[0].Category != tc.wantCategory || results[0].Field != "artifact_and_log_retention.days" {
				t.Fatalf("results = %+v, error = %v", results, err)
			}
			output := alter.FormatOutput(results, nil, nil, nil, tc.mode)
			if tc.writeStatus != 0 {
				if strings.Contains(output, " = 14") || !strings.Contains(output, "would skip (insufficient scope") || !strings.Contains(output, "set actions artifact and log retention") {
					t.Fatalf("output = %q", output)
				}
			} else if tc.readStatus == 0 && !strings.Contains(output, "actions.artifact_and_log_retention.days") {
				t.Fatalf("output = %q", output)
			}
		})
	}
}

func TestProcessActionsRetentionOmittedMakesNoCalls(t *testing.T) {
	for _, retention := range []*model.ArtifactAndLogRetentionSettings{nil, {}} {
		cfg := &config.Config{Actions: &model.ActionsSettings{ArtifactAndLogRetention: retention}}
		results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(nil, "acme", "widget", true))
		if err != nil || len(results) != 0 {
			t.Fatalf("results = %+v, error = %v", results, err)
		}
	}
}

func TestProcessActionsRetentionIndependentOfDeniedCore(t *testing.T) {
	var retentionWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"unavailable"}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/artifact-and-log-retention"):
			fmt.Fprint(w, `{"days":30,"maximum_allowed_days":90}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/artifact-and-log-retention"):
			retentionWrites.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: new(true), ArtifactAndLogRetention: &model.ArtifactAndLogRetentionSettings{Days: new(14)}}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil || retentionWrites.Load() != 1 {
		t.Fatalf("results = %+v, error = %v, writes = %d", results, err, retentionWrites.Load())
	}
	output := alter.FormatOutput(results, nil, nil, nil, alter.Apply)
	if !strings.Contains(output, "actions.artifact_and_log_retention.days = 14") || !strings.Contains(output, "would skip (insufficient scope") {
		t.Fatalf("output = %q", output)
	}
}

func TestProcessActionsCanonicalNoChange(t *testing.T) {
	var writes atomic.Int32
	server := actionsServer(t, &writes, false)
	t.Cleanup(server.Close)
	patterns := []string{"a/*", "z/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(true),
		GitHubOwnedAllowed: new(true), VerifiedAllowed: new(false), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("writes = %d, want 0", writes.Load())
	}
	for _, result := range results {
		if result.Category != alter.RepoNoChange || result.Section != "actions" {
			t.Errorf("result = %+v, want actions no change", result)
		}
	}
}

func TestProcessActionsDryRunAndApply(t *testing.T) {
	var writes atomic.Int32
	server := actionsServer(t, &writes, false)
	t.Cleanup(server.Close)
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: new(false)}}
	client := testutil.NewTestClient(t, server)
	results, err := alter.ProcessActions(cfg, alter.DryRun, repoTarget(client, "acme", "widget", true))
	if err != nil || len(results) != 1 || results[0].Category != alter.WouldSet || writes.Load() != 0 {
		t.Fatalf("dry run = %+v, %v, writes %d", results, err, writes.Load())
	}
	if _, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(client, "acme", "widget", true)); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", writes.Load())
	}
}

func TestProcessActionsTransitionsToSelectedInOneApply(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
			t.Error("selected-actions was read before the selected policy was active")
			w.WriteHeader(http.StatusConflict)
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode core body: %v", err)
			}
			if len(body) != 2 || body["enabled"] != true || body["allowed_actions"] != "selected" {
				t.Errorf("core body = %v, want enabled selected policy", body)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		AllowedActions:  new("selected"),
		PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Category != alter.WouldSet || results[1].Category != alter.WouldSet {
		t.Fatalf("results = %+v, want two changes", results)
	}
	wantCalls := []string{
		"GET /repos/acme/widget/actions/permissions",
		"PUT /repos/acme/widget/actions/permissions",
		"PUT /repos/acme/widget/actions/permissions/selected-actions",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestProcessActionsAccessErrorProducesSkip(t *testing.T) {
	var writes atomic.Int32
	server := actionsServer(t, &writes, true)
	t.Cleanup(server.Close)
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: new(true)}}
	results, err := alter.ProcessActions(cfg, alter.DryRun, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Category != alter.WouldSkipScope || results[0].Field != "enabled" {
		t.Fatalf("results = %+v, want enabled access skip", results)
	}
}

func TestProcessActionsUnknownCoreSkipsAllDeclaredPolicyFields(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)
	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want only the core read", calls.Load())
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want two skips", results)
	}
	for _, result := range results {
		if result.Category != alter.WouldSkipScope {
			t.Fatalf("result = %+v, want access skip", result)
		}
	}
}

func TestProcessActionsUnknownSelectedPolicyBlocksEnable(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"enabled":false,"allowed_actions":"selected","sha_pinning_required":true}`)
		default:
			writes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(false),
		GitHubOwnedAllowed: new(true), VerifiedAllowed: new(true), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("writes = %d, want none while selected policy is unknown", writes.Load())
	}
	skipped := map[string]bool{}
	for _, result := range results {
		if result.Category == alter.WouldSet {
			t.Fatalf("results = %+v, want no set result", results)
		}
		if result.Category == alter.WouldSkipScope {
			skipped[result.Field] = true
		}
	}
	for _, field := range []string{"enabled", "sha_pinning_required"} {
		if !skipped[field] {
			t.Errorf("results do not report a skip for %s: %+v", field, results)
		}
	}
}

func TestProcessActionsDisablesBeforeChangingSelectedPolicyAndRelaxingSHAPinning(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":true,"patterns_allowed":["*"]}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true}`)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(false),
		GitHubOwnedAllowed: new(true), VerifiedAllowed: new(true), PatternsAllowed: &patterns,
	}}
	if _, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /repos/acme/widget/actions/permissions",
		"GET /repos/acme/widget/actions/permissions/selected-actions",
		"PUT /repos/acme/widget/actions/permissions",
		"PUT /repos/acme/widget/actions/permissions/selected-actions",
		"PUT /repos/acme/widget/actions/permissions",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestProcessActionsSelectedOnlyWriteAccessError(t *testing.T) {
	var corePuts atomic.Int32
	var selectedPuts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":true,"patterns_allowed":["acme/*"]}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":false}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			selectedPuts.Add(1)
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
		case r.Method == http.MethodPut:
			corePuts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	patterns := []string{}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(false),
		GitHubOwnedAllowed: new(true), VerifiedAllowed: new(true), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	scopeSkips := 0
	for _, result := range results {
		if result.Category == alter.WouldSkipScope {
			scopeSkips++
		}
	}
	if scopeSkips != 1 {
		t.Fatalf("scope skips = %d, want 1: %+v", scopeSkips, results)
	}
	output := alter.FormatOutput(results, nil, nil, nil, alter.Apply)
	if strings.Contains(output, "set:") {
		t.Fatalf("output contains a false set result: %q", output)
	}
	if corePuts.Load() != 0 || selectedPuts.Load() != 1 {
		t.Fatalf("PUTs = core %d, selected %d", corePuts.Load(), selectedPuts.Load())
	}
}

func TestProcessActionsPreDisabledSelectedWriteAccessErrorSkipsCore(t *testing.T) {
	var corePuts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":true,"patterns_allowed":[]}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"enabled":false,"allowed_actions":"selected","sha_pinning_required":false}`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
		case r.Method == http.MethodPut:
			corePuts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(false),
		GitHubOwnedAllowed: new(true), VerifiedAllowed: new(true), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatalf("ProcessActions() error = %v, want access skip", err)
	}
	selectedScopeSkip := false
	for _, result := range results {
		if result.Category == alter.WouldSkipScope && result.Operation.Kind == gh.OpSetSelectedActionsPermissions {
			selectedScopeSkip = true
		}
	}
	if !selectedScopeSkip {
		t.Fatalf("ProcessActions() results = %+v, want selected-policy scope skip", results)
	}
	if corePuts.Load() != 0 {
		t.Fatalf("core PUTs = %d, want 0", corePuts.Load())
	}
}

func TestProcessActionsWriteAccessErrorProducesClearOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: new(false)}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	output := alter.FormatOutput(results, nil, nil, nil, alter.Apply)
	if output == "" || !strings.Contains(output, "would skip (insufficient scope") || !strings.Contains(output, "set actions permissions") {
		t.Fatalf("output = %q, want clear Actions access skip", output)
	}
}

func TestProcessActionsStopsSelectedWriteAfterCoreFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(fmt.Sprintf("status %d", status), func(t *testing.T) {
			var selectedWrites atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions":
					fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":false}`)
				case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
					fmt.Fprint(w, `{"github_owned_allowed":false,"verified_allowed":false,"patterns_allowed":[]}`)
				case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions":
					w.WriteHeader(status)
					fmt.Fprint(w, `{"message":"failed"}`)
				case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
					selectedWrites.Add(1)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			patterns := []string{"acme/*"}
			cfg := &config.Config{Actions: &model.ActionsSettings{
				Enabled: new(false), AllowedActions: new("selected"), PatternsAllowed: &patterns,
			}}
			results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
			if status == http.StatusForbidden && err != nil {
				t.Fatalf("ProcessActions() error = %v, want access skip", err)
			}
			if status == http.StatusForbidden {
				output := alter.FormatOutput(results, nil, nil, nil, alter.Apply)
				if strings.Contains(output, "set:") || !strings.Contains(output, "set selected actions permissions") {
					t.Fatalf("output = %q, want skipped core and dependent writes", output)
				}
			}
			if status == http.StatusInternalServerError && err == nil {
				t.Fatal("ProcessActions() error = nil, want core failure")
			}
			if selectedWrites.Load() != 0 {
				t.Fatalf("selected writes = %d, want 0", selectedWrites.Load())
			}
		})
	}
}

func TestProcessActionsTightensSelectedPolicyBeforeEnabling(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/selected-actions"):
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":true,"patterns_allowed":["*"]}`)
		case r.Method == http.MethodGet:
			fmt.Fprint(w, `{"enabled":false,"allowed_actions":"selected","sha_pinning_required":false}`)
		case r.Method == http.MethodPut:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}}
	if _, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true)); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /repos/acme/widget/actions/permissions",
		"GET /repos/acme/widget/actions/permissions/selected-actions",
		"PUT /repos/acme/widget/actions/permissions/selected-actions",
		"PUT /repos/acme/widget/actions/permissions",
	}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestProcessActionsInitialTransitionSkipSuppressesUnattemptedWrites(t *testing.T) {
	var puts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
			return
		}
		puts.Add(1)
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)
	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	if puts.Load() != 1 {
		t.Fatalf("PUT calls = %d, want only the final core policy", puts.Load())
	}
	output := alter.FormatOutput(results, nil, nil, nil, alter.Apply)
	if strings.Contains(output, "set:") || !strings.Contains(output, "set selected actions permissions") || !strings.Contains(output, "set actions permissions") {
		t.Fatalf("output = %q, want all transition operations skipped", output)
	}
}

func TestProcessActionsLocalOnlyTransitionFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden, http.StatusNotFound} {
		for _, failStep := range []string{"selected", "final"} {
			t.Run(fmt.Sprintf("%d/%s", status, failStep), func(t *testing.T) {
				enabled := true
				coreWrites := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet:
						fmt.Fprint(w, `{"enabled":true,"allowed_actions":"local_only","sha_pinning_required":false}`)
					case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
						if failStep == "selected" {
							w.WriteHeader(status)
							fmt.Fprint(w, `{"message":"failed"}`)
							return
						}
						w.WriteHeader(http.StatusNoContent)
					case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions":
						coreWrites++
						var body map[string]any
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Fatal(err)
						}
						if coreWrites == 2 && failStep == "final" {
							w.WriteHeader(status)
							fmt.Fprint(w, `{"message":"failed"}`)
							return
						}
						enabled = body["enabled"].(bool)
						w.WriteHeader(http.StatusNoContent)
					default:
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(server.Close)
				patterns := []string{"acme/*"}
				cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: new("selected"), PatternsAllowed: &patterns}}
				_, err := alter.ProcessActions(cfg, alter.Apply, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true))
				if err == nil || !strings.Contains(err.Error(), "while actions are disabled") {
					t.Fatalf("ProcessActions() error = %v, want explicit disabled transition failure", err)
				}
				if enabled {
					t.Fatal("Actions enabled after failed selected transition")
				}
			})
		}
	}
}

func TestProcessActionsSelectedTransitionDryRunIsReadOnly(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		fmt.Fprint(w, `{"enabled":true,"allowed_actions":"local_only","sha_pinning_required":false}`)
	}))
	t.Cleanup(server.Close)
	patterns := []string{"acme/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: new("selected"), PatternsAllowed: &patterns}}
	if _, err := alter.ProcessActions(cfg, alter.DryRun, repoTarget(testutil.NewTestClient(t, server), "acme", "widget", true)); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("baste writes = %d, want 0", writes.Load())
	}
}

func TestRunRejectsInvalidActionsBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Actions: &model.ActionsSettings{
		AllowedActions:  new("all"),
		VerifiedAllowed: new(true),
	}}
	err := alter.Run(cfg, dir, alter.Apply, nil, io.Discard, io.Discard)
	if err == nil {
		t.Fatal("Run() error = nil, want invalid selected-action combination")
	}
}

func TestRunRejectsIncompleteSelectedActions(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: new("selected")}}
	err := alter.Run(cfg, dir, alter.Apply, nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "requires github_owned_allowed, verified_allowed, and patterns_allowed") {
		t.Fatalf("Run() error = %v, want incomplete selected policy error", err)
	}
}
