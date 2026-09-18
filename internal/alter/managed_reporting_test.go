package alter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestManagedReportingSeparatesPlannedAndConfirmedResults(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "flake.nix", []byte("custom flake\n"))
	loader := managedContent("just/loader.just", "current")
	writeManagedTestFile(t, dir, "just/loader.just", loader)
	writeManagedTestFile(t, dir, "nix/go.nix", managedContent("nix/go.nix", "old"))

	selections := []managedSelection{
		{Entry: managedRegistryEntry{Path: "flake.nix", Policy: managedPolicyRoot}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "just/loader.just", Policy: managedPolicyLoader}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo}},
		{Entry: managedRegistryEntry{Path: "nix/pages.nix", Policy: managedPolicyFragment, Capability: managedCapabilityPages}, Enabled: true},
	}
	plan, err := preflightManagedFiles(dir, selections, managedRenderedFiles{
		"flake.nix":        []byte("generated flake\n"),
		"just/loader.just": loader,
		"nix/pages.nix":    managedContent("nix/pages.nix", "new"),
	})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := managedPlannedResults(dir, plan)
	if err != nil {
		t.Fatal(err)
	}
	wantPlanned := map[string]SwatchCategory{
		"flake.nix": Skipped, "just/loader.just": NoChange,
		"nix/go.nix": WouldRemove, "nix/pages.nix": WouldCopy,
	}
	for _, result := range planned {
		if wantPlanned[result.Path] != result.Category {
			t.Errorf("planned %s category = %q, want %q", result.Path, result.Category, wantPlanned[result.Path])
		}
	}

	preview := buildReport("baste", "", nil, nil, nil, planned, DryRun)
	if preview.Document.Summary.Alterations != 2 || preview.Document.Summary.Preserved != 1 || preview.Document.Summary.Unchanged != 1 {
		t.Fatalf("preview summary = %#v", preview.Document.Summary)
	}
	for _, text := range []string{"would copy:", "nix/pages.nix", "would remove:", "nix/go.nix", "existing root preserved"} {
		if !strings.Contains(preview.Plain, text) {
			t.Errorf("preview plain report lacks %q: %s", text, preview.Plain)
		}
	}

	confirmed, err := managedConfirmedResults(planned, []managedApplyResult{{Path: "nix/pages.nix", Operation: managedOperationWrite}})
	if err != nil {
		t.Fatal(err)
	}
	partial := buildReport("alter", "", nil, nil, nil, confirmed, Apply)
	if strings.Contains(partial.Plain, "nix/go.nix") || strings.Contains(partial.Plain, "removed:") {
		t.Fatalf("partial report claimed an unconfirmed removal: %s", partial.Plain)
	}
	if !strings.Contains(partial.Plain, "copied:") || !strings.Contains(partial.Plain, "nix/pages.nix") {
		t.Fatalf("partial report omitted the confirmed copy: %s", partial.Plain)
	}
	if partial.Document.Summary.Applied != 1 || partial.Document.Summary.Preserved != 1 || partial.Document.Summary.Unchanged != 1 {
		t.Fatalf("partial summary = %#v", partial.Document.Summary)
	}
}

func TestManagedReportingAddsConditionalWarningsAndAdoptionGuidance(t *testing.T) {
	results := []SwatchResult{
		{Path: "flake.nix", Category: Skipped, Reason: SkipManagedRootExists},
		{Path: "nix/loader.nix", Category: WouldCopy},
	}
	report := buildReport("baste", "", nil, nil, nil, results, DryRun)
	appendManagedReporting(&report, results, []string{".mcp.json"})

	for _, text := range []string{
		"warning: review and add new Nix files to Git because Nix flakes exclude untracked files: `nix/loader.nix`",
		"warning: mcp.playwright is false, but retained shared settings can invoke the unavailable Playwright MCP executable: `.mcp.json`",
		"Connect the preserved `flake.nix` to `nix/loader.nix` manually.",
	} {
		if !strings.Contains(report.Plain, text) {
			t.Errorf("plain report lacks %q: %s", text, report.Plain)
		}
	}
	if strings.Contains(strings.ToLower(report.Plain), "index") {
		t.Fatalf("report claims Git index inspection: %s", report.Plain)
	}
	if len(report.Document.Notices) != 2 {
		t.Fatalf("structured notices = %#v", report.Document.Notices)
	}
	if !containsGuidance(report.Document.Guidance, "Connect the preserved `flake.nix` to `nix/loader.nix` manually.") {
		t.Fatalf("structured guidance = %#v", report.Document.Guidance)
	}
}

func TestManagedConflictIsAttentionInPlainAndStructuredReports(t *testing.T) {
	result := SwatchResult{Path: "just/loader.just", Category: ManagedConflict, Reason: ManagedNotOwned}
	report := buildReport("alter", "", nil, nil, nil, []SwatchResult{result}, Apply)
	if report.Plain != "conflict:                            just/loader.just (not owned by Tailor)\n" {
		t.Fatalf("plain conflict report = %q", report.Plain)
	}
	if len(report.Document.Items) != 1 || report.Document.Items[0].Outcome != output.Attention || report.Document.Items[0].Action != "resolve" {
		t.Fatalf("structured conflict item = %#v", report.Document.Items)
	}
}

func TestExecuteDoesNotActivateManagedFiles(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)

	report, err := Execute(&config.Config{}, dir, Apply, testutil.NewTestClient(t, server), io.Discard, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Plain != "" || len(report.Document.Items) != 0 || len(report.Document.Notices) != 0 {
		t.Fatalf("public execution reported inactive managed files: %#v", report)
	}
	for _, path := range []string{"just/loader.just", "nix/loader.nix", "just/tailor.just"} {
		if _, statErr := os.Lstat(filepath.Join(dir, path)); !os.IsNotExist(statErr) {
			t.Fatalf("public execution activated %q: %v", path, statErr)
		}
	}
}

func TestManagedExecutionPreflightsBeforeAuthenticationOrWrites(t *testing.T) {
	dir := t.TempDir()
	writeManagedTestFile(t, dir, "nix/loader.nix", []byte("user content\n"))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)

	report, err := execute(&config.Config{}, dir, Apply, testutil.NewTestClient(t, server), io.Discard, Options{}, managedTestRenderer)
	if err == nil || !strings.Contains(err.Error(), `managed destination "nix/loader.nix" is not owned by Tailor`) {
		t.Fatalf("execute() error = %v, want ownership conflict", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("API calls before managed preflight completed = %d", calls.Load())
	}
	if _, statErr := os.Stat(filepath.Join(dir, "just", "loader.just")); !os.IsNotExist(statErr) {
		t.Fatalf("managed write occurred before late conflict: %v", statErr)
	}
	if report.Document.Summary.Attention != 1 || !strings.Contains(report.Plain, "conflict:") {
		t.Fatalf("conflict report = %#v, plain = %q", report.Document, report.Plain)
	}
}

func TestManagedExecutionReportsConfirmedChangesBeforeApplyFailure(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		writeManagedTestFile(t, dir, "nix/loader.nix", []byte("user content\n"))
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)

	report, err := execute(&config.Config{}, dir, Apply, testutil.NewTestClient(t, server), io.Discard, Options{}, managedTestRenderer)
	if err == nil || !strings.Contains(err.Error(), `managed destination "nix/loader.nix" is not owned by Tailor`) {
		t.Fatalf("execute() error = %v, want ownership conflict", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "just", "loader.just")); statErr != nil {
		t.Fatalf("confirmed earlier write is missing: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "just", "tailor.just")); !os.IsNotExist(statErr) {
		t.Fatalf("later write ran after failure: %v", statErr)
	}
	var copied, conflict bool
	for _, item := range report.Document.Items {
		switch item.Name {
		case "just/loader.just":
			copied = item.Outcome == output.Applied && item.Action == "copy"
		case "nix/loader.nix":
			conflict = item.Outcome == output.Attention && item.Action == "resolve"
		case "just/tailor.just":
			t.Fatalf("partial report claimed an unconfirmed write: %#v", item)
		}
	}
	if !copied || !conflict || !strings.Contains(report.Plain, "copied:") || !strings.Contains(report.Plain, "conflict:") {
		t.Fatalf("partial report = %#v, plain = %q", report.Document.Items, report.Plain)
	}
}

func TestManagedExecutionExcludesManagedRootsFromOrdinarySwatches(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: "justfile", Alteration: swatch.FirstFit}}}
	renderer := func(selections []managedSelection) (managedRenderedFiles, error) {
		files, err := managedTestRenderer(selections)
		files["justfile"] = []byte("synthetic root\n")
		return files, err
	}

	report, err := execute(cfg, dir, Apply, testutil.NewTestClient(t, server), io.Discard, Options{}, renderer)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "justfile"))
	if err != nil || string(content) != "synthetic root\n" {
		t.Fatalf("managed root content = %q, error = %v", content, err)
	}
	count := 0
	for _, item := range report.Document.Items {
		if item.Name == "justfile" {
			count++
		}
	}
	if count != 1 || strings.Contains(report.Plain, "justfile (first-fit, exists)") {
		t.Fatalf("managed root was also processed as an ordinary swatch: %#v, plain = %q", report.Document.Items, report.Plain)
	}
}

func managedTestRenderer(selections []managedSelection) (managedRenderedFiles, error) {
	files := make(managedRenderedFiles)
	for _, selection := range selections {
		if !selection.Enabled {
			continue
		}
		if selection.Entry.Policy.marked() {
			files[selection.Entry.Path] = managedContent(selection.Entry.Path, "synthetic")
		} else {
			files[selection.Entry.Path] = []byte("synthetic\n")
		}
	}
	return files, nil
}

func containsGuidance(guidance []output.Guidance, text string) bool {
	for _, item := range guidance {
		if item.Text == text {
			return true
		}
	}
	return false
}
