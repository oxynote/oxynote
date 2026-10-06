package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_result(t *testing.T) {
	t.Parallel()

	// error
	_, err := result(make(chan int))
	assert.Error(t, err)

	// success
	res, err := result(map[string]any{"ok": true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, res)
}

func Test_errRequired(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, errRequired("document_id"), "document_id is required")
}

func Test_countPhrase(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1 page", countPhrase(1, "page"))
	assert.Equal(t, "3 blocks", countPhrase(3, "block"))
}

func Test_sentence(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Clauses []string
		Result  string
	}{
		"One clause": {
			Clauses: []string{"rename it"},
			Result:  "Rename it",
		},
		"Two clauses": {
			Clauses: []string{"rename it", "move it"},
			Result:  "Rename it and move it",
		},
		"Three clauses": {
			Clauses: []string{"rename it", "recolour it", "move it"},
			Result:  "Rename it, recolour it and move it",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, sentence(c.Clauses))
		})
	}
}
