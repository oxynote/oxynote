package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ValidColor(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name   string
		Result bool
	}{
		"Unknown name": {
			Name: "navy",
		},
		"Palette hex": {
			Name: "#00a63e",
		},
		"Uppercase palette name": {
			Name: "Green",
		},
		"Palette name": {
			Name:   "green",
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, ValidColor(c.Name))
		})
	}
}

func Test_ColorNames(t *testing.T) {
	t.Parallel()

	names := ColorNames()

	require.Len(t, names, len(_palette))
	assert.Equal(t, "red", names[0])
	assert.Equal(t, "pink", names[len(names)-1])
}
