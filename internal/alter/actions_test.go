package alter_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
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
	results, err := alter.ProcessActions(&config.Config{}, alter.Apply, nil, "acme", "widget", true)
	if err != nil || results != nil {
		t.Fatalf("ProcessActions() = %v, %v", results, err)
	}
}

func TestProcessActionsCanonicalNoChange(t *testing.T) {
	var writes atomic.Int32
	server := actionsServer(t, &writes, false)
	t.Cleanup(server.Close)
	patterns := []string{"a/*", "z/*"}
	cfg := &config.Config{Actions: &model.ActionsSettings{
		Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(true),
		GitHubOwnedAllowed: ptr.Ptr(true), VerifiedAllowed: ptr.Ptr(false), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: ptr.Ptr(false)}}
	client := testutil.NewTestClient(t, server)
	results, err := alter.ProcessActions(cfg, alter.DryRun, client, "acme", "widget", true)
	if err != nil || len(results) != 1 || results[0].Category != alter.WouldSet || writes.Load() != 0 {
		t.Fatalf("dry run = %+v, %v, writes %d", results, err, writes.Load())
	}
	if _, err := alter.ProcessActions(cfg, alter.Apply, client, "acme", "widget", true); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", writes.Load())
	}
}

func TestProcessActionsTransitionsToSelectedInOneApply(t *testing.T) {
	var calls []string
	var coreWrites int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions":
			t.Error("selected-actions was read before the selected policy was active")
			w.WriteHeader(http.StatusConflict)
		case r.Method == http.MethodPut && r.URL.Path == "/repos/acme/widget/actions/permissions":
			coreWrites++
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode core body: %v", err)
			}
			if coreWrites == 1 && (body["enabled"] != false || body["allowed_actions"] != "selected") {
				t.Errorf("transition body = %v, want disabled selected policy", body)
			}
			if coreWrites == 2 && body["allowed_actions"] != "selected" {
				t.Errorf("final allowed_actions = %v, want selected", body["allowed_actions"])
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
		AllowedActions:  ptr.Ptr("selected"),
		PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
		"PUT /repos/acme/widget/actions/permissions",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestProcessActionsAccessErrorProducesSkip(t *testing.T) {
	var writes atomic.Int32
	server := actionsServer(t, &writes, true)
	t.Cleanup(server.Close)
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: ptr.Ptr(true)}}
	results, err := alter.ProcessActions(cfg, alter.DryRun, testutil.NewTestClient(t, server), "acme", "widget", true)
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
		AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
		Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(false),
		GitHubOwnedAllowed: ptr.Ptr(true), VerifiedAllowed: ptr.Ptr(true), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
		Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(false),
		GitHubOwnedAllowed: ptr.Ptr(true), VerifiedAllowed: ptr.Ptr(true), PatternsAllowed: &patterns,
	}}
	if _, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true); err != nil {
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
		Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(false),
		GitHubOwnedAllowed: ptr.Ptr(true), VerifiedAllowed: ptr.Ptr(true), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
	output := alter.FormatOutput(results, nil, nil, alter.Apply)
	if strings.Contains(output, "set:") {
		t.Fatalf("output contains a false set result: %q", output)
	}
	if corePuts.Load() != 0 || selectedPuts.Load() != 1 {
		t.Fatalf("PUTs = core %d, selected %d", corePuts.Load(), selectedPuts.Load())
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
	cfg := &config.Config{Actions: &model.ActionsSettings{Enabled: ptr.Ptr(false)}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
	if err != nil {
		t.Fatal(err)
	}
	output := alter.FormatOutput(results, nil, nil, alter.Apply)
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
				Enabled: ptr.Ptr(false), AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns,
			}}
			results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
			if status == http.StatusForbidden && err != nil {
				t.Fatalf("ProcessActions() error = %v, want access skip", err)
			}
			if status == http.StatusForbidden {
				output := alter.FormatOutput(results, nil, nil, alter.Apply)
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
		Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns,
	}}
	if _, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true); err != nil {
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
		AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns,
	}}
	results, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
	if err != nil {
		t.Fatal(err)
	}
	if puts.Load() != 1 {
		t.Fatalf("PUT calls = %d, want only the initial disable", puts.Load())
	}
	output := alter.FormatOutput(results, nil, nil, alter.Apply)
	if strings.Contains(output, "set:") || !strings.Contains(output, "set selected actions permissions") || !strings.Contains(output, "set actions permissions") {
		t.Fatalf("output = %q, want all transition operations skipped", output)
	}
}

func TestProcessActionsSelectedTransitionFailsClosed(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusForbidden, http.StatusNotFound} {
		for _, failStep := range []string{"selected", "final"} {
			t.Run(fmt.Sprintf("%d/%s", status, failStep), func(t *testing.T) {
				enabled := true
				coreWrites := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodGet:
						fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
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
				cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns}}
				_, err := alter.ProcessActions(cfg, alter.Apply, testutil.NewTestClient(t, server), "acme", "widget", true)
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
	cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns}}
	if _, err := alter.ProcessActions(cfg, alter.DryRun, testutil.NewTestClient(t, server), "acme", "widget", true); err != nil {
		t.Fatal(err)
	}
	if writes.Load() != 0 {
		t.Fatalf("baste writes = %d, want 0", writes.Load())
	}
}

func TestRunRejectsInvalidActionsBeforeWrites(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Actions: &model.ActionsSettings{
		AllowedActions:  ptr.Ptr("all"),
		VerifiedAllowed: ptr.Ptr(true),
	}}
	err := alter.Run(cfg, dir, alter.Apply, nil)
	if err == nil {
		t.Fatal("Run() error = nil, want invalid selected-action combination")
	}
}

func TestRunRejectsIncompleteSelectedActions(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Actions: &model.ActionsSettings{AllowedActions: ptr.Ptr("selected")}}
	err := alter.Run(cfg, dir, alter.Apply, nil)
	if err == nil || !strings.Contains(err.Error(), "requires github_owned_allowed, verified_allowed, and patterns_allowed") {
		t.Fatalf("Run() error = %v, want incomplete selected policy error", err)
	}
}
