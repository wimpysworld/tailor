package alter

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
)

// RepoSettingCategory classifies the outcome of processing a single repository setting.
type RepoSettingCategory string

const (
	WouldSet       RepoSettingCategory = "would set"
	RepoNoChange   RepoSettingCategory = "no change"
	WouldSkipScope RepoSettingCategory = "would skip (insufficient scope)"
	WouldSkipRole  RepoSettingCategory = "would skip (insufficient role)"
)

// RepoSettingResult records the field name, category, and display value for one
// repository setting. Annotation carries optional context for skip categories,
// embedded in the label (e.g. "token missing required scope").
type RepoSettingResult struct {
	Field      string
	Category   RepoSettingCategory
	Value      string
	Annotation string
}

// ProcessRepoSettings compares declared settings against live settings
// and optionally applies them. Returns results for output formatting.
func ProcessRepoSettings(cfg *config.Config, mode ApplyMode, client *api.RESTClient, owner, name string, hasRepo bool) ([]RepoSettingResult, error) {
	if cfg.Repository == nil {
		return nil, nil
	}

	if !hasRepo {
		fmt.Fprintln(os.Stderr, "No GitHub repository context found. Repository settings will be applied once a remote is configured.")
		return nil, nil
	}

	live, _, err := gh.ReadRepoSettings(client, owner, name)
	if err != nil {
		return nil, err
	}

	results := compareSettings(cfg.Repository, live)

	if mode.ShouldWrite() && hasChanges(results) {
		applyResult, err := gh.ApplyRepoSettings(client, owner, name, cfg.Repository)
		if err != nil {
			return nil, err
		}
		results = append(results, skippedToResults(applyResult)...)
	}

	return results, nil
}

// skippedToResults converts gh.ApplyResult skipped operations into
// RepoSettingResult entries with WouldSkipScope or WouldSkipRole categories.
func skippedToResults(ar *gh.ApplyResult) []RepoSettingResult {
	if ar == nil {
		return nil
	}
	var results []RepoSettingResult
	for _, sk := range ar.Skipped {
		cat := classifySkipCategory(sk.Err)
		results = append(results, RepoSettingResult{
			Field:      sk.Operation,
			Category:   cat,
			Value:      sk.Err.Error(),
			Annotation: skipAnnotation(sk.Err),
		})
	}
	return results
}

// skipAnnotation extracts a short annotation string from a skip error.
// For ErrInsufficientRole it returns "<role> required"; for
// ErrInsufficientScope it returns "token missing required scope".
func skipAnnotation(err error) string {
	var roleErr *gh.ErrInsufficientRole
	if errors.As(err, &roleErr) {
		return roleErr.RequiredRole + " required"
	}
	return "token missing required scope"
}

// classifySkipCategory returns WouldSkipRole for ErrInsufficientRole and
// WouldSkipScope for ErrInsufficientScope (or any other access error).
func classifySkipCategory(err error) RepoSettingCategory {
	var roleErr *gh.ErrInsufficientRole
	if errors.As(err, &roleErr) {
		return WouldSkipRole
	}
	return WouldSkipScope
}

// compareSettings iterates non-nil pointer fields in declared and compares
// each against the corresponding field in live. Returns a result per declared field.
func compareSettings(declared, live *config.RepositorySettings) []RepoSettingResult {
	dv := reflect.ValueOf(declared).Elem()
	lv := reflect.ValueOf(live).Elem()
	dt := dv.Type()

	var results []RepoSettingResult

	for i := range dt.NumField() {
		field := dt.Field(i)
		tag := field.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")

		dfv := dv.Field(i)
		if dfv.Kind() != reflect.Ptr || dfv.IsNil() {
			continue
		}

		declaredVal := dfv.Elem().Interface()

		var displayVal string
		var equal bool

		lfv := lv.Field(i)

		if dfv.Elem().Kind() == reflect.Slice {
			dSlice := dfv.Elem().Interface().([]string)
			displayVal = strings.Join(dSlice, ", ")
			if !lfv.IsNil() {
				lSlice := lfv.Elem().Interface().([]string)
				equal = slices.Equal(dSlice, lSlice)
			}
		} else {
			displayVal = fmt.Sprintf("%v", declaredVal)
			equal = !lfv.IsNil() && lfv.Elem().Interface() == declaredVal
		}

		if equal {
			results = append(results, RepoSettingResult{
				Field:    key,
				Category: RepoNoChange,
				Value:    displayVal,
			})
		} else {
			results = append(results, RepoSettingResult{
				Field:    key,
				Category: WouldSet,
				Value:    displayVal,
			})
		}
	}

	return results
}

// hasChanges returns true if any result is WouldSet.
func hasChanges(results []RepoSettingResult) bool {
	for _, r := range results {
		if r.Category == WouldSet {
			return true
		}
	}
	return false
}
