package pgdemo

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/oxynote/heimdall/datagen/internal/demodata"
	"github.com/oxynote/heimdall/datagen/internal/mockmetrics"
)

const _backfillDays = 30

// Generator inserts demo time-series data into PostgreSQL.
type Generator struct {
	dsn string
	r   *rand.Rand
	log *slog.Logger
}

// NewGenerator creates a new demo data generator.
func NewGenerator(dsn string, log *slog.Logger) *Generator {
	return &Generator{
		dsn: dsn,
		r:   mockmetrics.NewRand(time.Now().UnixNano()),
		log: log,
	}
}

// Run backfills historical data if needed, then appends new rows on each tick.
// It blocks until ctx is cancelled.
func (g *Generator) Run(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, g.dsn)
	if err != nil {
		return fmt.Errorf("pgdemo: connect: %w", err)
	}
	defer conn.Close(ctx)

	backfilled, err := g.needsBackfill(ctx, conn)
	if err != nil {
		return fmt.Errorf("pgdemo: check backfill: %w", err)
	}

	if !backfilled {
		g.log.Info("pgdemo: backfilling historical data", slog.Int("days", _backfillDays))

		if err := g.backfill(ctx, conn); err != nil {
			return fmt.Errorf("pgdemo: backfill: %w", err)
		}

		g.log.Info("pgdemo: backfill complete")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(demodata.TickInterval):
			now := time.Now().UTC()

			if err := g.insertTick(ctx, conn, now); err != nil {
				g.log.Error("pgdemo: tick insert failed", slog.String("error", err.Error()))
			}
		}
	}
}

// needsBackfill returns true if historical data already exists.
func (g *Generator) needsBackfill(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var count int

	err := conn.QueryRow(ctx, "SELECT COUNT(*) FROM deployments").Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// backfill generates _backfillDays worth of historical data.
func (g *Generator) backfill(ctx context.Context, conn *pgx.Conn) error {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -_backfillDays)

	for t := start; t.Before(now); t = t.Add(demodata.TickInterval) {
		if err := g.insertTick(ctx, conn, t); err != nil {
			return err
		}
	}

	return nil
}

// insertTick generates and inserts one tick's worth of data at the given time.
func (g *Generator) insertTick(ctx context.Context, conn *pgx.Conn, t time.Time) error {
	tick := demodata.GenerateTick(g.r, t)
	batch := &pgx.Batch{}

	for _, d := range tick.Deployments {
		batch.Queue(
			"INSERT INTO deployments (time, service, environment, duration_seconds, success, rollback) VALUES ($1, $2, $3, $4, $5, $6)",
			d.Time, d.Service, d.Environment, d.DurationSeconds, d.Success, d.Rollback,
		)
	}

	for _, i := range tick.Incidents {
		batch.Queue(
			"INSERT INTO incidents (time, severity, service, time_to_detect_minutes, time_to_resolve_minutes) VALUES ($1, $2, $3, $4, $5)",
			i.Time, i.Severity, i.Service, i.TimeToDetectMinutes, i.TimeToResolveMinutes,
		)
	}

	for _, b := range tick.BuildMetrics {
		batch.Queue(
			"INSERT INTO build_metrics (time, repository, branch, duration_seconds, test_count, tests_failed, coverage_pct) VALUES ($1, $2, $3, $4, $5, $6, $7)",
			b.Time, b.Repository, b.Branch, b.DurationSeconds, b.TestCount, b.TestsFailed, b.CoveragePct,
		)
	}

	if batch.Len() == 0 {
		return nil
	}

	br := conn.SendBatch(ctx, batch)
	defer br.Close()

	for range batch.Len() {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("batch exec at %s: %w", t.Format(time.RFC3339), err)
		}
	}

	return nil
}
