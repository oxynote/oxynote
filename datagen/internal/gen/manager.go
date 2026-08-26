// Package gen generates the mock engineering-culture metrics the demo
// Prometheus scrapes.
package gen

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/oxynote/oxynote/datagen/internal/mockmetrics"
	"github.com/oxynote/oxynote/server/core/pkg/metricutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/prometheus/client_golang/prometheus"
)

// _genInterval defines how often metrics are refreshed.
const _genInterval = 5 * time.Minute

// _labelOutcome names the label both outcome-split gauges are broken down by.
const _labelOutcome = "outcome"

// Sampling parameters of the friday_deploys gauge, whose two buckets are
// derived from one another instead of being sampled independently.
const (
	// _fridayDeployMean is the weekly number of Friday deploys attempted.
	_fridayDeployMean = 30_000

	// _fridayDeployRelStdDev spreads that count tick to tick.
	_fridayDeployRelStdDev = 0.15

	// _fridayDeploySuccessMean is the mean fraction of attempts that survive.
	_fridayDeploySuccessMean = 0.68

	// _fridayDeploySuccessStdDev spreads that fraction tick to tick.
	_fridayDeploySuccessStdDev = 0.12
)

// continuousMetric holds the running state for a single drifting gauge.
type continuousMetric struct {
	// value is the current gauge reading, updated on each tick.
	value float64

	// p holds the generation parameters governing drift and mean reversion.
	p mockmetrics.GaugeParams
}

// newContinuous creates a continuousMetric with its value initialized to p.Start (clamped to [p.Min, p.Max]).
func newContinuous(p mockmetrics.GaugeParams) *continuousMetric {
	return &continuousMetric{value: mockmetrics.Clamp(p.Start, p.Min, p.Max), p: p}
}

// tick advances the metric by one step and returns the new value.
func (c *continuousMetric) tick(r *rand.Rand) float64 {
	v := c.value
	v += c.p.MeanReversion * (c.p.Target - v)
	v += c.p.DriftPerStep + mockmetrics.Normal(r, 0, c.p.NoiseStdDev)

	if r.Float64() < c.p.SpikeChance {
		v += mockmetrics.Normal(r, 0, c.p.SpikeStdDev)
	}

	v = mockmetrics.Clamp(v, c.p.Min, c.p.Max)
	c.value = v

	return v
}

// Manager generates mock engineering-culture metrics and exposes them via Prometheus.
type Manager struct {
	// r is the shared random source used for all metric generation.
	r *rand.Rand

	// log is the structured logger.
	log *slog.Logger

	// interval defines how often metrics are refreshed.
	interval time.Duration

	// Continuous metric states — these drift smoothly between ticks.

	// tempFixAge states by original_comment label — hacks live the longest.
	tempFixAgeTemp  *continuousMetric
	tempFixAgeHack  *continuousMetric
	tempFixAgeFixme *continuousMetric
	tempFixAgeTodo  *continuousMetric

	// deployConf states by vibe label — oncall_already_paged is grimly low.
	deployConfFeelingLucky *continuousMetric
	deployConfNervous      *continuousMetric
	deployConfOncallPaged  *continuousMetric

	// prReviewState tracks the median PR Time to First Review in minutes.
	prReviewState *continuousMetric

	// hotfix states by root_cause label.
	hotfixWOMachine     *continuousMetric
	hotfixNotMyFault    *continuousMetric
	hotfixTimezoneIssue *continuousMetric

	// womMachine states by platform label — my_special_setup wins.
	womLinux        *continuousMetric
	womMac          *continuousMetric
	womSpecialSetup *continuousMetric

	// staleBranch states by owner_status label.
	staleBranchStillEmployed *continuousMetric
	staleBranchLeftCompany   *continuousMetric
	staleBranchNobodyKnows   *continuousMetric

	// featureFlag states by removal_likelihood label — never is the biggest bucket.
	featureFlagMaybeSomeday   *continuousMetric
	featureFlagNever          *continuousMetric
	featureFlagWhatDoesThisDo *continuousMetric

	// Prometheus gauge vecs.

	// meetingsCouldbDoc counts weekly meetings that could've been a document.
	meetingsCouldbDoc metricutil.GaugeVec

	// circleBackCount counts weekly "let's circle back" mentions.
	circleBackCount metricutil.GaugeVec

	// fridayDeploys counts weekly Friday deploys labelled by outcome (attempted/successful).
	fridayDeploys metricutil.GaugeVec

	// featureFlagGrave is the number of feature flags that have outlived their purpose.
	featureFlagGrave metricutil.GaugeVec

	// pizzaFridays is the number of Pizza Fridays scheduled in the next 30 days.
	pizzaFridays metricutil.GaugeVec

	// deployConfidence is the self-reported deploy confidence index (0–100).
	deployConfidence metricutil.GaugeVec

	// shipItApprovals counts weekly "ship it" approvals left after 5 pm.
	shipItApprovals metricutil.GaugeVec

	// prsMergedFriday counts weekly PRs merged on Fridays after 3 pm.
	prsMergedFriday metricutil.GaugeVec

	// tempFixAge is the age in days of the oldest surviving "temporary" fix.
	tempFixAge metricutil.GaugeVec

	// staleBranches is the number of branches with no activity for more than 90 days.
	staleBranches metricutil.GaugeVec

	// prReviewTime is the median time from PR open to first review comment in minutes.
	prReviewTime metricutil.GaugeVec

	// hotfixesRelease is the average number of hotfixes per release.
	hotfixesRelease metricutil.GaugeVec

	// secretsDetected counts secrets detected in commits over the past week.
	secretsDetected metricutil.GaugeVec

	// womMachineRate is the self-reported "works on my machine" incidence rate (0–100).
	womMachineRate metricutil.GaugeVec

	// quickSyncMeetings counts weekly "quick sync" meetings scheduled same-day.
	quickSyncMeetings metricutil.GaugeVec
}

// NewManager creates a ready-to-run Manager.
//
//nolint:maintidx // a flat table of tuned gauge parameters, not logic
func NewManager(fc metricutil.Factory, log *slog.Logger) *Manager {
	newGaugeVec := func(name, help string, labels ...string) metricutil.GaugeVec {
		return fc.NewGaugeVec(metricutil.Options{Name: name, Help: help}, labels)
	}

	return &Manager{
		r:        mockmetrics.NewRand(timeutil.Now().UnixNano()),
		log:      log,
		interval: _genInterval,
		// Temporary fix age by comment type — hacks accumulate and never leave.
		tempFixAgeTemp: newContinuous(mockmetrics.GaugeParams{
			Min:           365,
			Max:           5_000,
			Start:         2_000,
			Target:        2_200,
			MeanReversion: 0.015,
			NoiseStdDev:   80,
			DriftPerStep:  0.2,
			SpikeChance:   0.08,
			SpikeStdDev:   200,
		}),
		tempFixAgeHack: newContinuous(mockmetrics.GaugeParams{
			Min:           1_000,
			Max:           18_250,
			Start:         6_000,
			Target:        7_000,
			MeanReversion: 0.008,
			NoiseStdDev:   150,
			DriftPerStep:  0.4,
			SpikeChance:   0.07,
			SpikeStdDev:   400,
		}),
		tempFixAgeFixme: newContinuous(mockmetrics.GaugeParams{
			Min:           180,
			Max:           4_000,
			Start:         1_500,
			Target:        1_700,
			MeanReversion: 0.015,
			NoiseStdDev:   60,
			DriftPerStep:  0.2,
			SpikeChance:   0.08,
			SpikeStdDev:   150,
		}),
		tempFixAgeTodo: newContinuous(mockmetrics.GaugeParams{
			Min:           500,
			Max:           10_000,
			Start:         3_500,
			Target:        4_000,
			MeanReversion: 0.008,
			NoiseStdDev:   100,
			DriftPerStep:  0.3,
			SpikeChance:   0.08,
			SpikeStdDev:   250,
		}),

		// Deploy confidence by vibe.
		deployConfFeelingLucky: newContinuous(mockmetrics.GaugeParams{
			Min:           60,
			Max:           100,
			Start:         78,
			Target:        80,
			MeanReversion: 0.08,
			NoiseStdDev:   3,
			SpikeChance:   0.05,
			SpikeStdDev:   8,
		}),
		deployConfNervous: newContinuous(mockmetrics.GaugeParams{
			Min:           30,
			Max:           75,
			Start:         52,
			Target:        55,
			MeanReversion: 0.08,
			NoiseStdDev:   4,
			SpikeChance:   0.07,
			SpikeStdDev:   10,
		}),
		deployConfOncallPaged: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           40,
			Start:         18,
			Target:        20,
			MeanReversion: 0.1,
			NoiseStdDev:   3,
			SpikeChance:   0.1,
			SpikeStdDev:   8,
		}),

		// PR review time — global median, no breakdown needed.
		prReviewState: newContinuous(mockmetrics.GaugeParams{
			Min:           5,
			Max:           10_080,
			Start:         280,
			Target:        300,
			MeanReversion: 0.1,
			NoiseStdDev:   25,
			SpikeChance:   0.08,
			SpikeStdDev:   180,
		}),

		// Hotfixes per release by root cause — timezone_issue is rarest.
		hotfixWOMachine: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           12,
			Start:         0.9,
			Target:        1.0,
			MeanReversion: 0.1,
			NoiseStdDev:   0.2,
			SpikeChance:   0.05,
			SpikeStdDev:   1,
		}),
		hotfixNotMyFault: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           10,
			Start:         0.8,
			Target:        0.9,
			MeanReversion: 0.1,
			NoiseStdDev:   0.2,
			SpikeChance:   0.05,
			SpikeStdDev:   0.8,
		}),
		hotfixTimezoneIssue: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           5,
			Start:         0.4,
			Target:        0.45,
			MeanReversion: 0.1,
			NoiseStdDev:   0.1,
			SpikeChance:   0.04,
			SpikeStdDev:   0.5,
		}),

		// "Works on my machine" rate by platform — my_special_setup is a disaster.
		womLinux: newContinuous(mockmetrics.GaugeParams{
			Min:           10,
			Max:           60,
			Start:         28,
			Target:        30,
			MeanReversion: 0.06,
			NoiseStdDev:   2,
			SpikeChance:   0.05,
			SpikeStdDev:   6,
		}),
		womMac: newContinuous(mockmetrics.GaugeParams{
			Min:           25,
			Max:           75,
			Start:         48,
			Target:        50,
			MeanReversion: 0.06,
			NoiseStdDev:   2.5,
			SpikeChance:   0.06,
			SpikeStdDev:   8,
		}),
		womSpecialSetup: newContinuous(mockmetrics.GaugeParams{
			Min:           60,
			Max:           100,
			Start:         82,
			Target:        85,
			MeanReversion: 0.05,
			NoiseStdDev:   3,
			SpikeChance:   0.07,
			SpikeStdDev:   5,
		}),

		// Stale branches by owner status — left_the_company only grows.
		staleBranchStillEmployed: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           800_000,
			Start:         95_000,
			Target:        120_000,
			MeanReversion: 0.03,
			NoiseStdDev:   6_000,
			DriftPerStep:  200,
			SpikeChance:   0.08,
			SpikeStdDev:   30_000,
		}),
		staleBranchLeftCompany: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           700_000,
			Start:         85_000,
			Target:        100_000,
			MeanReversion: 0.015,
			NoiseStdDev:   5_000,
			DriftPerStep:  300,
			SpikeChance:   0.07,
			SpikeStdDev:   25_000,
		}),
		staleBranchNobodyKnows: newContinuous(mockmetrics.GaugeParams{
			Min:           0,
			Max:           500_000,
			Start:         50_000,
			Target:        80_000,
			MeanReversion: 0.008,
			NoiseStdDev:   4_000,
			DriftPerStep:  100,
			SpikeChance:   0.07,
			SpikeStdDev:   20_000,
		}),

		// Feature flag graveyard by removal likelihood — never is the dominant bucket.
		featureFlagMaybeSomeday: newContinuous(mockmetrics.GaugeParams{
			Min:           20_000,
			Max:           2_000_000,
			Start:         200_000,
			Target:        240_000,
			MeanReversion: 0.02,
			NoiseStdDev:   15_000,
			DriftPerStep:  400,
			SpikeChance:   0.08,
			SpikeStdDev:   60_000,
		}),
		featureFlagNever: newContinuous(mockmetrics.GaugeParams{
			Min:           30_000,
			Max:           3_000_000,
			Start:         250_000,
			Target:        280_000,
			MeanReversion: 0.02,
			NoiseStdDev:   18_000,
			DriftPerStep:  500,
			SpikeChance:   0.09,
			SpikeStdDev:   70_000,
		}),
		featureFlagWhatDoesThisDo: newContinuous(mockmetrics.GaugeParams{
			Min:           5_000,
			Max:           1_000_000,
			Start:         70_000,
			Target:        80_000,
			MeanReversion: 0.02,
			NoiseStdDev:   8_000,
			DriftPerStep:  200,
			SpikeChance:   0.08,
			SpikeStdDev:   30_000,
		}),

		meetingsCouldbDoc: newGaugeVec(
			"meetings_could_be_doc_count",
			"Weekly count of meetings that could've been a document.",
			"excuse",
		),
		circleBackCount: newGaugeVec(
			"circle_back_count",
			`Weekly count of "let's circle back" mentions.`,
			_labelOutcome,
		),
		fridayDeploys: newGaugeVec(
			"friday_deploys_count",
			"Weekly Friday deploy counts.",
			_labelOutcome,
		),
		featureFlagGrave: newGaugeVec(
			"feature_flag_graveyard_size",
			"Number of feature flags that have outlived their purpose.",
			"removal_likelihood",
		),
		pizzaFridays: newGaugeVec(
			"pizza_fridays_next_30_days",
			"Pizza Fridays scheduled in the next 30 days.",
			"pizza_count",
		),
		deployConfidence: newGaugeVec(
			"deploy_confidence_index",
			"Self-reported deploy confidence index (0–100).",
			"vibe",
		),
		shipItApprovals: newGaugeVec(
			"ship_it_approvals_after_5pm_count",
			`Weekly count of "ship it" PR approvals left after 5 pm.`,
			"reviewer_state",
		),
		prsMergedFriday: newGaugeVec(
			"prs_merged_friday_afternoon_count",
			"Weekly count of PRs merged on Fridays after 3 pm.",
			"confidence",
		),
		tempFixAge: newGaugeVec(
			"temporary_fix_age_days",
			`Age in days of the oldest surviving "temporary" fix.`,
			"original_comment",
		),
		staleBranches: newGaugeVec(
			"stale_branches_total",
			"Number of branches with no activity for more than 90 days.",
			"owner_status",
		),
		prReviewTime: newGaugeVec(
			"pr_time_to_first_review_minutes",
			"Median time from PR open to first review comment (minutes).",
		),
		hotfixesRelease: newGaugeVec(
			"hotfixes_per_release",
			"Average number of hotfixes per release.",
			"root_cause",
		),
		secretsDetected: newGaugeVec(
			"secrets_detected_commits_count",
			"Secrets detected in commits over the past week.",
			"severity",
		),
		womMachineRate: newGaugeVec(
			"works_on_my_machine_rate",
			`Self-reported "works on my machine" incidence rate (0–100).`,
			"platform",
		),
		quickSyncMeetings: newGaugeVec(
			"quick_sync_meetings_same_day_count",
			`Weekly count of "quick sync" meetings scheduled same-day.`,
			"duration",
		),
	}
}

// Run starts the generation loop. It blocks until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	timeutil.NewPeriodicExec(
		m.interval,
		0,
		func(context.Context) { m.generateData() },
		nil,
		true,
	).Start(ctx)
}

// countBucket specifies the sampling parameters of one labelled bucket of a
// gauge that is resampled from scratch on every tick.
type countBucket struct {
	// label is the label value the bucket is published under.
	label string

	// mean is the average count sampled for the bucket each tick.
	mean float64

	// relStdDev is the bucket's standard deviation as a fraction of mean.
	relStdDev float64
}

// setCounts resamples every bucket of a gauge and publishes the results
// under the given label key.
func (m *Manager) setCounts(vec metricutil.GaugeVec, key string, bb []countBucket) {
	for _, b := range bb {
		vec.With(prometheus.Labels{key: b.label}).
			Set(float64(mockmetrics.RandCount(m.r, b.mean, b.relStdDev)))
	}
}

// generateData ticks all metrics and pushes values to their Prometheus gauges.
func (m *Manager) generateData() {
	r := m.r

	// --- Count metrics (resampled from scratch each tick) ---

	// Means are aggregate weekly totals across ~10k organisations.

	// meetings_could_be_doc: split by the excuse used to justify the meeting.
	m.setCounts(m.meetingsCouldbDoc, "excuse", []countBucket{
		{label: "alignment", mean: 42_000, relStdDev: 0.15},
		{label: "bandwidth", mean: 36_000, relStdDev: 0.15},
		{label: "synergy", mean: 30_000, relStdDev: 0.15},
		{label: "optics", mean: 12_000, relStdDev: 0.20},
	})

	// circle_back_count: split by what actually happened afterward.
	m.setCounts(m.circleBackCount, _labelOutcome, []countBucket{
		{label: "circled_back", mean: 16_000, relStdDev: 0.20},
		{label: "forgotten", mean: 48_000, relStdDev: 0.15},
		{label: "rescheduled_to_circle_back", mean: 16_000, relStdDev: 0.20},
	})

	// friday_deploys: the successful bucket is a fraction of the attempted
	// one rather than an independent sample, so it cannot join a table.
	attempted := mockmetrics.RandCount(r, _fridayDeployMean, _fridayDeployRelStdDev)
	successRate := mockmetrics.Clamp(
		mockmetrics.Normal(r, _fridayDeploySuccessMean, _fridayDeploySuccessStdDev),
		0,
		1,
	)
	successful := int(mockmetrics.Clamp(float64(attempted)*successRate, 0, float64(attempted)))

	m.fridayDeploys.With(prometheus.Labels{_labelOutcome: "attempted"}).Set(float64(attempted))
	m.fridayDeploys.With(prometheus.Labels{_labelOutcome: "successful"}).Set(float64(successful))

	// pizza_fridays: split by how many pizzas were ordered.
	m.setCounts(m.pizzaFridays, "pizza_count", []countBucket{
		{label: "1_to_5", mean: 10_000, relStdDev: 0.15},
		{label: "5_to_10", mean: 7_000, relStdDev: 0.20},
		{label: "10_plus", mean: 3_000, relStdDev: 0.25},
	})

	// ship_it_approvals: split by reviewer's apparent state of mind.
	m.setCounts(m.shipItApprovals, "reviewer_state", []countBucket{
		{label: "sober", mean: 20_000, relStdDev: 0.15},
		{label: "one_beer_in", mean: 14_000, relStdDev: 0.20},
		{label: "heading_to_the_door", mean: 6_000, relStdDev: 0.25},
	})

	// prs_merged_friday: split by the deployer's stated confidence level.
	m.setCounts(m.prsMergedFriday, "confidence", []countBucket{
		{label: "yolo", mean: 20_000, relStdDev: 0.15},
		{label: "fingers_crossed", mean: 22_500, relStdDev: 0.15},
		{label: "vacation_starts_monday", mean: 7_500, relStdDev: 0.20},
	})

	// secrets_detected: split by severity — nuclear is rare but very bad.
	m.setCounts(m.secretsDetected, "severity", []countBucket{
		{label: "normal", mean: 2_100, relStdDev: 0.25},
		{label: "critical", mean: 750, relStdDev: 0.30},
		{label: "nuclear", mean: 150, relStdDev: 0.35},
	})

	// quick_sync_meetings: split by how "quick" they actually were.
	m.setCounts(m.quickSyncMeetings, "duration", []countBucket{
		{label: "10m_to_30m", mean: 33_000, relStdDev: 0.15},
		{label: "30m_to_1h", mean: 21_000, relStdDev: 0.15},
		{label: "1h_plus", mean: 6_000, relStdDev: 0.25},
	})

	// --- Continuous drifting metrics ---

	m.featureFlagGrave.With(prometheus.Labels{"removal_likelihood": "maybe_someday"}).Set(m.featureFlagMaybeSomeday.tick(r))
	m.featureFlagGrave.With(prometheus.Labels{"removal_likelihood": "never"}).Set(m.featureFlagNever.tick(r))
	m.featureFlagGrave.With(prometheus.Labels{"removal_likelihood": "what_does_this_do"}).Set(m.featureFlagWhatDoesThisDo.tick(r))

	m.deployConfidence.With(prometheus.Labels{"vibe": "feeling_lucky"}).Set(m.deployConfFeelingLucky.tick(r))
	m.deployConfidence.With(prometheus.Labels{"vibe": "nervous"}).Set(m.deployConfNervous.tick(r))
	m.deployConfidence.With(prometheus.Labels{"vibe": "oncall_already_paged"}).Set(m.deployConfOncallPaged.tick(r))

	m.tempFixAge.With(prometheus.Labels{"original_comment": "temp"}).Set(m.tempFixAgeTemp.tick(r))
	m.tempFixAge.With(prometheus.Labels{"original_comment": "hack"}).Set(m.tempFixAgeHack.tick(r))
	m.tempFixAge.With(prometheus.Labels{"original_comment": "fixme"}).Set(m.tempFixAgeFixme.tick(r))
	m.tempFixAge.With(prometheus.Labels{"original_comment": "todo"}).Set(m.tempFixAgeTodo.tick(r))

	m.staleBranches.With(prometheus.Labels{"owner_status": "still_employed"}).Set(m.staleBranchStillEmployed.tick(r))
	m.staleBranches.With(prometheus.Labels{"owner_status": "left_the_company"}).Set(m.staleBranchLeftCompany.tick(r))
	m.staleBranches.With(prometheus.Labels{"owner_status": "nobody_knows"}).Set(m.staleBranchNobodyKnows.tick(r))

	m.prReviewTime.With(prometheus.Labels{}).Set(m.prReviewState.tick(r))

	m.hotfixesRelease.With(prometheus.Labels{"root_cause": "works_on_my_machine"}).Set(m.hotfixWOMachine.tick(r))
	m.hotfixesRelease.With(prometheus.Labels{"root_cause": "not_my_fault"}).Set(m.hotfixNotMyFault.tick(r))
	m.hotfixesRelease.With(prometheus.Labels{"root_cause": "timezone_issue"}).Set(m.hotfixTimezoneIssue.tick(r))

	m.womMachineRate.With(prometheus.Labels{"platform": "linux"}).Set(m.womLinux.tick(r))
	m.womMachineRate.With(prometheus.Labels{"platform": "mac"}).Set(m.womMac.tick(r))
	m.womMachineRate.With(prometheus.Labels{"platform": "my_special_setup"}).Set(m.womSpecialSetup.tick(r))

	m.log.Debug("mock metrics generated")
}
