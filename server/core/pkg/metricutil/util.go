package metricutil

import "github.com/prometheus/client_golang/prometheus"

// _defaultHistogramBuckets is used when an Options omits Buckets.
// Spans 5ms → 10s at four points; suitable for short ops only.
var _defaultHistogramBuckets = []float64{.005, .05, .5, 10}

// Options are the options for creating a new metric.
type Options struct {
	// Subsystem is the subsystem of the metric.
	// Is is optional.
	Subsystem string

	// Name is the name of the metric.
	Name string

	// Help is the help string of the metric.
	Help string

	// Buckets, when non-nil, overrides the default histogram
	// buckets. Only consulted by NewHistogram and NewHistogramVec.
	// Pass an ascending slice tailored to the metric's expected
	// distribution; otherwise the package default is used.
	Buckets []float64
}

// discarder is a helper structure to discard metrics collection.
type discarder struct{}

// Set implements Gauge.
func (d discarder) Set(_ float64) {}

// Deb implements Gauge.
func (d discarder) Dec() {}

// Sub implements Gauge.
func (d discarder) Sub(_ float64) {}

// Inc implements Counter.
func (d discarder) Inc() {}

// Add implements Counter.
func (d discarder) Add(_ float64) {}

// Observe implements Observer.
func (d discarder) Observe(_ float64) {}

// CounterVec is a counter vector.
type CounterVec interface {
	// With returns a counter with the given labels.
	With(labels prometheus.Labels) Counter
}

// Counter is a counter.
type Counter interface {
	// Inc increments the counter by 1.
	Inc()

	// Add adds the given value to the counter.
	Add(value float64)
}

// HistogramVec is a histogram vector.
type HistogramVec interface {
	// With returns an observer with the given labels.
	With(labels prometheus.Labels) Observer
}

// Observer is an observer.
type Observer interface {
	// Observe observes the given value.
	Observe(value float64)
}

// GaugeVec is a gauge vector.
type GaugeVec interface {
	// With returns a gauge with the given labels.
	With(labels prometheus.Labels) Gauge
}

// Gauge is a gauge data collector.
type Gauge interface {
	// Set should set the Gauge to an arbitrary value.
	Set(value float64)
	// Inc should increment the Gauge by 1. Use Add to increment it by
	// arbitrary values.
	Inc()
	// Dec should decrement the Gauge by 1. Use Sub to decrement it by
	// arbitrary values.
	Dec()
	// Add should add the given value to the Gauge. (The value can be
	// negative, resulting in a decrease of the Gauge.)
	Add(value float64)
	// Sub should subtract the given value from the Gauge. (The value can
	// be negative, resulting in an increase of the Gauge.)
	Sub(value float64)
}

// histogramBuckets returns custom buckets if non-empty, otherwise
// the package default. Bucket boundaries must be strictly
// ascending; the prometheus client validates that itself.
func histogramBuckets(custom []float64) []float64 {
	if len(custom) > 0 {
		return custom
	}

	return _defaultHistogramBuckets
}
