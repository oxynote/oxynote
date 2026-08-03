package redisutil

import (
	"time"
)

// _defaultSettings specifies sane defaults for the redis settings.
var _defaultSettings = settings{
	maxIdle:      5,
	idleTimeout:  time.Minute,
	dialTimeout:  time.Second * 3,
	readTimeout:  time.Second * 3,
	writeTimeout: time.Second * 3,
}

// settings is a struct used to determine custom redis configuration.
// Attributes have sensible default values which are most used in all the projects.
type settings struct {
	// maxIdle sets the maximum number of idle connections in the pool.
	maxIdle int

	// idleTimeout sets the duration after which the idle connection is
	// closed.
	idleTimeout time.Duration

	// dialTimeout sets the duration after which the connection request is
	// closed.
	dialTimeout time.Duration

	// readTimeout sets the duration after which the read request is
	// closed.
	readTimeout time.Duration

	// writeTimeout sets the duration after which the write request is
	// closed.
	writeTimeout time.Duration

	// useTLS sets the use of TLS for the redis connection.
	useTLS bool
}

// Option specifies an option for the redis.Client handler.
type Option func(*settings)

// WithMaxIdle sets the maximum idle connections in redis.
// The default is 3 connections.
func WithMaxIdle(mi int) Option {
	return func(settings *settings) {
		settings.maxIdle = mi
	}
}

// WithIdleTimeout sets the idle timeout duration.
// The default is 1 minute.
func WithIdleTimeout(dur time.Duration) Option {
	return func(settings *settings) {
		settings.idleTimeout = dur
	}
}

// WithDialTimeout sets the connection timeout duration.
// The default is 3 seconds.
func WithDialTimeout(dur time.Duration) Option {
	return func(settings *settings) {
		settings.dialTimeout = dur
	}
}

// WithReadTimeout sets the read timeout duration.
// The default is 3 seconds.
func WithReadTimeout(dur time.Duration) Option {
	return func(settings *settings) {
		settings.readTimeout = dur
	}
}

// WithWriteTimeout sets the write timeout duration.
// The default is 3 seconds.
func WithWriteTimeout(dur time.Duration) Option {
	return func(settings *settings) {
		settings.writeTimeout = dur
	}
}

// WithUseTLS sets the use of TLS for the redis connection.
func WithUseTLS(useTLS bool) Option {
	return func(settings *settings) {
		settings.useTLS = useTLS
	}
}
