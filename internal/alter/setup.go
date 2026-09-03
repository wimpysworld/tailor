package alter

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
)

// WouldSkipSetup marks a code scanning or Code Quality field that Tailor
// skipped because the feature is not available to the repository or a setup
// run is in progress. Annotation carries the reason.
const WouldSkipSetup RepoSettingCategory = "would skip"

// resultComparer collects one result per declared field of one config
// section. A nil live value means the field is absent, so it would be set.
type resultComparer struct {
	section string
	results []RepoSettingResult
}

func (c *resultComparer) add(field, value string, equal bool) {
	category := WouldSet
	if equal {
		category = RepoNoChange
	}
	c.results = append(c.results, RepoSettingResult{Section: c.section, Field: field, Category: category, Value: value})
}

func (c *resultComparer) str(field string, declared, live *string) {
	if declared != nil {
		c.add(field, *declared, live != nil && *live == *declared)
	}
}

func (c *resultComparer) boolean(field string, declared, live *bool) {
	if declared != nil {
		c.add(field, strconv.FormatBool(*declared), live != nil && *live == *declared)
	}
}

func (c *resultComparer) enabled(field string, declared, live *bool) {
	if declared != nil {
		c.add(field, enabledText(*declared), live != nil && *live == *declared)
	}
}

func (c *resultComparer) count(field string, declared, live *int) {
	if declared != nil {
		c.add(field, strconv.Itoa(*declared), live != nil && *live == *declared)
	}
}

// set compares two lists as sets and renders the declared list joined by
// ", ", or "(none)" when empty.
func (c *resultComparer) set(field string, declared, live *[]string) {
	if declared != nil {
		c.add(field, listText(*declared), live != nil && equalStringSets(*declared, *live))
	}
}

// languages compares a declared language list against the live list as a
// set and renders the declared list sorted. An empty declared list means
// GitHub detects the languages, so it produces no result.
func (c *resultComparer) languages(declared, live *[]string) {
	if declared == nil || len(*declared) == 0 {
		return
	}
	desired := slices.Clone(*declared)
	slices.Sort(desired)
	c.add("languages", strings.Join(desired, ", "), live != nil && equalStringSets(desired, *live))
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func listText(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

// skipResult returns the skip result for one field, keeping its section
// and name and carrying the category and annotation.
func skipResult(result RepoSettingResult, category RepoSettingCategory, annotation string) RepoSettingResult {
	return RepoSettingResult{
		Section:    result.Section,
		Field:      result.Field,
		Category:   category,
		Annotation: annotation,
	}
}

// skipResults replaces every result with a skip result for the category
// and annotation.
func skipResults(results []RepoSettingResult, category RepoSettingCategory, annotation string) []RepoSettingResult {
	skipped := make([]RepoSettingResult, 0, len(results))
	for _, result := range results {
		skipped = append(skipped, skipResult(result, category, annotation))
	}
	return skipped
}

// processSetup is the shared skeleton of the ruleset, code scanning, and
// Code Quality stages. declared holds the results against an empty live
// state, so a read that is skipped can report every declared field. read
// fetches the live state and compares it. write sends the change. A read
// that returns *gh.ErrSetupSkipped or *gh.ErrInsufficientScope turns every
// declared field into a skip result and the command continues.
func processSetup(declared []RepoSettingResult, mode ApplyMode, read func() ([]RepoSettingResult, error), write func(results []RepoSettingResult) error) ([]RepoSettingResult, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	results, err := read()
	var skipped *gh.ErrSetupSkipped
	if errors.As(err, &skipped) {
		return skipResults(declared, WouldSkipSetup, string(skipped.Reason)), nil
	}
	var scope *gh.ErrInsufficientScope
	if errors.As(err, &scope) {
		return skipResults(declared, WouldSkipScope, skipAnnotation), nil
	}
	if err != nil {
		return nil, err
	}
	if !mode.ShouldWrite() || !hasChanges(results) {
		return results, nil
	}
	return applySetup(results, func() error { return write(results) })
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
			result = skipResult(result, WouldSkipSetup, string(skipped.Reason))
		}
		applied = append(applied, result)
	}
	return applied, nil
}
