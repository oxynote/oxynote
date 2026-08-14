package processor

import (
	"math"
	"net/http"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/stretchr/testify/assert"
)

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
