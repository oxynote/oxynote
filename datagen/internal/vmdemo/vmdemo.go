// Package vmdemo fills the demo VictoriaMetrics data source with the history
// of the metrics datagen serves.
package vmdemo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/oxynote/oxynote/datagen/internal/gen"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// _backfillDays defines how much history an empty VictoriaMetrics is filled
// with. It stays below VictoriaMetrics' default retention of one month,
// which drops older samples.
const _backfillDays = 30

// _flushSize defines how many bytes of samples are buffered before they are
// sent in one import request.
const _flushSize = 1 << 20

// _requestTimeout bounds how long a single request may take.
const _requestTimeout = 30 * time.Second

// _job and _instance are the labels the dev stack's scrape config gives
// datagen's series. The imported history carries them too, so it joins the
// scraped series instead of starting new ones.
const (
	_job      = "datagen"
	_instance = "datagen:8080"
)

// _doneMetric names the sample written once the whole history is in. A
// scrape that failed before datagen started leaves datagen series too, and
// so does a backfill that stopped half way. So only this sample counts as a
// finished backfill.
const _doneMetric = "datagen_backfill_done"

// Importer writes generated history into VictoriaMetrics.
type Importer struct {
	// log is the structured logger.
	log *slog.Logger

	// httpClient sends the requests to VictoriaMetrics.
	httpClient *http.Client

	// url is the base address of VictoriaMetrics.
	url string

	// buf holds the samples not yet sent.
	buf bytes.Buffer

	// backfillDays defines how much history an empty VictoriaMetrics is
	// filled with.
	backfillDays int
}

// NewImporter creates a fresh instance of Importer.
func NewImporter(log *slog.Logger, httpClient *http.Client, baseURL string) *Importer {
	return &Importer{
		log:          log,
		httpClient:   httpClient,
		url:          baseURL,
		backfillDays: _backfillDays,
	}
}

// Fill writes the history src generates up to now into VictoriaMetrics,
// unless a backfill finished within that period already.
func (i *Importer) Fill(ctx context.Context, src Source) error {
	now := timeutil.Now()
	from := now.AddDate(0, 0, -i.backfillDays)

	done, err := i.isDone(ctx, from)
	if err != nil {
		return fmt.Errorf("vmdemo: check backfill: %w", err)
	}

	if done {
		return nil
	}

	i.log.Info("vmdemo: backfilling historical data", slog.Int("days", i.backfillDays))

	if err := src.Backfill(ctx, from, now, i); err != nil {
		return fmt.Errorf("vmdemo: backfill: %w", err)
	}

	fmt.Fprintf(&i.buf, "%s 1 %d\n", _doneMetric, now.UnixMilli())

	if err := i.Flush(ctx); err != nil {
		return fmt.Errorf("vmdemo: mark backfill done: %w", err)
	}

	// VictoriaMetrics makes new samples searchable only after a few
	// seconds. So it flushes them now, or a restart right after the
	// backfill would miss the marker and backfill again.
	if _, err := i.do(ctx, http.MethodGet, "/internal/force_flush", nil); err != nil {
		return fmt.Errorf("vmdemo: flush backfill: %w", err)
	}

	i.log.Info("vmdemo: backfill complete")

	return nil
}

// WriteSamples buffers every sample of ff with the timestamp t, and sends
// the buffer once it is large enough.
func (i *Importer) WriteSamples(ctx context.Context, t time.Time, ff []*dto.MetricFamily) error {
	ms := t.UnixMilli()

	for _, f := range ff {
		for _, m := range f.GetMetric() {
			m.TimestampMs = &ms
		}

		if _, err := expfmt.MetricFamilyToText(&i.buf, f); err != nil {
			return fmt.Errorf("encoding metric family: %w", err)
		}
	}

	if i.buf.Len() < _flushSize {
		return nil
	}

	return i.Flush(ctx)
}

// Flush sends the buffered samples to VictoriaMetrics in one import request.
func (i *Importer) Flush(ctx context.Context) error {
	if i.buf.Len() == 0 {
		return nil
	}

	q := url.Values{}
	q.Add("extra_label", "job="+_job)
	q.Add("extra_label", "instance="+_instance)

	if _, err := i.do(ctx, http.MethodPost, "/api/v1/import/prometheus?"+q.Encode(), i.buf.Bytes()); err != nil {
		return fmt.Errorf("importing samples: %w", err)
	}

	i.buf.Reset()

	return nil
}

// isDone reports whether a backfill finished since the given time.
func (i *Importer) isDone(ctx context.Context, since time.Time) (bool, error) {
	q := url.Values{}
	q.Set("match[]", fmt.Sprintf("%s{job=%q}", _doneMetric, _job))
	q.Set("start", strconv.FormatInt(since.Unix(), 10))

	raw, err := i.do(ctx, http.MethodGet, "/api/v1/series?"+q.Encode(), nil)
	if err != nil {
		return false, fmt.Errorf("listing series: %w", err)
	}

	var body struct {
		Data []json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(raw, &body); err != nil {
		return false, fmt.Errorf("decoding series: %w", err)
	}

	return len(body.Data) > 0, nil
}

// do sends a request to VictoriaMetrics and returns the response body, or
// an error when the status is not a success.
func (i *Importer) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, _requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, i.url+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return raw, nil
}

// Source generates the history of the metrics datagen serves.
//
//go:generate ../../../server/core/scripts/codegen/mock -t internal Source source
type Source interface {
	// Backfill should generate a tick per interval between from and to,
	// and write each tick to w at its time.
	Backfill(ctx context.Context, from, to time.Time, w gen.SampleWriter) error
}
