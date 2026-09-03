package alter

import "slices"

const skipAnnotation = "token missing required scope"

// replaceWithScopeSkip removes the result for field, when present, and appends
// a WouldSkipScope result for it. A field with no result is left alone, so an
// undeclared field never produces a skip result.
func replaceWithScopeSkip(results []RepoSettingResult, section, field string) []RepoSettingResult {
	index := slices.IndexFunc(results, func(result RepoSettingResult) bool {
		return result.Field == field
	})
	if index == -1 {
		return results
	}
	results = slices.Delete(results, index, index+1)
	return append(results, RepoSettingResult{Section: section, Field: field, Category: WouldSkipScope, Annotation: skipAnnotation})
}
