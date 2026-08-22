package assistant

import (
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/tools"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// _subsystemAssistant namespaces every assistant metric.
	_subsystemAssistant = "assistant"

	// _toolLabel labels a metric with the tool it describes.
	_toolLabel = "tool"

	// _statusLabel labels a tool call with its outcome.
	_statusLabel = "status"

	// _tokenTypeLabel labels token usage with the kind of token counted.
	_tokenTypeLabel = "type"

	// _providerLabel labels token usage with the configured model
	// provider.
	_providerLabel = "provider"
)

// metrics holds Prometheus metrics for AI assistant operations.
type metrics struct {
	toolCalls    metricutil.CounterVec
	toolDuration metricutil.HistogramVec
	tokenUsage   metricutil.CounterVec

	// provider labels token usage so a deployment that switches
	// providers can still read its history.
	provider string
}

// newMetrics creates assistant metrics from the given factory.
func newMetrics(fc metricutil.Factory, provider string) *metrics {
	return &metrics{
		toolCalls: fc.NewCounterVec(
			metricutil.Options{
				Subsystem: _subsystemAssistant,
				Name:      "tool_calls_total",
				Help:      "Tracks the number of AI assistant tool calls.",
			},
			[]string{
				_toolLabel,
				_statusLabel,
			},
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
			[]string{
				_tokenTypeLabel,
				_providerLabel,
			},
		),
		provider: provider,
	}
}

// observeToolCall records one finished tool call's outcome. Its shape
// matches the observer middleware's observe callback.
func (m *metrics) observeToolCall(tool tools.Name, status string, seconds float64) {
	m.recordToolCall(tool, status)
	m.recordToolDuration(tool, seconds)
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

// recordTokenUsage records token usage from one model response.
//
// Cached-token counts are only emitted when the provider reports them.
// Providers without prompt caching would otherwise publish a permanent
// zero series, which reads as "caching is on and never hits" rather
// than "this provider has no caching".
func (m *metrics) recordTokenUsage(usage *schema.TokenUsage) {
	if usage == nil {
		return
	}

	m.addTokens("input", usage.PromptTokens)
	m.addTokens("output", usage.CompletionTokens)

	if cached := usage.PromptTokenDetails.CachedTokens; cached > 0 {
		m.addTokens("cache_read", cached)
	}

	if reasoning := usage.CompletionTokensDetails.ReasoningTokens; reasoning > 0 {
		m.addTokens("reasoning", reasoning)
	}
}

// addTokens increments one token-type counter for the active provider.
func (m *metrics) addTokens(kind string, count int) {
	if count <= 0 {
		return
	}

	m.tokenUsage.With(prometheus.Labels{
		_tokenTypeLabel: kind,
		_providerLabel:  m.provider,
	}).Add(float64(count))
}
