package simulation

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	datasourceCore "github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/variable"
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
		document.AttrSimulationActive: true,
	}
}

// sqlRunner hands out a client of the given dialect answering with one
// row, which is what "the real data has arrived" looks like for SQL. It
// also returns both clients, so a case can count their queries.
func sqlRunner(
	typ datasourceCore.Type,
) (*datasourceMock.Runner, *datasourceMock.PostgreSQL, *datasourceMock.MySQL) {
	rows := [][]any{{1700000000.0, 42.0}}

	pg := &datasourceMock.PostgreSQL{
		QueryFunc: func(context.Context, string, processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
			return &processor.PostgreSQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    rows,
			}, nil
		},
	}

	my := &datasourceMock.MySQL{
		QueryFunc: func(context.Context, string, processor.TimeRange) (*processor.MySQLQueryResult, error) {
			return &processor.MySQLQueryResult{
				Columns: []string{"time", "value"},
				Rows:    rows,
			}, nil
		},
	}

	runner := &datasourceMock.Runner{
		TypeFunc: func() datasourceCore.Type { return typ },
		PostgreSQLFunc: func(context.Context) (datasourceCore.PostgreSQL, error) {
			return pg, nil
		},
		MySQLFunc: func(context.Context) (datasourceCore.MySQL, error) {
			return my, nil
		},
	}

	return runner, pg, my
}

func Test_Checker_Check(t *testing.T) {
	t.Parallel()

	documentID, branchID := xid.New(), xid.New()

	// answers with one sample, which is what "the real data has arrived"
	// looks like.
	sampled := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				return &processor.PrometheusQueryResult{
					Type: model.ValMatrix,
					Result: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{"instance": "web-1"},
							Values: []model.SamplePair{{Timestamp: 1700000000000, Value: 1}},
						},
					},
				}, nil
			},
		}
	}

	// answers only the query with $env substituted, so a raw reference
	// keeps the block simulating.
	substituted := func() *datasourceMock.Prometheus {
		return &datasourceMock.Prometheus{
			QueryRangeFunc: func(_ context.Context, q string, _ processor.TimeRange) (*processor.PrometheusQueryResult, error) {
				if q != `up{env="prod"}` {
					return nil, assert.AnError
				}

				return &processor.PrometheusQueryResult{
					Type: model.ValMatrix,
					Result: model.Matrix{
						&model.SampleStream{
							Metric: model.Metric{"instance": "web-1"},
							Values: []model.SamplePair{{Timestamp: 1700000000000, Value: 1}},
						},
					},
				}, nil
			},
		}
	}

	referencing := withAttr(document.AttrQueries, []any{
		map[string]any{"name": "Query 1", "query": `up{env="$env"}`, "legendFormat": ""},
	})

	type tcase struct {
		Attrs                  document.Attributes
		Prometheus             *datasourceMock.Prometheus
		PostgreSQL             *datasourceMock.PostgreSQL
		MySQL                  *datasourceMock.MySQL
		Runner                 *datasourceMock.Runner
		DataSourceErr          bool
		Variables              func(context.Context, string, string) (variable.Tree, error)
		ApplyResult            edit.Result
		ApplyErr               error
		Calls                  int
		Cleared                bool
		Err                    error
		FetchDataSourceCalls   int
		FetchVariableTreeCalls int
		RunnerCalls            int
		TypeCalls              int
		PrometheusCalls        int
		PostgreSQLCalls        int
		MySQLCalls             int
		ApplyCalls             int
		QueryCalls             int
		ProbeSpan              time.Duration
		AssertClearOp          bool
	}

	cc := map[string]tcase{
		"The real data has arrived": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
			AssertClearOp:        true,
		},
		"A query is probed with the default values of its variables": {
			Attrs:      referencing,
			Prometheus: substituted(),
			// holds $env only for the block's organization and no user,
			// which is how the probe fetches the defaults.
			Variables: func(_ context.Context, organizationID, userID string) (variable.Tree, error) {
				if organizationID != _organizationID || userID != "" {
					return variable.Tree{}, nil
				}

				return variable.Tree{{Variables: []variable.Entry{
					{Key: "env", Type: variable.TypeText, DefaultValue: null.StringFrom("prod")},
				}}}, nil
			},
			Cleared:                true,
			FetchDataSourceCalls:   1,
			FetchVariableTreeCalls: 1,
			RunnerCalls:            1,
			TypeCalls:              1,
			PrometheusCalls:        1,
			ApplyCalls:             1,
			QueryCalls:             1,
		},
		"Error returned by DB.FetchVariableTree": {
			Attrs:      referencing,
			Prometheus: substituted(),
			Variables: func(context.Context, string, string) (variable.Tree, error) {
				return nil, assert.AnError
			},
			FetchDataSourceCalls:   1,
			FetchVariableTreeCalls: 1,
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
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
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
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
		},
		// a block set to a window in the past would otherwise be probed
		// over a window its new data can never land in. It keeps the
		// span and gives up the past.
		"A past window is probed over its span ending now": {
			Attrs:                withAttr(document.AttrTimeRange, "previous_week"),
			Prometheus:           sampled(),
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
			ProbeSpan:            7 * 24 * time.Hour,
		},
		// the span also decides how densely the source is sampled, and a
		// metric minutes old is only visible to a short one: the step is
		// the span over a hundred points.
		"A short window is probed over minutes so new data is visible at once": {
			Attrs:                withAttr(document.AttrTimeRange, "last_5_minutes"),
			Prometheus:           sampled(),
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
			ProbeSpan:            5 * time.Minute,
		},
		"A block naming no window is probed over a day": {
			Attrs:                withoutAttr(document.AttrTimeRange),
			Prometheus:           sampled(),
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
			ProbeSpan:            24 * time.Hour,
		},
		"A window following the built-in variable is probed over its default": {
			Attrs:      withAttr(document.AttrTimeRange, "$metric_range"),
			Prometheus: sampled(),
			Variables: func(context.Context, string, string) (variable.Tree, error) {
				return variable.Tree{{Variables: []variable.Entry{
					{Key: "metric_range", Type: variable.TypeTimeRange, DefaultValue: null.StringFrom("last_7_days")},
				}}}, nil
			},
			Cleared:                true,
			FetchDataSourceCalls:   1,
			FetchVariableTreeCalls: 1,
			RunnerCalls:            1,
			TypeCalls:              1,
			PrometheusCalls:        1,
			ApplyCalls:             1,
			QueryCalls:             1,
			ProbeSpan:              7 * 24 * time.Hour,
		},
		"Error returned by DB.FetchVariableTree for the window": {
			Attrs:      withAttr(document.AttrTimeRange, "$metric_range"),
			Prometheus: sampled(),
			Variables: func(context.Context, string, string) (variable.Tree, error) {
				return nil, assert.AnError
			},
			FetchDataSourceCalls:   1,
			FetchVariableTreeCalls: 1,
		},
		"A window this build does not know is probed over a day": {
			Attrs:                withAttr(document.AttrTimeRange, "since_forever"),
			Prometheus:           sampled(),
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
			ProbeSpan:            24 * time.Hour,
		},
		"PostgreSQL answers too": func() tcase {
			runner, pg, _ := sqlRunner(datasourceCore.TypePostgreSQL)

			return tcase{
				Attrs:                metricAttrs(),
				Runner:               runner,
				PostgreSQL:           pg,
				Cleared:              true,
				FetchDataSourceCalls: 1,
				RunnerCalls:          1,
				TypeCalls:            1,
				PostgreSQLCalls:      1,
				ApplyCalls:           1,
				QueryCalls:           1,
			}
		}(),
		"Supabase answers too": func() tcase {
			runner, pg, _ := sqlRunner(datasourceCore.TypeSupabase)

			return tcase{
				Attrs:                metricAttrs(),
				Runner:               runner,
				PostgreSQL:           pg,
				Cleared:              true,
				FetchDataSourceCalls: 1,
				RunnerCalls:          1,
				TypeCalls:            1,
				PostgreSQLCalls:      1,
				ApplyCalls:           1,
				QueryCalls:           1,
			}
		}(),
		"MySQL answers too": func() tcase {
			runner, _, my := sqlRunner(datasourceCore.TypeMySQL)

			return tcase{
				Attrs:                metricAttrs(),
				Runner:               runner,
				MySQL:                my,
				Cleared:              true,
				FetchDataSourceCalls: 1,
				RunnerCalls:          1,
				TypeCalls:            1,
				MySQLCalls:           1,
				ApplyCalls:           1,
				QueryCalls:           1,
			}
		}(),
		"MariaDB answers too": func() tcase {
			runner, _, my := sqlRunner(datasourceCore.TypeMariaDB)

			return tcase{
				Attrs:                metricAttrs(),
				Runner:               runner,
				MySQL:                my,
				Cleared:              true,
				FetchDataSourceCalls: 1,
				RunnerCalls:          1,
				TypeCalls:            1,
				MySQLCalls:           1,
				ApplyCalls:           1,
				QueryCalls:           1,
			}
		}(),
		"The queries answer with nothing": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return &processor.PrometheusQueryResult{
						Type:   model.ValMatrix,
						Result: model.Matrix{},
					}, nil
				},
			},
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			QueryCalls:           1,
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
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			QueryCalls:           1,
		},
		"The data source answers with nothing at all": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				//nolint:nilnil // a client that found nothing answers exactly this
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return nil, nil
				},
			},
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			QueryCalls:           1,
		},
		"Error returned by Prometheus.QueryRange": {
			Attrs: metricAttrs(),
			Prometheus: &datasourceMock.Prometheus{
				QueryRangeFunc: func(context.Context, string, processor.TimeRange) (*processor.PrometheusQueryResult, error) {
					return nil, assert.AnError
				},
			},
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			QueryCalls:           1,
		},
		"Error returned by Runner.Prometheus": {
			Attrs: metricAttrs(),
			Runner: &datasourceMock.Runner{
				TypeFunc: func() datasourceCore.Type { return datasourceCore.TypePrometheus },
				PrometheusFunc: func(context.Context) (datasourceCore.Prometheus, error) {
					return nil, assert.AnError
				},
			},
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
		},
		"Error returned by DB.FetchDataSource": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			DataSourceErr:        true,
			FetchDataSourceCalls: 1,
		},
		"The data source cannot be queried at all": {
			Attrs: metricAttrs(),
			Runner: &datasourceMock.Runner{
				TypeFunc: func() datasourceCore.Type { return datasourceCore.Type("carrier-pigeon") },
			},
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
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
			Attrs:                withAttr(document.AttrQueries, "up"),
			Prometheus:           sampled(),
			FetchDataSourceCalls: 1,
		},
		"A query row is not an object": {
			Attrs:                withAttr(document.AttrQueries, []any{"up"}),
			Prometheus:           sampled(),
			FetchDataSourceCalls: 1,
		},
		"The block carries no queries at all": {
			Attrs:                withoutAttr(document.AttrQueries),
			Prometheus:           sampled(),
			FetchDataSourceCalls: 1,
		},
		"No query has been written yet": {
			Attrs: withAttr(document.AttrQueries, []any{
				map[string]any{"name": "Query 1", "query": "", "legendFormat": ""},
			}),
			Prometheus:           sampled(),
			FetchDataSourceCalls: 1,
		},
		"The block names no preset": {
			Attrs:      withoutAttr(document.AttrSimulationPreset),
			Prometheus: sampled(),
			Err:        ErrNotSimulated,
		},
		"The simulation is switched off": {
			Attrs:      withAttr(document.AttrSimulationActive, false),
			Prometheus: sampled(),
			Err:        ErrNotSimulated,
		},
		"The block carries a preset and no flag": {
			Attrs:      withoutAttr(document.AttrSimulationActive),
			Prometheus: sampled(),
			Err:        ErrNotSimulated,
		},
		"The flag is not a boolean": {
			Attrs:      withAttr(document.AttrSimulationActive, "true"),
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
					return &processor.PrometheusQueryResult{
						Type:   model.ValMatrix,
						Result: model.Matrix{},
					}, nil
				},
			},
			Calls:                3,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			QueryCalls:           1,
		},
		"A cleared verdict is not cleared again": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			Calls:                2,
			Cleared:              true,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
		},
		"Error returned by Applier.Apply": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			ApplyErr:             assert.AnError,
			Err:                  assert.AnError,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
		},
		"The operation is refused": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			ApplyResult:          edit.Result{Errors: []edit.OpError{{Index: 0, Message: "block_uid not found"}}},
			Err:                  assert.AnError,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
		},
		// a failed clear leaves the block simulating, and remembering
		// that is what stops the next tick from probing and writing again.
		// the second reader is answered from the remembered verdict, so
		// the failure is reported once rather than on every tick.
		"Error returned by Applier.Apply is not retried within the window": {
			Attrs:                metricAttrs(),
			Prometheus:           sampled(),
			ApplyErr:             assert.AnError,
			Calls:                2,
			FetchDataSourceCalls: 1,
			RunnerCalls:          1,
			TypeCalls:            1,
			PrometheusCalls:      1,
			ApplyCalls:           1,
			QueryCalls:           1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runner := c.Runner
			if runner == nil {
				runner = &datasourceMock.Runner{
					TypeFunc: func() datasourceCore.Type { return datasourceCore.TypePrometheus },
					PrometheusFunc: func(context.Context) (datasourceCore.Prometheus, error) {
						return c.Prometheus, nil
					},
				}
			}

			db := &DBMock{
				FetchDataSourceFunc: func(_ context.Context, id xid.ID, _ string) (*datasourceCore.DataSource, error) {
					if c.DataSourceErr {
						return nil, assert.AnError
					}

					return &datasourceCore.DataSource{ID: id}, nil
				},
				FetchVariableTreeFunc: c.Variables,
			}

			runners := &RunnersMock{
				RunnerFunc: func(datasourceCore.DataSource) datasourceCore.Runner {
					return runner
				},
			}

			applier := &ApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
					return c.ApplyResult, c.ApplyErr
				},
			}

			checker := NewChecker(slog.New(slog.DiscardHandler), db, runners, applier)

			probed := document.Block{Type: document.BlockNodeMetricBlock, Attrs: c.Attrs}

			calls := max(c.Calls, 1)

			var (
				res Result
				err error
			)

			for range calls {
				res, err = checker.Check(
					context.Background(),
					documentID,
					branchID,
					probed,
					_organizationID,
				)
			}

			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Cleared, res.Cleared)
			assert.Len(t, db.FetchDataSourceCalls(), c.FetchDataSourceCalls)
			assert.Len(t, db.FetchVariableTreeCalls(), c.FetchVariableTreeCalls)
			assert.Len(t, runners.RunnerCalls(), c.RunnerCalls)
			assert.Len(t, runner.TypeCalls(), c.TypeCalls)
			assert.Len(t, runner.PrometheusCalls(), c.PrometheusCalls)
			assert.Len(t, runner.PostgreSQLCalls(), c.PostgreSQLCalls)
			assert.Len(t, runner.MySQLCalls(), c.MySQLCalls)
			assert.Len(t, applier.ApplyCalls(), c.ApplyCalls)

			switch {
			case c.Prometheus != nil:
				assert.Len(t, c.Prometheus.QueryRangeCalls(), c.QueryCalls)
			case c.PostgreSQL != nil:
				assert.Len(t, c.PostgreSQL.QueryCalls(), c.QueryCalls)
			case c.MySQL != nil:
				assert.Len(t, c.MySQL.QueryCalls(), c.QueryCalls)
			}

			if c.AssertClearOp {
				call := applier.ApplyCalls()[0]

				assert.Equal(t, documentID, call.DocumentID)
				assert.Equal(t, branchID, call.BranchID)

				// the clear is core's own write: an ordinary one
				// would be refused on a protected branch.
				assert.True(t, call.System)
				require.Len(t, call.Ops, 1)

				assert.Equal(t, edit.Operation{
					Kind:     "update_attrs",
					BlockUID: "block-1",
					Attrs:    map[string]any{"simulationActive": false},
				}, call.Ops[0])
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

	known := markup.MetricEnums()[document.AttrTimeRange]

	assert.Len(t, _probeSpans, len(known))

	for _, preset := range known {
		span, ok := _probeSpans[preset]

		assert.True(t, ok, preset)
		assert.Positive(t, span, preset)
	}

	cc := map[string]struct {
		Preset string
		Span   time.Duration
	}{
		"An unknown preset is probed over the default span": {
			Preset: "",
			Span:   _defaultProbeSpan,
		},
		"A known preset is probed over its own span": {
			Preset: "last_5_minutes",
			Span:   5 * time.Minute,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Span, probeSpan(c.Preset))
		})
	}
}

func Test_Checker_Start(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	checker := NewChecker(
		slog.New(slog.DiscardHandler),
		&DBMock{},
		&RunnersMock{},
		&ApplierMock{},
	)

	done := make(chan struct{})

	go func() {
		defer close(done)

		checker.Start(ctx)
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
