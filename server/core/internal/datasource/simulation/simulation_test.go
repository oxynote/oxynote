package simulation

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/common/model"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const (
	// _blockUID names the metric block every case addresses.
	_blockUID = "block-1"

	// _organizationID is the organization every case belongs to.
	_organizationID = "org-1"
)

// metricAttrs builds the attributes of a simulated metric block, which
// every case starts from and then narrows.
func metricAttrs() document.Attributes {
	return document.Attributes{
		document.AttrUID:               _blockUID,
		document.AttrDataSourceID:      xid.New().String(),
		document.AttrVisualizationType: string(processor.ChartTypeLine),
		document.AttrQueries: []any{
			map[string]any{"name": "Query 1", "query": "up", "legendFormat": ""},
		},
		document.AttrSimulationPreset: "cpu_usage",
	}
}

// metricBlock is the block the run layer hands the checker.
func metricBlock(attrs document.Attributes) document.Block {
	return document.Block{Type: document.BlockNodeMetricBlock, Attrs: attrs}
}

// promRunner hands out a Prometheus client answering with the given
// result, or refusing to hand one out at all when the client is nil.
func promRunner(client *datasourceMock.Prometheus) *datasourceMock.Runner {
	return &datasourceMock.Runner{
		TypeFunc: func() datasourceCore.Type { return datasourceCore.TypePrometheus },
		PrometheusFunc: func(context.Context) (datasourceCore.Prometheus, error) {
			if client == nil {
				return nil, assert.AnError
			}

			return client, nil
		},
	}
}

// sqlRunner hands out a client of the given dialect answering with one
// row, which is what "the real data has arrived" looks like for SQL.
func sqlRunner(typ datasourceCore.Type) *datasourceMock.Runner {
	rows := [][]any{{1700000000.0, 42.0}}

	return &datasourceMock.Runner{
		TypeFunc: func() datasourceCore.Type { return typ },
		PostgreSQLFunc: func(context.Context) (datasourceCore.PostgreSQL, error) {
			return &datasourceMock.PostgreSQL{
				QueryFunc: func(context.Context, string, processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
					return &processor.PostgreSQLQueryResult{
						Columns: []string{"time", "value"},
						Rows:    rows,
					}, nil
				},
			}, nil
		},
		MySQLFunc: func(context.Context) (datasourceCore.MySQL, error) {
			return &datasourceMock.MySQL{
				QueryFunc: func(context.Context, string, processor.TimeRange) (*processor.MySQLQueryResult, error) {
					return &processor.MySQLQueryResult{
						Columns: []string{"time", "value"},
						Rows:    rows,
					}, nil
				},
			}, nil
		},
	}
}

// matrixResult is a Prometheus range answer carrying one sample, which
// is what "the real data has arrived" looks like.
func matrixResult() *processor.PrometheusQueryResult {
	return &processor.PrometheusQueryResult{
		Type: model.ValMatrix,
		Result: model.Matrix{
			&model.SampleStream{
				Metric: model.Metric{"instance": "web-1"},
				Values: []model.SamplePair{{Timestamp: 1700000000000, Value: 1}},
			},
		},
	}
}

// emptyMatrixResult is a range answer with no series at all.
func emptyMatrixResult() *processor.PrometheusQueryResult {
	return &processor.PrometheusQueryResult{
		Type:   model.ValMatrix,
		Result: model.Matrix{},
	}
}

// deps assembles a checker over the given collaborators, defaulting the
// ones a case does not care about.
type deps struct {
	db      *DBMock
	runners *RunnersMock
	applier *ApplierMock
	checker *Checker
}

func newDeps(runner datasourceCore.Runner) *deps {
	typ := datasourceCore.TypePrometheus
	if runner != nil {
		typ = runner.Type()
	}

	d := &deps{
		db: &DBMock{
			FetchDataSourceFunc: func(_ context.Context, id xid.ID, _ string) (*datasourceCore.DataSource, error) {
				return &datasourceCore.DataSource{ID: id, Type: typ}, nil
			},
		},
		runners: &RunnersMock{
			RunnerFunc: func(datasourceCore.DataSource) datasourceCore.Runner {
				return runner
			},
		},
		applier: &ApplierMock{},
	}

	d.checker = NewChecker(slog.New(slog.DiscardHandler), d.db, d.runners, d.applier)

	return d
}

func Test_Checker_Check(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	sampled := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				return matrixResult(), nil
			},
		}
	}

	cc := map[string]struct {
		Attrs         document.Attributes
		Block         *document.Block
		Prometheus    *datasourceMock.Prometheus
		Runner        datasourceCore.Runner
		DataSourceErr bool
		ApplyResult   edit.Result
		ApplyErr      error
		Calls         int
		Cleared       bool
		Err           error
		ApplyCalls    int
		QueryCalls    int
		ProbeSpan     time.Duration
		AssertClearOp bool
	}{
		"The real data has arrived": {
			Attrs:         metricAttrs(),
			Prometheus:    sampled(),
			Cleared:       true,
			ApplyCalls:    1,
			QueryCalls:    1,
			AssertClearOp: true,
		},
		"A scalar answers as well as a range": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return &processor.PrometheusQueryResult{
						Type:   model.ValScalar,
						Result: &model.Scalar{Timestamp: 1700000000000, Value: 3},
					}, nil
				},
			},
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
		},
		"An instant vector answers as well as a range": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return &processor.PrometheusQueryResult{
						Type: model.ValVector,
						Result: model.Vector{
							&model.Sample{
								Metric:    model.Metric{"instance": "web-1"},
								Timestamp: 1700000000000,
								Value:     7,
							},
						},
					}, nil
				},
			},
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
		},
		// a block set to a window in the past would otherwise be probed
		// over a window its new data can never land in. It keeps the
		// span and gives up the past.
		"A past window is probed over its span, ending now": {
			Attrs:      withAttr(document.AttrTimeRange, "previous_week"),
			Prometheus: sampled(),
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
			ProbeSpan:  7 * 24 * time.Hour,
		},
		// the span also decides how densely the source is sampled, and a
		// metric minutes old is only visible to a short one: the step is
		// the span over a hundred points.
		"A short window is probed over minutes, so new data is visible at once": {
			Attrs:      withAttr(document.AttrTimeRange, "last_5_minutes"),
			Prometheus: sampled(),
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
			ProbeSpan:  5 * time.Minute,
		},
		"A block naming no window is probed over a day": {
			Attrs:      withoutAttr(document.AttrTimeRange),
			Prometheus: sampled(),
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
			ProbeSpan:  24 * time.Hour,
		},
		"A window this build does not know is probed over a day": {
			Attrs:      withAttr(document.AttrTimeRange, "since_forever"),
			Prometheus: sampled(),
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
			ProbeSpan:  24 * time.Hour,
		},
		"PostgreSQL answers too": {
			Attrs:      metricAttrs(),
			Runner:     sqlRunner(datasourceCore.TypePostgreSQL),
			Cleared:    true,
			ApplyCalls: 1,
		},
		"MySQL answers too": {
			Attrs:      metricAttrs(),
			Runner:     sqlRunner(datasourceCore.TypeMySQL),
			Cleared:    true,
			ApplyCalls: 1,
		},
		"MariaDB answers too": {
			Attrs:      metricAttrs(),
			Runner:     sqlRunner(datasourceCore.TypeMariaDB),
			Cleared:    true,
			ApplyCalls: 1,
		},
		"The queries answer with nothing": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return emptyMatrixResult(), nil
				},
			},
			QueryCalls: 1,
		},
		"The answering series carries no points": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return &processor.PrometheusQueryResult{
						Type: model.ValMatrix,
						Result: model.Matrix{
							&model.SampleStream{Metric: model.Metric{"instance": "web-1"}},
						},
					}, nil
				},
			},
			QueryCalls: 1,
		},
		"The data source answers with nothing at all": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				//nolint:nilnil // a client that found nothing answers exactly this
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return nil, nil
				},
			},
			QueryCalls: 1,
		},
		"The query is refused": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return nil, assert.AnError
				},
			},
			QueryCalls: 1,
		},
		"The data source hands out no client": {
			Attrs:  metricAttrs(),
			Runner: promRunner(nil),
		},
		"The data source is gone": {
			Attrs:         metricAttrs(),
			Prometheus:    sampled(),
			DataSourceErr: true,
		},
		"The data source cannot be queried at all": {
			Attrs: metricAttrs(),
			Runner: &datasourceMock.Runner{
				TypeFunc: func() datasourceCore.Type { return datasourceCore.Type("carrier-pigeon") },
			},
		},
		"The block names no data source": {
			Attrs:      withoutAttr(document.AttrDataSourceID),
			Prometheus: sampled(),
		},
		"The data source id is not an id": {
			Attrs:      withAttr(document.AttrDataSourceID, "not-an-xid"),
			Prometheus: sampled(),
		},
		"The data source id is not a string": {
			Attrs:      withAttr(document.AttrDataSourceID, 42),
			Prometheus: sampled(),
		},
		"The queries attribute is not an array": {
			Attrs:      withAttr(document.AttrQueries, "up"),
			Prometheus: sampled(),
		},
		"A query row is not an object": {
			Attrs:      withAttr(document.AttrQueries, []any{"up"}),
			Prometheus: sampled(),
		},
		"The block carries no queries at all": {
			Attrs:      withoutAttr(document.AttrQueries),
			Prometheus: sampled(),
		},
		"No query has been written yet": {
			Attrs: withAttr(document.AttrQueries, []any{
				map[string]any{"name": "Query 1", "query": "", "legendFormat": ""},
			}),
			Prometheus: sampled(),
		},
		"The block is not simulating": {
			Attrs:      withoutAttr(document.AttrSimulationPreset),
			Prometheus: sampled(),
			Err:        ErrNotSimulated,
		},
		"The block carries no uid": {
			Attrs:      withoutAttr(document.AttrUID),
			Prometheus: sampled(),
			Err:        ErrNotSimulated,
		},
		// the window is what bounds how often a data source is reached,
		// however many readers are looking at the block.
		"A second reader within the window is answered from the verdict": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return emptyMatrixResult(), nil
				},
			},
			Calls:      3,
			QueryCalls: 1,
		},
		"A cleared verdict is not cleared again": {
			Attrs:      metricAttrs(),
			Prometheus: sampled(),
			Calls:      2,
			Cleared:    true,
			ApplyCalls: 1,
			QueryCalls: 1,
		},
		"The realtime service cannot be reached": {
			Attrs:      metricAttrs(),
			Prometheus: sampled(),
			ApplyErr:   assert.AnError,
			Err:        assert.AnError,
			ApplyCalls: 1,
			QueryCalls: 1,
		},
		"The operation is refused": {
			Attrs:       metricAttrs(),
			Prometheus:  sampled(),
			ApplyResult: edit.Result{Errors: []edit.OpError{{Index: 0, Message: "block_uid not found"}}},
			Err:         assert.AnError,
			ApplyCalls:  1,
			QueryCalls:  1,
		},
		// a failed clear leaves the block simulating, and remembering
		// that is what stops the next tick from probing and writing again.
		// the second reader is answered from the remembered verdict, so
		// the failure is reported once rather than on every tick.
		"A failed clear is not retried within the window": {
			Attrs:      metricAttrs(),
			Prometheus: sampled(),
			ApplyErr:   assert.AnError,
			Calls:      2,
			ApplyCalls: 1,
			QueryCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runner := c.Runner
			if runner == nil {
				runner = promRunner(c.Prometheus)
			}

			d := newDeps(runner)

			if c.DataSourceErr {
				d.db.FetchDataSourceFunc = func(context.Context, xid.ID, string) (*datasourceCore.DataSource, error) {
					return nil, assert.AnError
				}
			}

			d.applier.ApplyFunc = func(context.Context, xid.ID, xid.ID, []edit.Operation, bool) (edit.Result, error) {
				return c.ApplyResult, c.ApplyErr
			}

			probed := metricBlock(c.Attrs)
			if c.Block != nil {
				probed = *c.Block
			}

			calls := max(c.Calls, 1)

			var (
				res Result
				err error
			)

			for range calls {
				res, err = d.checker.Check(
					context.Background(),
					documentID,
					branchID,
					probed,
					_organizationID,
				)
			}

			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Cleared, res.Cleared)
			assert.Len(t, d.applier.ApplyCalls(), c.ApplyCalls)

			if c.Prometheus != nil {
				assert.Len(t, c.Prometheus.QueryRangeCalls(), c.QueryCalls)
			}

			if c.AssertClearOp {
				call := d.applier.ApplyCalls()[0]

				assert.Equal(t, documentID, call.DocumentID)
				assert.Equal(t, branchID, call.BranchID)

				// the clear is core's own write: an ordinary one
				// would be refused on a protected branch.
				assert.True(t, call.System)
				require.Len(t, call.Ops, 1)

				op, opErr := call.Ops[0]()
				require.NoError(t, opErr)

				raw, jerr := json.Marshal(op)
				require.NoError(t, jerr)

				assert.JSONEq(
					t,
					`{"kind":"update_attrs","block_uid":"block-1","attrs":{"simulationPreset":null}}`,
					string(raw),
				)
			}

			if c.ProbeSpan > 0 {
				now := timeutil.Now()
				probed := c.Prometheus.QueryRangeCalls()[0].Tr

				assert.WithinDuration(t, now, probed.To, time.Minute)
				assert.WithinDuration(
					t,
					now.Add(-c.ProbeSpan),
					probed.From,
					time.Minute,
				)
			}
		})
	}
}

// the spans and the assistant's list are two enumerations of the editor's
// presets, and probeSpan answers an unknown one with the default rather
// than an error — so a preset renamed in one place and not the other
// would quietly probe the wrong window.
func Test_probeSpan(t *testing.T) {
	t.Parallel()

	known := block.MetricEnums()[document.AttrTimeRange]

	assert.Len(t, _probeSpans, len(known))

	for _, preset := range known {
		span, ok := _probeSpans[preset]

		assert.True(t, ok, preset)
		assert.Positive(t, span, preset)
	}

	assert.Equal(
		t,
		_defaultProbeSpan,
		probeSpan(metricBlock(withoutAttr(document.AttrTimeRange))),
	)
	assert.Equal(
		t,
		5*time.Minute,
		probeSpan(metricBlock(withAttr(document.AttrTimeRange, "last_5_minutes"))),
	)
}

func Test_Checker_Start(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	d := newDeps(promRunner(&datasourceMock.Prometheus{}))

	done := make(chan struct{})

	go func() {
		defer close(done)

		d.checker.Start(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after the context was cancelled")
	}
}

// withAttr returns the simulated block's attributes with one of them
// replaced.
func withAttr(key string, value any) document.Attributes {
	attrs := metricAttrs()
	attrs[key] = value

	return attrs
}

// withoutAttr returns the simulated block's attributes with one of them
// removed.
func withoutAttr(key string) document.Attributes {
	attrs := metricAttrs()
	delete(attrs, key)

	return attrs
}
