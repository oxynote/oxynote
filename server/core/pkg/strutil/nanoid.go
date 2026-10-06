// Package strutil provides small string helpers shared across the
// codebase.
package strutil

import (
	"crypto/rand"
	"strings"
)

// _nanoidAlphabet is the default alphabet used by nanoid.
// This matches the Node.js nanoid default: A-Za-z0-9_-.
const _nanoidAlphabet = "useandom-26T198340PX75pxJACKVERYMINDBUSHWOLF_GQZbfghjklqvwyzrict"

// NanoIDLength is the default length of nanoid IDs (21 characters).
const NanoIDLength = 21

// NanoID generates a nanoid-compatible unique identifier. The output is 1:1
// compatible with the Node.js nanoid() function using default settings.
func NanoID() string {
	bytes := make([]byte, NanoIDLength)

	_, err := rand.Read(bytes)
	if err != nil {
		// NOCOV: crypto/rand failures cannot be simulated in tests.
		panic(err)
	}

	id := make([]byte, NanoIDLength)

	for i := range NanoIDLength {
		id[i] = _nanoidAlphabet[bytes[i]&63]
	}

	return string(id)
}

// IsNanoID reports whether s has the exact shape NanoID produces: the
// default length, every byte from the default alphabet.
func IsNanoID(s string) bool {
	if len(s) != NanoIDLength {
		return false
	}

	for i := range len(s) {
		if !strings.ContainsRune(_nanoidAlphabet, rune(s[i])) {
			return false
		}
	}

	return true
}
