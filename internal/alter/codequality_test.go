package alter_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func codeQualityStub() *setupStub {
	return &setupStub{
		path:        "/repos/acme/widget/code-quality/setup",
		readStatus:  http.StatusOK,
		readBody:    `{"state":"not-configured","languages":["go"],"ai_findings_option":"disabled"}`,
		patchStatus: http.StatusAccepted,
	}
}

func TestProcessCodeQualityAbsentMakesNoCalls(t *testing.T) {
	for name, cfg := range map[string]*config.Config{
		"nil section":     {},
		"empty languages": {CodeQuality: &model.CodeQualitySettings{Languages: &[]string{}}},
	} {
		t.Run(name, func(t *testing.T) {
			results, err := alter.ProcessCodeQuality(cfg, alter.Apply, repoTarget(nil, "acme", "widget", true))
			if err != nil || results != nil {
				t.Fatalf("ProcessCodeQuality() = %v, %v", results, err)
			}
		})
	}
}

func TestProcessCodeQualityCompare(t *testing.T) {
	tests := []struct {
		name     string
		declared *model.CodeQualitySettings
		want     []string
	}{
		{
			name:     "match with automatic languages",
			declared: &model.CodeQualitySettings{State: new("not-configured"), Languages: &[]string{}},
			want:     []string{"code_quality.state no change not-configured "},
		},
		{
			name:     "state and languages differ",
			declared: &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{"python", "go"}},
			want: []string{
				"code_quality.state would set configured ",
				"code_quality.languages would set go, python ",
			},
		},
		{
			name:     "languages match",
			declared: &model.CodeQualitySettings{Languages: &[]string{"go"}},
			want:     []string{"code_quality.languages no change go "},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := codeQualityStub()
			client := testutil.NewTestClient(t, stub.server(t))
			results, err := alter.ProcessCodeQuality(&config.Config{CodeQuality: tt.declared}, alter.DryRun, repoTarget(client, "acme", "widget", true))
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

func TestProcessCodeQualityApply(t *testing.T) {
	stub := codeQualityStub()
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeQuality: &model.CodeQualitySettings{State: new("configured"), Languages: &[]string{"go"}}}
	results, err := alter.ProcessCodeQuality(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{
		"code_quality.state would set configured ",
		"code_quality.languages no change go ",
	})
	if stub.writes.Load() != 1 {
		t.Fatalf("writes = %d, want 1", stub.writes.Load())
	}
	if got, _ := json.Marshal(stub.lastBody); string(got) != `{"state":"configured"}` {
		t.Errorf("PATCH body = %s, want state only", got)
	}
}

func TestProcessCodeQualityReadNotAvailable(t *testing.T) {
	stub := codeQualityStub()
	stub.readStatus = http.StatusForbidden
	stub.readBody = `{"message":"Forbidden"}`
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeQuality: &model.CodeQualitySettings{State: new("configured")}}
	results, err := alter.ProcessCodeQuality(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{"code_quality.state would skip  not available"})
	if stub.writes.Load() != 0 {
		t.Fatalf("writes = %d, want 0", stub.writes.Load())
	}
}

func TestProcessCodeQualityWriteInProgress(t *testing.T) {
	stub := codeQualityStub()
	stub.patchStatus = http.StatusConflict
	client := testutil.NewTestClient(t, stub.server(t))
	cfg := &config.Config{CodeQuality: &model.CodeQualitySettings{State: new("configured")}}
	results, err := alter.ProcessCodeQuality(cfg, alter.Apply, repoTarget(client, "acme", "widget", true))
	if err != nil {
		t.Fatal(err)
	}
	assertResults(t, results, []string{"code_quality.state would skip  setup in progress"})
}
