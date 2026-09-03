package gh

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

const codeQualitySetupJSON = `{"state":"not-configured","languages":["go"],"updated_at":null,"schedule":null,"runner_type":"standard","runner_label":null,"ai_findings_option":"disabled"}`

func TestReadCodeQualitySetup(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantReason SetupSkipReason
		wantErr    string
	}{
		{name: "ok", status: http.StatusOK, body: codeQualitySetupJSON},
		{name: "forbidden", status: http.StatusForbidden, body: `{"message":"Forbidden"}`, wantReason: SetupNotAvailable},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, wantReason: SetupNotAvailable},
		{name: "server error", status: http.StatusInternalServerError, body: `{"message":"boom"}`, wantErr: "fetch code quality setup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := setupServer(t, "/repos/acme/widget/code-quality/setup", tt.status, tt.body, nil)
			got, err := ReadCodeQualitySetup(testutil.NewTestClient(t, server), "acme", "widget")
			assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, OpFetchCodeQualitySetup)
			if err != nil {
				return
			}
			testutil.AssertPtrEqual(t, got.State, new("not-configured"), "state")
			if got.Languages == nil || len(*got.Languages) != 1 || (*got.Languages)[0] != "go" {
				t.Fatalf("languages = %v, want [go]", got.Languages)
			}
		})
	}
}

func TestApplyCodeQualitySetup(t *testing.T) {
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
		{name: "server error", status: http.StatusInternalServerError, body: `{"message":"boom"}`, wantErr: "set code quality setup"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var patched map[string]any
			server := setupServer(t, "/repos/acme/widget/code-quality/setup", tt.status, tt.body, &patched)
			desired := &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{"python", "go"}}
			err := ApplyCodeQualitySetup(testutil.NewTestClient(t, server), "acme", "widget", desired)
			assertSetupOutcome(t, err, tt.wantReason, tt.wantErr, OpSetCodeQualitySetup)
			want := `{"languages":["go","python"],"state":"configured"}`
			if got, _ := json.Marshal(patched); string(got) != want {
				t.Errorf("PATCH body = %s, want %s", got, want)
			}
		})
	}
}

func TestCodeQualitySetupBody(t *testing.T) {
	tests := []struct {
		name    string
		desired *model.CodeQualitySettings
		want    string
	}{
		{name: "empty", desired: &model.CodeQualitySettings{}, want: `{}`},
		{name: "empty languages omitted", desired: &model.CodeQualitySettings{State: new("not-configured"), Languages: &[]string{}}, want: `{"state":"not-configured"}`},
		{name: "languages only", desired: &model.CodeQualitySettings{Languages: &[]string{"python", "go"}}, want: `{"languages":["go","python"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := json.Marshal(codeQualitySetupBody(tt.desired))
			if string(got) != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}
