package assistant

import (
	"testing"

	"github.com/cloudwego/eino/schema"
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

	m := newMetrics(metricutil.NewFactory("test", prometheus.NewRegistry()), "claude")
	require.NotNil(t, m)

	assert.NotNil(t, m.toolCalls)
	assert.NotNil(t, m.toolDuration)
	assert.NotNil(t, m.tokenUsage)
	assert.Equal(t, "claude", m.provider)
}

func Test_metrics_observeToolCall(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

	m.observeToolCall("read_block", "success", 0.25)

	assert.InEpsilon(t, 1.0, gatherValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "read_block", "status": "success"}), 0.0001)
	assert.InEpsilon(t, 1.0, gatherValue(t, rg, "test_assistant_tool_call_duration_seconds",
		map[string]string{"tool": "read_block"}), 0.0001)
}

func Test_metrics_recordToolCall(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

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
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

	m.recordToolDuration("read_block", 0.25)
	m.recordToolDuration("read_block", 0.5)

	assert.InEpsilon(t, 2.0, gatherValue(t, rg, "test_assistant_tool_call_duration_seconds",
		map[string]string{"tool": "read_block"}), 0.0001)
}

func Test_metrics_recordTokenUsage(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

	// nothing to record must not panic or invent series.
	m.recordTokenUsage(nil)

	// a provider without prompt caching reports no cache series at all,
	// rather than a permanent zero that reads as "caching never hits".
	m.recordTokenUsage(&schema.TokenUsage{PromptTokens: 3, CompletionTokens: 5})

	assert.InEpsilon(t, 3.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "input", "provider": "claude"}), 0.0001)
	assert.InEpsilon(t, 5.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "output", "provider": "claude"}), 0.0001)
	assert.InDelta(t, -1.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "cache_read"}), 0.0001)

	// a provider that does report caching registers the series.
	usage := &schema.TokenUsage{PromptTokens: 1, CompletionTokens: 1}
	usage.PromptTokenDetails.CachedTokens = 9
	usage.CompletionTokensDetails.ReasoningTokens = 4

	m.recordTokenUsage(usage)

	assert.InEpsilon(t, 9.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "cache_read", "provider": "claude"}), 0.0001)
	assert.InEpsilon(t, 4.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "reasoning", "provider": "claude"}), 0.0001)
}

func Test_metrics_addTokens(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "openai")

	// non-positive counts never create a series.
	m.addTokens("input", 0)
	m.addTokens("input", -1)

	assert.InDelta(t, -1.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "input"}), 0.0001)

	m.addTokens("input", 6)

	assert.InEpsilon(t, 6.0, gatherValue(t, rg, "test_assistant_token_usage_total",
		map[string]string{"type": "input", "provider": "openai"}), 0.0001)
}
