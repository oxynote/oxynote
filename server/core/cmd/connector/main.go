package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/buildinfo"
	"github.com/oxynote/oxynote/server/core/internal/connector"
	"github.com/oxynote/purse/util/ioutil"
	"github.com/oxynote/purse/util/logutil"
	"github.com/oxynote/purse/util/metricutil"
	"github.com/oxynote/purse/util/sentryutil"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	var closers []io.Closer

	log, flushLogs, ok := newLogger()
	if !ok {
		return
	}

	closers = append(closers, ioutil.CloserFunc(func() error {
		flushLogs()
		return nil
	}))

	metrics, cleanup := newMetrics(log)
	defer cleanup()

	termCtx, termCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer termCancel()

	srv, err := connector.NewServer(
		termCtx,
		log,
		connector.Options{
			Port: 8080,
		},
		metrics,
	)
	if err != nil {
		err = ioutil.AppendCloseErr(
			ioutil.MultiCloser(true, closers...),
			err,
		)

		log.With("error", err).
			Error("cannot create a server")

		return
	}

	closers = append([]io.Closer{srv}, closers...)

	var wg sync.WaitGroup

	wg.Go(srv.Listen)

	<-termCtx.Done()

	cerr := ioutil.MultiCloser(true, closers...).Close()
	if cerr != nil {
		log.With("error", cerr).
			Error("error closing resources")
	}

	wg.Wait()
}

// newLogger initializes a new logger with Sentry support.
func newLogger() (*slog.Logger, func(), bool) {
	var level slog.Level

	buildLevel := buildinfo.Getenv("LOG_LEVEL")
	if buildLevel != "" {
		err := level.UnmarshalText([]byte(buildLevel))
		if err != nil {
			slog.Warn(`invalid log level, defaulting to "INFO"`)
		}
	}

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})).With("version", buildinfo.Full().Version)

	dsn := buildinfo.Getenv("SENTRY_DSN")
	if dsn == "" && !buildinfo.IsDevEnv() {
		log.Error("invalid sentry dsn")
		return nil, nil, false
	}

	fn, err := sentryutil.Setup(sentryutil.Config{
		DSN:              dsn,
		Release:          buildinfo.Full().PlainVersion(),
		Environment:      buildinfo.Full().Env,
		ReleaseName:      buildinfo.Full().VersionName(),
		ReleaseCommit:    buildinfo.Full().Commit,
		ReleaseTimestamp: buildinfo.Full().FormattedTimestamp(),
		Deduplication: sentryutil.DeduplicationConfig{
			Interval: 10 * time.Minute,
			Capacity: 100,
		},
	})
	if err != nil {
		log.Error("cannot set up sentry")
		return nil, nil, false
	}

	return log, fn, true
}

// newMetrics initializes and starts the metrics collection.
func newMetrics(log *slog.Logger) (metricutil.Factory, func()) {
	metricsStopCh := make(chan struct{})

	metrics := metricutil.NewFactory(buildinfo.Full().Name, prometheus.NewRegistry())

	metricsCtx, metricsCancel := context.WithCancel(context.Background())

	go func() {
		defer logutil.Recover(log, nil)

		metrics.CollectRuntimeMetrics(metricsCtx, time.Second)

		close(metricsStopCh)
	}()

	return metrics, func() {
		metricsCancel()
		<-metricsStopCh
	}
}
