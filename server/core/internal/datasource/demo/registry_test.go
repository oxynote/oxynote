package demo

import (
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_builder_next(t *testing.T) {
	t.Parallel()

	b := &builder{}

	// every series gets a seed of its own, so no two draw the same
	// numbers.
	assert.Equal(t, int64(1), b.next())
	assert.Equal(t, int64(2), b.next())
	assert.Equal(t, int64(3), b.next())
}

func Test_builder_count(t *testing.T) {
	t.Parallel()

	b := &builder{}

	one := b.count("first", 1_000, 0.1)
	two := b.count("second", 1_000, 0.1)

	assert.Equal(t, "first", one.label)
	assert.Equal(t, "second", two.label)

	require.NotNil(t, one.value)

	// the reading is the count the tick names, and two buckets declared
	// alike still read differently because their seeds differ.
	assert.Equal(t, countAt(1, 42, 1_000, 0.1), one.value(42))
	assert.NotEqual(t, one.value(42), two.value(42))
}

func Test_builder_walk(t *testing.T) {
	t.Parallel()

	b := &builder{}

	one := b.walk("first", _testWalk)
	two := b.walk("second", _testWalk)

	assert.Equal(t, "first", one.label)
	require.NotNil(t, one.value)

	// the bucket reads its walk, which starts where the params say.
	assert.Equal(t, _testWalk.Start, one.value(0))

	// same params, different seed, so the two diverge once they move.
	assert.NotEqual(t, one.value(100), two.value(100))
}

func Test_builder_deploys(t *testing.T) {
	t.Parallel()

	bb := (&builder{}).deploys()
	require.Len(t, bb, 2)

	assert.Equal(t, "attempted", bb[0].label)
	assert.Equal(t, "successful", bb[1].label)

	// the successful bucket is a share of the attempted one, so a chart
	// of both never shows more deploys surviving than were tried.
	for tick := range int64(500) {
		attempted := bb[0].value(tick)
		successful := bb[1].value(tick)

		assert.GreaterOrEqual(t, successful, 0.0)
		assert.LessOrEqual(t, successful, attempted, "tick %d", tick)
	}
}

func Test_newRegistry(t *testing.T) {
	t.Parallel()

	ff := newRegistry()
	require.Len(t, ff, 15)

	var (
		names   = map[string]struct{}{}
		seen    = map[string]float64{}
		buckets int
	)

	for _, f := range ff {
		// every family is namespaced, described, and populated.
		assert.True(t, strings.HasPrefix(f.name, _namespace), f.name)
		assert.NotEmpty(t, f.help, f.name)
		require.NotEmpty(t, f.buckets, f.name)

		_, dup := names[f.name]
		assert.False(t, dup, "duplicate family %q", f.name)
		names[f.name] = struct{}{}

		bucketLabels := map[string]struct{}{}

		for _, b := range f.buckets {
			buckets++

			require.NotNil(t, b.value, f.name)

			// a family either breaks down by a label or publishes one
			// unlabelled series; a bucket never disagrees with its family.
			if f.label == "" {
				assert.Empty(t, b.label, f.name)
				assert.Len(t, f.buckets, 1, f.name)
			} else {
				assert.NotEmpty(t, b.label, f.name)
			}

			_, dup := bucketLabels[b.label]
			assert.False(t, dup, "duplicate bucket %q in %q", b.label, f.name)
			bucketLabels[b.label] = struct{}{}

			// every series reads, and no two read alike — a seed shared by
			// accident would show up here as two identical lines.
			v := b.value(1_000)
			key := f.name + "/" + b.label

			for other, ov := range seen {
				assert.NotEqual(t, ov, v, "%s and %s read alike", other, key)
			}

			seen[key] = v
		}
	}

	assert.Len(t, seen, buckets)
}

func Test_seriesLabels(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Family family
		Bucket bucket
		Result labels.Labels
	}{
		"A labelled family carries its breakdown alongside the metric name": {
			Family: family{name: "metric", label: "colour"},
			Bucket: bucket{label: "red"},
			Result: labels.FromStrings(model.MetricNameLabel, "metric", "colour", "red"),
		},
		// not an empty label, which would show up in autocomplete as a
		// real dimension.
		"An unlabelled family carries nothing but the name": {
			Family: family{name: "metric"},
			Result: labels.FromStrings(model.MetricNameLabel, "metric"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, seriesLabels(c.Family, c.Bucket))
		})
	}
}

func Test_matches(t *testing.T) {
	t.Parallel()

	lbls := labels.FromStrings(model.MetricNameLabel, "metric", "colour", "red")

	cc := map[string]struct {
		Matchers []*labels.Matcher
		Result   bool
	}{
		"No matchers at all": {Result: true},
		"Matching name":      {Matchers: []*labels.Matcher{{Type: labels.MatchEqual, Name: model.MetricNameLabel, Value: "metric"}}, Result: true},
		"Other name":         {Matchers: []*labels.Matcher{{Type: labels.MatchEqual, Name: model.MetricNameLabel, Value: "nope"}}},
		"Matching label":     {Matchers: []*labels.Matcher{{Type: labels.MatchEqual, Name: "colour", Value: "red"}}, Result: true},
		"Other label value":  {Matchers: []*labels.Matcher{{Type: labels.MatchEqual, Name: "colour", Value: "blue"}}},
		"Absent label matched empty": {
			Matchers: []*labels.Matcher{{Type: labels.MatchEqual, Name: "size", Value: ""}},
			Result:   true,
		},
		"One of several fails": {
			Matchers: []*labels.Matcher{
				{Type: labels.MatchEqual, Name: model.MetricNameLabel, Value: "metric"},
				{Type: labels.MatchEqual, Name: "colour", Value: "blue"},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, matches(lbls, c.Matchers))
		})
	}
}

func Test_registry_forEach(t *testing.T) {
	t.Parallel()

	var buckets int

	for _, f := range newRegistry() {
		buckets += len(f.buckets)
	}

	cc := map[string]struct {
		Matchers []*labels.Matcher
		Result   int
		Check    func(t *testing.T, lbls labels.Labels, value func(int64) float64)
	}{
		"No matchers reach every series the registry declares": {
			Result: buckets,
		},
		"A matcher narrows to the family it names": {
			Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"deploy_confidence_index")},
			Result:   3,
			Check: func(t *testing.T, lbls labels.Labels, value func(int64) float64) {
				// every series handed over carries a reading.
				assert.Equal(t, _namespace+"deploy_confidence_index", lbls.Get(model.MetricNameLabel))
				require.NotNil(t, value)
				assert.NotZero(t, value(500))
			},
		},
		"A matcher naming nothing reaches nothing": {
			Matchers: []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, "not_a_metric")},
			Check: func(t *testing.T, _ labels.Labels, _ func(int64) float64) {
				t.Fatal("matched a series that does not exist")
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var got int

			newRegistry().forEach(c.Matchers, func(lbls labels.Labels, value func(int64) float64) {
				got++

				if c.Check != nil {
					c.Check(t, lbls, value)
				}
			})

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_sortedKeys(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(map[string]struct{}{
		"c": {},
		"a": {},
		"b": {},
	}))

	// an empty set answers an empty slice rather than nil, so a JSON
	// response carries [] instead of null.
	assert.Equal(t, []string{}, sortedKeys(map[string]struct{}{}))
}

func Test_registry_labelNames(t *testing.T) {
	t.Parallel()

	names := newRegistry().labelNames([][]*labels.Matcher{nil})

	assert.Contains(t, names, model.MetricNameLabel)
	assert.Contains(t, names, "vibe")
	assert.Contains(t, names, "severity")
	assert.IsIncreasing(t, names)

	// narrowing to one family leaves only that family's dimension.
	names = newRegistry().labelNames([][]*labels.Matcher{
		{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"deploy_confidence_index")},
	})

	assert.Equal(t, []string{model.MetricNameLabel, "vibe"}, names)

	// several selectors are unioned, the way the endpoint answers when
	// handed more than one.
	names = newRegistry().labelNames([][]*labels.Matcher{
		{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"deploy_confidence_index")},
		{labels.MustNewMatcher(labels.MatchEqual, model.MetricNameLabel, _namespace+"secrets_detected_commits_count")},
	})

	assert.Equal(t, []string{model.MetricNameLabel, "severity", "vibe"}, names)
}

func Test_registry_labelValues(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Name   string
		Result []string
	}{
		// the editor's metric autocomplete reads these.
		"The metric names are a label value like any other": {
			Name: model.MetricNameLabel,
			Result: []string{
				_namespace + "circle_back_count",
				_namespace + "deploy_confidence_index",
				_namespace + "feature_flag_graveyard_size",
				_namespace + "friday_deploys_count",
				_namespace + "hotfixes_per_release",
				_namespace + "meetings_could_be_doc_count",
				_namespace + "pizza_fridays_next_30_days",
				_namespace + "pr_time_to_first_review_minutes",
				_namespace + "prs_merged_friday_afternoon_count",
				_namespace + "quick_sync_meetings_same_day_count",
				_namespace + "secrets_detected_commits_count",
				_namespace + "ship_it_approvals_after_5pm_count",
				_namespace + "stale_branches_total",
				_namespace + "temporary_fix_age_days",
				_namespace + "works_on_my_machine_rate",
			},
		},
		"Every value of a label": {
			Name:   "vibe",
			Result: []string{"feeling_lucky", "nervous", "oncall_already_paged"},
		},
		"A label no series carries answers empty rather than a blank value": {
			Name:   "nonexistent",
			Result: []string{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, newRegistry().labelValues(c.Name, [][]*labels.Matcher{nil}))
		})
	}
}
