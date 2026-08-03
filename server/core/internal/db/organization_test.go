package db

import (
	"strconv"
	"testing"
	"time"

	"github.com/oxynote/purse/util/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/require"
)

func prepOrganizations(t *testing.T, db *DB, count int) []string {
	t.Helper()

	res := make([]string, count)

	for i := range count {
		id := xid.New().String()

		res[i] = id

		q, args := db.builder.Insert("organizations").Values(
			id,
			"Organization "+strconv.Itoa(i),
			"org-"+strconv.Itoa(i),
			"",
			timeutil.Now().Truncate(time.Second),
			"",
		).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}
