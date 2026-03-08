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

	live, warnings, err := gh.ReadRepoSettings(client, owner, name)
	if err != nil {
		return nil, err
	}

	// Convert read-path warnings into skip results and collect the affected
	// field names so the corresponding WouldSet entries can be suppressed.
	skipResults, skippedFields := readWarningsToResults(warnings, cfg.Repository)

	results := compareSettings(cfg.Repository, live)

	// Remove false-positive WouldSet entries for fields whose live value is
	// nil only because the read returned a 403.
	if len(skippedFields) > 0 {
		filtered := results[:0]
		for _, r := range results {
			if skippedFields[r.Field] {
				continue
			}
			filtered = append(filtered, r)
		}
		results = filtered
	}

	results = append(results, skipResults...)

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

// readWarningOperationFields maps read-path operation names (from
// ErrInsufficientScope/ErrInsufficientRole) to the config field names
// (YAML tags) they affect. Workflow permissions covers two fields.
var readWarningOperationFields = map[string][]string{
	"fetch vulnerability alerts":            {"vulnerability_alerts_enabled"},
	"fetch automated security fixes":        {"automated_security_fixes_enabled"},
	"fetch private vulnerability reporting": {"private_vulnerability_reporting_enabled"},
	"fetch workflow permissions":            {"default_workflow_permissions", "can_approve_pull_request_reviews"},
}

// readWarningsToResults converts read-path access-error warnings into
// RepoSettingResult entries with the appropriate skip category. Only fields
// that the user declared in their config produce results - undeclared fields
// are silently ignored. It also returns a set of field names that should be
// suppressed from compareSettings output (because their nil live value is due
// to a 403, not a real diff).
func readWarningsToResults(warnings []error, declared *config.RepositorySettings) ([]RepoSettingResult, map[string]bool) {
	if len(warnings) == 0 {
		return nil, nil
	}

	declaredFields := declaredFieldNames(declared)

	var results []RepoSettingResult
	skippedFields := make(map[string]bool)

	for _, w := range warnings {
		op := warningOperation(w)
		fields, ok := readWarningOperationFields[op]
		if !ok {
			continue
		}

		cat := classifySkipCategory(w)
		ann := skipAnnotation(w)

		for _, f := range fields {
			if !declaredFields[f] {
				continue
			}
			skippedFields[f] = true
			results = append(results, RepoSettingResult{
				Field:      f,
				Category:   cat,
				Value:      w.Error(),
				Annotation: ann,
			})
		}
	}

	return results, skippedFields
}

// declaredFieldNames returns the set of YAML field names that have non-nil
// values in the given RepositorySettings.
func declaredFieldNames(s *config.RepositorySettings) map[string]bool {
	if s == nil {
		return nil
	}
	rv := reflect.ValueOf(s).Elem()
	rt := rv.Type()
	names := make(map[string]bool)
	for i := range rt.NumField() {
		f := rt.Field(i)
		tag := f.Tag.Get("yaml")
		if tag == "" || tag == ",inline" {
			continue
		}
		key, _, _ := strings.Cut(tag, ",")
		fv := rv.Field(i)
		if fv.Kind() == reflect.Ptr && !fv.IsNil() {
			names[key] = true
		}
	}
	return names
}

// warningOperation extracts the Operation field from a read-path warning.
func warningOperation(err error) string {
	var scopeErr *gh.ErrInsufficientScope
	if errors.As(err, &scopeErr) {
		return scopeErr.Operation
	}
	var roleErr *gh.ErrInsufficientRole
	if errors.As(err, &roleErr) {
		return roleErr.Operation
	}
	return ""
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
