package mariademo

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/oxynote/oxynote/datagen/internal/demodata"
	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
)

const _backfillDays = 30

// Generator inserts demo time-series data into MariaDB.
type Generator struct {
	dsn string
	r   *rand.Rand
	log *slog.Logger
}

// NewGenerator creates a new demo data generator for MariaDB.
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
	db, err := sql.Open("mysql", g.dsn)
	if err != nil {
		return fmt.Errorf("mariademo: open: %w", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Second)

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("mariademo: ping: %w", err)
	}

	backfilled, err := g.needsBackfill(ctx, db)
	if err != nil {
		return fmt.Errorf("mariademo: check backfill: %w", err)
	}

	if !backfilled {
		g.log.Info("mariademo: backfilling historical data", slog.Int("days", _backfillDays))

		if err := g.backfill(ctx, db); err != nil {
			return fmt.Errorf("mariademo: backfill: %w", err)
		}

		g.log.Info("mariademo: backfill complete")
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(demodata.TickInterval):
			now := time.Now().UTC()

			if err := g.insertTick(ctx, db, now); err != nil {
				g.log.Error("mariademo: tick insert failed", slog.String("error", err.Error()))
			}
		}
	}
}

func (g *Generator) needsBackfill(ctx context.Context, db *sql.DB) (bool, error) {
	var count int

	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deployments").Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (g *Generator) backfill(ctx context.Context, db *sql.DB) error {
	now := time.Now().UTC()
	start := now.AddDate(0, 0, -_backfillDays)

	for t := start; t.Before(now); t = t.Add(demodata.TickInterval) {
		if err := g.insertTick(ctx, db, t); err != nil {
			return err
		}
	}

	return nil
}

func (g *Generator) insertTick(ctx context.Context, db *sql.DB, t time.Time) error {
	tick := demodata.GenerateTick(g.r, t)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx at %s: %w", t.Format(time.RFC3339), err)
	}
	defer tx.Rollback()

	for _, d := range tick.Deployments {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO deployments (time, service, environment, duration_seconds, success, rollback) VALUES (?, ?, ?, ?, ?, ?)",
			d.Time, d.Service, d.Environment, d.DurationSeconds, d.Success, d.Rollback,
		); err != nil {
			return fmt.Errorf("insert deployment at %s: %w", t.Format(time.RFC3339), err)
		}
	}

	for _, i := range tick.Incidents {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO incidents (time, severity, service, time_to_detect_minutes, time_to_resolve_minutes) VALUES (?, ?, ?, ?, ?)",
			i.Time, i.Severity, i.Service, i.TimeToDetectMinutes, i.TimeToResolveMinutes,
		); err != nil {
			return fmt.Errorf("insert incident at %s: %w", t.Format(time.RFC3339), err)
		}
	}

	for _, b := range tick.BuildMetrics {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO build_metrics (time, repository, branch, duration_seconds, test_count, tests_failed, coverage_pct) VALUES (?, ?, ?, ?, ?, ?, ?)",
			b.Time, b.Repository, b.Branch, b.DurationSeconds, b.TestCount, b.TestsFailed, b.CoveragePct,
		); err != nil {
			return fmt.Errorf("insert build_metric at %s: %w", t.Format(time.RFC3339), err)
		}
	}

	return tx.Commit()
}
