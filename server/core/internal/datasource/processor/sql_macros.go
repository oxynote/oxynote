package processor

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// _maxIntervalSeconds is the longest interval a group macro takes.
	_maxIntervalSeconds = 10 * 365 * _secondsPerDay

	// _groupMacroArgs is how many arguments a group macro takes before its
	// optional fill mode: a column and an interval.
	_groupMacroArgs = 2

	// All available interval unit multipliers in seconds.
	_secondsPerMinute = 60
	_secondsPerHour   = 3600
	_secondsPerDay    = 86400
	_secondsPerWeek   = 604800
)

// _sqlMacros holds every SQL macro call by name.
//
//	$__time(col), $__timeEpoch(col)   → unix seconds of col AS time
//	$__timeFilter(col)                → (col) BETWEEN the range ends
//	$__timeFrom(), $__timeTo()        → a range end as a timestamp
//	$__timeGroup(col, '5m'[, fill])   → unix seconds of col, floored to the interval
//	$__timeGroupAlias(col, '5m'[, fill]) → the same AS time
//	$__unixEpochFilter(col)           → (col) within the range in unix seconds
//	$__unixEpochNanoFilter(col)       → (col) within the range in unix nanoseconds
//	$__unixEpochFrom(), $__unixEpochTo(), $__unixEpochNanoFrom(), $__unixEpochNanoTo()
//	$__unixEpochGroup(col, '5m'[, fill]) → (col) floored to the interval
//	$__unixEpochGroupAlias(col, '5m'[, fill]) → the same AS time
//
// A fill of NULL, previous or a number fills the buckets a series has no
// row for.
var _sqlMacros = map[string]sqlMacro{
	"time":                {args: 1, render: renderTime},
	"timeEpoch":           {args: 1, render: renderTime},
	"timeFilter":          {args: 1, render: renderTimeFilter},
	"timeFrom":            {render: renderTimeFrom},
	"timeTo":              {render: renderTimeTo},
	"timeGroup":           {args: _groupMacroArgs, fill: true, render: renderTimeGroup},
	"timeGroupAlias":      {args: _groupMacroArgs, fill: true, render: renderTimeGroup},
	"unixEpochFilter":     {args: 1, render: renderUnixEpochFilter},
	"unixEpochNanoFilter": {args: 1, render: renderUnixEpochFilter},
	"unixEpochFrom":       {render: renderUnixEpochBound},
	"unixEpochTo":         {render: renderUnixEpochBound},
	"unixEpochNanoFrom":   {render: renderUnixEpochBound},
	"unixEpochNanoTo":     {render: renderUnixEpochBound},
	"unixEpochGroup":      {args: _groupMacroArgs, fill: true, render: renderUnixEpochGroup},
	"unixEpochGroupAlias": {args: _groupMacroArgs, fill: true, render: renderUnixEpochGroup},
}

// sqlMacro is one SQL macro call.
type sqlMacro struct {
	// args specifies how many arguments the call takes.
	args int

	// fill indicates that the call takes a fill mode as one more,
	// optional argument.
	fill bool

	// render returns the SQL of a call named name with the given
	// arguments, already trimmed and counted.
	render func(e *sqlExpander, name string, args []string) (string, error)
}

// renderTime renders $__time and $__timeEpoch.
func renderTime(e *sqlExpander, _ string, args []string) (string, error) {
	return e.d.epoch(args[0]) + e.d.timeAlias, nil
}

// renderTimeFilter renders $__timeFilter.
func renderTimeFilter(e *sqlExpander, _ string, args []string) (string, error) {
	return e.d.between("("+args[0]+")", e.tr.From.UTC(), e.tr.To.UTC()), nil
}

// renderTimeFrom renders $__timeFrom.
func renderTimeFrom(e *sqlExpander, _ string, _ []string) (string, error) {
	return e.d.timestamp(e.tr.From.UTC()), nil
}

// renderTimeTo renders $__timeTo.
func renderTimeTo(e *sqlExpander, _ string, _ []string) (string, error) {
	return e.d.timestamp(e.tr.To.UTC()), nil
}

// renderTimeGroup renders $__timeGroup and $__timeGroupAlias.
func renderTimeGroup(e *sqlExpander, name string, args []string) (string, error) {
	return renderGroup(e, name, e.d.epoch(args[0]), args[1:])
}

// renderUnixEpochGroup renders $__unixEpochGroup and
// $__unixEpochGroupAlias. The column is parenthesised, so an expression
// such as a+b is divided as a whole.
func renderUnixEpochGroup(e *sqlExpander, name string, args []string) (string, error) {
	return renderGroup(e, name, "("+args[0]+")", args[1:])
}

// renderGroup floors the unix seconds in epoch to the interval in args,
// naming the result the time column for an Alias macro. A fill mode after
// the interval becomes the query's.
func renderGroup(e *sqlExpander, name, epoch string, args []string) (string, error) {
	seconds, err := parseInterval(args[0], e.d.stringQuotes)
	if err != nil {
		return "", err
	}

	if len(args) > 1 {
		if e.fill, err = newSQLFill(args[1], seconds, e.tr); err != nil {
			return "", err
		}
	}

	out := fmt.Sprintf("floor(%s/%d)*%d", epoch, seconds, seconds)

	if strings.HasSuffix(name, "Alias") {
		out += e.d.timeAlias
	}

	return out, nil
}

// renderUnixEpochFilter renders $__unixEpochFilter and
// $__unixEpochNanoFilter.
func renderUnixEpochFilter(e *sqlExpander, name string, args []string) (string, error) {
	from, to := e.tr.From.Unix(), e.tr.To.Unix()

	if strings.Contains(name, "Nano") {
		from, to = e.tr.From.UnixNano(), e.tr.To.UnixNano()
	}

	return fmt.Sprintf("(%s) >= %d AND (%s) <= %d", args[0], from, args[0], to), nil
}

// renderUnixEpochBound renders $__unixEpochFrom, $__unixEpochTo and their
// Nano forms.
func renderUnixEpochBound(e *sqlExpander, name string, _ []string) (string, error) {
	t := e.tr.From
	if strings.HasSuffix(name, "To") {
		t = e.tr.To
	}

	if strings.Contains(name, "Nano") {
		return strconv.FormatInt(t.UnixNano(), 10), nil
	}

	return strconv.FormatInt(t.Unix(), 10), nil
}

// parseInterval parses an interval such as 5m, or one written as a string
// in one of quotes such as '5m', into seconds.
func parseInterval(arg, quotes string) (int64, error) {
	s := arg
	if len(s) >= 2 && s[0] == s[len(s)-1] && strings.IndexByte(quotes, s[0]) >= 0 {
		s = s[1 : len(s)-1]
	}

	var mult int64

	if s != "" {
		switch s[len(s)-1] {
		case 's':
			mult = 1
		case 'm':
			mult = _secondsPerMinute
		case 'h':
			mult = _secondsPerHour
		case 'd':
			mult = _secondsPerDay
		case 'w':
			mult = _secondsPerWeek
		}
	}

	if mult == 0 {
		return 0, invalidIntervalError(arg)
	}

	n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil || n <= 0 || n > _maxIntervalSeconds/mult {
		return 0, invalidIntervalError(arg)
	}

	return n * mult, nil
}

// invalidIntervalError returns the error for an interval that is not one.
func invalidIntervalError(arg string) error {
	return NewInvalidQueryError(fmt.Sprintf("interval %s must be a whole number of s, m, h, d or w, at most 10 years", arg))
}
