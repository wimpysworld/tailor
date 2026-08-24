package gh

// isTrue reports whether p is a known true value.
func isTrue(p *bool) bool {
	return p != nil && *p
}

// isFalse reports whether p is a known false value.
func isFalse(p *bool) bool {
	return p != nil && !*p
}

// strEq reports whether p is a known value equal to want.
func strEq(p *string, want string) bool {
	return p != nil && *p == want
}
