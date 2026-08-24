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
	"github.com/wimpysworld/tailor/internal/model"
)

// RepoSettingCategory classifies the outcome of processing a single repository setting.
type RepoSettingCategory string

const (
	WouldSet       RepoSettingCategory = "would set"
	RepoNoChange   RepoSettingCategory = "no change"
	WouldSkipScope RepoSettingCategory = "would skip (insufficient scope)"
)

// RepoSettingResult records the field name, category, and display value for one
// repository setting. Annotation carries optional context for skip categories,
// embedded in the label (e.g. "token missing required scope").
type RepoSettingResult struct {
	Section    string
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
	skipResults, skippedFields := readWarningsToResults(warnings, cfg.Repository, live)

	if cfg.Repository.AutomatedSecurityFixesEnabled != nil &&
		*cfg.Repository.AutomatedSecurityFixesEnabled &&
		(cfg.Repository.VulnerabilityAlertsEnabled == nil || !*cfg.Repository.VulnerabilityAlertsEnabled) &&
		live.VulnerabilityAlertsEnabled != nil &&
		!*live.VulnerabilityAlertsEnabled {
		fmt.Fprintln(os.Stderr, "warning: automated_security_fixes_enabled is true but vulnerability alerts are disabled on GitHub; enable vulnerability_alerts_enabled first")
	}

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
		applyResult, err := gh.ApplyRepoSettingsWithCurrent(client, owner, name, settingsForApply(cfg.Repository, results), live)
		if err != nil {
			return nil, err
		}
		results = append(results, skippedToResults(applyResult)...)
	}

	return results, nil
}

func settingsForApply(declared *model.RepositorySettings, results []RepoSettingResult) *model.RepositorySettings {
	changed := make(map[string]bool)
	for _, result := range results {
		if result.Category == WouldSet {
			changed[result.Field] = true
		}
	}
	apply := *declared
	if !changed["private_vulnerability_reporting_enabled"] {
		apply.PrivateVulnerabilityReportEnabled = nil
	}
	if !changed["vulnerability_alerts_enabled"] {
		apply.VulnerabilityAlertsEnabled = nil
	}
	if !changed["automated_security_fixes_enabled"] {
		apply.AutomatedSecurityFixesEnabled = nil
	}
	if changed["vulnerability_alerts_enabled"] &&
		declared.VulnerabilityAlertsEnabled != nil && !*declared.VulnerabilityAlertsEnabled &&
		declared.AutomatedSecurityFixesEnabled != nil && !*declared.AutomatedSecurityFixesEnabled {
		apply.AutomatedSecurityFixesEnabled = declared.AutomatedSecurityFixesEnabled
	}
	return &apply
}

// skippedToResults converts gh.ApplyResult skipped operations into
// RepoSettingResult entries with WouldSkipScope categories.
func skippedToResults(ar *gh.ApplyResult) []RepoSettingResult {
	if ar == nil {
		return nil
	}
	var results []RepoSettingResult
	for _, sk := range ar.Skipped {
		results = append(results, RepoSettingResult{
			Field:      sk.Operation,
			Category:   WouldSkipScope,
			Annotation: skipAnnotation,
		})
	}
	return results
}

// compareSettings iterates non-nil pointer fields in declared and compares
// each against the corresponding field in live. Returns a result per declared field.
func compareSettings(declared, live *model.RepositorySettings) []RepoSettingResult {
	var results []RepoSettingResult

	liveFields := model.RepositorySettingFields(live)
	for _, field := range model.RepositorySettingFields(declared) {
		if !field.Set {
			continue
		}

		dfv := field.Value
		declaredVal := dfv.Elem().Interface()

		var displayVal string
		var equal bool

		lfv := liveFields[field.Index].Value

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

		category := WouldSet
		if equal {
			category = RepoNoChange
		}
		results = append(results, RepoSettingResult{
			Field:    field.YAMLKey,
			Category: category,
			Value:    displayVal,
		})
	}

	return results
}

// readWarningOperationFields maps read-path operation names from
// ErrInsufficientScope to the config field names they affect.
var readWarningOperationFields = map[string][]string{
	"fetch vulnerability alerts":            {"vulnerability_alerts_enabled"},
	"fetch automated security fixes":        {"automated_security_fixes_enabled"},
	"fetch private vulnerability reporting": {"private_vulnerability_reporting_enabled"},
	"fetch workflow permissions":            {"default_workflow_permissions", "can_approve_pull_request_reviews"},
}

// readWarningsToResults converts read-path access-error warnings into
// RepoSettingResult entries with the appropriate skip category. Only fields
// that the user declared in their config produce results. Undeclared fields
// are silently ignored. It also returns a set of field names that should be
// suppressed from compareSettings output (because their nil live value is due
// to a 403, not a real diff).
func readWarningsToResults(warnings []error, declared, live *model.RepositorySettings) ([]RepoSettingResult, map[string]bool) {
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

		for _, f := range fields {
			if !declaredFields[f] {
				continue
			}
			if skippedFields[f] {
				continue
			}
			skippedFields[f] = true
			results = append(results, RepoSettingResult{
				Field:      f,
				Category:   WouldSkipScope,
				Annotation: skipAnnotation,
			})
		}

		dependent := ""
		switch {
		case op == "fetch vulnerability alerts" && declared.AutomatedSecurityFixesEnabled != nil && *declared.AutomatedSecurityFixesEnabled &&
			live.AutomatedSecurityFixesEnabled != nil && !*live.AutomatedSecurityFixesEnabled:
			dependent = "automated_security_fixes_enabled"
		case op == "fetch automated security fixes" && declared.VulnerabilityAlertsEnabled != nil && !*declared.VulnerabilityAlertsEnabled &&
			live.VulnerabilityAlertsEnabled != nil && *live.VulnerabilityAlertsEnabled:
			dependent = "vulnerability_alerts_enabled"
		}
		if dependent != "" && !skippedFields[dependent] {
			skippedFields[dependent] = true
			results = append(results, RepoSettingResult{
				Field:      dependent,
				Category:   WouldSkipScope,
				Annotation: skipAnnotation,
			})
		}
	}

	return results, skippedFields
}

// declaredFieldNames returns the set of YAML field names that have non-nil
// values in the given RepositorySettings.
func declaredFieldNames(s *model.RepositorySettings) map[string]bool {
	if s == nil {
		return nil
	}
	names := make(map[string]bool)
	for _, field := range model.RepositorySettingFields(s) {
		if field.Set {
			names[field.YAMLKey] = true
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
