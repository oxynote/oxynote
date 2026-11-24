package processor

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/prometheus/common/model"
)

const (
	// _lokiBuildInfoPath reports the server's version.
	_lokiBuildInfoPath = "/loki/api/v1/status/buildinfo"

	// _lokiQueryRangePath runs log and metric queries over a window.
	_lokiQueryRangePath = "/loki/api/v1/query_range"

	// _lokiLabelsPath lists the indexed label names.
	_lokiLabelsPath = "/loki/api/v1/labels"

	// _lokiDetectedFieldsPath lists structured metadata and the keys a
	// parser can extract from the lines.
	_lokiDetectedFieldsPath = "/loki/api/v1/detected_fields"

	// _lokiEncodingFlagsHeader names the header that changes how Loki
	// encodes a streams result.
	_lokiEncodingFlagsHeader = "X-Loki-Response-Encoding-Flags"

	// _lokiCategorizeLabels keeps structured metadata and parsed keys out
	// of the stream labels. Without it every distinct set of them is a
	// stream of its own.
	_lokiCategorizeLabels = "categorize-labels"

	// _lokiResultStreams is the result type of a log query.
	_lokiResultStreams = "streams"

	// _lokiMinMajorVersion is the oldest Loki major version supported. It
	// is the first to know the categorize-labels encoding flag.
	_lokiMinMajorVersion = 3

	// _lokiEntryLine is the position of the line in an entry.
	_lokiEntryLine = 1

	// _lokiEntryMeta is the position of the optional metadata object in an
	// entry.
	_lokiEntryMeta = 2

	// _lokiDetectedLevel is where Loki 3.1 and later store a line's level.
	_lokiDetectedLevel = "detected_level"

	// _lokiLevel is where Loki 3.0 stores a line's level, and a common
	// label name.
	_lokiLevel = "level"
)

// Loki represents a Loki data source processor.
type Loki struct {
	httpClient
}

// NewLoki creates a new Loki data source processor.
func NewLoki(inp Input) *Loki {
	return &Loki{
		httpClient{inp: inp},
	}
}

// Close closes the idle connections of the data source's own transport.
func (l *Loki) Close() error {
	l.transport.close()

	return nil
}

// TestConnection tests the connection to the Loki data source.
//
// A version that does not parse is accepted: hosted Loki services report
// their own build names rather than release numbers.
func (l *Loki) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	client, err := l.authClient()
	if err != nil {
		if errors.Is(err, credentials.ErrInvalidCACertificate) {
			return ConnectionStatusInvalidCACertificate, nil
		}

		return "", err
	}

	status, body, err := l.doRequest(ctx, client, _lokiBuildInfoPath, nil, nil, nil)
	if err != nil {
		return ConnectionStatusUnreachable, nil
	}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ConnectionStatusUnauthorized, nil
	case status != http.StatusOK:
		return ConnectionStatusUnreachable, nil
	}

	var info struct {
		// Version specifies the server's release.
		Version string `json:"version"`
	}

	// a 200 that is not build info comes from something other than Loki.
	if json.Unmarshal(body, &info) != nil {
		return ConnectionStatusUnreachable, nil
	}

	major, _, _ := strings.Cut(strings.TrimPrefix(info.Version, "v"), ".")
	if n, err := strconv.Atoi(major); err == nil && n < _lokiMinMajorVersion {
		return ConnectionStatusVersionNotSupported, nil
	}

	return ConnectionStatusSuccess, nil
}

// Logs reads one page of the lines a log query matches in the window.
func (l *Loki) Logs(ctx context.Context, q string, tr TimeRange, page LogsPage) (*LogsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	return PageLogs(ctx, l.fetchLines(q), tr, page, time.Nanosecond)
}

// LogsVolume counts the lines a log query matches per time bucket, split
// by level. A bucket starting at t covers [t, t + step). The first and
// last bucket are whole, so they can count lines just outside the window.
//
// Loki evaluates a range aggregation at T over (T - step, T], so the
// point at T is the bucket starting at T - step. A line exactly on a
// bucket boundary therefore lands one bucket earlier than in the other
// sources; Loki aligns the evaluation to the step, so this cannot be
// shifted.
func (l *Loki) LogsVolume(ctx context.Context, q string, tr TimeRange) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	step := tr.VolumeStep()
	stepSeconds := int64(step.Seconds())
	first := tr.From.Unix() / stepSeconds * stepSeconds
	last := (tr.To.Unix() - 1) / stepSeconds * stepSeconds

	expr := fmt.Sprintf(
		"sum by (%s, %s) (count_over_time(%s | drop __error__ [%s]))",
		_lokiDetectedLevel,
		_lokiLevel,
		q,
		formatInterval(step),
	)

	params := url.Values{
		"query": {expr},
		"start": {strconv.FormatInt(time.Unix(first+stepSeconds, 0).UnixNano(), 10)},
		"end":   {strconv.FormatInt(time.Unix(last+stepSeconds, 0).UnixNano(), 10)},
		"step":  {strconv.FormatInt(stepSeconds, 10)},
	}

	res, err := l.fetchJSON[lokiQueryResponse](ctx, _lokiQueryRangePath, params, nil)
	if err != nil {
		return nil, err
	}

	var matrix model.Matrix

	if err := json.Unmarshal(res.Data.Result, &matrix); err != nil {
		return nil, fmt.Errorf("decoding loki volume: %w", err)
	}

	counts := LogVolume{}

	for _, ss := range matrix {
		lvl := newLogLevel(cmp.Or(
			string(ss.Metric[_lokiDetectedLevel]),
			string(ss.Metric[_lokiLevel]),
		))

		for _, sp := range ss.Values {
			counts.Add(lvl, sp.Timestamp.Unix()-stepSeconds, float64(sp.Value))
		}
	}

	return counts.Result(first, last, stepSeconds), nil
}

// LogFields lists the field names the lines carry in the window: the
// indexed labels, and, when q is given, the structured metadata and the
// keys a parser finds in the lines q matches. The label endpoints take a
// stream selector only, so they get q's selector and nothing after it.
func (l *Loki) LogFields(ctx context.Context, q string, tr TimeRange) (*LogFieldsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	labels, err := l.fetchJSON[lokiListResponse](ctx, _lokiLabelsPath, createLokiWindowParams(LokiStreamSelector(q), tr), nil)
	if err != nil {
		return nil, err
	}

	res := &LogFieldsResult{Fields: make([]LogField, 0, len(labels.Data))}
	known := make(map[string]bool, len(labels.Data))

	for _, name := range slices.Sorted(slices.Values(labels.Data)) {
		res.Fields = append(res.Fields, LogField{Name: name, Kind: LogFieldKindLabel})
		known[name] = true
	}

	if q == "" {
		return res, nil
	}

	detected, err := l.fetchJSON[lokiDetectedFieldsResponse](ctx, _lokiDetectedFieldsPath, createLokiWindowParams(q, tr), nil)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(detected.Fields))

	for _, f := range detected.Fields {
		if !known[f.Label] {
			names = append(names, f.Label)
		}
	}

	for _, name := range slices.Sorted(slices.Values(names)) {
		res.Fields = append(res.Fields, LogField{Name: name, Kind: LogFieldKindField})
	}

	return res, nil
}

// LogFieldValues lists the values one field takes in the window. A label
// is read from the index, narrowed by q's stream selector; anything else
// is detected in the lines q matches, so it needs q, and Loki 3.3 or
// newer. Loki counts neither.
func (l *Loki) LogFieldValues(ctx context.Context, field, q string, tr TimeRange) (*LogFieldValuesResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	path := "/loki/api/v1/label/" + url.PathEscape(field) + "/values"

	values, err := l.fetchJSON[lokiListResponse](ctx, path, createLokiWindowParams(LokiStreamSelector(q), tr), nil)
	if err != nil {
		return nil, err
	}

	if len(values.Data) == 0 && q != "" {
		path = "/loki/api/v1/detected_field/" + url.PathEscape(field) + "/values"

		detected, err := l.fetchJSON[lokiDetectedValuesResponse](ctx, path, createLokiWindowParams(q, tr), nil)
		if err != nil {
			// a Loki older than 3.3 has no such endpoint, which leaves the
			// field without suggestions rather than failing them.
			if !errors.Is(err, _errEndpointNotFound) {
				return nil, err
			}
		}

		values.Data = detected.Values
	}

	return NewLogFieldValuesResult(values.Data), nil
}

// fetchLines returns the fetcher that reads q's lines from Loki.
func (l *Loki) fetchLines(q string) LogsFetcher {
	return func(ctx context.Context, from, end time.Time, limit int, order LogsOrder) ([]LogLine, error) {
		direction := "backward"
		if order == LogsOrderAsc {
			direction = "forward"
		}

		params := url.Values{
			"query":     {q},
			"start":     {strconv.FormatInt(from.UnixNano(), 10)},
			"end":       {strconv.FormatInt(end.UnixNano(), 10)},
			"limit":     {strconv.Itoa(limit)},
			"direction": {direction},
		}

		header := http.Header{_lokiEncodingFlagsHeader: {_lokiCategorizeLabels}}

		res, err := l.fetchJSON[lokiQueryResponse](ctx, _lokiQueryRangePath, params, header)
		if err != nil {
			return nil, err
		}

		if res.Data.ResultType != _lokiResultStreams {
			return nil, NewInvalidQueryError("a logs block needs a log query; a metric query such as rate() belongs in a metrics block")
		}

		// a server or proxy that drops the flag merges structured metadata
		// into the stream labels, which the lines cannot be told apart
		// from.
		if !slices.Contains(res.Data.EncodingFlags, _lokiCategorizeLabels) {
			return nil, ConnectionStatusVersionNotSupported.Error()
		}

		var streams []lokiStream

		if err := json.Unmarshal(res.Data.Result, &streams); err != nil {
			return nil, fmt.Errorf("decoding loki streams: %w", err)
		}

		var lines []LogLine

		for _, s := range streams {
			for _, e := range s.Values {
				lines = append(lines, e.logLine(s.Stream))
			}
		}

		// the result is sorted by stream labels, not by time.
		order.SortLines(lines)

		return lines[:min(len(lines), limit)], nil
	}
}

// lokiQueryResponse is the answer to a query_range request.
type lokiQueryResponse struct {
	// Data specifies the result.
	Data struct {
		// ResultType specifies whether the result holds streams or
		// series.
		ResultType string `json:"resultType"`

		// EncodingFlags specifies the encoding flags the server applied.
		EncodingFlags []string `json:"encodingFlags"`

		// Result specifies the streams or series, decoded per type.
		Result json.RawMessage `json:"result"`
	} `json:"data"`
}

// lokiListResponse is the answer to a request that lists names or
// values.
type lokiListResponse struct {
	// Data specifies the names or values.
	Data []string `json:"data"`
}

// lokiDetectedFieldsResponse is the answer of the detected fields
// endpoint.
type lokiDetectedFieldsResponse struct {
	// Fields specifies the names Loki found.
	Fields []struct {
		// Label specifies the name.
		Label string `json:"label"`
	} `json:"fields"`
}

// lokiDetectedValuesResponse is the answer of the detected field values
// endpoint.
type lokiDetectedValuesResponse struct {
	// Values specifies the values Loki found.
	Values []string `json:"values"`
}

// lokiStream is one stream of a streams result.
type lokiStream struct {
	// Stream specifies the stream's indexed labels.
	Stream map[string]string `json:"stream"`

	// Values specifies the stream's lines, newest first.
	Values []lokiEntry `json:"values"`
}

// lokiEntry is one line of a stream, encoded with the categorize-labels
// flag as [timestamp, line, {structuredMetadata, parsed}].
type lokiEntry struct {
	// ns is the line's time in nanoseconds since the epoch.
	ns int64

	// line is the message.
	line string

	// metadata is the line's structured metadata.
	metadata map[string]string

	// parsed is the keys a parser stage took from the line.
	parsed map[string]string
}

// UnmarshalJSON decodes an entry from its array form.
func (le *lokiEntry) UnmarshalJSON(data []byte) error {
	var parts []json.RawMessage

	if err := json.Unmarshal(data, &parts); err != nil {
		return fmt.Errorf("decoding loki entry: %w", err)
	}

	if len(parts) <= _lokiEntryLine {
		return fmt.Errorf("loki entry has %d parts, want a time and a line", len(parts))
	}

	var rawTime string

	if err := json.Unmarshal(parts[0], &rawTime); err != nil {
		return fmt.Errorf("decoding loki entry time: %w", err)
	}

	ns, err := strconv.ParseInt(rawTime, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing loki entry time: %w", err)
	}

	var line string

	if err := json.Unmarshal(parts[_lokiEntryLine], &line); err != nil {
		return fmt.Errorf("decoding loki entry line: %w", err)
	}

	var meta struct {
		// StructuredMetadata specifies the line's non-indexed labels.
		StructuredMetadata map[string]string `json:"structuredMetadata"`

		// Parsed specifies the keys a parser stage extracted.
		Parsed map[string]string `json:"parsed"`
	}

	if len(parts) > _lokiEntryMeta {
		if err := json.Unmarshal(parts[_lokiEntryMeta], &meta); err != nil {
			return fmt.Errorf("decoding loki entry metadata: %w", err)
		}
	}

	metadata := make(map[string]string, len(meta.StructuredMetadata))
	maps.Copy(metadata, meta.StructuredMetadata)

	*le = lokiEntry{
		ns:       ns,
		line:     line,
		metadata: metadata,
		parsed:   meta.Parsed,
	}

	return nil
}

// logLine returns the entry as a normalized line of the given stream.
func (le lokiEntry) logLine(stream map[string]string) LogLine {
	labels := make(map[string]string, len(stream))
	maps.Copy(labels, stream)

	return LogLine{
		Time: time.Unix(0, le.ns).UTC(),
		Level: newLogLevel(cmp.Or(
			le.parsed[_lokiDetectedLevel],
			le.metadata[_lokiDetectedLevel],
			le.parsed[_lokiLevel],
			le.metadata[_lokiLevel],
			labels[_lokiDetectedLevel],
			labels[_lokiLevel],
		)),
		Line:   le.line,
		Labels: labels,
		Fields: le.metadata,
		Parsed: le.parsed,
	}
}

// LokiStreamSelector returns the stream selector a LogQL query starts
// with, braces included, or "" when it does not start with a closed one.
// Braces inside quoted label values do not end it.
func LokiStreamSelector(q string) string {
	q = strings.TrimLeftFunc(q, unicode.IsSpace)

	if !strings.HasPrefix(q, "{") {
		return ""
	}

	var quote rune

	escaped := false

	for i, ch := range q {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && ch == '\\':
			escaped = true
		case quote != 0:
			if ch == quote {
				quote = 0
			}
		case ch == '"' || ch == '`':
			quote = ch
		case ch == '}':
			return q[:i+1]
		}
	}

	return ""
}

// createLokiWindowParams returns the params that scope a discovery request to
// the window and, when given, the query.
func createLokiWindowParams(q string, tr TimeRange) url.Values {
	params := url.Values{
		"start": {strconv.FormatInt(tr.From.UnixNano(), 10)},
		"end":   {strconv.FormatInt(tr.To.UnixNano(), 10)},
	}

	if q != "" {
		params.Set("query", q)
	}

	return params
}
