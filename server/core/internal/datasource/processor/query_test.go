package processor

import (
	"math"
	"net/http"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
)

func Test_ChartType_UnmarshalText(t *testing.T) {
	cc := map[string]struct {
		Text   string
		Result ChartType
		Err    error
	}{
		"Line chart":         {Text: "line_chart", Result: ChartTypeLine},
		"Bar chart":          {Text: "bar_chart", Result: ChartTypeBar},
		"Gauge chart":        {Text: "gauge_chart", Result: ChartTypeGauge},
		"Empty chart type":   {Text: "", Err: assert.AnError},
		"Unknown chart type": {Text: "bogus", Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var ct ChartType

			err := ct.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, ct)
		})
	}
}

func Test_ChartType_IsValid(t *testing.T) {
	cc := map[string]struct {
		ChartType ChartType
		Result    bool
	}{
		"Line chart": {
			ChartType: ChartTypeLine,
			Result:    true,
		},
		"Bar chart": {
			ChartType: ChartTypeBar,
			Result:    true,
		},
		"Gauge chart": {
			ChartType: ChartTypeGauge,
			Result:    true,
		},
		"Empty chart type": {
			ChartType: ChartType(""),
		},
		"Unknown chart type": {
			ChartType: ChartType("bogus"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.ChartType.IsValid())
		})
	}
}

func Test_isValidValue(t *testing.T) {
	cc := map[string]struct {
		Value  float64
		Result bool
	}{
		"Valid value": {
			Value:  42.5,
			Result: true,
		},
		"NaN value": {
			Value: math.NaN(),
		},
		"Positive infinity": {
			Value: math.Inf(1),
		},
		"Negative infinity": {
			Value: math.Inf(-1),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, isValidValue(c.Value))
		})
	}
}

func Test_NewInvalidQueryError(t *testing.T) {
	t.Parallel()

	assert.Equal(
		t,
		errutil.New(http.StatusBadRequest, "query.error", "boom"),
		NewInvalidQueryError("boom"),
	)
}
