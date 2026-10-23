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
	type tcase struct {
		OrganizationID string
		Result         int64
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Installation not found": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				OrganizationID: "non-existent-org-id",
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, org)

			return tcase{
				OrganizationID: org,
				Result:         ids[0],
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchGithubInstallationByOrganizationID(context.Background(), c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchGithubInstallationOrganizationID(t *testing.T) {
	type tcase struct {
		InstallationID int64
		Result         null.String
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Installation not found": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				InstallationID: 404,
				Err:            sql.ErrNoRows,
			}
		},
		"Unassigned installation": func(t *testing.T, db *DB) tcase {
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
			}
		},
		"Assigned installation": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, org)

			return tcase{
				InstallationID: ids[0],
				Result:         null.StringFrom(org),
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchGithubInstallationOrganizationID(context.Background(), c.InstallationID)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_UpdateGithubInstallationOrganizationID(t *testing.T) {
	type tcase struct {
		InstallationID int64
		OrganizationID string
		// Owner is the organization the installation belongs to afterwards.
		Owner null.String
		Err   error
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
		"An already claimed installation is not reassigned": func(t *testing.T, db *DB) tcase {
			orgs := prepOrganizations(t, db, 2)

			const installationID = int64(4242)

			require.NoError(t, db.InsertGithubInstallation(context.Background(), installationID))
			require.NoError(t, db.UpdateGithubInstallationOrganizationID(
				context.Background(), installationID, orgs[0],
			))

			// a second connect must lose the race rather than take the
			// installation away from the organization that claimed it.
			return tcase{
				InstallationID: installationID,
				OrganizationID: orgs[1],
				Owner:          null.StringFrom(orgs[0]),
				Err:            errutil.ErrNotFound,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ids := prepGithubInstallations(t, db, 1, "")

			return tcase{
				InstallationID: ids[0],
				OrganizationID: org,
				Owner:          null.StringFrom(org),
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateGithubInstallationOrganizationID(context.Background(), c.InstallationID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			var organizationID null.String

			q, args := db.builder.Select("fk_organization_id").
				From("github_installations").
				Where(sq.Eq{
					"installation_id": c.InstallationID,
				}).MustSql()

			err = db.sql.Get(&organizationID, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Owner, organizationID)
		})
	}
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

func Test_agent_DeleteGithubInstallationsByOrganizationID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		KeptID           int64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			prepGithubInstallations(t, db, 1, org)

			return tcase{
				CancelledContext: true,
				OrganizationID:   org,
				Err:              assert.AnError,
			}
		},
		"Only installations of the organization are deleted": func(t *testing.T, db *DB) tcase {
			orgs := prepOrganizations(t, db, 2)

			prepGithubInstallations(t, db, 1, orgs[0])

			// the fixture numbers installations from one, so the second
			// organization's row is inserted with an id of its own.
			const keptID = int64(2)

			q, args := db.builder.Insert("github_installations").
				SetMap(map[string]any{
					"installation_id":    keptID,
					"fk_organization_id": orgs[1],
				}).MustSql()

			_, err := db.sql.Exec(q, args...)
			require.NoError(t, err)

			return tcase{
				OrganizationID: orgs[0],
				KeptID:         keptID,
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

			err := db.DeleteGithubInstallationsByOrganizationID(ctx, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var ids []int64

			q, args := db.builder.Select("installation_id").From("github_installations").MustSql()

			require.NoError(t, db.sql.Select(&ids, q, args...))
			assert.Equal(t, []int64{c.KeptID}, ids)
		})
	}
}
