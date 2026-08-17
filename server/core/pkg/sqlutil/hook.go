package sqlutil

import (
	"context"
	"regexp"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/client_golang/prometheus"
)

type contextKey int

const timeKey contextKey = iota + 1

const (
	// _queryLabel specifies a constant raw query label.
	_queryLabel = "query"
)

// Hooks specifies custom database querying hooks and implements
// sqlhook.hooks interface.
type Hooks struct {
	// dbName specifies a string that is used in all
	// metrics as a subsystem.
	dbName string

	// durationThreshold specifies a duration threshold which must be
	// exceeded in order to create a metric log.
	durationThreshold time.Duration

	// errorHandler specifies a function for handling errors.
	errorHandler func(error) error

	queriesCounter  metricutil.CounterVec
	queriesDuration metricutil.HistogramVec
}

// HookOption specifies a hook option.
type HookOption func(*Hooks)

// WithHookDatabaseName specifies a database prefix.
func WithHookDatabaseName(prefix string) HookOption {
	return func(h *Hooks) {
		h.dbName = prefix
	}
}

// WithHookDurationThreshold specifies a duration threshold option.
func WithHookDurationThreshold(durationThreshold time.Duration) HookOption {
	return func(h *Hooks) {
		h.durationThreshold = durationThreshold
	}
}

// WithHookErrorHandler specifies an error handling function.
func WithHookErrorHandler(hdl func(error) error) HookOption {
	return func(h *Hooks) {
		h.errorHandler = hdl
	}
}

// NewHooks creates a new hooks instance.
func NewHooks(
	fc metricutil.Factory,
	opts ...HookOption,
) *Hooks {
	h := &Hooks{}

	for _, opt := range opts {
		opt(h)
	}

	h.queriesCounter = fc.NewCounterVec(
		metricutil.Options{
			Subsystem: h.dbName, //nolint:promlinter // cannot validate a variable
			Name:      "database_queries_total",
			Help:      "Tracks the number of database queries.",
		},
		[]string{
			_queryLabel,
		},
	)

	h.queriesDuration = fc.NewHistogramVec(
		metricutil.Options{
			Subsystem: h.dbName, //nolint:promlinter // cannot validate a variable
			Name:      "database_queries_duration_seconds",
			Help:      "Tracks the durations of database queries.",
		},
		[]string{
			_queryLabel,
		},
	)

	return h
}

// Before executes before the query.
func (h *Hooks) Before(
	ctx context.Context,
	query string,
	_ ...any,
) (context.Context, error) {
	h.queriesCounter.With(prometheus.Labels{
		_queryLabel: normalizeQuery(query),
	}).Inc()

	return context.WithValue(ctx, timeKey, timeutil.Now()), nil
}

// OnError executes on error.
func (h *Hooks) OnError(_ context.Context, err error, _ string, _ ...any) error {
	if h.errorHandler != nil {
		return h.errorHandler(err)
	}

	return err
}

// After executes after the query.
func (h *Hooks) After(
	ctx context.Context,
	query string,
	_ ...any,
) (context.Context, error) {
	startedAt, ok := ctx.Value(timeKey).(time.Time)
	if ok && time.Since(startedAt) > h.durationThreshold {
		h.queriesDuration.With(prometheus.Labels{
			_queryLabel: normalizeQuery(query),
		}).Observe(time.Since(startedAt).Seconds())
	}

	return ctx, nil
}

// _placeholderListRe matches a run of two or more positional placeholders,
// which is the one part of a squirrel query that varies with the data.
var _placeholderListRe = regexp.MustCompile(`\$\d+(\s*,\s*\$\d+)+`)

// normalizeQuery collapses the parts of a query that vary per call, so an IN
// clause over a slice mints one label value instead of one per slice length.
// Every squirrel query is otherwise constant-shaped.
func normalizeQuery(query string) string {
	return _placeholderListRe.ReplaceAllString(query, "$$?")
}
