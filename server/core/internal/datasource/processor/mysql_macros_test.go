package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_mysqlEpoch(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "UNIX_TIMESTAMP(`createdAt`)", mysqlEpoch("`createdAt`"))
}

func Test_mysqlBetween(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"(ts) BETWEEN FROM_UNIXTIME(1705312800) AND FROM_UNIXTIME(1705316400)",
		mysqlBetween("(ts)", _testTimeRange.From, _testTimeRange.To),
	)
}

func Test_mysqlTimestamp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "FROM_UNIXTIME(1705312800)", mysqlTimestamp(_testTimeRange.From))
}
