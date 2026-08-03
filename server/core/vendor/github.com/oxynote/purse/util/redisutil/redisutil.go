// Package redisutil provides a set of helpers for working with redis pool.
package redisutil

import (
	"context"
	"errors"

	"github.com/gomodule/redigo/redis"
)

// NewPool prepares a new redis connection pool.
func NewPool(network, address string, opts ...Option) (*redis.Pool, error) {
	if network == "" {
		return nil, errors.New("invalid redis network")
	}

	if address == "" {
		return nil, errors.New("invalid redis address")
	}

	stg := _defaultSettings
	for _, opt := range opts {
		opt(&stg)
	}

	return &redis.Pool{
		MaxIdle:     stg.maxIdle,
		IdleTimeout: stg.idleTimeout,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.DialContext(
				ctx,
				network,
				address,
				redis.DialConnectTimeout(stg.dialTimeout),
				redis.DialWriteTimeout(stg.writeTimeout),
				redis.DialReadTimeout(stg.readTimeout),
				redis.DialUseTLS(stg.useTLS),
			)
		},
	}, nil
}
