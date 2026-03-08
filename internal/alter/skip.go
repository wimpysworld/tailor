package alter

import (
	"errors"

	"github.com/wimpysworld/tailor/internal/gh"
)

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
