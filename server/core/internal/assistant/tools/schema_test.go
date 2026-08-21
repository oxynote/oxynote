package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_stringProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, map[string]any{"type": "string", "description": "a doc"}, stringProp("a doc"))
}

func Test_documentIDProp(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		map[string]any{"document_id": map[string]any{"type": "string", "description": "a doc"}},
		documentIDProp("a doc"))
}
