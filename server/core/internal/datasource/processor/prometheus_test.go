package processor

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// prepPrometheusInput starts a fake Prometheus API server serving the
// provided handler and returns an input pointing at it. The cleanup also
// drops the Prometheus client's shared keep-alive connections so goleak
// stays clean.
func prepPrometheusInput(t *testing.T, h http.Handler) *InputMock {
	t.Helper()

	srv := httptest.NewServer(h)

	t.Cleanup(func() {
		srv.Close()
		api.DefaultRoundTripper.(*http.Transport).CloseIdleConnections()
	})

	return &InputMock{
		URLFunc: func() string { return srv.URL },
	}
}

// prometheusEndpoint builds a handler serving a single Prometheus API path
// with the given status code and body, 404-ing every other path.
func prometheusEndpoint(path string, code int, body string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprint(w, body) //nolint:errcheck // error provides no meaningful info
	})

	return mux
}

func Test_NewPrometheus(t *testing.T) {
	t.Parallel()

	inp := &InputMock{}

	p := NewPrometheus(inp)
	require.NotNil(t, p)
	assert.Same(t, inp, p.inp)
}

func Test_Prometheus_TestConnection(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  ConnectionStatus
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Unauthorized data source": {
			Handler: prometheusEndpoint("/api/v1/status/buildinfo", http.StatusUnauthorized, `{}`),
			Result:  ConnectionStatusUnauthorized,
		},
		"Unreachable data source": {
			Handler: prometheusEndpoint("/api/v1/status/buildinfo", http.StatusInternalServerError, `{}`),
			Result:  ConnectionStatusUnreachable,
		},
		"Unsupported version": {
			Handler: prometheusEndpoint("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":""}}`),
			Result:  ConnectionStatusVersionNotSupported,
		},
		"Successful connection": {
			Handler: prometheusEndpoint("/api/v1/status/buildinfo", http.StatusOK, `{"status":"success","data":{"version":"2.50.0"}}`),
			Result:  ConnectionStatusSuccess,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			cs, err := p.TestConnection(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, cs)
		})
	}
}

func Test_Prometheus_Metadata(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusMetadataResult
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by client.Metadata": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Successful metadata retrieval": {
			Handler: prometheusEndpoint("/api/v1/metadata", http.StatusOK, `{"status":"success","data":{"go_goroutines":[{"type":"gauge","help":"Number of goroutines.","unit":""}]}}`),
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
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			result, err := p.Metadata(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_Prometheus_QueryRange(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusQueryResult
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by client.QueryRange": {
			Inp: &InputMock{URLFunc: func() string { return "http://127.0.0.1:1" }},
			Err: assert.AnError,
		},
		"Invalid query error": {
			Handler: prometheusEndpoint("/api/v1/query_range", http.StatusBadRequest, `{"status":"error","errorType":"bad_data","error":"parse error"}`),
			Err:     NewInvalidQueryError("parse error"),
		},
		"Successful query": {
			Handler: prometheusEndpoint("/api/v1/query_range", http.StatusOK, `{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"up"},"values":[[1700000000,"1"]]}]}}`),
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

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			result, err := p.QueryRange(context.Background(), "up", _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_Prometheus_LabelNames(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusLabelNamesResult
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by client.LabelNames": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Successful label names retrieval": {
			Handler: prometheusEndpoint("/api/v1/labels", http.StatusOK, `{"status":"success","data":["__name__","job"]}`),
			Result: &PrometheusLabelNamesResult{
				Result: []string{"__name__", "job"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			result, err := p.LabelNames(context.Background(), []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_Prometheus_LabelValues(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusLabelValuesResult
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by client.LabelValues": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Successful label values retrieval": {
			Handler: prometheusEndpoint("/api/v1/label/instance/values", http.StatusOK, `{"status":"success","data":["a","b"]}`),
			Result: &PrometheusLabelValuesResult{
				Result: []string{"a", "b"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			result, err := p.LabelValues(context.Background(), "instance", []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_Prometheus_Series(t *testing.T) {
	cc := map[string]struct {
		Inp     *InputMock
		Handler http.Handler
		Result  *PrometheusSeriesResult
		Err     error
	}{
		"Error returned by createPrometheusClient": {
			Inp: &InputMock{URLFunc: func() string { return "://" }},
			Err: assert.AnError,
		},
		"Error returned by client.Series": {
			Handler: http.NewServeMux(),
			Err:     assert.AnError,
		},
		"Successful series retrieval": {
			Handler: prometheusEndpoint("/api/v1/series", http.StatusOK, `{"status":"success","data":[{"__name__":"up","job":"prometheus"}]}`),
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

			inp := c.Inp
			if inp == nil {
				inp = prepPrometheusInput(t, c.Handler)
			}

			p := NewPrometheus(inp)

			result, err := p.Series(context.Background(), []string{"up"}, _testTimeRange)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, result)
		})
	}
}

func Test_createPrometheusClient(t *testing.T) {
	t.Parallel()

	// error
	_, err := createPrometheusClient("http://prometheus.test", NewCredentials([]byte(`{`)))
	assert.Error(t, err)

	_, err = createPrometheusClient("://", Credentials{})
	assert.Error(t, err)

	// success
	client, err := createPrometheusClient("http://prometheus.test", Credentials{})
	require.NoError(t, err)
	assert.NotNil(t, client)

	client, err = createPrometheusClient("http://prometheus.test", NewCredentials([]byte(`{"username":"user","password":"pass"}`)))
	require.NoError(t, err)
	assert.NotNil(t, client)
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
