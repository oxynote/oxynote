package migrations

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_Go(t *testing.T) {
	for _, m := range Go() {
		assert.NotNil(t, m.UpFnContext, "migration %d has no up step on a transaction", m.Version)
		assert.NotNil(t, m.DownFnContext, "migration %d has no down step on a transaction", m.Version)
	}
}
