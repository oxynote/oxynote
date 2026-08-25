package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_TimeRange_ProcessMySQLQuery(t *testing.T) {
	tests := map[string]struct {
		TimeRange TimeRange
		Query     string
		Result    string
	}{
		"$__time": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__time(created_at), value FROM metrics",
			Result:    "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics",
		},
		"$__timeEpoch": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeEpoch(created_at), value FROM metrics",
			Result:    "SELECT UNIX_TIMESTAMP(created_at) AS `time`, value FROM metrics",
		},
		"$__timeFilter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__timeFilter(created_at)",
			Result:    "SELECT * FROM metrics WHERE created_at BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)",
		},
		"$__timeFrom": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at >= $__timeFrom()",
			Result:    "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800)",
		},
		"$__timeTo": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at <= $__timeTo()",
			Result:    "SELECT * FROM metrics WHERE created_at <= FROM_UNIXTIME(1705316400)",
		},
		"$__timeFrom and $__timeTo together": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE created_at >= $__timeFrom() AND created_at <= $__timeTo()",
			Result:    "SELECT * FROM metrics WHERE created_at >= FROM_UNIXTIME(1705312800) AND created_at <= FROM_UNIXTIME(1705316400)",
		},
		"$__timeGroup with 5m": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"$__timeGroup with 1h": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'1h') FROM metrics",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/3600)*3600 FROM metrics",
		},
		"$__timeGroup with fill mode 0": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', 0), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"$__timeGroup with fill mode NULL": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', NULL), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"$__timeGroup with fill mode previous": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroup(created_at,'5m', previous) FROM metrics",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 FROM metrics",
		},
		"$__timeGroupAlias with 5m": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__timeGroupAlias(created_at,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT FLOOR(UNIX_TIMESTAMP(created_at)/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1",
		},
		"$__unixEpochFilter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__unixEpochFilter(epoch_col)",
			Result:    "SELECT * FROM metrics WHERE epoch_col >= 1705312800 AND epoch_col <= 1705316400",
		},
		"$__unixEpochFrom": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE epoch >= $__unixEpochFrom()",
			Result:    "SELECT * FROM metrics WHERE epoch >= 1705312800",
		},
		"$__unixEpochTo": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE epoch <= $__unixEpochTo()",
			Result:    "SELECT * FROM metrics WHERE epoch <= 1705316400",
		},
		"$__unixEpochNanoFilter": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE $__unixEpochNanoFilter(ns_col)",
			Result:    "SELECT * FROM metrics WHERE ns_col >= 1705312800000000000 AND ns_col <= 1705316400000000000",
		},
		"$__unixEpochNanoFrom": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE ns >= $__unixEpochNanoFrom()",
			Result:    "SELECT * FROM metrics WHERE ns >= 1705312800000000000",
		},
		"$__unixEpochNanoTo": {
			TimeRange: _testTimeRange,
			Query:     "SELECT * FROM metrics WHERE ns <= $__unixEpochNanoTo()",
			Result:    "SELECT * FROM metrics WHERE ns <= 1705316400000000000",
		},
		"$__unixEpochGroup with 5m": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroup(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT floor(epoch_col/300)*300, avg(value) FROM metrics GROUP BY 1",
		},
		"$__unixEpochGroup with fill mode": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroup(epoch_col,'5m', 0) FROM metrics",
			Result:    "SELECT floor(epoch_col/300)*300 FROM metrics",
		},
		"$__unixEpochGroupAlias with 5m": {
			TimeRange: _testTimeRange,
			Query:     "SELECT $__unixEpochGroupAlias(epoch_col,'5m'), avg(value) FROM metrics GROUP BY 1",
			Result:    "SELECT floor(epoch_col/300)*300 AS `time`, avg(value) FROM metrics GROUP BY 1",
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
		"$__timeGroupAlias with $__interval": {
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := tc.TimeRange.ProcessMySQLQuery(tc.Query)
			assert.Equal(t, tc.Result, result)
		})
	}
}
