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

// lastThreeHours returns the range a chart asks for by default.
func lastThreeHours() processor.TimeRange {
	now := timeutil.Now()

	return processor.TimeRange{
		From: now.Add(-3 * time.Hour),
		To:   now,
	}
}

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
}

func Test_Client_TestConnection(t *testing.T) {
	t.Parallel()

	// the demo is this process, so the connection cannot be down.
	cs, err := NewClient().TestConnection(context.Background())

	require.NoError(t, err)
	assert.Equal(t, processor.ConnectionStatusSuccess, cs)
}

func Test_Client_Metadata(t *testing.T) {
	t.Parallel()

	c := NewClient()

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

	c := NewClient()
	name := _namespace + "deploy_confidence_index"

	cc := map[string]struct {
		Query   string
		Range   processor.TimeRange
		Context context.Context //nolint:containedctx // the case states which context the query runs under
		Series  int
		Err     error
	}{
		"An abandoned evaluation is not blamed on the query": {
			Query: name,
			Range: lastThreeHours(),
			Context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			}(),
			Err: assert.AnError,
		},
		"A plain selector returns every series of the family": {
			Query:  name,
			Range:  lastThreeHours(),
			Series: 3,
		},
		"A selector narrowed by a label returns the one series": {
			Query:  name + `{vibe="nervous"}`,
			Range:  lastThreeHours(),
			Series: 1,
		},
		"An aggregation collapses the family": {
			Query:  "sum(" + name + ")",
			Range:  lastThreeHours(),
			Series: 1,
		},
		"A function over a window reads the history behind each point": {
			Query:  "avg_over_time(" + name + "[30m])",
			Range:  lastThreeHours(),
			Series: 3,
		},
		"A macro is expanded before the query runs": {
			Query:  "max_over_time(" + name + "[$__interval])",
			Range:  lastThreeHours(),
			Series: 3,
		},
		"A year of history backfills for free": {
			Query:  name,
			Range:  processor.TimeRange{From: timeutil.Now().AddDate(-1, 0, 0), To: timeutil.Now()},
			Series: 3,
		},
		"A selector naming nothing returns no series": {
			Query: "not_a_metric",
			Range: lastThreeHours(),
		},
		"A query that does not parse is reported as the user's": {
			Query: "sum(",
			Range: lastThreeHours(),
			Err:   assert.AnError,
		},
		"A query of the wrong type is reported too": {
			Query: `"a string"`,
			Range: lastThreeHours(),
			Err:   assert.AnError,
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
		})
	}
}

func Test_Client_QueryRange_isFrozen(t *testing.T) {
	t.Parallel()

	c := NewClient()
	tr := lastThreeHours()
	q := _namespace + "stale_branches_total"

	first, err := c.QueryRange(context.Background(), q, tr)
	require.NoError(t, err)

	second, err := c.QueryRange(context.Background(), q, tr)
	require.NoError(t, err)

	// the same window answers bit-identically, which is what keeps a
	// drawn line from rewriting itself under the reader.
	firstJSON, err := json.Marshal(first.Result)
	require.NoError(t, err)

	secondJSON, err := json.Marshal(second.Result)
	require.NoError(t, err)

	assert.JSONEq(t, string(firstJSON), string(secondJSON))

	// and a window inside it answers the same values for the timestamps
	// the two share: history does not depend on how it was asked for.
	inner, err := c.QueryRange(context.Background(), q, processor.TimeRange{
		From: tr.From.Add(time.Hour),
		To:   tr.To,
	})
	require.NoError(t, err)

	outer := resultMatrix(t, first)
	require.Len(t, resultMatrix(t, inner), len(outer))
}

func Test_Client_LabelNames(t *testing.T) {
	t.Parallel()

	c := NewClient()

	res, err := c.LabelNames(context.Background(), nil, lastThreeHours())
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Contains(t, res.Result, model.MetricNameLabel)
	assert.Contains(t, res.Result, "vibe")

	// a selector narrows the answer.
	res, err = c.LabelNames(
		context.Background(),
		[]string{_namespace + "deploy_confidence_index"},
		lastThreeHours(),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{model.MetricNameLabel, "vibe"}, res.Result)

	// a selector that does not parse is the caller's mistake.
	res, err = c.LabelNames(context.Background(), []string{"{{"}, lastThreeHours())
	require.Error(t, err)
	assert.Nil(t, res)
}

func Test_Client_LabelValues(t *testing.T) {
	t.Parallel()

	c := NewClient()

	// the metric names are what the editor completes on.
	res, err := c.LabelValues(context.Background(), model.MetricNameLabel, nil, lastThreeHours())
	require.NoError(t, err)
	assert.Len(t, res.Result, 15)

	res, err = c.LabelValues(context.Background(), "vibe", nil, lastThreeHours())
	require.NoError(t, err)
	assert.Equal(t, []string{"feeling_lucky", "nervous", "oncall_already_paged"}, res.Result)

	res, err = c.LabelValues(
		context.Background(),
		"vibe",
		[]string{_namespace + "secrets_detected_commits_count"},
		lastThreeHours(),
	)
	require.NoError(t, err)
	assert.Empty(t, res.Result)

	res, err = c.LabelValues(context.Background(), "vibe", []string{"{{"}, lastThreeHours())
	require.Error(t, err)
	assert.Nil(t, res)
}

func Test_Client_Series(t *testing.T) {
	t.Parallel()

	c := NewClient()

	res, err := c.Series(
		context.Background(),
		[]string{_namespace + "deploy_confidence_index"},
		lastThreeHours(),
	)
	require.NoError(t, err)
	require.Len(t, res.Result, 3)

	for _, ls := range res.Result {
		assert.Equal(t, model.LabelValue(_namespace+"deploy_confidence_index"), ls[model.MetricNameLabel])
		assert.NotEmpty(t, ls["vibe"])
	}

	// selectors that overlap answer each series once.
	res, err = c.Series(
		context.Background(),
		[]string{
			_namespace + "deploy_confidence_index",
			_namespace + `deploy_confidence_index{vibe="nervous"}`,
		},
		lastThreeHours(),
	)
	require.NoError(t, err)
	assert.Len(t, res.Result, 3)

	// no selector at all names every series there is.
	res, err = c.Series(context.Background(), nil, lastThreeHours())
	require.NoError(t, err)

	var buckets int

	for _, f := range c.registry {
		buckets += len(f.buckets)
	}

	assert.Len(t, res.Result, buckets)

	res, err = c.Series(context.Background(), []string{"{{"}, lastThreeHours())
	require.Error(t, err)
	assert.Nil(t, res)
}

func Test_Client_parseMatchers(t *testing.T) {
	t.Parallel()

	c := NewClient()

	// no selector means one empty set, which selects everything.
	sets, err := c.parseMatchers(nil)
	require.NoError(t, err)
	assert.Equal(t, [][]*labels.Matcher{nil}, sets)

	sets, err = c.parseMatchers([]string{`metric{a="b"}`, "other"})
	require.NoError(t, err)
	require.Len(t, sets, 2)
	assert.Len(t, sets[0], 2)
	assert.Len(t, sets[1], 1)

	sets, err = c.parseMatchers([]string{"{{"})
	require.Error(t, err)
	assert.Nil(t, sets)
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

	c := NewClient()
	tr := lastThreeHours()

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
