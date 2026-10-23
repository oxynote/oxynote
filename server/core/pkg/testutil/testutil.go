// Package testutil implements helper functions for more convenient testing
// and may be imported in '_test.go' files only.
// Usage of some of these functions would have to be replaced by the ones
// provided by the official testing toolkit packages, should they decide
// to include helper functions of similar logic and kind.
package testutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jarcoal/httpmock"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// IgnoreBleveWorkers ignores the analysis workers bleve starts when it is
// loaded and never stops, for goleak.VerifyTestMain in every package that
// links the search index.
func IgnoreBleveWorkers() goleak.Option {
	return goleak.IgnoreTopFunction("github.com/blevesearch/bleve_index_api.AnalysisWorker")
}

// AddChiCtx adds the provided URL parameter to the context's chi route
// context, creating one when the context doesn't carry it yet.
func AddChiCtx(ctx context.Context, key, value string) context.Context {
	rctx, ok := ctx.Value(chi.RouteCtxKey).(*chi.Context)
	if !ok {
		rctx = chi.NewRouteContext()
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}

	rctx.URLParams.Add(key, value)

	return ctx
}

// AssertResponse asserts that the recorded response carries the status code
// and a JSON body equal to body, or no body at all when body is empty.
func AssertResponse(t *testing.T, rec *httptest.ResponseRecorder, code int, body string) {
	t.Helper()

	assert.Equal(t, code, rec.Code)

	if body == "" {
		assert.Zero(t, rec.Body.Len(), rec.Body.String())
		return
	}

	assert.JSONEq(t, body, rec.Body.String())
}

// JSONHandler creates a handler that answers the path with the status code
// and JSON body, and every other path with a 404.
func JSONHandler(path string, code int, body string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		fmt.Fprint(w, body) //nolint:errcheck // error provides no meaningful info
	})

	return mux
}

// MultipartBody creates a multipart form body carrying a single file field
// and returns it with its Content-Type header value.
func MultipartBody(t *testing.T, field, name string, content []byte) (io.Reader, string) {
	t.Helper()

	var buf bytes.Buffer

	mw := multipart.NewWriter(&buf)

	fw, err := mw.CreateFormFile(field, name)
	require.NoError(t, err)

	_, err = fw.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	return &buf, mw.FormDataContentType()
}

// AssertEqualError uses testify's assert package to check if errors
// are equal or, if assert.AnError is expected, whether an error exists
// or not.
func AssertEqualError(t *testing.T, exp, err error) {
	t.Helper()

	if exp != nil {
		if exp == assert.AnError { //nolint:err113,errorlint // direct check is needed
			assert.Error(t, err)
			return
		}

		assert.Equal(t, exp, err)

		return
	}

	assert.NoError(t, err)
}

// RequireEqualError uses testify's require package to check if errors
// are equal or, if assert.AnError is expected, whether an error exists
// or not.
func RequireEqualError(t *testing.T, exp, err error) {
	t.Helper()

	if exp != nil {
		if exp == assert.AnError { //nolint:err113,errorlint // direct check is needed
			require.Error(t, err)
			return
		}

		require.Equal(t, exp, err)

		return
	}

	require.NoError(t, err)
}

// FilterEqual asserts that two objects are equal.
// All ignored types found on any of the provided objects will not be compared.
func FilterEqual(v1, v2 any, ignoreTypes ...any) error {
	diff := cmp.Diff(v1, v2, cmpopts.IgnoreTypes(ignoreTypes...),
		cmp.Exporter(func(reflect.Type) bool { return true }))
	if diff != "" {
		return fmt.Errorf("not equal:\n%s", diff)
	}

	return nil
}

// AssertFilterEqual asserts that two objects are equal.
// All ignored types found on any of the provided objects will not be compared.
func AssertFilterEqual(t *testing.T, v1, v2 any, ignoreTypes ...any) {
	t.Helper()

	if err := FilterEqual(v1, v2, ignoreTypes...); err != nil {
		t.Error(err)
	}
}

// MetricValue returns the value of the named metric whose labels include
// every pair in labels, or -1 when there is none. Counters report their
// value, histograms their sample count and other types -1.
func MetricValue(t *testing.T, rg prometheus.Gatherer, name string, labels map[string]string) float64 {
	t.Helper()

	families, err := rg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}

	metric:
		for _, m := range mf.GetMetric() {
			got := map[string]string{}

			for _, lp := range m.GetLabel() {
				got[lp.GetName()] = lp.GetValue()
			}

			for k, v := range labels {
				if got[k] != v {
					continue metric
				}
			}

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				return m.GetCounter().GetValue()
			case dto.MetricType_HISTOGRAM:
				return float64(m.GetHistogram().GetSampleCount())
			default:
				return -1
			}
		}
	}

	return -1
}

// StubBeginTx creates a BeginTx stub that fails with err or, without one,
// sets tx into the pointer it is given, as the real BeginTx does.
func StubBeginTx(tx any, err error) func(context.Context, any) error {
	return func(_ context.Context, dest any) error {
		if err != nil {
			return err
		}

		reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(tx))

		return nil
	}
}

// MockHTTP creates an HTTP client backed by an isolated httpmock
// transport, so tests never activate httpmock globally.
func MockHTTP() (*http.Client, *httpmock.MockTransport) {
	mt := httpmock.NewMockTransport()

	return &http.Client{Transport: mt}, mt
}

// NewBuffer creates a fresh instance of concurrent writer and its buffer.
func NewBuffer() (*Writer, *bytes.Buffer) {
	b := &bytes.Buffer{}
	out := bufio.NewWriter(b)

	return &Writer{out: out}, b
}

// Writer wraps the underlying bufio.Writer to allow concurrently safe
// access.
type Writer struct {
	out *bufio.Writer
	mu  sync.Mutex
}

// Write writes the contents of p into the buffer.
func (w *Writer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.out.Write(p)
}

// Flush writes any buffered data to the underlying io.Writer.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.out.Flush()
}
