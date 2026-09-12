package alter

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/wimpysworld/tailor/internal/gh"
)

// WouldSkipSetup marks a setting that Tailor cannot apply or must preserve.
// Annotation gives the access, availability, readiness or owner-policy reason.
const WouldSkipSetup RepoSettingCategory = "would skip"

// resultComparer collects one result per declared field of one config
// section. A nil live value means the field is absent, so it would be set.
type resultComparer struct {
	section string
	results []RepoSettingResult
}

func (c *resultComparer) add(field, value, before string, equal bool) {
	category := WouldSet
	if equal {
		category = RepoNoChange
	}
	c.results = append(c.results, RepoSettingResult{Section: c.section, Field: field, Category: category, Value: value, Before: before})
}

func (c *resultComparer) str(field string, declared, live *string) {
	if declared != nil {
		before := ""
		if live != nil {
			before = *live
		}
		c.add(field, *declared, before, live != nil && *live == *declared)
	}
}

func (c *resultComparer) boolean(field string, declared, live *bool) {
	if declared != nil {
		before := ""
		if live != nil {
			before = strconv.FormatBool(*live)
		}
		c.add(field, strconv.FormatBool(*declared), before, live != nil && *live == *declared)
	}
}

func (c *resultComparer) enabled(field string, declared, live *bool) {
	if declared != nil {
		before := ""
		if live != nil {
			before = enabledText(*live)
		}
		c.add(field, enabledText(*declared), before, live != nil && *live == *declared)
	}
}

func (c *resultComparer) count(field string, declared, live *int) {
	if declared != nil {
		before := ""
		if live != nil {
			before = strconv.Itoa(*live)
		}
		c.add(field, strconv.Itoa(*declared), before, live != nil && *live == *declared)
	}
}

// set compares two lists as sets and renders the declared list joined by
// ", ", or "(none)" when empty.
func (c *resultComparer) set(field string, declared, live *[]string) {
	if declared != nil {
		before := ""
		if live != nil {
			before = listText(*live)
		}
		value := listText(*declared)
		equal := live != nil && equalStringSets(*declared, *live)
		if equal {
			before = value
		}
		c.add(field, value, before, equal)
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
	before := ""
	if live != nil {
		current := slices.Clone(*live)
		slices.Sort(current)
		before = listText(current)
	}
	c.add("languages", strings.Join(desired, ", "), before, live != nil && equalStringSets(desired, *live))
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

// processSetup shares comparison and write handling for rulesets, code scanning and Code Quality.
// declared contains comparisons against an empty state, so skipped reads can report each declared field.
// Reads that return *gh.ErrSetupSkipped or *gh.ErrInsufficientScope produce skip results without stopping the command.
func processSetup(declared []RepoSettingResult, mode ApplyMode, read func() ([]RepoSettingResult, error), write func(results []RepoSettingResult) error) ([]RepoSettingResult, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	results, err := read()
	if skipped, ok := errors.AsType[*gh.ErrSetupSkipped](err); ok {
		return skipResults(declared, WouldSkipSetup, string(skipped.Reason)), nil
	}
	if _, ok := errors.AsType[*gh.ErrInsufficientScope](err); ok {
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
		if err != nil {
			results = slices.DeleteFunc(results, func(result RepoSettingResult) bool {
				return result.Category == WouldSet
			})
		}
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
