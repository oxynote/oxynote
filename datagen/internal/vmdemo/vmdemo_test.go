package vmdemo

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/oxynote/oxynote/datagen/internal/gen"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/protobuf/proto"
)

const (
	_testURL    = "http://vm.test"
	_seriesURL  = _testURL + "/api/v1/series"
	_importURL  = _testURL + "/api/v1/import/prometheus"
	_noSeries   = `{"status":"success","data":[]}`
	_someSeries = `{"status":"success","data":[{"__name__":"engineering_up","job":"datagen"}]}`
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewImporter(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)
	client := &http.Client{}

	i := NewImporter(log, client, _testURL)
	require.NotNil(t, i)

	assert.Equal(t, log, i.log)
	assert.Equal(t, client, i.httpClient)
	assert.Equal(t, _testURL, i.url)
	assert.Zero(t, i.buf.Len())
	assert.Equal(t, _backfillDays, i.backfillDays)
}

func Test_Importer_Fill(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Resp      httpmock.Responder
		Source    *SourceMock
		Err       error
		Backfills int
	}{
		"Error returned by the series request": {
			Resp:      httpmock.NewStringResponder(http.StatusInternalServerError, ""),
			Source:    &SourceMock{},
			Err:       assert.AnError,
			Backfills: 0,
		},
		"Existing history skips the backfill": {
			Resp:      httpmock.NewStringResponder(http.StatusOK, _someSeries),
			Source:    &SourceMock{},
			Backfills: 0,
		},
		"Error returned by Source.Backfill": {
			Resp: httpmock.NewStringResponder(http.StatusOK, _noSeries),
			Source: &SourceMock{
				BackfillFunc: func(context.Context, time.Time, time.Time, gen.SampleWriter) error {
					return assert.AnError
				},
			},
			Err:       assert.AnError,
			Backfills: 1,
		},
		"Successful backfill": {
			Resp:      httpmock.NewStringResponder(http.StatusOK, _noSeries),
			Source:    &SourceMock{},
			Backfills: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := testutil.MockHTTP()
			mt.RegisterResponder(http.MethodGet, _seriesURL, c.Resp)

			i := NewImporter(slog.New(slog.DiscardHandler), client, _testURL)

			before := timeutil.Now()

			err := i.Fill(context.Background(), c.Source)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, 1, mt.GetTotalCallCount())

			calls := c.Source.BackfillCalls()
			require.Len(t, calls, c.Backfills)

			if c.Backfills == 0 {
				return
			}

			// the window ends now and reaches back the backfill period.
			assert.WithinRange(t, calls[0].To, before, timeutil.Now())
			assert.Equal(t, calls[0].To.AddDate(0, 0, -_backfillDays), calls[0].From)
			assert.Same(t, i, calls[0].W)
		})
	}
}

func Test_Importer_WriteSamples(t *testing.T) {
	t.Parallel()

	at := time.UnixMilli(1_767_607_200_000)

	cc := map[string]struct {
		Families []*dto.MetricFamily
		Buffered int
		Resp     httpmock.Responder
		Err      error
		Imports  int
		Buf      string
	}{
		"Error returned by encoding": {
			Families: []*dto.MetricFamily{{
				Name:   new("engineering_broken"),
				Type:   dto.MetricType_GAUGE.Enum(),
				Metric: []*dto.Metric{{Counter: &dto.Counter{Value: proto.Float64(1)}}},
			}},
			Err:     assert.AnError,
			Imports: 0,
		},
		"Samples below the flush size stay buffered": {
			Families: []*dto.MetricFamily{gauge("engineering_up", "vibe", "nervous", 2)},
			Imports:  0,
			Buf:      "# TYPE engineering_up gauge\nengineering_up{vibe=\"nervous\"} 2 1767607200000\n",
		},
		"Error returned by the import request once the buffer is full": {
			Families: []*dto.MetricFamily{gauge("engineering_up", "vibe", "nervous", 2)},
			Buffered: _flushSize,
			Resp:     httpmock.NewStringResponder(http.StatusBadRequest, ""),
			Err:      assert.AnError,
			Imports:  1,
		},
		"Successful import once the buffer is full": {
			Families: []*dto.MetricFamily{gauge("engineering_up", "vibe", "nervous", 2)},
			Buffered: _flushSize,
			Resp:     httpmock.NewStringResponder(http.StatusNoContent, ""),
			Imports:  1,
			Buf:      "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			client, mt := testutil.MockHTTP()
			if c.Resp != nil {
				mt.RegisterResponder(http.MethodPost, _importURL, c.Resp)
			}

			i := NewImporter(slog.New(slog.DiscardHandler), client, _testURL)
			i.buf.WriteString(strings.Repeat("#", c.Buffered))

			err := i.WriteSamples(context.Background(), at, c.Families)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Imports, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Buf, i.buf.String())
		})
	}
}

func Test_Importer_Flush(t *testing.T) {
	t.Parallel()

	const samples = "engineering_up{vibe=\"nervous\"} 2 1767607200000\n"

	cc := map[string]struct {
		Buf     string
		Resp    httpmock.Responder
		Err     error
		Imports int
		Kept    string
	}{
		"An empty buffer sends nothing": {
			Buf:     "",
			Imports: 0,
		},
		"Error returned by the import request": {
			Buf:     samples,
			Resp:    httpmock.NewStringResponder(http.StatusBadRequest, ""),
			Err:     assert.AnError,
			Imports: 1,
			Kept:    samples,
		},
		"Successful import": {
			Buf:     samples,
			Resp:    httpmock.NewStringResponder(http.StatusNoContent, ""),
			Imports: 1,
			Kept:    "",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				query url.Values
				body  string
			)

			client, mt := testutil.MockHTTP()
			if c.Resp != nil {
				mt.RegisterResponder(http.MethodPost, _importURL, func(req *http.Request) (*http.Response, error) {
					query = req.URL.Query()

					raw, err := io.ReadAll(req.Body)
					require.NoError(t, err)

					body = string(raw)

					return c.Resp(req)
				})
			}

			i := NewImporter(slog.New(slog.DiscardHandler), client, _testURL)
			i.buf.WriteString(c.Buf)

			err := i.Flush(context.Background())
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Imports, mt.GetTotalCallCount())
			assert.Equal(t, c.Kept, i.buf.String())

			if c.Imports == 0 {
				return
			}

			assert.Equal(t, samples, body)
			assert.Equal(t, []string{"job=datagen", "instance=datagen:8080"}, query["extra_label"])
		})
	}
}

func Test_Importer_hasData(t *testing.T) {
	t.Parallel()

	since := time.Unix(1_767_607_200, 0)

	cc := map[string]struct {
		Resp   httpmock.Responder
		Err    error
		Result bool
	}{
		"Error returned by the request": {
			Resp: httpmock.NewStringResponder(http.StatusInternalServerError, ""),
			Err:  assert.AnError,
		},
		"Invalid JSON": {
			Resp: httpmock.NewStringResponder(http.StatusOK, "{"),
			Err:  assert.AnError,
		},
		"No series": {
			Resp:   httpmock.NewStringResponder(http.StatusOK, _noSeries),
			Result: false,
		},
		"Some series": {
			Resp:   httpmock.NewStringResponder(http.StatusOK, _someSeries),
			Result: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var query url.Values

			client, mt := testutil.MockHTTP()
			mt.RegisterResponder(http.MethodGet, _seriesURL, func(req *http.Request) (*http.Response, error) {
				query = req.URL.Query()

				return c.Resp(req)
			})

			i := NewImporter(slog.New(slog.DiscardHandler), client, _testURL)

			res, err := i.hasData(context.Background(), since)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, 1, mt.GetTotalCallCount())
			assert.Equal(t, `{job="datagen"}`, query.Get("match[]"))
			assert.Equal(t, "1767607200", query.Get("start"))

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_Importer_do(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Method string
		Resp   httpmock.Responder
		Err    error
		Result []byte
		Calls  int
	}{
		"Error returned by request creation": {
			Method: "BAD METHOD",
			Err:    assert.AnError,
			Calls:  0,
		},
		"Error returned by the client": {
			Method: http.MethodPost,
			Resp:   httpmock.ConnectionFailure,
			Err:    assert.AnError,
			Calls:  1,
		},
		"Unexpected status": {
			Method: http.MethodPost,
			Resp:   httpmock.NewStringResponder(http.StatusServiceUnavailable, ""),
			Err:    assert.AnError,
			Calls:  1,
		},
		"Error returned by reading the response": {
			Method: http.MethodPost,
			Resp: httpmock.ResponderFromResponse(&http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(iotest.ErrReader(assert.AnError)),
			}),
			Err:   assert.AnError,
			Calls: 1,
		},
		"Successful request": {
			Method: http.MethodPost,
			Resp:   httpmock.NewStringResponder(http.StatusOK, "done"),
			Result: []byte("done"),
			Calls:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var body string

			client, mt := testutil.MockHTTP()
			if c.Resp != nil {
				mt.RegisterResponder(c.Method, _testURL+"/path", func(req *http.Request) (*http.Response, error) {
					raw, err := io.ReadAll(req.Body)
					require.NoError(t, err)

					body = string(raw)

					return c.Resp(req)
				})
			}

			i := NewImporter(slog.New(slog.DiscardHandler), client, _testURL)

			res, err := i.do(context.Background(), c.Method, "/path", []byte("payload"))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Calls, mt.GetTotalCallCount())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, "payload", body)
		})
	}
}

// gauge builds a gauge family with one labelled value.
func gauge(name, label, value string, v float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: new(name),
		Type: dto.MetricType_GAUGE.Enum(),
		Metric: []*dto.Metric{{
			Label: []*dto.LabelPair{{Name: new(label), Value: new(value)}},
			Gauge: &dto.Gauge{Value: new(v)},
		}},
	}
}
