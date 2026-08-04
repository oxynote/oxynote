// Package redisutil provides a set of helpers for working with redis pool.
package redisutil

import (
	"context"
	"errors"
	"time"

	"github.com/gomodule/redigo/redis"
)

// NewPool prepares a new redis connection pool.
func NewPool(network, address string) (*redis.Pool, error) {
	if network == "" {
		return nil, errors.New("invalid redis network")
	}

	if address == "" {
		return nil, errors.New("invalid redis address")
	}

	return &redis.Pool{
		MaxIdle:     5,
		IdleTimeout: time.Minute,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.DialContext(
				ctx,
				network,
				address,
				redis.DialConnectTimeout(time.Second*3),
				redis.DialWriteTimeout(time.Second*3),
				redis.DialReadTimeout(time.Second*3),
			)
		},
	}, nil
}
