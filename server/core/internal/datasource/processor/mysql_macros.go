package processor

import (
	"fmt"
	"strings"
	"time"
)

// ProcessMySQLQuery processes the query string to replace MySQL-specific
// macros with appropriate SQL expressions.
//
// Supported macros:
//
//	$__time(dateColumn)                          → UNIX_TIMESTAMP(dateColumn) AS `time`
//	$__timeEpoch(dateColumn)                     → UNIX_TIMESTAMP(dateColumn) AS `time`
//	$__timeFilter(dateColumn)                    → dateColumn BETWEEN 'from' AND 'to'
//	$__timeFrom()                                → 'from'
//	$__timeTo()                                  → 'to'
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
	// Resolve generic macros first so they can be used as macro arguments
	// (e.g., $__timeGroupAlias("time", $__interval)).
	q = tr.ProcessQuery(q)

	return expandMacros(q, func(name string, args []string) (string, bool) {
		// the macro helpers fall back to the invocation itself when the
		// arguments do not fit, so the raw form is rebuilt for them.
		match := _macroPrefix + name + "(" + strings.Join(args, ",") + ")"

		switch name {
		case _timeColumn, "timeEpoch":
			return mysqlMacroTime(args, match), true
		case "timeFilter":
			return mysqlMacroTimeFilter(tr, args, match), true
		case "timeFrom":
			return mysqlMacroTimeFrom(tr), true
		case "timeTo":
			return mysqlMacroTimeTo(tr), true
		case "timeGroup":
			return mysqlMacroTimeGroup(args, match), true
		case "timeGroupAlias":
			return mysqlMacroTimeGroupAlias(args, match), true
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
			return mysqlMacroUnixEpochGroupAlias(args, match), true
		default:
			return "", false
		}
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
func mysqlMacroTimeFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s BETWEEN '%s' AND '%s'",
		args[0],
		tr.From.UTC().Format(time.RFC3339),
		tr.To.UTC().Format(time.RFC3339),
	)
}

// $__timeFrom().
func mysqlMacroTimeFrom(tr TimeRange) string {
	return fmt.Sprintf("'%s'", tr.From.UTC().Format(time.RFC3339))
}

// $__timeTo().
func mysqlMacroTimeTo(tr TimeRange) string {
	return fmt.Sprintf("'%s'", tr.To.UTC().Format(time.RFC3339))
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
