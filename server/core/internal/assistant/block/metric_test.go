package block

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MetricEnums(t *testing.T) {
	t.Parallel()

	got := MetricEnums()

	require.Len(t, got, 5)
	assert.Equal(t, []string{"line_chart", "bar_chart", "gauge_chart"}, got[document.AttrVisualizationType])
	assert.Equal(t, []string{"compact", "standard", "wide"}, got[document.AttrWidth])
	assert.Len(t, got[document.AttrTimeRange], 28)
	assert.Len(t, got[document.AttrRefreshInterval], 10)
	assert.Len(t, got[document.AttrUnitType], 20)

	// the copy protects the package's own table from a caller that
	// sorts or appends to what it was handed.
	got[document.AttrVisualizationType][0] = "wibble"
	assert.Equal(t, "line_chart", MetricEnums()[document.AttrVisualizationType][0])
}

func Test_validateMetric(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Block        Block
		Attrs        document.Attributes
		Err          error
		ExpectedPath string
	}{
		// a metric says everything in its attrs and carries no blocks of
		// its own.
		"Text is not accepted": {
			Block: Block{Text: "x"},
			Err:   assert.AnError,
		},
		"Items are not accepted": {
			Block: Block{Items: []Block{{Type: BlockParagraph, Text: "x"}}},
			Err:   assert.AnError,
		},
		"No attrs at all": {},
		"A fully configured block": {
			Attrs: document.Attributes{
				document.AttrTitle:              "Request rate",
				document.AttrDataSourceID:       "d1qbc8kv2vg000cnb6ag",
				document.AttrVisualizationType:  "line_chart",
				document.AttrQueries:            []any{map[string]any{"name": "Query 1", "query": "up", "legendFormat": "{{job}}"}},
				document.AttrTimeRange:          "last_1_hour",
				document.AttrRefreshInterval:    "5m",
				document.AttrThresholds:         []any{map[string]any{"value": 90.0, "label": "warn", "color": "#f00"}},
				document.AttrBaseThresholdColor: "#0f0",
				document.AttrDecimals:           2,
				document.AttrUnitType:           "percent0to100",
				document.AttrUnitCustom:         "rps",
				document.AttrAxisBoundsMin:      0,
				document.AttrAxisBoundsMax:      100.5,
				document.AttrWidth:              "wide",
				document.AttrSimulationPreset:   "cpu_usage",
			},
		},
		"Nulls are as absent as missing keys": {
			Attrs: document.Attributes{
				document.AttrVisualizationType: nil,
				document.AttrTimeRange:         nil,
				document.AttrQueries:           nil,
				document.AttrThresholds:        nil,
				document.AttrDecimals:          nil,
				document.AttrAxisBoundsMin:     nil,
			},
		},
		"An unknown attr passes through": {
			Attrs: document.Attributes{"config": map[string]any{"type": "line_chart"}, "wibble": 1},
		},
		"Unknown visualization type": {
			Attrs:        document.Attributes{document.AttrVisualizationType: "pie_chart"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.visualizationType",
		},
		"Non-string visualization type": {
			Attrs:        document.Attributes{document.AttrVisualizationType: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.visualizationType",
		},
		"Unknown time range": {
			Attrs:        document.Attributes{document.AttrTimeRange: "last_4_hours"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.timeRange",
		},
		"Unknown refresh interval": {
			Attrs:        document.Attributes{document.AttrRefreshInterval: "45s"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.refreshInterval",
		},
		"Unknown unit type": {
			Attrs:        document.Attributes{document.AttrUnitType: "furlongs"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.unitType",
		},
		"Unknown width": {
			Attrs:        document.Attributes{document.AttrWidth: "enormous"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.width",
		},
		"Non-string title": {
			Attrs:        document.Attributes{document.AttrTitle: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.title",
		},
		"Non-string data source id": {
			Attrs:        document.Attributes{document.AttrDataSourceID: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.dataSourceId",
		},
		"Non-string unit custom": {
			Attrs:        document.Attributes{document.AttrUnitCustom: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.unitCustom",
		},
		"Non-string base threshold color": {
			Attrs:        document.Attributes{document.AttrBaseThresholdColor: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.baseThresholdColor",
		},
		"Non-string simulation preset": {
			Attrs:        document.Attributes{document.AttrSimulationPreset: 1},
			Err:          assert.AnError,
			ExpectedPath: "attrs.simulationPreset",
		},
		"Non-numeric decimals": {
			Attrs:        document.Attributes{document.AttrDecimals: "two"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.decimals",
		},
		"Non-numeric axis bounds min": {
			Attrs:        document.Attributes{document.AttrAxisBoundsMin: "zero"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.axisBoundsMin",
		},
		"Non-numeric axis bounds max": {
			Attrs:        document.Attributes{document.AttrAxisBoundsMax: "ten"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.axisBoundsMax",
		},
		"Queries that are not an array": {
			Attrs:        document.Attributes{document.AttrQueries: "up"},
			Err:          assert.AnError,
			ExpectedPath: "attrs.queries",
		},
		"A query row missing its query": {
			Attrs:        document.Attributes{document.AttrQueries: []any{map[string]any{"name": "Query 1"}}},
			Err:          assert.AnError,
			ExpectedPath: "attrs.queries[0]",
		},
		"A query row with a non-string legend format": {
			Attrs: document.Attributes{
				document.AttrQueries: []any{map[string]any{"name": "Query 1", "query": "up", "legendFormat": 1}},
			},
			Err:          assert.AnError,
			ExpectedPath: "attrs.queries[0]",
		},
		"Thresholds that are not an array": {
			Attrs:        document.Attributes{document.AttrThresholds: 90},
			Err:          assert.AnError,
			ExpectedPath: "attrs.thresholds",
		},
		"A threshold row with a non-numeric value": {
			Attrs:        document.Attributes{document.AttrThresholds: []any{map[string]any{"value": "90"}}},
			Err:          assert.AnError,
			ExpectedPath: "attrs.thresholds[0]",
		},
		"A threshold row with a non-string color": {
			Attrs:        document.Attributes{document.AttrThresholds: []any{map[string]any{"color": 1}}},
			Err:          assert.AnError,
			ExpectedPath: "attrs.thresholds[0]",
		},
		"A threshold row with nothing set at all": {
			Attrs: document.Attributes{document.AttrThresholds: []any{map[string]any{}}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			b := c.Block
			b.Type = BlockMetric
			b.Attrs = c.Attrs

			err := validateMetric(b, "")

			testutil.AssertEqualError(t, c.Err, err)

			if c.Err == nil {
				return
			}

			var ve *validationError

			require.ErrorAs(t, err, &ve, "expected validationError, got %T", err)
			assert.Equal(t, c.ExpectedPath, ve.Path, "validationError path mismatch (full: %s)", ve.Error())
		})
	}
}

func Test_validateMetricEnum(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricEnum(document.Attributes{}, document.AttrTimeRange, ""))
	require.NoError(t, validateMetricEnum(document.Attributes{document.AttrTimeRange: "today"}, document.AttrTimeRange, ""))
	require.Error(t, validateMetricEnum(document.Attributes{document.AttrTimeRange: "tomorrow"}, document.AttrTimeRange, ""))
}

func Test_validateMetricString(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricString(document.Attributes{}, document.AttrTitle, ""))
	require.NoError(t, validateMetricString(document.Attributes{document.AttrTitle: ""}, document.AttrTitle, ""))
	require.Error(t, validateMetricString(document.Attributes{document.AttrTitle: 1.5}, document.AttrTitle, ""))
}

func Test_validateMetricNumber(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricNumber(document.Attributes{}, document.AttrDecimals, ""))
	require.NoError(t, validateMetricNumber(document.Attributes{document.AttrDecimals: 0}, document.AttrDecimals, ""))
	require.Error(t, validateMetricNumber(document.Attributes{document.AttrDecimals: true}, document.AttrDecimals, ""))
}

func Test_validateMetricQueries(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricQueries(document.Attributes{}, ""))
	require.NoError(t, validateMetricQueries(document.Attributes{
		document.AttrQueries: []any{map[string]any{"name": "a", "query": "up"}},
	}, ""))
	require.Error(t, validateMetricQueries(document.Attributes{
		document.AttrQueries: []any{map[string]any{"name": 1, "query": "up"}},
	}, ""))
}

func Test_validateMetricThresholds(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMetricThresholds(document.Attributes{}, ""))
	require.NoError(t, validateMetricThresholds(document.Attributes{
		document.AttrThresholds: []any{map[string]any{"value": 1, "label": "x"}},
	}, ""))
	require.Error(t, validateMetricThresholds(document.Attributes{
		document.AttrThresholds: []any{map[string]any{"label": 1}},
	}, ""))
}

func Test_objectRows(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Input    any
		Expected []document.Attributes
		Err      error
	}{
		"Decoded JSON array": {
			Input:    []any{map[string]any{"a": 1}, map[string]any{"b": 2}},
			Expected: []document.Attributes{{"a": 1}, {"b": 2}},
		},
		"An element that is not an object": {
			Input: []any{[]any{1}},
			Err:   assert.AnError,
		},
		"Empty array": {
			Input:    []any{},
			Expected: []document.Attributes{},
		},
		"Not an array at all": {
			Input: "a",
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := objectRows(c.Input)

			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Expected, got)
		})
	}
}

func Test_isNumber(t *testing.T) {
	t.Parallel()

	for _, v := range []any{1.5, float32(1.5), 1, int64(1), int32(1)} {
		assert.True(t, isNumber(v), "%T should be a number", v)
	}

	for _, v := range []any{"1", true, nil, []any{1}} {
		assert.False(t, isNumber(v), "%T should not be a number", v)
	}
}
