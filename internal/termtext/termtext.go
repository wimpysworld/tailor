// Package termtext renders untrusted text safely for terminal output.
package termtext

import (
	"fmt"
	"strings"
	"unicode"
)

// EscapeControlText renders control characters as visible escapes so that
// values from config or the GitHub API cannot inject terminal control
// sequences into output. C0 controls and DEL render as \xNN; C1 and other
// Unicode controls render as \uNNNN. Text without control characters passes
// through unchanged.
func EscapeControlText(input string) string {
	if !strings.ContainsFunc(input, unicode.IsControl) {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		switch {
		case !unicode.IsControl(r):
			b.WriteRune(r)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}
