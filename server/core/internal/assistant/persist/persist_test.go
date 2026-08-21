package persist

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// discardLog returns a logger that writes nowhere.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func Test_SessionKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user", SessionKey("org", "user"))
}
