package mockmetrics

// GaugeParams configures a continuous gauge that evolves each tick via drift and mean reversion.
type GaugeParams struct {
	// Min is the lower bound of the gauge value.
	Min float64

	// Max is the upper bound of the gauge value.
	Max float64

	// Start is the initial value before any ticks.
	Start float64

	// DriftPerStep is a constant added each tick, modelling a slow linear trend.
	DriftPerStep float64

	// NoiseStdDev is the standard deviation of the Gaussian noise added each tick.
	NoiseStdDev float64

	// SpikeChance is the probability (0–1) of an additional spike occurring on any given tick.
	SpikeChance float64

	// SpikeStdDev is the standard deviation of the spike magnitude when one occurs.
	SpikeStdDev float64

	// MeanReversion is the fraction (0–1) of the distance to Target pulled back each tick.
	MeanReversion float64

	// Target is the attractor value for mean reversion.
	Target float64
}
