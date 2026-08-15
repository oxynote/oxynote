package assistant

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gatherValue returns the sample value of the named metric whose
// labels are a superset of want, or -1 when absent. Histograms
// report their sample count.
func gatherValue(t *testing.T, rg prometheus.Gatherer, name string, want map[string]string) float64 {
	t.Helper()

	families, err := rg.Gather()
	require.NoError(t, err)

	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}

	metric:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}

			for k, v := range want {
				if labels[k] != v {
					continue metric
				}
			}

			switch mf.GetType() {
			case dto.MetricType_COUNTER:
				return m.GetCounter().GetValue()
			case dto.MetricType_HISTOGRAM:
				return float64(m.GetHistogram().GetSampleCount())
			default:
				return -1
			}
		}
	}

	return -1
}

func Test_newMetrics(t *testing.T) {
	t.Parallel()

	m := newMetrics(metricutil.NewFactory("test", prometheus.NewRegistry()))
	require.NotNil(t, m)

	assert.NotNil(t, m.toolCalls)
	assert.NotNil(t, m.toolDuration)
	assert.NotNil(t, m.tokenUsage)
}

func Test_metrics_recordToolCall(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg))

	m.recordToolCall("read_block", "success")
	m.recordToolCall("read_block", "success")
	m.recordToolCall("read_block", "error")

	assert.InEpsilon(t, 2.0, gatherValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "read_block", "status": "success"}), 0.0001)
	assert.InEpsilon(t, 1.0, gatherValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "read_block", "status": "error"}), 0.0001)
}

func Test_metrics_recordToolDuration(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg))

	m.recordToolDuration("read_block", 0.25)
	m.recordToolDuration("read_block", 0.5)

	assert.InEpsilon(t, 2.0, gatherValue(t, rg, "test_assistant_tool_call_duration_seconds",
		map[string]string{"tool": "read_block"}), 0.0001)
}

func Test_metrics_recordTokenUsage(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg))

	// zero cache values skip their series entirely
	m.recordTokenUsage(3, 5, 0, 0)

	assert.InEpsilon(t, 3.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "input"}), 0.0001)
	assert.InEpsilon(t, 5.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "output"}), 0.0001)
	assert.InEpsilon(t, -1.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "cache_creation"}), 0.0001)

	// cache values register once non-zero
	m.recordTokenUsage(0, 0, 7, 9)

	assert.InEpsilon(t, 7.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "cache_creation"}), 0.0001)
	assert.InEpsilon(t, 9.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "cache_read"}), 0.0001)
}
