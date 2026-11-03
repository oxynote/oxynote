package strutil

import (
	"strings"
	"unicode/utf8"
)

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

// ValidateName reports whether a trimmed name is usable: it fails with
// emptyErr when the name is empty, and with longErr when it holds more
// than maxLen runes.
func ValidateName(name string, maxLen int, emptyErr, longErr error) error {
	if name == "" {
		return emptyErr
	}

	if utf8.RuneCountInString(name) > maxLen {
		return longErr
	}

	return nil
}
