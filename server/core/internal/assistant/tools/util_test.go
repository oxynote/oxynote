package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_countPhrase(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		N      int
		Noun   string
		Result string
	}{
		"Singular count": {
			N:      1,
			Noun:   "page",
			Result: "1 page",
		},
		"Plural count": {
			N:      3,
			Noun:   "block",
			Result: "3 blocks",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, countPhrase(c.N, c.Noun))
		})
	}
}

func Test_joinClauses(t *testing.T) {
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

			assert.Equal(t, c.Result, joinClauses(c.Clauses))
		})
	}
}

func Test_errRequired(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, errRequired("document_id"), "document_id is required")
}
