package processor

import (
	"fmt"
	"strconv"
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
	return tr.processSQLQuery(q, sqlMacros{
		time:                pgMacroTime,
		timeFilter:          pgMacroTimeFilter,
		timeFrom:            pgMacroTimeFrom,
		timeTo:              pgMacroTimeTo,
		timeGroup:           pgMacroTimeGroup,
		timeGroupAlias:      pgMacroTimeGroupAlias,
		unixEpochGroupAlias: pgMacroUnixEpochGroupAlias,
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
