package processor

import (
	"fmt"
	"regexp"
	"time"
)

// _mysqlMacroRe matches MySQL macro invocations: $__macroName(args...).
var _mysqlMacroRe = regexp.MustCompile(`\$__(\w+)\(([^)]*)\)`)

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

	q = _mysqlMacroRe.ReplaceAllStringFunc(q, func(match string) string {
		submatch := _mysqlMacroRe.FindStringSubmatch(match)
		if len(submatch) != _macroSubmatchCount {
			return match
		}

		name := submatch[1]
		args := parseMacroArgs(submatch[2])

		switch name {
		case _timeColumn, "timeEpoch":
			return mysqlMacroTime(args, match)
		case "timeFilter":
			return mysqlMacroTimeFilter(tr, args, match)
		case "timeFrom":
			return mysqlMacroTimeFrom(tr)
		case "timeTo":
			return mysqlMacroTimeTo(tr)
		case "timeGroup":
			return mysqlMacroTimeGroup(args, match)
		case "timeGroupAlias":
			return mysqlMacroTimeGroupAlias(args, match)
		case "unixEpochFilter":
			return pgMacroUnixEpochFilter(tr, args, match)
		case "unixEpochFrom":
			return pgMacroUnixEpochFrom(tr)
		case "unixEpochTo":
			return pgMacroUnixEpochTo(tr)
		case "unixEpochNanoFilter":
			return pgMacroUnixEpochNanoFilter(tr, args, match)
		case "unixEpochNanoFrom":
			return pgMacroUnixEpochNanoFrom(tr)
		case "unixEpochNanoTo":
			return pgMacroUnixEpochNanoTo(tr)
		case "unixEpochGroup":
			return pgMacroUnixEpochGroup(args, match)
		case "unixEpochGroupAlias":
			return mysqlMacroUnixEpochGroupAlias(args, match)
		default:
			return match
		}
	})

	return q
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
