package server

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oxynote/purse/util/metricutil"
	"github.com/oxynote/purse/util/timeutil"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// _pathLabel specifies a constant metrics path label.
	_pathLabel = "path"

	// _methodLabel specifies a constant metrics method label.
	_methodLabel = "method"

	// _subsystemServer represents the server subsystem.
	_subsystemServer = "server"

	// _topicTemplateLabel specifies a constant metrics topic template label.
	_topicTemplateLabel = "topic_template"

	// _durationThreshold specifies a duration threshold for metrics.
	_durationThreshold = time.Millisecond * 200
)

// wrapMetrics wraps handler to retrieve metrics information.
func wrapMetrics(fc metricutil.Factory) func(next http.Handler) http.Handler {
	requestsCounter := fc.NewCounterVec(
		metricutil.Options{
			Subsystem: _subsystemServer,
			Name:      "http_requests_total",
			Help:      "Tracks the number of HTTP requests.",
		},
		[]string{
			_pathLabel,
			_methodLabel,
		},
	)

	requestsDuration := fc.NewHistogramVec(
		metricutil.Options{
			Subsystem: _subsystemServer,
			Name:      "http_request_duration_seconds",
			Help:      "Tracks the latencies for HTTP requests.",
		},
		[]string{
			_pathLabel,
			_methodLabel,
		},
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tstamp := timeutil.Now()

			next.ServeHTTP(w, r)

			path := chi.RouteContext(r.Context()).RoutePattern()

			requestsCounter.With(prometheus.Labels{
				_pathLabel:   path,
				_methodLabel: r.Method,
			}).Inc()

			if time.Since(tstamp) > _durationThreshold {
				requestsDuration.With(prometheus.Labels{
					_pathLabel:   path,
					_methodLabel: r.Method,
				}).Observe(time.Since(tstamp).Seconds())
			}
		})
	}
}

// wsMetricsBinder returns a function that binds WebSocket topic and
// additionally wraps it into a metric counter.
func wsMetricsBinder(r *wsserver.Router, fc metricutil.Factory) func(string, wsserver.BinderFunc, ...func(context.Context, string) bool) {
	messagesCounter := fc.NewCounterVec(
		metricutil.Options{
			Subsystem: _subsystemServer,
			Name:      "ws_messages_total",
			Help:      "Tracks the number of WebSocket messages.",
		},
		[]string{
			_topicTemplateLabel,
		},
	)

	return func(topic string, binderFn wsserver.BinderFunc, middlewares ...func(context.Context, string) bool) {
		r.BindFunc(topic, binderFn, append(middlewares, func(_ context.Context, _ string) bool {
			messagesCounter.With(prometheus.Labels{
				_topicTemplateLabel: topic,
			}).Inc()

			return true
		})...)
	}
}
