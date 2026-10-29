package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/orlangure/gnomock"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _lokiVersions are the Loki releases the integration tests run against:
// the oldest supported one and the newest.
var _lokiVersions = []string{"3.0.1", "3.7.9"}

var (
	// _lokiAddrs maps a Loki version to the address of its container.
	_lokiAddrs = map[string]string{}

	// _logsCorpus is the corpus every log container holds.
	_logsCorpus []corpusLine

	// _logsWindow is the hour the corpus spans.
	_logsWindow TimeRange
)

// corpusLine is one line of the shared logs corpus.
type corpusLine struct {
	// Seq specifies the line's position, which identifies it.
	Seq int

	// Time specifies when the line was written.
	Time time.Time

	// Service specifies the stream the line belongs to.
	Service string

	// Level specifies the line's level.
	Level string

	// TraceID specifies a high-cardinality value kept out of the index.
	TraceID string

	// Message specifies the line itself.
	Message string
}

// newLogsCorpus returns 6000 lines over the hour from base, with the
// traps that break log paging and decoding: one timestamp holding 300
// lines, groups of four sharing a timestamp, pairs one nanosecond apart,
// multi-line, JSON, unicode, quoted, empty and 20 KiB messages.
func newLogsCorpus(base time.Time) []corpusLine {
	const n = 6000

	r := rand.New(rand.NewPCG(42, 42)) //nolint:gosec // a reproducible corpus, not a secret

	offsets := make([]time.Duration, n)

	for i := range offsets {
		offsets[i] = time.Duration(r.Int64N(int64(time.Hour)))
	}

	slices.Sort(offsets)

	for i := 2000; i < 2300; i++ {
		offsets[i] = 30 * time.Minute
	}

	for g := range 50 {
		b := 100 + g*100
		for i := b; i < b+4; i++ {
			offsets[i] = offsets[b]
		}
	}

	for g := range 20 {
		i := 3025 + g*50
		offsets[i+1] = offsets[i] + time.Nanosecond
	}

	slices.Sort(offsets)

	services := []string{"checkout", "payments", "deploy-bot"}
	lines := make([]corpusLine, n)

	for seq, off := range offsets {
		service := services[r.IntN(len(services))]
		level := corpusLevel(r)

		lines[seq] = corpusLine{
			Seq:     seq,
			Time:    base.Add(off),
			Service: service,
			Level:   level,
			TraceID: fmt.Sprintf("%016x", r.Uint64()),
			Message: corpusMessage(r, service, level, seq),
		}
	}

	for _, seq := range []int{10, 2500, 5990} {
		lines[seq].Message = ""
	}

	lines[4000].Message = strings.Repeat("x", 20480)

	return lines
}

// corpusLevel draws a level: 70% info, 10% debug, 12% warn, 8% error.
func corpusLevel(r *rand.Rand) string {
	switch x := r.IntN(100); {
	case x < 70:
		return "info"
	case x < 80:
		return "debug"
	case x < 92:
		return "warn"
	default:
		return "error"
	}
}

// corpusMessage draws a message. Every one ends with its sequence number,
// because Loki drops a line equal to the one before it in its stream.
func corpusMessage(r *rand.Rand, service, level string, seq int) string {
	suffix := " seq=" + strconv.Itoa(seq)

	switch k := r.IntN(100); {
	case level == "error" && k < 30:
		return "panic: runtime error: invalid memory address or nil pointer dereference\n\n" +
			"goroutine 1 [running]:\nmain.handle()\n\t/app/handler.go:42 +0x1d" + suffix
	case (level == "error" || level == "warn") && k < 60:
		return "upstream timeout after 3000ms calling inventory" + suffix
	case k < 75:
		return `{"msg":"cart updated","items":` + strconv.Itoa(r.IntN(9)+1) + `,"seq":` + strconv.Itoa(seq) + `}`
	case k < 80:
		return "Užsakymas patvirtintas ✅ 注文確認 مرحبا" + suffix
	case k < 83:
		return `path "C:\temp\x" not found` + suffix
	default:
		return service + " handled a request" + suffix
	}
}

// startLoki starts a Loki container of the given version holding the
// corpus. Old lines are accepted and kept in the ingesters, which is
// where queries read them from.
func startLoki(version string, corpus []corpusLine) (*gnomock.Container, error) {
	return gnomock.StartCustom(
		"docker.io/grafana/loki:"+version,
		gnomock.DefaultTCP(3100),
		gnomock.WithCommand(
			"-config.file=/etc/loki/local-config.yaml",
			"-validation.reject-old-samples=false",
			"-ingester.max-chunk-age=168h",
			"-querier.query-ingesters-within=0",
		),
		gnomock.WithTimeout(3*time.Minute),
		gnomock.WithHealthCheck(func(ctx context.Context, c *gnomock.Container) error {
			return lokiRequest(ctx, http.MethodGet, "http://"+c.DefaultAddress()+"/ready", nil, http.StatusOK)
		}),
		gnomock.WithInit(func(ctx context.Context, c *gnomock.Container) error {
			return pushLokiCorpus(ctx, c.DefaultAddress(), corpus)
		}),
	)
}

// pushLokiCorpus pushes the corpus into the Loki at addr, one stream per
// service, the level, trace id and sequence number as structured
// metadata.
func pushLokiCorpus(ctx context.Context, addr string, corpus []corpusLine) error {
	type stream struct {
		Stream map[string]string `json:"stream"`
		Values [][]any           `json:"values"`
	}

	streams := map[string]*stream{}

	for _, l := range corpus {
		s, ok := streams[l.Service]
		if !ok {
			s = &stream{Stream: map[string]string{"service_name": l.Service, "env": "prod"}}
			streams[l.Service] = s
		}

		s.Values = append(s.Values, []any{
			strconv.FormatInt(l.Time.UnixNano(), 10),
			l.Message,
			map[string]string{"level": l.Level, "trace_id": l.TraceID, "seq": strconv.Itoa(l.Seq)},
		})
	}

	body := struct {
		Streams []*stream `json:"streams"`
	}{}

	for _, s := range streams {
		body.Streams = append(body.Streams, s)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}

	return lokiRequest(ctx, http.MethodPost, "http://"+addr+"/loki/api/v1/push", raw, http.StatusNoContent)
}

// lokiRequest sends a request to a test Loki and fails unless it answers
// with the wanted status.
func lokiRequest(ctx context.Context, method, target string, body []byte, want int) error {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode != want {
		return fmt.Errorf("loki answered %d, want %d", resp.StatusCode, want)
	}

	return nil
}

// prepLokiInput returns an input pointing at the given URL, or at a fake
// Loki serving h when the URL is empty. The cleanup drops the shared
// transport's keep-alive connections so goleak stays clean.
func prepLokiInput(t *testing.T, target string, h http.Handler) *InputMock {
	t.Helper()

	if target == "" {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)

		target = srv.URL
	}

	t.Cleanup(func() {
		http.DefaultTransport.(*http.Transport).CloseIdleConnections()
	})

	return &InputMock{
		URLFunc: func() string { return target },
	}
}

// corpusWhere returns the corpus lines in the window that match.
func corpusWhere(tr TimeRange, match func(corpusLine) bool) map[string]corpusLine {
	out := map[string]corpusLine{}

	for _, l := range _logsCorpus {
		if !l.Time.Before(tr.From) && l.Time.Before(tr.To) && match(l) {
			out[strconv.Itoa(l.Seq)] = l
		}
	}

	return out
}

// assertCorpusLines checks that lines are newest first and are exactly
// the expected corpus lines, each read once, each with its time, level, message, labels
// and metadata.
func assertCorpusLines(t *testing.T, want map[string]corpusLine, lines []LogLine) {
	t.Helper()

	assert.True(t, slices.IsSortedFunc(lines, func(a, b LogLine) int {
		return b.Time.Compare(a.Time)
	}), "lines are not newest first")

	got := make(map[string]LogLine, len(lines))

	for _, l := range lines {
		got[l.Fields["seq"]] = l
	}

	require.Len(t, lines, len(want), "a line was read twice or missed")
	require.Len(t, got, len(want))

	for seq, w := range want {
		g, ok := got[seq]
		require.True(t, ok, "line %s is missing", seq)

		assert.True(t, w.Time.Equal(g.Time), "line %s time", seq)
		assert.Equal(t, newLogLevel(w.Level), g.Level, "line %s level", seq)
		assert.Equal(t, w.Message, g.Line, "line %s message", seq)
		assert.Equal(t, map[string]string{"service_name": w.Service, "env": "prod"}, g.Labels, "line %s labels", seq)
		assert.Equal(t, w.TraceID, g.Fields["trace_id"], "line %s trace id", seq)
	}
}

func Test_NewLoki(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	l := NewLoki(inp)
	require.NotNil(t, l)
	assert.Same(t, inp, l.inp)
}

func Test_Loki_Close(t *testing.T) {
	t.Parallel()

	assert.NoError(t, NewLoki(&InputMock{}).Close())
}

func Test_Loki_TestConnection(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Version string

		// TLS indicates whether the handler is served over TLS, with
		// CACertificate as the credentials' CA, or the server's own
		// certificate when it is empty.
		TLS           bool
		CACertificate string

		Result ConnectionStatus
		Err    error
	}{
		"Error returned by Loki.client": {
			Inp: &InputMock{
				CredentialsFunc: func() Credentials { return NewCredentials([]byte(`{`)) },
			},
			Err: assert.AnError,
		},
		"Invalid CA certificate": {
			Inp: &InputMock{
				URLFunc:         func() string { return "https://loki.test" },
				CredentialsFunc: func() Credentials { return caCredentials("not a certificate") },
			},
			Result: ConnectionStatusInvalidCACertificate,
		},
		"Unreachable data source": {
			Inp:    &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Result: ConnectionStatusUnreachable,
		},
		"Server the CA certificate did not sign": {
			Handler:       testutil.JSONHandler(_lokiBuildInfoPath, http.StatusOK, `{"version":"v3.7.9"}`),
			TLS:           true,
			CACertificate: _testCA,
			Result:        ConnectionStatusUnreachable,
		},
		"Successful connection verified with a CA certificate": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusOK, `{"version":"v3.7.9"}`),
			TLS:     true,
			Result:  ConnectionStatusSuccess,
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusUnauthorized, `no org id`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Redirecting data source": {
			Handler: http.RedirectHandler("http://example.com/", http.StatusMovedPermanently),
			Result:  ConnectionStatusUnreachable,
		},
		"Forbidden data source": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusForbidden, `{}`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Not a Loki": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusNotFound, `not found`),
			Result:  ConnectionStatusUnreachable,
		},
		"Build info that is not JSON": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusOK, `<html>`),
			Result:  ConnectionStatusUnreachable,
		},
		"Unsupported version": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusOK, `{"version":"2.9.17"}`),
			Result:  ConnectionStatusVersionNotSupported,
		},
		"Unparsed version is accepted": {
			Handler: testutil.JSONHandler(_lokiBuildInfoPath, http.StatusOK, `{"version":"k330-5aa4b92"}`),
			Result:  ConnectionStatusSuccess,
		},
		"Successful connection with credentials": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if u, p, ok := r.BasicAuth(); !ok || u != "user" || p != "pass" {
					w.WriteHeader(http.StatusUnauthorized)

					return
				}

				_, err := w.Write([]byte(`{"version":"v3.7.9"}`))
				assert.NoError(t, err)
			}),
			Result: ConnectionStatusSuccess,
		},
		"Successful connection to the oldest supported Loki": {
			Version: "3.0.1",
			Result:  ConnectionStatusSuccess,
		},
		"Successful connection to the newest Loki": {
			Version: "3.7.9",
			Result:  ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp

			switch {
			case inp != nil:
				t.Cleanup(http.DefaultTransport.(*http.Transport).CloseIdleConnections)
			case c.TLS:
				inp = prepTLSInput(t, c.Handler, c.CACertificate)
			default:
				target := ""
				if c.Version != "" {
					target = "http://" + _lokiAddrs[c.Version]
				}

				inp = prepLokiInput(t, target, c.Handler)
				inp.CredentialsFunc = func() Credentials {
					return NewCredentials([]byte(`{"username":"user","password":"pass"}`))
				}
			}

			l := NewLoki(inp)

			t.Cleanup(func() { assert.NoError(t, l.Close()) })

			cs, err := l.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Loki_Logs(t *testing.T) {
	lastQuarter := TimeRange{
		From: _logsWindow.To.Add(-15 * time.Minute),
		To:   _logsWindow.To,
	}

	streamsBody := func(flags string) string {
		return `{"status":"success","data":{"resultType":"streams",` + flags + `"result":[` +
			`{"stream":{"service_name":"b"},"values":[["2000","second",{"structuredMetadata":{"level":"warn"}}]]},` +
			`{"stream":{"service_name":"a"},"values":[["3000","third",{}],["1000","first",{"parsed":{"detected_level":"error"}}]]}]}}`
	}

	cc := map[string]struct {
		Handler   http.Handler
		Query     string
		Range     TimeRange
		Page      LogsPage
		Match     func(corpusLine) bool
		WalkLimit int
		Result    *LogsResult
		Err       error
	}{
		"Error returned by Loki": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusBadRequest, `parse error at line 1, col 12: syntax error`),
			Range:   _logsWindow,
			Page:    LogsPage{Limit: 10},
			Err:     NewInvalidQueryError("parse error at line 1, col 12: syntax error"),
		},
		"Merged streams come back newest first": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, streamsBody(`"encodingFlags":["categorize-labels"],`)),
			Range:   TimeRange{From: time.Unix(0, 0), To: time.Unix(0, 4000)},
			Page:    LogsPage{Limit: 10},
			Result: &LogsResult{
				Status: QueryStatusOK,
				Lines: []LogLine{
					{Time: time.Unix(0, 3000).UTC(), Level: LogLevelUnknown, Line: "third", Labels: map[string]string{"service_name": "a"}, Fields: map[string]string{}},
					{Time: time.Unix(0, 2000).UTC(), Level: LogLevelWarning, Line: "second", Labels: map[string]string{"service_name": "b"}, Fields: map[string]string{"level": "warn"}},
					{Time: time.Unix(0, 1000).UTC(), Level: LogLevelError, Line: "first", Labels: map[string]string{"service_name": "a"}, Fields: map[string]string{"detected_level": "error"}},
				},
			},
		},
		"Label filter": {
			Query:     `{service_name="checkout"}`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return l.Service == "checkout" },
			WalkLimit: _maxLogsLimit,
		},
		"Level filter": {
			Query:     `{env="prod"} | level="error"`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return l.Level == "error" },
			WalkLimit: _maxLogsLimit,
		},
		"Text match": {
			Query:     `{env="prod"} |= "timeout"`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return strings.Contains(l.Message, "timeout") },
			WalkLimit: _maxLogsLimit,
		},
		"Last fifteen minutes": {
			Query:     `{env="prod"}`,
			Range:     lastQuarter,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: _maxLogsLimit,
		},
		"First page of a hundred": {
			Query: `{env="prod"}`,
			Range: _logsWindow,
			Page:  LogsPage{Limit: 100},
			Match: func(corpusLine) bool { return true },
		},
		"Walk at limit 7": {
			Query:     `{env="prod"}`,
			Range:     _logsWindow,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 7,
		},
		"Walk at limit 100": {
			Query:     `{env="prod"}`,
			Range:     _logsWindow,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 100,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewLoki(prepLokiInput(t, "", c.Handler)).Logs(context.Background(), c.Query, c.Range, c.Page)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				for i := range res.Lines {
					res.Lines[i].ID = ""
				}

				assert.Equal(t, c.Result, res)

				return
			}

			for _, v := range _lokiVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					l := NewLoki(prepLokiInput(t, "http://"+_lokiAddrs[v], nil))
					want := corpusWhere(c.Range, c.Match)

					if c.WalkLimit == 0 {
						res, err := l.Logs(context.Background(), c.Query, c.Range, c.Page)
						require.NoError(t, err)
						require.NotEmpty(t, res.Lines)

						oldest := res.Lines[len(res.Lines)-1].Time
						assertCorpusLines(t, corpusWhere(c.Range, func(cl corpusLine) bool {
							return !cl.Time.Before(oldest)
						}), res.Lines)
						assert.True(t, res.Cursor.Valid)

						return
					}

					assertCorpusLines(t, want, walkLogs(t, l.fetchLines(c.Query), c.Range, c.WalkLimit, time.Nanosecond))
				})
			}
		})
	}
}

func Test_Loki_fetchLines(t *testing.T) {
	cc := map[string]struct {
		Handler http.Handler
		Limit   int
		Result  []LogLine
		Err     error
	}{
		"Error returned by Loki.get": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusBadRequest, `bad`),
			Err:     NewInvalidQueryError("bad"),
		},
		"Metric query": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"matrix","result":[]}}`),
			Err:     NewInvalidQueryError("a logs block needs a log query; a metric query such as rate() belongs in a metrics block"),
		},
		"Encoding flag dropped": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"streams","result":[]}}`),
			Err:     ConnectionStatusVersionNotSupported.Error(),
		},
		"Streams that do not decode": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"streams","encodingFlags":["categorize-labels"],"result":{}}}`),
			Err:     assert.AnError,
		},
		"Lines past the limit are dropped": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"streams","encodingFlags":["categorize-labels"],"result":[`+
				`{"stream":{},"values":[["2","b"],["1","a"]]}]}}`),
			Limit: 1,
			Result: []LogLine{
				{Time: time.Unix(0, 2).UTC(), Level: LogLevelUnknown, Line: "b", Labels: map[string]string{}, Fields: map[string]string{}},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var query http.Header

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.Header.Clone()
				c.Handler.ServeHTTP(w, r)
			})

			fetch := NewLoki(prepLokiInput(t, "", h)).fetchLines(`{a="b"}`)

			lines, err := fetch(context.Background(), time.Unix(0, 0), time.Unix(0, 10), c.Limit)

			assert.Equal(t, _lokiCategorizeLabels, query.Get(_lokiEncodingFlagsHeader))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, lines)
		})
	}
}

func Test_Loki_LogsVolume(t *testing.T) {
	cc := map[string]struct {
		Handler http.Handler
		Range   TimeRange
		Result  *QueryResult
		Err     error
	}{
		"Error returned by Loki": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusBadRequest, `bad`),
			Range:   _logsWindow,
			Err:     NewInvalidQueryError("bad"),
		},
		"Result that does not decode": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"matrix","result":{}}}`),
			Range:   _logsWindow,
			Err:     assert.AnError,
		},
		"No lines": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"matrix","result":[]}}`),
			Range:   _logsWindow,
			Result:  &QueryResult{Status: QueryStatusNoData},
		},
		"Points become bucket starts and levels merge": {
			Handler: testutil.JSONHandler(_lokiQueryRangePath, http.StatusOK, `{"data":{"resultType":"matrix","result":[`+
				`{"metric":{"detected_level":"warn"},"values":[[75,"2"]]},`+
				`{"metric":{"level":"WARNING"},"values":[[75,"3"],[90,"1"]]},`+
				`{"metric":{},"values":[[90,"4"]]}]}}`),
			Range: TimeRange{From: time.Unix(60, 0), To: time.Unix(90, 0)},
			Result: &QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{Labels: map[string]string{"level": "unknown"}, Metrics: [][2]any{{int64(75), float64(4)}}},
					{Labels: map[string]string{"level": "warning"}, Metrics: [][2]any{{int64(60), float64(5)}, {int64(75), float64(1)}}},
				},
			},
		},
		"Volume of the corpus": {
			Range: _logsWindow,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewLoki(prepLokiInput(t, "", c.Handler)).LogsVolume(context.Background(), `{a="b"}`, c.Range)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				assert.Equal(t, c.Result, res)

				return
			}

			want := map[string]float64{}

			for _, l := range corpusWhere(c.Range, func(corpusLine) bool { return true }) {
				want[string(newLogLevel(l.Level))]++
			}

			for _, v := range _lokiVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := NewLoki(prepLokiInput(t, "http://"+_lokiAddrs[v], nil)).
						LogsVolume(context.Background(), `{env="prod"}`, c.Range)
					require.NoError(t, err)

					got := map[string]float64{}

					for _, s := range res.Data {
						for _, p := range s.Metrics {
							got[s.Labels["level"]] += p[1].(float64)
						}
					}

					assert.Equal(t, want, got)
				})
			}
		})
	}
}

func Test_Loki_LogFields(t *testing.T) {
	labels := []LogField{
		{Name: "env", Kind: LogFieldKindLabel},
		{Name: "service_name", Kind: LogFieldKindLabel},
	}

	cc := map[string]struct {
		Handler http.Handler
		Query   string
		Result  *LogFieldsResult
		Contain map[string][]LogField
		Err     error
	}{
		"Error returned by the labels request": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, `bad`),
			Err:     NewInvalidQueryError("bad"),
		},
		"Error returned by the detected fields request": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == _lokiLabelsPath {
					_, err := w.Write([]byte(`{"data":["env"]}`))
					assert.NoError(t, err)

					return
				}

				w.WriteHeader(http.StatusBadRequest)
			}),
			Query: `{a="b"}`,
			Err:   assert.AnError,
		},
		"Labels only without a query": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusOK, `{"data":["service_name","env"]}`),
			Result: &LogFieldsResult{Fields: []LogField{
				{Name: "env", Kind: LogFieldKindLabel},
				{Name: "service_name", Kind: LogFieldKindLabel},
			}},
		},
		"Labels then detected fields": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{"fields":[{"label":"trace_id"},{"label":"env"},{"label":"detected_level"}]}`
				query := `{a="b"} |= "x"`

				if r.URL.Path == _lokiLabelsPath {
					body = `{"data":["env"]}`
					query = `{a="b"}`
				}

				assert.Equal(t, query, r.URL.Query().Get("query"))

				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			}),
			Query: `{a="b"} |= "x"`,
			Result: &LogFieldsResult{Fields: []LogField{
				{Name: "env", Kind: LogFieldKindLabel},
				{Name: "detected_level", Kind: LogFieldKindField},
				{Name: "trace_id", Kind: LogFieldKindField},
			}},
		},
		// structured metadata appears in detected fields from Loki 3.2.
		"Fields of the corpus": {
			Query: `{env="prod"} |= "timeout"`,
			Contain: map[string][]LogField{
				"3.0.1": labels,
				"3.7.9": append(slices.Clone(labels),
					LogField{Name: "level", Kind: LogFieldKindField},
					LogField{Name: "trace_id", Kind: LogFieldKindField},
				),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewLoki(prepLokiInput(t, "", c.Handler)).LogFields(context.Background(), c.Query, _logsWindow)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				assert.Equal(t, c.Result, res)

				return
			}

			for _, v := range _lokiVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := NewLoki(prepLokiInput(t, "http://"+_lokiAddrs[v], nil)).
						LogFields(context.Background(), c.Query, _logsWindow)
					require.NoError(t, err)
					assert.Subset(t, res.Fields, c.Contain[v])
				})
			}
		})
	}
}

func Test_Loki_LogFieldValues(t *testing.T) {
	cc := map[string]struct {
		Handler  http.Handler
		Field    string
		Query    string
		Result   *LogFieldValuesResult
		Contains map[string][]string
		Err      error
	}{
		"Error returned by the label values request": {
			Handler: testutil.JSONHandler("/loki/api/v1/label/env/values", http.StatusBadRequest, `bad`),
			Field:   "env",
			Err:     NewInvalidQueryError("bad"),
		},
		"Error returned by the detected field values request": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/loki/api/v1/label/") {
					_, err := w.Write([]byte(`{"data":[]}`))
					assert.NoError(t, err)

					return
				}

				w.WriteHeader(http.StatusBadRequest)
			}),
			Field: "trace_id",
			Query: `{a="b"}`,
			Err:   assert.AnError,
		},
		"Label values": {
			Handler: testutil.JSONHandler("/loki/api/v1/label/env/values", http.StatusOK, `{"data":["prod"]}`),
			Field:   "env",
			Result:  &LogFieldValuesResult{Values: []LogFieldValue{{Value: "prod", Count: null.Int{}}}},
		},
		"No values without a query": {
			Handler: testutil.JSONHandler("/loki/api/v1/label/trace_id/values", http.StatusOK, `{"data":[]}`),
			Field:   "trace_id",
			Result:  &LogFieldValuesResult{Values: []LogFieldValue{}},
		},
		"Detected values when the field is not a label": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body := `{"values":["abc"]}`
				query := `{a="b"} | json`

				if strings.HasPrefix(r.URL.Path, "/loki/api/v1/label/") {
					body = `{"data":[]}`
					query = `{a="b"}`
				}

				assert.Equal(t, query, r.URL.Query().Get("query"))

				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			}),
			Field:  "trace_id",
			Query:  `{a="b"} | json`,
			Result: &LogFieldValuesResult{Values: []LogFieldValue{{Value: "abc", Count: null.Int{}}}},
		},
		"Loki without detected field values": {
			Handler: testutil.JSONHandler("/loki/api/v1/label/trace_id/values", http.StatusOK, `{"data":[]}`),
			Field:   "trace_id",
			Query:   `{a="b"}`,
			Result:  &LogFieldValuesResult{Values: []LogFieldValue{}},
		},
		"Values are capped": {
			Handler: testutil.JSONHandler("/loki/api/v1/label/env/values", http.StatusOK,
				`{"data":["`+strings.Repeat(`v","`, _maxLogFieldValues)+`v"]}`),
			Field: "env",
		},
		"Service names of the corpus": {
			Field: "service_name",
			Result: &LogFieldValuesResult{Values: []LogFieldValue{
				{Value: "checkout"},
				{Value: "deploy-bot"},
				{Value: "payments"},
			}},
		},
		// detected field values exist from Loki 3.3.
		"Levels of the corpus": {
			Field: "level",
			Query: `{env="prod"} |= "seq="`,
			Contains: map[string][]string{
				"3.0.1": {},
				"3.7.9": {"debug", "error", "info", "warn"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewLoki(prepLokiInput(t, "", c.Handler)).
					LogFieldValues(context.Background(), c.Field, c.Query, _logsWindow)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				if c.Result == nil {
					assert.Len(t, res.Values, _maxLogFieldValues)

					return
				}

				assert.Equal(t, c.Result, res)

				return
			}

			for _, v := range _lokiVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := NewLoki(prepLokiInput(t, "http://"+_lokiAddrs[v], nil)).
						LogFieldValues(context.Background(), c.Field, c.Query, _logsWindow)
					require.NoError(t, err)

					if c.Result != nil {
						assert.Equal(t, c.Result, res)

						return
					}

					values := make([]string, 0, len(res.Values))

					for _, fv := range res.Values {
						values = append(values, fv.Value)
					}

					assert.ElementsMatch(t, c.Contains[v], values)
				})
			}
		})
	}
}

func Test_Loki_get(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  lokiListResponse
		Err     error
	}{
		"Error returned by Loki.client": {
			Inp: &InputMock{
				CredentialsFunc: func() Credentials { return NewCredentials([]byte(`{`)) },
			},
			Err: assert.AnError,
		},
		"Error returned by Loki.request": {
			Inp: &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Err: assert.AnError,
		},
		"Unauthorized": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusUnauthorized, `no org id`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Endpoint not found": {
			Handler: testutil.JSONHandler("/elsewhere", http.StatusOK, `{}`),
			Err:     _errLokiEndpointNotFound,
		},
		"Redirect is not followed": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != _lokiLabelsPath {
					assert.Fail(t, "the redirect was followed")
				}

				http.Redirect(w, r, "/internal/admin", http.StatusFound)
			}),
			Err: _errLokiRedirected,
		},
		"Rejected request in plain text": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusGatewayTimeout, "request timed out\n"),
			Err:     NewInvalidQueryError("request timed out"),
		},
		"Rejected request wrapped by a proxy": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusTooManyRequests, `{"message":"too many requests"}`),
			Err:     NewInvalidQueryError("too many requests"),
		},
		"Rejected request in the Prometheus shape": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, `{"status":"error","error":"bad step"}`),
			Err:     NewInvalidQueryError("bad step"),
		},
		"Rejected request as JSON without a message": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, `{"status":"error"}`),
			Err:     NewInvalidQueryError(`{"status":"error"}`),
		},
		"Rejected request without a body": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, ``),
			Err:     NewInvalidQueryError("the data source rejected the query"),
		},
		"Rejected request longer than the cap": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, strings.Repeat("x", _maxErrorMessageSize+10)),
			Err:     NewInvalidQueryError(strings.Repeat("x", _maxErrorMessageSize)),
		},
		"Proxy message longer than the cap": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusBadRequest, `{"message":"`+strings.Repeat("y", _maxErrorMessageSize+10)+`"}`),
			Err:     NewInvalidQueryError(strings.Repeat("y", _maxErrorMessageSize)),
		},
		"Body that does not decode": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusOK, `{`),
			Err:     assert.AnError,
		},
		"Successful request": {
			Handler: testutil.JSONHandler(_lokiLabelsPath, http.StatusOK, `{"data":["env"]}`),
			Result:  lokiListResponse{Data: []string{"env"}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp
			if inp == nil {
				inp = prepLokiInput(t, "", c.Handler)
			} else {
				t.Cleanup(http.DefaultTransport.(*http.Transport).CloseIdleConnections)
			}

			var res lokiListResponse

			err := NewLoki(inp).get(context.Background(), _lokiLabelsPath, nil, nil, &res)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Loki_request(t *testing.T) {
	cc := map[string]struct {
		URL     string
		Handler http.Handler
		Status  int
		Body    string
		Err     error
	}{
		"Invalid URL": {
			URL: "://",
			Err: assert.AnError,
		},
		"Unreachable server": {
			URL: "http://127.0.0.1:1",
			Err: assert.AnError,
		},
		"Answer over the size cap": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, err := w.Write(bytes.Repeat([]byte("x"), _maxLokiResponseSize+1))
				assert.NoError(t, err)
			}),
			Err: _errLokiResponseTooLarge,
		},
		"Body cut short": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "10")

				_, err := w.Write([]byte("abc"))
				assert.NoError(t, err)
			}),
			Err: assert.AnError,
		},
		"Path prefix params and header reach the server": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/proxy"+_lokiLabelsPath, r.URL.Path)
				assert.Equal(t, "1", r.URL.Query().Get("start"))
				assert.Equal(t, "yes", r.Header.Get("X-Test"))
				w.WriteHeader(http.StatusTeapot)

				_, err := w.Write([]byte("body"))
				assert.NoError(t, err)
			}),
			Status: http.StatusTeapot,
			Body:   "body",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := prepLokiInput(t, c.URL, c.Handler)
			if c.Handler != nil {
				base := inp.URL()
				inp.URLFunc = func() string { return base + "/proxy" }
			}

			status, body, err := NewLoki(inp).request(
				context.Background(),
				&http.Client{},
				_lokiLabelsPath,
				map[string][]string{"start": {"1"}},
				http.Header{"X-Test": {"yes"}},
			)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, status)
			assert.Equal(t, c.Body, string(body))
		})
	}
}

func Test_Loki_client(t *testing.T) {
	cc := map[string]struct {
		URL         string
		Credentials Credentials

		// TLS indicates whether the data source is served over TLS, with
		// its own certificate as the credentials' CA.
		TLS bool

		// Auth is the Authorization header the data source receives.
		Auth string

		Err error
	}{
		"Credentials that do not decode": {
			URL:         "http://loki.test",
			Credentials: NewCredentials([]byte(`{`)),
			Err:         assert.AnError,
		},
		"Error returned by ownTransport.roundTripper": {
			URL:         "http://loki.test",
			Credentials: caCredentials("not a certificate"),
			Err:         _errInvalidCACertificate,
		},
		"No credentials": {},
		"Basic credentials": {
			Credentials: NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
			Auth:        "Basic dXNlcjpwYXNz",
		},
		// credentials holding only a CA certificate authenticate nobody.
		"Only a CA certificate": {
			TLS: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var auth atomic.Value

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				auth.Store(r.Header.Get("Authorization"))
				w.WriteHeader(http.StatusNoContent)
			})

			var inp *InputMock

			switch {
			case c.URL != "":
				inp = &InputMock{URLFunc: func() string { return c.URL }}
			case c.TLS:
				inp = prepTLSInput(t, h, "")
			default:
				inp = prepLokiInput(t, "", h)
			}

			if !c.TLS {
				inp.CredentialsFunc = func() Credentials { return c.Credentials }
			}

			l := NewLoki(inp)

			t.Cleanup(func() { assert.NoError(t, l.Close()) })

			client, err := l.client()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotNil(t, client.CheckRedirect)

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, inp.URL(), http.NoBody)
			require.NoError(t, err)

			resp, err := client.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			assert.Equal(t, c.Auth, auth.Load())
		})
	}
}

func Test_lokiEntry_UnmarshalJSON(t *testing.T) {
	cc := map[string]struct {
		JSON   string
		Result lokiEntry
		Err    error
	}{
		"Not an array":            {JSON: `{}`, Err: assert.AnError},
		"Too few parts":           {JSON: `["1"]`, Err: assert.AnError},
		"Time that is a number":   {JSON: `[1, "a"]`, Err: assert.AnError},
		"Time that is not an int": {JSON: `["soon", "a"]`, Err: assert.AnError},
		"Line that is a number":   {JSON: `["1", 2]`, Err: assert.AnError},
		"Metadata that is not an object": {
			JSON: `["1", "a", []]`,
			Err:  assert.AnError,
		},
		"Time and line only": {
			JSON:   `["1793019494670388731", "a"]`,
			Result: lokiEntry{ns: 1793019494670388731, line: "a", fields: map[string]string{}},
		},
		"Parsed keys win over structured metadata": {
			JSON: `["1", "a", {"structuredMetadata":{"k":"meta","m":"1"},"parsed":{"k":"parsed","p":"2"}}]`,
			Result: lokiEntry{ns: 1, line: "a", fields: map[string]string{
				"k": "parsed",
				"m": "1",
				"p": "2",
			}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var le lokiEntry

			err := json.Unmarshal([]byte(c.JSON), &le)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, le)
		})
	}
}

func Test_lokiEntry_logLine(t *testing.T) {
	cc := map[string]struct {
		Entry  lokiEntry
		Stream map[string]string
		Result LogLine
	}{
		"Detected level in metadata wins": {
			Entry:  lokiEntry{ns: 5, line: "a", fields: map[string]string{"detected_level": "error", "level": "info"}},
			Stream: map[string]string{"level": "debug"},
			Result: LogLine{
				Time:   time.Unix(0, 5).UTC(),
				Level:  LogLevelError,
				Line:   "a",
				Labels: map[string]string{"level": "debug"},
				Fields: map[string]string{"detected_level": "error", "level": "info"},
			},
		},
		"Level from the stream labels": {
			Entry:  lokiEntry{ns: 5, line: "a", fields: map[string]string{}},
			Stream: map[string]string{"level": "WARN"},
			Result: LogLine{
				Time:   time.Unix(0, 5).UTC(),
				Level:  LogLevelWarning,
				Line:   "a",
				Labels: map[string]string{"level": "WARN"},
				Fields: map[string]string{},
			},
		},
		"No level at all": {
			Entry:  lokiEntry{ns: 5, line: "a", fields: map[string]string{}},
			Stream: nil,
			Result: LogLine{
				Time:   time.Unix(0, 5).UTC(),
				Level:  LogLevelUnknown,
				Line:   "a",
				Labels: map[string]string{},
				Fields: map[string]string{},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Entry.logLine(c.Stream))
		})
	}
}

func Test_LokiStreamSelector(t *testing.T) {
	cc := map[string]struct {
		Query  string
		Result string
	}{
		"Selector and pipeline":        {Query: ` {a="b"} |= "x"`, Result: `{a="b"}`},
		"Brace inside a quoted value":  {Query: `{a="}"} | json`, Result: `{a="}"}`},
		"Escaped quote inside a value": {Query: `{a="\"}"}`, Result: `{a="\"}"}`},
		"Brace inside a raw string":    {Query: "{a=`}`}", Result: "{a=`}`}"},
		"Never closed":                 {Query: `{a="b"`, Result: ""},
		"Metric query":                 {Query: `rate({a="b"}[1m])`, Result: ""},
		"Empty query":                  {Query: ``, Result: ""},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, LokiStreamSelector(c.Query))
		})
	}
}

func Test_lokiWindowParams(t *testing.T) {
	tr := TimeRange{From: time.Unix(0, 1), To: time.Unix(0, 2)}

	cc := map[string]struct {
		Query  string
		Result map[string][]string
	}{
		"Without a query": {
			Result: map[string][]string{"start": {"1"}, "end": {"2"}},
		},
		"With a query": {
			Query:  `{a="b"}`,
			Result: map[string][]string{"start": {"1"}, "end": {"2"}, "query": {`{a="b"}`}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, map[string][]string(lokiWindowParams(c.Query, tr)))
		})
	}
}
