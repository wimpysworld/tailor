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
		{Path: "justfile", Category: Skipped, Reason: SkipManagedRootExists},
		{Path: "flake.nix", Category: Skipped, Reason: SkipManagedRootExists},
		{Path: ".mcp.json", Category: Skipped, Reason: SkipManagedSharedExists},
		{Path: ".codex/config.toml", Category: Skipped, Reason: SkipManagedSharedExists},
		{Path: "opencode.json", Category: Skipped, Reason: SkipManagedSharedExists},
		{Path: ".pi/mcp.json", Category: Skipped, Reason: SkipManagedSharedExists},
		{Path: "nix/loader.nix", Category: WouldCopy},
	}
	report := buildReport("baste", "", nil, nil, nil, results, DryRun)
	appendManagedReporting(&report, &config.Config{}, results)

	for _, text := range []string{
		"warning: review and add new Nix files to Git because Nix flakes exclude untracked files: `nix/loader.nix`",
		"If absent, add `import 'just/loader.just'` to the preserved `justfile`.",
		"If absent, add `++ import ./nix/loader.nix { inherit pkgs; }` to the existing package list in the preserved `flake.nix`.",
		"If absent, add Tailor's `mcpServers.playwright` starter entry to the preserved `.mcp.json`.",
		"If absent, add Tailor's `[mcp_servers.playwright]` starter table to the preserved `.codex/config.toml`.",
		"If absent, add Tailor's `mcp.playwright` starter entry to the preserved `opencode.json`.",
		"If absent, add Tailor's `mcpServers.playwright` starter entry to the preserved `.pi/mcp.json`.",
	} {
		if !strings.Contains(report.Plain, text) {
			t.Errorf("plain report lacks %q: %s", text, report.Plain)
		}
	}
	if strings.Contains(strings.ToLower(report.Plain), "index") {
		t.Fatalf("report claims Git index inspection: %s", report.Plain)
	}
	if len(report.Document.Notices) != 1 {
		t.Fatalf("structured notices = %#v", report.Document.Notices)
	}
	for _, text := range []string{
		"If absent, add `import 'just/loader.just'` to the preserved `justfile`.",
		"If absent, add `++ import ./nix/loader.nix { inherit pkgs; }` to the existing package list in the preserved `flake.nix`.",
		"If absent, add Tailor's `mcpServers.playwright` starter entry to the preserved `.mcp.json`.",
		"If absent, add Tailor's `[mcp_servers.playwright]` starter table to the preserved `.codex/config.toml`.",
		"If absent, add Tailor's `mcp.playwright` starter entry to the preserved `opencode.json`.",
		"If absent, add Tailor's `mcpServers.playwright` starter entry to the preserved `.pi/mcp.json`.",
	} {
		if !containsGuidance(report.Document.Guidance, text) {
			t.Fatalf("structured guidance lacks %q: %#v", text, report.Document.Guidance)
		}
	}
}

func TestManagedReportingWarnsWhenPlaywrightIsExplicitlyDisabled(t *testing.T) {
	disabled := false
	cfg := &config.Config{MCP: &config.MCPSettings{Playwright: &disabled}}
	warning := "warning: mcp.playwright is false, so Tailor removes `nix/playwright.nix` but preserves MCP client settings. Disable or remove their Playwright servers to avoid a missing `playwright-mcp` executable"
	tests := []struct {
		name    string
		mode    ApplyMode
		results []SwatchResult
		warn    bool
	}{
		{name: "missing fragment", mode: DryRun, results: []SwatchResult{{Path: "nix/playwright.nix", Category: NoChange}}, warn: true},
		{name: "preview removal", mode: DryRun, results: []SwatchResult{{Path: "nix/playwright.nix", Category: WouldRemove}}, warn: true},
		{name: "confirmed removal", mode: Apply, results: []SwatchResult{{Path: "nix/playwright.nix", Category: WouldRemove}}, warn: true},
		{name: "empty partial report", mode: Apply},
		{name: "ownership conflict", mode: Apply, results: []SwatchResult{{Path: "nix/playwright.nix", Category: ManagedConflict}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := buildReport("alter", "", nil, nil, nil, tt.results, tt.mode)
			appendManagedReporting(&report, cfg, tt.results)

			if got := strings.Contains(report.Plain, warning); got != tt.warn {
				t.Fatalf("retained-settings warning present = %t, want %t: %s", got, tt.warn, report.Plain)
			}
			if tt.warn {
				if len(report.Document.Notices) != 1 || report.Document.Notices[0].Text != strings.TrimPrefix(warning, "warning: ") {
					t.Fatalf("structured notices = %#v", report.Document.Notices)
				}
			} else if len(report.Document.Notices) != 0 {
				t.Fatalf("structured notices = %#v, want none", report.Document.Notices)
			}
		})
	}

	qualifying := []SwatchResult{{Path: "nix/playwright.nix", Category: NoChange}}
	for _, cfg := range []*config.Config{{}, {MCP: &config.MCPSettings{Playwright: new(true)}}} {
		report := buildReport("baste", "", nil, nil, nil, qualifying, DryRun)
		appendManagedReporting(&report, cfg, qualifying)
		if strings.Contains(report.Plain, "missing `playwright-mcp` executable") {
			t.Fatalf("report warns without explicit false: %s", report.Plain)
		}
	}
}

func TestManagedExecutionOmitsDisabledPlaywrightWarningBeforeManagedStage(t *testing.T) {
	disabled := false
	cfg := &config.Config{
		MCP: &config.MCPSettings{Playwright: &disabled},
		Swatches: []config.SwatchEntry{
			{Path: ".gitignore", Alteration: swatch.FirstFit},
			{Path: ".gitignore", Alteration: swatch.FirstFit},
		},
	}
	renderer := func([]managedSelection) (managedRenderedFiles, error) {
		t.Fatal("managed stage ran after configuration failure")
		return nil, nil
	}

	report, err := execute(cfg, t.TempDir(), Apply, nil, io.Discard, Options{}, renderer)
	if err == nil || !strings.Contains(err.Error(), "duplicate swatch path") {
		t.Fatalf("execute() error = %v, want duplicate swatch path", err)
	}
	if strings.Contains(report.Plain, "missing `playwright-mcp` executable") || len(report.Document.Notices) != 0 {
		t.Fatalf("early partial report claims Playwright removal: %#v, %s", report.Document.Notices, report.Plain)
	}
}

func TestManagedExclusionsProtectGenericEntries(t *testing.T) {
	for _, mode := range []ApplyMode{Apply, Recut} {
		for _, path := range []string{"justfile", "flake.nix", ".mcp.json"} {
			t.Run(managedModeName(mode)+"/"+path, func(t *testing.T) {
				dir := t.TempDir()
				content := []byte("custom settings\n")
				writeManagedTestFile(t, dir, path, content)
				cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: path, Alteration: swatch.Always}}}

				results, err := processSwatches(cfg, dir, mode, &TokenContext{}, managedExcludedPaths())
				if err != nil {
					t.Fatal(err)
				}
				if len(results) != 0 {
					t.Fatalf("ordinary swatch results = %#v, want none", results)
				}
				assertManagedBytes(t, filepath.Join(dir, path), content)
			})
		}
	}
}

func TestManagedExecutionReportsUnselectedExistingRootsWithoutBootstrappingAbsentRoots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)

	modes := []struct {
		name string
		mode ApplyMode
	}{
		{name: "dry-run", mode: DryRun},
		{name: "apply", mode: Apply},
		{name: "recut", mode: Recut},
	}
	for _, mode := range modes {
		t.Run(mode.name+"/existing", func(t *testing.T) {
			dir := t.TempDir()
			contents := map[string][]byte{
				"justfile":  []byte("custom just root\n"),
				"flake.nix": []byte("custom Nix root\n"),
			}
			for path, content := range contents {
				writeManagedTestFile(t, dir, path, content)
			}
			cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: "justfile", Alteration: swatch.Never}}}

			report, err := execute(cfg, dir, mode.mode, testutil.NewTestClient(t, server), io.Discard, Options{}, managedTestRenderer)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"justfile", "flake.nix"} {
				assertManagedBytes(t, filepath.Join(dir, path), contents[path])
				found := false
				for _, item := range report.Document.Items {
					if item.Name == path && item.Outcome == output.Preserved {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("report does not preserve unselected root %q: %#v", path, report.Document.Items)
				}
			}
			for _, guidance := range []string{
				"If absent, add `import 'just/loader.just'` to the preserved `justfile`.",
				"If absent, add `++ import ./nix/loader.nix { inherit pkgs; }` to the existing package list in the preserved `flake.nix`.",
			} {
				if !containsGuidance(report.Document.Guidance, guidance) {
					t.Errorf("report guidance lacks %q: %#v", guidance, report.Document.Guidance)
				}
			}
		})

		t.Run(mode.name+"/absent", func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config.Config{Swatches: []config.SwatchEntry{{Path: "justfile", Alteration: swatch.Never}}}

			report, err := execute(cfg, dir, mode.mode, testutil.NewTestClient(t, server), io.Discard, Options{}, managedTestRenderer)
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"justfile", "flake.nix"} {
				if _, err := os.Lstat(filepath.Join(dir, path)); !os.IsNotExist(err) {
					t.Errorf("unselected root %q exists or cannot be checked: %v", path, err)
				}
				for _, item := range report.Document.Items {
					if item.Name == path {
						t.Errorf("report includes absent unselected root %q: %#v", path, item)
					}
				}
			}
			if strings.Contains(report.Plain, "preserved `justfile`") || strings.Contains(report.Plain, "preserved `flake.nix`") {
				t.Errorf("report gives adoption guidance for absent roots: %s", report.Plain)
			}
		})
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

func TestExecuteProvisionsManagedCoreFiles(t *testing.T) {
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
	for _, path := range []string{"just/loader.just", "nix/loader.nix", "just/tailor.just"} {
		content, readErr := os.ReadFile(filepath.Join(dir, path))
		if readErr != nil {
			t.Fatalf("public execution did not provision %q: %v", path, readErr)
		}
		if !hasManagedMarker(content, path) {
			t.Errorf("public execution provisioned %q without its ownership marker", path)
		}
		if !strings.Contains(report.Plain, path) {
			t.Errorf("public execution report omits %q: %s", path, report.Plain)
		}
	}
	if strings.Contains(strings.ToLower(report.Plain), "mcp") || strings.Contains(strings.ToLower(report.Plain), "playwright") {
		t.Fatalf("public execution reported reserved managed files: %s", report.Plain)
	}
}

func TestManagedExecutionRejectsDuplicateRecipesBeforeAuthenticationOrWrites(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)
	renderer := func(selections []managedSelection) (managedRenderedFiles, error) {
		files, err := managedTestRenderer(selections)
		files["just/loader.just"] = managedContent("just/loader.just", "duplicate:")
		files["just/tailor.just"] = managedContent("just/tailor.just", "duplicate:")
		return files, err
	}

	_, err := execute(&config.Config{}, dir, Apply, testutil.NewTestClient(t, server), io.Discard, Options{}, renderer)
	if err == nil || !strings.Contains(err.Error(), `duplicate recipe "duplicate"`) {
		t.Fatalf("execute() error = %v, want duplicate recipe error", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("API calls before recipe validation completed = %d", calls.Load())
	}
	if entries, readErr := os.ReadDir(dir); readErr != nil || len(entries) != 0 {
		t.Fatalf("duplicate recipe preflight changed project: %v, %v", entries, readErr)
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

func TestManagedExecutionExcludesManagedPathsFromOrdinarySwatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)

	for _, mode := range []ApplyMode{Apply, Recut} {
		t.Run(managedModeName(mode), func(t *testing.T) {
			dir := t.TempDir()
			playwright := true
			cfg := &config.Config{
				License: "none",
				MCP:     &config.MCPSettings{Playwright: &playwright},
				Swatches: []config.SwatchEntry{
					{Path: "justfile", Alteration: swatch.FirstFit},
					{Path: "flake.nix", Alteration: swatch.Always},
				},
			}

			report, err := Execute(cfg, dir, mode, testutil.NewTestClient(t, server), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"justfile", "flake.nix", ".mcp.json"} {
				want, err := swatch.Content(path)
				if err != nil {
					t.Fatal(err)
				}
				assertManagedBytes(t, filepath.Join(dir, path), want)

				count := 0
				for _, item := range report.Document.Items {
					if item.Name == path {
						count++
					}
				}
				if count != 1 {
					t.Fatalf("managed path %q appeared %d times: %#v", path, count, report.Document.Items)
				}
			}
		})
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
