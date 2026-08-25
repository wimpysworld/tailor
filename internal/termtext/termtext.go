// Package termtext renders untrusted text safely for terminal output.
package termtext

import (
	"fmt"
	"strings"
	"unicode"
)

// EscapeControlText renders control characters as visible escapes so that
// values from config or the GitHub API cannot inject terminal control
// sequences into output. C0 controls and DEL render as \xNN; C1 controls,
// other Unicode controls, and bidirectional control characters (U+061C,
// U+200E, U+200F, U+202A-U+202E, U+2066-U+2069) render as \uNNNN, blocking
// Trojan Source reordering (CVE-2021-42574). Other format characters, such
// as U+200D zero width joiner, pass through so legitimate emoji sequences
// survive. Text without escapable characters passes through unchanged.
func EscapeControlText(input string) string {
	if !strings.ContainsFunc(input, needsEscape) {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for _, r := range input {
		switch {
		case !needsEscape(r):
			b.WriteRune(r)
		case r < 0x80:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	return b.String()
}

// needsEscape reports whether r must render as a visible escape.
func needsEscape(r rune) bool {
	return unicode.IsControl(r) || isBidiControl(r)
}

// isBidiControl reports whether r is a bidirectional control character that
// can visually reorder terminal text.
func isBidiControl(r rune) bool {
	switch r {
	case '\u061c', '\u200e', '\u200f':
		return true
	}
	return (r >= '\u202a' && r <= '\u202e') || (r >= '\u2066' && r <= '\u2069')
}
