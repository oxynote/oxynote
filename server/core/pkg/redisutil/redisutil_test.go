package redisutil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewPool(t *testing.T) {
	cc := map[string]struct {
		URL string
		Err error
	}{
		"Empty url": {
			URL: "",
			Err: errors.New("invalid redis url"),
		},
		"Error returned by url.Parse": {
			URL: "redis://[::1",
			Err: assert.AnError,
		},
		"Invalid url scheme": {
			URL: "http://127.0.0.1:6379",
			Err: errors.New("invalid redis url scheme"),
		},
		"Successful creation": { //nolint:gosec // static test credentials
			URL: "redis://user:pass@127.0.0.1:1",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			pool, err := NewPool(c.URL)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, pool)
				return
			}

			require.NotNil(t, pool)

			assert.Equal(t, 5, pool.MaxIdle)
			assert.Equal(t, time.Minute, pool.IdleTimeout)

			conn, err := pool.DialContext(context.Background())
			require.Error(t, err)
			require.Nil(t, conn)
		})
	}
}
