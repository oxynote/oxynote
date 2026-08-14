package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepUsers(t *testing.T, db *DB, count int) []string {
	t.Helper()

	res := make([]string, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		id := xid.New().String()

		res[i] = id

		q, args := db.builder.Insert("users").
			SetMap(map[string]any{
				"id":             id,
				"name":           "User " + strconv.Itoa(i),
				"email":          id + "@test.test",
				"email_verified": true,
				"image":          "image-" + strconv.Itoa(i),
				"created_at":     now,
				"updated_at":     now,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_UpdateUserImage(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Image            string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)

			return tcase{
				CancelledContext: true,
				UserID:           users[0],
				Image:            "new-image",
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)

			return tcase{
				UserID: users[0],
				Image:  "new-image",
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.UpdateUserImage(ctx, c.UserID, c.Image)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var image string

			q, args := db.builder.Select("image").
				From("users").
				Where(sq.Eq{
					"id": c.UserID,
				}).MustSql()

			err = db.sql.Get(&image, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Image, image)
		})
	}
}

func Test_agent_FetchUserName(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	name, err := db.FetchUserName(context.Background(), "non-existent-user-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Empty(t, name)

	// success
	users := prepUsers(t, db, 1)

	name, err = db.FetchUserName(context.Background(), users[0])
	assert.NoError(t, err)
	assert.Equal(t, "User 0", name)
}
