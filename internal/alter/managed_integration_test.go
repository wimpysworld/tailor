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

func TestManagedProductionTransitionsAcrossModes(t *testing.T) {
	transitions := []struct {
		name        string
		states      [2]string
		initialCaps bool
		wantCaps    [2]bool
	}{
		{name: "absent-to-true", states: [2]string{"absent", "true"}, wantCaps: [2]bool{false, true}},
		{name: "true-to-absent", states: [2]string{"true", "absent"}, initialCaps: true, wantCaps: [2]bool{true, true}},
		{name: "true-to-false", states: [2]string{"true", "false"}, initialCaps: true, wantCaps: [2]bool{true, false}},
		{name: "false-to-true", states: [2]string{"false", "true"}, initialCaps: true, wantCaps: [2]bool{false, true}},
	}
	modes := []struct {
		name string
		mode ApplyMode
	}{
		{name: "dry-run", mode: DryRun},
		{name: "apply", mode: Apply},
		{name: "recut", mode: Recut},
	}

	for _, mode := range modes {
		for _, transition := range transitions {
			t.Run(mode.name+"/"+transition.name, func(t *testing.T) {
				dir := t.TempDir()
				writeManagedTestFile(t, dir, "unrelated.txt", []byte("keep\n"))
				if transition.initialCaps {
					seedOwnedCapabilityFragments(t, dir)
				}
				initial := snapshotManagedTree(t, dir)
				client := managedProductionClient(t)

				for run, state := range transition.states {
					before := snapshotManagedTree(t, dir)
					report, err := Execute(managedProductionConfig(state), dir, mode.mode, client, io.Discard, Options{})
					if err != nil {
						t.Fatalf("run %d (%s): Execute() error = %v", run+1, state, err)
					}
					assertManagedProductionReport(t, report, state)
					if mode.mode == DryRun {
						if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, before) {
							t.Fatalf("run %d changed files during dry-run: before=%v after=%v", run+1, before, after)
						}
						assertCapabilityFragments(t, dir, transition.initialCaps)
						continue
					}
					assertManagedCoreFiles(t, dir)
					assertCapabilityFragments(t, dir, transition.wantCaps[run])
					assertManagedBytes(t, filepath.Join(dir, "unrelated.txt"), []byte("keep\n"))
				}

				if mode.mode == DryRun {
					if after := snapshotManagedTree(t, dir); !reflect.DeepEqual(after, initial) {
						t.Fatalf("dry-run changed transition fixture: before=%v after=%v", initial, after)
					}
				}
			})
		}
	}
}

func TestManagedProductionRootModesAndPreservation(t *testing.T) {
	modes := []struct {
		name string
		mode ApplyMode
	}{
		{name: "dry-run", mode: DryRun},
		{name: "apply", mode: Apply},
		{name: "recut", mode: Recut},
	}

	for _, mode := range modes {
		t.Run(mode.name+"/existing-roots", func(t *testing.T) {
			dir := t.TempDir()
			justRoot := []byte("# custom just root\n")
			nixRoot := []byte("# custom Nix root\n")
			writeManagedTestFile(t, dir, "justfile", justRoot)
			writeManagedTestFile(t, dir, "flake.nix", nixRoot)
			cfg := managedRootConfig(swatch.Always, swatch.FirstFit)

			report, err := Execute(cfg, dir, mode.mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertManagedBytes(t, filepath.Join(dir, "justfile"), justRoot)
			assertManagedBytes(t, filepath.Join(dir, "flake.nix"), nixRoot)
			assertRootReportOutcome(t, report, "justfile", output.Preserved)
			assertRootReportOutcome(t, report, "flake.nix", output.Preserved)
		})

		t.Run(mode.name+"/never-and-omitted", func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config.Config{
				License:  "none",
				Swatches: []config.SwatchEntry{{Path: "justfile", Alteration: swatch.Never}},
			}
			report, err := Execute(cfg, dir, mode.mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertManagedMissing(t, filepath.Join(dir, "justfile"))
			assertManagedMissing(t, filepath.Join(dir, "flake.nix"))
			assertRootReportOutcome(t, report, "justfile", "")
			assertRootReportOutcome(t, report, "flake.nix", "")
		})

		t.Run(mode.name+"/bootstrap", func(t *testing.T) {
			dir := t.TempDir()
			cfg := managedRootConfig(swatch.FirstFit, swatch.Always)
			report, err := Execute(cfg, dir, mode.mode, managedProductionClient(t), io.Discard, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, root := range []string{"justfile", "flake.nix"} {
				if mode.mode == DryRun {
					assertManagedMissing(t, filepath.Join(dir, root))
					assertRootReportOutcome(t, report, root, output.Alteration)
					continue
				}
				want, err := swatch.Content(root)
				if err != nil {
					t.Fatal(err)
				}
				assertManagedBytes(t, filepath.Join(dir, root), want)
				assertRootReportOutcome(t, report, root, output.Applied)
			}
		})
	}
}

func TestManagedProductionPreservesRootSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-justfile")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	managedSymlinkOrSkip(t, outside, filepath.Join(dir, "justfile"))

	report, err := Execute(managedRootConfig(swatch.Always, swatch.Never), dir, Recut, managedProductionClient(t), io.Discard, Options{})
	if err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(dir, "justfile"))
	if err != nil || target != outside {
		t.Fatalf("justfile symlink target = %q, error = %v, want %q", target, err, outside)
	}
	assertManagedBytes(t, outside, []byte("outside\n"))
	assertRootReportOutcome(t, report, "justfile", output.Preserved)
}

func managedProductionConfig(state string) *config.Config {
	cfg := &config.Config{License: "none"}
	if state == "absent" {
		return cfg
	}
	enabled := state == "true"
	cfg.Languages = &config.LanguageSettings{Go: &enabled}
	cfg.Pages = &model.PagesSettings{Enabled: &enabled}
	return cfg
}

func managedLifecycleConfig(state string) *config.Config {
	cfg := managedProductionConfig(state)
	if state != "absent" {
		enabled := state == "true"
		cfg.MCP = &config.MCPSettings{Playwright: &enabled}
	}
	cfg.Swatches = []config.SwatchEntry{
		{Path: "justfile", Alteration: swatch.FirstFit},
		{Path: "flake.nix", Alteration: swatch.Always},
	}
	return cfg
}

func managedRootConfig(justMode, nixMode swatch.AlterationMode) *config.Config {
	return &config.Config{
		License: "none",
		Swatches: []config.SwatchEntry{
			{Path: "justfile", Alteration: justMode},
			{Path: "flake.nix", Alteration: nixMode},
		},
	}
}

func seedOwnedCapabilityFragments(t *testing.T, dir string) {
	t.Helper()
	for _, destination := range managedAvailableCapabilityFragments() {
		writeManagedTestFile(t, dir, destination, managedContent(destination, "old"))
	}
}

func assertManagedProductionReport(t *testing.T, report Report, state string) {
	t.Helper()
	items := make(map[string]output.Outcome)
	for _, item := range report.Document.Items {
		if item.Domain != "Files" {
			continue
		}
		items[item.Name] = item.Outcome
		if item.Name == "nix/playwright.nix" || item.Name == ".mcp.json" || item.Name == ".codex/config.toml" || item.Name == "opencode.json" || item.Name == ".pi/mcp.json" {
			t.Errorf("production report contains reserved destination %q", item.Name)
		}
	}
	for _, destination := range []string{"just/loader.just", "just/tailor.just", "nix/loader.nix"} {
		if _, exists := items[destination]; !exists {
			t.Errorf("production report lacks managed core destination %q", destination)
		}
	}
	for _, destination := range managedAvailableCapabilityFragments() {
		_, exists := items[destination]
		if want := state != "absent"; exists != want {
			t.Errorf("report presence for %q = %t, want %t in state %q", destination, exists, want, state)
		}
	}
}

func assertManagedCoreFiles(t *testing.T, dir string) {
	t.Helper()
	for _, destination := range []string{"just/loader.just", "just/tailor.just", "nix/loader.nix"} {
		content, err := os.ReadFile(filepath.Join(dir, destination))
		if err != nil || !hasManagedMarker(content, destination) {
			t.Errorf("managed core file %q content=%q error=%v", destination, content, err)
		}
	}
}

func assertCapabilityFragments(t *testing.T, dir string, wantPresent bool) {
	t.Helper()
	for _, destination := range managedAvailableCapabilityFragments() {
		content, err := os.ReadFile(filepath.Join(dir, destination))
		if !wantPresent {
			if !os.IsNotExist(err) {
				t.Errorf("capability fragment %q exists: content=%q error=%v", destination, content, err)
			}
			continue
		}
		if err != nil || !hasManagedMarker(content, destination) {
			t.Errorf("capability fragment %q content=%q error=%v", destination, content, err)
		}
	}
}

func assertRootReportOutcome(t *testing.T, report Report, name string, want output.Outcome) {
	t.Helper()
	for _, item := range report.Document.Items {
		if item.Domain == "Files" && item.Name == name {
			if item.Outcome != want {
				t.Errorf("root %q outcome = %q, want %q", name, item.Outcome, want)
			}
			return
		}
	}
	if want != "" {
		t.Errorf("report lacks root %q with outcome %q", name, want)
	}
}

func managedAvailableCapabilityFragments() []string {
	return []string{"just/go.just", "nix/go.nix", "just/pages.just", "nix/pages.nix"}
}

func managedProductionClient(t *testing.T) *api.RESTClient {
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
