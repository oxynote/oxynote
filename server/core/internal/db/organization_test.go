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

func prepOrganizations(t *testing.T, db *DB, count int) []string {
	t.Helper()

	res := make([]string, count)

	for i := range count {
		id := xid.New().String()

		res[i] = id

		// the slug embeds the random ID so repeated fixture calls
		// within one database never collide on the unique constraint.
		q, args := db.builder.Insert("organizations").Values(
			id,
			"Organization "+strconv.Itoa(i),
			"org-"+id,
			"",
			timeutil.Now().Truncate(time.Second),
			"",
		).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepOrganizationMembers(t *testing.T, db *DB, organizationID string, userIDs []string) {
	t.Helper()

	now := timeutil.Now().Truncate(time.Second)

	for i, userID := range userIDs {
		q, args := db.builder.Insert("organization_members").
			SetMap(map[string]any{
				"id":                 xid.New().String(),
				"fk_organization_id": organizationID,
				"fk_user_id":         userID,
				"role":               "member",
				// distinct timestamps keep the fetch order deterministic.
				"created_at": now.Add(time.Duration(i) * time.Second),
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}
}

func Test_agent_FetchOrganizationMembers(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Result           []string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   "org-id",
				Err:              assert.AnError,
			}
		},
		"Organization without members": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				OrganizationID: "non-existent-org-id",
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			users := prepUsers(t, db, 3)
			prepOrganizationMembers(t, db, org, users)

			return tcase{
				OrganizationID: org,
				Result:         users,
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

			res, err := db.FetchOrganizationMembers(ctx, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_CheckOrganizationMember(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		Result           bool
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			users := prepUsers(t, db, 1)

			return tcase{
				CancelledContext: true,
				OrganizationID:   org,
				UserID:           users[0],
				Err:              assert.AnError,
			}
		},
		"User is not a member": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			users := prepUsers(t, db, 1)

			return tcase{
				OrganizationID: org,
				UserID:         users[0],
			}
		},
		"User is a member": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			users := prepUsers(t, db, 1)
			prepOrganizationMembers(t, db, org, users)

			return tcase{
				OrganizationID: org,
				UserID:         users[0],
				Result:         true,
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

			ok, err := db.CheckOrganizationMember(ctx, c.OrganizationID, c.UserID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.False(t, ok)

				return
			}

			assert.Equal(t, c.Result, ok)
		})
	}
}

func Test_agent_FetchOrganizationSlug(t *testing.T) {
	type tcase struct {
		OrganizationID string
		Result         string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent organization": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				OrganizationID: "non-existent-org-id",
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				OrganizationID: org,
				Result:         "org-" + org,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			slug, err := db.FetchOrganizationSlug(context.Background(), c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Empty(t, slug)

				return
			}

			assert.Equal(t, c.Result, slug)
		})
	}
}

func Test_agent_UpdateOrganizationLogo(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Logo             string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   org,
				Logo:             "new-logo",
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				OrganizationID: org,
				Logo:           "new-logo",
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

			err := db.UpdateOrganizationLogo(ctx, c.OrganizationID, c.Logo)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var logo string

			q, args := db.builder.Select("logo").
				From("organizations").
				Where(sq.Eq{
					"id": c.OrganizationID,
				}).MustSql()

			err = db.sql.Get(&logo, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Logo, logo)
		})
	}
}
