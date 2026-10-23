package metricutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_discarder_Set(_ *testing.T) {
	(&discarder{}).Set(123)
}

func Test_discarder_Dec(_ *testing.T) {
	(&discarder{}).Dec()
}

func Test_discarder_Sub(_ *testing.T) {
	(&discarder{}).Sub(123)
}

func Test_discarder_Inc(_ *testing.T) {
	(&discarder{}).Inc()
}

func Test_discarder_Add(_ *testing.T) {
	(&discarder{}).Add(5)
}

func Test_discarder_Observe(_ *testing.T) {
	(&discarder{}).Observe(1.0)
}

func Test_histogramBuckets(t *testing.T) {
	cc := map[string]struct {
		Buckets []float64
		Result  []float64
	}{
		"Custom buckets take precedence": {
			Buckets: []float64{1, 2},
			Result:  []float64{1, 2},
		},
		"Package default without custom buckets": {
			Result: _defaultHistogramBuckets,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, histogramBuckets(c.Buckets))
		})
	}
}
