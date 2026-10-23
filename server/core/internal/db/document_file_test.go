package db

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/internal/document/history"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
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
			ID:          "file-" + xid.New().String(),
			Location:    file.LocationDocument,
			StorageKey:  "organizations/org/documents/doc/files/file",
			Name:        "shot.png",
			Size:        1024,
			ContentType: "image/png",
			CreatedAt:   now.Add(-time.Duration(i) * time.Second),
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
				"name":               f.Name,
				"size":               f.Size,
				"content_type":       f.ContentType,
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
		"Existing id": func(t *testing.T, db *DB) tcase {
			f := prepDocumentFiles(t, db, 1, nil)[0]
			f.Name = "notes.zip"

			return tcase{File: f, Err: errutil.New(http.StatusConflict, "document_file.exists", "file id is already in use")}
		},
		"Existing id from another organization": func(t *testing.T, db *DB) tcase {
			f := prepDocumentFiles(t, db, 1, nil)[0]
			doc := prepDocuments(t, db, 1, nil)[0]

			f.DocumentID = null.ValueFrom(doc.ID)
			f.OrganizationID = null.StringFrom(doc.OrganizationID)

			return tcase{File: f, Err: errutil.New(http.StatusConflict, "document_file.exists", "file id is already in use")}
		},
		"Missing document": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				File: file.NewFile("file-1", file.LocationDocument, "key", xid.New(), "org-1", "shot.png", 1024, "image/png"),
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
					Name:           "notes.pdf",
					Size:           4096,
					ContentType:    "application/pdf",
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
	type tcase struct {
		OffsetID string
		Limit    int64
		Result   []string
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Empty": func(_ *testing.T, _ *DB) tcase {
			return tcase{Limit: 10}
		},
		"First page": func(t *testing.T, db *DB) tcase {
			ff := prepDocumentFiles(t, db, 3, func(i int, f *file.File) {
				f.ID = "file-" + strconv.Itoa(i)
			})

			return tcase{Limit: 2, Result: []string{ff[0].ID, ff[1].ID}}
		},
		"Second page excludes everything up to the offset id": func(t *testing.T, db *DB) tcase {
			ff := prepDocumentFiles(t, db, 3, func(i int, f *file.File) {
				f.ID = "file-" + strconv.Itoa(i)
			})

			return tcase{OffsetID: ff[1].ID, Limit: 2, Result: []string{ff[2].ID}}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchPaginatedDocumentFiles(context.Background(), c.OffsetID, c.Limit)
			require.NoError(t, err)
			require.Len(t, res, len(c.Result))

			for i, id := range c.Result {
				assert.Equal(t, id, res[i].ID)
			}
		})
	}
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
	type tcase struct {
		ID         string
		DocumentID xid.ID
		Result     bool
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Referenced by branch content": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}
			})[0]

			return tcase{ID: "file-ref", DocumentID: doc.ID, Result: true}
		},
		"Referenced by a file block in branch content": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.Content.Content = append(doc.Content.Content, document.Block{
					Type: document.BlockNodeFileBlock,
					Attrs: document.Attributes{
						"uid":         "file-ref",
						"src":         "/api/documents/" + doc.ID.String() + "/files/file-ref",
						"name":        "notes.zip",
						"size":        2048,
						"contentType": "application/zip",
					},
				})
			})[0]

			return tcase{ID: "file-ref", DocumentID: doc.ID, Result: true}
		},
		"Referenced by a history entry": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}

			insertHistoryEntry(t, db, history.NewEntry(*doc, timeutil.Now(), null.String{}, nil, false))

			return tcase{ID: "file-ref", DocumentID: doc.ID, Result: true}
		},
		"Referenced by a comment": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
				c.Content = comment.Content{"uid": "file-ref"}
			})[0]

			return tcase{ID: "file-ref", DocumentID: c.DocumentID, Result: true}
		},
		"Referenced by a comment reply": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]

			prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
				r.Content = comment.Content{"uid": "file-ref"}
			})

			return tcase{ID: "file-ref", DocumentID: c.DocumentID, Result: true}
		},
		"Referenced by another document": func(t *testing.T, db *DB) tcase {
			prepDocuments(t, db, 1, func(_ int, doc *document.Document) {
				doc.Content.Content[0].Attrs = document.Attributes{"uid": "file-ref"}
			})

			other := prepDocuments(t, db, 1, nil)[0]

			return tcase{ID: "file-ref", DocumentID: other.ID, Result: false}
		},
		"Not referenced": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{ID: "file-ref", DocumentID: doc.ID, Result: false}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.CheckDocumentFileReferenced(context.Background(), c.ID, c.DocumentID)
			require.NoError(t, err)
			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_agent_DeleteExpiredDocumentBranchHistoryEntries(t *testing.T) {
	type tcase struct {
		Before time.Time
		// Rows maps each branch to how many entries it holds afterwards.
		Rows map[xid.ID]int
		// Newest maps each branch to its newest entry afterwards.
		Newest map[xid.ID]xid.ID
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Nothing expired": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			entries := prepHistoryEntries(t, db, doc, 4)

			return tcase{
				Before: entries[0].CreatedAt.Add(-time.Second),
				Rows:   map[xid.ID]int{doc.BranchID: 4},
				Newest: map[xid.ID]xid.ID{doc.BranchID: entries[3].ID},
			}
		},
		"Expired entries go": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			entries := prepHistoryEntries(t, db, doc, 4)

			return tcase{
				Before: entries[2].CreatedAt,
				Rows:   map[xid.ID]int{doc.BranchID: 2},
				Newest: map[xid.ID]xid.ID{doc.BranchID: entries[3].ID},
			}
		},
		"Newest entry of each branch stays": func(t *testing.T, db *DB) tcase {
			docs := prepDocuments(t, db, 2, nil)
			entries := prepHistoryEntries(t, db, docs[0], 4)
			other := prepHistoryEntries(t, db, docs[1], 2)

			return tcase{
				Before: entries[3].CreatedAt.Add(time.Hour),
				Rows: map[xid.ID]int{
					docs[0].BranchID: 1,
					docs[1].BranchID: 1,
				},
				Newest: map[xid.ID]xid.ID{
					docs[0].BranchID: entries[3].ID,
					docs[1].BranchID: other[1].ID,
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			require.NoError(t, db.DeleteExpiredDocumentBranchHistoryEntries(context.Background(), c.Before))

			for branchID, rows := range c.Rows {
				assert.Equal(t, rows, countHistoryEntries(t, db, branchID))
			}

			for branchID, id := range c.Newest {
				newest, err := db.fetchNewestDocumentBranchHistoryEntry(context.Background(), db.sql, branchID)
				require.NoError(t, err)
				require.NotNil(t, newest)
				assert.Equal(t, id, newest.ID)
			}
		})
	}
}
