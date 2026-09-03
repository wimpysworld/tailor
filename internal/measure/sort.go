package measure

import (
	"cmp"
	"slices"
)

// concatSortedByPath sorts each group lexicographically by the path that
// path returns, then concatenates the groups in the order given. Paths are
// unique within a group, so the unstable sort yields a deterministic order.
func concatSortedByPath[T any](path func(T) string, groups ...[]T) []T {
	for _, group := range groups {
		slices.SortFunc(group, func(a, b T) int {
			return cmp.Compare(path(a), path(b))
		})
	}
	return slices.Concat(groups...)
}
