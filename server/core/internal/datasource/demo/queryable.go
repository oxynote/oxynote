package demo

import (
	"context"
	"slices"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
)

// _samplesPerRange is the least number of samples a range selector's
// window holds, so a function reading a window has something to read.
const _samplesPerRange = 4

// _maxSamplesPerSeries caps what one series contributes to a single
// evaluation. Reaching it widens the spacing rather than cutting the
// series short: a coarser line still spans the window that was asked
// for, while a truncated one would simply stop mid-chart.
//
// It has to stay well above the hundred-odd points a time range resolves
// its step to. Widening spaces the samples span/_maxSamplesPerSeries
// apart, and the lookback window a query opens is its own step wide — so
// a cap below that ratio would space samples further apart than the
// window that has to find them, and points of the grid would read empty.
const _maxSamplesPerSeries = 5_000

// selectStep reports how far apart the samples of one Select sit. The
// engine reads on a fixed grid and every point of it has to find a sample
// behind it, so the step the query evaluates at is also the coarsest the
// samples may be spaced; a range selector needs several samples inside
// every window on top of that.
func selectStep(hints *storage.SelectHints, span int64) int64 {
	step := _tickMillis

	if hints != nil {
		if hints.Step > step {
			step = hints.Step
		}

		if hints.Range > 0 {
			if within := hints.Range / _samplesPerRange; within < step {
				step = within
			}
		}
	}

	if step < _tickMillis {
		step = _tickMillis
	}

	if widest := span / _maxSamplesPerSeries; widest > step {
		step = widest
	}

	return step
}

// samplesIn returns one series' readings across the given millisecond
// window, spaced a step apart and aligned to absolute multiples of it so
// the same window always yields the same timestamps. The first sample
// sits at or before the window's start, which is what gives the first
// point of the evaluation grid something to look back at.
func samplesIn(value func(tick int64) float64, start, end, step int64) []chunks.Sample {
	if last := timeAt(latestTick()).UnixMilli(); end > last {
		end = last
	}

	var ss []chunks.Sample

	for ts := start / step * step; ts <= end; ts += step {
		tick := tickAtMillis(ts)
		if tick < 0 {
			continue
		}

		ss = append(ss, floatSample{t: ts, f: value(tick)})
	}

	return ss
}

// queryable serves one client's registry to the promql engine.
type queryable struct {
	// registry is what the evaluation reads out of.
	registry registry
}

// Querier returns a querier scoped to the given millisecond bounds.
func (q queryable) Querier(mint, maxt int64) (storage.Querier, error) {
	return &querier{registry: q.registry, mint: mint, maxt: maxt}, nil
}

// querier answers one promql evaluation out of the registry.
type querier struct {
	// registry is what the evaluation reads out of.
	registry registry

	// mint and maxt are the inclusive millisecond bounds the evaluation
	// reads within.
	mint int64
	maxt int64
}

// Select returns the series the matchers name, sampled across the window
// the hints ask about.
func (q *querier) Select(
	_ context.Context,
	sortSeries bool,
	hints *storage.SelectHints,
	mm ...*labels.Matcher,
) storage.SeriesSet {
	start, end := q.mint, q.maxt

	if hints != nil {
		start, end = hints.Start, hints.End
	}

	step := selectStep(hints, end-start)

	var ss []storage.Series

	q.registry.forEach(mm, func(lbls labels.Labels, value func(tick int64) float64) {
		ss = append(ss, storage.NewListSeries(lbls, samplesIn(value, start, end, step)))
	})

	if sortSeries {
		slices.SortFunc(ss, func(a, b storage.Series) int {
			return labels.Compare(a.Labels(), b.Labels())
		})
	}

	return &seriesSet{series: ss, index: -1}
}

// LabelNames returns every label name the matching series carry, sorted.
func (q *querier) LabelNames(
	_ context.Context,
	_ *storage.LabelHints,
	mm ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.registry.labelNames([][]*labels.Matcher{mm}), nil, nil
}

// LabelValues returns every value the matching series carry for the named
// label, sorted.
func (q *querier) LabelValues(
	_ context.Context,
	name string,
	_ *storage.LabelHints,
	mm ...*labels.Matcher,
) ([]string, annotations.Annotations, error) {
	return q.registry.labelValues(name, [][]*labels.Matcher{mm}), nil, nil
}

// Close releases the querier's resources, of which it holds none.
func (q *querier) Close() error {
	return nil
}

// seriesSet iterates the series one Select produced.
type seriesSet struct {
	// series are the selected series, already sorted when the caller
	// asked for that.
	series []storage.Series

	// index is the position At reports, before Next has run for the
	// first time -1.
	index int
}

// Next advances to the next series and reports whether one is there.
func (ss *seriesSet) Next() bool {
	ss.index++

	return ss.index < len(ss.series)
}

// At returns the series the iteration currently sits on.
func (ss *seriesSet) At() storage.Series {
	return ss.series[ss.index]
}

// Err returns the error iteration failed with, of which synthesized data
// has none.
func (ss *seriesSet) Err() error {
	return nil
}

// Warnings returns the warnings gathered over the set, of which
// synthesized data has none.
func (ss *seriesSet) Warnings() annotations.Annotations {
	return nil
}

// floatSample is one demo reading in the shape the storage layer reads
// samples in. The demo publishes gauges only, so the histogram accessors
// report none.
type floatSample struct {
	// t is the sample's millisecond timestamp.
	t int64

	// f is the value read at that instant.
	f float64
}

// T returns the sample's timestamp in milliseconds.
func (s floatSample) T() int64 {
	return s.t
}

// ST returns the sample's start timestamp, which a synthesized reading
// does not carry.
func (s floatSample) ST() int64 {
	return 0
}

// F returns the sample's value.
func (s floatSample) F() float64 {
	return s.f
}

// H returns the sample's histogram, of which a gauge has none.
func (s floatSample) H() *histogram.Histogram {
	return nil
}

// FH returns the sample's float histogram, of which a gauge has none.
func (s floatSample) FH() *histogram.FloatHistogram {
	return nil
}

// Type reports what kind of value the sample carries.
func (s floatSample) Type() chunkenc.ValueType {
	return chunkenc.ValFloat
}

// Copy returns a deep copy of the sample, which holds no references and
// so is itself.
func (s floatSample) Copy() chunks.Sample {
	return s
}
