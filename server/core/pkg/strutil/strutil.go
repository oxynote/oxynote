// Package strutil provides small string helpers shared across the
// codebase.
package strutil

import "strings"

// Ellipsize returns at most maxLen runes of s, appending a single
// ellipsis rune when the input was longer. A non-positive maxLen
// returns s unchanged.
func Ellipsize(s string, maxLen int) string {
	if maxLen <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	return string(runes[:maxLen]) + "…"
}

// Preview returns at most maxLen runes of s, collapsed to a single line
// and trimmed, elided the way Ellipsize does.
func Preview(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)

	return Ellipsize(s, maxLen)
}
