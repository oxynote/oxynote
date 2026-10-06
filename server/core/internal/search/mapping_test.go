package search

import (
	"testing"

	bleveIndex "github.com/blevesearch/bleve_index_api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newIndexMapping(t *testing.T) {
	t.Parallel()

	im, err := newIndexMapping()
	require.NoError(t, err)
	require.NotNil(t, im)

	assert.Equal(t, bleveIndex.BM25Scoring, im.ScoringModel)
	assert.Equal(t, _analyzerName, im.AnalyzerNameForPath(_fieldText))
	assert.Equal(t, "keyword", im.AnalyzerNameForPath(_fieldOrganizationID))
	assert.Equal(t, "keyword", im.AnalyzerNameForPath(_fieldBranchID))
	assert.Equal(t, "keyword", im.AnalyzerNameForPath(_fieldDocumentID))
	assert.Equal(t, "keyword", im.AnalyzerNameForPath(_fieldType))

	assert.Equal(t, "keyword", im.AnalyzerNameForPath(_fieldAttrs+".language"))

	// the analyzer lowercases and stems without dropping stop words.
	tokens := im.AnalyzerNamed(_analyzerName).Analyze([]byte("Not Deploying"))
	require.Len(t, tokens, 2)
	assert.Equal(t, "not", string(tokens[0].Term))
	assert.Equal(t, "deploy", string(tokens[1].Term))

	// the branch name is kept for display only.
	name := im.DefaultMapping.Properties[_fieldBranchName].Fields[0]
	assert.False(t, name.Index)
	assert.True(t, name.Store)
}

func Test_registerHighlighter(t *testing.T) {
	t.Parallel()

	// registration happens once per process, so a second call is a no-op
	// rather than a duplicate-definition error.
	require.NoError(t, registerHighlighter())
	require.NoError(t, registerHighlighter())
}
