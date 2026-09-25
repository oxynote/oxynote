package syncutil

import (
	"context"
	"testing"

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

	km := NewKeyedMutex[string]()

	unlock, err := km.Lock(context.Background(), "a")
	require.NoError(t, err)

	// a second caller gives up once its context ends.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = km.Lock(ctx, "a")
	assert.Equal(t, context.Canceled, err)

	// another key is not held by the first one's mutex.
	other, err := km.Lock(context.Background(), "b")
	require.NoError(t, err)

	other()
	unlock()

	// every mutex is dropped once no one uses it.
	assert.Empty(t, km.locks)

	unlock, err = km.Lock(context.Background(), "a")
	require.NoError(t, err)

	unlock()
}

func Test_KeyedMutex_TryLock(t *testing.T) {
	t.Parallel()

	km := NewKeyedMutex[string]()

	unlock, ok := km.TryLock("a")
	require.True(t, ok)

	_, ok = km.TryLock("a")
	assert.False(t, ok)

	unlock()

	assert.Empty(t, km.locks)

	unlock, ok = km.TryLock("a")
	require.True(t, ok)

	unlock()
}
