package measure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestCheckHealthEmptyDir(t *testing.T) {
	dir := t.TempDir()
	results := CheckHealth(dir)

	// 10 health swatches + LICENSE = 11 missing, plus 1 README.md warning = 12
	if len(results) != 12 {
		t.Fatalf("CheckHealth() returned %d results, want 12", len(results))
	}

	for _, r := range results {
		if r.Path == "README.md" {
			if r.Status != Warning {
				t.Errorf("README.md: status = %q, want %q", r.Status, Warning)
			}
			continue
		}
		if r.Status != Missing {
			t.Errorf("destination %q: status = %q, want %q", r.Path, r.Status, Missing)
		}
	}
}

func TestCheckHealthAllPresent(t *testing.T) {
	dir := t.TempDir()

	// Create all 11 health check files plus README.md.
	files := []string{
		"CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md",
		"LICENSE",
		"README.md",
		"SECURITY.md",
		"SUPPORT.md",
		".github/FUNDING.yml",
		".github/dependabot.yml",
		".github/ISSUE_TEMPLATE/bug_report.yml",
		".github/ISSUE_TEMPLATE/feature_request.yml",
		".github/ISSUE_TEMPLATE/config.yml",
		".github/pull_request_template.md",
	}
	for _, f := range files {
		testutil.CreateFile(t, dir, f)
	}

	results := CheckHealth(dir)

	// 11 present, no README.md warning because it exists
	if len(results) != 11 {
		t.Fatalf("CheckHealth() returned %d results, want 11", len(results))
	}

	for _, r := range results {
		if r.Status != Present {
			t.Errorf("destination %q: status = %q, want %q", r.Path, r.Status, Present)
		}
	}
}

func TestCheckHealthMixedPresence(t *testing.T) {
	dir := t.TempDir()

	// Create a subset: LICENSE, CODE_OF_CONDUCT.md, SECURITY.md
	testutil.CreateFile(t, dir, "LICENSE")
	testutil.CreateFile(t, dir, "CODE_OF_CONDUCT.md")
	testutil.CreateFile(t, dir, "SECURITY.md")

	results := CheckHealth(dir)

	// 8 missing + 1 warning (README.md) + 3 present = 12
	if len(results) != 12 {
		t.Fatalf("CheckHealth() returned %d results, want 12", len(results))
	}

	missing := 0
	warning := 0
	present := 0
	for _, r := range results {
		switch r.Status {
		case Missing:
			missing++
		case Warning:
			warning++
		case Present:
			present++
		default:
			t.Errorf("unexpected status %q for %q", r.Status, r.Path)
		}
	}

	if missing != 8 {
		t.Errorf("missing count = %d, want 8", missing)
	}
	if warning != 1 {
		t.Errorf("warning count = %d, want 1", warning)
	}
	if present != 3 {
		t.Errorf("present count = %d, want 3", present)
	}
}

func TestCheckHealthSortOrder(t *testing.T) {
	dir := t.TempDir()

	// Create just LICENSE so we get a mix of missing, warning, and present.
	testutil.CreateFile(t, dir, "LICENSE")

	results := CheckHealth(dir)

	// Verify group order: all missing, then all warning, then all present.
	statusOrder := map[HealthStatus]int{Missing: 0, Warning: 1, Present: 2}
	maxSeen := 0
	for _, r := range results {
		order := statusOrder[r.Status]
		if order < maxSeen {
			t.Errorf("entry %q (%s) appeared after a later status group", r.Path, r.Status)
		}
		if order > maxSeen {
			maxSeen = order
		}
	}

	// Within each group, destinations are sorted lexicographically.
	var missingDests, warningDests, presentDests []string
	for _, r := range results {
		switch r.Status {
		case Missing:
			missingDests = append(missingDests, r.Path)
		case Warning:
			warningDests = append(warningDests, r.Path)
		case Present:
			presentDests = append(presentDests, r.Path)
		}
	}

	for i := 1; i < len(missingDests); i++ {
		if missingDests[i] < missingDests[i-1] {
			t.Errorf("missing entries not sorted: %q before %q", missingDests[i-1], missingDests[i])
		}
	}
	for i := 1; i < len(warningDests); i++ {
		if warningDests[i] < warningDests[i-1] {
			t.Errorf("warning entries not sorted: %q before %q", warningDests[i-1], warningDests[i])
		}
	}
	for i := 1; i < len(presentDests); i++ {
		if presentDests[i] < presentDests[i-1] {
			t.Errorf("present entries not sorted: %q before %q", presentDests[i-1], presentDests[i])
		}
	}
}

func TestCheckHealthDirectoryNotCountedAsFile(t *testing.T) {
	dir := t.TempDir()

	// Create LICENSE as a directory, not a file.
	if err := os.MkdirAll(filepath.Join(dir, "LICENSE"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "LICENSE" {
			if r.Status != Missing {
				t.Errorf("LICENSE directory should be reported as missing, got %q", r.Status)
			}
			return
		}
	}
	t.Error("LICENSE not found in results")
}

func TestCheckHealthLicenseWithPlaceholders(t *testing.T) {
	dir := t.TempDir()

	// Write a LICENSE with unresolved placeholders.
	content := "MIT License\n\nCopyright (c) [year] [fullname]\n"
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "LICENSE" {
			if r.Status != Warning {
				t.Errorf("LICENSE with placeholders: status = %q, want %q", r.Status, Warning)
			}
			if r.Detail != "(contains unresolved placeholders)" {
				t.Errorf("LICENSE detail = %q, want %q", r.Detail, "(contains unresolved placeholders)")
			}
			return
		}
	}
	t.Error("LICENSE not found in results")
}

func TestCheckHealthLicenseResolved(t *testing.T) {
	dir := t.TempDir()

	// Write a LICENSE without placeholders.
	content := "MIT License\n\nCopyright (c) 2024 Jane Smith\n"
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "LICENSE" {
			if r.Status != Present {
				t.Errorf("resolved LICENSE: status = %q, want %q", r.Status, Present)
			}
			if r.Detail != "" {
				t.Errorf("resolved LICENSE: detail = %q, want empty", r.Detail)
			}
			return
		}
	}
	t.Error("LICENSE not found in results")
}

func TestCheckHealthLicenseWithBracePlaceholders(t *testing.T) {
	dir := t.TempDir()

	// Write a LICENSE with curly-brace placeholders.
	content := "Apache License 2.0\n\nCopyright {project}\n"
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "LICENSE" {
			if r.Status != Warning {
				t.Errorf("LICENSE with brace placeholders: status = %q, want %q", r.Status, Warning)
			}
			return
		}
	}
	t.Error("LICENSE not found in results")
}

func TestCheckHealthReadmeMissing(t *testing.T) {
	dir := t.TempDir()

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "README.md" {
			if r.Status != Warning {
				t.Errorf("missing README.md: status = %q, want %q", r.Status, Warning)
			}
			if r.Detail != "(not managed by tailor)" {
				t.Errorf("README.md detail = %q, want %q", r.Detail, "(not managed by tailor)")
			}
			return
		}
	}
	t.Error("README.md not found in results")
}

func TestCheckHealthReadmePresent(t *testing.T) {
	dir := t.TempDir()
	testutil.CreateFile(t, dir, "README.md")

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "README.md" {
			t.Errorf("README.md should not appear in results when present, got status %q", r.Status)
		}
	}
}

func TestCheckHealthSingleResultPerPath(t *testing.T) {
	dir := t.TempDir()

	// LICENSE with placeholders should appear once as warning, not also as present.
	content := "MIT License\n\nCopyright (c) [year] [fullname]\n"
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	results := CheckHealth(dir)

	pathCount := make(map[string]int)
	for _, r := range results {
		pathCount[r.Path]++
	}

	for path, count := range pathCount {
		if count > 1 {
			t.Errorf("path %q appears %d times, want 1", path, count)
		}
	}
}
