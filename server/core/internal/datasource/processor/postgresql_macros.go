package processor

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// _timeGroupMinArgs specifies the minimum number of arguments a
	// time-group macro requires (column, interval).
	_timeGroupMinArgs = 2

	// _minIntervalLen specifies the minimum length of an interval
	// string (one digit plus a unit suffix).
	_minIntervalLen = 2

	// All available interval unit multipliers in seconds.
	_secondsPerMinute = 60
	_secondsPerHour   = 3600
	_secondsPerDay    = 86400
	_secondsPerWeek   = 604800
)

// ProcessPostgreSQLQuery processes the query string to replace PostgreSQL-specific
// macros with appropriate SQL expressions.
//
// Supported macros:
//
//	$__time(dateColumn)                          → EXTRACT(EPOCH FROM dateColumn) AS "time"
//	$__timeEpoch(dateColumn)                     → EXTRACT(EPOCH FROM dateColumn) AS "time"
//	$__timeFilter(dateColumn)                    → dateColumn BETWEEN 'from'::timestamptz AND 'to'::timestamptz
//	$__timeFrom()                                → 'from'::timestamptz
//	$__timeTo()                                  → 'to'::timestamptz
//	$__timeGroup(dateColumn,'5m'[, fill])        → floor(EXTRACT(EPOCH FROM dateColumn)/300)*300
//	$__timeGroupAlias(dateColumn,'5m'[, fill])   → floor(EXTRACT(EPOCH FROM dateColumn)/300)*300 AS "time"
//	$__unixEpochFilter(dateColumn)               → dateColumn >= unixFrom AND dateColumn <= unixTo
//	$__unixEpochFrom()                           → unix seconds
//	$__unixEpochTo()                             → unix seconds
//	$__unixEpochNanoFilter(dateColumn)            → dateColumn >= nanoFrom AND dateColumn <= nanoTo
//	$__unixEpochNanoFrom()                       → unix nanoseconds
//	$__unixEpochNanoTo()                         → unix nanoseconds
//	$__unixEpochGroup(dateColumn,'5m'[, fill])   → floor(dateColumn/300)*300
//	$__unixEpochGroupAlias(dateColumn,'5m'[, fill]) → floor(dateColumn/300)*300 AS "time"
func (tr TimeRange) ProcessPostgreSQLQuery(q string) string {
	// Resolve generic macros first so they can be used as macro arguments
	// (e.g., $__timeGroupAlias("time", $__interval)).
	q = tr.ProcessQuery(q)

	return expandMacros(q, func(name string, args []string) (string, bool) {
		// the macro helpers fall back to the invocation itself when the
		// arguments do not fit, so the raw form is rebuilt for them.
		match := _macroPrefix + name + "(" + strings.Join(args, ",") + ")"

		switch name {
		case _timeColumn, "timeEpoch":
			return pgMacroTime(args, match), true
		case "timeFilter":
			return pgMacroTimeFilter(tr, args, match), true
		case "timeFrom":
			return pgMacroTimeFrom(tr), true
		case "timeTo":
			return pgMacroTimeTo(tr), true
		case "timeGroup":
			return pgMacroTimeGroup(args, match), true
		case "timeGroupAlias":
			return pgMacroTimeGroupAlias(args, match), true
		case "unixEpochFilter":
			return pgMacroUnixEpochFilter(tr, args, match), true
		case "unixEpochFrom":
			return pgMacroUnixEpochFrom(tr), true
		case "unixEpochTo":
			return pgMacroUnixEpochTo(tr), true
		case "unixEpochNanoFilter":
			return pgMacroUnixEpochNanoFilter(tr, args, match), true
		case "unixEpochNanoFrom":
			return pgMacroUnixEpochNanoFrom(tr), true
		case "unixEpochNanoTo":
			return pgMacroUnixEpochNanoTo(tr), true
		case "unixEpochGroup":
			return pgMacroUnixEpochGroup(args, match), true
		case "unixEpochGroupAlias":
			return pgMacroUnixEpochGroupAlias(args, match), true
		default:
			return "", false
		}
	})
}

// parseInterval parses an interval string (e.g., "5m", "1h", "30s") to seconds.
func parseInterval(s string) (int64, bool) {
	if len(s) < _minIntervalLen {
		return 0, false
	}

	unit := s[len(s)-1]

	num, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil || num <= 0 {
		return 0, false
	}

	switch unit {
	case 's':
		return num, true
	case 'm':
		return num * _secondsPerMinute, true
	case 'h':
		return num * _secondsPerHour, true
	case 'd':
		return num * _secondsPerDay, true
	case 'w':
		return num * _secondsPerWeek, true
	default:
		return 0, false
	}
}

// $__time(dateColumn) / $__timeEpoch(dateColumn).
func pgMacroTime(args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("EXTRACT(EPOCH FROM %s) AS \"time\"", args[0])
}

// $__timeFilter(dateColumn).
func pgMacroTimeFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s BETWEEN '%s'::timestamptz AND '%s'::timestamptz",
		args[0],
		tr.From.UTC().Format(time.RFC3339),
		tr.To.UTC().Format(time.RFC3339),
	)
}

// $__timeFrom().
func pgMacroTimeFrom(tr TimeRange) string {
	return fmt.Sprintf("'%s'::timestamptz", tr.From.UTC().Format(time.RFC3339))
}

// $__timeTo().
func pgMacroTimeTo(tr TimeRange) string {
	return fmt.Sprintf("'%s'::timestamptz", tr.To.UTC().Format(time.RFC3339))
}

// $__timeGroup(dateColumn,'5m'[, fill]).
func pgMacroTimeGroup(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(EXTRACT(EPOCH FROM %s)/%d)*%d", args[0], seconds, seconds)
}

// $__timeGroupAlias(dateColumn,'5m'[, fill]).
func pgMacroTimeGroupAlias(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(EXTRACT(EPOCH FROM %s)/%d)*%d AS \"time\"", args[0], seconds, seconds)
}

// $__unixEpochFilter(dateColumn).
func pgMacroUnixEpochFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s >= %d AND %s <= %d",
		args[0], tr.From.Unix(),
		args[0], tr.To.Unix(),
	)
}

// $__unixEpochFrom().
func pgMacroUnixEpochFrom(tr TimeRange) string {
	return strconv.FormatInt(tr.From.Unix(), 10)
}

// $__unixEpochTo().
func pgMacroUnixEpochTo(tr TimeRange) string {
	return strconv.FormatInt(tr.To.Unix(), 10)
}

// $__unixEpochNanoFilter(dateColumn).
func pgMacroUnixEpochNanoFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s >= %d AND %s <= %d",
		args[0], tr.From.UnixNano(),
		args[0], tr.To.UnixNano(),
	)
}

// $__unixEpochNanoFrom().
func pgMacroUnixEpochNanoFrom(tr TimeRange) string {
	return strconv.FormatInt(tr.From.UnixNano(), 10)
}

// $__unixEpochNanoTo().
func pgMacroUnixEpochNanoTo(tr TimeRange) string {
	return strconv.FormatInt(tr.To.UnixNano(), 10)
}

// $__unixEpochGroup(dateColumn,'5m'[, fill]).
func pgMacroUnixEpochGroup(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(%s/%d)*%d", args[0], seconds, seconds)
}

// $__unixEpochGroupAlias(dateColumn,'5m'[, fill]).
func pgMacroUnixEpochGroupAlias(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(%s/%d)*%d AS \"time\"", args[0], seconds, seconds)
}
