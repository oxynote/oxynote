package demo

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _availableMetricsPath is the welcome document's pool of metric blocks,
// whose queries this package has to be able to answer.
const _availableMetricsPath = "../../document/files/available_metrics.json"

// resultMatrix returns the matrix a query result carries, failing the
// test if it carries anything else.
func resultMatrix(t *testing.T, res *processor.PrometheusQueryResult) model.Matrix {
	t.Helper()

	require.NotNil(t, res)
	require.Equal(t, model.ValMatrix, res.Type)

	m, ok := res.Result.(model.Matrix)
	require.True(t, ok, "result is %T, not a matrix", res.Result)

	return m
}

func Test_newEngine(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, newEngine())

	// a subquery with no step of its own is evaluated at the tick, so a
	// ten minute window holds ten points.
	q, err := newEngine().NewInstantQuery(
		context.Background(),
		queryable{registry: newRegistry()},
		nil,
		"count_over_time("+_namespace+"deploy_confidence_index[10m:])",
		timeAt(1_000),
	)
	require.NoError(t, err)

	defer q.Close()

	res := q.Exec(context.Background())
	require.NoError(t, res.Err)

	v, ok := res.Value.(promql.Vector)
	require.True(t, ok, "result is %T, not a vector", res.Value)
	require.Len(t, v, 3)

	for _, s := range v {
		assert.Equal(t, 10.0, s.F)
	}
}

func Test_NewClient(t *testing.T) {
	t.Parallel()

	c := NewClient()
	require.NotNil(t, c)

	// the client owns what a query needs, so one client answers every
	// query a process makes and the registry's replay cache survives
	// between them.
	assert.NotNil(t, c.engine)
	assert.NotNil(t, c.parser)
	assert.Len(t, c.registry, 15)

	// the client holds one registry for its lifetime: the walks behind it
	// cache the history they replay, and rebuilding it per query would
	// walk every tick since the epoch again.
	first := c.registry
	require.NotEmpty(t, first)

	assert.Equal(t, first[0].buckets[0].value(7), c.registry[0].buckets[0].value(7))

	// a separate client is a separate cache, reading the same history.
	other := NewClient()
	require.Len(t, other.registry, len(first))

	for i := range first {
		assert.Equal(t, first[i].name, other.registry[i].name)
		assert.Equal(t, first[i].buckets[0].value(7), other.registry[i].buckets[0].value(7))
	}
}

func Test_Client_TestConnection(t *testing.T) {
	t.Parallel()

	// the demo is this process, so the connection cannot be down.
	cs, err := _client.TestConnection(context.Background())

	require.NoError(t, err)
	assert.Equal(t, processor.ConnectionStatusSuccess, cs)
}

func Test_Client_Metadata(t *testing.T) {
	t.Parallel()

	c := _client

	res, err := c.Metadata(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)

	md, ok := res.Result.(map[string][]v1.Metadata)
	require.True(t, ok)

	// every family the registry declares is described, as a gauge, with
	// the help text the editor shows.
	assert.Len(t, md, 15)

	for _, f := range c.registry {
		entry, ok := md[f.name]
		require.True(t, ok, "%s is missing from the metadata", f.name)
		require.Len(t, entry, 1)

		assert.Equal(t, v1.MetricTypeGauge, entry[0].Type)
		assert.Equal(t, f.help, entry[0].Help)
	}
}

func Test_Client_QueryRange(t *testing.T) {
	t.Parallel()

	c := _client
	name := _namespace + "deploy_confidence_index"
	stale := _namespace + "stale_branches_total"
	tr := processor.TimeRange{
		From: timeutil.Now().Add(-3 * time.Hour),
		To:   timeutil.Now(),
	}

	cc := map[string]struct {
		Query   string
		Range   processor.TimeRange
		Context context.Context //nolint:containedctx // the case states which context the query runs under
		Series  int
		Err     error
		Check   func(t *testing.T, res *processor.PrometheusQueryResult)
	}{
		"An abandoned evaluation is not blamed on the query": {
			Query: name,
			Range: tr,
			Context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			}(),
			Err: assert.AnError,
		},
		"A plain selector returns every series of the family": {
			Query:  name,
			Range:  tr,
			Series: 3,
		},
		"A selector narrowed by a label returns the one series": {
			Query:  name + `{vibe="nervous"}`,
			Range:  tr,
			Series: 1,
		},
		"An aggregation collapses the family": {
			Query:  "sum(" + name + ")",
			Range:  tr,
			Series: 1,
		},
		"A function over a window reads the history behind each point": {
			Query:  "avg_over_time(" + name + "[30m])",
			Range:  tr,
			Series: 3,
		},
		"A macro is expanded before the query runs": {
			Query:  "max_over_time(" + name + "[$__interval])",
			Range:  tr,
			Series: 3,
		},
		"A year of history backfills for free": {
			Query:  name,
			Range:  processor.TimeRange{From: timeutil.Now().AddDate(-1, 0, 0), To: timeutil.Now()},
			Series: 3,
		},
		"A selector naming nothing returns no series": {
			Query: "not_a_metric",
			Range: tr,
		},
		"A query that does not parse is reported as invalid": {
			Query: "sum(",
			Range: tr,
			Err:   assert.AnError,
		},
		"A query of the wrong type is reported too": {
			Query: `"a string"`,
			Range: tr,
			Err:   assert.AnError,
		},
		"The same window answers the same history every time": {
			Query:  stale,
			Range:  tr,
			Series: 3,
			Check: func(t *testing.T, first *processor.PrometheusQueryResult) {
				second, err := c.QueryRange(context.Background(), stale, tr)
				require.NoError(t, err)

				// the same window answers bit-identically, which is what
				// keeps a drawn line from rewriting itself under the reader.
				firstJSON, err := json.Marshal(first.Result)
				require.NoError(t, err)

				secondJSON, err := json.Marshal(second.Result)
				require.NoError(t, err)

				assert.JSONEq(t, string(firstJSON), string(secondJSON))

				// and a window inside it answers the same values for the
				// timestamps the two share: history does not depend on how
				// it was asked for.
				inner, err := c.QueryRange(context.Background(), stale, processor.TimeRange{
					From: tr.From.Add(time.Hour),
					To:   tr.To,
				})
				require.NoError(t, err)

				outer := resultMatrix(t, first)
				require.Len(t, resultMatrix(t, inner), len(outer))
			},
		},
	}

	for cn, c2 := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx := c2.Context
			if ctx == nil {
				ctx = context.Background()
			}

			res, err := c.QueryRange(ctx, c2.Query, c2.Range)
			testutil.AssertEqualError(t, c2.Err, err)

			if c2.Err != nil {
				assert.Nil(t, res)
				return
			}

			m := resultMatrix(t, res)
			require.Len(t, m, c2.Series)

			ntr := c2.Range.Normalize()

			for _, s := range m {
				require.NotEmpty(t, s.Values)

				// the line spans the window asked for, with no gap at
				// either edge, and every point carries a real number.
				first := s.Values[0].Timestamp.Time()
				last := s.Values[len(s.Values)-1].Timestamp.Time()

				assert.WithinDuration(t, ntr.From, first, 2*ntr.QueryStep())
				assert.WithinDuration(t, ntr.To, last, 2*ntr.QueryStep())

				for _, v := range s.Values {
					assert.False(t, math.IsNaN(float64(v.Value)))
					assert.False(t, math.IsInf(float64(v.Value), 0))
				}

				// the point count follows the step the range asks for,
				// not the range's width in ticks: a year spans 525,600
				// of them and still draws a few hundred points.
				assert.Less(t, len(s.Values), 1_000)
			}

			if c2.Check != nil {
				c2.Check(t, res)
			}
		})
	}
}

func Test_Client_LabelNames(t *testing.T) {
	t.Parallel()

	tr := processor.TimeRange{
		From: timeutil.Now().Add(-3 * time.Hour),
		To:   timeutil.Now(),
	}

	cc := map[string]struct {
		Matchers []string
		Result   *processor.PrometheusLabelNamesResult
		Err      error
	}{
		"No selector names every label": {
			Result: &processor.PrometheusLabelNamesResult{Result: []string{
				model.MetricNameLabel,
				"confidence",
				"duration",
				"excuse",
				"original_comment",
				"outcome",
				"owner_status",
				"pizza_count",
				"platform",
				"removal_likelihood",
				"reviewer_state",
				"root_cause",
				"severity",
				"vibe",
			}},
		},
		"A selector narrows the answer": {
			Matchers: []string{_namespace + "deploy_confidence_index"},
			Result:   &processor.PrometheusLabelNamesResult{Result: []string{model.MetricNameLabel, "vibe"}},
		},
		"A selector that does not parse": {
			Matchers: []string{"{{"},
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := _client.LabelNames(context.Background(), c.Matchers, tr)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Client_LabelValues(t *testing.T) {
	t.Parallel()

	tr := processor.TimeRange{
		From: timeutil.Now().Add(-3 * time.Hour),
		To:   timeutil.Now(),
	}

	cc := map[string]struct {
		Label    string
		Matchers []string
		Result   *processor.PrometheusLabelValuesResult
		Err      error
	}{
		"The metric names are what the editor completes on": {
			Label: model.MetricNameLabel,
			Result: &processor.PrometheusLabelValuesResult{Result: []string{
				_namespace + "circle_back_count",
				_namespace + "deploy_confidence_index",
				_namespace + "feature_flag_graveyard_size",
				_namespace + "friday_deploys_count",
				_namespace + "hotfixes_per_release",
				_namespace + "meetings_could_be_doc_count",
				_namespace + "pizza_fridays_next_30_days",
				_namespace + "pr_time_to_first_review_minutes",
				_namespace + "prs_merged_friday_afternoon_count",
				_namespace + "quick_sync_meetings_same_day_count",
				_namespace + "secrets_detected_commits_count",
				_namespace + "ship_it_approvals_after_5pm_count",
				_namespace + "stale_branches_total",
				_namespace + "temporary_fix_age_days",
				_namespace + "works_on_my_machine_rate",
			}},
		},
		"Every value of a label": {
			Label:  "vibe",
			Result: &processor.PrometheusLabelValuesResult{Result: []string{"feeling_lucky", "nervous", "oncall_already_paged"}},
		},
		"A selector whose series lack the label": {
			Label:    "vibe",
			Matchers: []string{_namespace + "secrets_detected_commits_count"},
			Result:   &processor.PrometheusLabelValuesResult{Result: []string{}},
		},
		"A selector that does not parse": {
			Label:    "vibe",
			Matchers: []string{"{{"},
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := _client.LabelValues(context.Background(), c.Label, c.Matchers, tr)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Client_Series(t *testing.T) {
	t.Parallel()

	name := _namespace + "deploy_confidence_index"
	tr := processor.TimeRange{
		From: timeutil.Now().Add(-3 * time.Hour),
		To:   timeutil.Now(),
	}

	var buckets int

	for _, f := range _client.registry {
		buckets += len(f.buckets)
	}

	cc := map[string]struct {
		Matchers []string
		Err      error
		Check    func(t *testing.T, res []model.LabelSet)
	}{
		"A selector names the series of its family": {
			Matchers: []string{name},
			Check: func(t *testing.T, res []model.LabelSet) {
				require.Len(t, res, 3)

				for _, ls := range res {
					assert.Equal(t, model.LabelValue(name), ls[model.MetricNameLabel])
					assert.NotEmpty(t, ls["vibe"])
				}
			},
		},
		"Selectors that overlap answer each series once": {
			Matchers: []string{name, name + `{vibe="nervous"}`},
			Check: func(t *testing.T, res []model.LabelSet) {
				assert.Len(t, res, 3)
			},
		},
		"No selector at all names every series there is": {
			Check: func(t *testing.T, res []model.LabelSet) {
				assert.Len(t, res, buckets)
			},
		},
		"A selector that does not parse": {
			Matchers: []string{"{{"},
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := _client.Series(context.Background(), c.Matchers, tr)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			require.NotNil(t, res)
			c.Check(t, res.Result)
		})
	}
}

func Test_Client_parseMatchers(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Matchers []string
		Result   [][]*labels.Matcher
		Err      error
	}{
		"No selector means one empty set which selects everything": {
			Result: [][]*labels.Matcher{nil},
		},
		"Each selector is a set of its own": {
			Matchers: []string{`metric{a="b"}`, "other"},
			Result: [][]*labels.Matcher{
				{
					labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
					labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "metric"),
				},
				{
					labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "other"),
				},
			},
		},
		"A selector that does not parse": {
			Matchers: []string{"{{"},
			Err:      assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			sets, err := _client.parseMatchers(c.Matchers)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, sets)
		})
	}
}

func Test_toModelMatrix(t *testing.T) {
	t.Parallel()

	m := toModelMatrix(promql.Matrix{
		{
			Metric: labels.FromStrings(model.MetricNameLabel, "metric", "colour", "red"),
			Floats: []promql.FPoint{{T: 1_700_000_000_000, F: 1.5}},
		},
	})

	require.Len(t, m, 1)
	assert.Equal(t, model.LabelValue("metric"), m[0].Metric[model.MetricNameLabel])
	assert.Equal(t, model.LabelValue("red"), m[0].Metric["colour"])

	require.Len(t, m[0].Values, 1)
	assert.Equal(t, model.SampleValue(1.5), m[0].Values[0].Value)
	assert.Equal(t, int64(1_700_000_000), m[0].Values[0].Timestamp.Unix())

	assert.Empty(t, toModelMatrix(nil))
}

func Test_toModelLabelSet(t *testing.T) {
	t.Parallel()

	ls := toModelLabelSet(labels.FromStrings(model.MetricNameLabel, "metric", "colour", "red"))

	assert.Equal(t, model.LabelSet{
		model.MetricNameLabel: "metric",
		"colour":              "red",
	}, ls)

	assert.Empty(t, toModelLabelSet(labels.EmptyLabels()))
}

// Test_welcomeDocumentQueries guards the pair that has no compiler
// between them: the welcome document's metric blocks name metrics by
// string, and this package is what has to answer them. A family renamed
// on one side and not the other empties the document's charts, which
// nothing else here would notice.
func Test_welcomeDocumentQueries(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile(_availableMetricsPath)
	require.NoError(t, err)

	var blocks []struct {
		Attrs struct {
			Queries []struct {
				Query string `json:"query"`
			} `json:"queries"`
		} `json:"attrs"`
	}

	require.NoError(t, json.Unmarshal(raw, &blocks))
	require.NotEmpty(t, blocks)

	c := _client
	tr := processor.TimeRange{
		From: timeutil.Now().Add(-3 * time.Hour),
		To:   timeutil.Now(),
	}

	var queries int

	for _, b := range blocks {
		for _, q := range b.Attrs.Queries {
			queries++

			t.Run(q.Query, func(t *testing.T) {
				t.Parallel()

				res, err := c.QueryRange(context.Background(), q.Query, tr)
				require.NoError(t, err)

				m := resultMatrix(t, res)
				assert.NotEmpty(t, m, "%s returned no series", q.Query)

				for _, s := range m {
					assert.NotEmpty(t, s.Values, "%s returned an empty series", q.Query)
				}
			})
		}
	}

	// every family the registry declares earns its place in the pool the
	// document draws from, and vice versa.
	assert.Len(t, c.registry, queries)
}
