package processor

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

const (
	// _victoriaLogsQueryPath returns the logs a query matches.
	_victoriaLogsQueryPath = "/select/logsql/query"

	// _victoriaLogsHitsPath counts the logs a query matches per time
	// bucket.
	_victoriaLogsHitsPath = "/select/logsql/hits"

	// _victoriaLogsFieldNamesPath lists the field names of the logs a
	// query matches.
	_victoriaLogsFieldNamesPath = "/select/logsql/field_names"

	// _victoriaLogsStreamFieldNamesPath lists the stream field names of
	// the logs a query matches.
	_victoriaLogsStreamFieldNamesPath = "/select/logsql/stream_field_names"

	// _victoriaLogsFieldValuesPath lists the values one field takes in the
	// logs a query matches.
	_victoriaLogsFieldValuesPath = "/select/logsql/field_values"

	// _victoriaLogsMatchAll is the query that matches every log. The
	// endpoints refuse an empty one.
	_victoriaLogsMatchAll = "*"

	// _victoriaLogsTime is the field holding a log's time.
	_victoriaLogsTime = "_time"

	// _victoriaLogsMsg is the field holding a log's message.
	_victoriaLogsMsg = "_msg"

	// _victoriaLogsStream is the field holding a log's stream fields, as
	// {name="value",...}.
	_victoriaLogsStream = "_stream"

	// _victoriaLogsStreamID is the field holding a log's stream ID.
	_victoriaLogsStreamID = "_stream_id"

	// _victoriaLogsTestWindow is how far back the connection test looks.
	_victoriaLogsTestWindow = 5 * time.Minute
)

// _victoriaLogsLevelFields are the fields a log's level is read from, the
// first one present winning. VictoriaLogs stores an OpenTelemetry log's
// level in severity.
var _victoriaLogsLevelFields = []string{"level", "detected_level", "lvl", "loglevel", "severity"}

// _errVictoriaLogsNoTime is returned when the query returns a row without
// a time, such as the rows of a stats pipe.
var _errVictoriaLogsNoTime = NewInvalidQueryError("the query returned a row without _time")

// VictoriaLogs represents a VictoriaLogs data source processor.
type VictoriaLogs struct {
	httpClient
}

// NewVictoriaLogs creates a new VictoriaLogs data source processor.
func NewVictoriaLogs(inp Input) *VictoriaLogs {
	return &VictoriaLogs{
		httpClient{inp: inp},
	}
}

// Close closes the idle connections of the data source's own transport.
func (vl *VictoriaLogs) Close() error {
	vl.transport.close()

	return nil
}

// TestConnection tests the connection to the VictoriaLogs data source by
// listing the field names of its newest logs. VictoriaLogs reports no
// version, so any server that answers in its shape passes.
func (vl *VictoriaLogs) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	client, err := vl.authClient()
	if err != nil {
		if errors.Is(err, credentials.ErrInvalidCACertificate) {
			return ConnectionStatusInvalidCACertificate, nil
		}

		return "", err
	}

	to := timeutil.Now()
	params := createVictoriaLogsWindowParams("", TimeRange{From: to.Add(-_victoriaLogsTestWindow), To: to})

	status, body, err := vl.doRequest(ctx, client, _victoriaLogsFieldNamesPath, params, nil)
	if err != nil {
		return ConnectionStatusUnreachable, nil
	}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ConnectionStatusUnauthorized, nil
	case status != http.StatusOK:
		return ConnectionStatusUnreachable, nil
	}

	var res struct {
		// Values specifies the field names. It is nil when the answer is
		// not a list of values.
		Values *[]victoriaLogsValue `json:"values"`
	}

	// a 200 that is not a list of values comes from something other than
	// VictoriaLogs.
	if json.Unmarshal(body, &res) != nil || res.Values == nil {
		return ConnectionStatusUnreachable, nil
	}

	return ConnectionStatusSuccess, nil
}

// Logs reads one page of the logs a LogsQL query matches in the window.
func (vl *VictoriaLogs) Logs(ctx context.Context, q string, tr TimeRange, page LogsPage) (*LogsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	return PageLogs(ctx, vl.fetchLines(q, tr.To), tr, page, time.Nanosecond)
}

// LogsVolume counts the logs a LogsQL query matches per time bucket,
// split by level. A bucket starting at t covers [t, t + step). The first
// and last bucket are whole, so they can count logs just outside the
// window.
func (vl *VictoriaLogs) LogsVolume(ctx context.Context, q string, tr TimeRange) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	step := tr.VolumeStep()
	stepSeconds := int64(step.Seconds())
	first := tr.From.Unix() / stepSeconds * stepSeconds
	last := (tr.To.Unix() - 1) / stepSeconds * stepSeconds

	params := createVictoriaLogsWindowParams(q, TimeRange{
		From: time.Unix(first, 0),
		To:   time.Unix(last+stepSeconds, 0),
	})
	// the whole buckets can end past the window, so time keeps relative
	// filters on the window's end, as the lines do.
	params.Set("time", tr.To.UTC().Format(time.RFC3339Nano))
	params.Set("step", formatInterval(step))

	for _, f := range _victoriaLogsLevelFields {
		params.Add("field", f)
	}

	res, err := vl.fetchJSON[victoriaLogsHitsResponse](ctx, _victoriaLogsHitsPath, params, nil)
	if err != nil {
		return nil, err
	}

	counts := LogVolume{}

	for _, h := range res.Hits {
		lvl := newLogLevel(parseVictoriaLogsLevel(h.Fields))

		for i, ts := range h.Timestamps {
			if i < len(h.Values) {
				counts.Add(lvl, ts.Unix(), h.Values[i])
			}
		}
	}

	return counts.Result(first, last, stepSeconds), nil
}

// LogFields lists the field names the logs a LogsQL query matches carry in
// the window, every log when q is empty. Stream fields are labels.
func (vl *VictoriaLogs) LogFields(ctx context.Context, q string, tr TimeRange) (*LogFieldsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	params := createVictoriaLogsWindowParams(q, tr)

	streams, err := vl.fetchJSON[victoriaLogsListResponse](ctx, _victoriaLogsStreamFieldNamesPath, params, nil)
	if err != nil {
		return nil, err
	}

	all, err := vl.fetchJSON[victoriaLogsListResponse](ctx, _victoriaLogsFieldNamesPath, params, nil)
	if err != nil {
		return nil, err
	}

	labels := streams.names()
	res := &LogFieldsResult{Fields: make([]LogField, 0, len(all.Values))}

	for _, name := range slices.Sorted(slices.Values(labels)) {
		res.Fields = append(res.Fields, LogField{Name: name, Kind: LogFieldKindLabel})
	}

	var fields []string

	for _, name := range all.names() {
		if !slices.Contains(labels, name) && !isVictoriaLogsInternal(name) {
			fields = append(fields, name)
		}
	}

	for _, name := range slices.Sorted(slices.Values(fields)) {
		res.Fields = append(res.Fields, LogField{Name: name, Kind: LogFieldKindField})
	}

	return res, nil
}

// LogFieldValues lists the values one field takes in the logs a LogsQL
// query matches in the window, every log when q is empty. A value is
// counted unless the list was cut, as VictoriaLogs then counts none. The
// logs without the field, which VictoriaLogs lists as an empty value, are
// left out.
func (vl *VictoriaLogs) LogFieldValues(ctx context.Context, field, q string, tr TimeRange) (*LogFieldValuesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	params := createVictoriaLogsWindowParams(q, tr)
	params.Set("field", field)
	params.Set("limit", strconv.Itoa(_maxLogFieldValues))

	values, err := vl.fetchJSON[victoriaLogsListResponse](ctx, _victoriaLogsFieldValuesPath, params, nil)
	if err != nil {
		return nil, err
	}

	res := &LogFieldValuesResult{Values: make([]LogFieldValue, 0, len(values.Values))}

	listed := slices.DeleteFunc(values.Values, func(v victoriaLogsValue) bool { return v.Value == "" })

	for _, v := range listed[:min(len(listed), _maxLogFieldValues)] {
		res.Values = append(res.Values, LogFieldValue{Value: v.Value, Count: null.NewInt(v.Hits, v.Hits > 0)})
	}

	return res, nil
}

// fetchLines returns the fetcher that reads q's logs from VictoriaLogs,
// with relative filters such as _time:5m counted back from at.
//
// The limit arg picks the newest logs. Only v1.54.0 can pick the oldest
// with it, so an asc read picks them with a sort pipe after q instead.
// The pipe goes on its own line, because a # comment at the end of q
// would hide it.
func (vl *VictoriaLogs) fetchLines(q string, at time.Time) LogsFetcher {
	return func(ctx context.Context, from, end time.Time, limit int, order LogsOrder) ([]LogLine, error) {
		params := createVictoriaLogsWindowParams(q, TimeRange{From: from, To: end})

		// relative filters count back from the time arg, which is end when
		// unset. A page moves end, so time stays at the window's end.
		params.Set("time", at.UTC().Format(time.RFC3339Nano))

		if order == LogsOrderAsc {
			params.Set("query", fmt.Sprintf("%s\n| sort by (%s asc) limit %d", params.Get("query"), _victoriaLogsTime, limit))
		} else {
			params.Set("limit", strconv.Itoa(limit))
		}

		body, err := vl.fetchRaw(ctx, _victoriaLogsQueryPath, params, nil)
		if err != nil {
			return nil, err
		}

		var lines []LogLine

		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(nil, _maxHTTPResponseSize)

		for sc.Scan() {
			if len(bytes.TrimSpace(sc.Bytes())) == 0 {
				continue
			}

			var row victoriaLogsRow

			if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
				return nil, fmt.Errorf("decoding victorialogs log: %w", err)
			}

			line, err := row.logLine()
			if err != nil {
				return nil, err
			}

			lines = append(lines, line)
		}

		if err := sc.Err(); err != nil {
			// NOCOV: the body is in memory and under the scanner's cap.
			return nil, fmt.Errorf("reading victorialogs logs: %w", err)
		}

		// before v1.53.0 the sort pipe can return logs out of order after
		// pipes such as unpack_json, so they are sorted here.
		order.SortLines(lines)

		return lines, nil
	}
}

// victoriaLogsHitsResponse is the answer of the hits endpoint.
type victoriaLogsHitsResponse struct {
	// Hits specifies one series per distinct set of level fields.
	Hits []struct {
		// Fields specifies the level fields of the series.
		Fields map[string]string `json:"fields"`

		// Timestamps specifies the start of each bucket.
		Timestamps []time.Time `json:"timestamps"`

		// Values specifies the count of each bucket.
		Values []float64 `json:"values"`
	} `json:"hits"`
}

// victoriaLogsValue is one name or value of a listing, with how many logs
// carry it.
type victoriaLogsValue struct {
	// Value specifies the name or value.
	Value string `json:"value"`

	// Hits specifies how many logs carry it. It is zero when VictoriaLogs
	// cut the listing.
	Hits int64 `json:"hits"`
}

// victoriaLogsListResponse is the answer to a request that lists names or
// values.
type victoriaLogsListResponse struct {
	// Values specifies the names or values.
	Values []victoriaLogsValue `json:"values"`
}

// names returns the listed names, in their order.
func (vlr victoriaLogsListResponse) names() []string {
	names := make([]string, 0, len(vlr.Values))

	for _, v := range vlr.Values {
		names = append(names, v.Value)
	}

	return names
}

// victoriaLogsRow is one log as the query endpoint returns it, every
// field a string.
type victoriaLogsRow map[string]string

// logLine returns the row as a normalized line.
func (vr victoriaLogsRow) logLine() (LogLine, error) {
	raw, ok := vr[_victoriaLogsTime]
	if !ok {
		return LogLine{}, _errVictoriaLogsNoTime
	}

	// a pipe can overwrite _time and _stream, so a value that does not
	// parse is the query's fault.
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return LogLine{}, NewInvalidQueryError(fmt.Sprintf("the query returned a _time that is not a time: %q", raw))
	}

	labels, err := parseVictoriaLogsStream(vr[_victoriaLogsStream])
	if err != nil {
		return LogLine{}, NewInvalidQueryError(err.Error())
	}

	fields := make(map[string]string, len(vr))

	for k, v := range vr {
		if _, ok := labels[k]; !ok && !isVictoriaLogsInternal(k) {
			fields[k] = v
		}
	}

	return LogLine{
		Time:   t.UTC(),
		Level:  newLogLevel(parseVictoriaLogsLevel(labels, fields)),
		Line:   vr[_victoriaLogsMsg],
		Labels: labels,
		Fields: fields,
	}, nil
}

// parseVictoriaLogsStream returns the fields of a stream written as
// {name="value",...}, each value a quoted Go string.
func parseVictoriaLogsStream(raw string) (map[string]string, error) {
	labels := map[string]string{}

	rest, ok := strings.CutPrefix(raw, "{")
	if !ok {
		if raw == "" {
			return labels, nil
		}

		return nil, fmt.Errorf("victorialogs stream %q does not start with {", raw)
	}

	for rest != "}" {
		name, after, found := strings.Cut(rest, "=")
		if !found || name == "" {
			return nil, fmt.Errorf("victorialogs stream %q has a field without a name", raw)
		}

		quoted, err := strconv.QuotedPrefix(after)
		if err != nil {
			return nil, fmt.Errorf("parsing victorialogs stream %q: %w", raw, err)
		}

		value, err := strconv.Unquote(quoted)
		if err != nil {
			// NOCOV: QuotedPrefix only returns what Unquote accepts.
			return nil, fmt.Errorf("parsing victorialogs stream %q: %w", raw, err)
		}

		labels[name] = value
		rest = after[len(quoted):]

		if next, ok := strings.CutPrefix(rest, ","); ok {
			rest = next
		} else if rest != "}" {
			return nil, fmt.Errorf("victorialogs stream %q is not closed", raw)
		}
	}

	return labels, nil
}

// parseVictoriaLogsLevel returns the first non-empty level field in mm,
// taking the fields in their order across every map.
func parseVictoriaLogsLevel(mm ...map[string]string) string {
	for _, f := range _victoriaLogsLevelFields {
		for _, m := range mm {
			if v := m[f]; v != "" {
				return v
			}
		}
	}

	return ""
}

// isVictoriaLogsInternal reports whether a field is one VictoriaLogs
// keeps for itself, which the line carries in its own place.
func isVictoriaLogsInternal(name string) bool {
	switch name {
	case _victoriaLogsTime, _victoriaLogsMsg, _victoriaLogsStream, _victoriaLogsStreamID:
		return true
	default:
		return false
	}
}

// createVictoriaLogsWindowParams returns the params that scope a request
// to the window and to q, every log when q is empty. The end is exclusive.
func createVictoriaLogsWindowParams(q string, tr TimeRange) url.Values {
	return url.Values{
		"query": {cmp.Or(q, _victoriaLogsMatchAll)},
		"start": {tr.From.UTC().Format(time.RFC3339Nano)},
		"end":   {tr.To.UTC().Format(time.RFC3339Nano)},
	}
}
