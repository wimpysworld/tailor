package alter

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/config"
)

func TestManagedTestAggregateExecution(t *testing.T) {
	for _, state := range []string{"true", "false", "absent", "failure"} {
		t.Run(state, func(t *testing.T) {
			root, bin, log := qualityFixture(t, state == "true" || state == "failure")
			writeManagedTestFile(t, root, "just/project/test.sh", []byte("printf 'project %s\n' \"$PWD\" >> \"$CALLS\"\n"))
			fail := ""
			if state == "failure" {
				fail = "go vet ./..."
			}
			output, err := qualityRun(t, root, bin, log, fail, "test")
			if (err != nil) != (state == "failure") {
				t.Fatalf("test: %v %s", err, output)
			}
			want := []string{"project " + root}
			if state == "true" {
				want = append([]string{"go vet ./...", "go test -cover -covermode=atomic ./..."}, want...)
			}
			if state == "failure" {
				want = []string{"go vet ./..."}
			}
			if got := managedLintLog(t, log); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls %q, want %q", got, want)
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
		command := exec.CommandContext(t.Context(), managedLintExecutable(t), "--justfile", filepath.Join(root, "justfile"), "test")
		command.Env = managedEnvironmentWithout("BASH_ENV", "ENV")
		output, err := command.CombinedOutput()
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
	if !bytes.Contains(rendered["just/tailor.just"], []byte("go test -cover -covermode=atomic ./...")) || hasManagedRecipe(rendered["just/go.just"], "test-go") {
		t.Fatal("Go migration did not rename test")
	}
}

func TestManagedOperationPreflightRequiresRecipes(t *testing.T) {
	for _, operation := range managedOperations() {
		for _, missing := range []bool{false, true} {
			cfg := &config.Config{Languages: &config.LanguageSettings{Go: new(true), GoOptions: &config.GoOptions{FFmpegStatigo: new(true)}}}
			destination, recipe := "just/tailor.just", map[managedRecipeOperation]string{managedBuild: "goexe=", managedTest: "go vet ./...", managedLint: "go mod tidy -diff"}[operation]
			if operation == managedSetup {
				recipe = "git config pull.ff only"
			}
			selections, err := selectManagedFiles(cfg)
			if err != nil {
				t.Fatal(err)
			}
			rendered := renderSelectedManagedFiles(t, cfg)
			if missing {
				delete(rendered, destination)
			} else {
				rendered[destination] = bytes.ReplaceAll(rendered[destination], []byte(recipe), []byte("missing:"))
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
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestBody: "test go"}},
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestBody: "test-go"}, {Path: "just/extra.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestBody: "test-extra"}},
		{{Path: "just/go.just", Policy: managedPolicyFragment, Capability: managedCapabilityGo, TestBody: "same", BuildBody: "same"}},
	} {
		if err := validateManagedRegistry(entries); err == nil {
			t.Fatalf("accepted invalid metadata: %#v", entries)
		}
	}
}
