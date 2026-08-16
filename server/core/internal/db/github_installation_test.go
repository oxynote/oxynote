package db

import (
	"context"
	"database/sql"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepGithubInstallations(t *testing.T, db *DB, count int, organizationID string) []int64 { //nolint:unparam // fixture helpers share a uniform signature
	t.Helper()

	res := make([]int64, count)

	for i := range count {
		id := int64(i + 1)

		res[i] = id

		vals := map[string]any{
			"installation_id": id,
		}

		if organizationID != "" {
			vals["fk_organization_id"] = organizationID
		}

		q, args := db.builder.Insert("github_installations").
			SetMap(vals).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertGithubInstallation(t *testing.T) {
	type tcase struct {
		InstallationID int64
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate installation ID": func(t *testing.T, db *DB) tcase {
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
				Err:            assert.AnError,
			}
		},
		"Successful insert": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				InstallationID: 123,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertGithubInstallation(context.Background(), c.InstallationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var installationID int64

			q, args := db.builder.Select("installation_id").
				From("github_installations").
				Where(sq.Eq{
					"installation_id": c.InstallationID,
				}).MustSql()

			err = db.sql.Get(&installationID, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.InstallationID, installationID)
		})
	}
}

func Test_agent_FetchGithubInstallationByOrganizationID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchGithubInstallationByOrganizationID(context.Background(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Zero(t, res)

	// success
	org := prepOrganizations(t, db, 1)[0]
	ids := prepGithubInstallations(t, db, 1, org)

	res, err = db.FetchGithubInstallationByOrganizationID(context.Background(), org)
	assert.NoError(t, err)
	assert.Equal(t, ids[0], res)
}

func Test_agent_FetchGithubInstallationOrganizationID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchGithubInstallationOrganizationID(context.Background(), 404)
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Zero(t, res)

	// success - unassigned installation
	org := prepOrganizations(t, db, 1)[0]
	ids := prepGithubInstallations(t, db, 1, "")

	res, err = db.FetchGithubInstallationOrganizationID(context.Background(), ids[0])
	require.NoError(t, err)
	assert.False(t, res.Valid)

	// success - assigned installation
	q, args := db.builder.Update("github_installations").
		SetMap(map[string]any{
			"fk_organization_id": org,
		}).
		Where(sq.Eq{
			"installation_id": ids[0],
		}).MustSql()

	_, err = db.sql.Exec(q, args...)
	require.NoError(t, err)

	res, err = db.FetchGithubInstallationOrganizationID(context.Background(), ids[0])
	require.NoError(t, err)
	assert.Equal(t, null.StringFrom(org), res)
}

func Test_agent_UpdateGithubInstallationOrganizationID(t *testing.T) {
	type tcase struct {
		InstallationID int64
		OrganizationID string
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent organization": func(t *testing.T, db *DB) tcase {
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
				OrganizationID: "non-existent-org-id",
				Err:            assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
				OrganizationID: org,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateGithubInstallationOrganizationID(context.Background(), c.InstallationID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var organizationID null.String

			q, args := db.builder.Select("fk_organization_id").
				From("github_installations").
				Where(sq.Eq{
					"installation_id": c.InstallationID,
				}).MustSql()

			err = db.sql.Get(&organizationID, q, args...)
			require.NoError(t, err)
			assert.Equal(t, null.StringFrom(c.OrganizationID), organizationID)
		})
	}

	t.Run("An already claimed installation is not reassigned", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		orgs := prepOrganizations(t, db, 2)

		const installationID = int64(4242)

		require.NoError(t, db.InsertGithubInstallation(context.Background(), installationID))
		require.NoError(t, db.UpdateGithubInstallationOrganizationID(
			context.Background(), installationID, orgs[0],
		))

		// a second connect must lose the race rather than take the
		// installation away from the organization that claimed it.
		err := db.UpdateGithubInstallationOrganizationID(
			context.Background(), installationID, orgs[1],
		)
		testutil.AssertEqualError(t, errutil.ErrNotFound, err)

		res, err := db.FetchGithubInstallationByOrganizationID(context.Background(), orgs[0])
		require.NoError(t, err)
		assert.Equal(t, installationID, res)
	})
}

func Test_agent_DeleteGithubInstallation(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		InstallationID   int64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				CancelledContext: true,
				InstallationID:   ids[0],
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
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

			err := db.DeleteGithubInstallation(ctx, c.InstallationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var installationID int64

			q, args := db.builder.Select("installation_id").
				From("github_installations").
				Where(sq.Eq{
					"installation_id": c.InstallationID,
				}).MustSql()

			err = db.sql.Get(&installationID, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
		})
	}
}

func Test_agent_UnassignGithubInstallationOrganization(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		InstallationID   int64
		OrganizationID   string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, org)

			return tcase{
				CancelledContext: true,
				InstallationID:   ids[0],
				OrganizationID:   org,
				Err:              assert.AnError,
			}
		},
		"Successful unassign": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, org)

			return tcase{
				InstallationID: ids[0],
				OrganizationID: org,
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

			err := db.UnassignGithubInstallationOrganization(ctx, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var organizationID null.String

			q, args := db.builder.Select("fk_organization_id").
				From("github_installations").
				Where(sq.Eq{
					"installation_id": c.InstallationID,
				}).MustSql()

			err = db.sql.Get(&organizationID, q, args...)
			require.NoError(t, err)
			assert.False(t, organizationID.Valid)
		})
	}
}
