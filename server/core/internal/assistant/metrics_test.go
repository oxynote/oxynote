package assistant

import (
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	m.observeToolCall("get_document", "success", 0.25)

	assert.InEpsilon(t, 1.0, testutil.MetricValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "get_document", "status": "success"}), 0.0001)
	assert.InEpsilon(t, 1.0, testutil.MetricValue(t, rg, "test_assistant_tool_call_duration_seconds",
		map[string]string{"tool": "get_document"}), 0.0001)
}

func Test_metrics_recordToolCall(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

	m.recordToolCall("get_document", "success")
	m.recordToolCall("get_document", "success")
	m.recordToolCall("get_document", "error")

	assert.InEpsilon(t, 2.0, testutil.MetricValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "get_document", "status": "success"}), 0.0001)
	assert.InEpsilon(t, 1.0, testutil.MetricValue(t, rg, "test_assistant_tool_calls_total",
		map[string]string{"tool": "get_document", "status": "error"}), 0.0001)
}

func Test_metrics_recordToolDuration(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg), "claude")

	m.recordToolDuration("get_document", 0.25)
	m.recordToolDuration("get_document", 0.5)

	assert.InEpsilon(t, 2.0, testutil.MetricValue(t, rg, "test_assistant_tool_call_duration_seconds",
		map[string]string{"tool": "get_document"}), 0.0001)
}

func Test_metrics_recordTokenUsage(t *testing.T) {
	t.Parallel()

	cachedUsage := &schema.TokenUsage{PromptTokens: 1, CompletionTokens: 1}
	cachedUsage.PromptTokenDetails.CachedTokens = 9
	cachedUsage.CompletionTokensDetails.ReasoningTokens = 4

	// Tokens maps each token type to its counter, or -1 when no series
	// exists for it.
	cc := map[string]struct {
		Usage  *schema.TokenUsage
		Tokens map[string]float64
	}{
		"Nothing to record": {
			// nothing to record must not panic or invent series.
			Tokens: map[string]float64{
				"input":      -1,
				"output":     -1,
				"cache_read": -1,
				"reasoning":  -1,
			},
		},
		"Provider without prompt caching": {
			// a provider without prompt caching reports no cache series
			// at all, rather than a permanent zero that reads as "caching
			// never hits".
			Usage: &schema.TokenUsage{PromptTokens: 3, CompletionTokens: 5},
			Tokens: map[string]float64{
				"input":      3,
				"output":     5,
				"cache_read": -1,
				"reasoning":  -1,
			},
		},
		"Provider reporting caching": {
			// a provider that does report caching registers the series.
			Usage: cachedUsage,
			Tokens: map[string]float64{
				"input":      1,
				"output":     1,
				"cache_read": 9,
				"reasoning":  4,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rg := prometheus.NewRegistry()
			m := newMetrics(metricutil.NewFactory("test", rg), "claude")

			m.recordTokenUsage(c.Usage)

			for kind, exp := range c.Tokens {
				assert.InDelta(t, exp, testutil.MetricValue(t, rg, "test_assistant_token_usage_total",
					map[string]string{"type": kind, "provider": "claude"}), 0.0001, kind)
			}
		})
	}
}

func Test_metrics_addTokens(t *testing.T) {
	t.Parallel()

	// non-positive counts never create a series. Result is the input
	// counter, or -1 when no series exists for it.
	cc := map[string]struct {
		Count  int
		Result float64
	}{
		"Zero count creates no series": {
			Result: -1,
		},
		"Negative count creates no series": {
			Count:  -1,
			Result: -1,
		},
		"Positive count is added": {
			Count:  6,
			Result: 6,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rg := prometheus.NewRegistry()
			m := newMetrics(metricutil.NewFactory("test", rg), "openai")

			m.addTokens("input", c.Count)

			assert.InDelta(t, c.Result, testutil.MetricValue(t, rg, "test_assistant_token_usage_total",
				map[string]string{"type": "input", "provider": "openai"}), 0.0001)
		})
	}
}
