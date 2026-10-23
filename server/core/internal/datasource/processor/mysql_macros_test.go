package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_ProcessMySQLQuery(t *testing.T) {
	cc := map[string]struct {
		TimeRange TimeRange
		Query     string
		Result    string
	}{
		"Time column": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__time(created_at), value FROM metrics",
			Result:    "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics",
		},
		"Time epoch column": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeEpoch(created_at), value FROM metrics",
			Result:    "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics",
		},
		"Time filter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__timeFilter(created_at)",
			Result:    "SELECT * FROM metrics WHERE created_at BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)",
		},
		"Range start": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at >= $__timeFrom()",
			Result:    "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800)",
		},
		"Range end": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at <= $__timeTo()",
			Result:    "SELECT * FROM metrics WHERE created_at <= FROM_UNIXTIME(1705316400)",
		},
		"Range start and end together": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at >= $__timeFrom() AND created_at <= $__timeTo()",
			Result:    "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800) AND created_at <= FROM_UNIXTIME(1705316400)",
		},
		"Time group by five minutes": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"Time group by one hour": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'1h') FROM metrics",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/3600)*3600 FROM metrics",
		},
		"Time group with zero fill mode": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', 0), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"Time group with NULL fill mode": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', NULL), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"Time group with previous fill mode": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', previous) FROM metrics",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 FROM metrics",
		},
		"Time group alias by five minutes": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1",
		},
		"Unix epoch filter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__unixEpochFilter(epoch_col)",
			Result:    "SELECT * FROM metrics WHERE epoch_col >= 1705312800 AND epoch_col <= 1705316400",
		},
		"Unix epoch range start": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE epoch >= $__unixEpochFrom()",
			Result:    "SELECT * FROM metrics WHERE epoch >= 1705312800",
		},
		"Unix epoch range end": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE epoch <= $__unixEpochTo()",
			Result:    "SELECT * FROM metrics WHERE epoch <= 1705316400",
		},
		"Unix epoch nanosecond filter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__unixEpochNanoFilter(ns_col)",
			Result:    "SELECT * FROM metrics WHERE ns_col >= 1705312800000000000 AND ns_col <= 1705316400000000000",
		},
		"Unix epoch nanosecond range start": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE ns >= $__unixEpochNanoFrom()",
			Result:    "SELECT * FROM metrics WHERE ns >= 1705312800000000000",
		},
		"Unix epoch nanosecond range end": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE ns <= $__unixEpochNanoTo()",
			Result:    "SELECT * FROM metrics WHERE ns <= 1705316400000000000",
		},
		"Unix epoch group by five minutes": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroup(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT floor(epoch_col/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"Unix epoch group with fill mode": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroup(epoch_col,'5m', 0) FROM metrics",
			Result:    "SELECT floor(epoch_col/300)*300 FROM metrics",
		},
		"Unix epoch group alias by five minutes": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroupAlias(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT floor(epoch_col/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1",
		},
		"Time column without a column is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__time() FROM metrics",
			Result:    "SELECT $__time() FROM metrics",
		},
		"Time filter without a column is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__timeFilter()",
			Result:    "SELECT * FROM metrics WHERE $__timeFilter()",
		},
		"Time group without an interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at) FROM metrics",
			Result:    "SELECT $__timeGroup(created_at) FROM metrics",
		},
		"Time group with an invalid interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5x') FROM metrics",
			Result:    "SELECT $__timeGroup(created_at,'5x') FROM metrics",
		},
		"Time group alias without an interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroupAlias(created_at) FROM metrics",
			Result:    "SELECT $__timeGroupAlias(created_at) FROM metrics",
		},
		"Time group alias with an invalid interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroupAlias(created_at,'5x') FROM metrics",
			Result:    "SELECT $__timeGroupAlias(created_at,'5x') FROM metrics",
		},
		"Unix epoch group alias without an interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroupAlias(epoch_col) FROM metrics",
			Result:    "SELECT $__unixEpochGroupAlias(epoch_col) FROM metrics",
		},
		"Unix epoch group alias with an invalid interval is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroupAlias(epoch_col,'5x') FROM metrics",
			Result:    "SELECT $__unixEpochGroupAlias(epoch_col,'5x') FROM metrics",
		},
		"Unknown macro is left unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unknown(col) FROM metrics",
			Result:    "SELECT $__unknown(col) FROM metrics",
		},
		"No macros leaves query unchanged": {
			TimeRange: _testTimeRange,
			Query:     "SELECT time, value FROM metrics WHERE time > NOW() - INTERVAL 1 HOUR",
			Result:    "SELECT time, value FROM metrics WHERE time > NOW() - INTERVAL 1 HOUR",
		},
		"Multiple macros in one query": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics WHERE $__timeFilter(created_at) GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 AS `time`, avg(value) FROM metrics WHERE created_at BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400) GROUP BY 1",
		},
		"Time group alias by the calculated interval": {
			TimeRange: _testTimeRange,
			Query: `SELECT
    $__timeGroupAlias(time, $__interval),
    service,
    COUNT(*) AS deployments
FROM deployments
WHERE $__timeFilter(time)
GROUP BY 1, 2
ORDER BY 1, 2`,
			Result: `SELECT
    FLOOR(UNIX_TIMESTAMP(time)/60)*60 AS ` + "`time`" + `,
    service,
    COUNT(*) AS deployments
FROM deployments
WHERE time BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)
GROUP BY 1, 2
ORDER BY 1, 2`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			result := c.TimeRange.ProcessMySQLQuery(c.Query)
			assert.Equal(t, c.Result, result)
		})
	}
}
