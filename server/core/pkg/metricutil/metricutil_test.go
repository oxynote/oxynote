package metricutil

import (
	"os"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewFactory(t *testing.T) {
	host, err := os.Hostname()
	require.NoError(t, err)

	cc := map[string]struct {
		Opts []Option
		Host string
	}{
		"Machine hostname": {
			Host: host,
		},
		"Custom host": {
			Opts: []Option{WithCustomHost("custom")},
			Host: "custom",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()

			f := NewFactory("test", reg, c.Opts...)
			assert.Equal(t, &factory{
				RegistererGatherer: reg,
				namespace:          "test",
				host:               c.Host,
			}, f)
		})
	}
}

func Test_WithCustomHost(t *testing.T) {
	t.Parallel()

	f := &factory{}

	WithCustomHost("custom")(f)
	assert.Equal(t, "custom", f.host)
}

func Test_registerOrExisting(t *testing.T) {
	t.Parallel()

	opts := prometheus.GaugeOpts{
		Name: "test_registered",
		Help: "test help",
	}

	cc := map[string]struct {
		Registered bool
	}{
		"Fresh collector": {},
		"Equivalent collector already registered": {
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()
			gv := prometheus.NewGaugeVec(opts, []string{_hostLabel})
			inp := gv

			if c.Registered {
				require.NoError(t, reg.Register(gv))

				inp = prometheus.NewGaugeVec(opts, []string{_hostLabel})
			}

			assert.Same(t, gv, registerOrExisting(reg, inp))
			assert.True(t, reg.Unregister(gv), "collector must be registered")
		})
	}
}

func Test_factory_NewHistogram(t *testing.T) {
	cc := map[string]struct {
		Registry   RegistererGatherer
		Registered bool
		Discarded  bool
	}{
		"Registerer is nil": {
			Discarded: true,
		},
		"New histogram": {
			Registry: prometheus.NewRegistry(),
		},
		"Histogram already registered": {
			Registry:   prometheus.NewRegistry(),
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := factory{RegistererGatherer: c.Registry}
			opts := Options{
				Name: "test_duration",
				Help: "test help",
			}

			var existing Observer
			if c.Registered {
				existing = f.NewHistogram(opts)
			}

			cv := f.NewHistogram(opts)
			if c.Discarded {
				assert.Equal(t, discarder{}, cv.(discarder))
				return
			}

			assert.Implements(t, (*prometheus.Histogram)(nil), cv)

			if c.Registered {
				assert.Same(t, existing, cv)
			}
		})
	}
}

func Test_factory_NewGauge(t *testing.T) {
	cc := map[string]struct {
		Registry   RegistererGatherer
		Registered bool
		Discarded  bool
	}{
		"Registerer is nil": {
			Discarded: true,
		},
		"New gauge": {
			Registry: prometheus.NewRegistry(),
		},
		"Gauge already registered": {
			Registry:   prometheus.NewRegistry(),
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := factory{RegistererGatherer: c.Registry}
			opts := Options{
				Name: "test_duration",
				Help: "test help",
			}

			var existing Gauge
			if c.Registered {
				existing = f.NewGauge(opts)
			}

			cv := f.NewGauge(opts)
			if c.Discarded {
				assert.Equal(t, discarder{}, cv.(discarder))
				return
			}

			assert.Implements(t, (*prometheus.Gauge)(nil), cv)

			if c.Registered {
				assert.Same(t, existing, cv)
			}
		})
	}
}

func Test_factory_NewCounterVec(t *testing.T) {
	cc := map[string]struct {
		Registry   RegistererGatherer
		Registered bool
		Discarded  bool
	}{
		"Registerer is nil": {
			Discarded: true,
		},
		"New counter vector": {
			Registry: prometheus.NewRegistry(),
		},
		"Counter vector already registered": {
			Registry:   prometheus.NewRegistry(),
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := factory{RegistererGatherer: c.Registry}
			opts := Options{
				Name: "test_total",
				Help: "test help",
			}

			var existing CounterVec
			if c.Registered {
				existing = f.NewCounterVec(opts, []string{"foo"})
			}

			cv := f.NewCounterVec(opts, []string{"foo"})
			if c.Discarded {
				assert.Equal(t, &counterVec{}, cv.(*counterVec))
				return
			}

			assert.IsType(t, &prometheus.CounterVec{}, cv.(*counterVec).counter)

			if c.Registered {
				assert.Same(t, existing.(*counterVec).counter, cv.(*counterVec).counter)
			}
		})
	}
}

func Test_factory_NewHistogramVec(t *testing.T) {
	cc := map[string]struct {
		Registry   RegistererGatherer
		Registered bool
		Discarded  bool
	}{
		"Registerer is nil": {
			Discarded: true,
		},
		"New histogram vector": {
			Registry: prometheus.NewRegistry(),
		},
		"Histogram vector already registered": {
			Registry:   prometheus.NewRegistry(),
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := factory{RegistererGatherer: c.Registry}
			opts := Options{
				Name: "test_duration",
				Help: "test help",
			}

			var existing HistogramVec
			if c.Registered {
				existing = f.NewHistogramVec(opts, []string{"foo"})
			}

			cv := f.NewHistogramVec(opts, []string{"foo"})
			if c.Discarded {
				assert.Equal(t, &histogramVec{}, cv.(*histogramVec))
				return
			}

			assert.IsType(t, &prometheus.HistogramVec{}, cv.(*histogramVec).histogram)

			if c.Registered {
				assert.Same(t, existing.(*histogramVec).histogram, cv.(*histogramVec).histogram)
			}
		})
	}
}

func Test_factory_NewGaugeVec(t *testing.T) {
	cc := map[string]struct {
		Registry   RegistererGatherer
		Registered bool
		Discarded  bool
	}{
		"Registerer is nil": {
			Discarded: true,
		},
		"New gauge vector": {
			Registry: prometheus.NewRegistry(),
		},
		"Gauge vector already registered": {
			Registry:   prometheus.NewRegistry(),
			Registered: true,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			f := factory{RegistererGatherer: c.Registry}
			opts := Options{
				Name: "test_duration",
				Help: "test help",
			}

			var existing GaugeVec
			if c.Registered {
				existing = f.NewGaugeVec(opts, []string{"foo"})
			}

			gv := f.NewGaugeVec(opts, []string{"foo"})
			if c.Discarded {
				assert.Equal(t, &gaugeVec{}, gv.(*gaugeVec))
				return
			}

			assert.IsType(t, &prometheus.GaugeVec{}, gv.(*gaugeVec).gauge)

			if c.Registered {
				assert.Same(t, existing.(*gaugeVec).gauge, gv.(*gaugeVec).gauge)
			}
		})
	}
}

func Test_counterVec_With(t *testing.T) {
	cc := map[string]struct {
		Counter   *prometheus.CounterVec
		Discarded bool
	}{
		"Counter is nil": {
			Discarded: true,
		},
		"Counter is set": {
			Counter: prometheus.NewCounterVec(prometheus.CounterOpts{
				Name: "test_total",
				Help: "test help",
			}, []string{"foo", _hostLabel}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			labels := prometheus.Labels{"foo": "bar"}

			cv := counterVec{counter: c.Counter}
			got := cv.With(labels)
			assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")

			if c.Discarded {
				assert.IsType(t, discarder{}, got)
				return
			}

			_, ok := got.(prometheus.Counter)
			assert.True(t, ok)
		})
	}
}

func Test_histogramVec_With(t *testing.T) {
	cc := map[string]struct {
		Histogram *prometheus.HistogramVec
		Discarded bool
	}{
		"Histogram is nil": {
			Discarded: true,
		},
		"Histogram is set": {
			Histogram: prometheus.NewHistogramVec(prometheus.HistogramOpts{
				Name: "test_duration",
				Help: "test help",
			}, []string{"foo", _hostLabel}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			labels := prometheus.Labels{"foo": "bar"}

			hv := histogramVec{histogram: c.Histogram}
			got := hv.With(labels)
			assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")

			if c.Discarded {
				assert.IsType(t, discarder{}, got)
				return
			}

			_, ok := got.(prometheus.Histogram)
			assert.True(t, ok)
		})
	}
}

func Test_gaugeVec_With(t *testing.T) {
	cc := map[string]struct {
		Gauge     *prometheus.GaugeVec
		Discarded bool
	}{
		"Gauge is nil": {
			Discarded: true,
		},
		"Gauge is set": {
			Gauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: "test_duration",
				Help: "test help",
			}, []string{"foo", _hostLabel}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			labels := prometheus.Labels{"foo": "bar"}

			gv := gaugeVec{gauge: c.Gauge}
			got := gv.With(labels)
			assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")

			if c.Discarded {
				assert.IsType(t, discarder{}, got)
				return
			}

			_, ok := got.(prometheus.Gauge)
			assert.True(t, ok)
		})
	}
}
