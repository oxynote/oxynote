package metricutil

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewFactory(t *testing.T) {
	reg := prometheus.NewRegistry()
	f := NewFactory("test", reg)
	assert.Equal(t, reg, f.(*factory).RegistererGatherer)
	assert.Equal(t, "test", f.(*factory).namespace)

	f = NewFactory("test", reg, WithCustomHost("custom"))
	assert.Equal(t, "custom", f.(*factory).host)
}

func Test_WithCustomHost(t *testing.T) {
	t.Parallel()

	f := &factory{}

	WithCustomHost("custom")(f)
	assert.Equal(t, "custom", f.host)
}

func Test_registerOrExisting(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()

	opts := prometheus.GaugeOpts{
		Name: "test_registered",
		Help: "test help",
	}

	gv := prometheus.NewGaugeVec(opts, []string{_hostLabel})

	// a fresh collector is registered and handed back as it is.
	assert.Same(t, gv, registerOrExisting(reg, gv))

	// an equivalent collector is replaced by the registered one.
	assert.Same(t, gv, registerOrExisting(reg, prometheus.NewGaugeVec(opts, []string{_hostLabel})))
}

func Test_factory_NewHistogram(t *testing.T) {
	var f factory

	cv := f.NewHistogram(Options{})
	assert.Equal(t, discarder{}, cv.(discarder))

	f.RegistererGatherer = prometheus.NewRegistry()

	cv = f.NewHistogram(Options{
		Name: "test_duration",
		Help: "test help",
	})

	assert.Implements(t, (*prometheus.Histogram)(nil), cv)

	ncv := f.NewHistogram(Options{
		Name: "test_duration",
		Help: "test help",
	})

	assert.Implements(t, (*prometheus.Histogram)(nil), cv)
	assert.Same(t, cv, ncv)
}

func Test_factory_NewGauge(t *testing.T) {
	var f factory

	cv := f.NewGauge(Options{})
	assert.Equal(t, discarder{}, cv.(discarder))

	f.RegistererGatherer = prometheus.NewRegistry()

	cv = f.NewGauge(Options{
		Name: "test_duration",
		Help: "test help",
	})

	assert.Implements(t, (*prometheus.Gauge)(nil), cv)

	ncv := f.NewGauge(Options{
		Name: "test_duration",
		Help: "test help",
	})

	assert.Implements(t, (*prometheus.Gauge)(nil), cv)
	assert.Same(t, cv, ncv)
}

func Test_factory_NewCounterVec(t *testing.T) {
	var f factory

	cv := f.NewCounterVec(Options{}, []string{"foo"})
	assert.Equal(t, &counterVec{}, cv.(*counterVec))

	f.RegistererGatherer = prometheus.NewRegistry()

	cv = f.NewCounterVec(Options{
		Name: "test_total",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.CounterVec{}, cv.(*counterVec).counter)

	ncv := f.NewCounterVec(Options{
		Name: "test_total",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.CounterVec{}, ncv.(*counterVec).counter)
	assert.Same(t, cv.(*counterVec).counter, ncv.(*counterVec).counter)
}

func Test_factory_NewHistogramVec(t *testing.T) {
	var f factory

	cv := f.NewHistogramVec(Options{}, []string{"foo"})
	assert.Equal(t, &histogramVec{}, cv.(*histogramVec))

	f.RegistererGatherer = prometheus.NewRegistry()

	cv = f.NewHistogramVec(Options{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.HistogramVec{}, cv.(*histogramVec).histogram)

	ncv := f.NewHistogramVec(Options{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.HistogramVec{}, ncv.(*histogramVec).histogram)
	assert.Same(t, cv.(*histogramVec).histogram, ncv.(*histogramVec).histogram)
}

func Test_factory_NewGaugeVec(t *testing.T) {
	var f factory

	gv := f.NewGaugeVec(Options{}, []string{"foo"})
	assert.Equal(t, &gaugeVec{}, gv.(*gaugeVec))

	f.RegistererGatherer = prometheus.NewRegistry()

	gv = f.NewGaugeVec(Options{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.GaugeVec{}, gv.(*gaugeVec).gauge)

	ngv := f.NewGaugeVec(Options{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo"})

	assert.IsType(t, &prometheus.GaugeVec{}, ngv.(*gaugeVec).gauge)
	assert.Same(t, gv.(*gaugeVec).gauge, ngv.(*gaugeVec).gauge)
}

func Test_counterVec_With(t *testing.T) {
	var cv counterVec

	c := cv.With(prometheus.Labels{"foo": "bar"})
	assert.IsType(t, discarder{}, c)

	cv.counter = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "test_total",
		Help: "test help",
	}, []string{"foo", _hostLabel})

	c = cv.With(prometheus.Labels{"foo": "bar"})
	_, ok := c.(prometheus.Counter)
	assert.True(t, ok)

	labels := prometheus.Labels{"foo": "bar"}

	cv.With(labels)
	assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")
}

func Test_histogramVec_With(t *testing.T) {
	var hv histogramVec

	h := hv.With(prometheus.Labels{"foo": "bar"})
	assert.IsType(t, discarder{}, h)

	hv.histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo", _hostLabel})

	h = hv.With(prometheus.Labels{"foo": "bar"})
	_, ok := h.(prometheus.Histogram)
	assert.True(t, ok)

	labels := prometheus.Labels{"foo": "bar"}

	hv.With(labels)
	assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")
}

func Test_gaugeVec_With(t *testing.T) {
	var gv gaugeVec

	g := gv.With(prometheus.Labels{"foo": "bar"})
	assert.IsType(t, discarder{}, g)

	gv.gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "test_duration",
		Help: "test help",
	}, []string{"foo", _hostLabel})

	g = gv.With(prometheus.Labels{"foo": "bar"})
	_, ok := g.(prometheus.Gauge)
	assert.True(t, ok)

	labels := prometheus.Labels{"foo": "bar"}

	gv.With(labels)
	assert.Equal(t, prometheus.Labels{"foo": "bar"}, labels, "caller labels must not be mutated")
}

func Test_discarder_Set(_ *testing.T) {
	(&discarder{}).Set(123)
}

func Test_discarder_Dec(_ *testing.T) {
	(&discarder{}).Dec()
}

func Test_discarder_Sub(_ *testing.T) {
	(&discarder{}).Sub(123)
}

func Test_discarder_Inc(_ *testing.T) {
	(&discarder{}).Inc()
}

func Test_discarder_Add(_ *testing.T) {
	(&discarder{}).Add(5)
}

func Test_discarder_Observe(_ *testing.T) {
	(&discarder{}).Observe(1.0)
}
