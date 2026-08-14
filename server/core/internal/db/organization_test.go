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

func prepOrganizations(t *testing.T, db *DB, count int) []string { //nolint:unparam // fixture helpers share a uniform signature
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
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchOrganizationMembers(ctx, "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no members
	res, err = db.FetchOrganizationMembers(context.Background(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success
	org := prepOrganizations(t, db, 1)[0]
	users := prepUsers(t, db, 3)
	prepOrganizationMembers(t, db, org, users)

	res, err = db.FetchOrganizationMembers(context.Background(), org)
	assert.NoError(t, err)
	assert.Equal(t, users, res)
}

func Test_agent_CheckOrganizationMember(t *testing.T) {
	db := prepTempDB(t)

	org := prepOrganizations(t, db, 1)[0]
	users := prepUsers(t, db, 1)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := db.CheckOrganizationMember(ctx, org, users[0])
	require.Error(t, err)
	assert.False(t, ok)

	// success - not a member
	ok, err = db.CheckOrganizationMember(context.Background(), org, users[0])
	require.NoError(t, err)
	assert.False(t, ok)

	// success - member
	prepOrganizationMembers(t, db, org, users)

	ok, err = db.CheckOrganizationMember(context.Background(), org, users[0])
	require.NoError(t, err)
	assert.True(t, ok)
}

func Test_agent_FetchOrganizationSlug(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	slug, err := db.FetchOrganizationSlug(context.Background(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Empty(t, slug)

	// success
	org := prepOrganizations(t, db, 1)[0]

	slug, err = db.FetchOrganizationSlug(context.Background(), org)
	assert.NoError(t, err)
	assert.Equal(t, "org-"+org, slug)
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
