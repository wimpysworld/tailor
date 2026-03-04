package measure

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/wimpysworld/tailor/internal/swatch"
)

// healthResults implements sort.Interface, ordering by Destination.
type healthResults []HealthResult

func (h healthResults) Len() int           { return len(h) }
func (h healthResults) Less(i, j int) bool { return h[i].Destination < h[j].Destination }
func (h healthResults) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// HealthStatus indicates whether a health file is present or missing.
type HealthStatus string

const (
	Missing HealthStatus = "missing"
	Present HealthStatus = "present"
)

// HealthResult pairs a destination path with its on-disk status.
type HealthResult struct {
	Destination string
	Status      HealthStatus
}

// CheckHealth checks whether each health swatch destination and the LICENSE
// file exist in dir. Returns results sorted lexicographically by destination
// within each status group (missing first, then present).
func CheckHealth(dir string) []HealthResult {
	var destinations []string
	for _, s := range swatch.HealthSwatches() {
		destinations = append(destinations, s.Destination)
	}
	destinations = append(destinations, swatch.LicenseDestination)

	var missing, present []HealthResult
	for _, dest := range destinations {
		path := filepath.Join(dir, dest)
		if fileExists(path) {
			present = append(present, HealthResult{Destination: dest, Status: Present})
		} else {
			missing = append(missing, HealthResult{Destination: dest, Status: Missing})
		}
	}

	sort.Sort(healthResults(missing))
	sort.Sort(healthResults(present))

	return append(missing, present...)
}

// fileExists reports whether the given path exists as a file (not a directory).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
