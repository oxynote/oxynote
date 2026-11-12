package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_BlockNodeType_IsGrid(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type   BlockNodeType
		Result bool
	}{
		"Block grid":                 {Type: BlockNodeBlockGrid, Result: true},
		"Grid under its former name": {Type: BlockNodeMetricGrid, Result: true},
		"Block that is no grid":      {Type: BlockNodeMetricBlock},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.Result, c.Type.IsGrid())
		})
	}
}
