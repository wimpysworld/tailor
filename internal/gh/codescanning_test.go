package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

const codeScanningSetupJSON = `{"state":"not-configured","languages":["actions","go"],"query_suite":"default","threat_model":"remote","updated_at":null,"schedule":null,"runner_type":"standard","runner_label":null}`

// setupServer answers the setup endpoint with status and body, recording
// the last PATCH body.
func setupServer(t *testing.T, path string, status int, body string, patched *map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPatch && patched != nil {
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, patched)
		}
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "60")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestReadCodeScanningSetup(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantReason SetupSkipReason
		wantErr    string
	}{
		{name: "ok", status: http.StatusOK, body: codeScanningSetupJSON},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"Forbidden"}`, wantReason: SetupNotAvailable},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, wantReason: SetupNotAvailable},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"message":"rate limit exceeded"}`, wantErr: "rate limited"},
		{name: "server error", status: http.StatusInternalServerError, body: `{"message":"boom"}`, wantErr: "fetch code scanning setup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupServer(t, "/repos/acme/widget/code-scanning/default-setup", tt.status, tt.body, nil)
			got, err := ReadCodeScanningSetup(testutil.NewTestClient(t, server), "acme", "widget")
			assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, OpFetchCodeScanningSetup)
			if err != nil {
				return
			}
			testutil.AssertPtr(t, got.State, false, "not-configured", "state")
			testutil.AssertPtr(t, got.QuerySuite, false, "default", "query_suite")
			testutil.AssertPtr(t, got.ThreatModel, false, "remote", "threat_model")
			if got.Languages == nil || len(*got.Languages) != 2 {
				t.Fatalf("languages = %v, want [actions go]", got.Languages)
			}
		})
	}
}

func assertSetupOutcome(t *testing.T, err error, wantReason SetupSkipReason, wantErr string, wantKind OperationKind) {
	t.Helper()
	var skipped *ErrSetupSkipped
	switch {
	case wantReason != "":
		if !errors.As(err, &skipped) || skipped.Reason != wantReason || skipped.Operation.Kind != wantKind {
			t.Fatalf("error = %v, want skip %q", err, wantReason)
		}
	case wantErr != "":
		if err == nil || errors.As(err, &skipped) || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("error = %v, want hard error containing %q", err, wantErr)
		}
	default:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestApplyCodeScanningSetup(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantReason SetupSkipReason
		wantErr    string
	}{
		{name: "accepted", status: http.StatusAccepted, body: `{"run_id":1,"run_url":"https://example.test/runs/1"}`},
		{name: "in progress", status: http.StatusConflict, body: `{"message":"setup in progress"}`, wantReason: SetupInProgress},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"Forbidden"}`, wantReason: SetupNotAvailable},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, wantErr: "set code scanning setup"},
		{name: "server error", status: http.StatusInternalServerError, body: `{"message":"boom"}`, wantErr: "set code scanning setup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var patched map[string]any
			server := setupServer(t, "/repos/acme/widget/code-scanning/default-setup", tt.status, tt.body, &patched)
			desired := &model.CodeScanningSettings{State: new("configured"), Languages: &[]string{"go", "actions"}}
			err := ApplyCodeScanningSetup(testutil.NewTestClient(t, server), "acme", "widget", desired)
			assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, OpSetCodeScanningSetup)
			want := `{"languages":["actions","go"],"state":"configured"}`
			if got, _ := json.Marshal(patched); string(got) != want {
				t.Errorf("PATCH body = %s, want %s", got, want)
			}
		})
	}
}

func TestReadSetupLeavesEmptyFieldsNil(t *testing.T) {
	scanningServer := setupServer(t, "/repos/acme/widget/code-scanning/default-setup", http.StatusOK, `{"state":"not-configured","languages":[]}`, nil)
	scanning, err := ReadCodeScanningSetup(testutil.NewTestClient(t, scanningServer), "acme", "widget")
	if err != nil {
		t.Fatalf("ReadCodeScanningSetup() error: %v", err)
	}
	testutil.AssertPtr(t, scanning.State, false, "not-configured", "state")
	testutil.AssertPtr(t, scanning.QuerySuite, true, "", "query_suite")
	testutil.AssertPtr(t, scanning.ThreatModel, true, "", "threat_model")

	qualityServer := setupServer(t, "/repos/acme/widget/code-quality/setup", http.StatusOK, `{"languages":[]}`, nil)
	quality, err := ReadCodeQualitySetup(testutil.NewTestClient(t, qualityServer), "acme", "widget")
	if err != nil {
		t.Fatalf("ReadCodeQualitySetup() error: %v", err)
	}
	testutil.AssertPtr(t, quality.State, true, "", "state")
}

func TestApplyCodeScanningSetupSendsNothingWhenEmpty(t *testing.T) {
	var patched map[string]any
	server := setupServer(t, "/repos/acme/widget/code-scanning/default-setup", http.StatusAccepted, `{}`, &patched)
	err := ApplyCodeScanningSetup(testutil.NewTestClient(t, server), "acme", "widget", &model.CodeScanningSettings{Languages: &[]string{}})
	if err != nil {
		t.Fatalf("ApplyCodeScanningSetup() error: %v", err)
	}
	if patched != nil {
		t.Fatalf("PATCH body = %v, want no request", patched)
	}
}

func TestCodeScanningSetupBody(t *testing.T) {
	tests := []struct {
		name    string
		desired *model.CodeScanningSettings
		want    string
	}{
		{name: "empty", desired: &model.CodeScanningSettings{}, want: `{}`},
		{name: "empty languages omitted", desired: &model.CodeScanningSettings{State: new("configured"), Languages: &[]string{}}, want: `{"state":"configured"}`},
		{
			name:    "all managed fields",
			desired: &model.CodeScanningSettings{State: new("configured"), QuerySuite: new("extended"), ThreatModel: new("remote_and_local"), Languages: &[]string{"python", "go"}},
			want:    `{"languages":["go","python"],"query_suite":"extended","state":"configured","threat_model":"remote_and_local"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := json.Marshal(codeScanningSetupBody(tt.desired))
			if string(got) != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}
