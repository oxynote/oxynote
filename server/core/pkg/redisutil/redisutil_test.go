package redisutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewPool(t *testing.T) {
	// Error, network is empty.
	pool, err := NewPool("", "")
	require.EqualError(t, err, "invalid redis network")
	require.Nil(t, pool)

	// Error, address is empty.
	pool, err = NewPool("127.0.0.1", "")
	require.EqualError(t, err, "invalid redis address")
	require.Nil(t, pool)

	// Success.
	pool, err = NewPool("127.0.0.1", "8080")
	require.NoError(t, err)
	require.NotNil(t, pool)

	assert.Equal(t, 5, pool.MaxIdle)
	assert.Equal(t, time.Minute, pool.IdleTimeout)

	// Error, dial error.
	conn, err := pool.DialContext(context.Background())
	require.Error(t, err)
	require.Nil(t, conn)
}
