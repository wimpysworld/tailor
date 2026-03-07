package measure

import (
	"cmp"
	"path/filepath"
	"slices"

	"github.com/wimpysworld/tailor/internal/fsutil"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// HealthStatus indicates whether a health file is present or missing.
type HealthStatus string

const (
	Missing HealthStatus = "missing"
	Present HealthStatus = "present"
)

// Label returns the status string with a trailing colon, suitable for formatted output.
func (s HealthStatus) Label() string { return string(s) + ":" }

// HealthResult pairs a path with its on-disk status.
type HealthResult struct {
	Path   string
	Status HealthStatus
}

// CheckHealth checks whether each health swatch path and the LICENSE file
// exist in dir. Returns results sorted lexicographically by path within each
// status group (missing first, then present).
func CheckHealth(dir string) []HealthResult {
	healthSwatches := swatch.HealthSwatches()
	paths := make([]string, 0, len(healthSwatches)+1)
	for _, s := range healthSwatches {
		paths = append(paths, s.Path)
	}
	paths = append(paths, swatch.LicenseDestination)

	var missing, present []HealthResult
	for _, p := range paths {
		fullPath := filepath.Join(dir, p)
		if fsutil.FileExists(fullPath) {
			present = append(present, HealthResult{Path: p, Status: Present})
		} else {
			missing = append(missing, HealthResult{Path: p, Status: Missing})
		}
	}

	slices.SortFunc(missing, func(a, b HealthResult) int {
		return cmp.Compare(a.Path, b.Path)
	})
	slices.SortFunc(present, func(a, b HealthResult) int {
		return cmp.Compare(a.Path, b.Path)
	})

	return append(missing, present...)
}
