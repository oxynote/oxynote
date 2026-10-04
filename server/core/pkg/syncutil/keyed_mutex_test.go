package syncutil

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewKeyedMutex(t *testing.T) {
	t.Parallel()

	km := NewKeyedMutex[string]()
	require.NotNil(t, km)
	assert.Empty(t, km.locks)
}

func Test_KeyedMutex_Lock(t *testing.T) {
	t.Parallel()

	ended, cancel := context.WithCancel(context.Background())
	cancel()

	cc := map[string]struct {
		Context context.Context
		// Held is the key another caller holds during the lock.
		Held string
		Err  error
	}{
		"Free key is locked": {
			Context: context.Background(),
		},
		"Held key gives up when the context ends": {
			Context: ended,
			Held:    "a",
			Err:     context.Canceled,
		},
		"Other key is independent": {
			Context: context.Background(),
			Held:    "b",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			km := NewKeyedMutex[string]()

			held := func() {}

			if c.Held != "" {
				var err error

				held, err = km.Lock(context.Background(), c.Held)
				require.NoError(t, err)
			}

			unlock, err := km.Lock(c.Context, "a")
			testutil.AssertEqualError(t, c.Err, err)

			if err == nil {
				unlock()
			}

			held()

			assert.Empty(t, km.locks)
		})
	}
}
