package demo

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	// _logDay is a whole day two days back, well inside the retention.
	_logDay = processor.TimeRange{
		From: timeutil.Now().Truncate(24 * time.Hour).Add(-48 * time.Hour),
		To:   timeutil.Now().Truncate(24 * time.Hour).Add(-24 * time.Hour),
	}

	// _logExpired is two days that ended before the retention starts.
	_logExpired = processor.TimeRange{
		From: timeutil.Now().Add(-10 * 24 * time.Hour),
		To:   timeutil.Now().Add(-8 * 24 * time.Hour),
	}

	// _logStraddling starts before the retention and ends inside it.
	_logStraddling = processor.TimeRange{
		From: timeutil.Now().Add(-8 * 24 * time.Hour),
		To:   timeutil.Now().Add(-6 * 24 * time.Hour),
	}
)

// readAllLogs pages through the window a thousand lines at a time and
// returns every line read.
func readAllLogs(t *testing.T, q string, tr processor.TimeRange) []processor.LogLine {
	t.Helper()

	var lines []processor.LogLine

	page := processor.LogsPage{Limit: 1000}

	for {
		res, err := _client.Logs(context.Background(), q, tr, page)
		require.NoError(t, err)

		lines = append(lines, res.Lines...)

		if !res.Cursor.Valid {
			return lines
		}

		page.Cursor = res.Cursor
	}
}

func Test_Client_Logs(t *testing.T) {
	hour := processor.TimeRange{From: _logDay.From.Add(10 * time.Hour), To: _logDay.From.Add(11 * time.Hour)}

	cc := map[string]struct {
		Query string
		Check func(t *testing.T)
		Err   error
	}{
		"Query the demo does not answer": {
			Query: `{env="production"} | json`,
			Err:   _errUnsupportedLogQuery,
		},
		"Window past the retention holds no lines": {
			Check: func(t *testing.T) {
				res, err := _client.Logs(context.Background(), `{env="production"}`, _logExpired, processor.LogsPage{Limit: 10})
				require.NoError(t, err)
				assert.Equal(t, processor.QueryStatusNoData, res.Status)
			},
		},
		"Window reaching past the retention holds only kept lines": {
			Check: func(t *testing.T) {
				oldest := timeutil.Now().Add(-_logsRetention)

				res, err := _client.Logs(context.Background(), `{env="production"}`, _logStraddling, processor.LogsPage{Limit: 1000})
				require.NoError(t, err)
				require.NotEmpty(t, res.Lines)

				for _, l := range res.Lines {
					assert.False(t, l.Time.Before(oldest), "line at %s is past the retention", l.Time)
				}
			},
		},
		"Same window twice is byte-equal": {
			Check: func(t *testing.T) {
				page := processor.LogsPage{Limit: 100}

				a, err := _client.Logs(context.Background(), `{env="production"}`, hour, page)
				require.NoError(t, err)

				b, err := _client.Logs(context.Background(), `{env="production"}`, hour, page)
				require.NoError(t, err)

				rawA, err := json.Marshal(a)
				require.NoError(t, err)

				rawB, err := json.Marshal(b)
				require.NoError(t, err)

				assert.Equal(t, string(rawA), string(rawB))
				assert.Len(t, a.Lines, 100)
			},
		},
		"Overlapping windows agree on the lines they share": {
			Check: func(t *testing.T) {
				shared := processor.TimeRange{From: hour.From.Add(20 * time.Minute), To: hour.To}
				later := processor.TimeRange{From: shared.From, To: hour.To.Add(30 * time.Minute)}

				inShared := func(lines []processor.LogLine) []processor.LogLine {
					var out []processor.LogLine

					for _, l := range lines {
						if !l.Time.Before(shared.From) && l.Time.Before(shared.To) {
							out = append(out, l)
						}
					}

					return out
				}

				a := inShared(readAllLogs(t, `{env="production"}`, hour))
				b := inShared(readAllLogs(t, `{env="production"}`, later))

				require.NotEmpty(t, a)
				assert.Equal(t, a, b)
			},
		},
		"A day read page by page holds every line once": {
			Check: func(t *testing.T) {
				var total int

				for tick := tickAt(_logDay.From); tick < tickAt(_logDay.To); tick++ {
					for _, s := range _client.logSources {
						total += len(s.linesAt(tick))
					}
				}

				ids := map[string]bool{}

				for _, l := range readAllLogs(t, `{env="production"}`, _logDay) {
					assert.False(t, ids[l.ID], "line %s read twice", l.ID)

					ids[l.ID] = true
				}

				assert.Len(t, ids, total)
			},
		},
		"A page stops at the newest lines it needs": {
			Check: func(t *testing.T) {
				from := timeutil.Now().Add(-3 * 24 * time.Hour)
				window := processor.TimeRange{From: from, To: from.Add(time.Hour)}

				res, err := _client.Logs(context.Background(), `{env="production"}`, window, processor.LogsPage{Limit: 100})
				require.NoError(t, err)
				require.Len(t, res.Lines, 100)

				// about twenty lines a minute: a page that read past its
				// newest quarter of an hour read lines it did not need.
				oldest := res.Lines[len(res.Lines)-1].Time
				assert.True(t, oldest.After(window.To.Add(-15*time.Minute)), "page reached back to %s", oldest)
			},
		},
		"Nothing past now": {
			Check: func(t *testing.T) {
				now := timeutil.Now()
				lines := readAllLogs(t, `{env="production"}`, processor.TimeRange{From: now.Add(-10 * time.Minute), To: now.Add(time.Hour)})

				require.NotEmpty(t, lines)

				for _, l := range lines {
					assert.False(t, l.Time.After(timeutil.Now()), "line at %s is in the future", l.Time)
				}
			},
		},
		"Filters keep only the matching lines": {
			Check: func(t *testing.T) {
				lines := readAllLogs(t, `{service_name="deploy-bot"} |= "rolled back" | detected_level="error"`, _logDay)

				require.NotEmpty(t, lines)

				for _, l := range lines {
					assert.Equal(t, "deploy-bot", l.Labels[_logLabelService])
					assert.Equal(t, "rolled_back", l.Fields[_labelOutcome])
					assert.Equal(t, processor.LogLevelError, l.Level)
				}
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Check != nil {
				c.Check(t)

				return
			}

			_, err := _client.Logs(context.Background(), c.Query, hour, processor.LogsPage{Limit: 10})
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_Client_LogsVolume(t *testing.T) {
	// a pipeline that keeps every line forces the line-by-line count.
	const (
		allLines = `{env="production"}`
		readEach = `{env="production"} |= ""`
	)

	cc := map[string]struct {
		Query     string
		Range     processor.TimeRange
		Cancelled bool
		Compare   string
		Check     func(t *testing.T, res *processor.QueryResult)
		Result    *processor.QueryResult
		Err       error
	}{
		"Edge buckets are whole": {
			Query: allLines,
			Range: processor.TimeRange{From: _logDay.From.Add(90 * time.Second), To: _logDay.From.Add(3*time.Hour + 17*time.Second)},
			Check: func(t *testing.T, res *processor.QueryResult) {
				// three hours step two minutes: the first bucket starts at
				// the day's start, 90 seconds before the window does.
				var want float64

				for tick := tickAt(_logDay.From); tick < tickAt(_logDay.From.Add(2*time.Minute)); tick++ {
					for _, s := range _client.logSources {
						want += float64(len(s.linesAt(tick)))
					}
				}

				var got float64

				for _, series := range res.Data {
					for _, p := range series.Metrics {
						if p[0] == _logDay.From.Unix() {
							got += p[1].(float64)
						}
					}
				}

				assert.Positive(t, want)
				assert.Equal(t, want, got)
			},
		},
		"Query the demo does not answer": {
			Query: `rate({env="production"}[1m])`,
			Range: _logDay,
			Err:   _errUnsupportedLogQuery,
		},
		"Cancelled context": {
			Query:     allLines,
			Range:     _logDay,
			Cancelled: true,
			Err:       context.Canceled,
		},
		"Window past the retention": {
			Query:  allLines,
			Range:  _logExpired,
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
		},
		"Window before the demo starts": {
			Query:  allLines,
			Range:  processor.TimeRange{From: _epoch.AddDate(0, 0, -1), To: _epoch.Add(-time.Hour)},
			Result: &processor.QueryResult{Status: processor.QueryStatusNoData},
		},
		"Whole ticks count as the lines do over a day": {
			Query:   allLines,
			Range:   _logDay,
			Compare: readEach,
		},
		"Whole ticks count as the lines do over an unaligned window": {
			Query:   allLines,
			Range:   processor.TimeRange{From: _logDay.From.Add(90 * time.Second), To: _logDay.From.Add(3*time.Hour + 17*time.Second)},
			Compare: readEach,
		},
		"Steps shorter than a tick count the lines": {
			Query:   `{service_name="pizza-bot"}`,
			Range:   processor.TimeRange{From: _logDay.From, To: _logDay.From.Add(20 * time.Minute)},
			Compare: `{service_name="pizza-bot"} |= ""`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			res, err := _client.LogsVolume(ctx, c.Query, c.Range)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Result != nil {
				assert.Equal(t, c.Result, res)

				return
			}

			if c.Check != nil {
				c.Check(t, res)

				return
			}

			want, err := _client.LogsVolume(context.Background(), c.Compare, c.Range)
			require.NoError(t, err)
			require.Equal(t, processor.QueryStatusOK, res.Status)
			assert.Equal(t, want, res)
		})
	}
}

func Test_Client_LogFields(t *testing.T) {
	labels := []processor.LogField{
		{Name: "env", Kind: processor.LogFieldKindLabel},
		{Name: "service_name", Kind: processor.LogFieldKindLabel},
	}

	field := func(name string) processor.LogField {
		return processor.LogField{Name: name, Kind: processor.LogFieldKindField}
	}

	cc := map[string]struct {
		Query  string
		Result *processor.LogFieldsResult
		Err    error
	}{
		"Query the demo does not answer": {
			Query: `{env="production"} | logfmt`,
			Err:   _errUnsupportedLogQuery,
		},
		"Every source": {
			Result: &processor.LogFieldsResult{Fields: append(labels,
				field("confidence"), field("count"), field("detected_level"), field("duration"),
				field("excuse"), field("outcome"), field("pizza_count"), field("pr"),
				field("reviewer_state"), field("severity"), field("sha"), field("trace_id"),
				field("type"), field("user"),
			)},
		},
		"Sources the query selects": {
			Query: `{service_name="pizza-bot"}`,
			Result: &processor.LogFieldsResult{Fields: append(labels,
				field("count"), field("detected_level"), field("pizza_count"), field("trace_id"), field("user"),
			)},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := _client.LogFields(context.Background(), c.Query, _logDay)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Client_LogFieldValues(t *testing.T) {
	cc := map[string]struct {
		Field     string
		Query     string
		Range     processor.TimeRange
		Cancelled bool
		Values    []string
		Count     int
		Err       error
	}{
		"Cancelled context while reading lines": {
			Field:     "user",
			Cancelled: true,
			Err:       context.Canceled,
		},
		"Lines past the retention carry no values": {
			Field:  "user",
			Range:  _logExpired,
			Values: []string{},
		},
		"Query the demo does not answer": {
			Field: "service_name",
			Query: `{env="production"} | json`,
			Err:   _errUnsupportedLogQuery,
		},
		"Services": {
			Field:  "service_name",
			Values: []string{"calendar", "deploy-bot", "pizza-bot", "pr-bot", "secret-scanner"},
		},
		"Services the query selects": {
			Field:  "service_name",
			Query:  `{service_name=~"p.*"}`,
			Values: []string{"pizza-bot", "pr-bot"},
		},
		"Labels the pipeline does not narrow": {
			Field:  "service_name",
			Query:  `{service_name=~"p.*"} |= "no line says this"`,
			Values: []string{"pizza-bot", "pr-bot"},
		},
		"Environment": {
			Field:  "env",
			Values: []string{"production"},
		},
		"Levels": {
			Field:  "detected_level",
			Query:  `{service_name="secret-scanner"}`,
			Values: []string{"critical", "error", "warning"},
		},
		"Levels the pipeline keeps": {
			Field:  "detected_level",
			Query:  `{service_name="deploy-bot"} | detected_level="error"`,
			Values: []string{"error"},
		},
		"Bucket of a metric": {
			Field:  "severity",
			Values: []string{"critical", "normal", "nuclear"},
		},
		"Users read from the lines": {
			Field:  "user",
			Values: _logUsers,
		},
		"Trace ids read from the newest lines": {
			Field: "trace_id",
			Count: 1000,
		},
		"Field no line carries": {
			Field:  "team",
			Values: []string{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.Cancelled {
				cancel()
			}

			tr := _logDay
			if !c.Range.From.IsZero() {
				tr = c.Range
			}

			res, err := _client.LogFieldValues(ctx, c.Field, c.Query, tr)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			values := make([]string, 0, len(res.Values))

			for _, v := range res.Values {
				assert.False(t, v.Count.Valid)

				values = append(values, v.Value)
			}

			if c.Count > 0 {
				assert.Len(t, values, c.Count)

				return
			}

			assert.Equal(t, c.Values, values)
		})
	}
}

func Test_Client_fetchLines(t *testing.T) {
	cc := map[string]struct {
		Query   string
		Context func() context.Context
		Limit   int
		Err     error
	}{
		"Cancelled context": {
			Query: `{env="production"}`,
			Context: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				return ctx
			},
			Limit: 10,
			Err:   context.Canceled,
		},
		"Newest lines up to the limit": {
			Query: `{env="production"}`,
			Limit: 50,
		},
		"Selector that picks no source": {
			Query: `{service_name="billing"}`,
			Limit: 50,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lq, err := _client.parseLogQuery(c.Query)
			require.NoError(t, err)

			ctx := context.Background()
			if c.Context != nil {
				ctx = c.Context()
			}

			hour := processor.TimeRange{From: _logDay.From, To: _logDay.From.Add(time.Hour)}

			lines, err := _client.fetchLines(lq)(ctx, hour.From, hour.To, c.Limit)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if len(_client.logSourcesFor(lq)) == 0 {
				assert.Empty(t, lines)

				return
			}

			require.Len(t, lines, c.Limit)

			for i, l := range lines {
				assert.False(t, l.Time.Before(hour.From))
				assert.True(t, l.Time.Before(hour.To))

				if i > 0 {
					assert.False(t, l.Time.After(lines[i-1].Time), "line %d out of order", i)
				}
			}
		})
	}
}

func Test_Client_logSourcesFor(t *testing.T) {
	cc := map[string]struct {
		Query string
		Count int
	}{
		"Every source":    {Query: `{env="production"}`, Count: 25},
		"One service":     {Query: `{service_name="deploy-bot"}`, Count: 3},
		"No such service": {Query: `{service_name="billing"}`, Count: 0},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			lq, err := _client.parseLogQuery(c.Query)
			require.NoError(t, err)
			assert.Len(t, _client.logSourcesFor(lq), c.Count)
		})
	}
}

func Test_minTime(t *testing.T) {
	early := _logDay.From
	late := _logDay.To

	assert.Equal(t, early, minTime(early, late))
	assert.Equal(t, early, minTime(late, early))
}

func Test_retained(t *testing.T) {
	cc := map[string]struct {
		Range  processor.TimeRange
		Starts time.Duration
	}{
		"Window inside the retention is kept whole": {Range: _logDay},
		"Window reaching past it starts where it starts": {
			Range:  _logStraddling,
			Starts: _logsRetention,
		},
		"Window past it is cut to nothing": {
			Range:  _logExpired,
			Starts: _logsRetention,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res := retained(c.Range)
			assert.Equal(t, c.Range.To, res.To)

			if c.Starts == 0 {
				assert.Equal(t, c.Range.From, res.From)

				return
			}

			assert.WithinDuration(t, timeutil.Now().Add(-c.Starts), res.From, time.Minute)
		})
	}
}
