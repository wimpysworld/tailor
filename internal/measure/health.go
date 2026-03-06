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

// HealthResult pairs a destination path with its on-disk status.
type HealthResult struct {
	Destination string
	Status      HealthStatus
}

// CheckHealth checks whether each health swatch destination and the LICENSE
// file exist in dir. Returns results sorted lexicographically by destination
// within each status group (missing first, then present).
func CheckHealth(dir string) []HealthResult {
	healthSwatches := swatch.HealthSwatches()
	destinations := make([]string, 0, len(healthSwatches)+1)
	for _, s := range healthSwatches {
		destinations = append(destinations, s.Destination)
	}
	destinations = append(destinations, swatch.LicenseDestination)

	var missing, present []HealthResult
	for _, dest := range destinations {
		path := filepath.Join(dir, dest)
		if fsutil.FileExists(path) {
			present = append(present, HealthResult{Destination: dest, Status: Present})
		} else {
			missing = append(missing, HealthResult{Destination: dest, Status: Missing})
		}
	}

	slices.SortFunc(missing, func(a, b HealthResult) int {
		return cmp.Compare(a.Destination, b.Destination)
	})
	slices.SortFunc(present, func(a, b HealthResult) int {
		return cmp.Compare(a.Destination, b.Destination)
	})

	return append(missing, present...)
}
