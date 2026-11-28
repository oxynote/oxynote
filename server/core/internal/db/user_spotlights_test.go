package db

import (
	"context"
	"database/sql"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/user"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_agent_UpsertUserSpotlights(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Input            user.SpotlightsInput
		Check            func(t *testing.T, db *DB)
		Result           user.Spotlights
		Err              error
	}

	// prepSpotlights stores the given state for the user before the case
	// runs.
	prepSpotlights := func(t *testing.T, db *DB, userID string, state user.SpotlightState) {
		t.Helper()

		require.NoError(t, db.UpsertUserSpotlights(
			context.Background(),
			userID,
			user.SpotlightsInput{Star: state},
		))
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           prepUsers(t, db, 1)[0],
				Input:            user.SpotlightsInput{Star: user.SpotlightStateShown},
				Err:              assert.AnError,
			}
		},
		"Unknown user": func(*testing.T, *DB) tcase {
			return tcase{
				UserID: "unknown",
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Err:    assert.AnError,
			}
		},
		"Successful insertion": func(t *testing.T, db *DB) tcase {
			return tcase{
				UserID: prepUsers(t, db, 1)[0],
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)},
			}
		},
		"Dismissed replaces shown": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]
			prepSpotlights(t, db, userID, user.SpotlightStateShown)

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateDismissed},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
			}
		},
		"Shown does not replace dismissed": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]
			prepSpotlights(t, db, userID, user.SpotlightStateDismissed)

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
			}
		},
		"Other users keep their state": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)
			prepSpotlights(t, db, users[1], user.SpotlightStateShown)

			return tcase{
				UserID: users[0],
				Input:  user.SpotlightsInput{Star: user.SpotlightStateDismissed},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
				Check: func(t *testing.T, db *DB) {
					spotlights, err := db.FetchUserSpotlights(context.Background(), users[1])
					require.NoError(t, err)
					assert.Equal(t, user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)}, spotlights)
				},
			}
		},
		"State goes with the user": func(t *testing.T, db *DB) tcase {
			userID := prepUsers(t, db, 1)[0]

			return tcase{
				UserID: userID,
				Input:  user.SpotlightsInput{Star: user.SpotlightStateShown},
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateShown)},
				Check: func(t *testing.T, db *DB) {
					q, args := db.builder.Delete("users").Where(sq.Eq{"id": userID}).MustSql()

					_, err := db.sql.Exec(q, args...)
					require.NoError(t, err)

					var count int

					q, args = db.builder.Select("COUNT(*)").
						From("user_spotlights").
						Where(sq.Eq{"fk_user_id": userID}).
						MustSql()

					require.NoError(t, db.sql.Get(&count, q, args...))
					assert.Zero(t, count)
				},
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

			err := db.UpsertUserSpotlights(ctx, c.UserID, c.Input)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			spotlights, err := db.FetchUserSpotlights(context.Background(), c.UserID)
			require.NoError(t, err)
			assert.Equal(t, c.Result, spotlights)

			if c.Check != nil {
				c.Check(t, db)
			}
		})
	}
}

func Test_agent_FetchUserSpotlights(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		Result           user.Spotlights
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           prepUsers(t, db, 1)[0],
				Err:              assert.AnError,
			}
		},
		"Unknown user": func(*testing.T, *DB) tcase {
			return tcase{
				UserID: "unknown",
				Err:    sql.ErrNoRows,
			}
		},
		"User without a stored state": func(t *testing.T, db *DB) tcase {
			return tcase{
				UserID: prepUsers(t, db, 1)[0],
			}
		},
		"User with a stored state": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 2)

			for i, state := range []user.SpotlightState{user.SpotlightStateDismissed, user.SpotlightStateShown} {
				require.NoError(t, db.UpsertUserSpotlights(
					context.Background(),
					users[i],
					user.SpotlightsInput{Star: state},
				))
			}

			return tcase{
				UserID: users[0],
				Result: user.Spotlights{Star: null.ValueFrom(user.SpotlightStateDismissed)},
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

			spotlights, err := db.FetchUserSpotlights(ctx, c.UserID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, spotlights)
		})
	}
}
