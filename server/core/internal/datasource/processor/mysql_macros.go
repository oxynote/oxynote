package processor

import (
	"fmt"
	"time"
)

// _mySQLDialect lexes a query the way MySQL and MariaDB read it in their
// default SQL mode, where double quotes open a string and a backslash
// escapes inside one, and renders the time macros in their terms.
var _mySQLDialect = sqlDialect{
	stringQuotes:       `'"`,
	nameQuote:          '`',
	backslashStrings:   true,
	spacedDashComments: true,
	hashComments:       true,
	executableComments: true,
	epoch:              mysqlEpoch,
	between:            mysqlBetween,
	timestamp:          mysqlTimestamp,
	timeAlias:          " AS `time`",
}

// mysqlEpoch renders the unix seconds of a time expression.
func mysqlEpoch(expr string) string {
	return "UNIX_TIMESTAMP(" + expr + ")"
}

// mysqlBetween renders expr lying between two times.
func mysqlBetween(expr string, from, to time.Time) string {
	return fmt.Sprintf("%s BETWEEN %s AND %s", expr, mysqlTimestamp(from), mysqlTimestamp(to))
}

// mysqlTimestamp renders a time as a DATETIME.
func mysqlTimestamp(t time.Time) string {
	return fmt.Sprintf("FROM_UNIXTIME(%d)", t.Unix())
}
