// Package sentryutil provides a utility to setup sentry.
package sentryutil

import (
	"encoding/json"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jellydator/ttlcache/v3"
)

// _flushTimeout specifies how long the returned closer waits for
// buffered sentry events to be delivered.
const _flushTimeout = 2 * time.Second

// Config represents the configuration of sentry.
type Config struct {
	// DSN specifies the dsn to connect to sentry.
	DSN string

	// Release specifies the release version of the application.
	Release string

	// Environment specifies the environment of the application.
	Environment string

	// ReleaseName specifies the name of the application.
	ReleaseName string

	// ReleaseCommit specifies the commit of the application.
	ReleaseCommit string

	// ReleaseTimestamp specifies the timestamp of the application.
	ReleaseTimestamp string

	// Deduplication specifies the deduplication configuration.
	Deduplication DeduplicationConfig
}

// DeduplicationConfig represents the configuration of deduplication.
type DeduplicationConfig struct {
	// Interval specifies the deduplication interval.
	// If it is set to 0, deduplication will be disabled.
	Interval time.Duration

	// Capacity specifies the capacity of the cache.
	Capacity uint64
}

// Setup initializes sentry and returns a cleanup function.
func Setup(cfg Config) (func(), error) {
	var dc *ttlcache.Cache[string, any]

	if cfg.Deduplication.Interval != 0 {
		dc = ttlcache.New(
			ttlcache.WithTTL[string, any](cfg.Deduplication.Interval),
			ttlcache.WithDisableTouchOnHit[string, any](),
			ttlcache.WithCapacity[string, any](cfg.Deduplication.Capacity),
		)
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         cfg.DSN,
		Release:     cfg.Release,
		Environment: cfg.Environment,
		// every category the SDK may attach, stated in full: an
		// omitted field takes a default that collects. Errors and
		// stack traces only, since anything else here would carry
		// document content. The same set is spelled out in the three
		// node services.
		DataCollection: &sentry.DataCollection{
			UserInfo: sentry.Set(false),
			Cookies: &sentry.KeyValueCollectionBehavior{
				Mode: sentry.CollectionOff,
			},
			HTTPHeaders: &sentry.HeaderCollectionConfig{
				Request: &sentry.KeyValueCollectionBehavior{
					Mode: sentry.CollectionOff,
				},
				Response: &sentry.KeyValueCollectionBehavior{
					Mode: sentry.CollectionOff,
				},
			},
			HTTPBodies: []sentry.BodyType{},
			QueryParams: &sentry.KeyValueCollectionBehavior{
				Mode: sentry.CollectionOff,
			},
		},
		// nothing in this codebase emits Sentry Logs, so keep the
		// default-on telemetry machinery switched off.
		DisableLogs: true,
		BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// NOTE: Remove default info.
			e.Modules = nil

			// NOTE: In case the deduplication is enabled, we marshal the
			// exception array as it includes the error and the stacktrace.
			// We use it as a key to check if the error has been sent before.
			if dc != nil {
				data, err := json.Marshal(e.Exception)
				if err == nil {
					if dc.Has(string(data)) {
						return nil
					}

					dc.Set(string(data), nil, ttlcache.DefaultTTL)
				}
			}

			return e
		},
	})
	if err != nil {
		return nil, err
	}

	sentry.ConfigureScope(func(scope *sentry.Scope) {
		if cfg.ReleaseName != "" {
			scope.SetTag("release.name", cfg.ReleaseName)
		}

		if cfg.ReleaseCommit != "" {
			scope.SetTag("release.commit", cfg.ReleaseCommit)
		}

		if cfg.ReleaseTimestamp != "" {
			scope.SetTag("release.timestamp", cfg.ReleaseTimestamp)
		}
	})

	if dc != nil {
		go dc.Start()
	}

	return func() {
		sentry.Flush(_flushTimeout)

		if dc != nil {
			dc.Stop()
		}
	}, nil
}
