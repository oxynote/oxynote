// Package metricutil implements helper structures for metrics collection.
package metricutil

import (
	"context"
	"errors"
	"maps"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// _hostLabel is the label for the hostname.
const _hostLabel = "host"

// Ensure factory implements Factory.
var _ Factory = (*factory)(nil)

// Factory is a metrics factory.
type Factory interface {
	RegistererGatherer

	// CollectRuntimeMetrics collects runtime metrics. It blocks
	// until the context is canceled.
	CollectRuntimeMetrics(ctx context.Context, dur time.Duration)

	// NewHistogram creates a new histogram.
	NewHistogram(opts Options) Observer

	// NewGauge creates a new gauge.
	NewGauge(opts Options) Gauge

	// NewCounterVec creates a new counter vector.
	NewCounterVec(opts Options, labels []string) CounterVec

	// NewHistogramVec creates a new histogram vector.
	NewHistogramVec(opts Options, labels []string) HistogramVec

	// NewGaugeVec creates a new gauge vector.
	NewGaugeVec(opts Options, labels []string) GaugeVec
}

// RegistererGatherer is a registerer and gatherer.
type RegistererGatherer interface {
	prometheus.Registerer
	prometheus.Gatherer
}

// factory is a metrics factory.
type factory struct {
	RegistererGatherer

	// namespace is the namespace of the metrics.
	namespace string

	// host is the hostname.
	host string
}

// NewFactory creates a new metrics factory. If registerer is nil, collectors
// are no-op.
func NewFactory(namespace string, rg RegistererGatherer, opts ...Option) Factory {
	host, err := os.Hostname()
	if err != nil {
		// NOCOV: this should never happen
		host = "unknown"
	}

	f := &factory{
		RegistererGatherer: rg,
		namespace:          namespace,
		host:               host,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

// Option modifies the factory during construction.
type Option func(f *factory)

// WithCustomHost sets a custom hostname for the metrics.
func WithCustomHost(host string) Option {
	return func(f *factory) {
		f.host = host
	}
}

// registerOrExisting registers the collector and returns it, or returns the
// equivalent collector the registry already holds. Anything else is a defect
// in the metric declaration itself, which cannot be recovered from.
func registerOrExisting[T prometheus.Collector](rg prometheus.Registerer, c T) T {
	err := rg.Register(c)
	if err == nil {
		return c
	}

	are, ok := errors.AsType[prometheus.AlreadyRegisteredError](err)
	if !ok {
		// NOCOV: this should never happen
		panic(err)
	}

	ec, ok := are.ExistingCollector.(T)
	if !ok {
		// NOCOV: this should never happen
		panic("existing collector has a different type")
	}

	return ec
}

// NewHistogram creates a new histogram.
func (f *factory) NewHistogram(opts Options) Observer {
	if f.RegistererGatherer == nil {
		return discarder{}
	}

	hv := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
		Buckets:   histogramBuckets(opts.Buckets),
	}, []string{_hostLabel})

	hv = registerOrExisting(f, hv)

	return hv.With(
		prometheus.Labels{
			_hostLabel: f.host,
		},
	)
}

// NewGauge creates a new gauge.
func (f *factory) NewGauge(opts Options) Gauge {
	if f.RegistererGatherer == nil {
		return discarder{}
	}

	gv := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
	}, []string{_hostLabel})

	gv = registerOrExisting(f, gv)

	return gv.With(
		prometheus.Labels{
			_hostLabel: f.host,
		},
	)
}

// NewCounterVec creates a new counter vector.
func (f *factory) NewCounterVec(opts Options, labels []string) CounterVec {
	cv := &counterVec{
		host: f.host,
	}

	if f.RegistererGatherer == nil {
		return cv
	}

	cv.counter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
	}, append(labels, _hostLabel))

	cv.counter = registerOrExisting(f, cv.counter)

	return cv
}

// NewHistogramVec creates a new histogram vector.
func (f *factory) NewHistogramVec(opts Options, labels []string) HistogramVec {
	hv := &histogramVec{
		host: f.host,
	}

	if f.RegistererGatherer == nil {
		return hv
	}

	hv.histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
		Buckets:   histogramBuckets(opts.Buckets),
	}, append(labels, _hostLabel))

	hv.histogram = registerOrExisting(f, hv.histogram)

	return hv
}

// NewGaugeVec creates a new gauge vector.
func (f *factory) NewGaugeVec(opts Options, labels []string) GaugeVec {
	gv := &gaugeVec{
		host: f.host,
	}

	if f.RegistererGatherer == nil {
		return gv
	}

	gv.gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
	}, append(labels, _hostLabel))

	gv.gauge = registerOrExisting(f, gv.gauge)

	return gv
}

// counterVec is a counter vector.
type counterVec struct {
	counter *prometheus.CounterVec
	host    string
}

// With implements CounterVec.
func (cv counterVec) With(labels prometheus.Labels) Counter {
	if cv.counter == nil {
		return discarder{}
	}

	return cv.counter.With(withHost(labels, cv.host))
}

// histogramVec is a histogram vector.
type histogramVec struct {
	histogram *prometheus.HistogramVec
	host      string
}

// With implements HistogramVec.
func (hv histogramVec) With(labels prometheus.Labels) Observer {
	if hv.histogram == nil {
		return discarder{}
	}

	return hv.histogram.With(withHost(labels, hv.host))
}

// gaugeVec is a gauge vector.
type gaugeVec struct {
	gauge *prometheus.GaugeVec
	host  string
}

// With implements GaugeVec.
func (gv gaugeVec) With(labels prometheus.Labels) Gauge {
	if gv.gauge == nil {
		return discarder{}
	}

	return gv.gauge.With(withHost(labels, gv.host))
}

// withHost returns a copy of the labels carrying the host label, leaving the
// caller's map untouched so it stays reusable across metric families.
func withHost(labels prometheus.Labels, host string) prometheus.Labels {
	out := maps.Clone(labels)
	if out == nil {
		out = prometheus.Labels{}
	}

	out[_hostLabel] = host

	return out
}
