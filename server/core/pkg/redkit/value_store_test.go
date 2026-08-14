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

func Test_NewValueStore(t *testing.T) {
	pool := &redis.Pool{}
	vs := NewValueStore[string](pool, time.Hour*168)
	require.NotNil(t, vs)
	assert.Equal(t, vs.pool, pool)
	assert.Equal(t, time.Hour*168, vs.expireAfter)
}

func Test_ValueStore_Set(t *testing.T) {
	cc := map[string]struct {
		Cancelled bool
		Conn      func() (*redigomock.Conn, func(*testing.T))
		Value     any
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Value: "123",
			Err:   assert.AnError,
		},
		"Invalid value": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Value: struct {
				Fn func()
			}{},
			Err: assert.AnError,
		},
		"SET returns an error": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("SET", "test", []byte(`"123"`), "EX", int((time.Hour * 168).Seconds())).ExpectError(assert.AnError)

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Value: "123",
			Err:   assert.AnError,
		},
		"Successfully set element": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("SET", "test", []byte(`"123"`), "EX", int((time.Hour * 168).Seconds()))

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Value: "123",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn, check := c.Conn()

			vs := &ValueStore[any]{
				pool: &redis.Pool{
					Dial: func() (redis.Conn, error) {
						return conn, nil
					},
					Wait:      true,
					MaxActive: 10,
				},
				expireAfter: time.Hour * 168,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			err := vs.Set(ctx, "test", c.Value)

			check(t)

			if c.Err != nil {
				if errors.Is(c.Err, assert.AnError) {
					assert.Error(t, err)
					return
				}

				assert.Equal(t, c.Err, err)

				return
			}

			assert.NoError(t, err)
		})
	}
}

func Test_ValueStore_Get(t *testing.T) {
	type testStruct struct {
		Value string `json:"value"`
	}

	cc := map[string]struct {
		Cancelled bool
		Conn      func() (*redigomock.Conn, func(*testing.T))
		Result    testStruct
		Err       error
	}{
		"Cancelled context": {
			Cancelled: true,
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Err: assert.AnError,
		},
		"GET returns an error": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").ExpectError(assert.AnError)

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Err: assert.AnError,
		},
		"Invalid value": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").Expect(`{"value":"123"`)

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Err: assert.AnError,
		},
		"Key is not found": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").ExpectError(redis.ErrNil)

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Err: errutil.ErrNotFound,
		},
		"Successfully retrieved value": {
			Conn: func() (*redigomock.Conn, func(*testing.T)) {
				conn := redigomock.NewConn()
				conn.Command("GET", "test").Expect(`{"value":"123"}`)

				return conn, func(t *testing.T) {
					err := conn.ExpectationsWereMet()
					assert.NoError(t, err)
				}
			},
			Result: testStruct{
				Value: "123",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			conn, check := c.Conn()

			vs := &ValueStore[testStruct]{
				pool: &redis.Pool{
					Dial: func() (redis.Conn, error) {
						return conn, nil
					},
					Wait:      true,
					MaxActive: 10,
				},
				expireAfter: time.Hour * 168,
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			res, err := vs.Get(ctx, "test")

			check(t)

			if c.Err != nil {
				assert.Nil(t, res)

				if errors.Is(c.Err, assert.AnError) {
					assert.Error(t, err)
					return
				}

				assert.Equal(t, c.Err, err)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, c.Result, *res)
		})
	}
}

func Test_ValueStore_Delete(t *testing.T) {
	conn := redigomock.NewConn()
	conn.Command("DEL", "test").Expect(int64(1))

	vs := &ValueStore[any]{
		pool: &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return conn, nil
			},
			Wait:      true,
			MaxActive: 10,
		},
		expireAfter: time.Hour * 168,
	}

	err := vs.Delete(context.Background(), "test")
	require.NoError(t, err)

	err = conn.ExpectationsWereMet()
	require.NoError(t, err)
}
