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
			ID: xid.New(),
			Branch: document.Branch{
				BranchID:     xid.New(),
				BranchName:   document.DefaultBranch,
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
			},
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
			Branch: document.Branch{
				BranchID:     xid.New(),
				BranchName:   document.DefaultBranch,
				DocumentName: "Test Document 1",
				Icon:         "icon-test2",
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
			},
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
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Successful delete": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				ID:             doc.ID,
				OrganizationID: doc.OrganizationID,
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
		})
	}
}
