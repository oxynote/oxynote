package processor

// VictoriaMetrics represents a VictoriaMetrics data source processor. It
// serves the Prometheus HTTP API and runs MetricsQL.
type VictoriaMetrics struct {
	prometheusAPI
}

// NewVictoriaMetrics creates a new VictoriaMetrics data source processor.
func NewVictoriaMetrics(inp Input) *VictoriaMetrics {
	return &VictoriaMetrics{prometheusAPI{inp: inp}}
}
