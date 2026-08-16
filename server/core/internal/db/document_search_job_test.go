package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentSearchJobs(t *testing.T, db *DB, count int, fn func(int, *search.DocumentSearchJob)) []search.DocumentSearchJob {
	t.Helper()

	res := make([]search.DocumentSearchJob, count)

	for i := range count {
		job := search.DocumentSearchJob{
			BlockDiff: search.BlocksDifference{
				Updated: []search.Block{
					{
						ID:             "block-" + strconv.Itoa(i),
						OrganizationID: "org-" + strconv.Itoa(i),
						DocumentID:     xid.New(),
						Type:           "paragraph",
						Text:           "Updated block " + strconv.Itoa(i),
					},
				},
			},
		}

		if fn != nil {
			fn(i, &job)
		}

		// the id column is a serial; let the database assign it so
		// the sequence stays usable for production inserts.
		q, args := db.builder.Insert("document_search_jobs").
			SetMap(map[string]any{
				"block_diff": job.BlockDiff,
			}).
			Suffix("RETURNING id").
			MustSql()

		err := db.sql.Get(&job.ID, q, args...)
		require.NoError(t, err)

		res[i] = job
	}

	return res
}

func Test_agent_InsertDocumentSearchJob(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BlockDiff        search.BlocksDifference
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				Err:              assert.AnError,
			}
		},
		"Successful insert": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				BlockDiff: search.BlocksDifference{
					Added: []search.Block{
						{
							ID:             "block-added",
							OrganizationID: "org-added",
							DocumentID:     xid.New(),
							Type:           "heading",
							Text:           "Added block",
						},
					},
					Removed: []search.Block{
						{
							ID:             "block-removed",
							OrganizationID: "org-removed",
							DocumentID:     xid.New(),
							Type:           "paragraph",
							Text:           "Removed block",
						},
					},
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

			err := db.InsertDocumentSearchJob(ctx, c.BlockDiff)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var blockDiff search.BlocksDifference

			q, args := db.builder.Select("block_diff").
				From("document_search_jobs").
				MustSql()

			err = db.sql.Get(&blockDiff, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.BlockDiff, blockDiff)
		})
	}
}

func Test_agent_FetchDocumentSearchJobs(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentSearchJobs(ctx, 10)
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no jobs
	res, err = db.FetchDocumentSearchJobs(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - limited batch in id order
	jobs := prepDocumentSearchJobs(t, db, 3, nil)

	res, err = db.FetchDocumentSearchJobs(context.Background(), 2)
	assert.NoError(t, err)
	assert.Equal(t, jobs[:2], res)
}

func Test_agent_DeleteDocumentSearchJob(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		ID               int64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			jobs := prepDocumentSearchJobs(t, db, 1, nil)

			return tcase{
				CancelledContext: true,
				ID:               jobs[0].ID,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			jobs := prepDocumentSearchJobs(t, db, 1, nil)

			return tcase{
				ID: jobs[0].ID,
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

			err := db.DeleteDocumentSearchJob(ctx, c.ID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var id int64

			q, args := db.builder.Select("id").
				From("document_search_jobs").
				Where(sq.Eq{
					"id": c.ID,
				}).MustSql()

			err = db.sql.Get(&id, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
		})
	}
}
