package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ColorHex(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name   string
		Result string
	}{
		"Unknown name": {
			Name: "navy",
		},
		"Palette name": {
			Name:   "green",
			Result: "#00a63e",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, ColorHex(c.Name))
		})
	}
}

func Test_ColorName(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Hex    string
		Result string
	}{
		"Uppercase palette hex": {
			Hex: "#00A63E",
		},
		"Palette hex": {
			Hex:    "#00a63e",
			Result: "green",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, ColorName(c.Hex))
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
