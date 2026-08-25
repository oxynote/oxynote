package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentFiles(t *testing.T, db *DB, count int, fn func(int, *file.File)) []file.File {
	t.Helper()

	res := make([]file.File, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		f := file.File{
			ID:         "file-" + xid.New().String(),
			Location:   file.LocationDocument,
			StorageKey: "organizations/org/documents/doc/files/file",
			CreatedAt:  now.Add(-time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, &f)
		}

		if !f.DocumentID.Valid {
			doc := prepDocuments(t, db, 1, nil)[0]
			f.DocumentID = null.ValueFrom(doc.ID)
			f.OrganizationID = null.StringFrom(doc.OrganizationID)
		}

		res[i] = f

		q, args := db.builder.Insert("document_files").
			SetMap(map[string]any{
				"id":                 f.ID,
				"location":           f.Location,
				"storage_key":        f.StorageKey,
				"fk_document_id":     f.DocumentID,
				"fk_organization_id": f.OrganizationID,
				"created_at":         f.CreatedAt,
				"unreferenced_at":    f.UnreferencedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDocumentFile(t *testing.T) {
	type tcase struct {
		File file.File
		Err  error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Re-upload refreshes the existing row": func(t *testing.T, db *DB) tcase {
			f := prepDocumentFiles(t, db, 1, func(_ int, f *file.File) {
				f.UnreferencedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			})[0]

			f.Location = file.LocationComment
			f.StorageKey = "organizations/org/documents/doc/files/replaced"
			f.CreatedAt = timeutil.Now().Truncate(time.Second)
			f.UnreferencedAt = null.Time{}

			return tcase{File: f}
		},
		"Missing document": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				File: file.NewFile("file-1", file.LocationDocument, "key", xid.New(), "org-1"),
				Err:  assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				File: file.File{
					ID:             "file-1",
					Location:       file.LocationComment,
					StorageKey:     "organizations/org/documents/doc/files/file-1",
					DocumentID:     null.ValueFrom(doc.ID),
					OrganizationID: null.StringFrom(doc.OrganizationID),
					CreatedAt:      timeutil.Now().Truncate(time.Second),
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertDocumentFile(context.Background(), c.File)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentFile(context.Background(), c.File.ID, c.File.OrganizationID.String)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.File, res)
		})
	}
}

func Test_agent_FetchDocumentFile(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentFile(context.Background(), "non-existent-file-id", "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	f := prepDocumentFiles(t, db, 1, nil)[0]

	res, err = db.FetchDocumentFile(context.Background(), f.ID, f.OrganizationID.String)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &f, res)
}

func Test_agent_FetchPaginatedDocumentFiles(t *testing.T) {
	db := prepTempDB(t)

	// empty
	res, err := db.FetchPaginatedDocumentFiles(context.Background(), "", 10)
	require.NoError(t, err)
	assert.Empty(t, res)

	ff := prepDocumentFiles(t, db, 3, func(i int, f *file.File) {
		f.ID = "file-" + strconv.Itoa(i)
	})

	// first page
	res, err = db.FetchPaginatedDocumentFiles(context.Background(), "", 2)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, ff[0].ID, res[0].ID)
	assert.Equal(t, ff[1].ID, res[1].ID)

	// second page, offset excludes everything up to the given id
	res, err = db.FetchPaginatedDocumentFiles(context.Background(), res[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, ff[2].ID, res[0].ID)
}

func Test_agent_UpdateDocumentFileUnreferencedAt(t *testing.T) {
	db := prepTempDB(t)

	f := prepDocumentFiles(t, db, 1, nil)[0]
	at := timeutil.Now().Truncate(time.Second)

	// set
	require.NoError(t, db.UpdateDocumentFileUnreferencedAt(context.Background(), f.ID, null.TimeFrom(at)))

	res, err := db.FetchDocumentFile(context.Background(), f.ID, f.OrganizationID.String)
	require.NoError(t, err)
	assert.Equal(t, at.UTC(), res.UnreferencedAt.Time.UTC())

	// clear
	require.NoError(t, db.UpdateDocumentFileUnreferencedAt(context.Background(), f.ID, null.Time{}))

	res, err = db.FetchDocumentFile(context.Background(), f.ID, f.OrganizationID.String)
	require.NoError(t, err)
	assert.False(t, res.UnreferencedAt.Valid)
}

func Test_agent_DeleteDocumentFile(t *testing.T) {
	db := prepTempDB(t)

	f := prepDocumentFiles(t, db, 1, nil)[0]

	require.NoError(t, db.DeleteDocumentFile(context.Background(), f.ID))

	_, err := db.FetchDocumentFile(context.Background(), f.ID, f.OrganizationID.String)
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
}

func Test_agent_CheckDocumentFileReferenced(t *testing.T) {
	cc := map[string]func(*testing.T, *DB) (string, xid.ID, bool){
		"Referenced by branch content": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}
			})[0]

			return "file-ref", doc.ID, true
		},
		"Referenced by a changelog snapshot": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}

			require.NoError(t, db.insertDocumentBranchChangelog(
				context.Background(),
				db.sql,
				doc.ID,
				doc.BranchID,
				doc.Changelog(),
			))

			return "file-ref", doc.ID, true
		},
		"Referenced by a comment": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
				c.Content = comment.Content{"uid": "file-ref"}
			})[0]

			return "file-ref", c.DocumentID, true
		},
		"Referenced by a comment reply": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			c := prepDocumentComments(t, db, 1, nil)[0]

			prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
				r.Content = comment.Content{"uid": "file-ref"}
			})

			return "file-ref", c.DocumentID, true
		},
		"Referenced by another document": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}
			})

			other := prepDocuments(t, db, 1, nil)[0]

			return "file-ref", other.ID, false
		},
		"Not referenced": func(t *testing.T, db *DB) (string, xid.ID, bool) {
			doc := prepDocuments(t, db, 1, nil)[0]

			return "file-ref", doc.ID, false
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			id, documentID, exp := cfn(t, db)

			res, err := db.CheckDocumentFileReferenced(context.Background(), id, documentID)
			require.NoError(t, err)
			assert.Equal(t, exp, res)
		})
	}
}
