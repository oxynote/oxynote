package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocuments(t *testing.T, db *DB, count int, fn func(int, *document.Document)) []*document.Document {
	t.Helper()

	res := make([]*document.Document, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		doc := &document.Document{
			ID:           xid.New(),
			BranchID:     xid.New(),
			BranchName:   document.DefaultBranch,
			Default:      true,
			DocumentName: "Test Document " + strconv.Itoa(i),
			Icon:         "icon-test",
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{
						Type: "paragraph",
						Text: "This is a test document content.",
					},
				},
			},
			RawContent: []byte("This is a test document content."),
			CreatedAt:  now,
			UpdatedAt:  now,
		}

		if fn != nil {
			fn(i, doc)
		}

		if doc.OrganizationID == "" {
			doc.OrganizationID = prepOrganizations(t, db, 1)[0]
		}

		res[i] = doc

		// Insert into documents table (structural fields only).
		q, args := db.builder.Insert("documents").
			SetMap(map[string]any{
				"id":                 doc.ID,
				"sort_index":         i,
				"fk_organization_id": doc.OrganizationID,
				"fk_parent_id":       doc.ParentID,
				"created_at":         doc.CreatedAt,
				"fk_created_by":      doc.CreatedBy,
				"updated_at":         doc.UpdatedAt,
				"fk_last_updated_by": doc.LastUpdatedBy,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		// Insert into document_branches table.
		q, args = db.builder.Insert("document_branches").
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

		_, err = db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDocument(t *testing.T) {
	now := timeutil.Now().Truncate(time.Second)

	stubDocument := func(organizationID string) document.Document {
		return document.Document{
			ID:             xid.New(),
			OrganizationID: organizationID,
			BranchID:       xid.New(),
			BranchName:     document.DefaultBranch,
			DocumentName:   "Test Document 1",
			Icon:           "icon-test2",
			Content: document.RootBlock{
				Type: document.BlockNodeDoc,
				Content: []document.Block{
					{
						Type: "paragraph",
						Text: "This is a test document content.",
					},
				},
			},
			RawContent: []byte("This is a test document content."),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}

	type tcase struct {
		CancelledContext bool
		Document         document.Document
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				CancelledContext: true,
				Document:         stubDocument(org),
				Err:              assert.AnError,
			}
		},
		"Sort index race exhausts retries": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			parent := prepDocuments(t, db, 1, func(_ int, ndoc *document.Document) {
				ndoc.OrganizationID = org
			})[0]

			// a single child whose sort_index is 1 leaves a gap at 0:
			// every attempt counts one sibling, picks sort_index 1,
			// and collides again, so the retry loop runs dry.
			child := stubDocument(org)
			child.ParentID = null.ValueFrom(parent.ID)

			q, args := db.builder.Insert("documents").
				SetMap(map[string]any{
					"id":                 child.ID,
					"sort_index":         1,
					"fk_organization_id": org,
					"fk_parent_id":       child.ParentID,
					"created_at":         child.CreatedAt,
					"updated_at":         child.UpdatedAt,
				}).MustSql()

			_, err := db.sql.Exec(q, args...)
			require.NoError(t, err)

			doc := stubDocument(org)
			doc.ParentID = null.ValueFrom(parent.ID)

			return tcase{
				Document: doc,
				Err:      assert.AnError,
			}
		},
		"Duplicate key": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			doc := prepDocuments(t, db, 1, func(_ int, ndoc *document.Document) {
				ndoc.OrganizationID = org
			})[0]

			return tcase{
				Document: *doc,
				Err:      assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]

			return tcase{
				Document: stubDocument(org),
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

			err := db.InsertDocument(ctx, c.Document)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var doc document.Document

			q, args := db.selectDocumentWithBranch(db.builder.Select(), document.DefaultBranch).
				Where(sq.Eq{
					"documents.id": c.Document.ID,
				}).
				MustSql()

			err = db.sql.Get(&doc, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Document, doc)
		})
	}

	t.Run("Branch insert failure rolls the document back", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		existing := prepDocuments(t, db, 1, nil)[0]

		// reusing the branch id trips the branch primary key, after the
		// documents row has already been inserted.
		doc := *existing
		doc.ID = xid.New()

		err := db.InsertDocument(context.Background(), doc)
		require.Error(t, err)

		err = db.CheckDocumentExists(context.Background(), doc.ID, doc.OrganizationID)
		testutil.AssertEqualError(t, sql.ErrNoRows, err)
	})

	t.Run("Root sort indexes are scoped per organization", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		orgs := prepOrganizations(t, db, 2)

		now := timeutil.Now().Truncate(time.Second)

		insert := func(org string) document.Document {
			doc := document.Document{
				ID:             xid.New(),
				OrganizationID: org,
				BranchID:       xid.New(),
				BranchName:     document.DefaultBranch,
				CreatedAt:      now,
				UpdatedAt:      now,
			}

			require.NoError(t, db.InsertDocument(context.Background(), doc))

			return doc
		}

		first := insert(orgs[0])
		second := insert(orgs[0])

		// a second organization starts its own sequence rather than
		// colliding with, or counting, the first one's roots.
		other := insert(orgs[1])

		sortIndex := func(id xid.ID) int {
			var idx int

			q, args := db.builder.Select("sort_index").From("documents").
				Where(sq.Eq{"id": id}).MustSql()

			require.NoError(t, db.sql.Get(&idx, q, args...))

			return idx
		}

		assert.Equal(t, 0, sortIndex(first.ID))
		assert.Equal(t, 1, sortIndex(second.ID))
		assert.Equal(t, 0, sortIndex(other.ID))
	})
}

func Test_agent_CheckDocumentExists(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	err := db.CheckDocumentExists(context.Background(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]

	err = db.CheckDocumentExists(context.Background(), doc.ID, doc.OrganizationID)
	assert.NoError(t, err)
}

func Test_agent_CheckDocumentCycle(t *testing.T) {
	// error - failing query
	a, mock := prepMockDB(t)

	mock.ExpectQuery("WITH RECURSIVE").WillReturnError(assert.AnError)

	_, err := a.CheckDocumentCycle(context.Background(), xid.New(), xid.New(), "org-id")
	assert.Equal(t, assert.AnError, err)

	// success
	db := prepTempDB(t)
	org := prepOrganizations(t, db, 1)[0]

	prepChild := func(parent null.Value[xid.ID]) *document.Document {
		return prepDocuments(t, db, 1, func(_ int, d *document.Document) {
			d.OrganizationID = org
			d.ParentID = parent
		})[0]
	}

	// the two roots are created together so they get distinct sort indexes.
	roots := prepDocuments(t, db, 2, func(_ int, d *document.Document) {
		d.OrganizationID = org
	})
	root, other := roots[0], roots[1]

	child := prepChild(null.ValueFrom(root.ID))
	grandchild := prepChild(null.ValueFrom(child.ID))

	cc := map[string]struct {
		ID             xid.ID
		ParentID       xid.ID
		OrganizationID string
		Result         bool
	}{
		"Document adopted by itself": {
			ID:       root.ID,
			ParentID: root.ID,
			Result:   true,
		},
		"Document adopted by its child": {
			ID:       root.ID,
			ParentID: child.ID,
			Result:   true,
		},
		"Document adopted by a deeper descendant": {
			ID:       root.ID,
			ParentID: grandchild.ID,
			Result:   true,
		},
		"Document adopted by its own parent": {
			ID:       child.ID,
			ParentID: root.ID,
		},
		"Document adopted by an unrelated document": {
			ID:       root.ID,
			ParentID: other.ID,
		},
		"Parent does not exist": {
			ID:       root.ID,
			ParentID: xid.New(),
		},
		"Parent belongs to another organization": {
			ID:             root.ID,
			ParentID:       child.ID,
			OrganizationID: "other-org-id",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			organizationID := c.OrganizationID
			if organizationID == "" {
				organizationID = org
			}

			res, err := db.CheckDocumentCycle(context.Background(), c.ID, c.ParentID, organizationID)
			assert.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_FetchDocument(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocument(context.Background(), xid.New(), "non-existent-doc-id", document.DefaultBranch)
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	doc := prepDocuments(t, db, 1, nil)[0]

	res, err = db.FetchDocument(context.Background(), doc.ID, doc.OrganizationID, document.DefaultBranch)
	assert.NoError(t, err)
	assert.Equal(t, doc, res)
}

func Test_agent_UpdateDocumentTree(t *testing.T) {
	// error - failing update statement inside the transaction
	a, mock := prepMockDB(t)

	mock.ExpectBegin()
	mock.ExpectExec("SET CONSTRAINTS").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE documents").WillReturnError(assert.AnError)
	mock.ExpectRollback()

	err := a.UpdateDocumentTree(
		context.Background(),
		document.Summaries{{ID: xid.New()}},
		"org-id",
	)
	assert.Equal(t, assert.AnError, err)

	db := prepTempDB(t)

	org := prepOrganizations(t, db, 1)[0]
	docs := prepDocuments(t, db, 3, func(_ int, ndoc *document.Document) {
		ndoc.OrganizationID = org
	})

	exp1 := make([]document.Summary, 0, len(docs))

	// Root documents
	for i, doc := range docs {
		st := document.Summary{
			DocumentName: doc.DocumentName,
			ID:           doc.ID,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
		}

		// Children documents
		prepDocuments(t, db, i+1, func(_ int, sdoc *document.Document) {
			sdoc.OrganizationID = org
			sdoc.ParentID = null.ValueFrom(docs[i].ID)

			st.Children = append(st.Children, document.Summary{
				ID:           sdoc.ID,
				DocumentName: sdoc.DocumentName,
				Icon:         sdoc.Icon,
				Protected:    sdoc.Protected,
			})
		})

		exp1 = append(exp1, st)
	}

	// success swapping root elements
	exp1[0], exp1[1] = exp1[1], exp1[0] // swap root elements

	err = db.UpdateDocumentTree(context.Background(), exp1, org)
	assert.NoError(t, err)

	exp2 := exp1[0].Children

	// success swapping children elements
	exp2[0], exp2[1] = exp2[1], exp2[0] // swap children elements

	err = db.UpdateDocumentTree(context.Background(), exp2, org)
	assert.NoError(t, err)
}

func Test_agent_FetchDocumentTree(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentTree(context.Background(), "123")
	require.NoError(t, err)
	assert.Empty(t, res)

	org := prepOrganizations(t, db, 1)[0]
	docs := prepDocuments(t, db, 3, func(_ int, ndoc *document.Document) {
		ndoc.OrganizationID = org
	})

	exp1 := make(document.Summaries, 0, len(docs))

	// Root documents
	for i, doc := range docs {
		st := document.Summary{
			DocumentName: doc.DocumentName,
			ID:           doc.ID,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
		}

		// Children documents
		prepDocuments(t, db, i+1, func(_ int, sdoc *document.Document) {
			sdoc.OrganizationID = org
			sdoc.ParentID = null.ValueFrom(docs[i].ID)

			st.Children = append(st.Children, document.Summary{
				ID:           sdoc.ID,
				DocumentName: sdoc.DocumentName,
				Icon:         sdoc.Icon,
				Protected:    sdoc.Protected,
			})
		})

		// Grandchildren documents
		for i, sst := range st.Children {
			prepDocuments(t, db, 2, func(_ int, sdoc *document.Document) {
				sdoc.OrganizationID = org
				sdoc.ParentID = null.ValueFrom(sst.ID)

				sst.Children = append(sst.Children, document.Summary{
					ID:           sdoc.ID,
					DocumentName: sdoc.DocumentName,
					Icon:         sdoc.Icon,
					Protected:    sdoc.Protected,
				})
			})

			st.Children[i] = sst
		}

		exp1 = append(exp1, st)
	}

	// success
	res, err = db.FetchDocumentTree(context.Background(), org)
	assert.NoError(t, err)
	assert.Equal(t, exp1, res)
}

func Test_agent_fetchDocumentTree(t *testing.T) {
	// sort_index drives row order, so the levels arrive interleaved: root,
	// child, grandchild, second child, second grandchild. The second child
	// grows the root's children slice after the first grandchild was
	// already attached to the first child.
	t.Run("Grandchildren survive sibling appends", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		org := prepOrganizations(t, db, 1)[0]

		ids := make([]xid.ID, 5)
		for i := range ids {
			ids[i] = xid.New()
		}

		parents := []null.Value[xid.ID]{
			{},
			null.ValueFrom(ids[0]),
			null.ValueFrom(ids[1]),
			null.ValueFrom(ids[0]),
			null.ValueFrom(ids[1]),
		}

		prepDocuments(t, db, len(ids), func(i int, doc *document.Document) {
			doc.ID = ids[i]
			doc.OrganizationID = org
			doc.ParentID = parents[i]
		})

		res, err := db.FetchDocumentTree(context.Background(), org)
		require.NoError(t, err)

		require.Len(t, res, 1)
		assert.Equal(t, ids[0], res[0].ID)

		require.Len(t, res[0].Children, 2)
		assert.Equal(t, ids[1], res[0].Children[0].ID)
		assert.Equal(t, ids[3], res[0].Children[1].ID)

		require.Len(t, res[0].Children[0].Children, 2)
		assert.Equal(t, ids[2], res[0].Children[0].Children[0].ID)
		assert.Equal(t, ids[4], res[0].Children[0].Children[1].ID)
	})
}

func Test_agent_FetchDocumentTreeByDocumentParentID(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchDocumentTreeByDocumentParentID(ctx, null.Value[xid.ID]{}, "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no documents
	res, err = db.FetchDocumentTreeByDocumentParentID(context.Background(), null.Value[xid.ID]{}, "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	org := prepOrganizations(t, db, 1)[0]
	roots := prepDocuments(t, db, 2, func(_ int, ndoc *document.Document) {
		ndoc.OrganizationID = org
	})

	exp := make(document.Summaries, 0, len(roots))

	for _, doc := range roots {
		exp = append(exp, document.Summary{
			ID:           doc.ID,
			DocumentName: doc.DocumentName,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
		})
	}

	children := prepDocuments(t, db, 2, func(_ int, sdoc *document.Document) {
		sdoc.OrganizationID = org
		sdoc.ParentID = null.ValueFrom(roots[0].ID)
	})

	// success - root documents only, children excluded
	res, err = db.FetchDocumentTreeByDocumentParentID(context.Background(), null.Value[xid.ID]{}, org)
	assert.NoError(t, err)
	assert.Equal(t, exp, res)

	exp = exp[:0]

	for _, doc := range children {
		exp = append(exp, document.Summary{
			ID:           doc.ID,
			DocumentName: doc.DocumentName,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
		})
	}

	// success - direct children of the first root
	res, err = db.FetchDocumentTreeByDocumentParentID(context.Background(), null.ValueFrom(roots[0].ID), org)
	assert.NoError(t, err)
	assert.Equal(t, exp, res)
}

func Test_agent_UpdateDocument(t *testing.T) {
	t.Run("Error returned by the document update", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("UPDATE documents").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.UpdateDocument(context.Background(), document.Document{})
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Error returned by the branch upsert", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("UPDATE documents").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO document_branches").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.UpdateDocument(context.Background(), document.Document{})
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Changelogs are kept per branch", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		// two branches of one document, updated inside the same
		// aggregation window.
		branches := prepDocumentBranches(t, db, 2, nil)
		now := timeutil.Now().Truncate(time.Second)

		for _, b := range branches {
			b.UpdatedAt = now

			require.NoError(t, db.UpdateDocument(context.Background(), *b))
		}

		var logs uint64

		q, args := db.builder.Select("COUNT(*)").From("document_branch_changelogs").
			Where(sq.Eq{
				"fk_document_id": branches[0].ID,
			}).MustSql()

		require.NoError(t, db.sql.Get(&logs, q, args...))
		assert.Equal(t, uint64(2), logs)
	})

	type tcase struct {
		Document document.Document
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Successful update": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.DocumentName = "Updated Document Name"
			doc.UpdatedAt = timeutil.Now().Truncate(time.Second)
			doc.Icon = "updated-icon"

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

			for range 3 {
				err := db.UpdateDocument(context.Background(), c.Document)
				testutil.RequireEqualError(t, c.Err, err)

				if err != nil {
					return
				}
			}

			var doc document.Document

			q, args := db.selectDocumentWithBranch(db.builder.Select(), document.DefaultBranch).
				Where(sq.Eq{
					"documents.id": c.Document.ID,
				}).
				MustSql()

			err := db.sql.Get(&doc, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Document, doc)

			var logs uint64

			q, args = db.builder.Select("COUNT(*)").From("document_branch_changelogs").
				Where(sq.Eq{
					"fk_document_id": c.Document.ID,
				}).MustSql()

			err = db.sql.Get(&logs, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.Document, doc)
			assert.Equal(t, uint64(1), logs)
		})
	}
}

func Test_agent_UpdateDocumentParentID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		ID               xid.ID
		ParentID         null.Value[xid.ID]
		OrganizationID   string
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				ID:               doc.ID,
				OrganizationID:   doc.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"Successful update to a new parent": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			docs := prepDocuments(t, db, 2, func(_ int, ndoc *document.Document) {
				ndoc.OrganizationID = org
			})

			return tcase{
				ID:             docs[1].ID,
				ParentID:       null.ValueFrom(docs[0].ID),
				OrganizationID: org,
			}
		},
		"Successful update to a root document": func(t *testing.T, db *DB) tcase {
			org := prepOrganizations(t, db, 1)[0]
			parent := prepDocuments(t, db, 1, func(_ int, ndoc *document.Document) {
				ndoc.OrganizationID = org
			})[0]
			child := prepDocuments(t, db, 1, func(_ int, ndoc *document.Document) {
				ndoc.OrganizationID = org
				ndoc.ParentID = null.ValueFrom(parent.ID)
			})[0]

			return tcase{
				ID:             child.ID,
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

			err := db.UpdateDocumentParentID(ctx, c.ID, c.ParentID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var parentID null.Value[xid.ID]

			q, args := db.builder.Select("fk_parent_id").
				From("documents").
				Where(sq.Eq{
					"id": c.ID,
				}).MustSql()

			err = db.sql.Get(&parentID, q, args...)
			require.NoError(t, err)
			assert.Equal(t, c.ParentID, parentID)
		})
	}
}

func Test_agent_DeleteDocument(t *testing.T) {
	type tcase struct {
		ID             xid.ID
		OrganizationID string
		FileID         string
		HookID         xid.ID
		SearchRemoved  []xid.ID
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Successful delete": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				ID:             doc.ID,
				OrganizationID: doc.OrganizationID,
				SearchRemoved:  []xid.ID{doc.ID},
			}
		},
		// the descendants go away with the cascade, so their index entries
		// have to be queued for removal by the delete that destroys them.
		"Descendants are queued for search removal": func(t *testing.T, db *DB) tcase {
			organizationID := prepOrganizations(t, db, 1)[0]

			docs := prepDocuments(t, db, 4, func(_ int, doc *document.Document) {
				doc.OrganizationID = organizationID
			})

			// parent -> child -> grandchild, plus an untouched sibling.
			setParent(t, db, docs[1].ID, docs[0].ID)
			setParent(t, db, docs[2].ID, docs[1].ID)

			return tcase{
				ID:             docs[0].ID,
				OrganizationID: organizationID,
				SearchRemoved:  []xid.ID{docs[0].ID, docs[1].ID, docs[2].ID},
			}
		},
		"Unknown document queues nothing": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				ID:             xid.New(),
				OrganizationID: doc.OrganizationID,
			}
		},
		// files and hooks outlive their document on purpose: the row is
		// the only record of the object or the external watcher, so the
		// managers need it to reclaim them.
		"External resources outlive the document": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			f := prepDocumentFiles(t, db, 1, func(_ int, f *file.File) {
				f.DocumentID = null.ValueFrom(doc.ID)
				f.OrganizationID = null.StringFrom(doc.OrganizationID)
			})[0]

			hk := prepDocumentHooks(t, db, 1, func(_ int, hk *hook.Hook) {
				hk.DocumentID = null.ValueFrom(doc.ID)
				hk.OrganizationID = null.StringFrom(doc.OrganizationID)
				hk.BranchID = null.ValueFrom(doc.BranchID)
			})[0]

			return tcase{
				ID:             doc.ID,
				OrganizationID: doc.OrganizationID,
				FileID:         f.ID,
				HookID:         hk.ID,
				SearchRemoved:  []xid.ID{doc.ID},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.DeleteDocument(context.Background(), c.ID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var doc document.Document

			q, args := db.selectDocumentWithBranch(db.builder.Select(), document.DefaultBranch).
				Where(sq.Eq{
					"documents.id": c.ID,
				}).
				MustSql()

			err = db.sql.Get(&doc, q, args...)
			require.Equal(t, sql.ErrNoRows, err)

			jobs, err := db.FetchDocumentSearchJobs(context.Background(), 0, 10)
			require.NoError(t, err)

			if len(c.SearchRemoved) == 0 {
				assert.Empty(t, jobs)
			} else {
				require.Len(t, jobs, 1)
				assert.ElementsMatch(t, c.SearchRemoved, jobs[0].BlockDiff.RemovedDocuments)
			}

			if c.FileID == "" {
				return
			}

			var f file.File

			q, args = db.selectDocumentFile(db.builder.Select()).
				Where(sq.Eq{"document_files.id": c.FileID}).
				MustSql()

			require.NoError(t, db.sql.Get(&f, q, args...))
			assert.False(t, f.DocumentID.Valid)
			assert.True(t, f.Orphaned())
			assert.NotEmpty(t, f.StorageKey, "the object stays reachable without its document")

			var hk hook.Hook

			q, args = db.selectDocumentHook(db.builder.Select()).
				Where(sq.Eq{"document_hooks.id": c.HookID}).
				MustSql()

			require.NoError(t, db.sql.Get(&hk, q, args...))
			assert.False(t, hk.DocumentID.Valid)
			assert.False(t, hk.BranchID.Valid)
		})
	}
}

// setParent re-parents a document, building a tree out of documents the
// fixture inserts as roots.
func setParent(t *testing.T, db *DB, id, parentID xid.ID) {
	t.Helper()

	q, args := db.builder.Update("documents").
		SetMap(map[string]any{"fk_parent_id": parentID}).
		Where(sq.Eq{"id": id}).
		MustSql()

	_, err := db.sql.Exec(q, args...)
	require.NoError(t, err)
}
