package alter

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/model"
)

func TestManagedTestAggregateExecution(t *testing.T) {
	just := managedLintExecutable(t)
	for _, state := range []string{"true", "false", "absent", "failure"} {
		t.Run(state, func(t *testing.T) {
			root := t.TempDir()
			cfg := &config.Config{Pages: &model.PagesSettings{Enabled: new(true)}}
			if state != "absent" {
				cfg.Languages = &config.LanguageSettings{Go: new(state != "false")}
			}
			registry := []managedRegistryEntry{
				{Path: "just/pages.just", Policy: managedPolicyFragment, Capability: managedCapabilityPages, TestRecipe: "test-pages"},
				{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestRecipe: "test-go"},
			}
			core, err := renderManagedAggregate(cfg, []byte("test: [[TAILOR_MANAGED_TEST_DEPENDENCIES]]\n    [[TAILOR_MANAGED_TEST_COMMAND]]\n"), registry, managedTest)
			if err != nil {
				t.Fatal(err)
			}
			writeManagedTestFile(t, root, "justfile", []byte("import 'just/tailor.just'\nimport 'just/go.just'\nimport 'just/pages.just'\n"))
			writeManagedTestFile(t, root, "just/tailor.just", core)
			goBody := "test-go:\n    @pwd\n    @echo go\n"
			if state == "failure" {
				goBody += "    @false\n"
			}
			writeManagedTestFile(t, root, "just/go.just", []byte(goBody))
			writeManagedTestFile(t, root, "just/pages.just", []byte("test-pages:\n    @echo pages\n"))
			// #nosec G204 -- The executable and temporary fixture are controlled by the test.
			cmd := exec.CommandContext(t.Context(), just, "--justfile", filepath.Join(root, "justfile"), "test")
			cmd.Dir = filepath.Join(root, "just")
			output, err := cmd.CombinedOutput()
			if state == "failure" {
				if err == nil || strings.Contains(string(output), "pages") {
					t.Fatalf("failure output=%q error=%v", output, err)
				}
			} else {
				want := "pages\n"
				if state == "true" {
					want = root + "\ngo\npages\n"
				}
				if err != nil || string(output) != want {
					t.Fatalf("output=%q error=%v, want %q", output, err, want)
				}
			}
		})
	}
}

func TestManagedTestEmptyAndReleaseDefault(t *testing.T) {
	for _, cfg := range []*config.Config{{}, {Languages: &config.LanguageSettings{Go: new(false)}}} {
		root := t.TempDir()
		rendered := renderSelectedManagedFiles(t, cfg)
		writeManagedJustFixture(t, root, rendered)
		writeManagedTestFile(t, root, "justfile", []byte("import 'just/loader.just'\n"))
		if !bytes.Contains(rendered["just/tailor.just"], []byte("release $VERSION:")) {
			t.Fatal("core lacks default release")
		}
		// #nosec G204 -- The executable and temporary fixture are controlled by the test.
		output, err := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "test").CombinedOutput()
		if err != nil || string(output) != "No test recipes are enabled.\n" {
			t.Fatalf("output=%q error=%v", output, err)
		}
	}
}

func TestManagedLegacyTestPreflightAndMigration(t *testing.T) {
	root := t.TempDir()
	legacy := managedContent("just/go.just", "test:\n    @go test ./...")
	writeManagedTestFile(t, root, "just/go.just", legacy)
	before := snapshotManagedTree(t, root)
	cfg := &config.Config{}
	selections, err := selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = preflightManagedFiles(root, selections, renderSelectedManagedFiles(t, cfg))
	if err == nil || !strings.Contains(err.Error(), "legacy test recipe") || !strings.Contains(err.Error(), "set languages.go to true") {
		t.Fatalf("error=%v", err)
	}
	if after := snapshotManagedTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("preflight changed files")
	}
	cfg.Languages = &config.LanguageSettings{Go: new(true)}
	selections, err = selectManagedFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	rendered := renderSelectedManagedFiles(t, cfg)
	if _, err := preflightManagedFiles(root, selections, rendered); err != nil {
		t.Fatal(err)
	}
	if !hasManagedRecipe(rendered["just/go.just"], "test-go") || hasManagedRecipe(rendered["just/go.just"], "test") {
		t.Fatal("Go migration did not rename test")
	}
}

func TestManagedOperationPreflightRequiresRecipes(t *testing.T) {
	for _, operation := range managedOperations() {
		for _, missing := range []bool{false, true} {
			cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true)}}
			selections, err := selectManagedFiles(cfg)
			if err != nil {
				t.Fatal(err)
			}
			rendered := renderSelectedManagedFiles(t, cfg)
			if missing {
				delete(rendered, "just/go.just")
			} else {
				rendered["just/go.just"] = bytes.ReplaceAll(rendered["just/go.just"], []byte(string(operation)+"-go:"), []byte("missing:"))
			}
			if _, err := preflightManagedFiles(t.TempDir(), selections, rendered); err == nil {
				t.Fatalf("accepted missing %s recipe, fragment=%v", operation, missing)
			}
		}
	}
}

func TestManagedTestAggregateRejectsMalformedPlaceholders(t *testing.T) {
	for _, content := range []string{
		"test:\n",
		"test: [[TAILOR_MANAGED_TEST_DEPENDENCIES]]\n",
		"test: [[TAILOR_MANAGED_TEST_DEPENDENCIES]] [[TAILOR_MANAGED_TEST_DEPENDENCIES]]\n    [[TAILOR_MANAGED_TEST_COMMAND]]\n",
		"test: [[TAILOR_MANAGED_TEST_DEPENDENCIES]]\n    [[TAILOR_MANAGED_TEST_COMMAND]]\n# [[TAILOR_MANAGED_TEST_UNKNOWN]]\n",
	} {
		if _, err := renderManagedAggregate(&config.Config{}, []byte(content), fixedManagedRegistry(), managedTest); err == nil {
			t.Fatalf("accepted malformed test placeholders: %q", content)
		}
	}
}

func TestManagedOperationMetadataValidation(t *testing.T) {
	for _, entries := range [][]managedRegistryEntry{
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestRecipe: "test go"}},
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestRecipe: "test-go"}, {Path: "just/extra.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestRecipe: "test-extra"}},
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestRecipe: "same", BuildRecipe: "same"}},
	} {
		if err := validateManagedRegistry(entries); err == nil {
			t.Fatalf("accepted invalid metadata: %#v", entries)
		}
	}
}
