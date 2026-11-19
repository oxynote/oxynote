package processor

// Prometheus represents a Prometheus data source processor.
type Prometheus struct {
	prometheusAPI
}

// NewPrometheus creates a new Prometheus data source processor.
func NewPrometheus(inp Input) *Prometheus {
	return &Prometheus{prometheusAPI{inp: inp}}
}
