package sliceutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Move(t *testing.T) {
	cc := map[string]struct {
		Slice    []int
		OldIndex int
		NewIndex int
		Result   []int
	}{
		"No change when index is too small": {
			Slice:    []int{1, 2, 3, 4, 5},
			OldIndex: -5,
			NewIndex: 0,
			Result:   []int{1, 2, 3, 4, 5},
		},
		"No change when index is too large": {
			Slice:    []int{1, 2, 3, 4, 5},
			OldIndex: 1,
			NewIndex: 9,
			Result:   []int{1, 2, 3, 4, 5},
		},
		"No change when index is the same": {
			Slice:    []int{1, 2, 3, 4, 5},
			OldIndex: 2,
			NewIndex: 2,
			Result:   []int{1, 2, 3, 4, 5},
		},
		"Successful forward move": {
			Slice:    []int{1, 2, 3, 4, 5},
			OldIndex: 1,
			NewIndex: 3,
			Result:   []int{1, 3, 4, 2, 5},
		},
		"Successful backward move": {
			Slice:    []int{1, 3, 4, 2, 5},
			OldIndex: 3,
			NewIndex: 1,
			Result:   []int{1, 2, 3, 4, 5},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			Move(c.Slice, c.OldIndex, c.NewIndex)
			assert.Equal(t, c.Result, c.Slice)
		})
	}
}
