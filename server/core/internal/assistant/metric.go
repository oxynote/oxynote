package assistant

import (
	"github.com/oxynote/heimdall/internal/assistant/tools"
	"github.com/oxynote/purse/util/metricutil"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	_subsystemAssistant = "assistant"
	_toolLabel          = "tool"
	_statusLabel        = "status"
	_tokenTypeLabel     = "type"
)

// metrics holds Prometheus metrics for AI assistant operations.
type metrics struct {
	toolCalls    metricutil.CounterVec
	toolDuration metricutil.HistogramVec
	tokenUsage   metricutil.CounterVec
}

// newMetrics creates assistant metrics from the given factory.
func newMetrics(fc metricutil.Factory) *metrics {
	return &metrics{
		toolCalls: fc.NewCounterVec(
			metricutil.Options{
				Subsystem: _subsystemAssistant,
				Name:      "tool_calls_total",
				Help:      "Tracks the number of AI assistant tool calls.",
			},
			[]string{_toolLabel, _statusLabel},
		),
		toolDuration: fc.NewHistogramVec(
			metricutil.Options{
				Subsystem: _subsystemAssistant,
				Name:      "tool_call_duration_seconds",
				Help:      "Tracks the duration of AI assistant tool calls.",
			},
			[]string{_toolLabel},
		),
		tokenUsage: fc.NewCounterVec(
			metricutil.Options{
				Subsystem: _subsystemAssistant,
				Name:      "token_usage_total",
				Help:      "Tracks the number of tokens used by AI assistant.",
			},
			[]string{_tokenTypeLabel},
		),
	}
}

// recordToolCall records a tool call metric with the given tool name and status.
func (m *metrics) recordToolCall(tool tools.Name, status string) {
	m.toolCalls.With(prometheus.Labels{
		_toolLabel:   string(tool),
		_statusLabel: status,
	}).Inc()
}

// recordToolDuration records the duration of a tool call.
func (m *metrics) recordToolDuration(tool tools.Name, seconds float64) {
	m.toolDuration.With(prometheus.Labels{
		_toolLabel: string(tool),
	}).Observe(seconds)
}

// recordTokenUsage records token usage from an Anthropic API response.
func (m *metrics) recordTokenUsage(inputTokens, outputTokens, cacheCreationTokens, cacheReadTokens int64) {
	m.tokenUsage.With(prometheus.Labels{_tokenTypeLabel: "input"}).
		Add(float64(inputTokens))

	m.tokenUsage.With(prometheus.Labels{_tokenTypeLabel: "output"}).
		Add(float64(outputTokens))

	if cacheCreationTokens > 0 {
		m.tokenUsage.With(prometheus.Labels{_tokenTypeLabel: "cache_creation"}).
			Add(float64(cacheCreationTokens))
	}

	if cacheReadTokens > 0 {
		m.tokenUsage.With(prometheus.Labels{_tokenTypeLabel: "cache_read"}).
			Add(float64(cacheReadTokens))
	}
}
