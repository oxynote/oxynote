package demo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

// _client is the client every test that reads the timeline goes through.
// One client is what a process has — the manager holds it — and building
// one per test would pay the registry's cold replay over and over, which
// on a small runner is the difference between a query answering and
// timing out.
var _client *Client

func TestMain(m *testing.M) {
	_client = NewClient()

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
