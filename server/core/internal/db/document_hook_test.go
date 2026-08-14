package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentHooks(t *testing.T, db *DB, count int, fn func(int, *hook.Hook)) []hook.Hook {
	t.Helper()

	res := make([]hook.Hook, count)

	now := timeutil.Now().Truncate(time.Second)

	// hooks missing a document after fn ran share one lazily created
	// branch, so a single call produces hooks of the same document.
	var branch *document.Document

	for i := range count {
		hk := hook.Hook{
			ID:      xid.New(),
			Type:    hook.TypeURLWatcher,
			BlockID: null.StringFrom("block-" + xid.New().String()),
			// JSONB round-trips in canonical form (space after the
			// colon), so the fixture uses that form for comparisons.
			Settings:  processor.Settings(`{"url": "http://watched.test"}`),
			State:     processor.State(`{"status":"ok"}`),
			Score:     decimal.NewFromInt(50),
			CreatedAt: now,
		}

		if fn != nil {
			fn(i, &hk)
		}

		if hk.DocumentID.IsZero() {
			if branch == nil {
				branch = prepDocumentBranches(t, db, 1, nil)[0]
			}

			hk.DocumentID = branch.ID
			hk.OrganizationID = branch.OrganizationID
			hk.BranchID = null.ValueFrom(branch.BranchID)
		}

		res[i] = hk

		q, args := db.builder.Insert("document_hooks").
			SetMap(map[string]any{
				"id":                 hk.ID,
				"type":               hk.Type,
				"fk_document_id":     hk.DocumentID,
				"fk_organization_id": hk.OrganizationID,
				"fk_branch_id":       hk.BranchID,
				"block_id":           hk.BlockID,
				"settings":           hk.Settings,
				"state":              hk.State,
				"score":              hk.Score,
				"created_at":         hk.CreatedAt,
				"updated_at":         hk.UpdatedAt,
				"soft_deleted_at":    hk.SoftDeletedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDocumentHook(t *testing.T) {
	type tcase struct {
		Hook hook.Hook
		Err  error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate hook ID": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				Hook: hk,
				Err:  assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			return tcase{
				Hook: hook.Hook{
					ID:             xid.New(),
					Type:           hook.TypeScheduledReminder,
					DocumentID:     branch.ID,
					OrganizationID: branch.OrganizationID,
					BranchID:       null.ValueFrom(branch.BranchID),
					Settings:       processor.Settings(`{"schedule": "0 0 * * * *"}`),
					State:          processor.State(`{}`),
					Score:          decimal.NewFromInt(100),
					CreatedAt:      timeutil.Now().Truncate(time.Second),
					UpdatedAt:      null.TimeFrom(timeutil.Now().Truncate(time.Second)),
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertDocumentHook(context.Background(), c.Hook)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Hook, res)
		})
	}
}

func Test_agent_FetchDocumentHook(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentHook(context.Background(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	hk := prepDocumentHooks(t, db, 1, nil)[0]

	res, err = db.FetchDocumentHook(context.Background(), hk.ID, hk.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &hk, res)
}

func Test_agent_FetchDocumentHooksByDocumentID(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentHooksByDocumentID(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no hooks
	res, err = db.FetchDocumentHooksByDocumentID(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success
	hooks := prepDocumentHooks(t, db, 2, nil)

	res, err = db.FetchDocumentHooksByDocumentID(context.Background(), hooks[0].DocumentID, hooks[0].OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, hooks, res)
}

func Test_agent_FetchPaginatedDocumentHooks(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchPaginatedDocumentHooks(ctx, xid.NilID(), 10)
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no hooks
	res, err = db.FetchPaginatedDocumentHooks(context.Background(), xid.NilID(), 10)
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - first page
	hooks := prepDocumentHooks(t, db, 3, nil)

	res, err = db.FetchPaginatedDocumentHooks(context.Background(), xid.NilID(), 2)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, hooks[:2], res)

	// success - second page
	res, err = db.FetchPaginatedDocumentHooks(context.Background(), hooks[1].ID, 2)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, hooks[2:], res)
}

func Test_agent_UpdateDocumentHook(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Hook             hook.Hook
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Hook:             hk,
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]
			hk.Settings = processor.Settings(`{"url": "http://updated.test"}`)
			hk.State = processor.State(`{"status":"failing"}`)
			hk.Score = decimal.NewFromInt(10)
			hk.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			hk.SoftDeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))

			return tcase{
				Hook: hk,
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

			err := db.UpdateDocumentHook(ctx, c.Hook)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Hook, res)
		})
	}
}

func Test_agent_DeleteDocumentHook(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Hook             hook.Hook
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Hook:             hk,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				Hook: hk,
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

			err := db.DeleteDocumentHook(ctx, c.Hook.ID, c.Hook.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_FetchDocumentHooksByBranchID(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentHooksByBranchID(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no hooks
	res, err = db.FetchDocumentHooksByBranchID(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - soft-deleted hooks are excluded
	hooks := prepDocumentHooks(t, db, 2, func(i int, hk *hook.Hook) {
		if i == 1 {
			hk.SoftDeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
		}
	})

	res, err = db.FetchDocumentHooksByBranchID(context.Background(), hooks[0].BranchID.V, hooks[0].OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, hooks[:1], res)
}

func Test_agent_SoftDeleteDocumentHooksByBranchID(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := db.SoftDeleteDocumentHooksByBranchID(ctx, xid.New(), "org-id")
	require.Error(t, err)

	// success - active hooks are stamped, already deleted ones keep
	// their original timestamp
	deletedAt := timeutil.Now().Truncate(time.Second).Add(-time.Hour)

	hooks := prepDocumentHooks(t, db, 2, func(i int, hk *hook.Hook) {
		if i == 1 {
			hk.SoftDeletedAt = null.TimeFrom(deletedAt)
		}
	})

	err = db.SoftDeleteDocumentHooksByBranchID(context.Background(), hooks[0].BranchID.V, hooks[0].OrganizationID)
	require.NoError(t, err)

	var stamps []null.Time

	q, args := db.builder.Select("soft_deleted_at").
		From("document_hooks").
		Where(sq.Eq{
			"id": []xid.ID{hooks[0].ID, hooks[1].ID},
		}).
		OrderBy("id ASC").
		MustSql()

	err = db.sql.Select(&stamps, q, args...)
	require.NoError(t, err)
	require.Len(t, stamps, 2)

	require.True(t, stamps[0].Valid)
	assert.WithinDuration(t, timeutil.Now(), stamps[0].Time, time.Minute)
	assert.True(t, stamps[1].Valid)
	assert.WithinDuration(t, deletedAt, stamps[1].Time, time.Second)
}
