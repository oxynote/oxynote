package sliceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Move(t *testing.T) {
	a := []int{1, 2, 3, 4, 5}

	// No change, index too small.
	Move(a, -5, 0)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, a)

	// No change, index too large.
	Move(a, 1, 9)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, a)

	// No change, index is the same.
	Move(a, 2, 2)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, a)

	// Success forward.
	Move(a, 1, 3)

	assert.Equal(t, []int{1, 3, 4, 2, 5}, a)

	// Success backward.
	Move(a, 3, 1)

	assert.Equal(t, []int{1, 2, 3, 4, 5}, a)
}
