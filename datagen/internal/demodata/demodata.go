// Package demodata synthesises the deployment, incident and build rows the
// demo data sources are filled with.
package demodata

import (
	"math"
	"math/rand"
	"time"

	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
)

// TickInterval defines how much demo time a single generated tick covers.
const TickInterval = 5 * time.Minute

// Deployment sampling parameters.
const (
	// _deploySuccessRate is the share of deployments that succeed outside
	// Friday afternoons.
	_deploySuccessRate = 0.92

	// _deployFridaySuccessRate is that share once Friday afternoon starts.
	_deployFridaySuccessRate = 0.78

	// _deployRollbackRate is the share of failed deployments rolled back.
	_deployRollbackRate = 0.6

	// _deployDurationMean and _deployDurationStdDev shape how long a
	// deployment takes, in seconds.
	_deployDurationMean   = 120
	_deployDurationStdDev = 45
)

// Incident sampling parameters.
const (
	// _incidentDetectRelStdDev spreads time-to-detect around its severity's
	// mean, as a fraction of that mean.
	_incidentDetectRelStdDev = 0.4

	// _incidentResolveRelStdDev does the same for time-to-resolve.
	_incidentResolveRelStdDev = 0.3
)

// Build sampling parameters.
const (
	// _buildFailureRate is the share of builds that fail any tests at all.
	_buildFailureRate = 0.15

	// _buildFailedTestsMean is the average number of tests that fail when a
	// build fails.
	_buildFailedTestsMean = 3

	// _buildDurationMean and _buildDurationStdDev shape how long a build
	// takes, in seconds.
	_buildDurationMean   = 180
	_buildDurationStdDev = 60

	// _buildTestCountMean and _buildTestCountRelStdDev shape how many tests
	// a build runs.
	_buildTestCountMean      = 850
	_buildTestCountRelStdDev = 0.1

	// _buildCoverageMean and _buildCoverageStdDev shape reported coverage,
	// which is then held within [_buildCoverageMin, _buildCoverageMax].
	_buildCoverageMean   = 72
	_buildCoverageStdDev = 8
	_buildCoverageMin    = 20
	_buildCoverageMax    = 98
)

var (
	// _services holds the service names demo rows are attributed to.
	_services = []string{"api-gateway", "user-service", "billing-service", "notification-service", "search-service"}

	// _environments holds the environments demo deployments target.
	_environments = []string{"production", "staging"}

	// _severities holds the severity levels demo incidents are raised at.
	_severities = []string{"critical", "major", "minor"}

	// _repositories holds the repository names demo builds run against.
	_repositories = []string{"frontend", "backend", "infra", "mobile", "shared-libs"}

	// _branches holds the branch names demo builds run on.
	_branches = []string{"main", "develop", "feature"}
)

// Deployment specifies a single demo deployment event.
type Deployment struct {
	// Time specifies when the deployment happened.
	Time time.Time

	// Service specifies the deployed service.
	Service string

	// Environment specifies the environment deployed to.
	Environment string

	// DurationSeconds specifies how long the deployment took.
	DurationSeconds float64

	// Success indicates whether the deployment succeeded.
	Success bool

	// Rollback indicates whether the deployment was rolled back.
	Rollback bool
}

// Incident specifies a single demo incident event.
type Incident struct {
	// Time specifies when the incident started.
	Time time.Time

	// Severity specifies how severe the incident was.
	Severity string

	// Service specifies the affected service.
	Service string

	// TimeToDetectMinutes specifies how long the incident went unnoticed.
	TimeToDetectMinutes float64

	// TimeToResolveMinutes specifies how long the incident took to resolve.
	TimeToResolveMinutes float64
}

// BuildMetric specifies a single demo CI build result.
type BuildMetric struct {
	// Time specifies when the build ran.
	Time time.Time

	// Repository specifies the built repository.
	Repository string

	// Branch specifies the built branch.
	Branch string

	// DurationSeconds specifies how long the build took.
	DurationSeconds float64

	// TestCount specifies how many tests the build ran.
	TestCount int

	// TestsFailed specifies how many of those tests failed.
	TestsFailed int

	// CoveragePct specifies the coverage percentage the build reported.
	CoveragePct float64
}

// Tick specifies everything generated for a single interval.
type Tick struct {
	// Deployments holds the deployments generated for the tick.
	Deployments []Deployment

	// Incidents holds the incidents generated for the tick.
	Incidents []Incident

	// BuildMetrics holds the build results generated for the tick.
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

// genDeployments generates the deployments of a single tick, at a rate that
// peaks during weekday office hours and again on Friday afternoons.
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
		success := r.Float64() < _deploySuccessRate
		if weekday == time.Friday && hour >= 15 {
			success = r.Float64() < _deployFridaySuccessRate
		}

		jitter := time.Duration(r.Intn(int(TickInterval.Seconds()))) * time.Second

		out = append(out, Deployment{
			Time:            t.Add(jitter),
			Service:         _services[r.Intn(len(_services))],
			Environment:     _environments[r.Intn(len(_environments))],
			DurationSeconds: math.Abs(mockmetrics.Normal(r, _deployDurationMean, _deployDurationStdDev)),
			Success:         success,
			Rollback:        !success && r.Float64() < _deployRollbackRate,
		})
	}

	return out
}

// genIncidents generates the incidents of a single tick, at a rate that rises
// on weekdays and spikes on Friday afternoons.
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
		severity := _severities[r.Intn(len(_severities))]

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
			Time:                 t.Add(jitter),
			Severity:             severity,
			Service:              _services[r.Intn(len(_services))],
			TimeToDetectMinutes:  math.Abs(mockmetrics.Normal(r, detectMean, detectMean*_incidentDetectRelStdDev)),
			TimeToResolveMinutes: math.Abs(mockmetrics.Normal(r, resolveMean, resolveMean*_incidentResolveRelStdDev)),
		})
	}

	return out
}

// genBuildMetrics generates the CI builds of a single tick, at a rate that
// peaks during weekday office hours.
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
		if r.Float64() < _buildFailureRate {
			testsFailed = mockmetrics.Poisson(r, _buildFailedTestsMean)
		}

		jitter := time.Duration(r.Intn(int(TickInterval.Seconds()))) * time.Second

		out = append(out, BuildMetric{
			Time:            t.Add(jitter),
			Repository:      _repositories[r.Intn(len(_repositories))],
			Branch:          _branches[r.Intn(len(_branches))],
			DurationSeconds: math.Abs(mockmetrics.Normal(r, _buildDurationMean, _buildDurationStdDev)),
			TestCount:       mockmetrics.RandCount(r, _buildTestCountMean, _buildTestCountRelStdDev),
			TestsFailed:     testsFailed,
			CoveragePct: mockmetrics.Clamp(
				mockmetrics.Normal(r, _buildCoverageMean, _buildCoverageStdDev),
				_buildCoverageMin,
				_buildCoverageMax,
			),
		})
	}

	return out
}
