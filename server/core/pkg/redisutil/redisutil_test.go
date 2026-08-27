package redisutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPool(t *testing.T) {
	// Error, url is empty.
	pool, err := NewPool("")
	require.EqualError(t, err, "invalid redis url")
	require.Nil(t, pool)

	// Error, url is malformed.
	pool, err = NewPool("redis://[::1")
	require.Error(t, err)
	require.Nil(t, pool)

	// Error, url scheme is not redis.
	pool, err = NewPool("http://127.0.0.1:6379")
	require.EqualError(t, err, "invalid redis url scheme")
	require.Nil(t, pool)

	// Success.
	pool, err = NewPool("redis://user:pass@127.0.0.1:1")
	require.NoError(t, err)
	require.NotNil(t, pool)

	assert.Equal(t, 5, pool.MaxIdle)
	assert.Equal(t, time.Minute, pool.IdleTimeout)

	// Error, dial error.
	conn, err := pool.DialContext(context.Background())
	require.Error(t, err)
	require.Nil(t, conn)
}
