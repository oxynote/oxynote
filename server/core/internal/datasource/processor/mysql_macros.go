package processor

import "fmt"

// ProcessMySQLQuery processes the query string to replace MySQL-specific
// macros with appropriate SQL expressions.
//
// Supported macros:
//
//	$__time(dateColumn)                          → UNIX_TIMESTAMP(dateColumn) AS `time`
//	$__timeEpoch(dateColumn)                     → UNIX_TIMESTAMP(dateColumn) AS `time`
//	$__timeFilter(dateColumn)                    → dateColumn BETWEEN FROM_UNIXTIME(from) AND FROM_UNIXTIME(to)
//	$__timeFrom()                                → FROM_UNIXTIME(from)
//	$__timeTo()                                  → FROM_UNIXTIME(to)
//	$__timeGroup(dateColumn,'5m'[, fill])        → FLOOR(UNIX_TIMESTAMP(dateColumn)/300)*300
//	$__timeGroupAlias(dateColumn,'5m'[, fill])   → FLOOR(UNIX_TIMESTAMP(dateColumn)/300)*300 AS `time`
//	$__unixEpochFilter(dateColumn)               → dateColumn >= unixFrom AND dateColumn <= unixTo
//	$__unixEpochFrom()                           → unix seconds
//	$__unixEpochTo()                             → unix seconds
//	$__unixEpochNanoFilter(dateColumn)            → dateColumn >= nanoFrom AND dateColumn <= nanoTo
//	$__unixEpochNanoFrom()                       → unix nanoseconds
//	$__unixEpochNanoTo()                         → unix nanoseconds
//	$__unixEpochGroup(dateColumn,'5m'[, fill])   → floor(dateColumn/300)*300
//	$__unixEpochGroupAlias(dateColumn,'5m'[, fill]) → floor(dateColumn/300)*300 AS `time`
func (tr TimeRange) ProcessMySQLQuery(q string) string {
	return tr.processSQLQuery(q, sqlMacros{
		time:                mysqlMacroTime,
		timeFilter:          mysqlMacroTimeFilter,
		timeFrom:            mysqlMacroTimeFrom,
		timeTo:              mysqlMacroTimeTo,
		timeGroup:           mysqlMacroTimeGroup,
		timeGroupAlias:      mysqlMacroTimeGroupAlias,
		unixEpochGroupAlias: mysqlMacroUnixEpochGroupAlias,
	})
}

// $__time(dateColumn) / $__timeEpoch(dateColumn).
func mysqlMacroTime(args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("UNIX_TIMESTAMP(%s) AS `time`", args[0])
}

// $__timeFilter(dateColumn).
//
// NOTE: time bounds are rendered as FROM_UNIXTIME(unix) rather than datetime
// literals: MySQL/MariaDB truncate an RFC3339 'Z' suffix with a warning and
// read the literal in the session time zone, silently shifting the window on
// any non-UTC server.
func mysqlMacroTimeFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)",
		args[0],
		tr.From.Unix(),
		tr.To.Unix(),
	)
}

// $__timeFrom().
func mysqlMacroTimeFrom(tr TimeRange) string {
	return fmt.Sprintf("FROM_UNIXTIME(%d)", tr.From.Unix())
}

// $__timeTo().
func mysqlMacroTimeTo(tr TimeRange) string {
	return fmt.Sprintf("FROM_UNIXTIME(%d)", tr.To.Unix())
}

// $__timeGroup(dateColumn,'5m'[, fill]).
func mysqlMacroTimeGroup(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("FLOOR(UNIX_TIMESTAMP(%s)/%d)*%d", args[0], seconds, seconds)
}

// $__timeGroupAlias(dateColumn,'5m'[, fill]).
func mysqlMacroTimeGroupAlias(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("FLOOR(UNIX_TIMESTAMP(%s)/%d)*%d AS `time`", args[0], seconds, seconds)
}

// $__unixEpochGroupAlias(dateColumn,'5m'[, fill]).
func mysqlMacroUnixEpochGroupAlias(args []string, original string) string {
	if len(args) < _timeGroupMinArgs {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(%s/%d)*%d AS `time`", args[0], seconds, seconds)
}
