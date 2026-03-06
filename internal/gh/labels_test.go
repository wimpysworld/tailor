package gh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestReadLabelsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "[]")
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	labels, err := ReadLabels(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadLabels() error: %v", err)
	}
	if labels == nil {
		t.Fatal("ReadLabels() returned nil, want empty slice")
	}
	if len(labels) != 0 {
		t.Errorf("ReadLabels() length = %d, want 0", len(labels))
	}
}

func TestReadLabelsSinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		labels := []labelResponse{
			{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
			{Name: "enhancement", Color: "a2eeef", Description: "New feature or request"},
		}
		body, _ := json.Marshal(labels)
		fmt.Fprint(w, string(body))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	labels, err := ReadLabels(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadLabels() error: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("ReadLabels() length = %d, want 2", len(labels))
	}
	if labels[0].Name != "bug" || labels[0].Color != "d73a4a" || labels[0].Description != "Something is not working" {
		t.Errorf("labels[0] = %+v", labels[0])
	}
	if labels[1].Name != "enhancement" || labels[1].Color != "a2eeef" || labels[1].Description != "New feature or request" {
		t.Errorf("labels[1] = %+v", labels[1])
	}
}

func TestReadLabelsPaginated(t *testing.T) {
	// Build two pages: page 1 has 100 labels, page 2 has 3 labels.
	page1 := make([]labelResponse, 100)
	for i := range page1 {
		page1[i] = labelResponse{
			Name:  fmt.Sprintf("label-%03d", i),
			Color: "aabbcc",
		}
	}
	page2 := []labelResponse{
		{Name: "label-100", Color: "aabbcc"},
		{Name: "label-101", Color: "aabbcc"},
		{Name: "label-102", Color: "aabbcc"},
	}

	page1JSON, _ := json.Marshal(page1)
	page2JSON, _ := json.Marshal(page2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageStr := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageStr)
		switch page {
		case 1, 0:
			w.Header().Set("Link", `<https://api.github.com/repos/testowner/testrepo/labels?per_page=100&page=2>; rel="next"`)
			fmt.Fprint(w, string(page1JSON))
		case 2:
			fmt.Fprint(w, string(page2JSON))
		default:
			fmt.Fprint(w, "[]")
		}
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	labels, err := ReadLabels(client, "testowner", "testrepo")
	if err != nil {
		t.Fatalf("ReadLabels() error: %v", err)
	}
	if len(labels) != 103 {
		t.Errorf("ReadLabels() length = %d, want 103", len(labels))
	}
}

func TestReadLabelsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message": "Not Found"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	_, err := ReadLabels(client, "testowner", "testrepo")
	if err == nil {
		t.Fatal("ReadLabels() expected error, got nil")
	}
}

func TestApplyLabelsCreatesMissing(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"name":"bug","color":"d73a4a","description":"Something is not working"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, nil)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/labels" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/labels", gotPath)
	}
	if gotBody["name"] != "bug" {
		t.Errorf("body name = %q, want %q", gotBody["name"], "bug")
	}
	if gotBody["color"] != "d73a4a" {
		t.Errorf("body color = %q, want %q", gotBody["color"], "d73a4a")
	}
	if gotBody["description"] != "Something is not working" {
		t.Errorf("body description = %q, want %q", gotBody["description"], "Something is not working")
	}
}

func TestApplyLabelsPatchesChanged(t *testing.T) {
	var gotMethod string
	var gotPath string
	var gotBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"name":"bug","color":"ff0000","description":"Updated desc"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "ff0000", Description: "Updated desc"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if gotPath != "/repos/testowner/testrepo/labels/bug" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/labels/bug", gotPath)
	}
	if gotBody["color"] != "ff0000" {
		t.Errorf("body color = %q, want %q", gotBody["color"], "ff0000")
	}
	if gotBody["description"] != "Updated desc" {
		t.Errorf("body description = %q, want %q", gotBody["description"], "Updated desc")
	}
}

func TestApplyLabelsSkipsMatched(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected 0 API calls for matched labels, got %d", requestCount)
	}
}

func TestApplyLabelsCaseInsensitiveMatch(t *testing.T) {
	var gotMethod string
	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"name":"Bug","color":"ff0000","description":"Updated"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "ff0000", Description: "Updated"},
	}
	current := []config.LabelEntry{
		{Name: "Bug", Color: "d73a4a", Description: "Old desc"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	// URL must use the current GitHub name, not the desired name.
	if gotPath != "/repos/testowner/testrepo/labels/Bug" {
		t.Errorf("path = %s, want /repos/testowner/testrepo/labels/Bug", gotPath)
	}
}

func TestApplyLabelsCaseInsensitiveSkip(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "Bug", Color: "d73a4a", Description: "Something is not working"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected 0 API calls for case-insensitive match, got %d", requestCount)
	}
}

func TestApplyLabelsColorCaseInsensitive(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "D73A4A", Description: "desc"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "desc"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected 0 API calls for colour match (case differs), got %d", requestCount)
	}
}

func TestApplyLabelsNoDeleteExtraLabels(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Something is not working"},
		{Name: "enhancement", Color: "a2eeef", Description: "New feature or request"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if requestCount != 0 {
		t.Errorf("expected 0 API calls (no prune), got %d", requestCount)
	}
}

func TestApplyLabelsMixedOperations(t *testing.T) {
	type call struct {
		method string
		path   string
	}
	var calls []call

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{method: r.Method, path: r.URL.Path})
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Same"},
		{Name: "enhancement", Color: "ff0000", Description: "Changed colour"},
		{Name: "new-label", Color: "00ff00", Description: "Brand new"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Same"},
		{Name: "enhancement", Color: "a2eeef", Description: "Changed colour"},
		{Name: "wontfix", Color: "ffffff", Description: "Will not fix"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}

	// bug: skip (no call), enhancement: PATCH, new-label: POST, wontfix: left alone
	if len(calls) != 2 {
		t.Fatalf("expected 2 API calls, got %d: %v", len(calls), calls)
	}
	if calls[0].method != http.MethodPatch || calls[0].path != "/repos/testowner/testrepo/labels/enhancement" {
		t.Errorf("call[0] = %s %s, want PATCH /repos/testowner/testrepo/labels/enhancement", calls[0].method, calls[0].path)
	}
	if calls[1].method != http.MethodPost || calls[1].path != "/repos/testowner/testrepo/labels" {
		t.Errorf("call[1] = %s %s, want POST /repos/testowner/testrepo/labels", calls[1].method, calls[1].path)
	}
}

func TestApplyLabelsCreateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		fmt.Fprint(w, `{"message": "Validation Failed"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "desc"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, nil)
	if err == nil {
		t.Fatal("ApplyLabels() expected error from POST, got nil")
	}
}

func TestApplyLabelsPatchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"message": "Internal Server Error"}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "ff0000", Description: "new"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "old"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err == nil {
		t.Fatal("ApplyLabels() expected error from PATCH, got nil")
	}
}

func TestHasNextPage(t *testing.T) {
	tests := []struct {
		link string
		want bool
	}{
		{`<https://api.github.com/repos/o/r/labels?page=2>; rel="next"`, true},
		{`<https://api.github.com/repos/o/r/labels?page=1>; rel="prev", <https://api.github.com/repos/o/r/labels?page=3>; rel="next"`, true},
		{`<https://api.github.com/repos/o/r/labels?page=1>; rel="prev"`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := hasNextPage(tt.link); got != tt.want {
			t.Errorf("hasNextPage(%q) = %v, want %v", tt.link, got, tt.want)
		}
	}
}

func TestApplyLabelsDescriptionOnlyChange(t *testing.T) {
	var gotMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{}`)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(t, server)
	desired := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Updated description"},
	}
	current := []config.LabelEntry{
		{Name: "bug", Color: "d73a4a", Description: "Old description"},
	}

	err := ApplyLabels(client, "testowner", "testrepo", desired, current)
	if err != nil {
		t.Fatalf("ApplyLabels() error: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH for description-only change", gotMethod)
	}
}
