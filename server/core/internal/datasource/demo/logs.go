package demo

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// _fieldValueLines is how many of the newest lines are read for the
// values of a field that varies per line.
const _fieldValueLines = 1000

// _logsRetention is how far back the demo keeps lines, as a real
// deployment's retention would. Every minute of a window is synthesized,
// so it is also what bounds the cost of one query.
const _logsRetention = 7 * 24 * time.Hour

// Logs reads one page of the lines a log query matches in the window,
// newest first.
func (c *Client) Logs(
	ctx context.Context,
	q string,
	tr processor.TimeRange,
	page processor.LogsPage,
) (*processor.LogsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	lq, err := c.parseLogQuery(q)
	if err != nil {
		return nil, err
	}

	return processor.PageLogs(ctx, c.fetchLines(lq), retained(tr), page, time.Nanosecond)
}

// LogsVolume counts the lines a log query matches per time bucket, split
// by level. A bucket starting at t covers [t, t + step). The first and
// last bucket are whole, as Loki counts them, so they can count lines
// just outside the window.
//
// A query without a pipeline counts whole ticks from the metric readings
// alone, which keeps a week-long window cheap; every other count reads
// the lines themselves.
func (c *Client) LogsVolume(ctx context.Context, q string, tr processor.TimeRange) (*processor.QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	lq, err := c.parseLogQuery(q)
	if err != nil {
		return nil, err
	}

	// the step follows the window asked for, so a chart's buckets do not
	// change size when part of the window is past the retention. The
	// window then grows to whole buckets before the retention cuts it.
	step := tr.QueryStep()
	stepSeconds := int64(step.Seconds())
	tr = retained(processor.TimeRange{
		From: time.Unix(tr.From.Unix()/stepSeconds*stepSeconds, 0).UTC(),
		To:   time.Unix(tr.To.Add(-time.Nanosecond).Unix()/stepSeconds*stepSeconds+stepSeconds, 0).UTC(),
	})
	end := minTime(tr.To, timeutil.Now().Add(time.Nanosecond))
	countTicks := len(lq.filters) == 0 && step%_tick == 0
	sources := c.logSourcesFor(lq)
	counts := processor.LogVolume{}

	for tick := max(tickAt(tr.From), 0); tick <= tickAt(end.Add(-time.Nanosecond)); tick++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		start := timeAt(tick)
		whole := countTicks && !start.Before(tr.From) && !start.Add(_tick).After(end)

		for _, s := range sources {
			if whole {
				if n := s.lineCount(s.streamAt(tick), tick); n > 0 {
					counts.Add(s.level, start.Unix()/stepSeconds*stepSeconds, float64(n))
				}

				continue
			}

			for _, l := range s.linesAt(tick) {
				if !l.Time.Before(tr.From) && l.Time.Before(end) && lq.matches(l) {
					counts.Add(l.Level, l.Time.Unix()/stepSeconds*stepSeconds, 1)
				}
			}
		}
	}

	return counts.Result(), nil
}

// LogFields lists the field names the demo's lines carry: its two stream
// labels, then the structured metadata of the sources q selects. Every
// source writes in every window, so the window narrows nothing.
func (c *Client) LogFields(_ context.Context, q string, _ processor.TimeRange) (*processor.LogFieldsResult, error) {
	sources := c.logSources

	if q != "" {
		lq, err := c.parseLogQuery(q)
		if err != nil {
			return nil, err
		}

		sources = c.logSourcesFor(lq)
	}

	res := &processor.LogFieldsResult{Fields: []processor.LogField{
		{Name: _logLabelEnv, Kind: processor.LogFieldKindLabel},
		{Name: _logLabelService, Kind: processor.LogFieldKindLabel},
	}}

	names := make(map[string]struct{})

	for _, s := range sources {
		for _, name := range s.fieldNames() {
			names[name] = struct{}{}
		}
	}

	for _, name := range slices.Sorted(maps.Keys(names)) {
		res.Fields = append(res.Fields, processor.LogField{Name: name, Kind: processor.LogFieldKindField})
	}

	return res, nil
}

// LogFieldValues lists the values one field takes. A value every line of
// a source shares is read off the sources q selects; one that varies per
// line is read from the newest lines in the window. The demo counts
// neither, as Loki does not.
func (c *Client) LogFieldValues(
	ctx context.Context,
	field, q string,
	tr processor.TimeRange,
) (*processor.LogFieldValuesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	tr = retained(tr)
	lq := logQuery{}

	if q != "" {
		var err error

		lq, err = c.parseLogQuery(q)
		if err != nil {
			return nil, err
		}
	}

	values := make(map[string]struct{})

	// a pipeline can drop some of a source's lines, so with one the values
	// are read from the lines it keeps rather than off the sources. Labels
	// still come off the sources, as Loki reads them from its index and
	// ignores the pipeline.
	if len(lq.filters) == 0 || field == _logLabelService || field == _logLabelEnv {
		for _, s := range c.logSourcesFor(lq) {
			switch field {
			case _logLabelService:
				values[s.service] = struct{}{}
			case _logLabelEnv:
				values[_logEnv] = struct{}{}
			case _logFieldLevel:
				values[string(s.level)] = struct{}{}
			case s.field:
				values[s.bucket] = struct{}{}
			}
		}
	}

	if len(values) == 0 {
		lines, err := c.fetchLines(lq)(ctx, tr.From, tr.To, _fieldValueLines)
		if err != nil {
			return nil, err
		}

		for _, l := range lines {
			if v, ok := l.Fields[field]; ok {
				values[v] = struct{}{}
			}
		}
	}

	return processor.NewLogFieldValuesResult(slices.Sorted(maps.Keys(values))), nil
}

// fetchLines returns the fetcher that reads the lines lq matches, walking
// the ticks back from the window's end until it has enough. Nothing past
// now is ever read.
func (c *Client) fetchLines(lq logQuery) processor.LogsFetcher {
	sources := c.logSourcesFor(lq)

	return func(ctx context.Context, from, end time.Time, limit int) ([]processor.LogLine, error) {
		end = minTime(end, timeutil.Now().Add(time.Nanosecond))

		var lines []processor.LogLine

		for tick := tickAt(end.Add(-time.Nanosecond)); tick >= max(tickAt(from), 0) && len(lines) < limit; tick-- {
			if err := ctx.Err(); err != nil {
				return nil, err
			}

			var tickLines []processor.LogLine

			for _, s := range sources {
				for _, l := range s.linesAt(tick) {
					if !l.Time.Before(from) && l.Time.Before(end) && lq.matches(l) {
						tickLines = append(tickLines, l)
					}
				}
			}

			slices.SortStableFunc(tickLines, func(a, b processor.LogLine) int {
				return b.Time.Compare(a.Time)
			})

			lines = append(lines, tickLines...)
		}

		return lines[:min(len(lines), limit)], nil
	}
}

// logSourcesFor returns the sources whose stream the query's selector
// picks.
func (c *Client) logSourcesFor(lq logQuery) []logSource {
	return slices.DeleteFunc(slices.Clone(c.logSources), func(s logSource) bool {
		return !lq.matchesStream(s.labels())
	})
}

// retained returns the part of the window the demo still keeps lines for,
// which is empty when the window ends before the retention starts.
func retained(tr processor.TimeRange) processor.TimeRange {
	if oldest := timeutil.Now().Add(-_logsRetention); tr.From.Before(oldest) {
		tr.From = oldest
	}

	return tr
}

// minTime returns the earlier of two instants.
func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}

	return a
}
