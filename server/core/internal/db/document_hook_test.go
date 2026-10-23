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
			ID:            xid.New(),
			CrossBranchID: xid.New(),
			Type:          hook.TypeURLWatcher,
			BlockID:       null.StringFrom("block-" + xid.New().String()),
			// JSONB round-trips in canonical form (space after the
			// colon), so the fixture uses that form for comparisons.
			Settings:  processor.Settings(`{"url": "http://watched.test"}`),
			State:     null.ValueFrom(processor.State(`{"watcherId":"w1"}`)),
			Status:    processor.StatusActive,
			Score:     decimal.NewFromInt(50),
			CreatedAt: now,
		}

		if fn != nil {
			fn(i, &hk)
		}

		if !hk.DocumentID.Valid {
			if branch == nil {
				branch = prepDocumentBranches(t, db, 1, nil)[0]
			}

			hk.DocumentID = null.ValueFrom(branch.ID)
			hk.OrganizationID = null.StringFrom(branch.OrganizationID)
			hk.BranchID = null.ValueFrom(branch.BranchID)
		}

		res[i] = hk

		q, args := db.builder.Insert("document_hooks").
			SetMap(map[string]any{
				"id":                 hk.ID,
				"cross_branch_id":    hk.CrossBranchID,
				"type":               hk.Type,
				"fk_document_id":     hk.DocumentID.V,
				"fk_organization_id": hk.OrganizationID,
				"fk_branch_id":       hk.BranchID,
				"block_id":           hk.BlockID,
				"settings":           hk.Settings,
				"state":              hk.State,
				"status":             hk.Status,
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
					CrossBranchID:  xid.New(),
					Type:           hook.TypeScheduledReminder,
					DocumentID:     null.ValueFrom(branch.ID),
					OrganizationID: null.StringFrom(branch.OrganizationID),
					BranchID:       null.ValueFrom(branch.BranchID),
					Settings:       processor.Settings(`{"schedule": "0 0 * * * *"}`),
					Status:         processor.StatusActive,
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

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID.String)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Hook, res)
		})
	}
}

func Test_agent_FetchDocumentHook(t *testing.T) {
	type tcase struct {
		ID             xid.ID
		OrganizationID string
		Result         *hook.Hook
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Hook not found": func(*testing.T, *DB) tcase {
			return tcase{
				ID:             xid.New(),
				OrganizationID: "non-existent-org-id",
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				ID:             hk.ID,
				OrganizationID: hk.OrganizationID.String,
				Result:         &hk,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchDocumentHook(context.Background(), c.ID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchPaginatedDocumentHooks(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OffsetID         xid.ID
		Limit            int64
		Result           []hook.Hook
		Err              error
		// Check inspects the result in place of Result.
		Check func(t *testing.T, res []hook.Hook)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				OffsetID:         xid.NilID(),
				Limit:            10,
				Err:              assert.AnError,
			}
		},
		"No hooks": func(*testing.T, *DB) tcase {
			return tcase{
				OffsetID: xid.NilID(),
				Limit:    10,
				Result:   []hook.Hook{},
			}
		},
		"First page": func(t *testing.T, db *DB) tcase {
			hooks := prepDocumentHooks(t, db, 3, nil)

			return tcase{
				OffsetID: xid.NilID(),
				Limit:    2,
				Result:   hooks[:2],
			}
		},
		"Second page": func(t *testing.T, db *DB) tcase {
			hooks := prepDocumentHooks(t, db, 3, nil)

			return tcase{
				OffsetID: hooks[1].ID,
				Limit:    2,
				Result:   hooks[2:],
			}
		},
		// the organization is deleted by the auth service, outside of core,
		// and the rows it leaves behind are the only trace of the external
		// watchers: the sweep has to keep finding them.
		"Hooks outlive their organization": func(t *testing.T, db *DB) tcase {
			hooks := prepDocumentHooks(t, db, 3, nil)

			q, args := db.builder.Delete("organizations").
				Where(sq.Eq{"id": hooks[0].OrganizationID}).
				MustSql()

			_, err := db.sql.Exec(q, args...)
			require.NoError(t, err)

			return tcase{
				OffsetID: xid.NilID(),
				Limit:    10,
				Check: func(t *testing.T, res []hook.Hook) {
					require.Len(t, res, len(hooks))

					for _, hk := range res {
						assert.False(t, hk.OrganizationID.Valid)
						assert.True(t, hk.State.Valid, "the watcher stays addressable without its organization")
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

			res, err := db.FetchPaginatedDocumentHooks(ctx, c.OffsetID, c.Limit)
			testutil.AssertEqualError(t, c.Err, err)

			if c.Check != nil {
				c.Check(t, res)

				return
			}

			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
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
			hk.State = null.ValueFrom(processor.State(`{"watcherId":"w2"}`))
			hk.Status = processor.StatusUnreachableURL
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

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID.String)
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

			err := db.DeleteDocumentHook(ctx, c.Hook.ID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentHook(context.Background(), c.Hook.ID, c.Hook.OrganizationID.String)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_FetchDocumentHooksByBranchID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		Result           []hook.Hook
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				BranchID:         xid.New(),
				OrganizationID:   "org-id",
				Err:              assert.AnError,
			}
		},
		"No hooks": func(*testing.T, *DB) tcase {
			return tcase{
				BranchID:       xid.New(),
				OrganizationID: "non-existent-org-id",
				Result:         []hook.Hook{},
			}
		},
		"Soft deleted hooks are excluded": func(t *testing.T, db *DB) tcase {
			hooks := prepDocumentHooks(t, db, 2, func(i int, hk *hook.Hook) {
				if i == 1 {
					hk.SoftDeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
				}
			})

			return tcase{
				BranchID:       hooks[0].BranchID.V,
				OrganizationID: hooks[0].OrganizationID.String,
				Result:         hooks[:1],
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

			res, err := db.FetchDocumentHooksByBranchID(ctx, c.BranchID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_DetachDocumentHooksByBranchID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		Err              error
		// Check inspects the rows the detach left behind.
		Check func(t *testing.T, db *DB)
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				BranchID:         xid.New(),
				OrganizationID:   "org-id",
				Err:              assert.AnError,
			}
		},
		// the branch's hooks lose their branch and document, a hook of
		// another branch keeps both.
		"Successful detach": func(t *testing.T, db *DB) tcase {
			hooks := prepDocumentHooks(t, db, 2, nil)
			other := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				BranchID:       hooks[0].BranchID.V,
				OrganizationID: hooks[0].OrganizationID.String,
				Check: func(t *testing.T, db *DB) {
					type refs struct {
						BranchID   null.Value[xid.ID] `db:"fk_branch_id"`
						DocumentID null.Value[xid.ID] `db:"fk_document_id"`
					}

					var rows []refs

					q, args := db.builder.Select("fk_branch_id", "fk_document_id").
						From("document_hooks").
						Where(sq.Eq{
							"id": []xid.ID{hooks[0].ID, hooks[1].ID},
						}).
						MustSql()

					err := db.sql.Select(&rows, q, args...)
					require.NoError(t, err)
					require.Len(t, rows, 2)

					for _, r := range rows {
						assert.False(t, r.BranchID.Valid)
						assert.False(t, r.DocumentID.Valid)
					}

					var kept refs

					q, args = db.builder.Select("fk_branch_id", "fk_document_id").
						From("document_hooks").
						Where(sq.Eq{"id": other.ID}).
						MustSql()

					err = db.sql.Get(&kept, q, args...)
					require.NoError(t, err)
					assert.Equal(t, other.BranchID, kept.BranchID)
					assert.Equal(t, other.DocumentID, kept.DocumentID)
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

			err := db.DetachDocumentHooksByBranchID(ctx, c.BranchID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Check != nil {
				c.Check(t, db)
			}
		})
	}
}

func Test_agent_FetchDocumentHooksByOrganizationID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		Result           []xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			hk := prepDocumentHooks(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   hk.OrganizationID.String,
				Err:              assert.AnError,
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			hooks := prepDocumentHooks(t, db, 2, func(_ int, hk *hook.Hook) {
				hk.DocumentID = null.ValueFrom(branch.ID)
				hk.OrganizationID = null.StringFrom(branch.OrganizationID)
				hk.BranchID = null.ValueFrom(branch.BranchID)
			})

			other := prepDocumentBranches(t, db, 1, nil)[0]

			prepDocumentHooks(t, db, 1, func(_ int, hk *hook.Hook) {
				hk.DocumentID = null.ValueFrom(other.ID)
				hk.OrganizationID = null.StringFrom(other.OrganizationID)
				hk.BranchID = null.ValueFrom(other.BranchID)
			})

			return tcase{
				OrganizationID: branch.OrganizationID,
				Result:         []xid.ID{hooks[0].ID, hooks[1].ID},
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

			res, err := db.FetchDocumentHooksByOrganizationID(ctx, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			require.Len(t, res, len(c.Result))

			ids := make([]xid.ID, len(res))
			for i, hk := range res {
				ids[i] = hk.ID
			}

			assert.ElementsMatch(t, c.Result, ids)
		})
	}
}
