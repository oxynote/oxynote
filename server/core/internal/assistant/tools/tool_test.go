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

func Test_plainTraits_Traits(t *testing.T) {
	t.Parallel()

	// a plain read writes nothing, reaches no outbound connection and
	// belongs on every surface.
	assert.Equal(t, Traits{}, plainTraits{}.Traits())
}

func Test_plainTitle_Title(t *testing.T) {
	t.Parallel()

	// a tool too generic to announce says nothing, and never fails
	// doing so: the arguments are not even read.
	got, err := plainTitle{}.Title(testInput(testDeps(nil, nil, nil), NameListDocuments, `{`))
	require.NoError(t, err)
	assert.Empty(t, got)
}
