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

func Test_registry_isCachedPerClient(t *testing.T) {
	t.Parallel()

	// the client holds one registry for its lifetime: the walks behind it
	// cache the history they replay, and rebuilding it per query would
	// walk every tick since the epoch again.
	c := NewClient()

	first := c.registry
	require.NotEmpty(t, first)

	assert.Equal(t, first[0].buckets[0].value(7), c.registry[0].buckets[0].value(7))

	// a separate client is a separate cache, reading the same history.
	other := NewClient()
	require.Len(t, other.registry, len(first))

	for i := range first {
		assert.Equal(t, first[i].name, other.registry[i].name)
		assert.Equal(t, first[i].buckets[0].value(7), other.registry[i].buckets[0].value(7))
	}
}

// nameMatcher builds a matcher selecting one metric by name, failing
// the test if it cannot.
func nameMatcher(t *testing.T, name string) *labels.Matcher {
	t.Helper()

	m, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, name)
	require.NoError(t, err)

	return m
}

func Test_seriesLabels(t *testing.T) {
	t.Parallel()

	// a labelled family carries its breakdown alongside the metric name.
	lbls := seriesLabels(
		family{name: "metric", label: "colour"},
		bucket{label: "red"},
	)

	assert.Equal(t, "metric", lbls.Get(model.MetricNameLabel))
	assert.Equal(t, "red", lbls.Get("colour"))
	assert.Equal(t, 2, lbls.Len())

	// an unlabelled one carries nothing but the name — not an empty
	// label, which would show up in autocomplete as a real dimension.
	lbls = seriesLabels(family{name: "metric"}, bucket{})

	assert.Equal(t, "metric", lbls.Get(model.MetricNameLabel))
	assert.Equal(t, 1, lbls.Len())
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

	// no matchers reaches every series the registry declares.
	var all int

	newRegistry().forEach(nil, func(labels.Labels, func(int64) float64) { all++ })

	var buckets int

	for _, f := range newRegistry() {
		buckets += len(f.buckets)
	}

	assert.Equal(t, buckets, all)

	// a matcher narrows to the family it names, and every series handed
	// over carries a reading.
	var named int

	newRegistry().forEach(
		[]*labels.Matcher{nameMatcher(t, _namespace+"deploy_confidence_index")},
		func(lbls labels.Labels, value func(int64) float64) {
			named++

			assert.Equal(t, _namespace+"deploy_confidence_index", lbls.Get(model.MetricNameLabel))
			require.NotNil(t, value)
			assert.NotZero(t, value(500))
		},
	)

	assert.Equal(t, 3, named)

	// a matcher naming nothing reaches nothing.
	newRegistry().forEach(
		[]*labels.Matcher{nameMatcher(t, "not_a_metric")},
		func(labels.Labels, func(int64) float64) { t.Fatal("matched a series that does not exist") },
	)
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
		{nameMatcher(t, _namespace+"deploy_confidence_index")},
	})

	assert.Equal(t, []string{model.MetricNameLabel, "vibe"}, names)

	// several selectors are unioned, the way the endpoint answers when
	// handed more than one.
	names = newRegistry().labelNames([][]*labels.Matcher{
		{nameMatcher(t, _namespace+"deploy_confidence_index")},
		{nameMatcher(t, _namespace+"secrets_detected_commits_count")},
	})

	assert.Equal(t, []string{model.MetricNameLabel, "severity", "vibe"}, names)
}

func Test_registry_labelValues(t *testing.T) {
	t.Parallel()

	// the metric names are a label value like any other, which is what
	// the editor's metric autocomplete reads.
	names := newRegistry().labelValues(model.MetricNameLabel, [][]*labels.Matcher{nil})

	assert.Len(t, names, 15)
	assert.Contains(t, names, _namespace+"pizza_fridays_next_30_days")
	assert.IsIncreasing(t, names)

	assert.Equal(
		t,
		[]string{"feeling_lucky", "nervous", "oncall_already_paged"},
		newRegistry().labelValues("vibe", [][]*labels.Matcher{nil}),
	)

	// a label no series carries answers empty rather than a blank value.
	assert.Empty(t, newRegistry().labelValues("nonexistent", [][]*labels.Matcher{nil}))
}
