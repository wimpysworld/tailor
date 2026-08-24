package measure

import (
	"bytes"
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/fsutil"
	"github.com/wimpysworld/tailor/internal/swatch"
)

// HealthStatus is the on-disk status of a health file: missing, warning,
// or present.
type HealthStatus string

// Health statuses, in output order.
const (
	Missing HealthStatus = "missing"
	Warning HealthStatus = "warning"
	Present HealthStatus = "present"
)

// HealthResult pairs a path with its on-disk status and optional detail.
type HealthResult struct {
	Path   string
	Status HealthStatus
	Detail string
}

var (
	placeholderWhitespaceRe = regexp.MustCompile(`[ \t\n\v\f\r]+`)
	placeholderNames        = map[string]struct{}{
		"year":                     {},
		"yyyy":                     {},
		"fullname":                 {},
		"name of copyright owner":  {},
		"name of copyright holder": {},
		"software name":            {},
		"project":                  {},
		"projecturl":               {},
		"email":                    {},
	}
)

// hasCompleteInlineLink reports whether offset starts a balanced Markdown
// inline-link destination. Backslashes escape the following byte.
func hasCompleteInlineLink(data []byte, offset int) bool {
	if offset >= len(data) || data[offset] != '(' {
		return false
	}

	depth := 0
	for i := offset; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// hasUnresolvedPlaceholders reports whether data contains a known unresolved
// token inside matching square or curly delimiters.
func hasUnresolvedPlaceholders(data []byte) bool {
	for start, open := range data {
		var closer byte
		switch open {
		case '[':
			closer = ']'
		case '{':
			closer = '}'
		default:
			continue
		}

		end := bytes.IndexByte(data[start+1:], closer)
		if end < 0 {
			continue
		}
		end += start + 1
		if open == '[' && hasCompleteInlineLink(data, end+1) {
			continue
		}

		name := placeholderWhitespaceRe.ReplaceAllString(string(data[start+1:end]), " ")
		name = strings.ToLower(strings.Trim(name, " "))
		if _, ok := placeholderNames[name]; ok {
			return true
		}
	}
	return false
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
