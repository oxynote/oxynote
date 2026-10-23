package persist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_SessionKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "assistant:session:org:user", SessionKey("org", "user"))
}
