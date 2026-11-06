package markup

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/stretchr/testify/assert"
)

func Test_enumValues(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Enums  map[string][]string
		Key    string
		Result []string
	}{
		"An enum without a reference": {
			Key:    document.AttrWidth,
			Result: []string{"compact", "standard", "wide"},
		},
		"An enum led by its reference": {
			Key:    document.AttrRefreshInterval,
			Result: []string{"$metric_refresh", "5s", "10s", "30s", "1m", "5m", "15m", "30m", "1h", "2h", "1d"},
		},
		"The time range led by its reference": {
			Key:    document.AttrTimeRange,
			Result: append([]string{"$metric_range"}, document.MetricTimeRanges()...),
		},
		"A log enum without a reference": {
			Enums:  _logEnums,
			Key:    document.AttrOrder,
			Result: []string{"desc", "asc"},
		},
		"A log time range led by its reference": {
			Enums:  _logEnums,
			Key:    document.AttrTimeRange,
			Result: append([]string{"$metric_range"}, document.MetricTimeRanges()...),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			enums := c.Enums
			if enums == nil {
				enums = _metricEnums
			}

			assert.Equal(t, c.Result, enumValues(enums, c.Key))

			// the reference is never written into the shared enum table.
			assert.NotContains(t, enums[c.Key], _windowReferences[c.Key])
		})
	}
}

func Test_setWindow(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Type     document.BlockNodeType
		Attrs    document.Attributes
		Stored   document.Block
		Expected document.Attributes
	}{
		"A new metric follows the built-in variables": {
			Attrs:    document.Attributes{},
			Expected: document.Attributes{"timeRange": "$metric_range", "refreshInterval": "$metric_refresh"},
		},
		"A new metric keeps the preset it names": {
			Attrs:    document.Attributes{"timeRange": "last_1_hour"},
			Expected: document.Attributes{"timeRange": "last_1_hour", "refreshInterval": "$metric_refresh"},
		},
		"A re-sent metric keeps its stored window": {
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"timeRange": "last_1_hour", "refreshInterval": "1m"}},
			Expected: document.Attributes{"timeRange": "last_1_hour", "refreshInterval": "1m"},
		},
		"A re-sent metric stored without a window stays without one": {
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{}},
			Expected: document.Attributes{},
		},
		"A re-sent metric takes the window it names": {
			Attrs:    document.Attributes{"timeRange": "$metric_range"},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"timeRange": "last_1_hour"}},
			Expected: document.Attributes{"timeRange": "$metric_range"},
		},
		"A new log block follows the built-in variables": {
			Type:     document.BlockNodeLogBlock,
			Attrs:    document.Attributes{},
			Expected: document.Attributes{"timeRange": "$metric_range", "refreshInterval": "$metric_refresh"},
		},
		"A re-sent log block keeps its stored window": {
			Type:     document.BlockNodeLogBlock,
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeLogBlock, Attrs: document.Attributes{"timeRange": "last_1_hour", "refreshInterval": "1m"}},
			Expected: document.Attributes{"timeRange": "last_1_hour", "refreshInterval": "1m"},
		},
		"A log block given a metric's id counts as new": {
			Type:     document.BlockNodeLogBlock,
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"timeRange": "last_1_hour"}},
			Expected: document.Attributes{"timeRange": "$metric_range", "refreshInterval": "$metric_refresh"},
		},
		"An id given to another kind of block counts as new": {
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"timeRange": "last_1_hour"}},
			Expected: document.Attributes{"timeRange": "$metric_range", "refreshInterval": "$metric_refresh"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			typ := c.Type
			if typ == "" {
				typ = document.BlockNodeMetricBlock
			}

			setWindow(c.Attrs, c.Stored, typ)

			assert.Equal(t, c.Expected, c.Attrs)
		})
	}
}
