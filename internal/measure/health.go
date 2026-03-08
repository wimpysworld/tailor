package measure

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/wimpysworld/tailor/internal/fsutil"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// HealthStatus indicates whether a health file is present or missing.
type HealthStatus string

const (
	Missing HealthStatus = "missing"
	Warning HealthStatus = "warning"
	Present HealthStatus = "present"
)

// Label returns the status string with a trailing colon, suitable for formatted output.
func (s HealthStatus) Label() string { return string(s) + ":" }

// HealthResult pairs a path with its on-disk status and optional detail.
type HealthResult struct {
	Path   string
	Status HealthStatus
	Detail string
}

// placeholderRe matches unresolved bracket or brace tokens in licence templates,
// such as [year], [fullname], [yyyy], [name of copyright owner], or {project}.
var placeholderRe = regexp.MustCompile(`\[[^\]]+\]|\{[^}]+\}`)

// hasUnresolvedPlaceholders reports whether data contains any bracket or brace
// tokens typical of GitHub licence templates.
func hasUnresolvedPlaceholders(data []byte) bool {
	return placeholderRe.Match(data)
}

// readmeFile is the exact filename checked as a local health diagnostic.
const readmeFile = "README.md"

// CheckHealth checks whether each health swatch path, the LICENSE file, and
// README.md exist in dir. LICENSE files containing unresolved placeholder
// tokens are reported as warnings rather than present. A missing README.md
// is reported as a warning. Returns results sorted lexicographically by path
// within each status group (missing, warning, present).
func CheckHealth(dir string) []HealthResult {
	healthSwatches := swatch.HealthSwatches()
	paths := make([]string, 0, len(healthSwatches)+1)
	for _, s := range healthSwatches {
		paths = append(paths, s.Path)
	}
	paths = append(paths, swatch.LicenseDestination)

	var missing, warning, present []HealthResult
	for _, p := range paths {
		fullPath := filepath.Join(dir, p)
		if !fsutil.FileExists(fullPath) {
			missing = append(missing, HealthResult{Path: p, Status: Missing})
			continue
		}
		if p == swatch.LicenseDestination {
			data, err := os.ReadFile(fullPath)
			if err == nil && hasUnresolvedPlaceholders(data) {
				warning = append(warning, HealthResult{
					Path:   p,
					Status: Warning,
					Detail: "(contains unresolved placeholders)",
				})
				continue
			}
		}
		present = append(present, HealthResult{Path: p, Status: Present})
	}

	// README.md is a local diagnostic, not a swatch. Warn when absent.
	if !fsutil.FileExists(filepath.Join(dir, readmeFile)) {
		warning = append(warning, HealthResult{
			Path:   readmeFile,
			Status: Warning,
			Detail: "(not managed by tailor)",
		})
	}

	sortByPath := func(a, b HealthResult) int {
		return cmp.Compare(a.Path, b.Path)
	}
	slices.SortFunc(missing, sortByPath)
	slices.SortFunc(warning, sortByPath)
	slices.SortFunc(present, sortByPath)

	return slices.Concat(missing, warning, present)
}
