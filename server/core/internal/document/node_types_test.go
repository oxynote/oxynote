package document

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_BlockNodeType_IsGrid(t *testing.T) {
	t.Parallel()

	assert.True(t, BlockNodeBlockGrid.IsGrid())
	assert.True(t, BlockNodeMetricGrid.IsGrid())
	assert.False(t, BlockNodeMetricBlock.IsGrid())
}
