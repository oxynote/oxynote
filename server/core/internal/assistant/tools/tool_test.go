package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_plainSummary_Summary(t *testing.T) {
	t.Parallel()

	// a tool that proposes nothing describes nothing, and never fails
	// doing so: the gate is only ever applied to a write.
	got, err := plainSummary{}.Summary(testInput(testDeps(nil, nil, nil), NameGetDocument, `{`))
	require.NoError(t, err)
	assert.Equal(t, ActionSummary{}, got)
}
