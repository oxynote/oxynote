// Package main runs the demo data generator: it serves mock engineering
// metrics for Prometheus to scrape and fills the demo databases with rows.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/oxynote/oxynote/datagen/internal/gen"
	"github.com/oxynote/oxynote/datagen/internal/mariademo"
	"github.com/oxynote/oxynote/datagen/internal/pgdemo"
	"github.com/oxynote/oxynote/datagen/internal/server"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	log := newLogger()

	metrics := metricutil.NewFactory(
		"engineering",
		prometheus.NewRegistry(),
		metricutil.WithCustomHost("demo"),
	)

	srv := server.New(log, metrics)

	termCtx, termCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer termCancel()

	var wg sync.WaitGroup

	wg.Go(srv.Listen)
	wg.Go(func() {
		gen.NewManager(metrics, log).Run(termCtx)
	})

	if dsn := os.Getenv("DATAGEN_POSTGRES_DSN"); dsn != "" {
		pg := pgdemo.NewGenerator(dsn, log)

		wg.Go(func() {
			if err := pg.Run(termCtx); err != nil {
				log.Error("pgdemo generator failed", slog.String("error", err.Error()))
			}
		})
	}

	if dsn := os.Getenv("DATAGEN_MARIADB_DSN"); dsn != "" {
		maria := mariademo.NewGenerator(dsn, log)

		wg.Go(func() {
			if err := maria.Run(termCtx); err != nil {
				log.Error("mariademo generator failed", slog.String("error", err.Error()))
			}
		})
	}

	<-termCtx.Done()

	cerr := srv.Close()
	if cerr != nil {
		log.Error("failed to close server", slog.String("error", cerr.Error()))
	}

	wg.Wait()
}

// newLogger initializes a new logger.
func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
