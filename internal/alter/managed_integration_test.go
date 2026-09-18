package alter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/output"
	"github.com/wimpysworld/tailor/internal/swatch"
	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestManagedLifecycleStatesConvergeAcrossModes(t *testing.T) {
	states := []string{"absent", "false", "true"}
	modes := []struct {
		name string
		mode ApplyMode
	}{
		{name: "dry-run", mode: DryRun},
		{name: "apply", mode: Apply},
		{name: "recut", mode: Recut},
	}

	for _, mode := range modes {
		for _, state := range states {
			t.Run(mode.name+"/"+state, func(t *testing.T) {
				dir := t.TempDir()
				cfg := managedLifecycleConfig(state)
				seedManagedLifecycleState(t, dir, state)
				initial := snapshotManagedTree(t, dir)
				client := managedLifecycleClient(t)

				for run := 1; run <= 2; run++ {
					before := snapshotManagedTree(t, dir)
					report, err := execute(cfg, dir, mode.mode, client, io.Discard, Options{}, managedTestRenderer)
					if err != nil {
						t.Fatalf("run %d: execute() error = %v", run, err)
					}
					assertManagedLifecycleReport(t, report, state, mode.mode, run)
					assertManagedLifecycleFiles(t, dir, state, mode.mode)
					if mode.mode == DryRun {
						if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
							t.Fatalf("run %d changed files during dry-run: before=%v after=%v", run, before, after)
						}
					}
				}

				if mode.mode == DryRun {
					if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, initial) {
						t.Fatalf("dry-run changed lifecycle fixture: before=%v after=%v", initial, after)
					}
				}
			})
		}
	}
}

func managedLifecycleConfig(state string) *config.Config {
	cfg := &config.Config{
		License: "none",
		Swatches: []config.SwatchEntry{
			{Path: "justfile", Alteration: swatch.FirstFit},
			{Path: "flake.nix", Alteration: swatch.Always},
		},
	}
	if state == "absent" {
		return cfg
	}
	enabled := state == "true"
	cfg.Languages = &config.LanguageSettings{Go: &enabled}
	cfg.Pages = &model.PagesSettings{Enabled: &enabled}
	cfg.MCP = &config.MCPSettings{Playwright: &enabled}
	return cfg
}

func seedManagedLifecycleState(t *testing.T, dir, state string) {
	t.Helper()
	writeManagedTestFile(t, dir, "unrelated.txt", []byte("keep\n"))
	if state == "true" {
		return
	}
	for _, destination := range managedCapabilityFragments() {
		content := []byte("user content\n")
		if state == "false" {
			content = managedContent(destination, "old")
		}
		writeManagedTestFile(t, dir, destination, content)
	}
	for _, destination := range managedSharedStarters() {
		writeManagedTestFile(t, dir, destination, []byte("custom shared settings\n"))
	}
}

func assertManagedLifecycleReport(t *testing.T, report Report, state string, mode ApplyMode, run int) {
	t.Helper()
	wantChanges := map[string]int{"absent": 5, "false": 10, "true": 14}[state]
	if mode.ShouldWrite() && run == 2 {
		wantChanges = 0
	}
	gotChanges := report.Document.Summary.Alterations
	if mode.ShouldWrite() {
		gotChanges = report.Document.Summary.Applied
	}
	if gotChanges != wantChanges {
		t.Fatalf("run %d summary changes = %d, want %d: %#v", run, gotChanges, wantChanges, report.Document.Summary)
	}

	active := make(map[string]bool)
	for _, item := range report.Document.Items {
		if item.Domain == "Files" {
			active[item.Name] = true
		}
		if mode.ShouldWrite() && run == 2 && item.Outcome == output.Applied {
			t.Fatalf("run 2 reported another managed change: %#v", item)
		}
	}
	for _, destination := range managedCapabilityFragments() {
		want := state != "absent"
		if active[destination] != want {
			t.Errorf("run %d report presence for %q = %t, want %t", run, destination, active[destination], want)
		}
	}
	for _, destination := range managedSharedStarters() {
		want := state == "true"
		if active[destination] != want {
			t.Errorf("run %d report presence for %q = %t, want %t", run, destination, active[destination], want)
		}
	}
}

func assertManagedLifecycleFiles(t *testing.T, dir, state string, mode ApplyMode) {
	t.Helper()
	if content, err := os.ReadFile(filepath.Join(dir, "unrelated.txt")); err != nil || string(content) != "keep\n" {
		t.Fatalf("unrelated file changed: content=%q error=%v", content, err)
	}
	if !mode.ShouldWrite() {
		return
	}
	for _, destination := range []string{"just/loader.just", "nix/loader.nix", "just/tailor.just"} {
		content, err := os.ReadFile(filepath.Join(dir, destination))
		if err != nil || !hasManagedMarker(content, destination) {
			t.Errorf("always-reconciled file %q content=%q error=%v", destination, content, err)
		}
	}
	for _, destination := range []string{"justfile", "flake.nix"} {
		if content, err := os.ReadFile(filepath.Join(dir, destination)); err != nil || string(content) != "synthetic\n" {
			t.Errorf("managed root %q content=%q error=%v", destination, content, err)
		}
	}
	for _, destination := range managedCapabilityFragments() {
		content, err := os.ReadFile(filepath.Join(dir, destination))
		switch state {
		case "true":
			if err != nil || !hasManagedMarker(content, destination) {
				t.Errorf("enabled fragment %q content=%q error=%v", destination, content, err)
			}
		case "false":
			if !os.IsNotExist(err) {
				t.Errorf("disabled fragment %q still exists: %v", destination, err)
			}
		case "absent":
			if err != nil || string(content) != "user content\n" {
				t.Errorf("absent fragment %q changed: content=%q error=%v", destination, content, err)
			}
		}
	}
	for _, destination := range managedSharedStarters() {
		content, err := os.ReadFile(filepath.Join(dir, destination))
		want := "custom shared settings\n"
		if state == "true" {
			want = "synthetic\n"
		}
		if err != nil || string(content) != want {
			t.Errorf("shared starter %q content=%q error=%v, want %q", destination, content, err, want)
		}
	}
}

func managedLifecycleClient(t *testing.T) *api.RESTClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, `{"login":"tailor"}`)
	}))
	t.Cleanup(server.Close)
	return testutil.NewTestClient(t, server)
}

func managedCapabilityFragments() []string {
	return []string{"just/go.just", "nix/go.nix", "just/pages.just", "nix/pages.nix", "nix/playwright.nix"}
}

func managedSharedStarters() []string {
	return []string{".mcp.json", ".codex/config.toml", "opencode.json", ".pi/mcp.json"}
}
