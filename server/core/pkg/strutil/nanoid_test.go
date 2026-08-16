package strutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_NanoID(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})

	for range 100 {
		id := NanoID()

		assert.Len(t, id, _nanoidLength)

		for _, r := range id {
			assert.True(
				t,
				strings.ContainsRune(_nanoidAlphabet, r),
				"unexpected character %q in %q", r, id,
			)
		}

		seen[id] = struct{}{}
	}

	assert.Len(t, seen, 100, "generated IDs should be unique")
}
