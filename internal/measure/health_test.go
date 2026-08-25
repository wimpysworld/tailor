package measure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wimpysworld/tailor/internal/testutil"
)

func TestHasUnresolvedPlaceholdersSupportedNames(t *testing.T) {
	names := []string{
		"year",
		"yyyy",
		"fullname",
		"name of copyright owner",
		"name of copyright holder",
		"software name",
		"project",
		"projecturl",
		"email",
	}
	delimiters := []struct {
		name  string
		open  string
		close string
	}{
		{name: "square", open: "[", close: "]"},
		{name: "curly", open: "{", close: "}"},
	}

	for _, name := range names {
		for _, delimiter := range delimiters {
			t.Run(name+"/"+delimiter.name, func(t *testing.T) {
				variants := []string{
					name,
					strings.ToUpper(name),
					" \t\n\v\f\r" + name + "\r\f\v\n\t ",
					strings.ReplaceAll(name, " ", " \t\n\v\f\r "),
				}
				for _, variant := range variants {
					token := delimiter.open + variant + delimiter.close
					if !hasUnresolvedPlaceholders([]byte(token)) {
						t.Errorf("hasUnresolvedPlaceholders(%q) = false, want true", token)
					}
				}
			})
		}
	}
}

func TestHasUnresolvedPlaceholdersRejectsOtherBracketedText(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "blue oak markdown link", content: "complied with [Notices](#notices)"},
		{name: "markdown link matching placeholder", content: "see [year](#year)"},
		{name: "markdown link with plain destination", content: "see [year](unfinished)"},
		{name: "markdown link with nested destination", content: "see [year](docs/(archive))"},
		{name: "cecill name fragments", content: "Ce[a] C[nrs] I[nria] L[ogiciel] L[ibre]"},
		{name: "gpl application example", content: "Copyright (C) <year> <name of author>"},
		{name: "resolved copyright", content: "Copyright (c) 2026 Jane Smith"},
		{name: "arbitrary square brackets", content: "[licence terms]"},
		{name: "arbitrary curly braces", content: "{licence terms}"},
		{name: "longer name", content: "[yearly]"},
		{name: "prefixed name", content: "[copyright year]"},
		{name: "space within projecturl", content: "{project url}"},
		{name: "punctuation within email", content: "[e-mail]"},
		{name: "square open curly close", content: "[year}"},
		{name: "curly open square close", content: "{year]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if hasUnresolvedPlaceholders([]byte(tt.content)) {
				t.Errorf("hasUnresolvedPlaceholders(%q) = true, want false", tt.content)
			}
		})
	}
}

func TestHasUnresolvedPlaceholdersFindsTokenAmongValidText(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "after valid markdown link", content: "complied with [Notices](#notices)\nCopyright (c) [year] Jane Smith"},
		{name: "link opening without destination", content: "[year]("},
		{name: "link destination without closing parenthesis", content: "[year](unfinished"},
		{name: "after malformed same delimiter", content: "[broken [year]"},
		{name: "inside mixed delimiters", content: "{text [year]}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !hasUnresolvedPlaceholders([]byte(tt.content)) {
				t.Errorf("hasUnresolvedPlaceholders(%q) = false, want true", tt.content)
			}
		})
	}
}

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

	// A lone LICENSE produces a mix of missing, warning, and present results.
	testutil.CreateFile(t, dir, "LICENSE")

	results := CheckHealth(dir)

	// Group order is all missing, then all warning, then all present.
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

func TestCheckHealthLicenseHardening(t *testing.T) {
	placeholderContent := "MIT License\n\nCopyright (c) [year] [fullname]\n"

	tests := []struct {
		name       string
		setup      func(t *testing.T, dir string)
		wantStatus HealthStatus
		wantDetail string
	}{
		{
			name: "symlinked licence is missing",
			setup: func(t *testing.T, dir string) {
				target := filepath.Join(dir, "target.txt")
				if err := os.WriteFile(target, []byte(placeholderContent), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				if err := os.Symlink(target, filepath.Join(dir, "LICENSE")); err != nil {
					t.Fatalf("Symlink: %v", err)
				}
			},
			wantStatus: Missing,
		},
		{
			name: "over-limit licence skips the placeholder check",
			setup: func(t *testing.T, dir string) {
				content := make([]byte, 1<<20+1)
				copy(content, placeholderContent)
				if err := os.WriteFile(filepath.Join(dir, "LICENSE"), content, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			},
			wantStatus: Present,
		},
		{
			name: "licence with placeholders warns",
			setup: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(placeholderContent), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			},
			wantStatus: Warning,
			wantDetail: "(contains unresolved placeholders)",
		},
		{
			name: "resolved licence is present",
			setup: func(t *testing.T, dir string) {
				content := "MIT License\n\nCopyright (c) 2024 Jane Smith\n"
				if err := os.WriteFile(filepath.Join(dir, "LICENSE"), []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
			},
			wantStatus: Present,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.setup(t, dir)

			results := CheckHealth(dir)

			for _, r := range results {
				if r.Path == "LICENSE" {
					if r.Status != tt.wantStatus {
						t.Errorf("LICENSE status = %q, want %q", r.Status, tt.wantStatus)
					}
					if r.Detail != tt.wantDetail {
						t.Errorf("LICENSE detail = %q, want %q", r.Detail, tt.wantDetail)
					}
					return
				}
			}
			t.Error("LICENSE not found in results")
		})
	}
}

func TestReadLicenceRejectsSymlinkOutsideProject(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target.txt")
	if err := os.WriteFile(target, []byte("MIT License\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dir := filepath.Join(base, "project")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "LICENSE")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer root.Close()

	if _, err := readLicence(root, "LICENSE"); err == nil {
		t.Error("readLicence() error = nil, want error for symlink outside the project")
	}
}

func TestCheckHealthSymlinkedParentEscapingProjectIsMissing(t *testing.T) {
	base := t.TempDir()

	// Populate a directory outside the project with the .github health files.
	outside := filepath.Join(base, "outside")
	githubFiles := []string{
		"FUNDING.yml",
		"dependabot.yml",
		"pull_request_template.md",
		"ISSUE_TEMPLATE/bug_report.yml",
		"ISSUE_TEMPLATE/feature_request.yml",
		"ISSUE_TEMPLATE/config.yml",
	}
	for _, f := range githubFiles {
		testutil.CreateFile(t, outside, f)
	}

	dir := filepath.Join(base, "project")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".github")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if strings.HasPrefix(r.Path, ".github/") && r.Status != Missing {
			t.Errorf("%q behind escaping symlinked parent: status = %q, want %q", r.Path, r.Status, Missing)
		}
	}
}

func TestCheckHealthSymlinkedReadmeWarns(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("# Project\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	results := CheckHealth(dir)

	for _, r := range results {
		if r.Path == "README.md" {
			if r.Status != Warning {
				t.Errorf("symlinked README.md: status = %q, want %q", r.Status, Warning)
			}
			return
		}
	}
	t.Error("README.md not found in results")
}

func TestHasUnresolvedPlaceholdersAdversarialContentStaysFast(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "many opens before one closer",
			content: strings.Repeat("[", 1<<19) + "]",
			want:    false,
		},
		{
			name:    "repeated unbalanced links",
			content: strings.Repeat("[a](", 1<<17),
			want:    false,
		},
		{
			name:    "unbalanced links before placeholder",
			content: strings.Repeat("[a](", 1<<17) + "[year]",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			got := hasUnresolvedPlaceholders([]byte(tt.content))
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Errorf("hasUnresolvedPlaceholders took %v, want under 5s", elapsed)
			}
			if got != tt.want {
				t.Errorf("hasUnresolvedPlaceholders() = %v, want %v", got, tt.want)
			}
		})
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
