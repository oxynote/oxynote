package demodata

import (
	"math"
	"math/rand"
	"time"

	"github.com/oxynote/heimdall/datagen/internal/mockmetrics"
)

const TickInterval = 5 * time.Minute

var (
	services     = []string{"api-gateway", "user-service", "billing-service", "notification-service", "search-service"}
	environments = []string{"production", "staging"}
	severities   = []string{"critical", "major", "minor"}
	repositories = []string{"frontend", "backend", "infra", "mobile", "shared-libs"}
	branches     = []string{"main", "develop", "feature"}
)

type Deployment struct {
	Time            time.Time
	Service         string
	Environment     string
	DurationSeconds float64
	Success         bool
	Rollback        bool
}

type Incident struct {
	Time                  time.Time
	Severity              string
	Service               string
	TimeToDetectMinutes   float64
	TimeToResolveMinutes  float64
}

type BuildMetric struct {
	Time            time.Time
	Repository      string
	Branch          string
	DurationSeconds float64
	TestCount       int
	TestsFailed     int
	CoveragePct     float64
}

type Tick struct {
	Deployments  []Deployment
	Incidents    []Incident
	BuildMetrics []BuildMetric
}

// GenerateTick produces one tick's worth of demo data at the given time.
func GenerateTick(r *rand.Rand, t time.Time) Tick {
	return Tick{
		Deployments:  genDeployments(r, t),
		Incidents:    genIncidents(r, t),
		BuildMetrics: genBuildMetrics(r, t),
	}
}

func genDeployments(r *rand.Rand, t time.Time) []Deployment {
	hour := t.Hour()
	weekday := t.Weekday()

	rate := 0.3
	if weekday >= time.Monday && weekday <= time.Friday {
		rate = 1.0
		if hour >= 9 && hour <= 17 {
			rate = 2.5
		}

		if weekday == time.Friday && hour >= 15 {
			rate = 3.5
		}
	}

	count := mockmetrics.Poisson(r, rate)
	out := make([]Deployment, 0, count)

	for range count {
		success := r.Float64() < 0.92
		if weekday == time.Friday && hour >= 15 {
			success = r.Float64() < 0.78
		}

		jitter := time.Duration(r.Intn(int(TickInterval.Seconds()))) * time.Second

		out = append(out, Deployment{
			Time:            t.Add(jitter),
			Service:         services[r.Intn(len(services))],
			Environment:     environments[r.Intn(len(environments))],
			DurationSeconds: math.Abs(mockmetrics.Normal(r, 120, 45)),
			Success:         success,
			Rollback:        !success && r.Float64() < 0.6,
		})
	}

	return out
}

func genIncidents(r *rand.Rand, t time.Time) []Incident {
	hour := t.Hour()
	weekday := t.Weekday()

	rate := 0.05
	if weekday >= time.Monday && weekday <= time.Friday {
		rate = 0.15
		if weekday == time.Friday && hour >= 15 {
			rate = 0.35
		}
	}

	count := mockmetrics.Poisson(r, rate)
	out := make([]Incident, 0, count)

	for range count {
		severity := severities[r.Intn(len(severities))]

		var detectMean, resolveMean float64

		switch severity {
		case "critical":
			detectMean, resolveMean = 5, 45
		case "major":
			detectMean, resolveMean = 15, 90
		case "minor":
			detectMean, resolveMean = 30, 120
		}

		jitter := time.Duration(r.Intn(int(TickInterval.Seconds()))) * time.Second

		out = append(out, Incident{
			Time:                  t.Add(jitter),
			Severity:              severity,
			Service:               services[r.Intn(len(services))],
			TimeToDetectMinutes:   math.Abs(mockmetrics.Normal(r, detectMean, detectMean*0.4)),
			TimeToResolveMinutes:  math.Abs(mockmetrics.Normal(r, resolveMean, resolveMean*0.3)),
		})
	}

	return out
}

func genBuildMetrics(r *rand.Rand, t time.Time) []BuildMetric {
	hour := t.Hour()
	weekday := t.Weekday()

	rate := 0.5
	if weekday >= time.Monday && weekday <= time.Friday {
		rate = 2.0
		if hour >= 9 && hour <= 17 {
			rate = 5.0
		}
	}

	count := mockmetrics.Poisson(r, rate)
	out := make([]BuildMetric, 0, count)

	for range count {
		testsFailed := 0
		if r.Float64() < 0.15 {
			testsFailed = mockmetrics.Poisson(r, 3)
		}

		jitter := time.Duration(r.Intn(int(TickInterval.Seconds()))) * time.Second

		out = append(out, BuildMetric{
			Time:            t.Add(jitter),
			Repository:      repositories[r.Intn(len(repositories))],
			Branch:          branches[r.Intn(len(branches))],
			DurationSeconds: math.Abs(mockmetrics.Normal(r, 180, 60)),
			TestCount:       mockmetrics.RandCount(r, 850, 0.1),
			TestsFailed:     testsFailed,
			CoveragePct:     mockmetrics.Clamp(mockmetrics.Normal(r, 72, 8), 20, 98),
		})
	}

	return out
}
