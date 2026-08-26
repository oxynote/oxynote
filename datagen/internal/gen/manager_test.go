package gen

import (
	"context"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_newContinuous(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Params mockmetrics.GaugeParams
		Result float64
	}{
		"Start within the bounds is kept": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 40},
			Result: 40,
		},
		"Start below the lower bound is raised": {
			Params: mockmetrics.GaugeParams{Min: 10, Max: 100, Start: 5},
			Result: 10,
		},
		"Start above the upper bound is lowered": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 50, Start: 90},
			Result: 50,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := newContinuous(c.Params)
			require.NotNil(t, m)

			assert.InDelta(t, c.Result, m.value, 1e-9)
			assert.Equal(t, c.Params, m.p)
		})
	}
}

func Test_continuousMetric_tick(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Params mockmetrics.GaugeParams

		// Result is the value the single tick must produce exactly, used
		// where the parameters leave no randomness in play.
		Result *float64

		// Ticks is how many times to advance when the case asserts over a
		// run rather than a single step.
		Ticks int
	}{
		"Static parameters hold the value still": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 50},
			Result: new(50.0),
		},
		"Drift moves the value by a fixed step": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 50, DriftPerStep: 2},
			Result: new(52.0),
		},
		"Mean reversion pulls the value toward the target": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 50, Target: 100, MeanReversion: 0.5},
			Result: new(75.0),
		},
		"Drift is held at the upper bound": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 50, Start: 50, DriftPerStep: 10},
			Result: new(50.0),
		},
		"Drift is held at the lower bound": {
			Params: mockmetrics.GaugeParams{Min: 10, Max: 100, Start: 10, DriftPerStep: -10},
			Result: new(10.0),
		},
		"Noise keeps the value inside the bounds": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 50, NoiseStdDev: 40},
			Ticks:  1_000,
		},
		"A certain spike keeps the value inside the bounds": {
			Params: mockmetrics.GaugeParams{Min: 0, Max: 100, Start: 50, SpikeChance: 1, SpikeStdDev: 80},
			Ticks:  1_000,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				m = newContinuous(c.Params)
				r = mockmetrics.NewRand(1)
			)

			if c.Result != nil {
				v := m.tick(r)

				assert.InDelta(t, *c.Result, v, 1e-9)

				// the returned value is the state the next tick builds on.
				assert.InDelta(t, v, m.value, 1e-9)

				return
			}

			for range c.Ticks {
				v := m.tick(r)

				assert.GreaterOrEqual(t, v, c.Params.Min)
				assert.LessOrEqual(t, v, c.Params.Max)
				assert.InDelta(t, v, m.value, 1e-9)
			}
		})
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	log := slog.New(slog.DiscardHandler)

	m := NewManager(newFactory(), log)
	require.NotNil(t, m)

	assert.NotNil(t, m.r)
	assert.Equal(t, log, m.log)
	assert.Equal(t, _genInterval, m.interval)

	// every drifting state and every gauge has to be wired, which the
	// compiler cannot check for a struct built field by field.
	v := reflect.ValueOf(*m)

	for i := range v.NumField() {
		name := v.Type().Field(i).Name

		switch v.Type().Field(i).Type.String() {
		case "*gen.continuousMetric", "metricutil.GaugeVec":
			assert.False(t, v.Field(i).IsNil(), name)
		}
	}
}

func Test_Manager_Run(t *testing.T) {
	t.Parallel()

	fc := newFactory()

	m := NewManager(fc, slog.New(slog.DiscardHandler))
	m.interval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())

	stopCh := make(chan struct{})

	go func() {
		defer close(stopCh)

		m.Run(ctx)
	}()

	// the first generation is immediate, so the gauges carry values well
	// before the hour-long interval could elapse.
	require.Eventually(t, func() bool {
		ff, err := fc.Gather()

		return err == nil && len(ff) > 0
	}, 5*time.Second, 10*time.Millisecond)

	cancel()

	<-stopCh
}

func Test_Manager_setCounts(t *testing.T) {
	t.Parallel()

	fc := newFactory()
	vec := fc.NewGaugeVec(metricutil.Options{Name: "test_counts", Help: "Tracks nothing."}, []string{"kind"})

	m := &Manager{r: mockmetrics.NewRand(1)}
	m.setCounts(vec, "kind", []countBucket{
		{label: "first", mean: 1_000, relStdDev: 0.1},
		{label: "second", mean: 2_000, relStdDev: 0.1},
	})

	values := gathered(t, fc, "engineering_test_counts")
	require.Len(t, values, 2)

	assert.InDelta(t, 1_000, values["first"], 500)
	assert.InDelta(t, 2_000, values["second"], 1_000)
}

func Test_Manager_generateData(t *testing.T) {
	t.Parallel()

	fc := newFactory()

	m := NewManager(fc, slog.New(slog.DiscardHandler))
	m.generateData()

	// every declared gauge has to report at least one labelled value.
	cc := map[string]int{
		"engineering_meetings_could_be_doc_count":        4,
		"engineering_circle_back_count":                  3,
		"engineering_friday_deploys_count":               2,
		"engineering_feature_flag_graveyard_size":        3,
		"engineering_pizza_fridays_next_30_days":         3,
		"engineering_deploy_confidence_index":            3,
		"engineering_ship_it_approvals_after_5pm_count":  3,
		"engineering_prs_merged_friday_afternoon_count":  3,
		"engineering_temporary_fix_age_days":             4,
		"engineering_stale_branches_total":               3,
		"engineering_pr_time_to_first_review_minutes":    1,
		"engineering_hotfixes_per_release":               3,
		"engineering_secrets_detected_commits_count":     3,
		"engineering_works_on_my_machine_rate":           3,
		"engineering_quick_sync_meetings_same_day_count": 3,
	}

	// these subtests read the registry the rest of this function goes on to
	// write to, so they stay sequential rather than deferring past it.
	for name, count := range cc {
		t.Run(name, func(t *testing.T) {
			assert.Len(t, gathered(t, fc, name), count)
		})
	}

	// friday deploys are derived rather than sampled, so successes can
	// never outnumber attempts.
	deploys := gathered(t, fc, "engineering_friday_deploys_count")
	assert.LessOrEqual(t, deploys["successful"], deploys["attempted"])

	// a second pass moves the drifting gauges on from where they stopped
	// rather than restarting them.
	before := gathered(t, fc, "engineering_pr_time_to_first_review_minutes")

	m.generateData()

	after := gathered(t, fc, "engineering_pr_time_to_first_review_minutes")

	assert.NotEqual(t, before, after)
}

// newFactory creates a metrics factory backed by a registry of its own, so
// parallel tests never collide over metric names.
func newFactory() metricutil.Factory {
	return metricutil.NewFactory(
		"engineering",
		prometheus.NewRegistry(),
		metricutil.WithCustomHost("demo"),
	)
}

// gathered collects the current values of one metric family, keyed by the
// value of the single label that splits it (or the empty string when the
// family carries no label of its own).
func gathered(t *testing.T, fc metricutil.Factory, name string) map[string]float64 {
	t.Helper()

	ff, err := fc.Gather()
	require.NoError(t, err)

	out := map[string]float64{}

	for _, f := range ff {
		if f.GetName() != name {
			continue
		}

		for _, m := range f.GetMetric() {
			out[labelValue(m)] = m.GetGauge().GetValue()
		}
	}

	return out
}

// labelValue returns the value of the metric's own label, skipping the host
// label the factory attaches to everything.
func labelValue(m *dto.Metric) string {
	for _, l := range m.GetLabel() {
		if l.GetName() != "host" {
			return l.GetValue()
		}
	}

	return ""
}
