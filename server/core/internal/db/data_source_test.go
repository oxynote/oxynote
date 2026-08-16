package db

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	"github.com/oxynote/oxynote/server/core/internal/datasource/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDataSources(t *testing.T, db *DB, count int, fn func(int, *datasource.DataSource)) []*datasource.DataSource {
	t.Helper()

	res := make([]*datasource.DataSource, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		ds := &datasource.DataSource{
			ID:          xid.New(),
			Name:        "Data Source " + strconv.Itoa(i),
			Type:        datasource.TypePrometheus,
			URL:         "http://prometheus-" + strconv.Itoa(i) + ".test",
			Credentials: processor.Credentials(`{"username":"user","password":"pass"}`),
			Status:      processor.ConnectionStatusSuccess,
			// distinct timestamps keep the fetch order deterministic.
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, ds)
		}

		if ds.OrganizationID == "" {
			ds.OrganizationID = prepOrganizations(t, db, 1)[0]
		}

		res[i] = ds

		safeCredentials, err := ds.Credentials.Encrypt(db.opts.DataSourceCredentialsSigningSecret)
		require.NoError(t, err)

		q, args := db.builder.Insert("data_sources").
			SetMap(map[string]any{
				"id":                 ds.ID,
				"fk_organization_id": ds.OrganizationID,
				"name":               ds.Name,
				"type":               ds.Type,
				"url":                ds.URL,
				"credentials":        safeCredentials,
				"status":             ds.Status,
				"created_at":         ds.CreatedAt,
				"updated_at":         ds.UpdatedAt,
			}).MustSql()

		_, err = db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDataSource(t *testing.T) {
	stubDataSource := func(organizationID, name string) *datasource.DataSource {
		return &datasource.DataSource{
			ID:             xid.New(),
			OrganizationID: organizationID,
			Name:           name,
			Type:           datasource.TypePrometheus,
			URL:            "http://prometheus.test",
			Credentials:    processor.Credentials(`{"token":"secret"}`),
			Status:         processor.ConnectionStatusSuccess,
			CreatedAt:      timeutil.Now().Truncate(time.Second),
		}
	}

	type tcase struct {
		InvalidSecret bool
		DataSource    *datasource.DataSource
		Err           error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Invalid signing secret": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				InvalidSecret: true,
				DataSource:    stubDataSource(org, "Data Source 1"),
				Err:           assert.AnError,
			}
		},
		"Duplicate name within the organization": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				DataSource: stubDataSource(ds.OrganizationID, ds.Name),
				Err: errutil.New(
					http.StatusBadRequest,
					"data_source.duplicate_name",
					"name is already in use",
				),
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				DataSource: stubDataSource(org, "Data Source 1"),
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			if c.InvalidSecret {
				db.opts.DataSourceCredentialsSigningSecret = "bad"
			}

			err := db.InsertDataSource(context.Background(), c.DataSource)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDataSource(context.Background(), c.DataSource.ID, c.DataSource.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.DataSource, res)
		})
	}
}

func Test_agent_UpdateDataSource(t *testing.T) {
	type tcase struct {
		InvalidSecret bool
		DataSource    *datasource.DataSource
		Err           error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Invalid signing secret": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				InvalidSecret: true,
				DataSource:    ds,
				Err:           assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			ds.Name = "Updated Data Source"
			ds.URL = "http://updated.test"
			ds.Credentials = processor.Credentials(`{"token":"rotated"}`)
			ds.Status = processor.ConnectionStatusSuccess
			ds.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))

			return tcase{
				DataSource: ds,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			if c.InvalidSecret {
				db.opts.DataSourceCredentialsSigningSecret = "bad"
			}

			err := db.UpdateDataSource(context.Background(), c.DataSource)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDataSource(context.Background(), c.DataSource.ID, c.DataSource.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.DataSource, res)
		})
	}

	t.Run("Another organization's row is untouched", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		ds := prepDataSources(t, db, 1, nil)[0]

		// the update must not reach a row owned by a different organization,
		// whatever identifier it is handed.
		foreign := *ds
		foreign.OrganizationID = prepOrganizations(t, db, 1)[0]
		foreign.Name = "renamed-by-another-org"

		require.NoError(t, db.UpdateDataSource(context.Background(), &foreign))

		res, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
		require.NoError(t, err)
		assert.Equal(t, ds.Name, res.Name)
	})
}

func Test_agent_DeleteDataSource(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		ID               xid.ID
		OrganizationID   string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				ID:               ds.ID,
				OrganizationID:   ds.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
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

			err := db.DeleteDataSource(ctx, c.ID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var id xid.ID

			q, args := db.builder.Select("id").
				From("data_sources").
				Where(sq.Eq{
					"id": c.ID,
				}).MustSql()

			err = db.sql.Get(&id, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
		})
	}
}

func Test_agent_FetchDataSource(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDataSource(context.Background(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// error - undecryptable credentials
	ds := prepDataSources(t, db, 1, nil)[0]

	q, args := db.builder.Update("data_sources").
		SetMap(map[string]any{
			"credentials": []byte("garbage"),
		}).
		Where(sq.Eq{
			"id": ds.ID,
		}).MustSql()

	_, err = db.sql.Exec(q, args...)
	require.NoError(t, err)

	res, err = db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
	require.Error(t, err)
	assert.Nil(t, res)

	// success
	ds = prepDataSources(t, db, 1, nil)[0]

	res, err = db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, ds, res)
}

func Test_agent_FetchDataSources(t *testing.T) {
	db := prepTempDB(t)

	// success - no data sources
	res, err := db.FetchDataSources(context.Background(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err = db.FetchDataSources(ctx, "non-existent-org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// error - undecryptable credentials
	org := prepOrganizations(t, db, 1)[0]
	ds := prepDataSources(t, db, 1, func(_ int, nds *datasource.DataSource) {
		nds.OrganizationID = org
	})[0]

	q, args := db.builder.Update("data_sources").
		SetMap(map[string]any{
			"credentials": []byte("garbage"),
		}).
		Where(sq.Eq{
			"id": ds.ID,
		}).MustSql()

	_, err = db.sql.Exec(q, args...)
	require.NoError(t, err)

	res, err = db.FetchDataSources(context.Background(), org)
	require.Error(t, err)
	assert.Nil(t, res)

	// success - ordered by creation time, newest first
	org = prepOrganizations(t, db, 1)[0]
	sources := prepDataSources(t, db, 3, func(_ int, nds *datasource.DataSource) {
		nds.OrganizationID = org
	})

	res, err = db.FetchDataSources(context.Background(), org)
	assert.NoError(t, err)
	require.Len(t, res, len(sources))

	for i, ds := range sources {
		testutil.AssertFilterEqual(t, *ds, res[i])
	}
}
