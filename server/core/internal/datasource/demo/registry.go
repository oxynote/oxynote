package demo

import (
	"math"
	"slices"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// _namespace prefixes every demo metric name.
const _namespace = "engineering_"

// _labelOutcome names the label both outcome-split families are broken
// down by.
const _labelOutcome = "outcome"

// Sampling parameters of the friday_deploys family, whose two buckets are
// derived from one another instead of being sampled independently.
const (
	// _fridayDeployMean is the weekly number of Friday deploys attempted.
	_fridayDeployMean = 30_000

	// _fridayDeployRelStdDev spreads that count tick to tick.
	_fridayDeployRelStdDev = 0.15

	// _fridayDeploySuccessMean is the mean fraction of attempts that
	// survive.
	_fridayDeploySuccessMean = 0.68

	// _fridayDeploySuccessStdDev spreads that fraction tick to tick.
	_fridayDeploySuccessStdDev = 0.12
)

// bucket is one series of a metric family.
type bucket struct {
	// label is the label value the bucket is published under. It is
	// empty for a family that publishes a single unlabelled series.
	label string

	// value reports the bucket's reading at the given tick.
	value func(tick int64) float64
}

// family is one metric family of the demo timeline.
type family struct {
	// name is the metric name the family is published under.
	name string

	// help describes what the family measures.
	help string

	// label is the label key the family's buckets are broken down by.
	// It is empty for a family that publishes a single unlabelled
	// series.
	label string

	// buckets are the family's series, one per label value.
	buckets []bucket
}

// registry is everything the demo publishes. One is built per client and
// read by every query it answers: the walks behind it cache the history
// they replay, so a registry that is rebuilt is a cache that is thrown
// away.
type registry []family

// forEach calls fn for every series the matchers select, in declaration
// order.
func (r registry) forEach(mm []*labels.Matcher, fn func(lbls labels.Labels, value func(tick int64) float64)) {
	for _, f := range r {
		for _, b := range f.buckets {
			lbls := seriesLabels(f, b)

			if matches(lbls, mm) {
				fn(lbls, b.value)
			}
		}
	}
}

// labelNames returns every label name the given matcher sets select,
// sorted. The sets are unioned, the way a series endpoint handed several
// selectors answers with everything any one of them names.
func (r registry) labelNames(sets [][]*labels.Matcher) []string {
	names := map[string]struct{}{}

	for _, set := range sets {
		r.forEach(set, func(lbls labels.Labels, _ func(tick int64) float64) {
			lbls.Range(func(l labels.Label) {
				names[l.Name] = struct{}{}
			})
		})
	}

	return sortedKeys(names)
}

// labelValues returns every value the given matcher sets carry for the
// named label, sorted and unioned across the sets.
func (r registry) labelValues(name string, sets [][]*labels.Matcher) []string {
	values := map[string]struct{}{}

	for _, set := range sets {
		r.forEach(set, func(lbls labels.Labels, _ func(tick int64) float64) {
			if v := lbls.Get(name); v != "" {
				values[v] = struct{}{}
			}
		})
	}

	return sortedKeys(values)
}

// seriesLabels returns the label set the given bucket of the given family
// is published under.
func seriesLabels(f family, b bucket) labels.Labels {
	if f.label == "" {
		return labels.FromStrings(model.MetricNameLabel, f.name)
	}

	return labels.FromStrings(model.MetricNameLabel, f.name, f.label, b.label)
}

// matches reports whether the label set satisfies every matcher.
func matches(lbls labels.Labels, mm []*labels.Matcher) bool {
	for _, m := range mm {
		if !m.Matches(lbls.Get(m.Name)) {
			return false
		}
	}

	return true
}

// sortedKeys returns the set's members in sorted order, and an empty
// slice when it holds none.
func sortedKeys(set map[string]struct{}) []string {
	kk := make([]string, 0, len(set))

	for k := range set {
		kk = append(kk, k)
	}

	slices.Sort(kk)

	return kk
}

// builder assigns every series its own random stream as the registry is
// declared, so the table below carries tuned parameters rather than
// hand-written seeds, and no two series ever draw the same numbers.
type builder struct {
	// seed is the last seed handed out.
	seed int64
}

// next hands out the next unused seed.
func (b *builder) next() int64 {
	b.seed++

	return b.seed
}

// count declares a bucket resampled from scratch every tick.
func (b *builder) count(label string, mean, relStdDev float64) bucket {
	seed := b.next()

	return bucket{
		label: label,
		value: func(tick int64) float64 {
			return countAt(seed, tick, mean, relStdDev)
		},
	}
}

// walk declares a bucket that drifts between ticks.
func (b *builder) walk(label string, p walkParams) bucket {
	w := newWalk(b.next(), p)

	return bucket{
		label: label,
		value: w.at,
	}
}

// deploys declares the friday_deploys buckets. The successful bucket is a
// fraction of the attempted one rather than an independent sample, so a
// chart of the two never shows more deploys surviving than were tried.
func (b *builder) deploys() []bucket {
	attempted := b.count("attempted", _fridayDeployMean, _fridayDeployRelStdDev)
	seed := b.next()

	return []bucket{
		attempted,
		{
			label: "successful",
			value: func(tick int64) float64 {
				rate := clamp(
					normal(newRand(seed, tick), _fridayDeploySuccessMean, _fridayDeploySuccessStdDev),
					0,
					1,
				)

				return math.Round(attempted.value(tick) * rate)
			},
		},
	}
}

// newRegistry declares the demo's metric families. Means are aggregate
// weekly totals across the ~10k organisations the demo pretends to
// measure.
//
//nolint:funlen,maintidx // a flat table of tuned parameters, not logic
func newRegistry() registry {
	b := &builder{}

	return registry{
		{
			name:  _namespace + "meetings_could_be_doc_count",
			help:  "Weekly count of meetings that could've been a document.",
			label: "excuse",
			buckets: []bucket{
				b.count("alignment", 42_000, 0.15),
				b.count("bandwidth", 36_000, 0.15),
				b.count("synergy", 30_000, 0.15),
				b.count("optics", 12_000, 0.20),
			},
		},
		{
			name:  _namespace + "circle_back_count",
			help:  `Weekly count of "let's circle back" mentions.`,
			label: _labelOutcome,
			buckets: []bucket{
				b.count("circled_back", 16_000, 0.20),
				b.count("forgotten", 48_000, 0.15),
				b.count("rescheduled_to_circle_back", 16_000, 0.20),
			},
		},
		{
			name:    _namespace + "friday_deploys_count",
			help:    "Weekly Friday deploy counts.",
			label:   _labelOutcome,
			buckets: b.deploys(),
		},
		{
			name:  _namespace + "pizza_fridays_next_30_days",
			help:  "Pizza Fridays scheduled in the next 30 days.",
			label: "pizza_count",
			buckets: []bucket{
				b.count("1_to_5", 10_000, 0.15),
				b.count("5_to_10", 7_000, 0.20),
				b.count("10_plus", 3_000, 0.25),
			},
		},
		{
			name:  _namespace + "ship_it_approvals_after_5pm_count",
			help:  `Weekly count of "ship it" PR approvals left after 5 pm.`,
			label: "reviewer_state",
			buckets: []bucket{
				b.count("sober", 20_000, 0.15),
				b.count("one_beer_in", 14_000, 0.20),
				b.count("heading_to_the_door", 6_000, 0.25),
			},
		},
		{
			name:  _namespace + "prs_merged_friday_afternoon_count",
			help:  "Weekly count of PRs merged on Fridays after 3 pm.",
			label: "confidence",
			buckets: []bucket{
				b.count("yolo", 20_000, 0.15),
				b.count("fingers_crossed", 22_500, 0.15),
				b.count("vacation_starts_monday", 7_500, 0.20),
			},
		},
		{
			name:  _namespace + "secrets_detected_commits_count",
			help:  "Secrets detected in commits over the past week.",
			label: "severity",
			buckets: []bucket{
				b.count("normal", 2_100, 0.25),
				b.count("critical", 750, 0.30),
				b.count("nuclear", 150, 0.35),
			},
		},
		{
			name:  _namespace + "quick_sync_meetings_same_day_count",
			help:  `Weekly count of "quick sync" meetings scheduled same-day.`,
			label: "duration",
			buckets: []bucket{
				b.count("10m_to_30m", 33_000, 0.15),
				b.count("30m_to_1h", 21_000, 0.15),
				b.count("1h_plus", 6_000, 0.25),
			},
		},
		{
			name:  _namespace + "temporary_fix_age_days",
			help:  `Age in days of the oldest surviving "temporary" fix.`,
			label: "original_comment",
			buckets: []bucket{
				b.walk("temp", walkParams{
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
				b.walk("hack", walkParams{
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
				b.walk("fixme", walkParams{
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
				b.walk("todo", walkParams{
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
			},
		},
		{
			name:  _namespace + "deploy_confidence_index",
			help:  "Self-reported deploy confidence index (0–100).",
			label: "vibe",
			buckets: []bucket{
				b.walk("feeling_lucky", walkParams{
					Min:           60,
					Max:           100,
					Start:         78,
					Target:        80,
					MeanReversion: 0.08,
					NoiseStdDev:   3,
					SpikeChance:   0.05,
					SpikeStdDev:   8,
				}),
				b.walk("nervous", walkParams{
					Min:           30,
					Max:           75,
					Start:         52,
					Target:        55,
					MeanReversion: 0.08,
					NoiseStdDev:   4,
					SpikeChance:   0.07,
					SpikeStdDev:   10,
				}),
				b.walk("oncall_already_paged", walkParams{
					Min:           0,
					Max:           40,
					Start:         18,
					Target:        20,
					MeanReversion: 0.1,
					NoiseStdDev:   3,
					SpikeChance:   0.1,
					SpikeStdDev:   8,
				}),
			},
		},
		{
			name: _namespace + "pr_time_to_first_review_minutes",
			help: "Median time from PR open to first review comment (minutes).",
			buckets: []bucket{
				b.walk("", walkParams{
					Min:           5,
					Max:           10_080,
					Start:         280,
					Target:        300,
					MeanReversion: 0.1,
					NoiseStdDev:   25,
					SpikeChance:   0.08,
					SpikeStdDev:   180,
				}),
			},
		},
		{
			name:  _namespace + "hotfixes_per_release",
			help:  "Average number of hotfixes per release.",
			label: "root_cause",
			buckets: []bucket{
				b.walk("works_on_my_machine", walkParams{
					Min:           0,
					Max:           12,
					Start:         0.9,
					Target:        1.0,
					MeanReversion: 0.1,
					NoiseStdDev:   0.2,
					SpikeChance:   0.05,
					SpikeStdDev:   1,
				}),
				b.walk("not_my_fault", walkParams{
					Min:           0,
					Max:           10,
					Start:         0.8,
					Target:        0.9,
					MeanReversion: 0.1,
					NoiseStdDev:   0.2,
					SpikeChance:   0.05,
					SpikeStdDev:   0.8,
				}),
				b.walk("timezone_issue", walkParams{
					Min:           0,
					Max:           5,
					Start:         0.4,
					Target:        0.45,
					MeanReversion: 0.1,
					NoiseStdDev:   0.1,
					SpikeChance:   0.04,
					SpikeStdDev:   0.5,
				}),
			},
		},
		{
			name:  _namespace + "works_on_my_machine_rate",
			help:  `Self-reported "works on my machine" incidence rate (0–100).`,
			label: "platform",
			buckets: []bucket{
				b.walk("linux", walkParams{
					Min:           10,
					Max:           60,
					Start:         28,
					Target:        30,
					MeanReversion: 0.06,
					NoiseStdDev:   2,
					SpikeChance:   0.05,
					SpikeStdDev:   6,
				}),
				b.walk("mac", walkParams{
					Min:           25,
					Max:           75,
					Start:         48,
					Target:        50,
					MeanReversion: 0.06,
					NoiseStdDev:   2.5,
					SpikeChance:   0.06,
					SpikeStdDev:   8,
				}),
				b.walk("my_special_setup", walkParams{
					Min:           60,
					Max:           100,
					Start:         82,
					Target:        85,
					MeanReversion: 0.05,
					NoiseStdDev:   3,
					SpikeChance:   0.07,
					SpikeStdDev:   5,
				}),
			},
		},
		{
			name:  _namespace + "stale_branches_total",
			help:  "Number of branches with no activity for more than 90 days.",
			label: "owner_status",
			buckets: []bucket{
				b.walk("still_employed", walkParams{
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
				b.walk("left_the_company", walkParams{
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
				b.walk("nobody_knows", walkParams{
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
			},
		},
		{
			name:  _namespace + "feature_flag_graveyard_size",
			help:  "Number of feature flags that have outlived their purpose.",
			label: "removal_likelihood",
			buckets: []bucket{
				b.walk("maybe_someday", walkParams{
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
				b.walk("never", walkParams{
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
				b.walk("what_does_this_do", walkParams{
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
			},
		},
	}
}
