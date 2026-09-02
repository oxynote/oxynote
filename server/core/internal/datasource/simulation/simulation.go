// Package simulation decides when a metric block that draws generated
// data can go back to drawing its data source's.
package simulation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/memkit"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// ErrNotSimulated is returned when the addressed metric block is not
// drawing generated data, so there is nothing to check.
var ErrNotSimulated = errutil.New(
	http.StatusNotFound,
	"metric_block.not_simulated",
	"This metric block is not simulating data.",
)

// Result is what running a simulated metric block reports back.
type Result struct {
	// Cleared reports whether the block's real data has arrived and the
	// simulation has been taken off it.
	Cleared bool `json:"cleared"`
}

// _resultTTL is how long one block's verdict answers the readers that ask
// after it. Several people viewing the same document ask on their own
// cadence, and each ask that finds a verdict is one that does not reach
// the data source; asks that miss together all probe, since nothing
// coordinates the ones in flight.
var _resultTTL = 5 * time.Second

// _defaultProbeSpan is how far back a probe looks when the block names no
// window, or names one this build does not know.
const _defaultProbeSpan = 24 * time.Hour

// _probeSpans is how far back a probe looks for each window the editor
// offers, mirroring web's TimeRangePreset. The span is taken from the
// block but the window always ends now, so a block set to a past window
// (previous week, yesterday) is not probed over a window its new data can
// never land in and left simulating for good.
//
// The cost is that for those presets the probe and the chart no longer
// look at the same stretch of time: a block drawing last calendar year
// clears as soon as the metric exists, and then draws its own window,
// which is empty. A relative window, which is what a block documenting a
// new metric carries, has no such gap.
//
// The span also decides how densely the source is sampled: the query step
// is the span over a hundred points, rounded up to a whole interval, so a
// five-minute block is sampled every fifteen seconds and notices a metric
// seconds old, while a thirty-day block is sampled every twelve hours,
// which is all a thirty-day chart could show anyway.
//
//nolint:mnd // the spans are the table; naming each one would bury it
var _probeSpans = map[string]time.Duration{
	"last_5_minutes":    5 * time.Minute,
	"last_15_minutes":   15 * time.Minute,
	"last_30_minutes":   30 * time.Minute,
	"last_1_hour":       time.Hour,
	"last_3_hours":      3 * time.Hour,
	"last_6_hours":      6 * time.Hour,
	"last_12_hours":     12 * time.Hour,
	"last_24_hours":     24 * time.Hour,
	"last_2_days":       2 * 24 * time.Hour,
	"last_7_days":       7 * 24 * time.Hour,
	"last_30_days":      30 * 24 * time.Hour,
	"last_90_days":      90 * 24 * time.Hour,
	"last_6_months":     180 * 24 * time.Hour,
	"last_1_year":       365 * 24 * time.Hour,
	"last_2_years":      730 * 24 * time.Hour,
	"last_5_years":      1825 * 24 * time.Hour,
	"today":             24 * time.Hour,
	"today_so_far":      24 * time.Hour,
	"yesterday":         24 * time.Hour,
	"this_week":         7 * 24 * time.Hour,
	"this_week_so_far":  7 * 24 * time.Hour,
	"previous_week":     7 * 24 * time.Hour,
	"this_month":        30 * 24 * time.Hour,
	"this_month_so_far": 30 * 24 * time.Hour,
	"previous_month":    30 * 24 * time.Hour,
	"this_year":         365 * 24 * time.Hour,
	"this_year_so_far":  365 * 24 * time.Hour,
	"previous_year":     365 * 24 * time.Hour,
}

// _queryQuery is the query text inside one of a metric block's query
// rows.
const _queryQuery = "query"

// Checker probes the data source of a simulated metric block and takes
// the simulation off the block once real data answers.
type Checker struct {
	// log is the component logger.
	log *slog.Logger

	// db reads the data source the block names.
	db DB

	// runners hands out the client that speaks to a data source.
	runners Runners

	// applier writes the cleared attribute onto the live document.
	applier Applier

	// results remembers each block's last verdict for _resultTTL.
	results *memkit.ValueStore[bool]
}

// NewChecker creates a fresh instance of Checker.
func NewChecker(
	log *slog.Logger,
	db DB,
	runners Runners,
	applier Applier,
) *Checker {
	return &Checker{
		log:     log.With("component", "metric-simulation"),
		db:      db,
		runners: runners,
		applier: applier,
		results: memkit.NewValueStore[bool](_resultTTL),
	}
}

// Start runs the maintenance of the verdict store until ctx is
// cancelled. It blocks, so the caller owns the goroutine it runs on.
func (c *Checker) Start(ctx context.Context) {
	c.results.Start(ctx)
}

// Check reports whether the simulation was taken off the block, probing
// the block's data source when no recent verdict is remembered. A data
// source that cannot be reached, or that answers with nothing, leaves the
// block simulating and is not an error: it is the state the simulation
// exists for.
func (c *Checker) Check(
	ctx context.Context,
	documentID, branchID xid.ID,
	block document.Block,
	organizationID string,
) (Result, error) {
	if _, simulated := block.Attrs.Value(document.AttrSimulationPreset); !simulated {
		return Result{}, ErrNotSimulated
	}

	blockUID, ok := block.UID()
	if !ok {
		return Result{}, ErrNotSimulated
	}

	key := cacheKey(documentID, branchID, blockUID)

	if cleared, err := c.results.Get(ctx, key); err == nil {
		return Result{Cleared: *cleared}, nil
	} else if !errors.Is(err, errutil.ErrNotFound) {
		// NOCOV: the store only fails to decode what it did not encode,
		// and the only thing written here is a bool.
		c.log.WarnContext(ctx, "cannot read the remembered verdict", "error", err)
	}

	cleared := c.checkMetricActive(ctx, block, organizationID)

	if cleared {
		if err := c.clear(ctx, documentID, branchID, blockUID); err != nil {
			// the block is still simulating, and remembering that is what
			// keeps a realtime service that cannot be reached from being
			// probed and written to on every reader's next tick.
			c.remember(ctx, key, false)

			return Result{}, err
		}
	}

	c.remember(ctx, key, cleared)

	return Result{Cleared: cleared}, nil
}

// checkMetricActive reports whether any of the block's queries answers
// with at least one point. Everything that stands between the block and
// its data — a deleted data source, a refused connection, a query the
// source rejects — reads as "not yet", which is what keeps the block
// simulating.
func (c *Checker) checkMetricActive(
	ctx context.Context,
	block document.Block,
	organizationID string,
) bool {
	ds, err := c.fetchDataSource(ctx, block, organizationID)
	if err != nil {
		c.log.DebugContext(ctx, "cannot reach the block's data source", "error", err)

		return false
	}

	now := timeutil.Now()
	tr := processor.TimeRange{From: now.Add(-probeSpan(block)), To: now}

	for _, q := range extractQueries(block) {
		res, qerr := c.query(ctx, *ds, q, tr)
		if qerr != nil {
			c.log.DebugContext(ctx, "the block's query did not answer", "error", qerr)

			continue
		}

		if res.HasData() {
			return true
		}
	}

	return false
}

// fetchDataSource retrieves the data source the block names.
func (c *Checker) fetchDataSource(
	ctx context.Context,
	block document.Block,
	organizationID string,
) (*datasourceCore.DataSource, error) {
	raw, ok := block.Attrs.Value(document.AttrDataSourceID)
	if !ok {
		return nil, errors.New("the block names no data source")
	}

	str, isString := raw.(string)
	if !isString {
		return nil, fmt.Errorf("data source id is %T, not a string", raw)
	}

	id, err := xid.FromString(str)
	if err != nil {
		return nil, fmt.Errorf("data source id: %w", err)
	}

	return c.db.FetchDataSource(ctx, id, organizationID)
}

// query runs one query against the data source and returns it in the
// unified shape, whatever the source speaks. It transforms as a gauge
// because that shape accepts every result a source can answer with —
// scalar, vector and matrix alike — and the probe only asks whether
// anything came back, not how it would be drawn.
func (c *Checker) query(
	ctx context.Context,
	ds datasourceCore.DataSource,
	q string,
	tr processor.TimeRange,
) (*processor.QueryResult, error) {
	runner := c.runners.Runner(ds)

	switch ds.Type {
	case datasourceCore.TypePrometheus:
		client, err := runner.Prometheus(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.QueryRange(ctx, q, tr)
		if err != nil || res == nil {
			return nil, err
		}

		return res.Transform(processor.ChartTypeGauge), nil
	case datasourceCore.TypePostgreSQL:
		client, err := runner.PostgreSQL(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.Query(ctx, q, tr)
		if err != nil || res == nil {
			return nil, err
		}

		return res.Transform(processor.ChartTypeGauge), nil
	case datasourceCore.TypeMariaDB, datasourceCore.TypeMySQL:
		client, err := runner.MySQL(ctx)
		if err != nil {
			return nil, err
		}

		res, err := client.Query(ctx, q, tr)
		if err != nil || res == nil {
			return nil, err
		}

		return res.Transform(processor.ChartTypeGauge), nil
	default:
		return nil, fmt.Errorf("data source type %q cannot be queried", ds.Type)
	}
}

// clear takes the simulation off the block on the live document.
func (c *Checker) clear(
	ctx context.Context,
	documentID, branchID xid.ID,
	blockUID string,
) error {
	res, err := c.applier.Apply(ctx, documentID, branchID, []edit.Operation{
		edit.UpdateAttrs(blockUID, map[string]any{
			document.AttrSimulationPreset: nil,
		}),
	}, true)
	if err != nil {
		return err
	}

	if len(res.Errors) > 0 {
		return fmt.Errorf("cannot clear the simulation: %s", res.Errors[0].Message)
	}

	return nil
}

// remember stores a verdict for the readers that follow within the
// window.
func (c *Checker) remember(ctx context.Context, key string, cleared bool) {
	if err := c.results.Set(ctx, key, cleared); err != nil {
		// NOCOV: the store only fails on a value it cannot encode, and a
		// bool always encodes.
		c.log.WarnContext(ctx, "cannot remember the verdict", "error", err)
	}
}

// cacheKey names one block's verdict. A block only exists on the branch
// it was written on, so the branch is part of its identity here.
func cacheKey(documentID, branchID xid.ID, blockUID string) string {
	return documentID.String() + "/" + branchID.String() + "/" + blockUID
}

// probeSpan reads how far back the block's window reaches.
func probeSpan(block document.Block) time.Duration {
	raw, ok := block.Attrs.Value(document.AttrTimeRange)
	if !ok {
		return _defaultProbeSpan
	}

	preset, isString := raw.(string)
	if !isString {
		return _defaultProbeSpan
	}

	span, known := _probeSpans[preset]
	if !known {
		return _defaultProbeSpan
	}

	return span
}

// extractQueries reads the query texts the block carries, skipping the
// rows nobody has written a query into yet.
func extractQueries(block document.Block) []string {
	raw, ok := block.Attrs.Value(document.AttrQueries)
	if !ok {
		return nil
	}

	rows, ok := raw.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(rows))

	for _, r := range rows {
		row, isObject := r.(map[string]any)
		if !isObject {
			continue
		}

		if q, isString := row[_queryQuery].(string); isString && q != "" {
			out = append(out, q)
		}
	}

	return out
}

// DB reads what a simulated block needs to be checked.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDataSource should retrieve a data source by ID and
	// organization ID.
	FetchDataSource(ctx context.Context, id xid.ID, organizationID string) (*datasourceCore.DataSource, error)
}

// Runners hands out the runner for a data source. The datasource
// package's Manager satisfies it.
//
//go:generate ../../../scripts/codegen/mock -t internal Runners runners
type Runners interface {
	// Runner should return the runner that operates the given data
	// source.
	Runner(ds datasourceCore.DataSource) datasourceCore.Runner
}

// Applier writes operations onto a live document on core's own behalf.
//
//go:generate ../../../scripts/codegen/mock -t internal Applier applier
type Applier interface {
	// Apply should ship the operation batch to the realtime service for
	// the (documentID, branchID) document and return the per-op
	// outcome. The clear is core's own write, so this package always
	// asks for a system one.
	Apply(ctx context.Context, documentID, branchID xid.ID, ops []edit.Operation, system bool) (edit.Result, error)
}
