package processor

import (
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_renderTime(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Result  string
	}{
		"PostgreSQL": {Dialect: _postgreSQLDialect, Result: `EXTRACT(EPOCH FROM ts) AS "time"`},
		"MySQL":      {Dialect: _mySQLDialect, Result: "UNIX_TIMESTAMP(ts) AS `time`"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderTime(&sqlExpander{d: c.Dialect, tr: _testTimeRange}, "time", []string{"ts"})
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_renderTimeFilter(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Arg     string
		Result  string
	}{
		"PostgreSQL":                        {Dialect: _postgreSQLDialect, Arg: "ts", Result: "(ts) BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz"},
		"MySQL":                             {Dialect: _mySQLDialect, Arg: "ts", Result: "(ts) BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)"},
		"Expression is compared as a whole": {Dialect: _mySQLDialect, Arg: "a OR b", Result: "(a OR b) BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderTimeFilter(&sqlExpander{d: c.Dialect, tr: _testTimeRange}, "timeFilter", []string{c.Arg})
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_renderTimeFrom(t *testing.T) {
	t.Parallel()

	res, err := renderTimeFrom(&sqlExpander{d: _mySQLDialect, tr: _testTimeRange}, "timeFrom", nil)
	require.NoError(t, err)
	assert.Equal(t, "FROM_UNIXTIME(1705312800)", res)
}

func Test_renderTimeTo(t *testing.T) {
	t.Parallel()

	res, err := renderTimeTo(&sqlExpander{d: _mySQLDialect, tr: _testTimeRange}, "timeTo", nil)
	require.NoError(t, err)
	assert.Equal(t, "FROM_UNIXTIME(1705316400)", res)
}

func Test_renderTimeGroup(t *testing.T) {
	cc := map[string]struct {
		Dialect sqlDialect
		Name    string
		Result  string
	}{
		"PostgreSQL":       {Dialect: _postgreSQLDialect, Name: "timeGroup", Result: "floor(EXTRACT(EPOCH FROM ts)/300)*300"},
		"PostgreSQL alias": {Dialect: _postgreSQLDialect, Name: "timeGroupAlias", Result: `floor(EXTRACT(EPOCH FROM ts)/300)*300 AS "time"`},
		"MySQL":            {Dialect: _mySQLDialect, Name: "timeGroup", Result: "floor(UNIX_TIMESTAMP(ts)/300)*300"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderTimeGroup(&sqlExpander{d: c.Dialect, tr: _testTimeRange}, c.Name, []string{"ts", "'5m'"})
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_renderUnixEpochGroup(t *testing.T) {
	cc := map[string]struct {
		Name   string
		Result string
	}{
		"Group":       {Name: "unixEpochGroup", Result: "floor((a+b)/300)*300"},
		"Group alias": {Name: "unixEpochGroupAlias", Result: `floor((a+b)/300)*300 AS "time"`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderUnixEpochGroup(&sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange}, c.Name, []string{"a+b", "'5m'"})
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_renderGroup(t *testing.T) {
	cc := map[string]struct {
		Name   string
		Args   []string
		Result string
		Fill   *sqlFill
		Err    error
	}{
		"Group":            {Name: "timeGroup", Args: []string{"'5m'"}, Result: "floor(x/300)*300"},
		"Group alias":      {Name: "timeGroupAlias", Args: []string{"1h"}, Result: `floor(x/3600)*3600 AS "time"`},
		"Fill mode":        {Name: "timeGroup", Args: []string{"'5m'", "previous"}, Result: "floor(x/300)*300", Fill: &sqlFill{Previous: true, Interval: 300, From: 1705312800, To: 1705316400}},
		"Invalid interval": {Name: "timeGroup", Args: []string{"'5x'"}, Err: invalidIntervalError("'5x'")},
		"Invalid fill":     {Name: "timeGroup", Args: []string{"'5m'", "zero"}, Err: NewInvalidQueryError("fill zero must be NULL, previous or a number")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			e := &sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange}

			res, err := renderGroup(e, c.Name, "x", c.Args)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
			assert.Equal(t, c.Fill, e.fill)
		})
	}
}

func Test_renderUnixEpochFilter(t *testing.T) {
	cc := map[string]struct {
		Name   string
		Result string
	}{
		"Seconds":     {Name: "unixEpochFilter", Result: "(a+b) >= 1705312800 AND (a+b) <= 1705316400"},
		"Nanoseconds": {Name: "unixEpochNanoFilter", Result: "(a+b) >= 1705312800000000000 AND (a+b) <= 1705316400000000000"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderUnixEpochFilter(&sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange}, c.Name, []string{"a+b"})
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_renderUnixEpochBound(t *testing.T) {
	cc := map[string]struct {
		Name   string
		Result string
	}{
		"Start":            {Name: "unixEpochFrom", Result: "1705312800"},
		"End":              {Name: "unixEpochTo", Result: "1705316400"},
		"Nanosecond start": {Name: "unixEpochNanoFrom", Result: "1705312800000000000"},
		"Nanosecond end":   {Name: "unixEpochNanoTo", Result: "1705316400000000000"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := renderUnixEpochBound(&sqlExpander{d: _postgreSQLDialect, tr: _testTimeRange}, c.Name, nil)
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_parseInterval(t *testing.T) {
	cc := map[string]struct {
		Input   string
		Quotes  string
		Seconds int64
		Err     error
	}{
		"Double-quoted where that is a string": {Input: `"5m"`, Quotes: `'"`, Seconds: 300},
		"Double-quoted where that is a name":   {Input: `"5m"`, Err: invalidIntervalError(`"5m"`)},
		"Mismatched quotes":                    {Input: `'5m"`, Quotes: `'"`, Err: invalidIntervalError(`'5m"`)},
		"Seconds":                              {Input: "30s", Seconds: 30},
		"Minutes":                              {Input: "5m", Seconds: 300},
		"Hours":                                {Input: "1h", Seconds: 3600},
		"Days":                                 {Input: "1d", Seconds: 86400},
		"Weeks":                                {Input: "1w", Seconds: 604800},
		"Quoted":                               {Input: "'5m'", Seconds: 300},
		"Invalid unit":                         {Input: "5x", Err: invalidIntervalError("5x")},
		"Milliseconds":                         {Input: "20ms", Err: invalidIntervalError("20ms")},
		"No number":                            {Input: "m", Err: invalidIntervalError("m")},
		"Empty":                                {Input: "", Err: invalidIntervalError("")},
		"Empty quotes":                         {Input: "''", Err: invalidIntervalError("''")},
		"Zero":                                 {Input: "0m", Err: invalidIntervalError("0m")},
		"Negative":                             {Input: "-5m", Err: invalidIntervalError("-5m")},
		"Overflow":                             {Input: "144115188075855872w", Err: invalidIntervalError("144115188075855872w")},
		"Longest":                              {Input: "520w", Seconds: 314496000},
		"Longer than ten years":                {Input: "522w", Err: invalidIntervalError("522w")},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			quotes := c.Quotes
			if quotes == "" {
				quotes = "'"
			}

			seconds, err := parseInterval(c.Input, quotes)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Seconds, seconds)
		})
	}
}

func Test_invalidIntervalError(t *testing.T) {
	t.Parallel()

	assert.Equal(t, NewInvalidQueryError("interval '5x' must be a whole number of s, m, h, d or w, at most 10 years"), invalidIntervalError("'5x'"))
}
