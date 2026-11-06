package processor

import (
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/stretchr/testify/assert"
)

func Test_newSQLFill(t *testing.T) {
	cc := map[string]struct {
		Mode     string
		Interval int64
		EndsLate bool
		Result   *sqlFill
		Err      error
	}{
		"NULL":                                {Mode: "NULL", Interval: 300, Result: &sqlFill{Interval: 300, From: 1705312800, To: 1705316400}},
		"Lowercase null":                      {Mode: "null", Interval: 300, Result: &sqlFill{Interval: 300, From: 1705312800, To: 1705316400}},
		"Previous":                            {Mode: "previous", Interval: 300, Result: &sqlFill{Previous: true, Interval: 300, From: 1705312800, To: 1705316400}},
		"Uppercase previous":                  {Mode: "PREVIOUS", Interval: 300, Result: &sqlFill{Previous: true, Interval: 300, From: 1705312800, To: 1705316400}},
		"Number":                              {Mode: "1.5", Interval: 300, Result: &sqlFill{Value: new(1.5), Interval: 300, From: 1705312800, To: 1705316400}},
		"Negative number":                     {Mode: "-2", Interval: 300, Result: &sqlFill{Value: new(-2.0), Interval: 300, From: 1705312800, To: 1705316400}},
		"First bucket starts earlier":         {Mode: "NULL", Interval: 7, Result: &sqlFill{Interval: 7, From: 1705312798, To: 1705316400}},
		"Range ending after now stops at now": {Mode: "NULL", Interval: 300, EndsLate: true, Result: &sqlFill{Interval: 300, From: 1705312800}},
		"Word is an error":                    {Mode: "zero", Interval: 300, Err: NewInvalidQueryError("fill zero must be NULL, previous or a number")},
		"Quoted number is an error":           {Mode: "'0'", Interval: 300, Err: NewInvalidQueryError("fill '0' must be NULL, previous or a number")},
		"Infinity is an error":                {Mode: "Inf", Interval: 300, Err: NewInvalidQueryError("fill Inf must be NULL, previous or a number")},
		"Not a number is an error":            {Mode: "NaN", Interval: 300, Err: NewInvalidQueryError("fill NaN must be NULL, previous or a number")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tr := _testTimeRange
			if c.EndsLate {
				tr.To = timeutil.Now().Add(time.Hour)
			}

			res, err := newSQLFill(c.Mode, c.Interval, tr)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.EndsLate {
				assert.InDelta(t, timeutil.Now().Unix(), res.To, 5)

				c.Result.To = res.To
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_sqlFill_buckets(t *testing.T) {
	cc := map[string]struct {
		Fill   sqlFill
		Result int64
	}{
		"Range ends on a bucket":     {Fill: sqlFill{Interval: 60, From: 0, To: 120}, Result: 3},
		"Range ends between buckets": {Fill: sqlFill{Interval: 60, From: 0, To: 119}, Result: 2},
		"Empty range":                {Fill: sqlFill{Interval: 60, From: 0, To: 0}, Result: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Fill.buckets())
		})
	}
}

func Test_sqlFill_apply(t *testing.T) {
	cc := map[string]struct {
		Fill   sqlFill
		Points []sqlPoint
		Result [][2]any
	}{
		"Empty bucket is null": {
			Fill:   sqlFill{Interval: 60, From: 0, To: 180},
			Points: []sqlPoint{{time: 0, value: 1}, {time: 120, value: 3}},
			Result: [][2]any{{int64(0), 1.0}, {int64(60), nil}, {int64(120), 3.0}, {int64(180), nil}},
		},
		"Empty bucket takes the value": {
			Fill:   sqlFill{Value: new(0.0), Interval: 60, From: 0, To: 120},
			Points: []sqlPoint{{time: 0, value: 1}, {time: 120, value: 3}},
			Result: [][2]any{{int64(0), 1.0}, {int64(60), 0.0}, {int64(120), 3.0}},
		},
		"Empty bucket repeats the previous value": {
			Fill:   sqlFill{Previous: true, Interval: 60, From: 0, To: 180},
			Points: []sqlPoint{{time: 0, value: 1}, {time: 120, value: 3}},
			Result: [][2]any{{int64(0), 1.0}, {int64(60), 1.0}, {int64(120), 3.0}, {int64(180), 3.0}},
		},
		"Previous value can come from before the range": {
			Fill:   sqlFill{Previous: true, Interval: 60, From: 0, To: 60},
			Points: []sqlPoint{{time: -90, value: 5}},
			Result: [][2]any{{int64(0), 5.0}, {int64(60), 5.0}},
		},
		"Previous value before the first point is null": {
			Fill:   sqlFill{Previous: true, Interval: 60, From: 0, To: 120},
			Points: []sqlPoint{{time: 120, value: 3}},
			Result: [][2]any{{int64(0), nil}, {int64(60), nil}, {int64(120), 3.0}},
		},
		"Point between buckets moves to the next": {
			Fill:   sqlFill{Interval: 60, From: 0, To: 60},
			Points: []sqlPoint{{time: 30, value: 2}},
			Result: [][2]any{{int64(0), nil}, {int64(60), 2.0}},
		},
		"Last point of a bucket wins": {
			Fill:   sqlFill{Interval: 60, From: 0, To: 60},
			Points: []sqlPoint{{time: 10, value: 1}, {time: 50, value: 2}},
			Result: [][2]any{{int64(0), nil}, {int64(60), 2.0}},
		},
		"Points are put in time order": {
			Fill:   sqlFill{Interval: 60, From: 0, To: 60},
			Points: []sqlPoint{{time: 60, value: 2}, {time: 0, value: 1}},
			Result: [][2]any{{int64(0), 1.0}, {int64(60), 2.0}},
		},
		"Point after the range is dropped": {
			Fill:   sqlFill{Interval: 60, From: 0, To: 60},
			Points: []sqlPoint{{time: 200, value: 9}},
			Result: [][2]any{{int64(0), nil}, {int64(60), nil}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Fill.apply(c.Points))
		})
	}
}
