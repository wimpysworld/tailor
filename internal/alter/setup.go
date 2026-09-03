package alter

import (
	"errors"
	"slices"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
)

// WouldSkipSetup marks a code scanning or Code Quality field that Tailor
// skipped because the feature is not available to the repository or a setup
// run is in progress. Annotation carries the reason.
const WouldSkipSetup RepoSettingCategory = "would skip"

// setupStringResult compares one declared string field against its live
// value. It reports false when the field is not declared.
func setupStringResult(section, field string, declared, live *string) (RepoSettingResult, bool) {
	if declared == nil {
		return RepoSettingResult{}, false
	}
	category := WouldSet
	if live != nil && *live == *declared {
		category = RepoNoChange
	}
	return RepoSettingResult{Section: section, Field: field, Category: category, Value: *declared}, true
}

// setupLanguagesResult compares a declared language list against the live
// list as a set. An empty declared list means GitHub detects the languages,
// so it produces no result.
func setupLanguagesResult(section string, declared, live *[]string) (RepoSettingResult, bool) {
	if declared == nil || len(*declared) == 0 {
		return RepoSettingResult{}, false
	}
	desired := slices.Clone(*declared)
	slices.Sort(desired)
	category := WouldSet
	if live != nil && equalStringSets(desired, *live) {
		category = RepoNoChange
	}
	return RepoSettingResult{Section: section, Field: "languages", Category: category, Value: strings.Join(desired, ", ")}, true
}

// setupSkipResult returns the skip result for one field and reason.
func setupSkipResult(result RepoSettingResult, reason gh.SetupSkipReason) RepoSettingResult {
	return RepoSettingResult{
		Section:    result.Section,
		Field:      result.Field,
		Category:   WouldSkipSetup,
		Annotation: string(reason),
	}
}

// setupSkipResults replaces every result with a skip result for the reason.
func setupSkipResults(results []RepoSettingResult, reason gh.SetupSkipReason) []RepoSettingResult {
	skipped := make([]RepoSettingResult, 0, len(results))
	for _, result := range results {
		skipped = append(skipped, setupSkipResult(result, reason))
	}
	return skipped
}

// applySetup runs the write and reports its outcome. When the write is
// skipped, every WouldSet result becomes a skip result and the command
// continues. Other errors stop the command.
func applySetup(results []RepoSettingResult, write func() error) ([]RepoSettingResult, error) {
	err := write()
	var skipped *gh.ErrSetupSkipped
	if !errors.As(err, &skipped) {
		return results, err
	}
	applied := make([]RepoSettingResult, 0, len(results))
	for _, result := range results {
		if result.Category == WouldSet {
			result = setupSkipResult(result, skipped.Reason)
		}
		applied = append(applied, result)
	}
	return applied, nil
}
