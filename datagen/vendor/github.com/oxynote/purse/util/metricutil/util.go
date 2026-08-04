package metricutil

import "github.com/prometheus/client_golang/prometheus"

// Options are the options for creating a new metric.
type Options struct {
	// Subsystem is the subsystem of the metric.
	// Is is optional.
	Subsystem string

	// Name is the name of the metric.
	Name string

	// Help is the help string of the metric.
	Help string
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
//
//go:generate ../../scripts/codegen/mock CounterVec
type CounterVec interface {
	// With returns a counter with the given labels.
	With(labels prometheus.Labels) Counter
}

// Counter is a counter.
//
//go:generate ../../scripts/codegen/mock Counter
type Counter interface {
	// Inc increments the counter by 1.
	Inc()

	// Add adds the given value to the counter.
	Add(float64)
}

// HistogramVec is a histogram vector.
//
//go:generate ../../scripts/codegen/mock HistogramVec
type HistogramVec interface {
	// With returns an observer with the given labels.
	With(labels prometheus.Labels) Observer
}

// Observer is an observer.
//
//go:generate ../../scripts/codegen/mock Observer
type Observer interface {
	// Observe observes the given value.
	Observe(float64)
}

// GaugeVec is a gauge vector.
//
//go:generate ../../scripts/codegen/mock GaugeVec
type GaugeVec interface {
	// With returns an observer with the given labels.
	With(labels prometheus.Labels) Gauge
}

// Gauge is a gauge data collector.
//
//go:generate ../../scripts/codegen/mock Gauge
type Gauge interface {
	// Set should set the Gauge to an arbitrary value.
	Set(float64)
	// Inc should increment the Gauge by 1. Use Add to increment it by
	// arbitrary values.
	Inc()
	// Dec should decrement the Gauge by 1. Use Sub to decrement it by
	// arbitrary values.
	Dec()
	// Add should add the given value to the Gauge. (The value can be
	// negative, resulting in a decrease of the Gauge.)
	Add(float64)
	// Sub should subtract the given value from the Gauge. (The value can
	// be negative, resulting in an increase of the Gauge.)
	Sub(float64)
}
