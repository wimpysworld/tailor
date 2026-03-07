package alter_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/ghfake"
	"github.com/wimpysworld/tailor/internal/testutil"
)

// labelsServer creates an httptest server that responds to label GET, POST,
// and PATCH requests. writeCalled is incremented on POST or PATCH.
func labelsServer(current []config.LabelEntry, writeCalled *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		switch {
		case r.Method == http.MethodGet && path == "/repos/testowner/testrepo/labels":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(current)

		case r.Method == http.MethodPost && path == "/repos/testowner/testrepo/labels":
			if writeCalled != nil {
				writeCalled.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{}`)

		case r.Method == http.MethodPatch && pathIsLabel(path):
			if writeCalled != nil {
				writeCalled.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)

		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"Not Found: %s %s"}`, r.Method, path) //nolint:gosec // test HTTP handler
		}
	}))
}

// pathIsLabel reports whether the path matches /repos/testowner/testrepo/labels/*.
func pathIsLabel(path string) bool {
	const prefix = "/repos/testowner/testrepo/labels/"
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}

func failingLabelsServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message":"Internal Server Error"}`)
	}))
}

func TestProcessLabelsNoLabels(t *testing.T) {
	cfg := &config.Config{}
	results, err := alter.ProcessLabels(cfg, alter.DryRun, nil, "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}
}

func TestProcessLabelsNoRepoContext(t *testing.T) {
	ghfake.FakeNoRepo(t)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	var results []alter.LabelResult
	var err error

	output := captureStderr(t, func() {
		results, err = alter.ProcessLabels(cfg, alter.DryRun, nil, "", "", false)
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil results, got %v", results)
	}

	want := "No GitHub repository context found."
	if output == "" || !containsSubstring(output, want) {
		t.Errorf("stderr = %q, want substring %q", output, want)
	}
}

func TestProcessLabelsWouldCreate(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.WouldCreate {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldCreate)
	}
	if results[0].Name != "bug" {
		t.Errorf("name = %q, want %q", results[0].Name, "bug")
	}
}

func TestProcessLabelsWouldUpdate(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "bug", Color: "fc5c65", Description: "Old description"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.WouldUpdate {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldUpdate)
	}
}

func TestProcessLabelsNoChange(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.LabelNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.LabelNoChange)
	}
}

func TestProcessLabelsCaseInsensitiveMatch(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "Bug", Color: "d73a4a", Description: "Something isn't working"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.WouldUpdate {
		t.Errorf("category = %q, want %q (casing differs)", results[0].Category, alter.WouldUpdate)
	}
}

func TestProcessLabelsApplyCallsAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.Apply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() == 0 {
		t.Error("expected API write call on Apply, but none received")
	}
}

func TestProcessLabelsRecutCallsAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.Recut, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() == 0 {
		t.Error("expected API write call on Recut, but none received")
	}
}

func TestProcessLabelsDryRunDoesNotCallAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() != 0 {
		t.Error("DryRun should not call API, but write was called")
	}
}

func TestProcessLabelsNoApplyWhenAllMatch(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		{Name: "enhancement", Color: "a2eeef", Description: "New feature"},
	}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
			{Name: "enhancement", Color: "a2eeef", Description: "New feature"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.Apply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() != 0 {
		t.Error("should not call API when all labels match, but write was called")
	}
}

func TestProcessLabelsErrorPropagated(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	server := failingLabelsServer()
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err == nil {
		t.Fatal("expected error from API failure, got nil")
	}
}

func TestProcessLabelsMixedResults(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		{Name: "enhancement", Color: "old123", Description: "Old description"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
			{Name: "enhancement", Color: "a2eeef", Description: "New feature"},
			{Name: "documentation", Color: "0075ca", Description: "Docs improvements"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	counts := map[alter.LabelCategory]int{}
	for _, r := range results {
		counts[r.Category]++
	}
	if counts[alter.LabelNoChange] != 1 {
		t.Errorf("LabelNoChange count = %d, want 1", counts[alter.LabelNoChange])
	}
	if counts[alter.WouldUpdate] != 1 {
		t.Errorf("WouldUpdate count = %d, want 1", counts[alter.WouldUpdate])
	}
	if counts[alter.WouldCreate] != 1 {
		t.Errorf("WouldCreate count = %d, want 1", counts[alter.WouldCreate])
	}
}

func TestProcessLabelsUpdateDescriptionOnly(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Old"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "New"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.WouldUpdate {
		t.Errorf("category = %q, want %q", results[0].Category, alter.WouldUpdate)
	}
}

func TestProcessLabelsColorCaseInsensitive(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	current := []config.LabelEntry{
		{Name: "bug", Color: "D73A4A", Description: "Something isn't working"},
	}
	server := labelsServer(current, nil)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.DryRun, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Category != alter.LabelNoChange {
		t.Errorf("category = %q, want %q", results[0].Category, alter.LabelNoChange)
	}
}

func TestProcessLabelsCasingOnlyApplyCallsAPI(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "Bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	_, err := alter.ProcessLabels(cfg, alter.Apply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() == 0 {
		t.Error("expected PATCH call for casing-only change, but none received")
	}
}

func TestProcessLabelsExactNameNoChange(t *testing.T) {
	ghfake.FakeRepo(t, "testowner", "testrepo")

	var writeCalled atomic.Int32
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
	}
	server := labelsServer(current, &writeCalled)
	t.Cleanup(server.Close)
	client := testutil.NewTestClient(t, server)

	cfg := &config.Config{
		Labels: []config.LabelEntry{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
		},
	}

	results, err := alter.ProcessLabels(cfg, alter.Apply, client, "testowner", "testrepo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writeCalled.Load() != 0 {
		t.Error("should not call API when name matches exactly, but write was called")
	}
	if len(results) != 1 || results[0].Category != alter.LabelNoChange {
		t.Errorf("expected LabelNoChange, got %v", results)
	}
}

// containsSubstring is a test helper for substring matching.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstringSearch(s, substr))
}

func containsSubstringSearch(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
