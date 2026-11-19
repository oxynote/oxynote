package processor

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/guregu/null/v5"
	"github.com/orlangure/gnomock"
	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// _victoriaLogsDefaultMsg is what VictoriaLogs stores as the message of
	// a log that has none.
	_victoriaLogsDefaultMsg = "missing _msg field; see https://docs.victoriametrics.com/victorialogs/keyconcepts/#message-field"

	// _victoriaLogsReadyInterval is how often the corpus is checked for
	// being readable after it was pushed.
	_victoriaLogsReadyInterval = 200 * time.Millisecond
)

// _victoriaLogsVersions are the VictoriaLogs image tags the integration
// tests run against: the oldest supported release and the newest.
var _victoriaLogsVersions = []string{"v1.39.0", "v1.54.0"}

// _victoriaLogsAddrs maps a VictoriaLogs version to the address of its
// container.
var _victoriaLogsAddrs = map[string]string{}

// startVictoriaLogs starts a VictoriaLogs container of the given version
// holding the corpus. It returns once every corpus line can be read.
func startVictoriaLogs(version string, corpus []corpusLine) (*gnomock.Container, error) {
	return gnomock.StartCustom(
		"docker.io/victoriametrics/victoria-logs:"+version,
		gnomock.DefaultTCP(9428),
		gnomock.WithTimeout(3*time.Minute),
		gnomock.WithHealthCheck(func(ctx context.Context, c *gnomock.Container) error {
			_, err := victoriaLogsRequest(ctx, http.MethodGet, "http://"+c.DefaultAddress()+"/health", nil)

			return err
		}),
		gnomock.WithInit(func(ctx context.Context, c *gnomock.Container) error {
			return pushVictoriaLogsCorpus(ctx, c.DefaultAddress(), corpus)
		}),
	)
}

// pushVictoriaLogsCorpus pushes the corpus into the VictoriaLogs at addr,
// one stream per service, the level, trace id and sequence number as
// plain fields. It returns once a count of every log matches the corpus.
func pushVictoriaLogsCorpus(ctx context.Context, addr string, corpus []corpusLine) error {
	var body bytes.Buffer

	enc := json.NewEncoder(&body)

	for _, l := range corpus {
		row := map[string]string{
			"time":         l.Time.Format(time.RFC3339Nano),
			"msg":          l.Message,
			"service_name": l.Service,
			"env":          "prod",
			"level":        l.Level,
			"trace_id":     l.TraceID,
			"seq":          strconv.Itoa(l.Seq),
		}

		if err := enc.Encode(row); err != nil {
			return err
		}
	}

	target := "http://" + addr + "/insert/jsonline?_stream_fields=service_name,env&_time_field=time&_msg_field=msg"
	if _, err := victoriaLogsRequest(ctx, http.MethodPost, target, body.Bytes()); err != nil {
		return err
	}

	params := url.Values{
		"query": {"* | stats count() n"},
		"start": {_logsWindow.From.Add(-time.Hour).Format(time.RFC3339Nano)},
		"end":   {_logsWindow.To.Add(time.Hour).Format(time.RFC3339Nano)},
	}
	want := fmt.Sprintf(`{"n":"%d"}`, len(corpus))

	// the pushed logs become readable once VictoriaLogs flushes them,
	// which takes about a second.
	return backoff.Retry(func() error {
		res, err := victoriaLogsRequest(ctx, http.MethodGet, "http://"+addr+_victoriaLogsQueryPath+"?"+params.Encode(), nil)
		if err != nil {
			return backoff.Permanent(err)
		}

		if got := strings.TrimSpace(string(res)); got != want {
			return fmt.Errorf("victorialogs counts %s, want %s", got, want)
		}

		return nil
	}, backoff.WithContext(backoff.NewConstantBackOff(_victoriaLogsReadyInterval), ctx))
}

// victoriaLogsRequest sends a request to a test VictoriaLogs and returns
// the body. It fails unless VictoriaLogs answers 200.
func victoriaLogsRequest(ctx context.Context, method, target string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// a form content type would make VictoriaLogs read the logs as form
	// values.
	req.Header.Set("Content-Type", "application/stream+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	var res bytes.Buffer

	if _, err := res.ReadFrom(resp.Body); err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("victorialogs answered %d: %s", resp.StatusCode, res.String())
	}

	return res.Bytes(), nil
}

// victoriaLogsCorpusWhere returns the corpus lines in the window that
// match, as VictoriaLogs returns them: an empty message is the default
// one.
func victoriaLogsCorpusWhere(tr TimeRange, match func(corpusLine) bool) map[string]corpusLine {
	lines := corpusWhere(tr, match)

	for seq, l := range lines {
		if l.Message == "" {
			l.Message = _victoriaLogsDefaultMsg
			lines[seq] = l
		}
	}

	return lines
}

// prepVictoriaLogs returns a processor reading the container of the given
// version.
func prepVictoriaLogs(t *testing.T, version string) *VictoriaLogs {
	t.Helper()

	return NewVictoriaLogs(prepHTTPInput(t, "http://"+_victoriaLogsAddrs[version], nil))
}

func Test_NewVictoriaLogs(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	vl := NewVictoriaLogs(inp)
	require.NotNil(t, vl)
	assert.Same(t, inp, vl.inp)
}

func Test_VictoriaLogs_Close(t *testing.T) {
	t.Parallel()

	assert.NoError(t, NewVictoriaLogs(&InputMock{}).Close())
}

func Test_VictoriaLogs_TestConnection(t *testing.T) {
	values := `{"values":[{"value":"_msg","hits":1}]}`

	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Version string

		// TLS indicates whether the handler is served over TLS, with the
		// server's own certificate as the credentials' CA.
		TLS bool

		// Creds is the credentials JSON the data source is given, in place
		// of a username and password.
		Creds string

		Result ConnectionStatus
		Err    error
	}{
		"Error returned by httpClient.authClient": {
			Inp: &InputMock{
				CredentialsFunc: func() credentials.Credentials { return stubCredentials(`{`) },
			},
			Err: assert.AnError,
		},
		"Invalid CA certificate": {
			Inp: &InputMock{
				URLFunc:         func() string { return "https://victorialogs.test" },
				CredentialsFunc: func() credentials.Credentials { return caCredentials("not a certificate") },
			},
			Result: ConnectionStatusInvalidCACertificate,
		},
		"Unreachable data source": {
			Inp:    &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Result: ConnectionStatusUnreachable,
		},
		"Successful connection verified with a CA certificate": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusOK, values),
			TLS:     true,
			Result:  ConnectionStatusSuccess,
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusUnauthorized, `Unauthorized`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Forbidden data source": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusForbidden, `{}`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Redirecting data source": {
			Handler: http.RedirectHandler("http://example.com/", http.StatusMovedPermanently),
			Result:  ConnectionStatusUnreachable,
		},
		"Rejected token": { //nolint:gosec // static test credentials
			Handler: tokenHandler(testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusOK, values)),
			Creds:   `{"kind":"token","token":"bad"}`,
			Result:  ConnectionStatusUnauthorized,
		},
		"Successful connection with a token": {
			Handler: tokenHandler(testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusOK, values)),
			Creds:   `{"kind":"token","token":"` + _testToken + `"}`,
			Result:  ConnectionStatusSuccess,
		},
		"Successful connection with credentials": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if u, p, ok := r.BasicAuth(); !ok || u != "user" || p != "pass" {
					w.WriteHeader(http.StatusUnauthorized)

					return
				}

				assert.Equal(t, _victoriaLogsMatchAll, r.URL.Query().Get("query"))

				_, err := w.Write([]byte(`{"values":[]}`))
				assert.NoError(t, err)
			}),
			Result: ConnectionStatusSuccess,
		},
		"Server that has no such endpoint": {
			Handler: testutil.JSONHandler("/elsewhere", http.StatusOK, values),
			Result:  ConnectionStatusUnreachable,
		},
		"Answer that is not JSON": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusOK, `<html>`),
			Result:  ConnectionStatusUnreachable,
		},
		"Answer without values": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldNamesPath, http.StatusOK, `{"status":"success"}`),
			Result:  ConnectionStatusUnreachable,
		},
		"Successful connection to the oldest supported VictoriaLogs": {
			Version: _victoriaLogsVersions[0],
			Result:  ConnectionStatusSuccess,
		},
		"Successful connection to the newest VictoriaLogs": {
			Version: _victoriaLogsVersions[1],
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
				inp = prepTLSInput(t, c.Handler, "")
			default:
				target := ""
				if c.Version != "" {
					target = "http://" + _victoriaLogsAddrs[c.Version]
				}

				creds := cmp.Or(c.Creds, `{"kind":"basic","username":"user","password":"pass"}`)

				inp = prepHTTPInput(t, target, c.Handler)
				inp.CredentialsFunc = func() credentials.Credentials { return stubCredentials(creds) }
			}

			vl := NewVictoriaLogs(inp)

			t.Cleanup(func() { assert.NoError(t, vl.Close()) })

			cs, err := vl.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_VictoriaLogs_Logs(t *testing.T) {
	lastQuarter := TimeRange{
		From: _logsWindow.To.Add(-15 * time.Minute),
		To:   _logsWindow.To,
	}

	rows := `{"_time":"1970-01-01T00:00:00.000002Z","_stream":"{service_name=\"b\"}","_msg":"second","service_name":"b","severity":"WARN"}` + "\n" +
		`{"_time":"1970-01-01T00:00:00.000003Z","_stream":"{service_name=\"a\"}","_stream_id":"x","_msg":"third","service_name":"a"}` + "\n" +
		`{"_time":"1970-01-01T00:00:00.000001Z","_stream":"{service_name=\"a\"}","_msg":"first","service_name":"a","level":"error"}` + "\n"

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
		"Error returned by httpClient.fetchRaw": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusBadRequest,
				`cannot parse query [foo |]: unexpected pipe ""`),
			Range: _logsWindow,
			Page:  LogsPage{Limit: 10},
			Err:   NewInvalidQueryError(`cannot parse query [foo |]: unexpected pipe ""`),
		},
		"Rows come back newest first": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK, rows),
			Range:   TimeRange{From: time.Unix(0, 0), To: time.Unix(0, 4000)},
			Page:    LogsPage{Limit: 10},
			Result: &LogsResult{
				Status: QueryStatusOK,
				Lines: []LogLine{
					{Time: time.Unix(0, 3000).UTC(), Level: LogLevelUnknown, Line: "third", Labels: map[string]string{"service_name": "a"}, Fields: map[string]string{}},
					{Time: time.Unix(0, 2000).UTC(), Level: LogLevelWarning, Line: "second", Labels: map[string]string{"service_name": "b"}, Fields: map[string]string{"severity": "WARN"}},
					{Time: time.Unix(0, 1000).UTC(), Level: LogLevelError, Line: "first", Labels: map[string]string{"service_name": "a"}, Fields: map[string]string{"level": "error"}},
				},
				PreviousCursor: null.ValueFrom(newCursor(time.Unix(0, 3001), time.Nanosecond)),
			},
		},
		"Row without a time": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK, `{"n":"3"}`+"\n"),
			Query:   "* | stats count() n",
			Range:   _logsWindow,
			Page:    LogsPage{Limit: 10},
			Err:     _errVictoriaLogsNoTime,
		},
		"Stream filter": {
			Query:     `_stream:{service_name="checkout"}`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return l.Service == "checkout" },
			WalkLimit: _maxLogsLimit,
		},
		"Level filter": {
			Query:     `level:="error"`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return l.Level == "error" },
			WalkLimit: _maxLogsLimit,
		},
		"Text match": {
			Query:     `timeout`,
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return strings.Contains(l.Message, "timeout") },
			WalkLimit: _maxLogsLimit,
		},
		"Last fifteen minutes": {
			Range:     lastQuarter,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: _maxLogsLimit,
		},
		"Relative time filter": {
			Query:     "_time:15m",
			Range:     _logsWindow,
			Match:     func(l corpusLine) bool { return !l.Time.Before(lastQuarter.From) },
			WalkLimit: 100,
		},
		"First page of a hundred": {
			Range: _logsWindow,
			Page:  LogsPage{Limit: 100},
			Match: func(corpusLine) bool { return true },
		},
		"Walk at limit 7": {
			Range:     _logsWindow,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 7,
		},
		"Walk at limit 100": {
			Range:     _logsWindow,
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 100,
		},
		"First page of a hundred asc": {
			Range: _logsWindow,
			Page:  LogsPage{Limit: 100, Order: LogsOrderAsc},
			Match: func(corpusLine) bool { return true },
		},
		"Asc walk at limit 7": {
			Range:     _logsWindow,
			Page:      LogsPage{Order: LogsOrderAsc},
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 7,
		},
		"Asc walk at limit 100": {
			Range:     _logsWindow,
			Page:      LogsPage{Order: LogsOrderAsc},
			Match:     func(corpusLine) bool { return true },
			WalkLimit: 100,
		},
		"Asc walk with a stream filter": {
			Query:     `_stream:{service_name="checkout"}`,
			Range:     _logsWindow,
			Page:      LogsPage{Order: LogsOrderAsc},
			Match:     func(l corpusLine) bool { return l.Service == "checkout" },
			WalkLimit: _maxLogsLimit,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewVictoriaLogs(prepHTTPInput(t, "", c.Handler)).Logs(context.Background(), c.Query, c.Range, c.Page)
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

			for _, v := range _victoriaLogsVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					vl := prepVictoriaLogs(t, v)
					want := victoriaLogsCorpusWhere(c.Range, c.Match)

					if c.WalkLimit == 0 {
						res, err := vl.Logs(context.Background(), c.Query, c.Range, c.Page)
						require.NoError(t, err)
						require.NotEmpty(t, res.Lines)

						asc := c.Page.Order == LogsOrderAsc
						last := res.Lines[len(res.Lines)-1].Time
						assertCorpusLines(t, c.Page.Order, victoriaLogsCorpusWhere(c.Range, func(cl corpusLine) bool {
							if asc {
								return !cl.Time.After(last)
							}

							return !cl.Time.Before(last)
						}), res.Lines)
						assert.True(t, res.NextCursor.Valid)

						return
					}

					page := LogsPage{Limit: c.WalkLimit, Order: c.Page.Order}
					lines, last := walkLogs(t, vl.fetchLines(c.Query, c.Range.To), c.Range, page, time.Nanosecond)
					assertCorpusLines(t, c.Page.Order, want, lines)

					back := walkLogsBack(t, vl.fetchLines(c.Query, c.Range.To), c.Range, c.Page.Order, c.WalkLimit, last, time.Nanosecond)
					assertCorpusLines(t, oppositeLogsOrder(c.Page.Order), want, back)
				})
			}
		})
	}
}

func Test_VictoriaLogs_LogsVolume(t *testing.T) {
	// a range from 60 to 80 seconds counts in one-second buckets.
	window := TimeRange{From: time.Unix(60, 0), To: time.Unix(80, 0)}

	// seconds lists the buckets of window, with the given counts and zero
	// elsewhere.
	seconds := func(counts map[int64]float64) [][2]any {
		points := make([][2]any, 0, 20)

		for sec := int64(60); sec < 80; sec++ {
			points = append(points, [2]any{sec, counts[sec]})
		}

		return points
	}

	cc := map[string]struct {
		Handler http.Handler
		Range   TimeRange
		Result  *QueryResult
		Err     error
	}{
		"Error returned by httpClient.fetchJSON": {
			Handler: testutil.JSONHandler(_victoriaLogsHitsPath, http.StatusOK, `{"hits":{}}`),
			Range:   _logsWindow,
			Err:     assert.AnError,
		},
		"No logs": {
			Handler: testutil.JSONHandler(_victoriaLogsHitsPath, http.StatusOK, `{"hits":[]}`),
			Range:   window,
			Result: &QueryResult{
				Status: QueryStatusNoData,
				Data: []QueryResultSeries{
					{Labels: map[string]string{}, Metrics: seconds(nil)},
				},
			},
		},
		"Groups merge by their first level field": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				params := r.URL.Query()
				assert.Equal(t, _victoriaLogsLevelFields, params["field"])
				assert.Equal(t, "1s", params.Get("step"))
				assert.Equal(t, "1970-01-01T00:01:00Z", params.Get("start"))
				assert.Equal(t, "1970-01-01T00:01:20Z", params.Get("end"))
				assert.Equal(t, "1970-01-01T00:01:20Z", params.Get("time"))

				_, err := w.Write([]byte(`{"hits":[` +
					`{"fields":{"level":"","severity":"WARN"},"timestamps":["1970-01-01T00:01:06Z"],"values":[2]},` +
					`{"fields":{"level":"warning","severity":"ERROR"},"timestamps":["1970-01-01T00:01:06Z","1970-01-01T00:01:10Z"],"values":[3,1]},` +
					`{"fields":{"level":""},"timestamps":["1970-01-01T00:01:10Z","1970-01-01T00:01:11Z"],"values":[4]}]}`))
				assert.NoError(t, err)
			}),
			Range: window,
			Result: &QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{Labels: map[string]string{"level": "unknown"}, Metrics: seconds(map[int64]float64{70: 4})},
					{Labels: map[string]string{"level": "warning"}, Metrics: seconds(map[int64]float64{66: 5, 70: 1})},
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
				res, err := NewVictoriaLogs(prepHTTPInput(t, "", c.Handler)).LogsVolume(context.Background(), "", c.Range)
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

			for _, v := range _victoriaLogsVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := prepVictoriaLogs(t, v).LogsVolume(context.Background(), "", c.Range)
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

func Test_VictoriaLogs_LogFields(t *testing.T) {
	cc := map[string]struct {
		Handler http.Handler
		Query   string
		Result  *LogFieldsResult
		Err     error
	}{
		"Error returned by httpClient.fetchJSON for the stream field names": {
			Handler: testutil.JSONHandler(_victoriaLogsStreamFieldNamesPath, http.StatusBadRequest, `bad`),
			Err:     NewInvalidQueryError("bad"),
		},
		"Error returned by httpClient.fetchJSON for the field names": {
			Handler: testutil.JSONHandler(_victoriaLogsStreamFieldNamesPath, http.StatusOK, `{"values":[]}`),
			Err:     assert.AnError,
		},
		"Stream fields then other fields": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "error", r.URL.Query().Get("query"))

				body := `{"values":[{"value":"_msg","hits":3},{"value":"_stream","hits":3},{"value":"_stream_id","hits":3},` +
					`{"value":"_time","hits":3},{"value":"trace_id","hits":2},{"value":"service","hits":3},{"value":"env","hits":3},{"value":"level","hits":1}]}`
				if r.URL.Path == _victoriaLogsStreamFieldNamesPath {
					body = `{"values":[{"value":"service","hits":3},{"value":"env","hits":3}]}`
				}

				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			}),
			Query: "error",
			Result: &LogFieldsResult{Fields: []LogField{
				{Name: "env", Kind: LogFieldKindLabel},
				{Name: "service", Kind: LogFieldKindLabel},
				{Name: "level", Kind: LogFieldKindField},
				{Name: "trace_id", Kind: LogFieldKindField},
			}},
		},
		"Fields of the corpus": {
			Query: "timeout",
			Result: &LogFieldsResult{Fields: []LogField{
				{Name: "env", Kind: LogFieldKindLabel},
				{Name: "service_name", Kind: LogFieldKindLabel},
				{Name: "level", Kind: LogFieldKindField},
				{Name: "seq", Kind: LogFieldKindField},
				{Name: "trace_id", Kind: LogFieldKindField},
			}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewVictoriaLogs(prepHTTPInput(t, "", c.Handler)).LogFields(context.Background(), c.Query, _logsWindow)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				assert.Equal(t, c.Result, res)

				return
			}

			for _, v := range _victoriaLogsVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := prepVictoriaLogs(t, v).LogFields(context.Background(), c.Query, _logsWindow)
					require.NoError(t, err)
					assert.Equal(t, c.Result, res)
				})
			}
		})
	}
}

func Test_VictoriaLogs_LogFieldValues(t *testing.T) {
	counted := func(match func(corpusLine) bool, value func(corpusLine) string) map[string]int64 {
		out := map[string]int64{}

		for _, l := range corpusWhere(_logsWindow, match) {
			out[value(l)]++
		}

		return out
	}

	cc := map[string]struct {
		Handler http.Handler
		Field   string
		Query   string
		Result  *LogFieldValuesResult
		Len     int
		Counts  map[string]int64
		Err     error
	}{
		"Error returned by httpClient.fetchJSON": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldValuesPath, http.StatusBadRequest, `bad`),
			Field:   "env",
			Err:     NewInvalidQueryError("bad"),
		},
		"Counted values": {
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				params := r.URL.Query()
				assert.Equal(t, "env", params.Get("field"))
				assert.Equal(t, "1000", params.Get("limit"))
				assert.Equal(t, _victoriaLogsMatchAll, params.Get("query"))

				_, err := w.Write([]byte(`{"values":[{"value":"prod","hits":5},{"value":"dev","hits":2}]}`))
				assert.NoError(t, err)
			}),
			Field: "env",
			Result: &LogFieldValuesResult{Values: []LogFieldValue{
				{Value: "prod", Count: null.IntFrom(5)},
				{Value: "dev", Count: null.IntFrom(2)},
			}},
		},
		"Logs without the field are left out": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldValuesPath, http.StatusOK, `{"values":[{"value":"","hits":7},{"value":"prod","hits":5}]}`),
			Field:   "env",
			Result:  &LogFieldValuesResult{Values: []LogFieldValue{{Value: "prod", Count: null.IntFrom(5)}}},
		},
		"Values of a cut listing are not counted": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldValuesPath, http.StatusOK, `{"values":[{"value":"prod","hits":0}]}`),
			Field:   "env",
			Result:  &LogFieldValuesResult{Values: []LogFieldValue{{Value: "prod", Count: null.Int{}}}},
		},
		"Values are capped": {
			Handler: testutil.JSONHandler(_victoriaLogsFieldValuesPath, http.StatusOK,
				`{"values":[`+strings.Repeat(`{"value":"v","hits":1},`, _maxLogFieldValues)+`{"value":"v","hits":1}]}`),
			Field: "env",
			Len:   _maxLogFieldValues,
		},
		"Service names of the corpus": {
			Field:  "service_name",
			Counts: counted(func(corpusLine) bool { return true }, func(l corpusLine) string { return l.Service }),
		},
		"Levels of the corpus": {
			Field: "level",
			Query: "timeout",
			Counts: counted(
				func(l corpusLine) bool { return strings.Contains(l.Message, "timeout") },
				func(l corpusLine) string { return l.Level },
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Handler != nil {
				res, err := NewVictoriaLogs(prepHTTPInput(t, "", c.Handler)).
					LogFieldValues(context.Background(), c.Field, c.Query, _logsWindow)
				testutil.AssertEqualError(t, c.Err, err)

				if err != nil {
					return
				}

				if c.Len > 0 {
					assert.Len(t, res.Values, c.Len)

					return
				}

				assert.Equal(t, c.Result, res)

				return
			}

			for _, v := range _victoriaLogsVersions {
				t.Run(v, func(t *testing.T) {
					t.Parallel()

					res, err := prepVictoriaLogs(t, v).LogFieldValues(context.Background(), c.Field, c.Query, _logsWindow)
					require.NoError(t, err)

					got := map[string]int64{}

					for _, val := range res.Values {
						got[val.Value] = val.Count.Int64
					}

					assert.Equal(t, c.Counts, got)
				})
			}
		})
	}
}

func Test_VictoriaLogs_fetchLines(t *testing.T) {
	tiny := TimeRange{From: time.Unix(0, 0), To: time.Unix(0, 10)}

	cc := map[string]struct {
		Handler http.Handler
		Version string
		Query   string
		Range   TimeRange
		Limit   int
		Order   LogsOrder
		Params  url.Values
		Count   int
		Result  []LogLine
		Err     error
	}{
		"Error returned by httpClient.fetchRaw": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusBadRequest, `bad`),
			Range:   tiny,
			Limit:   1,
			Err:     NewInvalidQueryError("bad"),
		},
		"Row that does not decode": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK, `{"_msg":1}`),
			Range:   tiny,
			Limit:   1,
			Err:     assert.AnError,
		},
		"Row whose line does not parse": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK, `{"_time":"yesterday"}`),
			Range:   tiny,
			Limit:   1,
			Err:     NewInvalidQueryError(`the query returned a _time that is not a time: "yesterday"`),
		},
		"Desc read asks for the newest logs with the limit arg": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK,
				"\n"+`{"_time":"1970-01-01T00:00:00.000000001Z","_msg":"a"}`+"\n\n"+`{"_time":"1970-01-01T00:00:00.000000002Z","_msg":"b"}`+"\n"),
			Query: "error",
			Range: tiny,
			Limit: 2,
			Order: LogsOrderDesc,
			Params: url.Values{
				"query": {"error"},
				"start": {"1970-01-01T00:00:00Z"},
				"end":   {"1970-01-01T00:00:00.00000001Z"},
				"time":  {"1970-01-01T00:00:00.00000001Z"},
				"limit": {"2"},
			},
			Result: []LogLine{
				{Time: time.Unix(0, 2).UTC(), Level: LogLevelUnknown, Line: "b", Labels: map[string]string{}, Fields: map[string]string{}},
				{Time: time.Unix(0, 1).UTC(), Level: LogLevelUnknown, Line: "a", Labels: map[string]string{}, Fields: map[string]string{}},
			},
		},
		"Asc read sorts with a pipe on its own line": {
			Handler: testutil.JSONHandler(_victoriaLogsQueryPath, http.StatusOK,
				`{"_time":"1970-01-01T00:00:00.000000002Z","_msg":"b"}`+"\n"+`{"_time":"1970-01-01T00:00:00.000000001Z","_msg":"a"}`),
			Range: tiny,
			Limit: 2,
			Order: LogsOrderAsc,
			Params: url.Values{
				"query": {"*\n| sort by (_time asc) limit 2"},
				"start": {"1970-01-01T00:00:00Z"},
				"end":   {"1970-01-01T00:00:00.00000001Z"},
				"time":  {"1970-01-01T00:00:00.00000001Z"},
			},
			Result: []LogLine{
				{Time: time.Unix(0, 1).UTC(), Level: LogLevelUnknown, Line: "a", Labels: map[string]string{}, Fields: map[string]string{}},
				{Time: time.Unix(0, 2).UTC(), Level: LogLevelUnknown, Line: "b", Labels: map[string]string{}, Fields: map[string]string{}},
			},
		},
		"Stats rows of the corpus have no time": {
			Version: _victoriaLogsVersions[0],
			Query:   "* | stats count() n",
			Range:   _logsWindow,
			Limit:   10,
			Err:     _errVictoriaLogsNoTime,
		},
		"Comment at the end of an asc query on the oldest VictoriaLogs": {
			Version: _victoriaLogsVersions[0],
			Query:   "timeout # slow calls",
			Range:   _logsWindow,
			Limit:   1,
			Order:   LogsOrderAsc,
			Count:   1,
		},
		"Comment at the end of an asc query on the newest VictoriaLogs": {
			Version: _victoriaLogsVersions[1],
			Query:   "timeout # slow calls",
			Range:   _logsWindow,
			Limit:   1,
			Order:   LogsOrderAsc,
			Count:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var params url.Values

			target := ""
			if c.Version != "" {
				target = "http://" + _victoriaLogsAddrs[c.Version]
			}

			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				params = r.URL.Query()
				c.Handler.ServeHTTP(w, r)
			})

			lines, err := NewVictoriaLogs(prepHTTPInput(t, target, h)).
				fetchLines(c.Query, c.Range.To)(context.Background(), c.Range.From, c.Range.To, c.Limit, c.Order)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Version != "" {
				assert.Len(t, lines, c.Count)

				return
			}

			assert.Equal(t, c.Params, params)
			assert.Equal(t, c.Result, lines)
		})
	}
}

func Test_victoriaLogsListResponse_names(t *testing.T) {
	t.Parallel()

	vlr := victoriaLogsListResponse{Values: []victoriaLogsValue{{Value: "b", Hits: 1}, {Value: "a"}}}

	assert.Equal(t, []string{"b", "a"}, vlr.names())
	assert.Equal(t, []string{}, victoriaLogsListResponse{}.names())
}

func Test_victoriaLogsRow_logLine(t *testing.T) {
	cc := map[string]struct {
		Row    victoriaLogsRow
		Result LogLine
		Err    error
	}{
		"Time that does not parse": {
			Row: victoriaLogsRow{"_time": "yesterday"},
			Err: NewInvalidQueryError(`the query returned a _time that is not a time: "yesterday"`),
		},
		"Stream that does not parse": {
			Row: victoriaLogsRow{"_time": "1970-01-01T00:00:00Z", "_stream": "service=a"},
			Err: NewInvalidQueryError(`victorialogs stream "service=a" does not start with {`),
		},
		"Row without a time": {
			Row: victoriaLogsRow{"n": "3"},
			Err: _errVictoriaLogsNoTime,
		},
		"Stream fields are labels and the rest are fields": {
			Row: victoriaLogsRow{
				"_time":      "2026-11-18T10:00:00.000000001+02:00",
				"_msg":       _victoriaLogsDefaultMsg,
				"_stream":    `{service="a",env="prod"}`,
				"_stream_id": "x",
				"service":    "a",
				"env":        "prod",
				"lvl":        "debug",
				"trace_id":   "t",
			},
			Result: LogLine{
				Time:   time.Date(2026, 11, 18, 8, 0, 0, 1, time.UTC),
				Level:  LogLevelDebug,
				Line:   _victoriaLogsDefaultMsg,
				Labels: map[string]string{"service": "a", "env": "prod"},
				Fields: map[string]string{"lvl": "debug", "trace_id": "t"},
			},
		},
		"Level field order holds across stream fields and fields": {
			Row: victoriaLogsRow{
				"_time":   "1970-01-01T00:00:00Z",
				"_stream": `{severity="INFO"}`,
				"level":   "error",
			},
			Result: LogLine{
				Time:   time.Unix(0, 0).UTC(),
				Level:  LogLevelError,
				Labels: map[string]string{"severity": "INFO"},
				Fields: map[string]string{"level": "error"},
			},
		},
		"OpenTelemetry severity number": {
			Row: victoriaLogsRow{"_time": "1970-01-01T00:00:00Z", "severity": "17"},
			Result: LogLine{
				Time:   time.Unix(0, 0).UTC(),
				Level:  LogLevelError,
				Labels: map[string]string{},
				Fields: map[string]string{"severity": "17"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := c.Row.logLine()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_parseVictoriaLogsStream(t *testing.T) {
	cc := map[string]struct {
		Raw    string
		Result map[string]string
		Err    error
	}{
		"Empty":                  {Raw: "", Result: map[string]string{}},
		"No fields":              {Raw: "{}", Result: map[string]string{}},
		"Missing brace":          {Raw: `a="b"}`, Err: assert.AnError},
		"Field without a name":   {Raw: `{="b"}`, Err: assert.AnError},
		"Field without a value":  {Raw: `{a}`, Err: assert.AnError},
		"Value that is unquoted": {Raw: `{a=b}`, Err: assert.AnError},
		"Unclosed stream":        {Raw: `{a="b"`, Err: assert.AnError},
		"Text after a value":     {Raw: `{a="b"c}`, Err: assert.AnError},
		"Fields with dots quotes commas and braces": {
			Raw: `{k8s.pod="a.b \"q\", x",env="}{",empty=""}`,
			Result: map[string]string{
				"k8s.pod": `a.b "q", x`,
				"env":     "}{",
				"empty":   "",
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := parseVictoriaLogsStream(c.Raw)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_parseVictoriaLogsLevel(t *testing.T) {
	cc := map[string]struct {
		Labels map[string]string
		Fields map[string]string
		Result string
	}{
		"None":                   {Fields: map[string]string{"msg": "x"}},
		"Empty level is skipped": {Fields: map[string]string{"level": "", "severity": "WARN"}, Result: "WARN"},
		"Order is level detected_level lvl loglevel severity": {
			Fields: map[string]string{"severity": "e", "loglevel": "d", "lvl": "c", "detected_level": "b"},
			Result: "b",
		},
		"Level comes first": {
			Fields: map[string]string{"level": "a", "detected_level": "b"},
			Result: "a",
		},
		"Loglevel before severity": {
			Fields: map[string]string{"loglevel": "d", "severity": "e"},
			Result: "d",
		},
		"Lvl before loglevel": {
			Fields: map[string]string{"lvl": "c", "loglevel": "d"},
			Result: "c",
		},
		"Order holds across maps": {
			Labels: map[string]string{"severity": "e"},
			Fields: map[string]string{"level": "a"},
			Result: "a",
		},
		"Earlier map wins for the same field": {
			Labels: map[string]string{"level": "a"},
			Fields: map[string]string{"level": "b"},
			Result: "a",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, parseVictoriaLogsLevel(c.Labels, c.Fields))
		})
	}
}

func Test_isVictoriaLogsInternal(t *testing.T) {
	cc := map[string]struct {
		Name   string
		Result bool
	}{
		"Time":      {Name: "_time", Result: true},
		"Message":   {Name: "_msg", Result: true},
		"Stream":    {Name: "_stream", Result: true},
		"Stream ID": {Name: "_stream_id", Result: true},
		"Other":     {Name: "_other"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isVictoriaLogsInternal(c.Name))
		})
	}
}

func Test_createVictoriaLogsWindowParams(t *testing.T) {
	tr := TimeRange{
		From: time.Date(2026, 11, 18, 12, 0, 0, 0, time.FixedZone("x", 3600)),
		To:   time.Date(2026, 11, 18, 11, 0, 0, 5, time.UTC),
	}

	cc := map[string]struct {
		Query  string
		Result url.Values
	}{
		"Empty query matches everything": {
			Result: url.Values{
				"query": {"*"},
				"start": {"2026-11-18T11:00:00Z"},
				"end":   {"2026-11-18T11:00:00.000000005Z"},
			},
		},
		"Query": {
			Query: "error",
			Result: url.Values{
				"query": {"error"},
				"start": {"2026-11-18T11:00:00Z"},
				"end":   {"2026-11-18T11:00:00.000000005Z"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, createVictoriaLogsWindowParams(c.Query, tr))
		})
	}
}
