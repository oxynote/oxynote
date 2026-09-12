package tag

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ColorHex(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "#00a63e", ColorHex("green"))
	assert.Empty(t, ColorHex("navy"))
}

func Test_ColorName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "green", ColorName("#00a63e"))
	assert.Empty(t, ColorName("#00A63E"))
}

func Test_ColorNames(t *testing.T) {
	t.Parallel()

	names := ColorNames()

	require.Len(t, names, len(_palette))
	assert.Equal(t, "red", names[0])
	assert.Equal(t, "pink", names[len(names)-1])
}
