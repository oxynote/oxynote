package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
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
			Branch: document.Branch{
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
			},
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

			err := db.upsertDocumentBranch(context.Background(), c.Document)
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

func Test_agent_ForkDocumentBranch(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := db.ForkDocumentBranch(ctx, xid.New(), "org-id", document.DefaultBranch, "feature-x", "user-1")
	require.Error(t, err)

	// success - source branch does not exist, nothing inserted
	doc := prepDocuments(t, db, 1, nil)[0]

	err = db.ForkDocumentBranch(context.Background(), doc.ID, doc.OrganizationID, "non-existent-branch", "feature-x", "user-1")
	require.NoError(t, err)

	count, err := db.CountDocumentBranches(context.Background(), doc.ID, doc.OrganizationID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// success - fork of the default branch
	users := prepUsers(t, db, 1)

	err = db.ForkDocumentBranch(context.Background(), doc.ID, doc.OrganizationID, document.DefaultBranch, "feature-x", users[0])
	require.NoError(t, err)

	var forked document.Document

	q, args := db.builder.Select(
		`id AS "branch_id"`,
		`branch_name AS "branch_name"`,
		`document_name AS "document_name"`,
		`icon AS "icon"`,
		`content AS "content"`,
		`raw_content AS "raw_content"`,
		`protected AS "protected"`,
		`"default" AS "default"`,
		`fk_created_by AS "fk_created_by"`,
		`fk_last_updated_by AS "fk_last_updated_by"`,
	).From("document_branches").
		Where(sq.Eq{
			"fk_document_id": doc.ID,
			"branch_name":    "feature-x",
		}).MustSql()

	err = db.sql.Get(&forked, q, args...)
	require.NoError(t, err)

	assert.Equal(t, doc.DocumentName, forked.DocumentName)
	assert.Equal(t, doc.Icon, forked.Icon)
	assert.Equal(t, doc.Content, forked.Content)
	assert.Equal(t, doc.RawContent, forked.RawContent)
	assert.False(t, forked.Default)
	assert.Equal(t, null.StringFrom(users[0]), forked.CreatedBy)
	assert.Equal(t, null.StringFrom(users[0]), forked.LastUpdatedBy)

	// success - target branch already exists, nothing overwritten
	err = db.ForkDocumentBranch(context.Background(), doc.ID, doc.OrganizationID, document.DefaultBranch, "feature-x", "someone-else")
	require.NoError(t, err)

	var createdBy null.String

	q, args = db.builder.Select("fk_created_by").
		From("document_branches").
		Where(sq.Eq{
			"fk_document_id": doc.ID,
			"branch_name":    "feature-x",
		}).MustSql()

	err = db.sql.Get(&createdBy, q, args...)
	require.NoError(t, err)
	assert.Equal(t, null.StringFrom(users[0]), createdBy)
}

func Test_agent_FetchMainBranchContent(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchMainBranchContent(context.Background(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Zero(t, res)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]

	res, err = db.FetchMainBranchContent(context.Background(), doc.ID, doc.OrganizationID)
	assert.NoError(t, err)
	assert.Equal(t, document.Content{
		OrganizationID: doc.OrganizationID,
		DocumentID:     doc.ID,
		DocumentName:   doc.DocumentName,
		Content:        doc.Content,
	}, res)
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

			// an edit leaves a changelog entry behind, which used to pin
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

// prepChangelogs writes count snapshots of the document's branch, one per
// aggregation bucket so each lands as its own entry, oldest first.
func prepChangelogs(t *testing.T, db *DB, doc *document.Document, count int) []document.Changelog {
	t.Helper()

	res := make([]document.Changelog, count)
	base := timeutil.Now().Truncate(time.Second)

	for i := range count {
		doc.UpdatedAt = base.Add(-time.Duration(count-i) * time.Hour)
		clog := doc.Changelog()

		require.NoError(t, db.insertDocumentBranchChangelog(
			context.Background(),
			doc.ID,
			doc.BranchID,
			clog,
		))

		res[i] = clog
	}

	return res
}

// countChangelogs returns how many snapshots the branch currently has.
func countChangelogs(t *testing.T, db *DB, branchID xid.ID) int {
	t.Helper()

	q, args := db.builder.Select("COUNT(*)").
		From("document_branch_changelogs").
		Where(sq.Eq{"fk_branch_id": branchID}).
		MustSql()

	var count int

	require.NoError(t, db.sql.Get(&count, q, args...))

	return count
}

func Test_agent_trimDocumentBranchChangelogs(t *testing.T) {
	cc := map[string]struct {
		Max       uint64
		Remaining int
	}{
		"Zero keeps every snapshot": {
			Max:       0,
			Remaining: 4,
		},
		"Only the newest snapshots survive": {
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

			clogs := prepChangelogs(t, db, doc, 4)

			db.opts.MaxDocumentChangelogs = c.Max

			require.NoError(t, db.trimDocumentBranchChangelogs(context.Background(), doc.BranchID))
			assert.Equal(t, c.Remaining, countChangelogs(t, db, doc.BranchID))

			if c.Max == 0 || c.Remaining == len(clogs) {
				return
			}

			// the survivors are the newest ones.
			var ids []string

			q, args := db.builder.Select("id").
				From("document_branch_changelogs").
				Where(sq.Eq{"fk_branch_id": doc.BranchID}).
				MustSql()

			require.NoError(t, db.sql.Select(&ids, q, args...))
			assert.ElementsMatch(t, []string{clogs[2].ID, clogs[3].ID}, ids)
		})
	}
}

func Test_agent_DeleteExpiredDocumentBranchChangelogs(t *testing.T) {
	t.Parallel()

	db := prepTempDB(t)
	doc := prepDocuments(t, db, 1, nil)[0]

	clogs := prepChangelogs(t, db, doc, 4)
	require.Equal(t, 4, countChangelogs(t, db, doc.BranchID))

	// nothing is old enough yet.
	require.NoError(t, db.DeleteExpiredDocumentBranchChangelogs(
		context.Background(),
		clogs[0].CreatedAt.Add(-time.Second),
	))
	assert.Equal(t, 4, countChangelogs(t, db, doc.BranchID))

	// everything created before the newest snapshot goes.
	require.NoError(t, db.DeleteExpiredDocumentBranchChangelogs(
		context.Background(),
		clogs[3].CreatedAt,
	))
	assert.Equal(t, 1, countChangelogs(t, db, doc.BranchID))
}

func Test_agent_insertDocumentBranchChangelog(t *testing.T) {
	type tcase struct {
		DocumentID xid.ID
		BranchID   xid.ID
		Changelog  document.Changelog
		Trimmed    int
		Err        error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			clog := doc.Changelog()

			return tcase{
				DocumentID: doc.ID,
				BranchID:   xid.New(),
				Changelog:  clog,
				Err:        assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				DocumentID: doc.ID,
				BranchID:   doc.BranchID,
				Changelog:  doc.Changelog(),
			}
		},
		"Insert trims the branch down to the retained snapshots": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			prepChangelogs(t, db, doc, 3)

			db.opts.MaxDocumentChangelogs = 2
			doc.UpdatedAt = timeutil.Now().Truncate(time.Second)

			return tcase{
				DocumentID: doc.ID,
				BranchID:   doc.BranchID,
				Changelog:  doc.Changelog(),
				Trimmed:    2,
			}
		},
		"Successful update of an existing entry": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			require.NoError(t, db.insertDocumentBranchChangelog(
				context.Background(),
				doc.ID,
				doc.BranchID,
				doc.Changelog(),
			))

			doc.RawContent = []byte("updated raw content")

			return tcase{
				DocumentID: doc.ID,
				BranchID:   doc.BranchID,
				Changelog:  doc.Changelog(),
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.insertDocumentBranchChangelog(context.Background(), c.DocumentID, c.BranchID, c.Changelog)
			testutil.RequireEqualError(t, c.Err, err)

			if c.Trimmed != 0 {
				assert.Equal(t, c.Trimmed, countChangelogs(t, db, c.BranchID))
			}

			if err != nil {
				return
			}

			var clog document.Changelog

			q, args := db.builder.Select(
				`id AS "id"`,
				`fk_document_id AS "fk_document_id"`,
				`content AS "content"`,
				`raw_content AS "raw_content"`,
				`created_at AS "created_at"`,
			).From("document_branch_changelogs").
				Where(sq.Eq{
					"id": c.Changelog.ID,
				}).MustSql()

			err = db.sql.Get(&clog, q, args...)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Changelog, clog)
		})
	}
}
