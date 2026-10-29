package datasource

import (
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// _subsystemDatasource namespaces every data source metric.
	_subsystemDatasource = "datasource"

	// _resultLabel labels a cache lookup with how it was answered.
	_resultLabel = "result"
)

// All available cache lookup results.
const (
	_lookupHit    = "hit"
	_lookupMiss   = "miss"
	_lookupShared = "shared"
)

// metrics holds Prometheus metrics for data source queries.
type metrics struct {
	cacheLookups metricutil.CounterVec
}

// newMetrics creates data source metrics from the given factory.
func newMetrics(fc metricutil.Factory) *metrics {
	return &metrics{
		cacheLookups: fc.NewCounterVec(
			metricutil.Options{
				Subsystem: _subsystemDatasource,
				Name:      "cache_lookups_total",
				Help:      "Tracks data source query cache lookups by how they were answered.",
			},
			[]string{_resultLabel},
		),
	}
}

// recordLookup counts one cache lookup answered as result: from the cache,
// by running the query, or by sharing a query another call was running.
func (m *metrics) recordLookup(result string) {
	m.cacheLookups.With(prometheus.Labels{_resultLabel: result}).Inc()
}
