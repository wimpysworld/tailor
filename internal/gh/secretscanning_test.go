package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestReadRepoSettingsSecurityAndAnalysis(t *testing.T) {
	tests := []struct {
		name               string
		repo               string
		wantWarning        bool
		wantScanning       *string
		wantPushProtection *string
		wantNonProvider    *string
	}{
		{
			name:               "present",
			repo:               `{"permissions":{"admin":true},"security_and_analysis":{"secret_scanning":{"status":"enabled"},"secret_scanning_push_protection":{"status":"disabled"},"secret_scanning_non_provider_patterns":{"status":"enabled"}}}`,
			wantScanning:       new("enabled"),
			wantPushProtection: new("disabled"),
			wantNonProvider:    new("enabled"),
		},
		{
			name:         "partial block",
			repo:         `{"permissions":{"admin":true},"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`,
			wantScanning: new("enabled"),
		},
		{
			name:        "absent block",
			repo:        `{"permissions":{"admin":false}}`,
			wantWarning: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/testowner/testrepo":
					fmt.Fprint(w, tt.repo)
				case "/repos/testowner/testrepo/actions/permissions/workflow":
					fmt.Fprint(w, wfPermsReadJSON)
				case "/repos/testowner/testrepo/private-vulnerability-reporting", "/repos/testowner/testrepo/automated-security-fixes":
					fmt.Fprint(w, `{"enabled":true}`)
				case "/repos/testowner/testrepo/vulnerability-alerts":
					w.WriteHeader(http.StatusNoContent)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			settings, warnings, err := ReadRepoSettings(testutil.NewTestClient(t, server), "testowner", "testrepo")
			if err != nil {
				t.Fatalf("ReadRepoSettings() error: %v", err)
			}
			testutil.AssertPtr(t, settings.SecretScanning, tt.wantScanning == nil, derefString(tt.wantScanning), "secret_scanning")
			testutil.AssertPtr(t, settings.SecretScanningPushProtection, tt.wantPushProtection == nil, derefString(tt.wantPushProtection), "secret_scanning_push_protection")
			testutil.AssertPtr(t, settings.SecretScanningNonProviderPatterns, tt.wantNonProvider == nil, derefString(tt.wantNonProvider), "secret_scanning_non_provider_patterns")
			if !tt.wantWarning {
				if len(warnings) != 0 {
					t.Fatalf("warnings = %v, want none", warnings)
				}
				return
			}
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want one access warning", warnings)
			}
			var scopeErr *ErrInsufficientScope
			if !errors.As(warnings[0], &scopeErr) || scopeErr.Operation.Kind != OpFetchSecurityAnalysis {
				t.Fatalf("warning = %v, want fetch security and analysis access warning", warnings[0])
			}
			wantText := "fetch security and analysis: security_and_analysis is absent from the repository response; the token lacks admin access"
			if got := warnings[0].Error(); got != wantText {
				t.Errorf("warning text = %q, want %q", got, wantText)
			}
		})
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func TestBuildSettingsPayloadSecurityAndAnalysis(t *testing.T) {
	tests := []struct {
		name     string
		settings *model.RepositorySettings
		want     map[string]any
	}{
		{
			name:     "all fields",
			settings: &model.RepositorySettings{SecretScanning: new("enabled"), SecretScanningPushProtection: new("disabled"), SecretScanningNonProviderPatterns: new("enabled")},
			want: map[string]any{
				"secret_scanning":                       map[string]any{"status": "enabled"},
				"secret_scanning_push_protection":       map[string]any{"status": "disabled"},
				"secret_scanning_non_provider_patterns": map[string]any{"status": "enabled"},
			},
		},
		{
			name:     "non-provider patterns only",
			settings: &model.RepositorySettings{SecretScanningNonProviderPatterns: new("disabled")},
			want:     map[string]any{"secret_scanning_non_provider_patterns": map[string]any{"status": "disabled"}},
		},
		{
			name:     "push protection only",
			settings: &model.RepositorySettings{SecretScanningPushProtection: new("enabled"), HasWiki: new(false)},
			want:     map[string]any{"secret_scanning_push_protection": map[string]any{"status": "enabled"}},
		},
		{
			name:     "neither field",
			settings: &model.RepositorySettings{HasWiki: new(false)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := buildSettingsPayload(tt.settings)
			for _, key := range []string{"secret_scanning", "secret_scanning_push_protection", "secret_scanning_non_provider_patterns"} {
				if _, ok := body[key]; ok {
					t.Errorf("%s must not appear in the flat PATCH body", key)
				}
			}
			got, ok := body["security_and_analysis"]
			if tt.want == nil {
				if ok {
					t.Fatalf("security_and_analysis = %v, want absent", got)
				}
				return
			}
			if !ok {
				t.Fatal("security_and_analysis missing from PATCH body")
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("security_and_analysis = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyRepoSettingsSecretScanningPatch(t *testing.T) {
	var patches int
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/repos/testowner/testrepo" {
			patches++
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &gotBody)
			fmt.Fprint(w, `{}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	settings := &model.RepositorySettings{SecretScanning: new("enabled")}
	if _, err := ApplyRepoSettings(client, "testowner", "testrepo", settings, nil); err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}
	if patches != 1 {
		t.Fatalf("PATCH calls = %d, want 1", patches)
	}
	want := `{"security_and_analysis":{"secret_scanning":{"status":"enabled"}}}`
	if got, _ := json.Marshal(gotBody); string(got) != want {
		t.Errorf("PATCH body = %s, want %s", got, want)
	}

	// A body with no PATCH fields sends no PATCH. The topics PUT answers 404,
	// which the apply records as a skip rather than an error.
	patches = 0
	topics := []string{"go"}
	result, err := ApplyRepoSettings(client, "testowner", "testrepo", &model.RepositorySettings{Topics: &topics}, nil)
	if err != nil {
		t.Fatalf("ApplyRepoSettings() error: %v", err)
	}
	if patches != 0 {
		t.Fatalf("PATCH calls = %d, want 0 for an empty body", patches)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Operation.Kind != OpSetTopics {
		t.Fatalf("skipped = %v, want set topics", result.Skipped)
	}
}
