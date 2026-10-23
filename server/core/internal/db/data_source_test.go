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
			Credentials: processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
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

		safeCredentials, err := ds.Credentials.Encrypt(db.opts.DataSourceCredentialsKeys, ds.ID.Bytes())
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
			Credentials:    processor.NewCredentials([]byte(`{"token":"secret"}`)),
			Status:         processor.ConnectionStatusSuccess,
			CreatedAt:      timeutil.Now().Truncate(time.Second),
		}
	}

	type tcase struct {
		DataSource *datasource.DataSource
		Err        error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Credentials that could not be read are refused": func(t *testing.T, db *DB) tcase {
			ds := stubDataSource(prepOrganizations(t, db, 1)[0], "Data Source 1")
			ds.Credentials = processor.NewCredentials([]byte("garbage"))
			require.Error(t, ds.Credentials.Decrypt(_dataSourceKeys, ds.ID.Bytes()))

			return tcase{
				DataSource: ds,
				Err:        assert.AnError,
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
		DataSource *datasource.DataSource
		Err        error
		// Check, when set, inspects the stored rows in place of comparing
		// DataSource with its row.
		Check func(t *testing.T, db *DB)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Successful update": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			ds.Name = "Updated Data Source"
			ds.URL = "http://updated.test"
			ds.Credentials = processor.NewCredentials([]byte(`{"token":"rotated"}`))
			ds.Status = processor.ConnectionStatusSuccess
			ds.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))

			return tcase{
				DataSource: ds,
			}
		},
		"Row owned by another organization is untouched": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			// the update must not reach a row owned by a different organization,
			// whatever identifier it is handed.
			foreign := *ds
			foreign.OrganizationID = prepOrganizations(t, db, 1)[0]
			foreign.Name = "renamed-by-another-org"

			return tcase{
				DataSource: &foreign,
				Check: func(t *testing.T, db *DB) {
					res, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
					require.NoError(t, err)
					assert.Equal(t, ds.Name, res.Name)
				},
			}
		},
		"Credentials that could not be read are not written back": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			poisonDataSourceCredentials(t, db, ds.ID)

			broken, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
			require.NoError(t, err)
			require.False(t, broken.Credentials.IsValid())

			// storing the data source as it came back would replace the
			// ciphertext with an encryption of the nothing left in its place.
			return tcase{
				DataSource: broken,
				Err:        assert.AnError,
			}
		},
		"Credentials that could not be read are replaced by new ones": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			poisonDataSourceCredentials(t, db, ds.ID)

			broken, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
			require.NoError(t, err)

			// what the user re-enters becomes the whole pair: there was nothing
			// readable left to merge onto.
			require.NoError(t, broken.ApplyUpdate(datasource.UpdateInput{
				Credentials: null.ValueFrom(processor.CredentialsUpdateInput(`{"username":"user","password":"pass"}`)),
			}))
			broken.Status = processor.ConnectionStatusSuccess

			return tcase{
				DataSource: broken,
				Check: func(t *testing.T, db *DB) {
					res, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
					require.NoError(t, err)
					assert.True(t, res.Credentials.IsValid())
					assert.Equal(
						t,
						processor.NewCredentials([]byte(`{"username":"user","password":"pass"}`)),
						res.Credentials,
					)
					assert.Equal(t, processor.ConnectionStatusSuccess, res.Status)
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpdateDataSource(context.Background(), c.DataSource)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Check != nil {
				c.Check(t, db)

				return
			}

			res, err := db.FetchDataSource(context.Background(), c.DataSource.ID, c.DataSource.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.DataSource, res)
		})
	}
}

func Test_agent_UpdateDataSourceStatus(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		ID               xid.ID
		OrganizationID   string
		Status           processor.ConnectionStatus
		Err              error
		// Check inspects the stored rows.
		Check func(t *testing.T, db *DB)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				ID:               ds.ID,
				OrganizationID:   ds.OrganizationID,
				Status:           processor.ConnectionStatusUnreachable,
				Err:              assert.AnError,
			}
		},
		"Row owned by another organization is untouched": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				ID:             ds.ID,
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Status:         processor.ConnectionStatusUnreachable,
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, processor.ConnectionStatusSuccess, fetchDataSourceStatus(t, db, ds.ID))
				},
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			credentials := fetchDataSourceCredentials(t, db, ds.ID)

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
				Status:         processor.ConnectionStatusUnreachable,
				Check: func(t *testing.T, db *DB) {
					exp := *ds
					exp.Status = processor.ConnectionStatusUnreachable

					res, err := db.FetchDataSource(context.Background(), ds.ID, ds.OrganizationID)
					require.NoError(t, err)
					testutil.AssertFilterEqual(t, &exp, res)

					// the credentials are not encrypted again.
					assert.Equal(t, credentials, fetchDataSourceCredentials(t, db, ds.ID))
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

			err := db.UpdateDataSourceStatus(ctx, c.ID, c.OrganizationID, c.Status)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			c.Check(t, db)
		})
	}
}

func Test_agent_ReencryptDataSourceCredentials(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Moved            int
		Unreadable       int
		Err              error
		// Check inspects the rows the sweep left behind.
		Check func(t *testing.T, db *DB)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			prepDataSources(t, db, 1, nil)

			return tcase{
				CancelledContext: true,
				Err:              assert.AnError,
			}
		},
		"No data sources": func(*testing.T, *DB) tcase {
			return tcase{}
		},
		"Rows on the newest key are left alone": func(t *testing.T, db *DB) tcase {
			sources := prepDataSources(t, db, 2, nil)
			before := [][]byte{
				fetchDataSourceCredentials(t, db, sources[0].ID),
				fetchDataSourceCredentials(t, db, sources[1].ID),
			}

			return tcase{
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, before[0], fetchDataSourceCredentials(t, db, sources[0].ID))
					assert.Equal(t, before[1], fetchDataSourceCredentials(t, db, sources[1].ID))
				},
			}
		},
		"Rows sealed under a retired key are moved": func(t *testing.T, db *DB) tcase {
			// the rows are written while the retired key was the only
			// one, then the keyring gains the key that replaces it.
			db.opts.DataSourceCredentialsKeys = mustParseKeys(_retiredDataSourceKey)
			retired := prepDataSources(t, db, 1, nil)[0]

			db.opts.DataSourceCredentialsKeys = mustParseKeys(_dataSourceKey + "," + _retiredDataSourceKey)
			current := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				Moved: 1,
				Check: func(t *testing.T, db *DB) {
					assert.True(t, db.opts.DataSourceCredentialsKeys.IsCurrent(fetchDataSourceCredentials(t, db, retired.ID)))
					assert.True(t, db.opts.DataSourceCredentialsKeys.IsCurrent(fetchDataSourceCredentials(t, db, current.ID)))

					// only the ciphertext changed: the row still reads as
					// it was written, and nothing the user sees moved.
					db.opts.DataSourceCredentialsKeys = _dataSourceKeys

					res, err := db.FetchDataSource(context.Background(), retired.ID, retired.OrganizationID)
					require.NoError(t, err)
					testutil.AssertFilterEqual(t, retired, res)
				},
			}
		},
		"Unreadable rows are skipped": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			poisonDataSourceCredentials(t, db, ds.ID)

			return tcase{
				Unreadable: 1,
				Check: func(t *testing.T, db *DB) {
					assert.Equal(t, []byte("garbage"), fetchDataSourceCredentials(t, db, ds.ID))
					assert.Equal(t, processor.ConnectionStatusSuccess, fetchDataSourceStatus(t, db, ds.ID))
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

			moved, unreadable, err := db.ReencryptDataSourceCredentials(ctx)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Moved, moved)
			assert.Equal(t, c.Unreadable, unreadable)

			if c.Check != nil {
				c.Check(t, db)
			}
		})
	}
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

// poisonDataSourceCredentials overwrites a data source's stored credentials
// with bytes no key decrypts, which is what a key dropped since the
// credentials were written leaves behind.
func poisonDataSourceCredentials(t *testing.T, db *DB, id xid.ID) {
	t.Helper()

	q, args := db.builder.Update("data_sources").
		SetMap(map[string]any{
			"credentials": []byte("garbage"),
		}).
		Where(sq.Eq{
			"id": id,
		}).
		MustSql()

	_, err := db.sql.Exec(q, args...)
	require.NoError(t, err)
}

// fetchDataSourceStatus reads the status column straight off the row, so a
// test can tell what was stored from what a fetch merely returned.
func fetchDataSourceStatus(t *testing.T, db *DB, id xid.ID) processor.ConnectionStatus {
	t.Helper()

	q, args := db.builder.Select("status").
		From("data_sources").
		Where(sq.Eq{
			"id": id,
		}).
		MustSql()

	var status processor.ConnectionStatus

	require.NoError(t, db.sql.Get(&status, q, args...))

	return status
}

// rejectInvalidEncryptionKeyStatus installs a constraint the status cannot
// satisfy, so the recording of an undecryptable data source fails while the
// row itself still reads fine. It is dropped again when the test ends.
func rejectInvalidEncryptionKeyStatus(t *testing.T, db *DB) {
	t.Helper()

	// NOT VALID leaves the rows already carrying the status alone; the
	// constraint still refuses every write that arrives after it.
	_, err := db.sql.Exec(
		`ALTER TABLE data_sources ADD CONSTRAINT status_not_invalid_encryption_key ` +
			`CHECK (status <> 'invalid_encryption_key') NOT VALID`,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := db.sql.Exec(`ALTER TABLE data_sources DROP CONSTRAINT status_not_invalid_encryption_key`)
		require.NoError(t, err)
	})
}

// fetchDataSourceCredentials reads a data source's stored ciphertext as it
// is.
func fetchDataSourceCredentials(t *testing.T, db *DB, id xid.ID) []byte {
	t.Helper()

	q, args := db.builder.Select("credentials").
		From("data_sources").
		Where(sq.Eq{
			"id": id,
		}).
		MustSql()

	var credentials []byte

	require.NoError(t, db.sql.Get(&credentials, q, args...))

	return credentials
}

func Test_agent_FetchDataSource(t *testing.T) {
	type tcase struct {
		ID             xid.ID
		OrganizationID string
		Err            error
		// Check inspects the result and the row it was read from.
		Check func(t *testing.T, db *DB, res *datasource.DataSource)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Data source not found": func(*testing.T, *DB) tcase {
			return tcase{
				ID:             xid.New(),
				OrganizationID: "non-existent-org-id",
				Err:            sql.ErrNoRows,
			}
		},
		"Undecryptable credentials are recorded": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			poisonDataSourceCredentials(t, db, ds.ID)

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
				Check: func(t *testing.T, db *DB, res *datasource.DataSource) {
					require.NotNil(t, res)
					assert.False(t, res.Credentials.IsValid())
					assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, res.Status)
					assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, fetchDataSourceStatus(t, db, ds.ID))
				},
			}
		},
		"Undecryptable credentials already recorded": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, func(_ int, nds *datasource.DataSource) {
				nds.Status = processor.ConnectionStatusInvalidEncryptionKey
			})[0]

			poisonDataSourceCredentials(t, db, ds.ID)

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
				Check: func(t *testing.T, _ *DB, res *datasource.DataSource) {
					require.NotNil(t, res)
					assert.False(t, res.Credentials.IsValid())
					assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, res.Status)
				},
			}
		},
		"Undecryptable credentials cannot be recorded": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			poisonDataSourceCredentials(t, db, ds.ID)
			rejectInvalidEncryptionKeyStatus(t, db)

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
				Err:            assert.AnError,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			// another organization's data source is stored first. The fetch
			// must pass over it.
			prepDataSources(t, db, 1, nil)
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				ID:             ds.ID,
				OrganizationID: ds.OrganizationID,
				Check: func(t *testing.T, _ *DB, res *datasource.DataSource) {
					testutil.AssertFilterEqual(t, ds, res)
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchDataSource(context.Background(), c.ID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			c.Check(t, db, res)
		})
	}
}

func Test_agent_FetchDataSources(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Err              error
		// Check inspects the result and the rows it was read from.
		Check func(t *testing.T, db *DB, res []datasource.DataSource)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"No data sources": func(*testing.T, *DB) tcase {
			return tcase{
				OrganizationID: "non-existent-org-id",
				Check: func(t *testing.T, _ *DB, res []datasource.DataSource) {
					assert.Empty(t, res)
				},
			}
		},
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   "non-existent-org-id",
				Err:              assert.AnError,
			}
		},
		"One undecryptable data source does not break the listing": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			mixed := prepDataSources(t, db, 2, func(_ int, nds *datasource.DataSource) {
				nds.OrganizationID = org
			})

			poisonDataSourceCredentials(t, db, mixed[1].ID)

			return tcase{
				OrganizationID: org,
				Check: func(t *testing.T, db *DB, res []datasource.DataSource) {
					require.Len(t, res, len(mixed))

					testutil.AssertFilterEqual(t, *mixed[0], res[0])

					assert.False(t, res[1].Credentials.IsValid())
					assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, res[1].Status)
					assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, fetchDataSourceStatus(t, db, mixed[1].ID))
				},
			}
		},
		"Undecryptable credentials cannot be recorded": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			ds := prepDataSources(t, db, 1, func(_ int, nds *datasource.DataSource) {
				nds.OrganizationID = org
			})[0]

			poisonDataSourceCredentials(t, db, ds.ID)
			rejectInvalidEncryptionKeyStatus(t, db)

			return tcase{
				OrganizationID: org,
				Err:            assert.AnError,
			}
		},
		"Ordered by creation time newest first": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			sources := prepDataSources(t, db, 3, func(_ int, nds *datasource.DataSource) {
				nds.OrganizationID = org
			})

			// another organization's data source must stay out of the listing.
			prepDataSources(t, db, 1, nil)

			return tcase{
				OrganizationID: org,
				Check: func(t *testing.T, _ *DB, res []datasource.DataSource) {
					require.Len(t, res, len(sources))

					for i, ds := range sources {
						testutil.AssertFilterEqual(t, *ds, res[i])
					}
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

			res, err := db.FetchDataSources(ctx, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			c.Check(t, db, res)
		})
	}
}

func Test_agent_markCredentialsUndecryptable(t *testing.T) {
	type tcase struct {
		DataSource *datasource.DataSource
		Err        error
		// Status is what the row stores afterwards.
		Status processor.ConnectionStatus
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Status already recorded": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			// the row still holds its old status, so a write would show.
			marked := *ds
			marked.Status = processor.ConnectionStatusInvalidEncryptionKey

			return tcase{
				DataSource: &marked,
				Status:     processor.ConnectionStatusSuccess,
			}
		},
		"Error returned by UpdateDataSourceStatus": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]
			rejectInvalidEncryptionKeyStatus(t, db)

			return tcase{
				DataSource: ds,
				Err:        assert.AnError,
			}
		},
		"Successful mark": func(t *testing.T, db *DB) tcase {
			ds := prepDataSources(t, db, 1, nil)[0]

			return tcase{
				DataSource: ds,
				Status:     processor.ConnectionStatusInvalidEncryptionKey,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.markCredentialsUndecryptable(context.Background(), c.DataSource)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, processor.ConnectionStatusInvalidEncryptionKey, c.DataSource.Status)
			assert.Equal(t, c.Status, fetchDataSourceStatus(t, db, c.DataSource.ID))
		})
	}
}
