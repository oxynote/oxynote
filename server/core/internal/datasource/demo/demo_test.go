package demo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_URL(t *testing.T) {
	t.Parallel()

	// the runner routes on the scheme before it routes on the whole URL,
	// so the one demo source has to carry it.
	assert.Equal(t, "demo://", Scheme)
	assert.Equal(t, "demo://engineering", URL)
}

func Test_ErrUnknownSource(t *testing.T) {
	t.Parallel()

	assert.Error(t, ErrUnknownSource)

	// the message names the source that does exist: the reader mistyped
	// something, and the only useful answer is what they meant.
	assert.Contains(t, ErrUnknownSource.Error(), URL)
}
