package redkit

import (
	"context"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

func Test_BytesStore_Start(t *testing.T) {
	t.Parallel()

	bs := &BytesStore{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	go func() {
		defer close(done)

		bs.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("start did not stop")
	}
}

func Test_BytesStore_Set(t *testing.T) {
	cc := map[string]struct {
		Cancelled   bool
		ExpireAfter time.Duration
		Conn        func() (*redigomock.Conn, func(*testing.T))
		Value       []byte
		Err         error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
				}
			},
			Value: []byte("payload"),
			Err:   assert.AnError,
		},
		"SET returns an error": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("SET", "test", []byte("payload"), "EX", int(_testExpiry.Seconds())).
					ExpectError(assert.AnError)

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Value: []byte("payload"),
			Err:   assert.AnError,
		},
		"Sub-second expiry stores without an expiry": {
			ExpireAfter: time.Millisecond,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("SET", "test", []byte("payload"))

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Value: []byte("payload"),
		},
		"Successfully set payload": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("SET", "test", []byte("payload"), "EX", int(_testExpiry.Seconds()))

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Value: []byte("payload"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn, check := c.Conn()

			expireAfter := c.ExpireAfter
			if expireAfter == 0 {
				expireAfter = _testExpiry
			}

			bs := &BytesStore{
				pool: &redis.Pool{
					Dial: func() (redis.Conn, error) {
						return conn, nil
					},
					Wait:      true,
					MaxActive: 10,
				},
				expireAfter: expireAfter,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			err := bs.Set(ctx, "test", c.Value)

			check(t)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_BytesStore_Get(t *testing.T) {
	cc := map[string]struct {
		Cancelled bool
		Conn      func() (*redigomock.Conn, func(*testing.T))
		Result    []byte
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
				}
			},
			Err: assert.AnError,
		},
		"GET returns an error": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("GET", "test").ExpectError(assert.AnError)

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Err: assert.AnError,
		},
		"Missing key is reported as not found": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("GET", "test").ExpectError(redis.ErrNil)

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Err: errutil.ErrNotFound,
		},
		"Successfully fetched payload": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("GET", "test").Expect([]byte("payload"))

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Result: []byte("payload"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn, check := c.Conn()

			bs := &BytesStore{
				pool: &redis.Pool{
					Dial: func() (redis.Conn, error) {
						return conn, nil
					},
					Wait:      true,
					MaxActive: 10,
				},
				expireAfter: _testExpiry,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			res, err := bs.Get(ctx, "test")

			check(t)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_BytesStore_Delete(t *testing.T) {
	cc := map[string]struct {
		Cancelled bool
		Conn      func() (*redigomock.Conn, func(*testing.T))
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
				}
			},
			Err: assert.AnError,
		},
		"DEL returns an error": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("DEL", "test").ExpectError(assert.AnError)

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
			Err: assert.AnError,
		},
		"Successfully deleted": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				cmd := conn.Command("DEL", "test").Expect(int64(1))

				return conn, func(t *testing.T) {
					assert.NoError(t, conn.ExpectationsWereMet())
					assert.Equal(t, 1, conn.Stats(cmd))
				}
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn, check := c.Conn()

			bs := &BytesStore{
				pool: &redis.Pool{
					Dial: func() (redis.Conn, error) {
						return conn, nil
					},
					Wait:      true,
					MaxActive: 10,
				},
				expireAfter: _testExpiry,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			err := bs.Delete(ctx, "test")

			check(t)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}
