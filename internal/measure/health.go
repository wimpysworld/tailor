package measure

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"

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

// completeInlineLinks returns, for each '(' in data, whether it starts a
// balanced Markdown inline-link destination. Backslashes escape the
// following byte.
func completeInlineLinks(data []byte) []bool {
	complete := make([]bool, len(data))
	var open []int
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
		case '(':
			open = append(open, i)
		case ')':
			if n := len(open); n > 0 {
				complete[open[n-1]] = true
				open = open[:n-1]
			}
		}
	}
	return complete
}

// hasUnresolvedPlaceholders reports whether data contains a known unresolved
// token inside matching square or curly delimiters. A single forward pass
// pairs each closing delimiter with the nearest unconsumed opener of its
// kind; an earlier opener cannot match because its content contains another
// opening delimiter, which no placeholder name allows.
func hasUnresolvedPlaceholders(data []byte) bool {
	inlineLinks := completeInlineLinks(data)
	lastOpen := map[byte]int{'[': -1, '{': -1}
	for end, b := range data {
		switch b {
		case '[', '{':
			lastOpen[b] = end
		case ']', '}':
			opener := byte('[')
			if b == '}' {
				opener = '{'
			}
			start := lastOpen[opener]
			if start < 0 {
				continue
			}
			lastOpen[opener] = -1
			if opener == '[' && end+1 < len(data) && inlineLinks[end+1] {
				continue
			}

			name := placeholderWhitespaceRe.ReplaceAllString(string(data[start+1:end]), " ")
			name = strings.ToLower(strings.Trim(name, " "))
			if _, ok := placeholderNames[name]; ok {
				return true
			}
		}
	}
	return false
}

// readmeFile is the exact filename checked as a local health diagnostic.
const readmeFile = "README.md"

// maxLicenceSize caps the licence placeholder read, matching the
// .tailor.yml limit in the config package.
const maxLicenceSize = 1 << 20

// readLicence reads the licence file at path inside root for the
// placeholder check. It returns an error when the file is not regular or
// exceeds maxLicenceSize.
func readLicence(root *os.Root, path string) ([]byte, error) {
	file, err := root.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening licence: %w", err)
	}
	defer file.Close()

	// Check the open handle: the path can change to a non-regular file
	// between the caller's Lstat and Open. The rooted open also refuses a
	// swap to a symlink that resolves outside the project.
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("reading licence metadata: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("licence is not a regular file")
	}
	if info.Size() > maxLicenceSize {
		return nil, errors.New("licence exceeds maximum size of 1 MiB")
	}

	data, err := io.ReadAll(io.LimitReader(file, maxLicenceSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading licence: %w", err)
	}
	if len(data) > maxLicenceSize {
		return nil, errors.New("licence exceeds maximum size of 1 MiB")
	}
	return data, nil
}

// isRegularFile reports whether path is a regular file inside root. The
// leaf is not followed, so a symlink is never a regular file, and rooted
// resolution rejects parent symlinks that leave the root.
func isRegularFile(root *os.Root, path string) bool {
	if root == nil {
		return false
	}
	info, err := root.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}

// CheckHealth checks whether each health swatch path, the LICENSE file, and
// README.md exist in dir as regular files; symlinks, paths whose parents
// resolve outside dir, and other non-regular files count as absent. LICENSE
// files containing unresolved placeholder tokens are reported as warnings
// rather than present. A missing README.md is reported as a warning. Returns
// results sorted lexicographically by path within each status group
// (missing, warning, present).
func CheckHealth(dir string) []HealthResult {
	healthSwatches := swatch.HealthSwatches()
	paths := make([]string, 0, len(healthSwatches)+1)
	for _, s := range healthSwatches {
		paths = append(paths, s.Path)
	}
	paths = append(paths, swatch.LicenseDestination)

	// Rooted access confines path resolution to dir, so a symlinked parent
	// such as .github cannot report files outside the project as present.
	// When dir cannot be opened, every path counts as absent.
	root, err := os.OpenRoot(dir)
	if err == nil {
		defer root.Close()
	}

	var missing, warning, present []HealthResult
	for _, p := range paths {
		if !isRegularFile(root, p) {
			missing = append(missing, HealthResult{Path: p, Status: Missing})
			continue
		}
		if p == swatch.LicenseDestination {
			data, err := readLicence(root, p)
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
	if !isRegularFile(root, readmeFile) {
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
