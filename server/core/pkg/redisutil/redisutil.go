// Package redisutil provides a set of helpers for working with redis pool.
package redisutil

import (
	"context"
	"errors"
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

// NewPool prepares a new redis connection pool.
func NewPool(network, address string) (*redis.Pool, error) {
	if network == "" {
		return nil, errors.New("invalid redis network")
	}

	if address == "" {
		return nil, errors.New("invalid redis address")
	}

	return &redis.Pool{
		MaxIdle:     _maxIdleConns,
		IdleTimeout: time.Minute,
		DialContext: func(ctx context.Context) (redis.Conn, error) {
			return redis.DialContext(
				ctx,
				network,
				address,
				redis.DialConnectTimeout(_dialTimeout),
				redis.DialWriteTimeout(_dialTimeout),
				redis.DialReadTimeout(_dialTimeout),
			)
		},
	}, nil
}
