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
