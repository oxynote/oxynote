package datasource

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_newMetrics(t *testing.T) {
	t.Parallel()

	m := newMetrics(metricutil.NewFactory("test", prometheus.NewRegistry()))
	require.NotNil(t, m)
	assert.NotNil(t, m.cacheLookups)
}

func Test_metrics_recordLookup(t *testing.T) {
	t.Parallel()

	rg := prometheus.NewRegistry()
	m := newMetrics(metricutil.NewFactory("test", rg))

	m.recordLookup(_lookupHit)
	m.recordLookup(_lookupHit)
	m.recordLookup(_lookupShared)

	assert.InEpsilon(t, 2.0, lookups(t, rg, _lookupHit), 0.0001)
	assert.InEpsilon(t, 1.0, lookups(t, rg, _lookupShared), 0.0001)
	assert.InDelta(t, -1.0, lookups(t, rg, _lookupMiss), 0.0001)
}
