package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_pgEpoch(t *testing.T) {
	t.Parallel()

	assert.Equal(t, `EXTRACT(EPOCH FROM "createdAt")`, pgEpoch(`"createdAt"`))
}

func Test_pgBetween(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		"(ts) BETWEEN '2024-01-15T10:00:00Z'::timestamptz AND '2024-01-15T11:00:00Z'::timestamptz",
		pgBetween("(ts)", _testTimeRange.From, _testTimeRange.To),
	)
}

func Test_pgTimestamp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "'2024-01-15T10:00:00Z'::timestamptz", pgTimestamp(_testTimeRange.From))
}
