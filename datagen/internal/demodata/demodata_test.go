package demodata

import (
	"math/rand"
	"slices"
	"testing"
	"time"

	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// Reference instants for each rate branch the generators switch on.
var (
	// _sunday is outside the working week entirely.
	_sunday = time.Date(2024, time.January, 7, 12, 0, 0, 0, time.UTC)

	// _tuesdayNight is a weekday outside office hours.
	_tuesdayNight = time.Date(2024, time.January, 9, 3, 0, 0, 0, time.UTC)

	// _tuesdayOffice is a weekday inside office hours.
	_tuesdayOffice = time.Date(2024, time.January, 9, 11, 0, 0, 0, time.UTC)

	// _fridayAfternoon is when everything gets worse.
	_fridayAfternoon = time.Date(2024, time.January, 12, 16, 0, 0, 0, time.UTC)
)

func Test_GenerateTick(t *testing.T) {
	t.Parallel()

	// a tick carries whatever each generator produced for the same instant.
	tick := GenerateTick(mockmetrics.NewRand(1), _tuesdayOffice)

	assert.Equal(t, genDeployments(mockmetrics.NewRand(1), _tuesdayOffice), tick.Deployments)

	// the generators share one source, so replaying them in order is the
	// only way to reproduce the later two.
	r := mockmetrics.NewRand(1)
	genDeployments(r, _tuesdayOffice)

	assert.Equal(t, genIncidents(r, _tuesdayOffice), tick.Incidents)
	assert.Equal(t, genBuildMetrics(r, _tuesdayOffice), tick.BuildMetrics)
}

func Test_genDeployments(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Time time.Time

		// MinRows is the number of rows the case must produce across its
		// draws for the branch to count as exercised.
		MinRows int
	}{
		"Weekend runs at the lowest rate": {
			Time:    _sunday,
			MinRows: 1,
		},
		"Weekday outside office hours": {
			Time:    _tuesdayNight,
			MinRows: 1,
		},
		"Weekday office hours": {
			Time:    _tuesdayOffice,
			MinRows: 1,
		},
		"Friday afternoon": {
			Time:    _fridayAfternoon,
			MinRows: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				r    = mockmetrics.NewRand(1)
				rows = make([]Deployment, 0, 200)
			)

			for range 200 {
				rows = append(rows, genDeployments(r, c.Time)...)
			}

			require.GreaterOrEqual(t, len(rows), c.MinRows)

			for _, d := range rows {
				assert.Contains(t, _services, d.Service)
				assert.Contains(t, _environments, d.Environment)
				assert.GreaterOrEqual(t, d.DurationSeconds, 0.0)
				assertJittered(t, c.Time, d.Time)

				// a successful deployment is never rolled back.
				if d.Success {
					assert.False(t, d.Rollback)
				}
			}
		})
	}

	// the branches exist to make busier periods busier, so the row counts
	// have to come out ordered.
	assert.Less(t, countDeployments(t, _sunday), countDeployments(t, _tuesdayOffice))
	assert.Less(t, countDeployments(t, _tuesdayNight), countDeployments(t, _tuesdayOffice))
	assert.Less(t, countDeployments(t, _tuesdayOffice), countDeployments(t, _fridayAfternoon))
}

func Test_genIncidents(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Time time.Time
	}{
		"Weekend runs at the lowest rate": {Time: _sunday},
		"Weekday":                         {Time: _tuesdayOffice},
		"Friday afternoon":                {Time: _fridayAfternoon},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				r    = mockmetrics.NewRand(1)
				rows = make([]Incident, 0, 500)
			)

			for range 500 {
				rows = append(rows, genIncidents(r, c.Time)...)
			}

			require.NotEmpty(t, rows)

			var seen []string

			for _, i := range rows {
				assert.Contains(t, _severities, i.Severity)
				assert.Contains(t, _services, i.Service)
				assert.GreaterOrEqual(t, i.TimeToDetectMinutes, 0.0)
				assert.GreaterOrEqual(t, i.TimeToResolveMinutes, 0.0)
				assertJittered(t, c.Time, i.Time)

				if !slices.Contains(seen, i.Severity) {
					seen = append(seen, i.Severity)
				}
			}

			// every severity carries its own detect/resolve means, so the
			// draw has to cover all three branches of the switch.
			assert.ElementsMatch(t, _severities, seen)
		})
	}

	// busier periods must come out busier.
	assert.Less(t, countIncidents(t, _sunday), countIncidents(t, _tuesdayOffice))
	assert.Less(t, countIncidents(t, _tuesdayOffice), countIncidents(t, _fridayAfternoon))
}

func Test_genBuildMetrics(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Time time.Time
	}{
		"Weekend runs at the lowest rate": {Time: _sunday},
		"Weekday outside office hours":    {Time: _tuesdayNight},
		"Weekday office hours":            {Time: _tuesdayOffice},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				r    = mockmetrics.NewRand(1)
				rows = make([]BuildMetric, 0, 200)
			)

			for range 200 {
				rows = append(rows, genBuildMetrics(r, c.Time)...)
			}

			require.NotEmpty(t, rows)

			var sawFailures bool

			for _, b := range rows {
				assert.Contains(t, _repositories, b.Repository)
				assert.Contains(t, _branches, b.Branch)
				assert.GreaterOrEqual(t, b.DurationSeconds, 0.0)
				assert.GreaterOrEqual(t, b.TestCount, 0)
				assert.GreaterOrEqual(t, b.TestsFailed, 0)
				assert.GreaterOrEqual(t, b.CoveragePct, float64(_buildCoverageMin))
				assert.LessOrEqual(t, b.CoveragePct, float64(_buildCoverageMax))
				assertJittered(t, c.Time, b.Time)

				if b.TestsFailed > 0 {
					sawFailures = true
				}
			}

			// the failing-build branch is rare but has to be reached.
			assert.True(t, sawFailures)
		})
	}

	// busier periods must come out busier.
	assert.Less(t, countBuilds(t, _sunday), countBuilds(t, _tuesdayOffice))
	assert.Less(t, countBuilds(t, _tuesdayNight), countBuilds(t, _tuesdayOffice))
}

// assertJittered checks that a generated row landed inside the tick it was
// generated for.
func assertJittered(t *testing.T, tick, got time.Time) {
	t.Helper()

	assert.False(t, got.Before(tick))
	assert.True(t, got.Before(tick.Add(TickInterval)))
}

// countDeployments totals the deployments generated for one instant over
// enough draws for the rates to separate.
func countDeployments(t *testing.T, at time.Time) int {
	t.Helper()

	return countRows(func(r *rand.Rand) int { return len(genDeployments(r, at)) })
}

// countIncidents does the same for incidents.
func countIncidents(t *testing.T, at time.Time) int {
	t.Helper()

	return countRows(func(r *rand.Rand) int { return len(genIncidents(r, at)) })
}

// countBuilds does the same for builds.
func countBuilds(t *testing.T, at time.Time) int {
	t.Helper()

	return countRows(func(r *rand.Rand) int { return len(genBuildMetrics(r, at)) })
}

// countRows sums what fn generates over a fixed number of draws from a fixed
// seed, so the totals of two instants are comparable.
func countRows(fn func(*rand.Rand) int) int {
	r := mockmetrics.NewRand(1)

	var total int

	for range 2_000 {
		total += fn(r)
	}

	return total
}
