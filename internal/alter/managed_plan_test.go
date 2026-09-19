package alter

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
	"github.com/wimpysworld/tailor/internal/swatch"
)

func TestFixedManagedRegistry(t *testing.T) {
	registry := fixedManagedRegistry()
	if err := validateManagedRegistry(registry); err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(registry))
	for _, entry := range registry {
		got = append(got, entry.Path)
	}
	want := []string{
		"justfile", "flake.nix",
		"just/loader.just", "nix/loader.nix", "just/tailor.just",
		"just/go.just", "nix/go.nix",
		"just/pages.just", "nix/pages.nix", "nix/playwright.nix",
		".mcp.json", ".codex/config.toml", "opencode.json", ".pi/mcp.json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fixedManagedRegistry() paths = %v, want %v", got, want)
	}
	for _, entry := range registry {
		wantLintRecipe := ""
		if entry.Path == "just/go.just" {
			wantLintRecipe = "lint-go"
		}
		if entry.LintRecipe != wantLintRecipe {
			t.Errorf("fixedManagedRegistry() lint recipe for %q = %q, want %q", entry.Path, entry.LintRecipe, wantLintRecipe)
		}
	}

	registry[0].Path = "changed"
	if fixedManagedRegistry()[0].Path != "justfile" {
		t.Fatal("fixedManagedRegistry() returned mutable shared state")
	}
}

func TestValidateManagedRegistryRejectsMalformedMetadata(t *testing.T) {
	tests := []struct {
		name     string
		registry []managedRegistryEntry
		want     string
	}{
		{
			name:     "unsafe absolute path",
			registry: []managedRegistryEntry{{Path: "/tmp/file", Policy: managedPolicyRoot}},
			want:     "unsafe managed destination",
		},
		{
			name:     "unsafe traversal",
			registry: []managedRegistryEntry{{Path: "../file", Policy: managedPolicyRoot}},
			want:     "unsafe managed destination",
		},
		{
			name: "duplicate destination",
			registry: []managedRegistryEntry{
				{Path: "same", Policy: managedPolicyRoot},
				{Path: "same", Policy: managedPolicyCore},
			},
			want: "duplicate destination",
		},
		{
			name:     "root with capability",
			registry: []managedRegistryEntry{{Path: "root", Policy: managedPolicyRoot, Capability: managedCapabilityGo}},
			want:     "invalid capability",
		},
		{
			name:     "fragment without capability",
			registry: []managedRegistryEntry{{Path: "fragment", Policy: managedPolicyFragment}},
			want:     "requires a capability",
		},
		{
			name:     "starter with wrong capability",
			registry: []managedRegistryEntry{{Path: "starter", Policy: managedPolicySharedStarter, Capability: managedCapabilityPages}},
			want:     "requires the playwright capability",
		},
		{
			name:     "invalid lint recipe identifier",
			registry: []managedRegistryEntry{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintRecipe: "lint go"}},
			want:     "invalid lint recipe",
		},
		{
			name: "duplicate lint recipe",
			registry: []managedRegistryEntry{
				{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintRecipe: "lint-ecosystem"},
				{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages, LintRecipe: "lint-ecosystem"},
			},
			want: "lint recipe \"lint-ecosystem\" is duplicated",
		},
		{
			name: "multiple lint recipes for capability",
			registry: []managedRegistryEntry{
				{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintRecipe: "lint-go"},
				{Path: "just/go-extra.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintRecipe: "lint-go-extra"},
			},
			want: "multiple lint recipes",
		},
		{
			name:     "lint recipe outside Just fragment",
			registry: []managedRegistryEntry{{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo, LintRecipe: "lint-go"}},
			want:     "must belong to a Just capability fragment",
		},
		{
			name:     "unknown policy",
			registry: []managedRegistryEntry{{Path: "file", Policy: managedPolicy(99)}},
			want:     "invalid policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateManagedRegistry(tt.registry)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateManagedRegistry() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSelectManagedFilesHonoursDeclarationsAndRootModes(t *testing.T) {
	trueValue, falseValue := true, false
	tests := []struct {
		name string
		cfg  *config.Config
		want map[string]bool
	}{
		{
			name: "absent capabilities and root entries",
			cfg:  &config.Config{},
			want: map[string]bool{
				"just/loader.just": true,
				"nix/loader.nix":   true,
				"just/tailor.just": true,
			},
		},
		{
			name: "never and missing roots prohibit bootstrap",
			cfg: &config.Config{Swatches: []config.SwatchEntry{
				{Path: "justfile", Alteration: swatch.Never},
			}},
			want: map[string]bool{
				"just/loader.just": true,
				"nix/loader.nix":   true,
				"just/tailor.just": true,
			},
		},
		{
			name: "false capabilities select fragments for removal",
			cfg: &config.Config{
				Languages: &config.LanguageSettings{Go: &falseValue},
				Pages:     &model.PagesSettings{Enabled: &falseValue},
				MCP:       &config.MCPSettings{Playwright: &falseValue},
			},
			want: map[string]bool{
				"just/loader.just":   true,
				"nix/loader.nix":     true,
				"just/tailor.just":   true,
				"just/go.just":       false,
				"nix/go.nix":         false,
				"just/pages.just":    false,
				"nix/pages.nix":      false,
				"nix/playwright.nix": false,
			},
		},
		{
			name: "true capabilities and permitted roots select all files",
			cfg: &config.Config{
				Languages: &config.LanguageSettings{Go: &trueValue},
				Pages:     &model.PagesSettings{Enabled: &trueValue},
				MCP:       &config.MCPSettings{Playwright: &trueValue},
				Swatches: []config.SwatchEntry{
					{Path: "justfile", Alteration: swatch.FirstFit},
					{Path: "flake.nix", Alteration: swatch.Always},
				},
			},
			want: map[string]bool{
				"justfile": true, "flake.nix": true,
				"just/loader.just": true, "nix/loader.nix": true, "just/tailor.just": true,
				"just/go.just": true, "nix/go.nix": true,
				"just/pages.just": true, "nix/pages.nix": true,
				"nix/playwright.nix": true,
				".mcp.json":          true, ".codex/config.toml": true, "opencode.json": true, ".pi/mcp.json": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := selectManagedFiles(tt.cfg)
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]bool, len(selected))
			for _, selection := range selected {
				got[selection.Entry.Path] = selection.Enabled
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("selectManagedFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectManagedFilesRejectsAmbiguousRootBootstrap(t *testing.T) {
	cfg := &config.Config{Swatches: []config.SwatchEntry{
		{Path: "justfile", Alteration: swatch.FirstFit},
		{Path: "justfile", Alteration: swatch.Always},
	}}
	_, err := selectManagedFiles(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate root bootstrap entry") {
		t.Fatalf("selectManagedFiles() error = %v, want duplicate root error", err)
	}
}

func TestPlanManagedProtectedFiles(t *testing.T) {
	for _, policy := range []managedPolicy{managedPolicyRoot, managedPolicySharedStarter} {
		for _, tt := range []struct {
			name string
			kind managedDestinationKind
			want managedOperation
		}{
			{name: "missing creates", kind: managedDestinationMissing, want: managedOperationWrite},
			{name: "regular preserves", kind: managedDestinationRegular, want: managedOperationPreserve},
			{name: "final symlink preserves", kind: managedDestinationSymlink, want: managedOperationPreserve},
		} {
			name := fmt.Sprintf("policy=%d/%s", policy, tt.name)
			t.Run(name, func(t *testing.T) {
				selection := managedSelection{Entry: managedRegistryEntry{Path: "protected", Policy: policy}, Enabled: true}
				if policy == managedPolicySharedStarter {
					selection.Entry.Capability = managedCapabilityPlaywright
				}
				plan, err := planManagedFiles(
					[]managedSelection{selection},
					managedRenderedFiles{"protected": []byte("unmarked content")},
					[]managedDestinationSnapshot{{Path: "protected", Kind: tt.kind, Content: []byte("custom content")}},
				)
				if err != nil {
					t.Fatal(err)
				}
				if got := plan.Files[0].Operation; got != tt.want {
					t.Fatalf("operation = %d, want %d", got, tt.want)
				}
			})
		}
	}
}

func TestPlanManagedEnabledOwnedFiles(t *testing.T) {
	selection := managedSelection{Entry: managedRegistryEntry{Path: "just/loader.just", Policy: managedPolicyLoader}, Enabled: true}
	desired := managedContent("just/loader.just", "new")
	tests := []struct {
		name         string
		snapshot     managedDestinationSnapshot
		want         managedOperation
		wantOwnerErr bool
	}{
		{name: "missing", snapshot: managedDestinationSnapshot{Kind: managedDestinationMissing}, want: managedOperationWrite},
		{name: "changed marked file", snapshot: managedDestinationSnapshot{Kind: managedDestinationRegular, Content: managedContent("just/loader.just", "old")}, want: managedOperationWrite},
		{name: "equal marked file", snapshot: managedDestinationSnapshot{Kind: managedDestinationRegular, Content: desired}, want: managedOperationNoop},
		{name: "final symlink", snapshot: managedDestinationSnapshot{Kind: managedDestinationSymlink}, want: managedOperationWrite},
		{name: "unmarked regular file", snapshot: managedDestinationSnapshot{Kind: managedDestinationRegular, Content: []byte("custom")}, wantOwnerErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.snapshot.Path = selection.Entry.Path
			plan, err := planManagedFiles(
				[]managedSelection{selection},
				managedRenderedFiles{selection.Entry.Path: desired},
				[]managedDestinationSnapshot{tt.snapshot},
			)
			if tt.wantOwnerErr {
				if _, ok := errors.AsType[*managedOwnershipError](err); !ok {
					t.Fatalf("planManagedFiles() error = %v, want managedOwnershipError", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.Files[0].Operation; got != tt.want {
				t.Fatalf("operation = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlanManagedDisabledFragments(t *testing.T) {
	selection := managedSelection{
		Entry: managedRegistryEntry{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo},
	}
	tests := []struct {
		name         string
		snapshot     managedDestinationSnapshot
		want         managedOperation
		wantOwnerErr bool
	}{
		{name: "missing is a no-op", snapshot: managedDestinationSnapshot{Kind: managedDestinationMissing}, want: managedOperationNoop},
		{name: "marked regular file", snapshot: managedDestinationSnapshot{Kind: managedDestinationRegular, Content: managedContent("nix/go.nix", "old")}, want: managedOperationRemove},
		{name: "final symlink", snapshot: managedDestinationSnapshot{Kind: managedDestinationSymlink}, want: managedOperationRemove},
		{name: "unmarked regular file conflicts", snapshot: managedDestinationSnapshot{Kind: managedDestinationRegular, Content: []byte("custom")}, wantOwnerErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.snapshot.Path = selection.Entry.Path
			plan, err := planManagedFiles([]managedSelection{selection}, nil, []managedDestinationSnapshot{tt.snapshot})
			if tt.wantOwnerErr {
				if _, ok := errors.AsType[*managedOwnershipError](err); !ok {
					t.Fatalf("planManagedFiles() error = %v, want managedOwnershipError", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := plan.Files[0].Operation; got != tt.want {
				t.Fatalf("operation = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPlanManagedRejectsUnsafeActiveDestinations(t *testing.T) {
	selection := managedSelection{Entry: managedRegistryEntry{Path: "just/tailor.just", Policy: managedPolicyCore}, Enabled: true}
	rendered := managedRenderedFiles{selection.Entry.Path: managedContent(selection.Entry.Path, "content")}
	tests := []struct {
		name     string
		snapshot managedDestinationSnapshot
		want     string
	}{
		{name: "linked parent", snapshot: managedDestinationSnapshot{Kind: managedDestinationMissing, UnsafeParent: true}, want: "unsafe parent"},
		{name: "directory", snapshot: managedDestinationSnapshot{Kind: managedDestinationDirectory}, want: "is a directory"},
		{name: "special file", snapshot: managedDestinationSnapshot{Kind: managedDestinationSpecial}, want: "not a regular file or symlink"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.snapshot.Path = selection.Entry.Path
			_, err := planManagedFiles([]managedSelection{selection}, rendered, []managedDestinationSnapshot{tt.snapshot})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("planManagedFiles() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestManagedMarkersAreExactFirstLines(t *testing.T) {
	path := "nix/pages.nix"
	selection := managedSelection{
		Entry:   managedRegistryEntry{Path: path, Policy: managedPolicyFragment, Capability: managedCapabilityPages},
		Enabled: true,
	}
	tests := []struct {
		name    string
		content []byte
		valid   bool
	}{
		{name: "LF", content: []byte(managedMarker(path) + "\nbody"), valid: true},
		{name: "CRLF", content: []byte(managedMarker(path) + "\r\nbody"), valid: true},
		{name: "missing newline", content: []byte(managedMarker(path)), valid: false},
		{name: "partial", content: []byte("# Managed by Tailor\nbody"), valid: false},
		{name: "misplaced", content: []byte("body\n" + managedMarker(path) + "\n"), valid: false},
		{name: "wrong path", content: []byte(managedMarker("nix/go.nix") + "\nbody"), valid: false},
		{name: "bare carriage return", content: []byte(managedMarker(path) + "\rbody"), valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasManagedMarker(tt.content, path); got != tt.valid {
				t.Fatalf("hasManagedMarker() = %t, want %t", got, tt.valid)
			}
			_, err := planManagedFiles(
				[]managedSelection{selection},
				managedRenderedFiles{path: tt.content},
				[]managedDestinationSnapshot{{Path: path, Kind: managedDestinationMissing}},
			)
			if (err == nil) != tt.valid {
				t.Fatalf("planManagedFiles() error = %v, valid = %t", err, tt.valid)
			}
		})
	}
}

func TestPlanManagedFilesIsDeterministicAndModeIndependent(t *testing.T) {
	selections := []managedSelection{
		{Entry: managedRegistryEntry{Path: "nix/go.nix", Policy: managedPolicyFragment, Capability: managedCapabilityGo}},
		{Entry: managedRegistryEntry{Path: "just/tailor.just", Policy: managedPolicyCore}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "justfile", Policy: managedPolicyRoot}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo}, Enabled: true},
		{Entry: managedRegistryEntry{Path: "nix/loader.nix", Policy: managedPolicyLoader}, Enabled: true},
	}
	rendered := managedRenderedFiles{
		"just/tailor.just": managedContent("just/tailor.just", "core"),
		"justfile":         []byte("root"),
		"just/go.just":     managedContent("just/go.just", "go"),
		"nix/loader.nix":   managedContent("nix/loader.nix", "loader"),
	}
	snapshots := []managedDestinationSnapshot{
		{Path: "just/go.just", Kind: managedDestinationMissing},
		{Path: "justfile", Kind: managedDestinationMissing},
		{Path: "nix/go.nix", Kind: managedDestinationMissing},
		{Path: "nix/loader.nix", Kind: managedDestinationMissing},
		{Path: "just/tailor.just", Kind: managedDestinationMissing},
	}

	var baseline managedPlan
	for i, mode := range []ApplyMode{Apply, Recut} {
		t.Run(fmt.Sprintf("mode=%d", mode), func(t *testing.T) {
			plan, err := planManagedFiles(selections, rendered, snapshots)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				baseline = plan
			} else if !reflect.DeepEqual(plan, baseline) {
				t.Fatalf("plan differs by mode: got %#v, want %#v", plan, baseline)
			}
			got := make([]string, 0, len(plan.Files))
			for _, file := range plan.Files {
				got = append(got, file.Selection.Entry.Path)
			}
			want := []string{"nix/loader.nix", "justfile", "just/go.just", "just/tailor.just", "nix/go.nix"}
			if !slices.Equal(got, want) {
				t.Fatalf("planned order = %v, want %v", got, want)
			}
		})
	}
}

func TestPlanManagedFilesRejectsInactiveInspectionAndDuplicates(t *testing.T) {
	selection := managedSelection{Entry: managedRegistryEntry{Path: "just/tailor.just", Policy: managedPolicyCore}, Enabled: true}
	rendered := managedRenderedFiles{selection.Entry.Path: managedContent(selection.Entry.Path, "content")}

	_, err := planManagedFiles(
		[]managedSelection{selection},
		rendered,
		[]managedDestinationSnapshot{
			{Path: selection.Entry.Path, Kind: managedDestinationMissing},
			{Path: "nix/go.nix", Kind: managedDestinationRegular},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "is not active") {
		t.Fatalf("inactive snapshot error = %v", err)
	}

	_, err = planManagedFiles([]managedSelection{selection, selection}, rendered, nil)
	if err == nil || !strings.Contains(err.Error(), "managed plan contains duplicate destination") {
		t.Fatalf("duplicate selection error = %v", err)
	}

	plan := managedPlan{Files: []managedPlanFile{
		{Selection: selection, Operation: managedOperationNoop},
		{Selection: selection, Operation: managedOperationNoop},
	}}
	if err := validateManagedPlan(plan); err == nil || !strings.Contains(err.Error(), "managed plan contains duplicate destination") {
		t.Fatalf("validateManagedPlan() error = %v", err)
	}
}

func managedContent(path, body string) []byte {
	return []byte(managedMarker(path) + "\n" + body + "\n")
}
