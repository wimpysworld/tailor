package ptr

// Bool returns a pointer to the given bool value.
func Bool(v bool) *bool { return &v }

// String returns a pointer to the given string value.
func String(v string) *string { return &v }
