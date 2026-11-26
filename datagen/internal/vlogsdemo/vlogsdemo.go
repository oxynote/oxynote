// Package vlogsdemo fills the demo VictoriaLogs data source with generated
// logs.
package vlogsdemo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

const (
	// _backfillDays defines how much history an empty store is filled with.
	_backfillDays = 7

	// _backfillStep defines how much time one backfilled tick covers.
	_backfillStep = time.Minute

	// _tickInterval defines how often new logs are appended.
	_tickInterval = 5 * time.Second

	// _maxTickLogs defines the most logs a single tick writes.
	_maxTickLogs = 4

	// _batchSize defines how many backfilled logs are sent in one request.
	_batchSize = 1000

	// _requestTimeout bounds a single request to VictoriaLogs.
	_requestTimeout = 30 * time.Second

	// _ingestPath is the JSON lines endpoint. Service and env form the
	// log stream.
	_ingestPath = "/insert/jsonline?_stream_fields=service,env&_time_field=_time&_msg_field=_msg"

	// _readyAttempts defines how many times a request to the store is tried
	// at startup.
	_readyAttempts = 30

	// _readyDelay defines the wait between those attempts.
	_readyDelay = 2 * time.Second

	// _queryPath is the LogsQL query endpoint.
	_queryPath = "/select/logsql/query"

	// _ingestContentType is the content type of the JSON lines endpoint.
	_ingestContentType = "application/stream+json"

	// _serviceAPI writes its message as JSON.
	_serviceAPI = "api"

	// _serviceWorker writes its message as logfmt.
	_serviceWorker = "worker"

	// _serviceGateway writes its message as a plain access log.
	_serviceGateway = "gateway"

	// _errorShare, _warnShare and _debugShare are cumulative shares of the
	// logs. The rest are info.
	_errorShare = 0.06
	_warnShare  = 0.18
	_debugShare = 0.25

	// _hostCount defines how many hosts each service runs on.
	_hostCount = 3

	// _maxAttempts defines the most attempts a worker job reports.
	_maxAttempts = 3

	// _minDurationMs and _durationSpreadMs shape how long a request or a
	// job takes.
	_minDurationMs    = 5
	_durationSpreadMs = 400

	// _subnetCount and _subnetHosts shape the client addresses.
	_subnetCount = 4
	_subnetHosts = 254

	// _minBytes and _bytesSpread shape the size of a gateway response.
	_minBytes    = 200
	_bytesSpread = 4000
)

// _services lists the services that write logs.
var _services = []string{_serviceAPI, _serviceWorker, _serviceGateway}

// _paths lists the request paths of the api and gateway logs.
var _paths = []string{"/api/orders", "/api/users", "/api/invoices", "/health"}

// _jobs lists the job names of the worker logs.
var _jobs = []string{"send_email", "sync_invoices", "resize_image", "rebuild_index"}

// _envs lists the environments, weighted towards prod.
var _envs = []string{"prod", "prod", "prod", "prod", "staging"}

// _statuses lists the HTTP statuses of each severity that has its own.
var _statuses = map[string][]int{
	"error": {http.StatusInternalServerError, http.StatusServiceUnavailable},
	"warn":  {http.StatusNotFound, http.StatusTooManyRequests},
}

// _okStatuses lists the HTTP statuses of every other severity.
var _okStatuses = []int{http.StatusOK, http.StatusCreated}

// entry is one log as VictoriaLogs ingests it.
type entry struct {
	// Time specifies when the log was written, in RFC 3339.
	Time string `json:"_time"` //nolint:tagliatelle // VictoriaLogs field name

	// Message specifies the log line.
	Message string `json:"_msg"` //nolint:tagliatelle // VictoriaLogs field name

	// Service specifies the service that wrote the log.
	Service string `json:"service"`

	// Env specifies the environment of the service.
	Env string `json:"env"`

	// Host specifies the machine that wrote the log.
	Host string `json:"host"`

	// Level specifies the severity of the log.
	Level string `json:"level"`
}

// Generator writes demo logs to VictoriaLogs.
type Generator struct {
	// log is the structured logger.
	log *slog.Logger

	// httpClient sends the requests to VictoriaLogs.
	httpClient *http.Client

	// url addresses the VictoriaLogs server.
	url string

	// r is the random source used for all log generation.
	r *rand.Rand

	// interval defines how often new logs are appended.
	interval time.Duration

	// backfillDays defines how much history an empty store is filled with.
	backfillDays int

	// readyDelay defines the wait between the startup attempts.
	readyDelay time.Duration
}

// NewGenerator creates a fresh instance of Generator.
func NewGenerator(log *slog.Logger, url string) *Generator {
	return &Generator{
		log:          log,
		httpClient:   &http.Client{Timeout: _requestTimeout},
		url:          strings.TrimRight(url, "/"),
		r:            mockmetrics.NewRand(timeutil.Now().UnixNano()),
		interval:     _tickInterval,
		backfillDays: _backfillDays,
		readyDelay:   _readyDelay,
	}
}

// Run backfills history if the store is empty, then appends logs on each
// tick. It blocks until ctx is cancelled.
func (g *Generator) Run(ctx context.Context) error {
	empty, err := g.awaitStore(ctx)
	if err != nil {
		return fmt.Errorf("vlogsdemo: check backfill: %w", err)
	}

	if empty {
		g.log.Info("vlogsdemo: backfilling historical logs", slog.Int("days", g.backfillDays))

		// NOTE: a partial history is still a working demo, so the new logs
		// keep coming after a failed backfill.
		if err := g.backfill(ctx, timeutil.Now()); err != nil {
			g.log.Error("cannot backfill demo logs", slog.String("error", err.Error()))
		} else {
			g.log.Info("vlogsdemo: backfill complete")
		}
	}

	timeutil.NewPeriodicExec(
		g.interval,
		0,
		func(ctx context.Context) {
			if err := g.ingest(ctx, g.newTick(timeutil.Now(), g.interval)); err != nil {
				g.log.Error("cannot insert demo logs", slog.String("error", err.Error()))
			}
		},
		nil,
		false,
	).Start(ctx)

	return nil
}

// awaitStore waits until the store answers, and tells whether it is empty.
// The server image has no shell for a compose health check, so the store
// may still be starting when the generator does.
func (g *Generator) awaitStore(ctx context.Context) (bool, error) {
	var empty bool

	err := g.retry(ctx, func() error {
		var err error

		empty, err = g.isEmpty(ctx)

		return err
	})

	return empty, err
}

// retry runs f until it succeeds, its attempts run out or ctx is done.
func (g *Generator) retry(ctx context.Context, f func() error) error {
	return backoff.Retry(
		f,
		backoff.WithContext(
			backoff.WithMaxRetries(backoff.NewConstantBackOff(g.readyDelay), _readyAttempts-1),
			ctx,
		),
	)
}

// isEmpty tells whether the store holds no logs.
func (g *Generator) isEmpty(ctx context.Context) (bool, error) {
	body := strings.NewReader("query=*&limit=1")

	res, err := g.post(ctx, _queryPath, "application/x-www-form-urlencoded", body)
	if err != nil {
		return false, err
	}

	return len(bytes.TrimSpace(res)) == 0, nil
}

// backfill writes backfillDays of history up to now, in batches.
func (g *Generator) backfill(ctx context.Context, now time.Time) error {
	var batch []entry

	for t := now.AddDate(0, 0, -g.backfillDays); t.Before(now); t = t.Add(_backfillStep) {
		batch = append(batch, g.newTick(t, _backfillStep)...)
		if len(batch) < _batchSize {
			continue
		}

		if err := g.ingestBatch(ctx, batch); err != nil {
			return err
		}

		batch = nil
	}

	return g.ingestBatch(ctx, batch)
}

// ingestBatch sends one backfilled batch, and retries it so that a short
// outage leaves no gap in the history.
func (g *Generator) ingestBatch(ctx context.Context, batch []entry) error {
	return g.retry(ctx, func() error {
		return g.ingest(ctx, batch)
	})
}

// newTick generates the logs of one tick, spread over the span before t.
func (g *Generator) newTick(t time.Time, span time.Duration) []entry {
	entries := make([]entry, 1+g.r.Intn(_maxTickLogs))

	for i := range entries {
		at := t.Add(-time.Duration(g.r.Int63n(int64(span))))
		entries[i] = g.newEntry(at)
	}

	return entries
}

// newEntry generates one log written at t.
func (g *Generator) newEntry(t time.Time) entry {
	service := _services[g.r.Intn(len(_services))]
	level := g.newLevel()

	return entry{
		Time:    t.UTC().Format(time.RFC3339Nano),
		Message: g.newMessage(service, level),
		Service: service,
		Env:     _envs[g.r.Intn(len(_envs))],
		Host:    fmt.Sprintf("%s-%d", service, 1+g.r.Intn(_hostCount)),
		Level:   level,
	}
}

// newLevel samples a severity, most logs being info.
func (g *Generator) newLevel() string {
	switch v := g.r.Float64(); {
	case v < _errorShare:
		return "error"
	case v < _warnShare:
		return "warn"
	case v < _debugShare:
		return "debug"
	default:
		return "info"
	}
}

// newMessage writes the log line in the format of the service, so the
// demo holds JSON, logfmt and plain lines.
func (g *Generator) newMessage(service, level string) string {
	var (
		path     = _paths[g.r.Intn(len(_paths))]
		job      = _jobs[g.r.Intn(len(_jobs))]
		status   = g.newStatus(level)
		duration = _minDurationMs + g.r.Intn(_durationSpreadMs)
		traceID  = fmt.Sprintf("%016x", g.r.Uint64())
	)

	switch service {
	case _serviceAPI:
		return fmt.Sprintf(
			`{"level":%q,"msg":"request handled","method":"GET","path":%q,"status":%d,"duration_ms":%d,"trace_id":%q}`,
			level,
			path,
			status,
			duration,
			traceID,
		)
	case _serviceWorker:
		return fmt.Sprintf(
			`level=%s msg="job finished" job=%s attempt=%d duration_ms=%d trace_id=%s`,
			level,
			job,
			1+g.r.Intn(_maxAttempts),
			duration,
			traceID,
		)
	default:
		return fmt.Sprintf(
			`10.0.%d.%d "GET %s HTTP/1.1" %d %d`,
			g.r.Intn(_subnetCount),
			1+g.r.Intn(_subnetHosts),
			path,
			status,
			_minBytes+g.r.Intn(_bytesSpread),
		)
	}
}

// newStatus picks an HTTP status that fits the severity.
func (g *Generator) newStatus(level string) int {
	statuses, ok := _statuses[level]
	if !ok {
		statuses = _okStatuses
	}

	return statuses[g.r.Intn(len(statuses))]
}

// ingest sends the logs to VictoriaLogs as JSON lines.
func (g *Generator) ingest(ctx context.Context, entries []entry) error {
	var body bytes.Buffer

	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)

	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			// NOCOV: an entry holds only strings.
			return fmt.Errorf("encoding log: %w", err)
		}
	}

	_, err := g.post(ctx, _ingestPath, _ingestContentType, &body)

	return err
}

// post sends a request to VictoriaLogs and returns the response body.
func (g *Generator) post(ctx context.Context, path, contentType string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // error provides no meaningful info

	res, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, res)
	}

	return res, nil
}
