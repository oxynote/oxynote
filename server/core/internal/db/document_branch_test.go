package db

import (
	"context"
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/history"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentBranches(t *testing.T, db *DB, count int, fn func(int, *document.Document)) []*document.Document {
	t.Helper()

	res := make([]*document.Document, count)

	now := timeutil.Now().Truncate(time.Second)

	// branches missing a document after fn ran share one lazily
	// created parent, so a single call produces sibling branches.
	var parent *document.Document

	for i := range count {
		doc := &document.Document{
			BranchID:     xid.New(),
			BranchName:   "branch-" + strconv.Itoa(i),
			DocumentName: "Branch Document " + strconv.Itoa(i),
			Icon:         "icon-branch",
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{
						Type: "paragraph",
						Text: "Branch content " + strconv.Itoa(i),
					},
				},
			},
			RawContent: []byte("Branch content " + strconv.Itoa(i)),
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if fn != nil {
			fn(i, doc)
		}

		if doc.ID.IsZero() {
			if parent == nil {
				parent = prepDocuments(t, db, 1, nil)[0]
			}

			doc.ID = parent.ID
			doc.OrganizationID = parent.OrganizationID
		}

		res[i] = doc

		q, args := db.builder.Insert("document_branches").
			SetMap(map[string]any{
				"id":                 doc.BranchID,
				"fk_document_id":     doc.ID,
				"fk_organization_id": doc.OrganizationID,
				"branch_name":        doc.BranchName,
				"document_name":      doc.DocumentName,
				"icon":               doc.Icon,
				"content":            doc.Content,
				"raw_content":        doc.RawContent,
				"protected":          doc.Protected,
				`"default"`:          doc.Default,
				"created_at":         doc.CreatedAt,
				"fk_created_by":      doc.CreatedBy,
				"updated_at":         doc.UpdatedAt,
				"fk_last_updated_by": doc.LastUpdatedBy,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_insertDocumentBranch(t *testing.T) {
	type tcase struct {
		Document document.Document
		Original *document.Document
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate branch ID": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.BranchID = branch.BranchID
			doc.BranchName = "another-branch"

			return tcase{
				Document: *doc,
				Err:      assert.AnError,
			}
		},
		"Existing branch is left untouched": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			doc := *branch
			doc.BranchID = xid.New()
			doc.DocumentName = "Should Not Overwrite"

			return tcase{
				Document: doc,
				Original: branch,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.BranchID = xid.New()
			doc.BranchName = "feature-x"

			return tcase{
				Document: *doc,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				return db.insertDocumentBranch(context.Background(), tx, c.Document)
			})
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			exp := &c.Document

			if c.Original != nil {
				exp = c.Original
			}

			res, err := db.FetchDocumentByBranchID(context.Background(), exp.BranchID, exp.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, exp, res)
		})
	}
}

func Test_agent_upsertDocumentBranch(t *testing.T) {
	type tcase struct {
		Document document.Document
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent document": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Document: document.Document{
					ID:             xid.New(),
					OrganizationID: "non-existent-org-id",
					Branch: document.Branch{
						BranchID:   xid.New(),
						BranchName: "feature-x",
						CreatedAt:  timeutil.Now().Truncate(time.Second),
						UpdatedAt:  timeutil.Now().Truncate(time.Second),
					},
				},
				Err: assert.AnError,
			}
		},
		"Successful insert of a new branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.BranchID = xid.New()
			doc.BranchName = "feature-x"

			return tcase{
				Document: *doc,
			}
		},
		"Successful update of an existing branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.DocumentName = "Updated Name"
			doc.Icon = "updated-icon"
			doc.RawContent = []byte("updated content")
			doc.Protected = true
			doc.UpdatedAt = timeutil.Now().Truncate(time.Second).Add(time.Hour)

			return tcase{
				Document: *doc,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.upsertDocumentBranch(context.Background(), db.sql, c.Document)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentByBranchID(context.Background(), c.Document.BranchID, c.Document.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Document, res)
		})
	}
}

func Test_agent_InsertDocumentBranch(t *testing.T) {
	type tcase struct {
		Document document.Document
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate branch name": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			doc := *branch
			doc.BranchID = xid.New()

			return tcase{
				Document: doc,
				Err: errutil.New(
					http.StatusBadRequest,
					"document_branch.duplicate_name",
					"branch name is already in use",
				),
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.BranchID = xid.New()
			doc.BranchName = "feature-x"
			doc.Default = false

			return tcase{
				Document: *doc,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertDocumentBranch(context.Background(), c.Document)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentByBranchID(context.Background(), c.Document.BranchID, c.Document.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Document, res)
		})
	}
}

func Test_agent_FetchDocumentBranches(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentBranches(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no branches
	res, err = db.FetchDocumentBranches(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]
	branch := prepDocumentBranches(t, db, 1, func(_ int, ndoc *document.Document) {
		ndoc.ID = doc.ID
		ndoc.OrganizationID = doc.OrganizationID
	})[0]

	exp := []document.BranchSummary{
		{
			BranchID:     doc.BranchID,
			BranchName:   doc.BranchName,
			DocumentName: doc.DocumentName,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
			Default:      doc.Default,
			CreatedAt:    doc.CreatedAt,
			UpdatedAt:    doc.UpdatedAt,
		},
		{
			BranchID:     branch.BranchID,
			BranchName:   branch.BranchName,
			DocumentName: branch.DocumentName,
			Icon:         branch.Icon,
			Protected:    branch.Protected,
			Default:      branch.Default,
			CreatedAt:    branch.CreatedAt,
			UpdatedAt:    branch.UpdatedAt,
		},
	}

	res, err = db.FetchDocumentBranches(context.Background(), doc.ID, doc.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, exp, res)
}

func Test_agent_CountDocumentBranches(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count, err := db.CountDocumentBranches(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Zero(t, count)

	// success - no branches
	count, err = db.CountDocumentBranches(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Zero(t, count)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]
	prepDocumentBranches(t, db, 2, func(_ int, ndoc *document.Document) {
		ndoc.ID = doc.ID
		ndoc.OrganizationID = doc.OrganizationID
	})

	count, err = db.CountDocumentBranches(context.Background(), doc.ID, doc.OrganizationID)
	assert.NoError(t, err)
	assert.Equal(t, 3, count)
}

func Test_agent_DeleteDocumentBranchByID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		HookID           xid.ID
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				BranchID:         branch.BranchID,
				OrganizationID:   branch.OrganizationID,
				Err:              assert.AnError,
			}
		},
		// the hook survives its branch: it holds an external watcher that
		// only the hook manager can tear down, and the row is what points
		// at it.
		"Hooks outlive the branch": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			hk := prepDocumentHooks(t, db, 1, func(_ int, hk *hook.Hook) {
				hk.DocumentID = null.ValueFrom(branch.ID)
				hk.OrganizationID = null.StringFrom(branch.OrganizationID)
				hk.BranchID = null.ValueFrom(branch.BranchID)
			})[0]

			return tcase{
				BranchID:       branch.BranchID,
				OrganizationID: branch.OrganizationID,
				HookID:         hk.ID,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			return tcase{
				BranchID:       branch.BranchID,
				OrganizationID: branch.OrganizationID,
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

			err := db.DeleteDocumentBranchByID(ctx, c.BranchID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var id xid.ID

			q, args := db.builder.Select("id").
				From("document_branches").
				Where(sq.Eq{
					"id": c.BranchID,
				}).MustSql()

			err = db.sql.Get(&id, q, args...)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)

			if c.HookID.IsZero() {
				return
			}

			var hk hook.Hook

			q, args = db.selectDocumentHook(db.builder.Select()).
				Where(sq.Eq{"document_hooks.id": c.HookID}).
				MustSql()

			require.NoError(t, db.sql.Get(&hk, q, args...))
			assert.False(t, hk.BranchID.Valid)
			assert.True(t, hk.DocumentID.Valid, "the document itself is still there")
		})
	}
}

func Test_agent_UpdateDocumentBranchMetadata(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Document         document.Document
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Document:         *branch,
				Err:              assert.AnError,
			}
		},
		"Successful update of an edited branch": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]

			// an edit leaves a history entry behind, which used to pin
			// the branch name in place.
			require.NoError(t, db.UpdateDocument(context.Background(), *branch))

			branch.BranchName = "renamed-edited-branch"

			return tcase{
				Document: *branch,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			branch.BranchName = "renamed-branch"
			branch.Protected = true
			branch.UpdatedAt = branch.UpdatedAt.Add(time.Hour)
			branch.LastUpdatedBy = null.StringFrom(users[0])

			return tcase{
				Document: *branch,
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

			err := db.UpdateDocumentBranchMetadata(ctx, c.Document)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentByBranchID(context.Background(), c.Document.BranchID, c.Document.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Document, res)
		})
	}
}

func Test_agent_FetchDocumentByBranchID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentByBranchID(context.Background(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	branch := prepDocumentBranches(t, db, 1, nil)[0]

	res, err = db.FetchDocumentByBranchID(context.Background(), branch.BranchID, branch.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, branch, res)
}

func Test_agent_FetchDocumentBranchesUnsafe(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentBranchesUnsafe(ctx, xid.New())
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no branches
	res, err = db.FetchDocumentBranchesUnsafe(context.Background(), xid.New())
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - no organization scoping applied
	doc := prepDocuments(t, db, 1, nil)[0]

	exp := []document.BranchSummary{
		{
			BranchID:     doc.BranchID,
			BranchName:   doc.BranchName,
			DocumentName: doc.DocumentName,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
			Default:      doc.Default,
			CreatedAt:    doc.CreatedAt,
			UpdatedAt:    doc.UpdatedAt,
		},
	}

	res, err = db.FetchDocumentBranchesUnsafe(context.Background(), doc.ID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, exp, res)
}

func Test_agent_FetchDocumentUnsafeByBranchID(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentUnsafeByBranchID(context.Background(), xid.New())
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success - no organization scoping applied
	branch := prepDocumentBranches(t, db, 1, nil)[0]

	res, err = db.FetchDocumentUnsafeByBranchID(context.Background(), branch.BranchID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, branch, res)
}

func Test_agent_FetchDocumentBranchesAfter(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentBranchesAfter(ctx, xid.ID{}, 10)
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no branches
	res, err = db.FetchDocumentBranchesAfter(context.Background(), xid.ID{}, 10)
	require.NoError(t, err)
	assert.Empty(t, res)

	// success - every branch across organizations, in id order. Each
	// prepared document comes with its own default branch, so two documents
	// and three extra branches make five.
	branches := prepDocumentBranches(t, db, 3, func(i int, doc *document.Document) {
		if i == 2 {
			other := prepDocuments(t, db, 1, nil)[0]
			doc.ID = other.ID
			doc.OrganizationID = other.OrganizationID
		}
	})

	all, err := db.FetchDocumentBranchesAfter(context.Background(), xid.ID{}, 10)
	require.NoError(t, err)
	require.Len(t, all, 5)

	ids := make([]string, 0, len(all))

	for _, doc := range all {
		ids = append(ids, doc.BranchID.String())
	}

	assert.IsIncreasing(t, ids)

	for _, branch := range branches {
		assert.Contains(t, ids, branch.BranchID.String())
	}

	// success - a page, then the rest after the page's last id
	res, err = db.FetchDocumentBranchesAfter(context.Background(), xid.ID{}, 2)
	require.NoError(t, err)
	testutil.AssertFilterEqual(t, all[:2], res, null.Value[xid.ID]{})

	res, err = db.FetchDocumentBranchesAfter(context.Background(), all[1].BranchID, 10)
	require.NoError(t, err)
	testutil.AssertFilterEqual(t, all[2:], res, null.Value[xid.ID]{})
}

// prepHistoryEntries writes count ordinary entries of the document's branch,
// one per aggregation bucket so each lands as its own row, oldest first.
func prepHistoryEntries(t *testing.T, db *DB, doc *document.Document, count int) []history.Entry {
	t.Helper()

	res := make([]history.Entry, count)
	base := timeutil.Now().Truncate(time.Second)

	for i := range count {
		entry := history.NewEntry(*doc,
			base.Add(-time.Duration(count-i)*time.Hour),
			null.String{},
			history.Hooks{},
			false,
		)

		// an entry equal to the newest one would not be written.
		entry.DocumentName += " " + strconv.Itoa(i)

		insertHistoryEntry(t, db, entry)

		res[i] = entry
	}

	return res
}

// insertHistoryEntry writes the entry through
// insertDocumentBranchHistoryEntry in a transaction of its own.
func insertHistoryEntry(t *testing.T, db *DB, entry history.Entry) {
	t.Helper()

	require.NoError(t, sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
		_, err := db.insertDocumentBranchHistoryEntry(context.Background(), tx, entry)

		return err
	}))
}

// assertHistoryEntryEqual compares two entries, matching hook settings as
// JSON since jsonb normalises their spacing.
func assertHistoryEntryEqual(t *testing.T, exp, got history.Entry) {
	t.Helper()

	require.Len(t, got.Hooks, len(exp.Hooks))

	// the hook slices are cloned: cases share their definitions and the
	// subtests run in parallel.
	exp.Hooks = slices.Clone(exp.Hooks)
	got.Hooks = slices.Clone(got.Hooks)

	for i := range exp.Hooks {
		assert.JSONEq(t, string(exp.Hooks[i].Settings), string(got.Hooks[i].Settings))

		exp.Hooks[i].Settings = nil
		got.Hooks[i].Settings = nil
	}

	assert.Equal(t, exp, got)
}

// countHistoryEntries returns how many entries the branch currently has.
func countHistoryEntries(t *testing.T, db *DB, branchID xid.ID) int {
	t.Helper()

	q, args := db.builder.Select("COUNT(*)").
		From("document_branch_history_entries").
		Where(sq.Eq{"fk_branch_id": branchID}).
		MustSql()

	var count int

	require.NoError(t, db.sql.Get(&count, q, args...))

	return count
}

// fetchNewestHistoryEntry reads the branch's newest entry in full.
func fetchNewestHistoryEntry(t *testing.T, db *DB, branchID xid.ID) history.Entry {
	t.Helper()

	q, args := db.builder.Select(
		`id AS "id"`,
		`fk_document_id AS "fk_document_id"`,
		`fk_branch_id AS "fk_branch_id"`,
		`document_name AS "document_name"`,
		`icon AS "icon"`,
		`content AS "content"`,
		`hooks AS "hooks"`,
		`fk_last_updated_by AS "fk_last_updated_by"`,
		`boundary AS "boundary"`,
		`created_at AS "created_at"`,
		`updated_at AS "updated_at"`,
	).
		From("document_branch_history_entries").
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		MustSql()

	var entry history.Entry

	require.NoError(t, db.sql.Get(&entry, q, args...))

	return entry
}

func Test_agent_trimDocumentBranchHistoryEntries(t *testing.T) {
	cc := map[string]struct {
		Max       uint64
		Remaining int
	}{
		"Zero keeps every entry": {
			Max:       0,
			Remaining: 4,
		},
		"Only the newest entries survive": {
			Max:       2,
			Remaining: 2,
		},
		"Limit above the count changes nothing": {
			Max:       10,
			Remaining: 4,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			doc := prepDocuments(t, db, 1, nil)[0]

			entries := prepHistoryEntries(t, db, doc, 4)

			db.opts.MaxDocumentHistoryEntries = c.Max

			require.NoError(t, db.trimDocumentBranchHistoryEntries(context.Background(), db.sql, doc.BranchID))
			assert.Equal(t, c.Remaining, countHistoryEntries(t, db, doc.BranchID))

			if c.Max == 0 || c.Remaining == len(entries) {
				return
			}

			// the survivors are the newest ones.
			var ids []xid.ID

			q, args := db.builder.Select("id").
				From("document_branch_history_entries").
				Where(sq.Eq{"fk_branch_id": doc.BranchID}).
				MustSql()

			require.NoError(t, db.sql.Select(&ids, q, args...))
			assert.ElementsMatch(t, []xid.ID{entries[2].ID, entries[3].ID}, ids)
		})
	}
}

func Test_agent_DeleteExpiredDocumentBranchHistoryEntries(t *testing.T) {
	t.Parallel()

	db := prepTempDB(t)
	doc := prepDocuments(t, db, 1, nil)[0]

	entries := prepHistoryEntries(t, db, doc, 4)
	require.Equal(t, 4, countHistoryEntries(t, db, doc.BranchID))

	// nothing is old enough yet.
	require.NoError(t, db.DeleteExpiredDocumentBranchHistoryEntries(
		context.Background(),
		entries[0].CreatedAt.Add(-time.Second),
	))
	assert.Equal(t, 4, countHistoryEntries(t, db, doc.BranchID))

	// everything created before the newest entry goes.
	require.NoError(t, db.DeleteExpiredDocumentBranchHistoryEntries(
		context.Background(),
		entries[3].CreatedAt,
	))
	assert.Equal(t, 1, countHistoryEntries(t, db, doc.BranchID))

	// the newest entry stays even once it has expired.
	require.NoError(t, db.DeleteExpiredDocumentBranchHistoryEntries(
		context.Background(),
		entries[3].CreatedAt.Add(time.Hour),
	))
	assert.Equal(t, 1, countHistoryEntries(t, db, doc.BranchID))
	assert.Equal(t, entries[3].ID, fetchNewestHistoryEntry(t, db, doc.BranchID).ID)
}

func Test_agent_insertDocumentBranchHistoryEntry(t *testing.T) {
	type tcase struct {
		Entry history.Entry
		// Rows is how many entries the branch holds afterwards.
		Rows int
		// Newest is the entry the branch's newest row is expected to
		// equal afterwards.
		Newest history.Entry
		Err    error
	}

	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	hooks := history.Hooks{
		{
			Type:     hook.TypeURLWatcher,
			BlockID:  null.StringFrom("b1"),
			Settings: processor.Settings(`{"url":"https://example.com"}`),
		},
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.BranchID = xid.New()

			return tcase{
				Entry: history.NewEntry(*doc, base, null.String{}, hooks, false),
				Err:   assert.AnError,
			}
		},
		"First entry of a branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			entry := history.NewEntry(*doc, base.Add(5*time.Minute), null.StringFrom(user), hooks, false)

			return tcase{
				Entry:  entry,
				Rows:   1,
				Newest: entry,
			}
		},
		"Edit in the same bucket updates the entry in place": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, history.Hooks{}, false)
			insertHistoryEntry(t, db, first)

			doc.DocumentName = "Renamed"
			doc.Icon = "📕"
			doc.Content.Content[0].Text = "Edited."

			entry := history.NewEntry(*doc, base.Add(29*time.Minute), null.StringFrom(user), hooks, false)

			newest := entry
			newest.ID = first.ID
			newest.CreatedAt = first.CreatedAt

			return tcase{
				Entry:  entry,
				Rows:   1,
				Newest: newest,
			}
		},
		"System edit in the same bucket keeps the author": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.StringFrom(user), history.Hooks{}, false)
			insertHistoryEntry(t, db, first)

			doc.Content.Content[0].Text = "Edited by the system."

			entry := history.NewEntry(*doc, base.Add(10*time.Minute), null.String{}, history.Hooks{}, false)

			newest := entry
			newest.ID = first.ID
			newest.CreatedAt = first.CreatedAt
			newest.LastUpdatedBy = null.StringFrom(user)

			return tcase{
				Entry:  entry,
				Rows:   1,
				Newest: newest,
			}
		},
		"Edit in the next bucket inserts": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, history.Hooks{}, false)
			insertHistoryEntry(t, db, first)

			entry := history.NewEntry(*doc, base.Add(31*time.Minute), null.String{}, hooks, false)

			return tcase{
				Entry:  entry,
				Rows:   2,
				Newest: entry,
			}
		},
		"Entry equal to the newest one is skipped": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, hooks, true)
			insertHistoryEntry(t, db, first)

			entry := history.NewEntry(*doc, base.Add(2*time.Hour), null.StringFrom(user), hooks, false)

			return tcase{
				Entry:  entry,
				Rows:   1,
				Newest: first,
			}
		},
		"Boundary entry equal to the newest one inserts": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, hooks, false)
			insertHistoryEntry(t, db, first)

			entry := history.NewEntry(*doc, base.Add(6*time.Minute), null.String{}, hooks, true)

			return tcase{
				Entry:  entry,
				Rows:   2,
				Newest: entry,
			}
		},
		"Boundary entry in an occupied bucket inserts": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			first := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, history.Hooks{}, false)
			insertHistoryEntry(t, db, first)

			entry := history.NewEntry(*doc, base.Add(6*time.Minute), null.String{}, hooks, true)

			return tcase{
				Entry:  entry,
				Rows:   2,
				Newest: entry,
			}
		},
		"Edit after a boundary in the same bucket inserts": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			boundary := history.NewEntry(*doc, base.Add(5*time.Minute), null.String{}, history.Hooks{}, true)
			insertHistoryEntry(t, db, boundary)

			entry := history.NewEntry(*doc, base.Add(6*time.Minute), null.String{}, hooks, false)

			return tcase{
				Entry:  entry,
				Rows:   2,
				Newest: entry,
			}
		},
		"Insert trims the branch down to the retained entries": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			prepHistoryEntries(t, db, doc, 3)

			db.opts.MaxDocumentHistoryEntries = 2

			entry := history.NewEntry(*doc, timeutil.Now().Truncate(time.Second), null.String{}, hooks, false)

			return tcase{
				Entry:  entry,
				Rows:   2,
				Newest: entry,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			var id xid.ID

			err := sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				var err error

				id, err = db.insertDocumentBranchHistoryEntry(context.Background(), tx, c.Entry)

				return err
			})
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Rows, countHistoryEntries(t, db, c.Entry.BranchID))

			newest := fetchNewestHistoryEntry(t, db, c.Entry.BranchID)
			assertHistoryEntryEqual(t, c.Newest, newest)

			// the id returned is the row holding the entry, which is the
			// newest.
			assert.Equal(t, newest.ID, id)
		})
	}
}

// prepHistoryBranch returns a branch whose content holds block "b1" and
// the hooks fn describes on it, fetched back so they match what a record
// reads.
func prepHistoryBranch(t *testing.T, db *DB, count int, fn func(int, *hook.Hook)) (*document.Document, []hook.Hook) {
	t.Helper()

	branch := prepDocumentBranches(t, db, 1, func(_ int, doc *document.Document) {
		doc.Content.Content[0].Attrs = document.Attributes{document.AttrUID: "b1"}
	})[0]

	hooks := prepDocumentHooks(t, db, count, func(i int, hk *hook.Hook) {
		hk.DocumentID = null.ValueFrom(branch.ID)
		hk.OrganizationID = null.StringFrom(branch.OrganizationID)
		hk.BranchID = null.ValueFrom(branch.BranchID)
		hk.BlockID = null.StringFrom("b1")
		hk.CreatedAt = hk.CreatedAt.Add(time.Duration(i) * time.Second)

		if fn != nil {
			fn(i, hk)
		}
	})

	doc, err := db.FetchDocumentUnsafeByBranchID(context.Background(), branch.BranchID)
	require.NoError(t, err)

	return doc, hooks
}

func Test_agent_RecordDocumentBranchHistoryEntry(t *testing.T) {
	type tcase struct {
		// Tx, when set, is the transaction the record runs in. It is
		// committed afterwards.
		Tx             *Tx
		BranchID       xid.ID
		OrganizationID string
		By             null.String
		Boundary       bool
		// Rows is how many entries the branch holds afterwards.
		Rows int
		// Newest is the entry the branch's newest row is expected to
		// equal afterwards, apart from its id and times.
		Newest history.Entry
		Err    error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Branch of another organization": func(t *testing.T, db *DB) tcase {
			doc, _ := prepHistoryBranch(t, db, 1, nil)

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: "other",
				Err:            sql.ErrNoRows,
			}
		},
		"Ordinary entry lists the hooks its content holds": func(t *testing.T, db *DB) tcase {
			doc, hooks := prepHistoryBranch(t, db, 4, func(i int, hk *hook.Hook) {
				switch i {
				case 1:
					// the block is back; the sweep lifts the mark later.
					hk.SoftDeletedAt = null.TimeFrom(timeutil.Now())
				case 2:
					// the block is gone; the sweep marks it later.
					hk.BlockID = null.StringFrom("removed")
				case 3:
					hk.BlockID = null.String{}
				}
			})
			user := prepUsers(t, db, 1)[0]

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				By:             null.StringFrom(user),
				Rows:           1,
				Newest: history.NewEntry(
					*doc,
					time.Time{},
					null.StringFrom(user),
					history.NewHooks(doc.Content, []hook.Hook{hooks[0], hooks[1], hooks[3]}),
					false,
				),
			}
		},
		"Boundary entry adds its own row": func(t *testing.T, db *DB) tcase {
			doc, hooks := prepHistoryBranch(t, db, 1, nil)

			insertHistoryEntry(t, db, history.NewEntry(*doc, timeutil.Now(), null.String{}, history.Hooks{}, false))

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Boundary:       true,
				Rows:           2,
				Newest:         history.NewEntry(*doc, time.Time{}, null.String{}, history.NewHooks(doc.Content, hooks), true),
			}
		},
		// a persist records inside the transaction that wrote the branch,
		// so the entry has to hold that write.
		"Inside a transaction the entry holds its write": func(t *testing.T, db *DB) tcase {
			doc, hooks := prepHistoryBranch(t, db, 1, nil)

			var tx *Tx

			require.NoError(t, db.BeginTx(context.Background(), &tx))

			doc.Content.Content[0].Text = "Written in the transaction."

			require.NoError(t, tx.UpdateDocument(context.Background(), *doc))

			return tcase{
				Tx:             tx,
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Rows:           1,
				Newest:         history.NewEntry(*doc, time.Time{}, null.String{}, history.NewHooks(doc.Content, hooks), false),
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			record := db.RecordDocumentBranchHistoryEntry

			if c.Tx != nil {
				defer c.Tx.Rollback() //nolint:errcheck // error provides no meaningful info

				record = c.Tx.RecordDocumentBranchHistoryEntry
			}

			before := timeutil.Now().Truncate(time.Microsecond)

			id, err := record(context.Background(), c.BranchID, c.OrganizationID, c.By, c.Boundary)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			if c.Tx != nil {
				require.NoError(t, c.Tx.Commit())
			}

			assert.Equal(t, c.Rows, countHistoryEntries(t, db, c.BranchID))

			newest := fetchNewestHistoryEntry(t, db, c.BranchID)
			assert.Equal(t, newest.ID, id)
			assert.False(t, newest.CreatedAt.Before(before))

			exp := c.Newest
			exp.ID = newest.ID
			exp.CreatedAt = newest.CreatedAt
			exp.UpdatedAt = newest.UpdatedAt
			assertHistoryEntryEqual(t, exp, newest)
		})
	}
}

func Test_agent_fetchNewestDocumentBranchHistoryEntry(t *testing.T) {
	t.Parallel()

	db := prepTempDB(t)
	doc := prepDocuments(t, db, 1, nil)[0]

	// no entries
	res, err := db.fetchNewestDocumentBranchHistoryEntry(
		context.Background(),
		db.sql,
		history.NewEntry(*doc, timeutil.Now(), null.String{}, history.Hooks{}, false),
	)
	require.NoError(t, err)
	assert.Nil(t, res)

	entries := prepHistoryEntries(t, db, doc, 3)

	// the newest of several, recording the same
	res, err = db.fetchNewestDocumentBranchHistoryEntry(context.Background(), db.sql, entries[2])
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, &history.Head{
		ID:        entries[2].ID,
		CreatedAt: entries[2].CreatedAt,
		Same:      true,
	}, res)

	// recording something else
	res, err = db.fetchNewestDocumentBranchHistoryEntry(context.Background(), db.sql, entries[0])
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, entries[2].ID, res.ID)
	assert.False(t, res.Same)
}
