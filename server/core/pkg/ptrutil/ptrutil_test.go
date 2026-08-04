package ptrutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_New(t *testing.T) {
	v := 42
	p := New(v)
	require.Equal(t, v, *p)
}
