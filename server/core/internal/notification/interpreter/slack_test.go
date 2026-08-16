package interpreter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewSlackFormatter(t *testing.T) {
	t.Parallel()

	require.NotNil(t, NewSlackFormatter())
}

func Test_SlackFormatter_Link(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		"<https://app.test/doc|My Doc>",
		NewSlackFormatter().Link("https://app.test/doc", "My Doc"),
	)
}
