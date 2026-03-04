package measure

import (
	"os"
	"path/filepath"
	"testing"
)

func createFile(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestCheckHealthEmptyDir(t *testing.T) {
	dir := t.TempDir()
	results := CheckHealth(dir)

	if len(results) != 11 {
		t.Fatalf("CheckHealth() returned %d results, want 11", len(results))
	}

	for _, r := range results {
		if r.Status != Missing {
			t.Errorf("destination %q: status = %q, want %q", r.Destination, r.Status, Missing)
		}
	}
}

func TestCheckHealthAllPresent(t *testing.T) {
	dir := t.TempDir()

	// Create all 11 health check files.
	files := []string{
		"CODE_OF_CONDUCT.md",
		"CONTRIBUTING.md",
		"LICENSE",
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
		createFile(t, dir, f)
	}

	results := CheckHealth(dir)

	if len(results) != 11 {
		t.Fatalf("CheckHealth() returned %d results, want 11", len(results))
	}

	for _, r := range results {
		if r.Status != Present {
			t.Errorf("destination %q: status = %q, want %q", r.Destination, r.Status, Present)
		}
	}
}

func TestCheckHealthMixedPresence(t *testing.T) {
	dir := t.TempDir()

	// Create a subset: LICENSE, CODE_OF_CONDUCT.md, SECURITY.md
	createFile(t, dir, "LICENSE")
	createFile(t, dir, "CODE_OF_CONDUCT.md")
	createFile(t, dir, "SECURITY.md")

	results := CheckHealth(dir)

	if len(results) != 11 {
		t.Fatalf("CheckHealth() returned %d results, want 11", len(results))
	}

	missing := 0
	present := 0
	for _, r := range results {
		switch r.Status {
		case Missing:
			missing++
		case Present:
			present++
		default:
			t.Errorf("unexpected status %q for %q", r.Status, r.Destination)
		}
	}

	if missing != 8 {
		t.Errorf("missing count = %d, want 8", missing)
	}
	if present != 3 {
		t.Errorf("present count = %d, want 3", present)
	}
}

func TestCheckHealthSortOrder(t *testing.T) {
	dir := t.TempDir()

	// Create just LICENSE so we get a mix.
	createFile(t, dir, "LICENSE")

	results := CheckHealth(dir)

	// All missing entries come first, then all present entries.
	seenPresent := false
	for _, r := range results {
		if r.Status == Present {
			seenPresent = true
		}
		if r.Status == Missing && seenPresent {
			t.Errorf("missing entry %q appeared after present entries", r.Destination)
		}
	}

	// Within each group, destinations are sorted lexicographically.
	var missingDests, presentDests []string
	for _, r := range results {
		if r.Status == Missing {
			missingDests = append(missingDests, r.Destination)
		} else {
			presentDests = append(presentDests, r.Destination)
		}
	}

	for i := 1; i < len(missingDests); i++ {
		if missingDests[i] < missingDests[i-1] {
			t.Errorf("missing entries not sorted: %q before %q", missingDests[i-1], missingDests[i])
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
		if r.Destination == "LICENSE" {
			if r.Status != Missing {
				t.Errorf("LICENSE directory should be reported as missing, got %q", r.Status)
			}
			return
		}
	}
	t.Error("LICENSE not found in results")
}
