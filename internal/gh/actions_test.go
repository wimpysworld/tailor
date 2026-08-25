package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/model"
)

func TestReadActionsPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"selected","sha_pinning_required":true}`)
		case "/repos/acme/widget/actions/permissions/selected-actions":
			fmt.Fprint(w, `{"github_owned_allowed":true,"verified_allowed":false,"patterns_allowed":["acme/*"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	got, warnings, err := ReadActionsPolicy(newTestClient(t, server), "acme", "widget", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || got.Enabled == nil || !*got.Enabled || got.AllowedActions == nil || *got.AllowedActions != "selected" {
		t.Fatalf("ReadActionsPolicy() = %+v, %v", got, warnings)
	}
	if got.PatternsAllowed == nil || len(*got.PatternsAllowed) != 1 || (*got.PatternsAllowed)[0] != "acme/*" {
		t.Fatalf("PatternsAllowed = %v", got.PatternsAllowed)
	}
}

func TestReadActionsPolicySkipsSelectedReadWhenCoreIsUnknown(t *testing.T) {
	var selectedReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions" {
			selectedReads.Add(1)
		}
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
	}))
	t.Cleanup(server.Close)

	got, warnings, err := ReadActionsPolicy(newTestClient(t, server), "acme", "widget", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || got.Enabled != nil || selectedReads.Load() != 0 {
		t.Fatalf("ReadActionsPolicy() = %+v, %v, selected reads %d", got, warnings, selectedReads.Load())
	}
}

func TestReadActionsPolicySkipsSelectedReadWhileAllIsActive(t *testing.T) {
	var selectedReads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/actions/permissions":
			fmt.Fprint(w, `{"enabled":true,"allowed_actions":"all","sha_pinning_required":false}`)
		case "/repos/acme/widget/actions/permissions/selected-actions":
			selectedReads.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"Conflict","errors":["Selected actions are unavailable while allowed_actions is all"]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	got, warnings, err := ReadActionsPolicy(newTestClient(t, server), "acme", "widget", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || got.AllowedActions == nil || *got.AllowedActions != "all" {
		t.Fatalf("ReadActionsPolicy() = %+v, %v", got, warnings)
	}
	if selectedReads.Load() != 0 {
		t.Fatalf("selected reads = %d, want 0", selectedReads.Load())
	}
}

func TestApplyActionsPolicyPayloads(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if r.URL.Path == "/repos/acme/widget/actions/permissions" && body["enabled"] != true {
			t.Errorf("enabled = %v, want true", body["enabled"])
		}
		if r.URL.Path == "/repos/acme/widget/actions/permissions/selected-actions" {
			patterns, ok := body["patterns_allowed"].([]any)
			if !ok || patterns[0] != "a/*" || patterns[1] != "z/*" {
				t.Errorf("patterns_allowed = %v, want sorted", body["patterns_allowed"])
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	patterns := []string{"z/*", "a/*"}
	desired := &model.ActionsSettings{AllowedActions: new("selected"), PatternsAllowed: &patterns}
	current := &model.ActionsSettings{Enabled: new(true)}
	if _, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestApplyActionsPolicyEmptyPatternsPayload(t *testing.T) {
	var coreWrites atomic.Int32
	var selectedWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/widget/actions/permissions":
			coreWrites.Add(1)
		case "/repos/acme/widget/actions/permissions/selected-actions":
			selectedWrites.Add(1)
			var body map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if got := string(body["patterns_allowed"]); got != "[]" {
				t.Errorf("patterns_allowed payload = %s, want []", got)
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	patterns := []string{}
	desired := &model.ActionsSettings{
		AllowedActions:     new("selected"),
		GitHubOwnedAllowed: new(true),
		VerifiedAllowed:    new(true),
		PatternsAllowed:    &patterns,
	}
	current := &model.ActionsSettings{Enabled: new(true), AllowedActions: new("selected")}
	if _, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, false, true); err != nil {
		t.Fatal(err)
	}
	if coreWrites.Load() != 0 || selectedWrites.Load() != 1 {
		t.Fatalf("writes = core %d, selected %d", coreWrites.Load(), selectedWrites.Load())
	}
}

func TestApplyActionsPolicyRestrictsEnabledAllToSelected(t *testing.T) {
	patterns := []string{
		"softprops/action-gh-release@*",
		"golangci/golangci-lint-action@*",
		"golang/govulncheck-action@*",
		"robherley/go-test-action@*",
		"freerangebytes/setup-actionlint@*",
		"nick-fields/retry@*",
	}
	desired := &model.ActionsSettings{
		Enabled:            new(true),
		AllowedActions:     new("selected"),
		SHAPinningRequired: new(false),
		GitHubOwnedAllowed: new(true),
		VerifiedAllowed:    new(true),
		PatternsAllowed:    &patterns,
	}
	current := &model.ActionsSettings{
		Enabled:            new(true),
		AllowedActions:     new("all"),
		SHAPinningRequired: new(false),
	}

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/repos/acme/widget/actions/permissions":
			want := map[string]any{
				"enabled":              true,
				"allowed_actions":      "selected",
				"sha_pinning_required": false,
			}
			if !mapsEqual(body, want) {
				t.Errorf("core body = %v, want %v", body, want)
			}
		case "/repos/acme/widget/actions/permissions/selected-actions":
			if body["github_owned_allowed"] != true || body["verified_allowed"] != true {
				t.Errorf("selected body = %v, want GitHub-owned and verified actions", body)
			}
			gotPatterns, ok := body["patterns_allowed"].([]any)
			if !ok || len(gotPatterns) != 6 {
				t.Errorf("patterns_allowed = %v, want six patterns", body["patterns_allowed"])
				break
			}
			wantPatterns := slices.Clone(patterns)
			slices.Sort(wantPatterns)
			for i, want := range wantPatterns {
				if gotPatterns[i] != want {
					t.Errorf("patterns_allowed = %v, want %v", gotPatterns, wantPatterns)
					break
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	if _, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true); err != nil {
		t.Fatal(err)
	}
	wantCalls := []string{
		"/repos/acme/widget/actions/permissions",
		"/repos/acme/widget/actions/permissions/selected-actions",
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("calls = %v, want %v", calls, wantCalls)
	}
}

func TestApplyActionsPolicySelectedTransitionPayloadsAndFailures(t *testing.T) {
	const (
		corePath     = "/repos/acme/widget/actions/permissions"
		selectedPath = corePath + "/selected-actions"
		selectedBody = `{"github_owned_allowed":true,"patterns_allowed":["a/*","z/*"],"verified_allowed":false}`
	)
	coreBody := func(enabled bool) string {
		return fmt.Sprintf(`{"allowed_actions":"selected","enabled":%t,"sha_pinning_required":false}`, enabled)
	}
	call := func(path, body string) string { return path + " " + body }

	transitions := []struct {
		name    string
		enabled bool
		policy  string
		want    []string
	}{
		{
			name: "enabled all", enabled: true, policy: "all",
			want: []string{call(corePath, coreBody(true)), call(selectedPath, selectedBody)},
		},
		{
			name: "disabled all", policy: "all",
			want: []string{
				call(corePath, `{"allowed_actions":"selected","enabled":false}`),
				call(selectedPath, selectedBody),
				call(corePath, coreBody(false)),
			},
		},
		{
			name: "enabled local_only", enabled: true, policy: "local_only",
			want: []string{
				call(corePath, `{"allowed_actions":"selected","enabled":false}`),
				call(selectedPath, selectedBody),
				call(corePath, coreBody(true)),
			},
		},
		{
			name: "disabled local_only", policy: "local_only",
			want: []string{
				call(corePath, `{"allowed_actions":"selected","enabled":false}`),
				call(selectedPath, selectedBody),
				call(corePath, coreBody(false)),
			},
		},
		{
			name: "enabled selected", enabled: true, policy: "selected",
			want: []string{
				call(corePath, `{"enabled":false}`),
				call(selectedPath, selectedBody),
				call(corePath, coreBody(true)),
			},
		},
		{
			name: "disabled selected", policy: "selected",
			want: []string{call(selectedPath, selectedBody), call(corePath, coreBody(false))},
		},
	}
	outcomes := []struct {
		name       string
		failTarget string
	}{
		{name: "success"},
		{name: "core failure", failTarget: corePath},
		{name: "selected failure", failTarget: selectedPath},
	}

	for _, transition := range transitions {
		for _, outcome := range outcomes {
			t.Run(transition.name+"/"+outcome.name, func(t *testing.T) {
				var got []string
				failed := false
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read body: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					got = append(got, call(r.URL.Path, string(body)))
					if !failed && r.URL.Path == outcome.failTarget {
						failed = true
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				}))
				t.Cleanup(server.Close)

				patterns := []string{"z/*", "a/*"}
				currentPatterns := []string{"a/*"}
				desired := &model.ActionsSettings{
					Enabled:            new(transition.enabled),
					AllowedActions:     new("selected"),
					SHAPinningRequired: new(false),
					GitHubOwnedAllowed: new(true),
					VerifiedAllowed:    new(false),
					PatternsAllowed:    &patterns,
				}
				current := &model.ActionsSettings{
					Enabled:            new(transition.enabled),
					AllowedActions:     new(transition.policy),
					SHAPinningRequired: new(false),
				}
				if transition.policy == "selected" {
					current.GitHubOwnedAllowed = new(false)
					current.VerifiedAllowed = new(false)
					current.PatternsAllowed = &currentPatterns
				}

				_, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
				if outcome.failTarget == "" && err != nil {
					t.Fatalf("ApplyActionsPolicy() error = %v", err)
				}
				if outcome.failTarget != "" && err == nil {
					t.Fatal("ApplyActionsPolicy() error = nil, want write failure")
				}

				want := transition.want
				if outcome.failTarget != "" {
					for i, item := range want {
						if strings.HasPrefix(item, outcome.failTarget+" ") {
							want = want[:i+1]
							break
						}
					}
				}
				if !slices.Equal(got, want) {
					t.Fatalf("writes = %v, want %v", got, want)
				}
			})
		}
	}
}

func TestApplyActionsPolicyAllToSelectedDefersSHAPinningRelaxation(t *testing.T) {
	const (
		corePath     = "/repos/acme/widget/actions/permissions"
		selectedPath = corePath + "/selected-actions"
	)
	tests := []struct {
		name            string
		failWrite       int
		wantError       bool
		wantSHAPinning  bool
		wantWriteCount  int
		wantFinalPolicy string
	}{
		{name: "success", wantWriteCount: 3, wantFinalPolicy: "selected"},
		{name: "initial core failure", failWrite: 1, wantError: true, wantSHAPinning: true, wantWriteCount: 1, wantFinalPolicy: "all"},
		{name: "selected failure", failWrite: 2, wantError: true, wantSHAPinning: true, wantWriteCount: 2, wantFinalPolicy: "selected"},
		{name: "final core failure", failWrite: 3, wantError: true, wantSHAPinning: true, wantWriteCount: 3, wantFinalPolicy: "selected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := "all"
			shaPinning := true
			var writes []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				writes = append(writes, r.URL.Path+" "+string(body))
				if len(writes) == tt.failWrite {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if r.URL.Path == corePath {
					var update map[string]any
					if err := json.Unmarshal(body, &update); err != nil {
						t.Errorf("decode body: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if value, ok := update["allowed_actions"].(string); ok {
						policy = value
					}
					if value, ok := update["sha_pinning_required"].(bool); ok {
						shaPinning = value
					}
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)

			patterns := []string{"acme/*"}
			desired := &model.ActionsSettings{
				Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(false),
				GitHubOwnedAllowed: new(true), VerifiedAllowed: new(false), PatternsAllowed: &patterns,
			}
			current := &model.ActionsSettings{
				Enabled: new(true), AllowedActions: new("all"), SHAPinningRequired: new(true),
			}
			_, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
			if (err != nil) != tt.wantError {
				t.Fatalf("ApplyActionsPolicy() error = %v, wantError %t", err, tt.wantError)
			}
			if len(writes) != tt.wantWriteCount {
				t.Fatalf("write count = %d, want %d", len(writes), tt.wantWriteCount)
			}
			if policy != tt.wantFinalPolicy {
				t.Errorf("policy = %q, want %q", policy, tt.wantFinalPolicy)
			}
			if shaPinning != tt.wantSHAPinning {
				t.Errorf("sha_pinning_required = %t, want %t", shaPinning, tt.wantSHAPinning)
			}
			if len(writes) >= 1 {
				want := corePath + ` {"allowed_actions":"selected","enabled":true,"sha_pinning_required":true}`
				if writes[0] != want {
					t.Errorf("first write = %q, want %q", writes[0], want)
				}
			}
			if len(writes) >= 2 && !strings.HasPrefix(writes[1], selectedPath+" ") {
				t.Errorf("second write = %q, want selected permissions", writes[1])
			}
			if len(writes) == 3 {
				want := corePath + ` {"allowed_actions":"selected","enabled":true,"sha_pinning_required":false}`
				if writes[2] != want {
					t.Errorf("final write = %q, want %q", writes[2], want)
				}
			}
		})
	}
}

func TestApplyActionsPolicyAllToSelectedStopsAfterInitialCoreFailure(t *testing.T) {
	var selectedWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/selected-actions") {
			selectedWrites.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		fmt.Fprint(w, `{"message":"Conflict","errors":["The repository policy could not be changed"]}`)
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	desired := &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}
	current := &model.ActionsSettings{Enabled: new(true), AllowedActions: new("all")}
	_, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
	if err == nil || !strings.Contains(err.Error(), "The repository policy could not be changed") {
		t.Fatalf("ApplyActionsPolicy() error = %v, want GitHub error detail", err)
	}
	if selectedWrites.Load() != 0 {
		t.Fatalf("selected writes = %d, want 0", selectedWrites.Load())
	}
}

func TestApplyActionsPolicyAllToSelectedFailureLeavesSelectedRestriction(t *testing.T) {
	policy := "all"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/selected-actions") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			fmt.Fprint(w, `{"message":"Conflict","errors":["Selected actions policy update is in progress"]}`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		policy = body["allowed_actions"].(string)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	desired := &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}
	current := &model.ActionsSettings{Enabled: new(true), AllowedActions: new("all")}
	_, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
	if err == nil || !strings.Contains(err.Error(), "Selected actions policy update is in progress") {
		t.Fatalf("ApplyActionsPolicy() error = %v, want selected-policy error detail", err)
	}
	if !strings.Contains(err.Error(), "restricted to selected") {
		t.Fatalf("ApplyActionsPolicy() error = %v, want preserved failure state", err)
	}
	if policy != "selected" {
		t.Fatalf("policy = %q, want selected", policy)
	}
}

func TestApplyActionsPolicyPreDisabledSelectedAccessErrorSkipsCore(t *testing.T) {
	var coreWrites atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/selected-actions") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"Resource not accessible by integration"}`)
			return
		}
		coreWrites.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	patterns := []string{"acme/*"}
	desired := &model.ActionsSettings{
		Enabled: new(true), AllowedActions: new("selected"), PatternsAllowed: &patterns,
	}
	currentPatterns := []string{}
	current := &model.ActionsSettings{
		Enabled: new(false), AllowedActions: new("selected"), PatternsAllowed: &currentPatterns,
	}
	result, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
	if err != nil {
		t.Fatalf("ApplyActionsPolicy() error = %v, want access skip", err)
	}
	if len(result.Skipped) != 2 || result.Skipped[0].Operation.Kind != OpSetSelectedActionsPermissions || result.Skipped[1].Operation.Kind != OpSetActionsPermissions {
		t.Fatalf("ApplyActionsPolicy() skips = %+v, want selected write and dependent core write", result.Skipped)
	}
	if coreWrites.Load() != 0 {
		t.Fatalf("core writes = %d, want 0", coreWrites.Load())
	}
}

func mapsEqual(got, want map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func TestActionsHTTPErrorBoundsRenderedLiveResponse(t *testing.T) {
	details := make([]string, 4)
	for i := range details {
		details[i] = fmt.Sprintf("detail-%d-\x00\x1b[31m\t\r%s-PRIVATE-TAIL-%d", i, strings.Repeat("x", 300), i)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"message": "Conflict\x1b[2J",
			"errors":  details,
		}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	path := "repos/acme/widget/actions/permissions"
	err := putActionsPolicy(newTestClient(t, server), path, map[string]any{"enabled": true})
	if err == nil {
		t.Fatal("putActionsPolicy() error = nil, want HTTP error")
	}
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("putActionsPolicy() error type = %T, want *api.HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", httpErr.StatusCode, http.StatusConflict)
	}
	if got, want := httpErr.RequestURL.String(), server.URL+"/"+path; got != want {
		t.Errorf("request URL = %q, want %q", got, want)
	}
	if len(httpErr.Errors) != 3 {
		t.Errorf("detail count = %d, want 3", len(httpErr.Errors))
	}
	rendered := err.Error()
	if strings.ContainsAny(rendered, "\x00\x1b\r\t") {
		t.Errorf("error contains terminal control characters: %q", rendered)
	}
	for i := range details {
		if strings.Contains(rendered, details[i]) || strings.Contains(rendered, fmt.Sprintf("PRIVATE-TAIL-%d", i)) {
			t.Errorf("error contains unbounded detail %d: %q", i, rendered)
		}
	}
	if strings.Contains(rendered, "detail-3-") {
		t.Errorf("error contains fourth detail: %q", rendered)
	}
	if len(rendered) > 1200 {
		t.Errorf("rendered error length = %d, want at most 1200", len(rendered))
	}
}

func TestApplyActionsPolicyDisablesBeforeSelectedBroadeningAndSHAPinning(t *testing.T) {
	tests := []struct {
		name            string
		desiredGitHub   bool
		currentGitHub   bool
		desiredVerified bool
		currentVerified bool
		desiredPatterns []string
		currentPatterns []string
	}{
		{
			name:            "GitHub-owned actions",
			desiredGitHub:   true,
			currentGitHub:   false,
			desiredPatterns: []string{"acme/*"},
			currentPatterns: []string{"acme/*"},
		},
		{
			name:            "verified actions",
			desiredVerified: true,
			currentVerified: false,
			desiredPatterns: []string{"acme/*"},
			currentPatterns: []string{"acme/*"},
		},
		{
			name:            "pattern addition",
			desiredPatterns: []string{"acme/*", "octo/*"},
			currentPatterns: []string{"acme/*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				calls = append(calls, r.URL.Path+" "+fmt.Sprint(body["enabled"]))
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)

			desired := &model.ActionsSettings{
				Enabled:            new(true),
				AllowedActions:     new("selected"),
				SHAPinningRequired: new(true),
				GitHubOwnedAllowed: new(tt.desiredGitHub),
				VerifiedAllowed:    new(tt.desiredVerified),
				PatternsAllowed:    &tt.desiredPatterns,
			}
			current := &model.ActionsSettings{
				Enabled:            new(true),
				AllowedActions:     new("selected"),
				SHAPinningRequired: new(false),
				GitHubOwnedAllowed: new(tt.currentGitHub),
				VerifiedAllowed:    new(tt.currentVerified),
				PatternsAllowed:    &tt.currentPatterns,
			}
			if _, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true); err != nil {
				t.Fatal(err)
			}

			want := []string{
				"/repos/acme/widget/actions/permissions false",
				"/repos/acme/widget/actions/permissions/selected-actions <nil>",
				"/repos/acme/widget/actions/permissions true",
			}
			if !slices.Equal(calls, want) {
				t.Fatalf("calls = %v, want %v", calls, want)
			}
		})
	}
}

func TestSelectedPolicyBroadensPatterns(t *testing.T) {
	tests := []struct {
		name    string
		desired []string
		current []string
		want    bool
	}{
		{name: "positive addition", desired: []string{"acme/*", "octo/*"}, current: []string{"acme/*"}, want: true},
		{name: "positive removal", desired: []string{"acme/*"}, current: []string{"acme/*", "octo/*"}},
		{name: "exclusion addition", desired: []string{"*", "!evil/*"}, current: []string{"*"}},
		{name: "exclusion removal", desired: []string{"*"}, current: []string{"*", "!evil/*"}, want: true},
		{name: "broader exclusion replacement", desired: []string{"*", "!evil/*"}, current: []string{"*", "!evil/tool@*"}},
		{name: "narrower exclusion replacement", desired: []string{"*", "!evil/tool@*"}, current: []string{"*", "!evil/*"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desired := &model.ActionsSettings{PatternsAllowed: &tt.desired}
			current := &model.ActionsSettings{PatternsAllowed: &tt.current}
			if got := selectedPolicyBroadens(desired, current); got != tt.want {
				t.Fatalf("selectedPolicyBroadens() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestApplyActionsPolicyMixedTighteningFailureLeavesActionsDisabled(t *testing.T) {
	tests := []struct {
		name            string
		failStep        string
		desiredSHAPin   bool
		currentSHAPin   bool
		desiredPatterns []string
		currentPatterns []string
	}{
		{
			name:            "selected write after pattern addition",
			failStep:        "selected",
			desiredSHAPin:   true,
			desiredPatterns: []string{"acme/*", "octo/*"},
			currentPatterns: []string{"acme/*"},
		},
		{
			name:            "final core write after pattern addition",
			failStep:        "final core",
			desiredSHAPin:   true,
			desiredPatterns: []string{"acme/*", "octo/*"},
			currentPatterns: []string{"acme/*"},
		},
		{
			name:            "final core write after pattern exclusion removal",
			failStep:        "final core",
			desiredSHAPin:   true,
			desiredPatterns: []string{"*"},
			currentPatterns: []string{"*", "!evil/*"},
		},
		{
			name:            "final core write after SHA pinning relaxation",
			failStep:        "final core",
			currentSHAPin:   true,
			desiredPatterns: []string{"acme/*", "octo/*"},
			currentPatterns: []string{"acme/*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := true
			coreWrites := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/selected-actions") {
					if tt.failStep == "selected" {
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprint(w, `{"message":"failed"}`)
						return
					}
					w.WriteHeader(http.StatusNoContent)
					return
				}

				coreWrites++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if coreWrites == 2 && tt.failStep == "final core" {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Fprint(w, `{"message":"failed"}`)
					return
				}
				enabled = body["enabled"].(bool)
				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)

			desired := &model.ActionsSettings{
				Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(tt.desiredSHAPin),
				GitHubOwnedAllowed: new(false), VerifiedAllowed: new(false), PatternsAllowed: &tt.desiredPatterns,
			}
			current := &model.ActionsSettings{
				Enabled: new(true), AllowedActions: new("selected"), SHAPinningRequired: new(tt.currentSHAPin),
				GitHubOwnedAllowed: new(false), VerifiedAllowed: new(false), PatternsAllowed: &tt.currentPatterns,
			}
			_, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, true, true)
			if err == nil || !strings.Contains(err.Error(), "while actions are disabled") {
				t.Fatalf("ApplyActionsPolicy() error = %v, want disabled-state failure", err)
			}
			if enabled {
				t.Fatal("Actions enabled after the mixed update failed")
			}
		})
	}
}
