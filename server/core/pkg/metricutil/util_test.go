package metricutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_histogramBuckets(t *testing.T) {
	// custom buckets take precedence
	assert.Equal(t, []float64{1, 2}, histogramBuckets([]float64{1, 2}))

	// package default is used when no custom buckets are given
	assert.Equal(t, _defaultHistogramBuckets, histogramBuckets(nil))
}
