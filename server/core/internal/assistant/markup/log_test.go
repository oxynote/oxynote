package markup

import (
	"errors"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_LogReference(t *testing.T) {
	t.Parallel()

	got := LogReference()

	// every enum only a log block has is published with its values.
	for key, values := range _logOnlyEnums {
		assert.Contains(t, got, key+" (")

		for _, v := range values {
			assert.Contains(t, got, v)
		}
	}

	assert.Contains(t, got, "dataSourceId, queries ([{name, query}]), timeRange, refreshInterval, width and height as a metric block's")
	assert.Contains(t, got, ". It may also hold title. order sorts the logs by time, and desc shows the newest first")
}

func Test_decodeLogAttrs(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Text   string
		Result document.Attributes
		Err    error
	}{
		"No attrs at all": {
			Text:   " ",
			Result: document.Attributes{},
		},
		"A fully configured log block": {
			Text: `{"title":"API errors","dataSourceId":"d1qbc8kv2vg000cnb6ag","queries":[{"name":"Query 1","query":"up"}],` +
				`"timestampFormat":"numeric_nano","order":"asc","width":"wide","height":"tall"}`,
			Result: document.Attributes{
				document.AttrTitle:           "API errors",
				document.AttrDataSourceID:    "d1qbc8kv2vg000cnb6ag",
				document.AttrQueries:         []any{map[string]any{"name": "Query 1", "query": "up"}},
				document.AttrTimestampFormat: "numeric_nano",
				document.AttrOrder:           "asc",
				document.AttrWidth:           "wide",
				document.AttrHeight:          "tall",
			},
		},
		"Nulls are as absent as missing keys": {
			Text:   `{"order":null,"queries":null}`,
			Result: document.Attributes{document.AttrOrder: nil, document.AttrQueries: nil},
		},
		"Not JSON": {
			Text: "{",
			Err:  assert.AnError,
		},
		"An enum that is not a string": {
			Text: `{"timestampFormat":1}`,
			Err:  errors.New("log block timestampFormat must be one of: none, readable, readable_milli, readable_nano, readable_time, readable_time_milli, readable_time_nano, numeric, numeric_milli, numeric_nano, numeric_time, numeric_time_milli, numeric_time_nano"),
		},
		"A time range that follows the built-in variable": {
			Text:   `{"timeRange":"$metric_range","refreshInterval":"1m"}`,
			Result: document.Attributes{document.AttrTimeRange: "$metric_range", document.AttrRefreshInterval: "1m"},
		},
		"An unknown refresh interval": {
			Text: `{"refreshInterval":"1y"}`,
			Err:  errors.New("log block refreshInterval must be one of: $metric_refresh, 5s, 10s, 30s, 1m, 5m, 15m, 30m, 1h, 2h, 1d"),
		},
		"An unknown enum value": {
			Text: `{"width":"huge"}`,
			Err:  errors.New("log block width must be one of: compact, standard, wide"),
		},
		"An unknown height": {
			Text: `{"height":"double"}`,
			Err:  errors.New("log block height must be one of: standard, tall"),
		},
		"A title that is not a string": {
			Text: `{"title":1}`,
			Err:  assert.AnError,
		},
		"A query without its text": {
			Text: `{"queries":[{"name":"Query 1"}]}`,
			Err:  errors.New("log block query 1: name and query are required"),
		},
		"A query without its name": {
			Text: `{"queries":[{"query":"up"}]}`,
			Err:  errors.New("log block query 1: name and query are required"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := decodeLogAttrs(c.Text)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
