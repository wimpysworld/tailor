package alter_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wimpysworld/tailor/internal/alter"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/output"
)

func TestDependabotPreflightPrecedesWikiEnablement(t *testing.T) {
	tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: true\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n")
	writeOnDisk(t, tc.Dir, ".github/dependabot.yaml", []byte("alternate\n"))

	report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
	if err == nil || !strings.Contains(err.Error(), "alternate .github/dependabot.yaml") {
		t.Fatalf("error = %v, want alternate-name conflict", err)
	}
	if calls := tc.Calls(); len(calls) != 0 {
		t.Fatalf("Dependabot preflight allowed API calls: %v", calls)
	}
	assertOneDependabotItem(t, report, output.Attention)
}

func TestDependabotHasOneResultAndNoOrdinaryWriter(t *testing.T) {
	tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: false\nlanguages:\n  go: true\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n")
	cfg := loadTestConfig(t, tc.Dir)

	report, err := alter.Execute(cfg, tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertOneDependabotItem(t, report, output.Applied)
	path := filepath.Join(tc.Dir, ".github/dependabot.yml")
	before, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(before, []byte("# Managed by Tailor: .github/dependabot.yml\n")) {
		t.Fatalf("Dependabot output = %q, error = %v", before, err)
	}
	results, err := alter.ProcessOrdinarySwatchesForTest(cfg, tc.Dir, alter.Recut, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("ordinary results = %v, want none", results)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("ordinary dispatch changed the dedicated Dependabot output")
	}
}

func TestDependabotPreparationUsesMergedDefaults(t *testing.T) {
	tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: false\nswatches:\n  - path: .tailor.yml\n    alteration: always\n")
	cfg := loadTestConfig(t, tc.Dir)

	report, err := alter.Execute(cfg, tc.Dir, alter.DryRun, tc.Client, nil, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SwatchActive(".github/dependabot.yml") {
		t.Fatal("default merge did not activate Dependabot before preparation")
	}
	assertOneDependabotItem(t, report, output.Alteration)
	if _, err := os.Lstat(filepath.Join(tc.Dir, ".github")); !os.IsNotExist(err) {
		t.Fatalf("preview created .github: %v", err)
	}
}

func TestDependabotDefaultPreparationConflictStopsBeforeCalls(t *testing.T) {
	tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: true\nswatches:\n  - path: .tailor.yml\n    alteration: always\n")
	writeOnDisk(t, tc.Dir, ".github/dependabot.yaml", []byte("alternate\n"))

	report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
	if err == nil || !strings.Contains(err.Error(), "alternate .github/dependabot.yaml") {
		t.Fatalf("error = %v, want merged-default conflict", err)
	}
	if calls := tc.Calls(); len(calls) != 0 {
		t.Fatalf("merged-default preflight allowed API calls: %v", calls)
	}
	assertOneDependabotItem(t, report, output.Attention)
}

func TestDependabotPublicationWaitsForRepositoryStage(t *testing.T) {
	tc := setupAlterTest(t, "license: none\nrepository:\n  has_wiki: false\n  description: changed\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n", WithPatchError(http.StatusInternalServerError))
	original := []byte("# Managed by Tailor: .github/dependabot.yml\nold\n")
	writeOnDisk(t, tc.Dir, ".github/dependabot.yml", original)

	report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
	if err == nil {
		t.Fatal("repository failure was not returned")
	}
	assertNoDependabotItem(t, report)
	got, readErr := os.ReadFile(filepath.Join(tc.Dir, ".github/dependabot.yml"))
	if readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("Dependabot changed before repository failure: %q, %v", got, readErr)
	}
}

func TestDependabotPublicationWaitsForLicenceStage(t *testing.T) {
	tc := setupAlterTest(t, "license: mit\nrepository:\n  has_wiki: false\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n", WithLicenceError(http.StatusInternalServerError))
	original := []byte("# Managed by Tailor: .github/dependabot.yml\nold\n")
	writeOnDisk(t, tc.Dir, ".github/dependabot.yml", original)

	report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.Apply, tc.Client, nil, alter.Options{})
	if err == nil {
		t.Fatal("licence failure was not returned")
	}
	assertNoDependabotItem(t, report)
	got, readErr := os.ReadFile(filepath.Join(tc.Dir, ".github/dependabot.yml"))
	if readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("Dependabot changed before licence failure: %q, %v", got, readErr)
	}
}

func TestDependabotPublishesAfterTrackedWorkflowParentCreation(t *testing.T) {
	t.Cleanup(gh.SetInspectWikiFunc(func(string, string, string) error { return nil }))
	for _, tc := range []struct {
		name       string
		configYAML string
		want       []string
		pages      bool
	}{
		{
			name:       "Pages only",
			configYAML: "license: none\nrepository:\n  has_wiki: false\npages:\n  enabled: true\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n",
			want:       []string{".github/dependabot.yml", ".github/workflows/tailor-pages.yml"},
			pages:      true,
		},
		{
			name:       "wiki only",
			configYAML: "license: none\nrepository:\n  has_wiki: true\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n  - path: wiki/Home.md\n    alteration: first-fit\n",
			want:       []string{".github/dependabot.yml", ".github/workflows/tailor-wiki.yml"},
		},
		{
			name:       "Pages and wiki",
			configYAML: "license: none\nrepository:\n  has_wiki: true\npages:\n  enabled: true\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n  - path: wiki/Home.md\n    alteration: first-fit\n",
			want:       []string{".github/dependabot.yml", ".github/workflows/tailor-pages.yml", ".github/workflows/tailor-wiki.yml"},
			pages:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, client := newPagesAcceptanceAPI(t)
			dir := t.TempDir()
			writeOnDisk(t, dir, ".tailor.yml", []byte(tc.configYAML))
			if tc.pages {
				writeOnDisk(t, dir, "pages/index.html", []byte("site\n"))
			}
			if _, err := os.Lstat(filepath.Join(dir, ".github")); !os.IsNotExist(err) {
				t.Fatalf("fixture has .github before execution: %v", err)
			}

			report, err := alter.Execute(loadTestConfig(t, dir), dir, alter.Apply, client, nil, alter.Options{})
			if err != nil {
				t.Fatal(err)
			}
			assertOneDependabotItem(t, report, output.Applied)
			for _, name := range tc.want {
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil {
					t.Fatalf("%s missing after execution: %v", name, err)
				}
			}
		})
	}
}

func TestDependabotPreviewLeavesCompleteTreeUnchanged(t *testing.T) {
	s, client := newPagesAcceptanceAPI(t)
	dir := t.TempDir()
	writeOnDisk(t, dir, ".tailor.yml", []byte("license: none\nrepository:\n  has_wiki: true\npages:\n  enabled: true\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n  - path: wiki/Home.md\n    alteration: first-fit\n"))
	writeOnDisk(t, dir, "pages/index.html", []byte("site\n"))
	before := pagesAcceptanceSnapshot(t, dir)

	report, err := alter.Execute(loadTestConfig(t, dir), dir, alter.DryRun, client, nil, alter.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertOneDependabotItem(t, report, output.Alteration)
	if len(s.writes) != 0 || !reflect.DeepEqual(before, pagesAcceptanceSnapshot(t, dir)) {
		t.Fatalf("preview changed API or filesystem state: writes=%v", s.writes)
	}
}

func TestDependabotReportingIsSafeAndStructured(t *testing.T) {
	t.Run("replacement", func(t *testing.T) {
		tc := setupAlterTest(t, "license: none\nlanguages:\n  go: false\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n")
		writeOnDisk(t, tc.Dir, ".github/dependabot.yml", []byte("# Managed by Tailor: .github/dependabot.yml\nregistries:\n  private:\n    password: old-secret\n"))

		report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.DryRun, tc.Client, nil, alter.Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"`github-actions` and `nix`", "replaces the complete Dependabot file", "Custom schedules"} {
			if !strings.Contains(report.Plain, want) || !dependabotGuidanceContains(report, want) {
				t.Errorf("replacement guidance lacks %q", want)
			}
		}
		assertDependabotReportOmitsSecret(t, report, "old-secret")
	})

	t.Run("unadopted", func(t *testing.T) {
		tc := setupAlterTest(t, "license: none\nswatches:\n  - path: .github/dependabot.yml\n    alteration: always\n")
		writeOnDisk(t, tc.Dir, ".github/dependabot.yml", []byte("password: old-secret\n"))

		report, err := alter.Execute(loadTestConfig(t, tc.Dir), tc.Dir, alter.DryRun, tc.Client, nil, alter.Options{})
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"Back up", "Review both supported", "`languages.go` explicitly", ".github/dependabot.yaml", "`tailor baste`", "only after approval"} {
			if !strings.Contains(report.Plain, want) || !dependabotGuidanceContains(report, want) {
				t.Errorf("adoption guidance lacks %q", want)
			}
		}
		assertDependabotReportOmitsSecret(t, report, "old-secret")
	})
}

func assertDependabotReportOmitsSecret(t *testing.T, report alter.Report, secret string) {
	t.Helper()
	structured, err := json.Marshal(report.Document)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(report.Plain, secret) || bytes.Contains(structured, []byte(secret)) {
		t.Fatalf("Dependabot report exposed secret %q", secret)
	}
}

func dependabotGuidanceContains(report alter.Report, text string) bool {
	for _, guidance := range report.Document.Guidance {
		if strings.Contains(guidance.Text, text) {
			return true
		}
	}
	return false
}

func assertNoDependabotItem(t *testing.T, report alter.Report) {
	t.Helper()
	for _, item := range report.Document.Items {
		if item.Name == ".github/dependabot.yml" {
			t.Fatalf("unexpected Dependabot result: %#v", item)
		}
	}
}

func assertOneDependabotItem(t *testing.T, report alter.Report, want output.Outcome) {
	t.Helper()
	count := 0
	for _, item := range report.Document.Items {
		if item.Name != ".github/dependabot.yml" {
			continue
		}
		count++
		if item.Outcome != want {
			t.Errorf("Dependabot outcome = %q, want %q", item.Outcome, want)
		}
	}
	if count != 1 {
		t.Fatalf("Dependabot result count = %d, want 1", count)
	}
}
