package redkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testExpiry is the expiry used across the byte-store cases.
var _testExpiry = time.Hour * 168

func Test_NewBytesStore(t *testing.T) {
	pool := &redis.Pool{}
	bs := NewBytesStore(pool, _testExpiry)
	require.NotNil(t, bs)
	assert.Equal(t, pool, bs.pool)
	assert.Equal(t, _testExpiry, bs.expireAfter)
}

func Test_BytesStore_Set(t *testing.T) {
	cc := map[string]struct {
		Cancelled   bool
		ExpireAfter time.Duration
		Conn        func() *redigomock.Conn
		Value       []byte
		Err         error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn:      redigomock.NewConn,
			Value:     []byte("payload"),
			Err:       assert.AnError,
		},
		"SET returns an error": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("SET", "test", []byte("payload"), "EX", int(_testExpiry.Seconds())).
					ExpectError(assert.AnError)

				return conn
			},
			Value: []byte("payload"),
			Err:   assert.AnError,
		},
		"Sub-second expiry stores without an expiry": {
			ExpireAfter: time.Millisecond,
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("SET", "test", []byte("payload"))

				return conn
			},
			Value: []byte("payload"),
		},
		"Successfully set payload": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("SET", "test", []byte("payload"), "EX", int(_testExpiry.Seconds()))

				return conn
			},
			Value: []byte("payload"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn := c.Conn()

			expireAfter := c.ExpireAfter
			if expireAfter == 0 {
				expireAfter = _testExpiry
			}

			bs := &BytesStore{
				pool:        stubPool(conn),
				expireAfter: expireAfter,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			err := bs.Set(ctx, "test", c.Value)

			assert.NoError(t, conn.ExpectationsWereMet())

			if c.Err != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_BytesStore_Get(t *testing.T) {
	cc := map[string]struct {
		Cancelled bool
		Conn      func() *redigomock.Conn
		Result    []byte
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn:      redigomock.NewConn,
			Err:       assert.AnError,
		},
		"GET returns an error": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").ExpectError(assert.AnError)

				return conn
			},
			Err: assert.AnError,
		},
		"Missing key is reported as not found": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").ExpectError(redis.ErrNil)

				return conn
			},
			Err: errutil.ErrNotFound,
		},
		"Successfully fetched payload": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").Expect([]byte("payload"))

				return conn
			},
			Result: []byte("payload"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn := c.Conn()

			bs := &BytesStore{pool: stubPool(conn), expireAfter: _testExpiry}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			res, err := bs.Get(ctx, "test")

			assert.NoError(t, conn.ExpectationsWereMet())

			if c.Err != nil {
				if errors.Is(c.Err, errutil.ErrNotFound) {
					assert.ErrorIs(t, err, errutil.ErrNotFound)

					return
				}

				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_BytesStore_Delete(t *testing.T) {
	cc := map[string]struct {
		Cancelled bool
		Conn      func() *redigomock.Conn
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn:      redigomock.NewConn,
			Err:       assert.AnError,
		},
		"DEL returns an error": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("DEL", "test").ExpectError(assert.AnError)

				return conn
			},
			Err: assert.AnError,
		},
		"Successfully deleted": {
			Conn: func() *redigomock.Conn {
				conn := redigomock.NewConn()
				conn.Command("DEL", "test").Expect(int64(1))

				return conn
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn := c.Conn()

			bs := &BytesStore{pool: stubPool(conn), expireAfter: _testExpiry}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			err := bs.Delete(ctx, "test")

			assert.NoError(t, conn.ExpectationsWereMet())

			if c.Err != nil {
				assert.Error(t, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

// stubPool wraps a mock connection in a pool the stores can draw from.
func stubPool(conn redis.Conn) *redis.Pool {
	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return conn, nil
		},
		Wait:      true,
		MaxActive: 10,
	}
}
