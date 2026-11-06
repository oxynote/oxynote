package processor

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
)

// _maxFilledPoints is the most points the series of a filled result hold
// together, about what the 5 MB of rows a query may return turn into.
const _maxFilledPoints = 250_000

// sqlFill is the fill mode of a group macro, such as the 0 of
// $__timeGroup(ts, '5m', 0). It gives every series a point on each bucket
// of the range.
type sqlFill struct {
	// Previous indicates that an empty bucket repeats the series' last
	// value before it.
	Previous bool `json:"previous,omitempty"`

	// Value specifies the value of an empty bucket. Nil without Previous
	// leaves the bucket null.
	Value *float64 `json:"value,omitempty"`

	// Interval specifies the length of a bucket in seconds.
	Interval int64 `json:"interval"`

	// From specifies the unix seconds of the first bucket.
	From int64 `json:"from"`

	// To specifies the unix seconds of the range end. No bucket starts
	// after it.
	To int64 `json:"to"`
}

// newSQLFill returns the fill that mode, the last argument of a group
// macro, asks for over tr in buckets of interval seconds.
func newSQLFill(mode string, interval int64, tr TimeRange) (*sqlFill, error) {
	// the range a query runs on can end up to a cache step past now, and a
	// bucket that has not started yet would be drawn as the fill value.
	to := tr.To
	if now := timeutil.Now(); now.Before(to) {
		to = now
	}

	f := sqlFill{
		Interval: interval,
		From:     tr.From.Unix() / interval * interval,
		To:       to.Unix(),
	}

	switch {
	case strings.EqualFold(mode, "NULL"):
	case strings.EqualFold(mode, "previous"):
		f.Previous = true
	default:
		v, err := strconv.ParseFloat(mode, 64)
		if err != nil || !isValidValue(v) {
			return nil, NewInvalidQueryError(fmt.Sprintf("fill %s must be NULL, previous or a number", mode))
		}

		f.Value = &v
	}

	return &f, nil
}

// buckets returns how many buckets the range holds.
func (f sqlFill) buckets() int64 {
	if f.To < f.From {
		return 0
	}

	return (f.To-f.From)/f.Interval + 1
}

// apply returns the points of one series with one on each bucket. A
// bucket takes the last point after the bucket before it, so a point
// between buckets moves to the next one.
func (f sqlFill) apply(points []sqlPoint) [][2]any {
	slices.SortStableFunc(points, func(a, b sqlPoint) int {
		return cmp.Compare(a.time, b.time)
	})

	var (
		out  = make([][2]any, 0, f.buckets())
		prev any
		i    int
	)

	for t := f.From; t <= f.To; t += f.Interval {
		var cur any

		for ; i < len(points) && points[i].time <= t; i++ {
			prev = points[i].value

			if points[i].time > t-f.Interval {
				cur = points[i].value
			}
		}

		switch {
		case cur != nil:
		case f.Previous:
			cur = prev
		case f.Value != nil:
			cur = *f.Value
		}

		out = append(out, [2]any{t, cur})
	}

	return out
}

// sqlPoint is one point of a series as it is read from the rows.
type sqlPoint struct {
	// time specifies the unix seconds of the point.
	time int64

	// value specifies the value of the point.
	value float64
}
