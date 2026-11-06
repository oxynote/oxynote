package processor

import (
	"fmt"
	"time"
)

// _postgreSQLDialect lexes a query the way PostgreSQL reads it with
// standard_conforming_strings on, the default since 9.1, and renders the
// time macros in PostgreSQL's terms.
var _postgreSQLDialect = sqlDialect{
	stringQuotes:   "'",
	nameQuote:      '"',
	eStrings:       true,
	dollarStrings:  true,
	nestedComments: true,
	epoch:          pgEpoch,
	between:        pgBetween,
	timestamp:      pgTimestamp,
	timeAlias:      ` AS "time"`,
}

// pgEpoch renders the unix seconds of a time expression.
func pgEpoch(expr string) string {
	return "EXTRACT(EPOCH FROM " + expr + ")"
}

// pgBetween renders expr lying between two times.
func pgBetween(expr string, from, to time.Time) string {
	return fmt.Sprintf("%s BETWEEN %s AND %s", expr, pgTimestamp(from), pgTimestamp(to))
}

// pgTimestamp renders a time as a timestamptz literal.
func pgTimestamp(t time.Time) string {
	return "'" + t.Format(time.RFC3339) + "'::timestamptz"
}
