package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPrometheus(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	p := NewPrometheus(inp)
	require.NotNil(t, p)
	assert.Same(t, inp, p.inp)
}
