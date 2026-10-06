package strutil

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NanoID(t *testing.T) {
	t.Parallel()

	seen := make(map[string]struct{})

	for range 100 {
		id := NanoID()

		assert.Len(t, id, NanoIDLength)

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

func Test_IsNanoID(t *testing.T) {
	cc := map[string]struct {
		ID     string
		Result bool
	}{
		"Generated id":     {ID: NanoID(), Result: true},
		"Id with dashes":   {ID: "f1-x_-xxxxxxxxxxxxxxx", Result: true},
		"Short":            {ID: "f1"},
		"Long":             {ID: "f1xxxxxxxxxxxxxxxxxxxx"},
		"Wrong charset":    {ID: "f1.................xx"},
		"Multi-byte runes": {ID: "é" + "xxxxxxxxxxxxxxxxxxx"},
		"Empty":            {},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, IsNanoID(c.ID))
		})
	}
}
