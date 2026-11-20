package processor

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/credentials"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prepPrometheusInput returns an input pointing at the given URL, or at a
// fake Prometheus API server serving h when the URL is empty. The cleanup
// drops the Prometheus client's shared keep-alive connections so goleak
// stays clean.
func prepPrometheusInput(t *testing.T, target string, h http.Handler) *InputMock {
	t.Helper()

	if target == "" {
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)

		target = srv.URL
	}

	t.Cleanup(api.DefaultRoundTripper.(*http.Transport).CloseIdleConnections)

	return &InputMock{
		URLFunc: func() string { return target },
	}
}

func Test_prometheusAPI_Close(t *testing.T) {
	t.Parallel()

	assert.NoError(t, (&prometheusAPI{inp: &InputMock{}}).Close())
}

func Test_prometheusAPI_TestConnection(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler

		// Version names the VictoriaMetrics container to test instead.
		Version string

		// TLS indicates whether the handler is served over TLS, with
		// CACertificate as the credentials' CA, or the server's own
		// certificate when it is empty.
		TLS           bool
		CACertificate string

		// URLPath is appended to the test server's URL. CloseServer stops
		// the server before the connection test runs, so nothing listens
		// on the address URLPath points at.
		URLPath     string
		CloseServer bool

		// Creds is the credentials JSON the data source is given.
		Creds string

		Result ConnectionStatus
		Err    error
	}{
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Invalid CA certificate": {
			Inp: &InputMock{
				URLFunc: func() string { return "https://prometheus.test" },
				CredentialsFunc: func() credentials.Credentials {
					return stubCredentials(`{"kind":"basic","caCertificate":"not a certificate"}`)
				},
			},
			Result: ConnectionStatusInvalidCACertificate,
		},
		"Server the CA certificate did not sign": {
			Handler:       testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			TLS:           true,
			CACertificate: _testCA,
			Result:        ConnectionStatusUnreachable,
		},
		"Successful connection verified with a CA certificate": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			TLS:     true,
			Result:  ConnectionStatusSuccess,
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusUnauthorized, `{}`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Forbidden data source": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusForbidden, `{}`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Rejected token": { //nolint:gosec // static test credentials
			Handler: tokenHandler(testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`)),
			Creds:   `{"kind":"token","token":"bad"}`,
			Result:  ConnectionStatusUnauthorized,
		},
		"Successful connection with a token": {
			Handler: tokenHandler(testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`)),
			Creds:   `{"kind":"token","token":"` + _testToken + `"}`,
			Result:  ConnectionStatusSuccess,
		},
		"Unreachable data source": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
			Result:  ConnectionStatusUnreachable,
		},
		"Unreachable data source with a bad gateway response": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusBadGateway, `{}`),
			Result:  ConnectionStatusUnreachable,
		},
		"Unreachable data source with a status code in its address": {
			URLPath:     "/prom401",
			CloseServer: true,
			Handler:     testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Result:      ConnectionStatusUnreachable,
		},
		"Unsupported version": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":""}}`),
			Result:  ConnectionStatusVersionNotSupported,
		},
		"Successful connection through a cluster path": {
			Handler: testutil.JSONHandler("/select/0/prometheus/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.24.0"}}`),
			URLPath: "/select/0/prometheus",
			Result:  ConnectionStatusSuccess,
		},
		"Oldest VictoriaMetrics": {
			Version: _victoriaMetricsVersions[0],
			Result:  ConnectionStatusSuccess,
		},
		"Newest VictoriaMetrics": {
			Version: _victoriaMetricsVersions[1],
			Result:  ConnectionStatusSuccess,
		},
		"Successful connection": {
			Handler: testutil.JSONHandler("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Result:  ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp

			switch {
			case inp != nil:
			case c.Version != "":
				inp = prepVictoriaMetricsInput(t, c.Version)
			case c.CloseServer:
				srv := httptest.NewServer(c.Handler)
				inp = &InputMock{URLFunc: func() string { return srv.URL + c.URLPath }}

				srv.Close()
			case c.TLS:
				inp = prepTLSInput(t, c.Handler, c.CACertificate)
			default:
				inp = prepPrometheusInput(t, "", c.Handler)

				base := inp.URLFunc()
				inp.URLFunc = func() string { return base + c.URLPath }
			}

			if c.Creds != "" {
				inp.CredentialsFunc = func() credentials.Credentials { return stubCredentials(c.Creds) }
			}

			pa := &prometheusAPI{inp: inp}

			t.Cleanup(func() { assert.NoError(t, pa.Close()) })

			cs, err := pa.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_prometheusAPI_Metadata(t *testing.T) {
	// noMetadata serves no metadata, and the metric names next to it.
	noMetadata := func(namesJSON string) http.Handler {
		mux := http.NewServeMux()
		mux.Handle("/api/v1/metadata", testutil.JSONHandler("/api/v1/metadata", http.StatusOK, `{"status":"success","data":{}}`))

		if namesJSON != "" {
			mux.Handle("/api/v1/label/__name__/values", testutil.JSONHandler("/api/v1/label/__name__/values", http.StatusOK, namesJSON))
		}

		return mux
	}

	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler

		// Version names the VictoriaMetrics container to read instead.
		Version string

		Result *PrometheusMetadataResult
		Err    error
	}{
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by v1.API.Metadata": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Bad data from Prometheus": {
			Handler: testutil.JSONHandler("/api/v1/metadata", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/metadata", http.StatusUnauthorized, `<html>unauthorized</html>`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited data source": {
			Handler: testutil.JSONHandler("/api/v1/metadata", http.StatusTooManyRequests, `too many requests`),
			Err:     _errRateLimited,
		},
		"Data source that is down": {
			Handler: testutil.JSONHandler("/api/v1/metadata", http.StatusServiceUnavailable, `<html>upstream down</html>`),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Error returned by v1.API.LabelValues": {
			Handler: noMetadata(""),
			Err:     assert.AnError,
		},
		"Metric names when the server keeps no metadata": {
			Handler: noMetadata(`{"status":"success","data":["a","b"]}`),
			Result: &PrometheusMetadataResult{Result: map[string][]v1.Metadata{
				"a": {},
				"b": {},
			}},
		},
		"Metric names on the oldest VictoriaMetrics": {
			Version: _victoriaMetricsVersions[0],
			Result: &PrometheusMetadataResult{Result: map[string][]v1.Metadata{
				"vm_test_requests_total": {},
				"vm_test_up":             {},
			}},
		},
		"Metadata on the newest VictoriaMetrics": {
			Version: _victoriaMetricsVersions[1],
			Result: &PrometheusMetadataResult{Result: map[string][]v1.Metadata{
				"vm_test_requests_total": {{Type: "counter", Help: "Requests served."}},
				"vm_test_up":             {{Type: "gauge", Help: "Whether the job is up."}},
			}},
		},
		"Successful metadata retrieval": {
			Handler: testutil.JSONHandler("/api/v1/metadata", http.StatusOK, `{"status":"success","data":{"go_goroutines":[{"type":"gauge","help":"Number of goroutines.","unit":""}]}}`),
			Result: &PrometheusMetadataResult{
				Result: map[string][]v1.Metadata{
					"go_goroutines": {
						{Type: "gauge", Help: "Number of goroutines.", Unit: ""},
					},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp

			switch {
			case inp != nil:
			case c.Version != "":
				inp = prepVictoriaMetricsInput(t, c.Version)
			default:
				inp = prepPrometheusInput(t, "", c.Handler)
			}

			pa := &prometheusAPI{inp: inp}

			result, err := pa.Metadata(context.Background())
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_prometheusAPI_QueryRange(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler

		// URLPath is appended to the test server's URL.
		URLPath string

		// Query runs against every VictoriaMetrics container when neither
		// Inp nor Handler is set.
		Query string

		Result *PrometheusQueryResult

		// Series is how many series a container answers with, and
		// InvalidQuery whether it rejects the query instead.
		Series       int
		InvalidQuery bool

		Err error
	}{
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by v1.API.QueryRange": {
			Inp: &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Err: assert.AnError,
		},
		"Invalid query error": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Query that failed to execute": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusUnprocessableEntity, `{"status":"error","errorType":"execution","error":"many-to-many matching"}`),
			Err:     NewInvalidQueryError("many-to-many matching"),
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusUnauthorized, `<html>unauthorized</html>`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Forbidden data source": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusForbidden, `{"status":"error","error":"forbidden"}`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited data source": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusTooManyRequests, `too many requests`),
			Err:     _errRateLimited,
		},
		"Data source that is down": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusServiceUnavailable, `<html>upstream down</html>`),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Every macro on VictoriaMetrics": {
			Query:  "sum by (code) (rate(vm_test_requests_total[$__rate_interval]) + increase(vm_test_requests_total[$__interval]) * 0 + rate(vm_test_requests_total[$__range]) * 0)",
			Series: 2,
		},
		"MetricsQL WITH template on VictoriaMetrics": {
			Query:  "WITH (r = rate(vm_test_requests_total[5m])) sum by (code) (r)",
			Series: 2,
		},
		"MetricsQL rate without a window on VictoriaMetrics": {
			Query:  "rate(vm_test_requests_total)",
			Series: 2,
		},
		"Invalid query on VictoriaMetrics": {
			Query:        "sum(",
			InvalidQuery: true,
		},
		"Successful query through a cluster path": {
			Handler: testutil.JSONHandler("/select/0/prometheus/api/v1/query_range", http.StatusOK, `{"status":"success","data":{"resultType":"matrix","result":[]}}`),
			URLPath: "/select/0/prometheus",
			Result:  &PrometheusQueryResult{Type: model.ValMatrix, Result: model.Matrix{}},
		},
		"Successful query": {
			Handler: testutil.JSONHandler("/api/v1/query_range", http.StatusOK, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[1700000000,"1"]]}]}}`),
			Result: &PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"__name__": "up"},
						Values: []model.SamplePair{
							{Timestamp: model.Time(1700000000000), Value: 1},
						},
					},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Inp == nil && c.Handler == nil {
				for _, v := range _victoriaMetricsVersions {
					t.Run(v, func(t *testing.T) {
						t.Parallel()

						pa := &prometheusAPI{inp: prepVictoriaMetricsInput(t, v)}

						res, err := pa.QueryRange(context.Background(), c.Query, _logsWindow)
						assert.Equal(t, c.InvalidQuery, IsInvalidQuery(err))

						if c.InvalidQuery {
							return
						}

						require.NoError(t, err)
						assert.Len(t, res.Result, c.Series)
					})
				}

				return
			}

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, "", c.Handler)

				base := inp.URLFunc()
				inp.URLFunc = func() string { return base + c.URLPath }
			}

			pa := &prometheusAPI{inp: inp}

			result, err := pa.QueryRange(context.Background(), "up", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_prometheusAPI_LabelNames(t *testing.T) {
	// a case with neither Inp nor Handler runs against every
	// VictoriaMetrics container.
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusLabelNamesResult
		Err     error
	}{
		"Labels on VictoriaMetrics": {
			Result: &PrometheusLabelNamesResult{Result: []string{"__name__", "code", "job"}},
		},
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by v1.API.LabelNames": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Bad data from Prometheus": {
			Handler: testutil.JSONHandler("/api/v1/labels", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/labels", http.StatusUnauthorized, `<html>unauthorized</html>`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited data source": {
			Handler: testutil.JSONHandler("/api/v1/labels", http.StatusTooManyRequests, `too many requests`),
			Err:     _errRateLimited,
		},
		"Data source that is down": {
			Handler: testutil.JSONHandler("/api/v1/labels", http.StatusServiceUnavailable, `<html>upstream down</html>`),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Successful label names retrieval": {
			Handler: testutil.JSONHandler("/api/v1/labels", http.StatusOK, `{"status":"success","data":["__name__","job"]}`),
			Result: &PrometheusLabelNamesResult{
				Result: []string{"__name__", "job"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Inp == nil && c.Handler == nil {
				for _, v := range _victoriaMetricsVersions {
					t.Run(v, func(t *testing.T) {
						t.Parallel()

						pa := &prometheusAPI{inp: prepVictoriaMetricsInput(t, v)}

						res, err := pa.LabelNames(context.Background(), []string{_victoriaMetricsMatcher}, _logsWindow)
						require.NoError(t, err)
						assert.Equal(t, c.Result, res)
					})
				}

				return
			}

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, "", c.Handler)
			}

			pa := &prometheusAPI{inp: inp}

			result, err := pa.LabelNames(context.Background(), []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_prometheusAPI_LabelValues(t *testing.T) {
	// a case with neither Inp nor Handler runs against every
	// VictoriaMetrics container.
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusLabelValuesResult
		Err     error
	}{
		"Label values on VictoriaMetrics": {
			Result: &PrometheusLabelValuesResult{Result: []string{"200", "500"}},
		},
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by v1.API.LabelValues": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Bad data from Prometheus": {
			Handler: testutil.JSONHandler("/api/v1/label/instance/values", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/label/instance/values", http.StatusUnauthorized, `<html>unauthorized</html>`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited data source": {
			Handler: testutil.JSONHandler("/api/v1/label/instance/values", http.StatusTooManyRequests, `too many requests`),
			Err:     _errRateLimited,
		},
		"Data source that is down": {
			Handler: testutil.JSONHandler("/api/v1/label/instance/values", http.StatusServiceUnavailable, `<html>upstream down</html>`),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Successful label values retrieval": {
			Handler: testutil.JSONHandler("/api/v1/label/instance/values", http.StatusOK, `{"status":"success","data":["a","b"]}`),
			Result: &PrometheusLabelValuesResult{
				Result: []string{"a", "b"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Inp == nil && c.Handler == nil {
				for _, v := range _victoriaMetricsVersions {
					t.Run(v, func(t *testing.T) {
						t.Parallel()

						pa := &prometheusAPI{inp: prepVictoriaMetricsInput(t, v)}

						res, err := pa.LabelValues(context.Background(), "code", []string{_victoriaMetricsMatcher}, _logsWindow)
						require.NoError(t, err)
						assert.Equal(t, c.Result, res)
					})
				}

				return
			}

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, "", c.Handler)
			}

			pa := &prometheusAPI{inp: inp}

			result, err := pa.LabelValues(context.Background(), "instance", []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_prometheusAPI_Series(t *testing.T) {
	// a case with neither Inp nor Handler runs against every
	// VictoriaMetrics container.
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusSeriesResult
		Err     error
	}{
		"Series on VictoriaMetrics": {
			Result: &PrometheusSeriesResult{Result: []model.LabelSet{
				{"__name__": "vm_test_requests_total", "job": "api", "code": "200"},
				{"__name__": "vm_test_requests_total", "job": "api", "code": "500"},
				{"__name__": "vm_test_up", "job": "api"},
			}},
		},
		"Error returned by prometheusAPI.client": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by v1.API.Series": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Bad data from Prometheus": {
			Handler: testutil.JSONHandler("/api/v1/series", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Unauthorized data source": {
			Handler: testutil.JSONHandler("/api/v1/series", http.StatusUnauthorized, `<html>unauthorized</html>`),
			Err:     ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited data source": {
			Handler: testutil.JSONHandler("/api/v1/series", http.StatusTooManyRequests, `too many requests`),
			Err:     _errRateLimited,
		},
		"Data source that is down": {
			Handler: testutil.JSONHandler("/api/v1/series", http.StatusServiceUnavailable, `<html>upstream down</html>`),
			Err:     ConnectionStatusUnreachable.Error(),
		},
		"Successful series retrieval": {
			Handler: testutil.JSONHandler("/api/v1/series", http.StatusOK, `{"status":"success","data":[{"__name__":"up","job":"prometheus"}]}`),
			Result: &PrometheusSeriesResult{
				Result: []model.LabelSet{
					{"__name__": "up", "job": "prometheus"},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			if c.Inp == nil && c.Handler == nil {
				for _, v := range _victoriaMetricsVersions {
					t.Run(v, func(t *testing.T) {
						t.Parallel()

						pa := &prometheusAPI{inp: prepVictoriaMetricsInput(t, v)}

						res, err := pa.Series(context.Background(), []string{_victoriaMetricsMatcher}, _logsWindow)
						require.NoError(t, err)
						assert.ElementsMatch(t, c.Result.Result, res.Result)
					})
				}

				return
			}

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, "", c.Handler)
			}

			pa := &prometheusAPI{inp: inp}

			result, err := pa.Series(context.Background(), []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Len(t, inp.URLCalls(), 1)
			assert.Len(t, inp.CredentialsCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_prometheusAPI_client(t *testing.T) {
	cc := map[string]struct {
		URL   string
		Creds string

		// TLS indicates whether the data source is served over TLS, with
		// its own certificate as the credentials' CA.
		TLS bool

		// Auth is the Authorization header the data source receives.
		Auth string

		Err error
	}{
		"Error returned by Credentials.read": {
			URL:   "http://prometheus.test",
			Creds: `{`,
			Err:   assert.AnError,
		},
		"Credentials without a kind": {
			URL:   "http://prometheus.test",
			Creds: `{"username":"user","password":"pass"}`,
			Err:   credentials.ErrInvalidKind,
		},
		"Error returned by ownTransport.roundTripper": { //nolint:gosec // static test credentials
			URL:   "http://prometheus.test",
			Creds: `{"kind":"basic","caCertificate":"not a certificate"}`,
			Err:   credentials.ErrInvalidCACertificate,
		},
		"Error returned by api.NewClient": {
			URL: "://",
			Err: assert.AnError,
		},
		"No credentials": {},
		"Username and password": { //nolint:gosec // static test credentials
			Creds: `{"kind":"basic","username":"user","password":"pass"}`,
			Auth:  "Basic dXNlcjpwYXNz",
		},
		"Token": { //nolint:gosec // static test credentials
			Creds: `{"kind":"token","token":"secret"}`,
			Auth:  "Bearer secret",
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
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"status":"success","data":{"version":"2.50.0"}}`) //nolint:errcheck // error provides no meaningful info
			})

			var inp *InputMock

			switch {
			case c.URL != "":
				inp = &InputMock{URLFunc: func() string { return c.URL }}
			case c.TLS:
				inp = prepTLSInput(t, h, "")
			default:
				inp = prepPrometheusInput(t, "", h)
			}

			if c.Creds != "" {
				inp.CredentialsFunc = func() credentials.Credentials { return stubCredentials(c.Creds) }
			}

			pa := &prometheusAPI{inp: inp}

			t.Cleanup(func() { assert.NoError(t, pa.Close()) })

			client, err := pa.client()
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			_, err = client.Buildinfo(context.Background())
			require.NoError(t, err)
			assert.Equal(t, c.Auth, auth.Load())
		})
	}
}

func Test_PrometheusQueryResult_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		JSON   string
		Result *PrometheusQueryResult
		Err    error
	}{
		"Invalid JSON": {
			JSON: `{`,
			Err:  assert.AnError,
		},
		"Matrix that does not decode": {
			JSON: `{"type":"matrix","result":{"not":"a matrix"}}`,
			Err:  assert.AnError,
		},
		// the type a range query answers with comes back as itself, which
		// is what the assistant's raw series caps rely on.
		"Matrix": {
			JSON: `{"warnings":["slow"],"type":"matrix","result":[{"metric":{"job":"api"},"values":[[1700000000,"1"]]}]}`,
			Result: &PrometheusQueryResult{
				Warnings: []string{"slow"},
				Type:     model.ValMatrix,
				Result: model.Matrix{
					{
						Metric: model.Metric{"job": "api"},
						Values: []model.SamplePair{{Timestamp: 1700000000000, Value: 1}},
					},
				},
			},
		},
		"Any other result is kept as it decodes": {
			JSON: `{"type":"scalar","result":[1700000000,"1"]}`,
			Result: &PrometheusQueryResult{
				Type:   model.ValScalar,
				Result: []any{float64(1700000000), "1"},
			},
		},
		"No result": {
			JSON: `{"type":"matrix"}`,
			Result: &PrometheusQueryResult{
				Type: model.ValMatrix,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var res PrometheusQueryResult

			err := res.UnmarshalJSON([]byte(c.JSON))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, &res)
		})
	}
}

func Test_PrometheusQueryResult_HasData(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Result *PrometheusQueryResult
		Has    bool
	}{
		"No result":    {},
		"Empty matrix": {Result: &PrometheusQueryResult{Type: model.ValMatrix, Result: model.Matrix{}}},
		"Matrix with a series": {
			Result: &PrometheusQueryResult{Type: model.ValMatrix, Result: model.Matrix{{Metric: model.Metric{"job": "api"}}}},
			Has:    true,
		},
		"Result of another type": {Result: &PrometheusQueryResult{Type: model.ValScalar, Result: []any{float64(1), "1"}}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Has, c.Result.HasData())
		})
	}
}

func Test_PrometheusQueryResult_Transform(t *testing.T) {
	cc := map[string]struct {
		Result    PrometheusQueryResult
		ChartType ChartType
		Expected  QueryResult
	}{
		"Empty chart type returns type-not-selected": {
			Result:    PrometheusQueryResult{Type: model.ValMatrix},
			ChartType: "",
			Expected:  QueryResult{Status: QueryStatusTypeNotSelected},
		},
		"String result returns chart-and-data-mismatch": {
			Result:    PrometheusQueryResult{Type: model.ValString},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Unknown result type returns no-data": {
			Result:    PrometheusQueryResult{Type: model.ValNone},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"Scalar with line chart returns chart-and-data-mismatch": {
			Result:    PrometheusQueryResult{Type: model.ValScalar},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Scalar marshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValScalar, Result: make(chan int)},
			ChartType: ChartTypeBar,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Scalar unmarshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValScalar, Result: "garbage"},
			ChartType: ChartTypeBar,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Scalar with invalid value returns invalid": {
			Result: PrometheusQueryResult{
				Type: model.ValScalar,
				Result: &model.Scalar{
					Value:     model.SampleValue(math.NaN()),
					Timestamp: model.Time(1700000000000),
				},
			},
			ChartType: ChartTypeGauge,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Scalar success": {
			Result: PrometheusQueryResult{
				Type: model.ValScalar,
				Result: &model.Scalar{
					Value:     42,
					Timestamp: model.Time(1700000000000),
				},
			},
			ChartType: ChartTypeBar,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{},
						Metrics: [][2]any{{int64(1700000000), float64(42)}},
					},
				},
			},
		},
		"Vector marshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValVector, Result: make(chan int)},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Vector unmarshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValVector, Result: "garbage"},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Empty vector returns no-data": {
			Result:    PrometheusQueryResult{Type: model.ValVector, Result: model.Vector{}},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"Histogram-only vector returns chart-and-data-mismatch": {
			Result: PrometheusQueryResult{
				Type: model.ValVector,
				Result: model.Vector{
					&model.Sample{
						Metric:    model.Metric{"job": "a"},
						Timestamp: model.Time(1700000000000),
						Histogram: &model.SampleHistogram{
							Count: 1,
							Sum:   2,
						},
					},
				},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Vector without valid values returns invalid": {
			Result: PrometheusQueryResult{
				Type: model.ValVector,
				Result: model.Vector{
					&model.Sample{
						Metric:    model.Metric{"job": "a"},
						Value:     model.SampleValue(math.NaN()),
						Timestamp: model.Time(1700000000000),
					},
				},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Vector success": {
			Result: PrometheusQueryResult{
				Type: model.ValVector,
				Result: model.Vector{
					&model.Sample{
						Metric:    model.Metric{"job": "a"},
						Value:     10,
						Timestamp: model.Time(1700000000000),
					},
					&model.Sample{
						Metric:    model.Metric{"job": "b"},
						Value:     20,
						Timestamp: model.Time(1700000060000),
					},
				},
			},
			ChartType: ChartTypeBar,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{"job": "a"},
						Metrics: [][2]any{{int64(1700000000), float64(10)}},
					},
					{
						Labels:  map[string]string{"job": "b"},
						Metrics: [][2]any{{int64(1700000060), float64(20)}},
					},
				},
			},
		},
		"Matrix marshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValMatrix, Result: make(chan int)},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Matrix unmarshal error returns invalid": {
			Result:    PrometheusQueryResult{Type: model.ValMatrix, Result: "garbage"},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
		"Empty matrix returns no-data": {
			Result:    PrometheusQueryResult{Type: model.ValMatrix, Result: model.Matrix{}},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusNoData},
		},
		"Histogram-only matrix returns chart-and-data-mismatch": {
			Result: PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
					},
				},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusChartAndDataMismatch},
		},
		"Matrix line chart skips invalid values": {
			Result: PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
						Values: []model.SamplePair{
							{Timestamp: model.Time(1700000000000), Value: 10},
							{Timestamp: model.Time(1700000060000), Value: model.SampleValue(math.NaN())},
							{Timestamp: model.Time(1700000120000), Value: 30},
						},
					},
				},
			},
			ChartType: ChartTypeLine,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels: map[string]string{"job": "a"},
						Metrics: [][2]any{
							{int64(1700000000), float64(10)},
							{int64(1700000120), float64(30)},
						},
					},
				},
			},
		},
		"Matrix gauge keeps only last valid value": {
			Result: PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
						Values: []model.SamplePair{
							{Timestamp: model.Time(1700000000000), Value: 10},
							{Timestamp: model.Time(1700000060000), Value: 20},
							{Timestamp: model.Time(1700000120000), Value: model.SampleValue(math.NaN())},
						},
					},
				},
			},
			ChartType: ChartTypeGauge,
			Expected: QueryResult{
				Status: QueryStatusOK,
				Data: []QueryResultSeries{
					{
						Labels:  map[string]string{"job": "a"},
						Metrics: [][2]any{{int64(1700000060), float64(20)}},
					},
				},
			},
		},
		"Matrix without valid values returns invalid": {
			Result: PrometheusQueryResult{
				Type: model.ValMatrix,
				Result: model.Matrix{
					&model.SampleStream{
						Metric: model.Metric{"job": "a"},
						Values: []model.SamplePair{
							{Timestamp: model.Time(1700000000000), Value: model.SampleValue(math.NaN())},
						},
					},
				},
			},
			ChartType: ChartTypeLine,
			Expected:  QueryResult{Status: QueryStatusInvalid},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.Result.Transform(c.ChartType)
			require.Equal(t, c.Expected.Status, result.Status)
			assert.Equal(t, c.Expected.Data, result.Data)
		})
	}
}

func Test_prometheusAPIError(t *testing.T) {
	cc := map[string]struct {
		Err    error
		Result error
	}{
		"Error that is not from the API": {
			Err:    assert.AnError,
			Result: fmt.Errorf("fetching: %w", assert.AnError),
		},
		"Server error": {
			Err:    &v1.Error{Type: v1.ErrServer, Msg: "server error: 503"},
			Result: ConnectionStatusUnreachable.Error(),
		},
		"Unauthorized": {
			Err:    &v1.Error{Type: v1.ErrClient, Msg: "client error: 401"},
			Result: ConnectionStatusUnauthorized.Error(),
		},
		"Forbidden": {
			Err:    &v1.Error{Type: v1.ErrClient, Msg: "client error: 403"},
			Result: ConnectionStatusUnauthorized.Error(),
		},
		"Rate limited": {
			Err:    &v1.Error{Type: v1.ErrClient, Msg: "client error: 429"},
			Result: _errRateLimited,
		},
		"Rejected query": {
			Err:    &v1.Error{Type: v1.ErrBadData, Msg: "parse error"},
			Result: NewInvalidQueryError("parse error"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, prometheusAPIError(c.Err, "fetching"))
		})
	}
}
