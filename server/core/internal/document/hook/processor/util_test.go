package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubInput is a test Input carrying a state and an optional change
// detection client.
type stubInput struct {
	state State
	cd    ChangeDetection
}

func (i stubInput) State() State {
	return i.state
}

func (i stubInput) Github(_ context.Context) (Github, error) {
	return nil, nil //nolint:nilnil // unused by the processors under test
}

func (i stubInput) ChangeDetection() ChangeDetection {
	return i.cd
}

func Test_PlainDeleter_Delete(t *testing.T) {
	t.Parallel()

	assert.NoError(t, (&PlainDeleter{}).Delete(context.Background(), stubInput{}))
}
