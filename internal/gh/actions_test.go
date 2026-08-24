package gh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/ptr"
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
	desired := &model.ActionsSettings{AllowedActions: ptr.Ptr("selected"), PatternsAllowed: &patterns}
	current := &model.ActionsSettings{Enabled: ptr.Ptr(true)}
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
		AllowedActions:     ptr.Ptr("selected"),
		GitHubOwnedAllowed: ptr.Ptr(true),
		VerifiedAllowed:    ptr.Ptr(true),
		PatternsAllowed:    &patterns,
	}
	current := &model.ActionsSettings{Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected")}
	if _, err := ApplyActionsPolicy(newTestClient(t, server), "acme", "widget", desired, current, false, true); err != nil {
		t.Fatal(err)
	}
	if coreWrites.Load() != 0 || selectedWrites.Load() != 1 {
		t.Fatalf("writes = core %d, selected %d", coreWrites.Load(), selectedWrites.Load())
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
				Enabled:            ptr.Ptr(true),
				AllowedActions:     ptr.Ptr("selected"),
				SHAPinningRequired: ptr.Ptr(true),
				GitHubOwnedAllowed: ptr.Ptr(tt.desiredGitHub),
				VerifiedAllowed:    ptr.Ptr(tt.desiredVerified),
				PatternsAllowed:    &tt.desiredPatterns,
			}
			current := &model.ActionsSettings{
				Enabled:            ptr.Ptr(true),
				AllowedActions:     ptr.Ptr("selected"),
				SHAPinningRequired: ptr.Ptr(false),
				GitHubOwnedAllowed: ptr.Ptr(tt.currentGitHub),
				VerifiedAllowed:    ptr.Ptr(tt.currentVerified),
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
				Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(tt.desiredSHAPin),
				GitHubOwnedAllowed: ptr.Ptr(false), VerifiedAllowed: ptr.Ptr(false), PatternsAllowed: &tt.desiredPatterns,
			}
			current := &model.ActionsSettings{
				Enabled: ptr.Ptr(true), AllowedActions: ptr.Ptr("selected"), SHAPinningRequired: ptr.Ptr(tt.currentSHAPin),
				GitHubOwnedAllowed: ptr.Ptr(false), VerifiedAllowed: ptr.Ptr(false), PatternsAllowed: &tt.currentPatterns,
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
