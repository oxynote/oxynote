package markup

import (
	"errors"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MetricEnums(t *testing.T) {
	t.Parallel()

	got := MetricEnums()

	require.Len(t, got, 6)

	// the chart types are the processor's, which this package does not
	// import, so only this check keeps the two lists equal.
	assert.Equal(t, []string{
		string(processor.ChartTypeLine),
		string(processor.ChartTypeBar),
		string(processor.ChartTypeGauge),
	}, got[document.AttrVisualizationType])
	assert.Equal(t, []string{"compact", "standard", "wide"}, got[document.AttrWidth])
	assert.Len(t, got[document.AttrSimulationPreset], 6)
	assert.Len(t, got[document.AttrTimeRange], 28)
	assert.Len(t, got[document.AttrRefreshInterval], 10)
	assert.Len(t, got[document.AttrUnitType], 20)

	// the copy protects the package's own table from a caller that
	// sorts or appends to what it was handed.
	got[document.AttrVisualizationType][0] = "wibble"
	assert.Equal(t, "line_chart", MetricEnums()[document.AttrVisualizationType][0])
}

func Test_MetricReference(t *testing.T) {
	t.Parallel()

	got := MetricReference()

	// every enum the decoder checks is published with its values.
	for key, values := range _metricEnums {
		assert.Contains(t, got, key+" (")

		for _, v := range values {
			assert.Contains(t, got, v)
		}
	}

	assert.Contains(t, got, "unitCustom")
	assert.Contains(t, got, "dataSourceId")
	assert.Contains(t, got, "timeRange ($metric_range|")
	assert.Contains(t, got, "refreshInterval ($metric_refresh|")
	assert.Contains(t, got, "timeRange and refreshInterval default to $metric_range and $metric_refresh")
	assert.Contains(t, got, "and the reference to unpin it")
	assert.Contains(t, got, "A threshold colour and baseThresholdColor are one of red|orange|")
}

func Test_decodeMetricAttrs(t *testing.T) {
	t.Parallel()

	// legacyColor is a threshold colour saved before thresholds stored
	// palette names.
	const legacyColor = "oklch(57.7% .245 27.325)"

	cc := map[string]struct {
		Text   string
		Stored document.Block
		Result document.Attributes
		Err    error
	}{
		"No attrs at all": {
			Text:   " ",
			Result: document.Attributes{},
		},
		"A fully configured metric": {
			Text: `{"title":"Request rate","dataSourceId":"d1qbc8kv2vg000cnb6ag","visualizationType":"line_chart",` +
				`"queries":[{"name":"Query 1","query":"up","legendFormat":"{{job}}"}],"timeRange":"last_1_hour","refreshInterval":"5m",` +
				`"thresholds":[{"value":90,"label":"warn","color":"red"}],"baseThresholdColor":"green","decimals":2,` +
				`"unitType":"percent0to100","unitCustom":"rps","axisBoundsMin":0,"axisBoundsMax":100.5,"width":"wide","simulationPreset":"cpu_usage"}`,
			Result: document.Attributes{
				document.AttrTitle:             "Request rate",
				document.AttrDataSourceID:      "d1qbc8kv2vg000cnb6ag",
				document.AttrVisualizationType: "line_chart",
				document.AttrQueries:           []any{map[string]any{"name": "Query 1", "query": "up", "legendFormat": "{{job}}"}},
				document.AttrTimeRange:         "last_1_hour",
				document.AttrRefreshInterval:   "5m",
				"thresholds":                   []any{map[string]any{"value": 90.0, "label": "warn", "color": "red"}},
				"baseThresholdColor":           "green",
				"decimals":                     2.0,
				document.AttrUnitType:          "percent0to100",
				"unitCustom":                   "rps",
				"axisBoundsMin":                0.0,
				"axisBoundsMax":                100.5,
				document.AttrWidth:             "wide",
				document.AttrSimulationPreset:  "cpu_usage",
			},
		},
		"Nulls are as absent as missing keys": {
			Text:   `{"visualizationType":null,"queries":null,"thresholds":null,"decimals":null}`,
			Result: document.Attributes{document.AttrVisualizationType: nil, document.AttrQueries: nil, "thresholds": nil, "decimals": nil},
		},
		"An unknown attr passes through": {
			Text:   `{"wibble":1}`,
			Result: document.Attributes{"wibble": 1.0},
		},
		"A threshold row with nothing set": {
			Text:   `{"thresholds":[{}]}`,
			Result: document.Attributes{"thresholds": []any{map[string]any{}}},
		},
		"Built-in variable references": {
			Text: `{"timeRange":"$metric_range","refreshInterval":"$metric_refresh"}`,
			Result: document.Attributes{
				document.AttrTimeRange:       "$metric_range",
				document.AttrRefreshInterval: "$metric_refresh",
			},
		},
		"A time range following the refresh interval": {
			Text: `{"timeRange":"$metric_refresh"}`,
			Err:  assert.AnError,
		},
		"A refresh interval following another variable": {
			Text: `{"refreshInterval":"$env"}`,
			Err:  errors.New("metric refreshInterval must be one of: $metric_refresh, 5s, 10s, 30s, 1m, 5m, 15m, 30m, 1h, 2h, 1d"),
		},
		"Not JSON": {
			Text: "{",
			Err:  assert.AnError,
		},
		"Non-string width": {
			Text: `{"width":1}`,
			Err:  errors.New("metric width must be one of: compact, standard, wide"),
		},
		"Unknown visualization type": {
			Text: `{"visualizationType":"pie_chart"}`,
			Err:  assert.AnError,
		},
		"Non-string title": {
			Text: `{"title":1.5}`,
			Err:  assert.AnError,
		},
		"Non-numeric decimals": {
			Text: `{"decimals":true}`,
			Err:  assert.AnError,
		},
		"Queries that are not an array": {
			Text: `{"queries":"up"}`,
			Err:  assert.AnError,
		},
		"A query row with a non-string legend format": {
			Text: `{"queries":[{"name":"a","query":"up","legendFormat":1}]}`,
			Err:  assert.AnError,
		},
		"A query row missing its query": {
			Text: `{"queries":[{"name":"a","query":"up"},{"name":"b"}]}`,
			Err:  errors.New("metric query 2: name and query are required"),
		},
		"Thresholds that are not an array": {
			Text: `{"thresholds":90}`,
			Err:  assert.AnError,
		},
		"A threshold row with a non-numeric value": {
			Text: `{"thresholds":[{"value":"90"}]}`,
			Err:  assert.AnError,
		},
		"A threshold colour outside the palette": {
			Text: `{"thresholds":[{"value":90,"color":"red"},{"value":95,"color":"crimson"}]}`,
			Err:  errors.New("metric threshold 2: color must be one of: red, orange, amber, yellow, lime, green, emerald, teal, cyan, sky, blue, indigo, violet, purple, fuchsia, pink"),
		},
		"A base threshold colour outside the palette": {
			Text: `{"baseThresholdColor":"#00ff00"}`,
			Err:  errors.New("metric baseThresholdColor must be one of: red, orange, amber, yellow, lime, green, emerald, teal, cyan, sky, blue, indigo, violet, purple, fuchsia, pink"),
		},
		"Colours the stored metric already carries": {
			Text: `{"thresholds":[{"value":90,"color":"` + legacyColor + `"}],"baseThresholdColor":"` + legacyColor + `"}`,
			Stored: document.Block{
				Type: document.BlockNodeMetricBlock,
				Attrs: document.Attributes{
					"thresholds": []any{map[string]any{"value": 90.0, "color": legacyColor}},
				},
			},
			Result: document.Attributes{
				"thresholds":         []any{map[string]any{"value": 90.0, "color": legacyColor}},
				"baseThresholdColor": legacyColor,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := decodeMetricAttrs(c.Text, c.Stored)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_storedMetricColors(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Stored document.Block
		Result []string
	}{
		"No stored metric": {
			Result: []string{""},
		},
		"Thresholds that are not an array": {
			Stored: document.Block{Attrs: document.Attributes{"baseThresholdColor": "green", "thresholds": 90.0}},
			Result: []string{"green"},
		},
		"Base and threshold colours": {
			Stored: document.Block{Attrs: document.Attributes{
				"baseThresholdColor": "green",
				"thresholds":         []any{map[string]any{"color": "red"}, "warn", map[string]any{"value": 1.0}},
			}},
			Result: []string{"green", "red", ""},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, storedMetricColors(c.Stored))
		})
	}
}

func Test_metricValues(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
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
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, metricValues(c.Key))

			// the reference is never written into the shared enum table.
			assert.NotContains(t, _metricEnums[c.Key], _metricReferences[c.Key])
		})
	}
}

func Test_setSimulation(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Attrs    document.Attributes
		Stored   document.Block
		Expected document.Attributes
	}{
		"A new metric naming a preset simulates": {
			Attrs:    document.Attributes{"simulationPreset": "cpu_usage"},
			Expected: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": true},
		},
		"A new metric naming no preset has no flag": {
			Attrs:    document.Attributes{"title": "x"},
			Expected: document.Attributes{"title": "x"},
		},
		"The stored preset keeps a flag that was switched off": {
			Attrs:    document.Attributes{"simulationPreset": "cpu_usage"},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": false}},
			Expected: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": false},
		},
		"The stored preset stored without a flag stays without one": {
			Attrs:    document.Attributes{"simulationPreset": "cpu_usage"},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"simulationPreset": "cpu_usage"}},
			Expected: document.Attributes{"simulationPreset": "cpu_usage"},
		},
		"Another preset simulates": {
			Attrs:    document.Attributes{"simulationPreset": "error_rate"},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": false}},
			Expected: document.Attributes{"simulationPreset": "error_rate", "simulationActive": true},
		},
		"A dropped preset drops the flag": {
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": true}},
			Expected: document.Attributes{},
		},
		"An id given to another kind of block counts as new": {
			Attrs: document.Attributes{"simulationPreset": "cpu_usage"},
			Stored: document.Block{
				Type:  document.BlockNodeParagraph,
				Attrs: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": false},
			},
			Expected: document.Attributes{"simulationPreset": "cpu_usage", "simulationActive": true},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			setSimulation(c.Attrs, c.Stored)

			assert.Equal(t, c.Expected, c.Attrs)
		})
	}
}

func Test_setMetricWindow(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
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
		"An id given to another kind of block counts as new": {
			Attrs:    document.Attributes{},
			Stored:   document.Block{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"timeRange": "last_1_hour"}},
			Expected: document.Attributes{"timeRange": "$metric_range", "refreshInterval": "$metric_refresh"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			setMetricWindow(c.Attrs, c.Stored)

			assert.Equal(t, c.Expected, c.Attrs)
		})
	}
}
