// Package redisutil provides a set of helpers for working with redis pool.
package redisutil

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/gomodule/redigo/redis"
)

const (
	// _maxIdleConns specifies the maximum number of idle connections
	// kept in the pool.
	_maxIdleConns = 5

	// _dialTimeout specifies the timeout applied to connect, read and
	// write operations of a dialed connection.
	_dialTimeout = time.Second * 3
)

// NewPool prepares a new redis connection pool. The URL follows the
// redis[s]://[username:password@]host:port[/db] form.
func NewPool(rawURL string) (*redis.Pool, error) {
	if rawURL == "" {
		return nil, errors.New("invalid redis url")
	}

	// the URL is checked eagerly so a malformed value fails at boot
	// instead of on the first dial.
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}

	if u.Scheme != "redis" && u.Scheme != "rediss" {
		return nil, errors.New("invalid redis url scheme")
	}

	return &redis.Pool{
		MaxIdle:     _maxIdleConns,
		IdleTimeout: time.Minute,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.DialURLContext(
				ctx,
				rawURL,
				redis.DialConnectTimeout(_dialTimeout),
				redis.DialWriteTimeout(_dialTimeout),
				redis.DialReadTimeout(_dialTimeout),
			)
		},
	}, nil
}
