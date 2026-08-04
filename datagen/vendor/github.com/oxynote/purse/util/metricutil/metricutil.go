// Package metricutil implements helper structures for metrics collection.
package metricutil

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// _hostLabel is the label for the hostname.
const _hostLabel = "host"

// Ensure factory implements Factory.
var _ Factory = (*factory)(nil)

// Factory is a metrics factory.
//
//go:generate ../../scripts/codegen/mock Factory
type Factory interface {
	RegistererGatherer

	// CollectRuntimeMetrics collects runtime metrics. It blocks
	// until the context is canceled.
	CollectRuntimeMetrics(ctx context.Context, dur time.Duration)

	// NewCounter creates a new counter.
	NewCounter(opts Options) Counter

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

// NewCounter creates a new counter.
func (f *factory) NewCounter(opts Options) Counter {
	if f.RegistererGatherer == nil {
		return discarder{}
	}

	cv := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: f.namespace, //nolint:promlinter // cannot validate a variable
		Subsystem: opts.Subsystem,
		Name:      opts.Name,
		Help:      opts.Help,
	}, []string{_hostLabel})

	if err := f.Register(cv); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		ncv, ok := are.ExistingCollector.(*prometheus.CounterVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a counter")
		}

		cv = ncv
	}

	return cv.With(
		prometheus.Labels{
			_hostLabel: f.host,
		},
	)
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
		Buckets:   []float64{.005, .05, .5, 10},
	}, []string{_hostLabel})

	if err := f.Register(hv); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		nh, ok := are.ExistingCollector.(*prometheus.HistogramVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a histogram")
		}

		hv = nh
	}

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

	if err := f.Register(gv); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		ngv, ok := are.ExistingCollector.(*prometheus.GaugeVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a gauge")
		}

		gv = ngv
	}

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

	if err := f.Register(cv.counter); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		ncv, ok := are.ExistingCollector.(*prometheus.CounterVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a counter vector")
		}

		cv.counter = ncv
	}

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
		Buckets:   []float64{.005, .05, .5, 10},
	}, append(labels, _hostLabel))

	if err := f.Register(hv.histogram); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		nhv, ok := are.ExistingCollector.(*prometheus.HistogramVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a histogram vector")
		}

		hv.histogram = nhv
	}

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

	if err := f.Register(gv.gauge); err != nil {
		are := &prometheus.AlreadyRegisteredError{}
		if !errors.As(err, are) {
			// NOCOV: this should never happen
			panic(err)
		}

		ngv, ok := are.ExistingCollector.(*prometheus.GaugeVec)
		if !ok {
			// NOCOV: this should never happen
			panic("existing collector is not a gauge vector")
		}

		gv.gauge = ngv
	}

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

	labels[_hostLabel] = cv.host

	return cv.counter.With(labels)
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

	labels[_hostLabel] = hv.host

	return hv.histogram.With(labels)
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

	labels[_hostLabel] = gv.host

	return gv.gauge.With(labels)
}

// Options are options for creating metrics.
type Option func(f *factory)

// WithCustomHost sets a custom hostname for the metrics.
func WithCustomHost(host string) Option {
	return func(f *factory) {
		f.host = host
	}
}
