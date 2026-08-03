package processor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// _pgMacroRe matches PostgreSQL macro invocations: $__macroName(args...).
var _pgMacroRe = regexp.MustCompile(`\$__(\w+)\(([^)]*)\)`)

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

	q = _pgMacroRe.ReplaceAllStringFunc(q, func(match string) string {
		submatch := _pgMacroRe.FindStringSubmatch(match)
		if len(submatch) != 3 {
			return match
		}

		name := submatch[1]
		args := parseMacroArgs(submatch[2])

		switch name {
		case "time", "timeEpoch":
			return pgMacroTime(args, match)
		case "timeFilter":
			return pgMacroTimeFilter(tr, args, match)
		case "timeFrom":
			return pgMacroTimeFrom(tr)
		case "timeTo":
			return pgMacroTimeTo(tr)
		case "timeGroup":
			return pgMacroTimeGroup(args, match)
		case "timeGroupAlias":
			return pgMacroTimeGroupAlias(args, match)
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
			return pgMacroUnixEpochGroupAlias(args, match)
		default:
			return match
		}
	})

	return q
}

// parseMacroArgs parses comma-separated macro arguments, stripping surrounding
// whitespace and quotes from each argument.
func parseMacroArgs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	args := make([]string, len(parts))

	for i, p := range parts {
		args[i] = strings.Trim(strings.TrimSpace(p), "'\"")
	}

	return args
}

// parseInterval parses an interval string (e.g., "5m", "1h", "30s") to seconds.
func parseInterval(s string) (int64, bool) {
	if len(s) < 2 {
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
		return num * 60, true
	case 'h':
		return num * 3600, true
	case 'd':
		return num * 86400, true
	case 'w':
		return num * 604800, true
	default:
		return 0, false
	}
}

// $__time(dateColumn) / $__timeEpoch(dateColumn)
func pgMacroTime(args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("EXTRACT(EPOCH FROM %s) AS \"time\"", args[0])
}

// $__timeFilter(dateColumn)
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

// $__timeFrom()
func pgMacroTimeFrom(tr TimeRange) string {
	return fmt.Sprintf("'%s'::timestamptz", tr.From.UTC().Format(time.RFC3339))
}

// $__timeTo()
func pgMacroTimeTo(tr TimeRange) string {
	return fmt.Sprintf("'%s'::timestamptz", tr.To.UTC().Format(time.RFC3339))
}

// $__timeGroup(dateColumn,'5m'[, fill])
func pgMacroTimeGroup(args []string, original string) string {
	if len(args) < 2 {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(EXTRACT(EPOCH FROM %s)/%d)*%d", args[0], seconds, seconds)
}

// $__timeGroupAlias(dateColumn,'5m'[, fill])
func pgMacroTimeGroupAlias(args []string, original string) string {
	if len(args) < 2 {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(EXTRACT(EPOCH FROM %s)/%d)*%d AS \"time\"", args[0], seconds, seconds)
}

// $__unixEpochFilter(dateColumn)
func pgMacroUnixEpochFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s >= %d AND %s <= %d",
		args[0], tr.From.Unix(),
		args[0], tr.To.Unix(),
	)
}

// $__unixEpochFrom()
func pgMacroUnixEpochFrom(tr TimeRange) string {
	return strconv.FormatInt(tr.From.Unix(), 10)
}

// $__unixEpochTo()
func pgMacroUnixEpochTo(tr TimeRange) string {
	return strconv.FormatInt(tr.To.Unix(), 10)
}

// $__unixEpochNanoFilter(dateColumn)
func pgMacroUnixEpochNanoFilter(tr TimeRange, args []string, original string) string {
	if len(args) < 1 {
		return original
	}

	return fmt.Sprintf("%s >= %d AND %s <= %d",
		args[0], tr.From.UnixNano(),
		args[0], tr.To.UnixNano(),
	)
}

// $__unixEpochNanoFrom()
func pgMacroUnixEpochNanoFrom(tr TimeRange) string {
	return strconv.FormatInt(tr.From.UnixNano(), 10)
}

// $__unixEpochNanoTo()
func pgMacroUnixEpochNanoTo(tr TimeRange) string {
	return strconv.FormatInt(tr.To.UnixNano(), 10)
}

// $__unixEpochGroup(dateColumn,'5m'[, fill])
func pgMacroUnixEpochGroup(args []string, original string) string {
	if len(args) < 2 {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(%s/%d)*%d", args[0], seconds, seconds)
}

// $__unixEpochGroupAlias(dateColumn,'5m'[, fill])
func pgMacroUnixEpochGroupAlias(args []string, original string) string {
	if len(args) < 2 {
		return original
	}

	seconds, ok := parseInterval(args[1])
	if !ok {
		return original
	}

	return fmt.Sprintf("floor(%s/%d)*%d AS \"time\"", args[0], seconds, seconds)
}
