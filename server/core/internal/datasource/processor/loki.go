package processor

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/prometheus/common/config"
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

	// _maxErrorMessageSize caps how much of an error body is shown.
	_maxErrorMessageSize = 1024

	// _maxLokiResponseSize caps the bytes read from one answer: Loki lines
	// may be up to 256 KiB each, and a page may ask for thousands.
	_maxLokiResponseSize = 32 << 20
)

// _errLokiEndpointNotFound is returned when the server has no route for a
// request, which is what a URL that is not Loki, or a Loki too old for
// the endpoint, answers.
var _errLokiEndpointNotFound = errutil.New(
	http.StatusBadRequest,
	"data_source.endpoint_not_found",
	"the data source does not serve this request; check that its url points at loki 3.0 or newer",
)

// _errLokiRedirected is returned when the server answers with a redirect,
// which the client does not follow.
var _errLokiRedirected = errutil.New(
	http.StatusBadRequest,
	"data_source.redirected",
	"the data source answered with a redirect; set its url to the address it redirects to",
)

// _errLokiResponseTooLarge is returned when an answer holds more than
// _maxLokiResponseSize bytes.
var _errLokiResponseTooLarge = NewInvalidQueryError(
	"the data source answered with more than 32 MiB; narrow the query or lower the limit",
)

// Loki represents a Loki data source processor.
type Loki struct {
	inp       Input
	transport ownTransport
}

// NewLoki creates a new Loki data source processor.
func NewLoki(inp Input) *Loki {
	return &Loki{
		inp: inp,
	}
}

// Close closes the idle connections of the data source's own transport.
// Without a CA certificate its connections belong to the transport every
// client shares.
func (l *Loki) Close() error {
	l.transport.close()

	return nil
}

// TestConnection tests the connection to the Loki data source.
//
// A version that does not parse is accepted: Grafana Cloud reports its
// own build names rather than release numbers.
func (l *Loki) TestConnection(ctx context.Context) (ConnectionStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	client, err := l.client()
	if err != nil {
		if errors.Is(err, _errInvalidCACertificate) {
			return ConnectionStatusInvalidCACertificate, nil
		}

		return "", err
	}

	status, body, err := l.request(ctx, client, _lokiBuildInfoPath, nil, nil)
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

	step := tr.QueryStep()
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

	var res lokiQueryResponse

	if err := l.get(ctx, _lokiQueryRangePath, params, nil, &res); err != nil {
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

	return counts.Result(), nil
}

// LogFields lists the field names the lines carry in the window: the
// indexed labels, and, when q is given, the structured metadata and the
// keys a parser finds in the lines q matches. The label endpoints take a
// stream selector only, so they get q's selector and nothing after it.
func (l *Loki) LogFields(ctx context.Context, q string, tr TimeRange) (*LogFieldsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, _queryTimeout)
	defer cancel()

	var labels lokiListResponse

	if err := l.get(ctx, _lokiLabelsPath, lokiWindowParams(LokiStreamSelector(q), tr), nil, &labels); err != nil {
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

	var detected struct {
		// Fields specifies the names Loki found.
		Fields []struct {
			// Label specifies the name.
			Label string `json:"label"`
		} `json:"fields"`
	}

	if err := l.get(ctx, _lokiDetectedFieldsPath, lokiWindowParams(q, tr), nil, &detected); err != nil {
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

	var values lokiListResponse

	path := "/loki/api/v1/label/" + url.PathEscape(field) + "/values"
	if err := l.get(ctx, path, lokiWindowParams(LokiStreamSelector(q), tr), nil, &values); err != nil {
		return nil, err
	}

	if len(values.Data) == 0 && q != "" {
		var detected struct {
			// Values specifies the values Loki found.
			Values []string `json:"values"`
		}

		path = "/loki/api/v1/detected_field/" + url.PathEscape(field) + "/values"
		if err := l.get(ctx, path, lokiWindowParams(q, tr), nil, &detected); err != nil {
			// a Loki older than 3.3 has no such endpoint, which leaves the
			// field without suggestions rather than failing them.
			if !errors.Is(err, _errLokiEndpointNotFound) {
				return nil, err
			}
		}

		values.Data = detected.Values
	}

	return NewLogFieldValuesResult(values.Data), nil
}

// fetchLines returns the fetcher that reads q's lines from Loki.
func (l *Loki) fetchLines(q string) LogsFetcher {
	return func(ctx context.Context, from, end time.Time, limit int) ([]LogLine, error) {
		params := url.Values{
			"query":     {q},
			"start":     {strconv.FormatInt(from.UnixNano(), 10)},
			"end":       {strconv.FormatInt(end.UnixNano(), 10)},
			"limit":     {strconv.Itoa(limit)},
			"direction": {"backward"},
		}

		var res lokiQueryResponse

		header := http.Header{_lokiEncodingFlagsHeader: {_lokiCategorizeLabels}}
		if err := l.get(ctx, _lokiQueryRangePath, params, header, &res); err != nil {
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
		slices.SortStableFunc(lines, func(a, b LogLine) int {
			return b.Time.Compare(a.Time)
		})

		return lines[:min(len(lines), limit)], nil
	}
}

// get sends a GET to the path and decodes the JSON answer into dst. An
// answer other than 200 is the error it describes.
func (l *Loki) get(ctx context.Context, path string, params url.Values, header http.Header, dst any) error {
	client, err := l.client()
	if err != nil {
		return err
	}

	status, body, err := l.request(ctx, client, path, params, header)
	if err != nil {
		return err
	}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return ConnectionStatusUnauthorized.Error()
	case status >= http.StatusMultipleChoices && status < http.StatusBadRequest:
		return _errLokiRedirected
	case status == http.StatusNotFound:
		return _errLokiEndpointNotFound
	}

	// Loki answers an error in plain text; proxies such as Grafana Cloud
	// wrap it as JSON. Either is cut to _maxErrorMessageSize.
	if status != http.StatusOK {
		var wrapped struct {
			// Message specifies the error, as proxies write it.
			Message string `json:"message"`

			// Error specifies the error, as Prometheus-style APIs write it.
			Error string `json:"error"`
		}

		msg := strings.TrimSpace(string(body))

		if json.Unmarshal(body, &wrapped) == nil {
			msg = cmp.Or(wrapped.Message, wrapped.Error, msg)
		}

		if len(msg) > _maxErrorMessageSize {
			msg = msg[:_maxErrorMessageSize]
		}

		return NewInvalidQueryError(cmp.Or(msg, "the data source rejected the query"))
	}

	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decoding loki response: %w", err)
	}

	return nil
}

// request sends a GET to the path through client and returns the
// status and body.
func (l *Loki) request(
	ctx context.Context,
	client *http.Client,
	path string,
	params url.Values,
	header http.Header,
) (int, []byte, error) {
	target, err := url.JoinPath(l.inp.URL(), path)
	if err != nil {
		return 0, nil, fmt.Errorf("building loki url: %w", err)
	}

	if len(params) > 0 {
		target += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		// NOCOV: url.JoinPath already parsed the same target.
		return 0, nil, fmt.Errorf("creating loki request: %w", err)
	}

	maps.Copy(req.Header, header)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("querying loki: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	body, err := io.ReadAll(io.LimitReader(resp.Body, _maxLokiResponseSize+1))
	if err != nil {
		return 0, nil, fmt.Errorf("reading loki response: %w", err)
	}

	if len(body) > _maxLokiResponseSize {
		return 0, nil, _errLokiResponseTooLarge
	}

	return resp.StatusCode, body, nil
}

// client returns the HTTP client that authenticates with the data
// source's credentials and stays on the data source's own address.
func (l *Loki) client() (*http.Client, error) {
	rawCreds := l.inp.Credentials()

	creds, err := rawCreds.basic()
	if err != nil {
		return nil, err
	}

	transport, err := l.transport.roundTripper(http.DefaultTransport, l.inp.URL(), creds)
	if err != nil {
		return nil, err
	}

	// credentials holding only a CA certificate authenticate nobody.
	if creds.Username != "" || creds.Password != "" {
		transport = config.NewBasicAuthRoundTripper(
			config.NewInlineSecret(creds.Username),
			config.NewInlineSecret(creds.Password),
			transport,
		)
	}

	return &http.Client{
		Transport: transport,
		// a followed redirect could lead to any host, whose error answers
		// would then reach the user.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
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

	// fields is the line's structured metadata and parsed keys.
	fields map[string]string
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

	// parsed keys win over structured metadata of the same name, as in
	// Grafana.
	fields := make(map[string]string, len(meta.StructuredMetadata)+len(meta.Parsed))
	maps.Copy(fields, meta.StructuredMetadata)
	maps.Copy(fields, meta.Parsed)

	*le = lokiEntry{
		ns:     ns,
		line:   line,
		fields: fields,
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
			le.fields[_lokiDetectedLevel],
			le.fields[_lokiLevel],
			labels[_lokiDetectedLevel],
			labels[_lokiLevel],
		)),
		Line:   le.line,
		Labels: labels,
		Fields: le.fields,
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

// lokiWindowParams returns the params that scope a discovery request to
// the window and, when given, the query.
func lokiWindowParams(q string, tr TimeRange) url.Values {
	params := url.Values{
		"start": {strconv.FormatInt(tr.From.UnixNano(), 10)},
		"end":   {strconv.FormatInt(tr.To.UnixNano(), 10)},
	}

	if q != "" {
		params.Set("query", q)
	}

	return params
}
