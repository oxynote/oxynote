package testutil

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_AddChiCtx(t *testing.T) {
	cc := map[string]struct {
		Context context.Context
		Key     string
		Value   string
		Params  map[string]string
		Reused  bool
	}{
		"Context without chi route context": {
			Context: context.Background(),
			Key:     "key",
			Value:   "value",
			Params:  map[string]string{"key": "value"},
		},
		"Context with chi route context": {
			Context: context.WithValue(context.Background(), chi.RouteCtxKey, &chi.Context{
				URLParams: chi.RouteParams{Keys: []string{"key"}, Values: []string{"value"}},
			}),
			Key:    "key2",
			Value:  "value2",
			Params: map[string]string{"key": "value", "key2": "value2"},
			Reused: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ctx := AddChiCtx(c.Context, c.Key, c.Value)

			rctx, ok := ctx.Value(chi.RouteCtxKey).(*chi.Context)
			require.True(t, ok)

			for k, v := range c.Params {
				assert.Equal(t, v, rctx.URLParam(k))
			}

			if c.Reused {
				assert.Equal(t, c.Context, ctx)
			}
		})
	}
}

func Test_AssertResponse(t *testing.T) {
	cc := map[string]struct {
		Code   int
		Body   string
		Write  string
		Failed bool
	}{
		"Status code differs": {
			Code:   http.StatusOK,
			Failed: true,
		},
		"Empty body expected and none written": {
			Code: http.StatusNoContent,
		},
		"Empty body expected but one written": {
			Code:   http.StatusNoContent,
			Write:  `{"a":1}`,
			Failed: true,
		},
		"JSON body expected and matched": {
			Code:  http.StatusNoContent,
			Body:  `{"a": 1}`,
			Write: `{"a":1}`,
		},
		"JSON body expected but a different one written": {
			Code:   http.StatusNoContent,
			Body:   `{"a":2}`,
			Write:  `{"a":1}`,
			Failed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusNoContent)
			rec.WriteString(c.Write) //nolint:errcheck,gosec // error provides no meaningful info

			nt := &testing.T{}
			AssertResponse(nt, rec, c.Code, c.Body)
			assert.Equal(t, c.Failed, nt.Failed())
		})
	}
}

func Test_JSONHandler(t *testing.T) {
	cc := map[string]struct {
		Path        string
		RespCode    int
		ContentType string
		RespBody    string
	}{
		"Served path": {
			Path:        "/served",
			RespCode:    http.StatusTeapot,
			ContentType: "application/json",
			RespBody:    `{"a":1}`,
		},
		"Other path": {
			Path:        "/other",
			RespCode:    http.StatusNotFound,
			ContentType: "text/plain; charset=utf-8",
			RespBody:    "404 page not found\n",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rec := httptest.NewRecorder()

			JSONHandler("/served", http.StatusTeapot, `{"a":1}`).ServeHTTP(
				rec,
				httptest.NewRequest(http.MethodGet, "http://test.com"+c.Path, http.NoBody),
			)

			assert.Equal(t, c.RespCode, rec.Code)
			assert.Equal(t, c.ContentType, rec.Header().Get("Content-Type"))
			assert.Equal(t, c.RespBody, rec.Body.String())
		})
	}
}

func Test_MultipartBody(t *testing.T) {
	body, contentType := MultipartBody(t, "file", "img.png", []byte("data"))

	req := httptest.NewRequest(http.MethodPost, "http://test.com/", body)
	req.Header.Set("Content-Type", contentType)

	f, fh, err := req.FormFile("file")
	require.NoError(t, err)

	defer f.Close() //nolint:errcheck // error provides no meaningful info

	assert.Equal(t, "img.png", fh.Filename)

	content, err := io.ReadAll(f)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), content)
}

func Test_AssertEqualError(t *testing.T) {
	exp := errors.New("expected error")

	cc := map[string]struct {
		Exp    error
		Act    error
		Failed bool
	}{
		"No error expected and none occurred": {},
		"No error expected but one occurred": {
			Act:    assert.AnError,
			Failed: true,
		},
		"Generic error expected and one occurred": {
			Exp: assert.AnError,
			Act: errors.New("error"),
		},
		"Generic error expected but none occurred": {
			Exp:    assert.AnError,
			Failed: true,
		},
		"Concrete error expected and matched": {
			Exp: exp,
			Act: exp,
		},
		"Concrete error expected but a different one occurred": {
			Exp:    exp,
			Act:    errors.New("other error"),
			Failed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nt := &testing.T{}
			AssertEqualError(nt, c.Exp, c.Act)
			assert.Equal(t, c.Failed, nt.Failed())
		})
	}
}

func Test_RequireEqualError(t *testing.T) {
	exp := errors.New("expected error")

	cc := map[string]struct {
		Exp    error
		Act    error
		Failed bool
	}{
		"No error expected and none occurred": {},
		"No error expected but one occurred": {
			Act:    assert.AnError,
			Failed: true,
		},
		"Generic error expected and one occurred": {
			Exp: assert.AnError,
			Act: errors.New("error"),
		},
		"Generic error expected but none occurred": {
			Exp:    assert.AnError,
			Failed: true,
		},
		"Concrete error expected and matched": {
			Exp: exp,
			Act: exp,
		},
		"Concrete error expected but a different one occurred": {
			Exp:    exp,
			Act:    errors.New("other error"),
			Failed: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// require assertions stop the calling goroutine via FailNow on
			// failure, so the call runs in its own goroutine.
			nt := &testing.T{}
			done := make(chan struct{})

			go func() {
				defer close(done)

				RequireEqualError(nt, c.Exp, c.Act) //nolint:testifylint // the helper's require behavior is under test
			}()

			<-done

			assert.Equal(t, c.Failed, nt.Failed())
		})
	}
}

func Test_FilterEqual(t *testing.T) {
	type object struct {
		Name      string
		CreatedAt time.Time
	}

	type hidden struct {
		name string
	}

	cc := map[string]struct {
		V1          any
		V2          any
		IgnoreTypes []any
		Err         error
	}{
		"Values differ": {
			V1:  object{Name: "a"},
			V2:  object{Name: "b"},
			Err: assert.AnError,
		},
		"Ignored types are not compared": {
			V1:          object{Name: "a", CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
			V2:          object{Name: "a", CreatedAt: time.Date(2022, 3, 4, 0, 0, 0, 0, time.UTC)},
			IgnoreTypes: []any{time.Time{}},
		},
		"Equal instants in different locations are equal": {
			V1: object{CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC)},
			V2: object{CreatedAt: time.Date(2021, 1, 2, 0, 0, 0, 0, time.UTC).Local()},
		},
		"Unexported fields are compared as well": {
			V1: hidden{name: "a"},
			V2: hidden{name: "a"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := FilterEqual(c.V1, c.V2, c.IgnoreTypes...)
			AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_AssertFilterEqual(t *testing.T) {
	cc := map[string]struct {
		V1     any
		V2     any
		Failed bool
	}{
		"Values differ": {
			V1:     1,
			V2:     2,
			Failed: true,
		},
		"Values are equal": {
			V1: 1,
			V2: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			nt := &testing.T{}
			AssertFilterEqual(nt, c.V1, c.V2)
			assert.Equal(t, c.Failed, nt.Failed())
		})
	}
}

func Test_MetricValue(t *testing.T) {
	rg := prometheus.NewRegistry()

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "test_total",
			Help: "Counts tests.",
		},
		[]string{"kind"},
	)
	counter.WithLabelValues("a").Add(2)

	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "test_seconds",
			Help: "Times tests.",
		},
		[]string{"kind"},
	)
	histogram.WithLabelValues("a").Observe(0.5)

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge",
		Help: "Gauges tests.",
	})
	gauge.Set(5)

	rg.MustRegister(counter, histogram, gauge)

	cc := map[string]struct {
		Name   string
		Labels map[string]string
		Result float64
	}{
		"Counter reports its value": {
			Name:   "test_total",
			Labels: map[string]string{"kind": "a"},
			Result: 2,
		},
		"Histogram reports its sample count": {
			Name:   "test_seconds",
			Labels: map[string]string{"kind": "a"},
			Result: 1,
		},
		"Other metric types report nothing": {
			Name:   "test_gauge",
			Result: -1,
		},
		"Labels without a matching series": {
			Name:   "test_total",
			Labels: map[string]string{"kind": "b"},
			Result: -1,
		},
		"Unknown metric": {
			Name:   "test_unknown",
			Result: -1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, c.Result, MetricValue(t, rg, c.Name, c.Labels), 0.0001)
		})
	}
}

func Test_StubBeginTx(t *testing.T) {
	tx := &bytes.Buffer{}

	cc := map[string]struct {
		Err    error
		Result io.Reader
	}{
		"Begin fails": {
			Err: assert.AnError,
		},
		"Begin hands out the transaction": {
			Result: tx,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var dest io.Reader

			err := StubBeginTx(tx, c.Err)(context.Background(), &dest)
			AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, dest)
		})
	}
}

func Test_MockHTTP(t *testing.T) {
	client, mt := MockHTTP()

	require.NotNil(t, client)
	require.NotNil(t, mt)
	assert.Same(t, mt, client.Transport)
}

func Test_NewBuffer(t *testing.T) {
	w, b := NewBuffer()

	require.NotNil(t, w)
	require.NotNil(t, b)
	assert.NotNil(t, w.out)
}

func Test_Writer_Write(t *testing.T) {
	w, _ := NewBuffer()

	n, err := w.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
}

func Test_Writer_Flush(t *testing.T) {
	w, b := NewBuffer()

	_, err := w.Write([]byte("data"))
	require.NoError(t, err)
	require.Empty(t, b.String())

	require.NoError(t, w.Flush())
	assert.Equal(t, "data", b.String())
}

func Test_IgnoreBleveWorkers(t *testing.T) {
	t.Parallel()

	assert.NotNil(t, IgnoreBleveWorkers())
}
