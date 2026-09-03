package alter_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// setupStub configures the fake setup endpoint for one test.
type setupStub struct {
	path        string
	readStatus  int
	readBody    string
	patchStatus int
	writes      atomic.Int32
	lastBody    map[string]any
}

func (s *setupStub) server(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != s.path {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(s.readStatus)
			fmt.Fprint(w, s.readBody)
		case http.MethodPatch:
			s.writes.Add(1)
			data, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(data, &s.lastBody)
			w.WriteHeader(s.patchStatus)
			fmt.Fprint(w, `{"run_id":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func codeScanningStub() *setupStub {
	return &setupStub{
		path:        "/repos/acme/widget/code-scanning/default-setup",
		readStatus:  http.StatusOK,
		readBody:    `{"state":"not-configured","languages":["actions","go"],"query_suite":"default","threat_model":"remote"}`,
		patchStatus: http.StatusAccepted,
	}
}

func resultKey(result alter.RepoSettingResult) string {
	return fmt.Sprintf("%s.%s %s %s %s", result.Section, result.Field, result.Category, result.Value, result.Annotation)
}

func assertResults(t *testing.T, got []alter.RepoSettingResult, want []string) {
	t.Helper()
	keys := make([]string, 0, len(got))
	for _, result := range got {
		keys = append(keys, resultKey(result))
	}
	if strings.Join(keys, "\n") != strings.Join(want, "\n") {
		t.Errorf("results:\n%s\nwant:\n%s", strings.Join(keys, "\n"), strings.Join(want, "\n"))
	}
}

func TestProcessCodeScanningAbsentMakesNoCalls(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"nil section":     {},
		"empty section":   {CodeScanning: &model.CodeScanningSettings{}},
		"empty languages": {CodeScanning: &model.CodeScanningSettings{Languages: &[]string{}}},
	} {
		t.Run(name, func(t *testing.T) {
			results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(nil, "acme", "widget", true))
			if err != nil || results != nil {
				t.Fatalf("ProcessCodeScanning() = %v, %v", results, err)
			}
		})
	}
}

func TestProcessCodeScanningNoRepoContext(t *testing.T) {
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("configured")}}
	results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(nil, "", "", false))
	if err != nil || results != nil {
		t.Fatalf("ProcessCodeScanning() = %v, %v", results, err)
	}
}

func TestProcessCodeScanningCompare(t *testing.T) {
	tests := []struct {
		name     string
		declared *model.CodeScanningSettings
		want     []string
	}{
		{
			name:     "all match with automatic languages",
			declared: &model.CodeScanningSettings{State: new("not-configured"), QuerySuite: new("default"), ThreatModel: new("remote"), Languages: &[]string{}},
			want: []string{
				"code_scanning.state no change not-configured ",
				"code_scanning.query_suite no change default ",
				"code_scanning.threat_model no change remote ",
			},
		},
		{
			name:     "state differs",
			declared: &model.CodeScanningSettings{State: new("configured")},
			want:     []string{"code_scanning.state would set configured "},
		},
		{
			name:     "languages equal as a set",
			declared: &model.CodeScanningSettings{Languages: &[]string{"go", "actions"}},
			want:     []string{"code_scanning.languages no change actions, go "},
		},
		{
			name:     "languages differ",
			declared: &model.CodeScanningSettings{Languages: &[]string{"python", "go"}},
			want:     []string{"code_scanning.languages would set go, python "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := codeScanningStub()
			client := testutil.NewTestClient(t, stub.server(t))
			results, err := alter.ProcessCodeScanning(&config.Config{CodeScanning: tt.declared}, alter.DryRun, repoTarget(client, "acme", "widget", true))
			if err != nil {
				t.Fatal(err)
			}
			assertResults(t, results, tt.want)
			if stub.writes.Load() != 0 {
				t.Fatalf("dry run made %d writes", stub.writes.Load())
			}
		})
	}
}

func TestProcessCodeScanningApplySendsOnlyChangedFields(t *testing.T) {
	stub := codeScanningStub()
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{
		State: new("configured"), QuerySuite: new("default"), ThreatModel: new("remote_and_local"), Languages: &[]string{},
	}}
	results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{
		"code_scanning.state would set configured ",
		"code_scanning.query_suite no change default ",
		"code_scanning.threat_model would set remote_and_local ",
	})
	if stub.writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", stub.writes.Load())
	}
	want := `{"state":"configured","threat_model":"remote_and_local"}`
	if got, _ := json.Marshal(stub.lastBody); string(got) != want {
		t.Errorf("PATCH body = %s, want %s", got, want)
	}
}

func TestProcessCodeScanningApplyLanguagesOnly(t *testing.T) {
	stub := codeScanningStub()
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{
		State: new("not-configured"), QuerySuite: new("default"), ThreatModel: new("remote"), Languages: &[]string{"python", "go"},
	}}
	results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{
		"code_scanning.state no change not-configured ",
		"code_scanning.query_suite no change default ",
		"code_scanning.threat_model no change remote ",
		"code_scanning.languages would set go, python ",
	})
	if stub.writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", stub.writes.Load())
	}
	want := `{"languages":["go","python"]}`
	if got, _ := json.Marshal(stub.lastBody); string(got) != want {
		t.Errorf("PATCH body = %s, want %s", got, want)
	}
}

func TestProcessCodeScanningApplyNoChangeSendsNothing(t *testing.T) {
	stub := codeScanningStub()
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("not-configured")}}
	if _, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true)); err != nil {
		t.Fatal(err)
	}
	if stub.writes.Load() != 0 {
		t.Fatalf("writes = %d, want 0", stub.writes.Load())
	}
}

func TestProcessCodeScanningReadNotAvailable(t *testing.T) {
	for name, status := range map[string]int{"forbidden": http.StatusForbidden, "not found": http.StatusNotFound} {
		t.Run(name, func(t *testing.T) {
			stub := codeScanningStub()
			stub.readStatus = status
			stub.readBody = `{"message":"Not Found"}`
			client := testutil.NewTestClient(t, stub.server(t))
			cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("configured"), Languages: &[]string{"go"}}}
			results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
			if err != nil {
				t.Fatal(err)
			}
			assertResults(t, results, []string{
				"code_scanning.state would skip  not available",
				"code_scanning.languages would skip  not available",
			})
			if stub.writes.Load() != 0 {
				t.Fatalf("writes = %d, want 0", stub.writes.Load())
			}
		})
	}
}

func TestProcessCodeScanningReadHardError(t *testing.T) {
	stub := codeScanningStub()
	stub.readStatus = http.StatusInternalServerError
	stub.readBody = `{"message":"boom"}`
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("configured")}}
	if _, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true)); err == nil {
		t.Fatal("ProcessCodeScanning() expected error for 500")
	}
}

func TestProcessCodeScanningWriteSkipped(t *testing.T) {
	tests := []struct {
		name   string
		status int
		reason string
	}{
		{name: "setup in progress", status: http.StatusConflict, reason: "setup in progress"},
		{name: "not available", status: http.StatusForbidden, reason: "not available"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := codeScanningStub()
			stub.patchStatus = tt.status
			client := testutil.NewTestClient(t, stub.server(t))
			cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("configured"), QuerySuite: new("default")}}
			results, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
			if err != nil {
				t.Fatal(err)
			}
			assertResults(t, results, []string{
				"code_scanning.state would skip  " + tt.reason,
				"code_scanning.query_suite no change default ",
			})

			output := alter.FormatOutput(results, nil, nil, alter.Apply)
			want := "no change:                           code_scanning.query_suite (already default)\n" +
				"would skip (" + tt.reason + "):" + strings.Repeat(" ", 37-len("would skip ("+tt.reason+"):")) + "code_scanning.state\n"
			if output != want {
				t.Errorf("FormatOutput() =\n%s\nwant:\n%s", output, want)
			}
		})
	}
}

func TestProcessCodeScanningWriteHardError(t *testing.T) {
	stub := codeScanningStub()
	stub.patchStatus = http.StatusInternalServerError
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeScanning: &model.CodeScanningSettings{State: new("configured")}}
	if _, err := alter.ProcessCodeScanning(cfg, alter.Apply, repoTarget(client, "acme", "widget", true)); err == nil {
		t.Fatal("ProcessCodeScanning() expected error for 500")
	}
}

func TestFormatOutputSetupSections(t *testing.T) {
	results := []alter.RepoSettingResult{
		{Section: "code_quality", Field: "state", Category: alter.RepoNoChange, Value: "not-configured"},
		{Section: "code_scanning", Field: "state", Category: alter.WouldSet, Value: "configured"},
		{Section: "code_scanning", Field: "languages", Category: alter.WouldSet, Value: "actions, go"},
	}
	got := alter.FormatOutput(results, nil, nil, alter.DryRun)
	want := "would set:                           code_scanning.languages = actions, go\n" +
		"would set:                           code_scanning.state = configured\n" +
		"no change:                           code_quality.state (already not-configured)\n"
	if got != want {
		t.Errorf("FormatOutput() =\n%s\nwant:\n%s", got, want)
	}
}
