package datasource

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/guregu/null/v5"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _testCacheFrom starts the window the cache tests query, between step
// boundaries so a few seconds later still falls in the same step.
var _testCacheFrom = time.Date(2024, 1, 1, 12, 0, 10, 0, time.UTC)

// recordingPrometheus answers range queries with res, recording the ranges
// it was asked for, and counts the label name reads it passes through.
type recordingPrometheus struct {
	Prometheus

	res        *processor.PrometheusQueryResult
	ranges     []processor.TimeRange
	labelReads int
}

func (rp *recordingPrometheus) QueryRange(_ context.Context, _ string, tr processor.TimeRange) (*processor.PrometheusQueryResult, error) {
	rp.ranges = append(rp.ranges, tr)

	return rp.res, nil
}

func (rp *recordingPrometheus) LabelNames(context.Context, []string, processor.TimeRange) (*processor.PrometheusLabelNamesResult, error) {
	rp.labelReads++

	return &processor.PrometheusLabelNamesResult{}, nil
}

// recordingPostgreSQL answers queries with res, recording the ranges it was
// asked for.
type recordingPostgreSQL struct {
	PostgreSQL

	res    *processor.PostgreSQLQueryResult
	ranges []processor.TimeRange
}

func (rp *recordingPostgreSQL) Query(_ context.Context, _ string, tr processor.TimeRange) (*processor.PostgreSQLQueryResult, error) {
	rp.ranges = append(rp.ranges, tr)

	return rp.res, nil
}

// recordingMySQL answers queries with res, recording the ranges it was
// asked for.
type recordingMySQL struct {
	MySQL

	res    *processor.MySQLQueryResult
	ranges []processor.TimeRange
}

func (rm *recordingMySQL) Query(_ context.Context, _ string, tr processor.TimeRange) (*processor.MySQLQueryResult, error) {
	rm.ranges = append(rm.ranges, tr)

	return rm.res, nil
}

// recordingLogs answers every log read with fixed results, recording the
// ranges it was asked for.
type recordingLogs struct {
	status processor.ConnectionStatus
	lines  *processor.LogsResult
	volume *processor.QueryResult
	fields *processor.LogFieldsResult
	values *processor.LogFieldValuesResult
	ranges []processor.TimeRange
}

func (rl *recordingLogs) TestConnection(context.Context) (processor.ConnectionStatus, error) {
	return rl.status, nil
}

func (rl *recordingLogs) Logs(_ context.Context, _ string, tr processor.TimeRange, _ processor.LogsPage) (*processor.LogsResult, error) {
	rl.ranges = append(rl.ranges, tr)

	return rl.lines, nil
}

func (rl *recordingLogs) LogsVolume(_ context.Context, _ string, tr processor.TimeRange) (*processor.QueryResult, error) {
	rl.ranges = append(rl.ranges, tr)

	return rl.volume, nil
}

func (rl *recordingLogs) LogFields(_ context.Context, _ string, tr processor.TimeRange) (*processor.LogFieldsResult, error) {
	rl.ranges = append(rl.ranges, tr)

	return rl.fields, nil
}

func (rl *recordingLogs) LogFieldValues(_ context.Context, _, _ string, tr processor.TimeRange) (*processor.LogFieldValuesResult, error) {
	rl.ranges = append(rl.ranges, tr)

	return rl.values, nil
}

// stubCache returns a cache keeping answers in a map that starts as kept.
// Reads fail with getErr and writes with setErr when they are set.
func stubCache(kept map[string][]byte, getErr, setErr error) *queryCacheMock {
	var mu sync.Mutex

	if kept == nil {
		kept = make(map[string][]byte)
	}

	return &queryCacheMock{
		GetFunc: func(_ context.Context, key string) ([]byte, error) {
			if getErr != nil {
				return nil, getErr
			}

			mu.Lock()
			defer mu.Unlock()

			data, ok := kept[key]
			if !ok {
				return nil, errutil.ErrNotFound
			}

			return data, nil
		},
		SetFunc: func(_ context.Context, key string, value []byte) error {
			if setErr != nil {
				return setErr
			}

			mu.Lock()
			defer mu.Unlock()

			kept[key] = value

			return nil
		},
	}
}

// prepCacheManager builds a manager answering queries out of cache, its
// lookups counted in the registry it returns.
func prepCacheManager(t *testing.T, cache *queryCacheMock) (*Manager, *prometheus.Registry) {
	t.Helper()

	rg := prometheus.NewRegistry()

	m := prepManager(t, nil)
	m.cache = cache
	m.metrics = newMetrics(metricutil.NewFactory("test", rg))

	return m, rg
}

// lookups reads how many cache lookups were answered as result, or -1
// when none was.
func lookups(t *testing.T, rg *prometheus.Registry, result string) float64 {
	t.Helper()

	return testutil.MetricValue(t, rg, "test_datasource_cache_lookups_total", map[string]string{_resultLabel: result})
}

func Test_cachedPrometheus_QueryRange(t *testing.T) {
	t.Parallel()

	inner := &recordingPrometheus{
		res: &processor.PrometheusQueryResult{
			Type: model.ValMatrix,
			Result: model.Matrix{
				{
					Metric: model.Metric{"job": "api"},
					Values: []model.SamplePair{{Timestamp: 1704110400000, Value: 1}},
				},
			},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cp := cachedPrometheus{Prometheus: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cp.QueryRange(context.Background(), "up", tr)
	require.NoError(t, err)

	// a viewer a few seconds later in the same step reads the same answer,
	// which comes back as the matrix the assistant's caps rely on.
	later := processor.TimeRange{From: tr.From.Add(5 * time.Second), To: tr.To.Add(5 * time.Second)}

	kept, err := cp.QueryRange(context.Background(), "up", later)
	require.NoError(t, err)
	assert.Equal(t, fresh, kept)
	assert.IsType(t, model.Matrix{}, kept.Result)

	_, err = cp.QueryRange(context.Background(), "rate(up[5m])", tr)
	require.NoError(t, err)

	// the client aligns the range itself, so it is given the one asked.
	assert.Equal(t, []processor.TimeRange{tr, tr}, inner.ranges)

	// what the cache does not cover reaches the data source every time.
	for range 2 {
		_, err = cp.LabelNames(context.Background(), nil, tr)
		require.NoError(t, err)
	}

	assert.Equal(t, 2, inner.labelReads)
}

func Test_cachedPostgreSQL_Query(t *testing.T) {
	t.Parallel()

	inner := &recordingPostgreSQL{
		res: &processor.PostgreSQLQueryResult{
			Columns: []string{"time", "host", "value"},
			Rows: [][]any{
				{"2024-01-01T12:00:00Z", "web-1", 10.5},
				{"2024-01-01T12:01:00Z", "web-2", 20.5},
			},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cp := cachedPostgreSQL{PostgreSQL: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cp.Query(context.Background(), "SELECT 1", tr)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cp.Query(context.Background(), "SELECT 1", later)
	require.NoError(t, err)

	// the kept answer draws the same chart as the fresh one.
	assert.Equal(t, fresh.Transform(processor.ChartTypeLine), kept.Transform(processor.ChartTypeLine))

	// another query is another answer.
	_, err = cp.Query(context.Background(), "SELECT 2", tr)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedMySQL_Query(t *testing.T) {
	t.Parallel()

	inner := &recordingMySQL{
		res: &processor.MySQLQueryResult{
			Columns:     []string{"time", "code", "day", "total", "ratio"},
			ColumnTypes: []string{"BIGINT", "VARCHAR", "DATE", "DECIMAL", "FLOAT"},
			Rows:        [][]any{{int64(1704110400), "200", "2024-01-01T00:00:00Z", "10.50", 1.5}},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cm := cachedMySQL{MySQL: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cm.Query(context.Background(), "SELECT 1", tr)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cm.Query(context.Background(), "SELECT 1", later)
	require.NoError(t, err)

	// the column types travel with the answer, so the VARCHAR of numbers
	// stays a label rather than turning into a series, and the date label
	// reads the same.
	assert.Equal(t, inner.res.ColumnTypes, kept.ColumnTypes)
	assert.Equal(t, fresh.Transform(processor.ChartTypeLine), kept.Transform(processor.ChartTypeLine))

	// another query is another answer.
	_, err = cm.Query(context.Background(), "SELECT 2", tr)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedLogs_TestConnection(t *testing.T) {
	t.Parallel()

	cl := cachedLogs{client: &recordingLogs{status: processor.ConnectionStatusUnreachable}}

	cs, err := cl.TestConnection(context.Background())
	require.NoError(t, err)
	assert.Equal(t, processor.ConnectionStatusUnreachable, cs)
}

func Test_cachedLogs_Logs(t *testing.T) {
	t.Parallel()

	cursor := &processor.Cursor{}
	require.NoError(t, cursor.UnmarshalText([]byte("ns:1704110400000000000")))

	inner := &recordingLogs{
		lines: &processor.LogsResult{
			Status: processor.QueryStatusOK,
			Lines: []processor.LogLine{
				{
					ID:     "a",
					Time:   time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC),
					Level:  processor.LogLevelInfo,
					Line:   "started",
					Labels: map[string]string{"app": "api"},
					Fields: map[string]string{},
				},
			},
			Cursor: null.ValueFrom(*cursor),
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cl := cachedLogs{client: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	// error
	_, err := cl.Logs(context.Background(), `{app="api"}`, tr, processor.LogsPage{Limit: 10, Cursor: null.ValueFrom(processor.Cursor{})})
	assert.Error(t, err)
	assert.Empty(t, inner.ranges)

	// success
	page := processor.LogsPage{Limit: 10, Cursor: null.ValueFrom(*cursor)}

	fresh, err := cl.Logs(context.Background(), `{app="api"}`, tr, page)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cl.Logs(context.Background(), `{app="api"}`, later, page)
	require.NoError(t, err)
	assert.Equal(t, fresh, kept)

	// a page of another size is another answer, and so is another query.
	_, err = cl.Logs(context.Background(), `{app="api"}`, tr, processor.LogsPage{Limit: 20, Cursor: page.Cursor})
	require.NoError(t, err)

	_, err = cl.Logs(context.Background(), `{app="web"}`, tr, page)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedLogs_LogsVolume(t *testing.T) {
	t.Parallel()

	inner := &recordingLogs{
		volume: &processor.QueryResult{
			Status: processor.QueryStatusOK,
			Data: []processor.QueryResultSeries{
				{Labels: map[string]string{"level": "info"}, Metrics: [][2]any{{int64(1704110460), float64(3)}}},
			},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cl := cachedLogs{client: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cl.LogsVolume(context.Background(), `{app="api"}`, tr)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cl.LogsVolume(context.Background(), `{app="api"}`, later)
	require.NoError(t, err)

	// numbers come back from JSON as float64, so the answer is compared
	// as the response it is sent as.
	freshJSON, err := json.Marshal(fresh)
	require.NoError(t, err)

	keptJSON, err := json.Marshal(kept)
	require.NoError(t, err)

	assert.JSONEq(t, string(freshJSON), string(keptJSON))

	// another query is another answer.
	_, err = cl.LogsVolume(context.Background(), `{app="web"}`, tr)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedLogs_LogFields(t *testing.T) {
	t.Parallel()

	inner := &recordingLogs{
		fields: &processor.LogFieldsResult{
			Fields: []processor.LogField{{Name: "app", Kind: processor.LogFieldKindLabel}},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cl := cachedLogs{client: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cl.LogFields(context.Background(), "", tr)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cl.LogFields(context.Background(), "", later)
	require.NoError(t, err)
	assert.Equal(t, fresh, kept)

	// another query is another answer.
	_, err = cl.LogFields(context.Background(), `{app="web"}`, tr)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedLogs_LogFieldValues(t *testing.T) {
	t.Parallel()

	inner := &recordingLogs{
		values: &processor.LogFieldValuesResult{
			Values: []processor.LogFieldValue{{Value: "api", Count: null.IntFrom(3)}},
		},
	}

	m, _ := prepCacheManager(t, stubCache(nil, nil, nil))
	cl := cachedLogs{client: inner, m: m, ds: DataSource{ID: xid.New()}}

	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}

	fresh, err := cl.LogFieldValues(context.Background(), "app", "", tr)
	require.NoError(t, err)

	// a viewer a few seconds later on the same grid reads the same answer.
	later := processor.TimeRange{From: tr.From.Add(3 * time.Second), To: tr.To.Add(3 * time.Second)}

	kept, err := cl.LogFieldValues(context.Background(), "app", "", later)
	require.NoError(t, err)
	assert.Equal(t, fresh, kept)

	// another field is another answer, and so is another query.
	_, err = cl.LogFieldValues(context.Background(), "level", "", tr)
	require.NoError(t, err)

	_, err = cl.LogFieldValues(context.Background(), "app", `{app="web"}`, tr)
	require.NoError(t, err)

	assert.Equal(t, []processor.TimeRange{tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid), tr.AlignTo(_cacheGrid)}, inner.ranges)
}

func Test_cachedQuery(t *testing.T) {
	t.Parallel()

	rows := &processor.PostgreSQLQueryResult{
		Columns: []string{"time", "value"},
		Rows:    [][]any{{1704110400.0, 10.0}},
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cc := map[string]struct {
		// Kept is what the cache holds before the calls.
		Kept   map[string][]byte
		GetErr error
		SetErr error

		// Context is the callers' context, and Callers how many of them
		// ask at once.
		Context context.Context
		Callers int

		Answer  *processor.PostgreSQLQueryResult
		CallErr error

		Result *processor.PostgreSQLQueryResult
		Err    error
		Calls  int

		// Sets counts the answers written to the cache, a failed write
		// included.
		Sets int

		// Hits, Misses and Shared count the lookups answered each way,
		// -1 standing for none.
		Hits   float64
		Misses float64
		Shared float64
		Log    string
	}{
		"Answer kept a moment ago": {
			Kept:    map[string][]byte{"key": []byte(`{"columns":["time","value"],"rows":[[1704110400,10]]}`)},
			Callers: 1,
			Result:  rows,
			Hits:    1,
			Misses:  -1,
			Shared:  -1,
		},
		"First call keeps its answer": {
			Callers: 1,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
		},
		"Error is not kept": {
			Callers: 1,
			CallErr: assert.AnError,
			Err:     assert.AnError,
			Calls:   1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
		},
		// a metric that starts reporting shows at once rather than after
		// the empty answer expires.
		"Answer without data is not kept": {
			Callers: 1,
			Answer:  &processor.PostgreSQLQueryResult{Columns: []string{"value"}},
			Result:  &processor.PostgreSQLQueryResult{Columns: []string{"value"}},
			Calls:   1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
		},
		"Cache that cannot be read": {
			GetErr:  assert.AnError,
			Callers: 1,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
			Log:     "cannot read the query cache",
		},
		"Kept answer that no longer decodes": {
			Kept:    map[string][]byte{"key": []byte(`{`)},
			Callers: 1,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
			Log:     "cannot decode a cached query answer",
		},
		"Cache that cannot be written": {
			SetErr:  assert.AnError,
			Callers: 1,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
			Log:     "cannot write the query cache",
		},
		"Answer that cannot be encoded": {
			Callers: 1,
			Answer:  &processor.PostgreSQLQueryResult{Columns: []string{"value"}, Rows: [][]any{{math.Inf(1)}}},
			Result:  &processor.PostgreSQLQueryResult{Columns: []string{"value"}, Rows: [][]any{{math.Inf(1)}}},
			Calls:   1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
			Log:     "cannot encode a query answer for the cache",
		},
		// the calls waiting on the query would get the cancellation too.
		"A caller that gave up does not cancel the query": {
			Context: cancelled,
			Callers: 1,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  -1,
		},
		"Calls at once share one query": {
			Callers: 5,
			Answer:  rows,
			Result:  rows,
			Calls:   1,
			Sets:    1,
			Hits:    -1,
			Misses:  1,
			Shared:  4,
		},
		"Calls at once share one failing query": {
			Callers: 5,
			CallErr: assert.AnError,
			Err:     assert.AnError,
			Calls:   1,
			Hits:    -1,
			Misses:  1,
			Shared:  4,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the bubble lets the test wait until every caller is either
			// inside the query or waiting on it before letting it answer.
			synctest.Test(t, func(t *testing.T) {
				ctx := c.Context
				if ctx == nil {
					ctx = context.Background()
				}

				cache := stubCache(c.Kept, c.GetErr, c.SetErr)
				m, rg := prepCacheManager(t, cache)

				w, buf := testutil.NewBuffer()
				m.log = slog.New(slog.NewTextHandler(w, nil))

				var (
					mu    sync.Mutex
					calls int
				)

				release := make(chan struct{})

				call := func(ctx context.Context) (*processor.PostgreSQLQueryResult, error) {
					mu.Lock()
					calls++
					mu.Unlock()

					<-release

					if err := ctx.Err(); err != nil {
						return nil, err
					}

					return c.Answer, c.CallErr
				}

				results := make([]*processor.PostgreSQLQueryResult, c.Callers)
				errs := make([]error, c.Callers)

				supv := xync.NewSupervisor()

				for i := range c.Callers {
					supv.Go(func(context.Context) {
						results[i], errs[i] = cachedQuery(ctx, m, "key", call)
					})
				}

				synctest.Wait()
				close(release)
				supv.Wait()

				require.NoError(t, w.Flush())

				for i := range c.Callers {
					testutil.AssertEqualError(t, c.Err, errs[i])
					assert.Equal(t, c.Result, results[i])
				}

				assert.Equal(t, c.Calls, calls)
				assert.Len(t, cache.SetCalls(), c.Sets)
				assert.InDelta(t, c.Hits, lookups(t, rg, _lookupHit), 0.0001)
				assert.InDelta(t, c.Misses, lookups(t, rg, _lookupMiss), 0.0001)
				assert.InDelta(t, c.Shared, lookups(t, rg, _lookupShared), 0.0001)

				if c.Log == "" {
					assert.Empty(t, buf.String())
					return
				}

				assert.Contains(t, buf.String(), c.Log)
			})
		})
	}
}

func Test_cacheKey(t *testing.T) {
	t.Parallel()

	ds := DataSource{ID: xid.New()}
	tr := processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(time.Hour)}.Normalize()
	key := cacheKey(ds, "query_range", tr, "up")

	edited := ds
	edited.UpdatedAt = null.TimeFrom(_testCacheFrom)

	cc := map[string]struct {
		DS     DataSource
		Method string
		TR     processor.TimeRange
		Args   []string
		Same   bool
	}{
		"Same query": {
			DS:     ds,
			Method: "query_range",
			TR:     tr,
			Args:   []string{"up"},
			Same:   true,
		},
		// two viewers' "last hour" a few seconds apart align to one step.
		"Range a few seconds later in the same step": {
			DS:     ds,
			Method: "query_range",
			TR:     processor.TimeRange{From: _testCacheFrom.Add(5 * time.Second), To: _testCacheFrom.Add(time.Hour + 5*time.Second)}.Normalize(),
			Args:   []string{"up"},
			Same:   true,
		},
		// the same query on two organizations' data sources never shares.
		"Another data source": {
			DS:     DataSource{ID: xid.New()},
			Method: "query_range",
			TR:     tr,
			Args:   []string{"up"},
		},
		"Edited data source": {
			DS:     edited,
			Method: "query_range",
			TR:     tr,
			Args:   []string{"up"},
		},
		"Another method": {
			DS:     ds,
			Method: "logs_volume",
			TR:     tr,
			Args:   []string{"up"},
		},
		"Another query": {
			DS:     ds,
			Method: "query_range",
			TR:     tr,
			Args:   []string{"down"},
		},
		"Another range": {
			DS:     ds,
			Method: "query_range",
			TR:     processor.TimeRange{From: _testCacheFrom, To: _testCacheFrom.Add(2 * time.Hour)}.Normalize(),
			Args:   []string{"up"},
		},
		"Another argument": {
			DS:     ds,
			Method: "query_range",
			TR:     tr,
			Args:   []string{"up", "10"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := cacheKey(c.DS, c.Method, c.TR, c.Args...)

			assert.Regexp(t, "^"+_cacheKeyPrefix+"[0-9a-f]{64}$", got)
			assert.Equal(t, c.Same, got == key)
		})
	}
}
