package alter

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/config"
	"github.com/wimpysworld/tailor/internal/gh"
	"github.com/wimpysworld/tailor/internal/model"
)

// RepoSettingCategory classifies the outcome of processing a single repository setting.
type RepoSettingCategory string

const (
	// WouldSet marks a declared setting that needs a write.
	WouldSet RepoSettingCategory = "would set"
	// RepoNoChange marks a setting that already matches its declaration.
	RepoNoChange RepoSettingCategory = "no change"
	// WouldSkipScope marks a setting or operation blocked by insufficient access.
	WouldSkipScope RepoSettingCategory = "would skip (insufficient scope)"
)

// RepoSettingResult records the field name, category, and display value for one
// repository setting. Skip results for write operations leave Field empty and
// carry the skipped Operation instead. Annotation carries optional context for
// skipped operations or repeated enable requests.
type RepoSettingResult struct {
	Section    string
	Field      string
	Category   RepoSettingCategory
	Value      string
	Before     string
	Annotation string
	Operation  gh.Operation
}

// ProcessRepoSettings compares declared repository settings and applies changes outside DryRun.
func ProcessRepoSettings(cfg *config.Config, mode ApplyMode, target RepoTarget) ([]RepoSettingResult, error) {
	if cfg.Repository == nil {
		return nil, nil
	}
	if pagesEnabled(cfg) && !cfg.HomepageDeclared() && cfg.Repository.Homepage != nil {
		copyConfig := *cfg
		copyRepository := *cfg.Repository
		copyRepository.Homepage = nil
		copyConfig.Repository = &copyRepository
		cfg = &copyConfig
	}

	if target.missingRepo("Repository settings") {
		return nil, nil
	}

	live, warnings, err := gh.ReadRepoSettings(target.Client, target.Owner, target.Name)
	if err != nil {
		return nil, err
	}
	if target.wikiEnabled {
		live.HasWiki = new(true)
	}

	if cfg.Repository.AutomatedSecurityFixesEnabled != nil &&
		*cfg.Repository.AutomatedSecurityFixesEnabled &&
		(cfg.Repository.VulnerabilityAlertsEnabled == nil || !*cfg.Repository.VulnerabilityAlertsEnabled) &&
		live.VulnerabilityAlertsEnabled != nil &&
		!*live.VulnerabilityAlertsEnabled {
		fmt.Fprintln(target.stderr(), "warning: automated_security_fixes_enabled is true but vulnerability alerts are disabled on GitHub; enable vulnerability_alerts_enabled first")
	}

	results := readWarningsToResults(compareSettings(cfg.Repository, live), warnings, cfg.Repository, live)

	if mode.ShouldWrite() && hasChanges(results) {
		applyResult, err := gh.ApplyRepoSettings(target.Client, target.Owner, target.Name, changedSettings(cfg.Repository, results, model.RepositorySettingFields), live)
		if err != nil {
			return nil, err
		}
		results = append(results, skippedToResults(applyResult)...)
	}

	return results, nil
}

// changedSettings copies the fields of declared whose result is WouldSet into
// a new settings value, so a write carries only actionable fields.
func changedSettings[T any](declared *T, results []RepoSettingResult, fields func(*T) []model.SettingField) *T {
	changed := make(map[string]bool)
	for _, result := range results {
		if result.Category == WouldSet {
			changed[result.Field] = true
		}
	}

	apply := new(T)
	applyValue := reflect.ValueOf(apply).Elem()
	for _, field := range fields(declared) {
		if changed[field.YAMLKey] {
			applyValue.Field(field.Index).Set(field.Value)
		}
	}
	return apply
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
			Operation:  sk.Operation,
			Category:   WouldSkipScope,
			Annotation: skipAnnotation,
		})
	}
	return results
}

// compareSettings returns one result per declared field, including repeated vulnerability-alert enable requests for Dependency Graph.
func compareSettings(declared, live *model.RepositorySettings) []RepoSettingResult {
	var results []RepoSettingResult

	liveFields := model.RepositorySettingFields(live)
	for _, field := range model.RepositorySettingFields(declared) {
		if !field.Set {
			continue
		}

		dfv := field.Value
		declaredVal := dfv.Elem().Interface()

		var displayVal, beforeVal string
		var equal bool

		lfv := liveFields[field.Index].Value

		if dfv.Elem().Kind() == reflect.Slice {
			dSlice := dfv.Elem().Interface().([]string)
			displayVal = strings.Join(dSlice, ", ")
			if !lfv.IsNil() {
				lSlice := lfv.Elem().Interface().([]string)
				beforeVal = strings.Join(lSlice, ", ")
				equal = equalStringSets(dSlice, lSlice)
				if equal {
					beforeVal = displayVal
				}
			}
		} else {
			displayVal = fmt.Sprintf("%v", declaredVal)
			if !lfv.IsNil() {
				beforeVal = fmt.Sprintf("%v", lfv.Elem().Interface())
			}
			equal = !lfv.IsNil() && lfv.Elem().Interface() == declaredVal
		}

		category := WouldSet
		annotation := ""
		if equal {
			category = RepoNoChange
			if field.YAMLKey == "vulnerability_alerts_enabled" && declaredVal == true {
				category = WouldSet
				annotation = "reapply enable request for Dependency Graph"
			}
		}
		results = append(results, RepoSettingResult{
			Field:      field.YAMLKey,
			Category:   category,
			Value:      displayVal,
			Before:     beforeVal,
			Annotation: annotation,
		})
	}

	return results
}

// equalStringSets compares sorted copies without changing either input.
// GitHub ignores list order for topics and patterns_allowed. Duplicate counts must still match.
func equalStringSets(a, b []string) bool {
	as := slices.Clone(a)
	bs := slices.Clone(b)
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}

// readWarningOperationFields maps read-path operation kinds from
// ErrInsufficientScope to the config field names they affect.
var readWarningOperationFields = map[gh.OperationKind][]string{
	gh.OpFetchVulnerabilityAlerts:           {"vulnerability_alerts_enabled"},
	gh.OpFetchAutomatedSecurityFixes:        {"automated_security_fixes_enabled"},
	gh.OpFetchPrivateVulnerabilityReporting: {"private_vulnerability_reporting_enabled"},
	gh.OpFetchWorkflowPermissions:           {"default_workflow_permissions", "can_approve_pull_request_reviews"},
	gh.OpFetchSecurityAnalysis:              {"secret_scanning", "secret_scanning_push_protection", "secret_scanning_non_provider_patterns"},
}

// readWarningsToResults skips declared fields and dependent changes when an access error prevents a safe comparison.
// An unreadable live value is not evidence that a setting differs. Undeclared fields produce no results.
func readWarningsToResults(results []RepoSettingResult, warnings []error, declared, live *model.RepositorySettings) []RepoSettingResult {
	for _, w := range warnings {
		op := warningOperation(w)
		fields, ok := readWarningOperationFields[op]
		if !ok {
			continue
		}

		for _, f := range fields {
			results = replaceWithScopeSkip(results, "", f)
		}

		dependent := ""
		switch {
		case op == gh.OpFetchVulnerabilityAlerts && declared.AutomatedSecurityFixesEnabled != nil && *declared.AutomatedSecurityFixesEnabled &&
			live.AutomatedSecurityFixesEnabled != nil && !*live.AutomatedSecurityFixesEnabled:
			dependent = "automated_security_fixes_enabled"
		case op == gh.OpFetchAutomatedSecurityFixes && declared.VulnerabilityAlertsEnabled != nil && !*declared.VulnerabilityAlertsEnabled &&
			live.VulnerabilityAlertsEnabled != nil && *live.VulnerabilityAlertsEnabled:
			dependent = "vulnerability_alerts_enabled"
		}
		if dependent != "" {
			results = replaceWithScopeSkip(results, "", dependent)
		}
	}

	return results
}

// warningOperation extracts the operation kind from a read-path warning.
func warningOperation(err error) gh.OperationKind {
	if scopeErr, ok := errors.AsType[*gh.ErrInsufficientScope](err); ok {
		return scopeErr.Operation.Kind
	}
	return gh.OpNone
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
