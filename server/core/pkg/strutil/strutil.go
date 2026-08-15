// Package strutil provides small string helpers shared across the
// codebase.
package strutil

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
