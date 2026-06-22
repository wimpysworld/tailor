package alter

// skipAnnotation extracts a short annotation string from a skip error.
// For ErrInsufficientScope it returns "token missing required scope".
func skipAnnotation(err error) string {
	return "token missing required scope"
}

// classifySkipCategory returns WouldSkipScope for access errors.
func classifySkipCategory(err error) RepoSettingCategory {
	return WouldSkipScope
}
