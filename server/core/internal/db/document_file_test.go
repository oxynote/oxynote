package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentFiles(t *testing.T, db *DB, count int, fn func(int, *document.File)) []document.File {
	t.Helper()

	res := make([]document.File, count)

	now := timeutil.Now().Truncate(time.Second)

	for i := range count {
		f := document.File{
			ID:        "file-" + xid.New().String(),
			Location:  document.LocationDocument,
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, &f)
		}

		if f.DocumentID.IsZero() {
			doc := prepDocuments(t, db, 1, nil)[0]
			f.DocumentID = doc.ID
			f.OrganizationID = doc.OrganizationID
		}

		res[i] = f

		q, args := db.builder.Insert("document_files").
			SetMap(map[string]any{
				"id":                 f.ID,
				"location":           f.Location,
				"fk_document_id":     f.DocumentID,
				"fk_organization_id": f.OrganizationID,
				"created_at":         f.CreatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDocumentFile(t *testing.T) {
	type tcase struct {
		File document.File
		Err  error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Duplicate file ID": func(t *testing.T, db *DB) tcase {
			f := prepDocumentFiles(t, db, 1, nil)[0]

			return tcase{
				File: f,
				Err:  assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				File: document.File{
					ID:             "file-1",
					Location:       document.LocationComment,
					DocumentID:     doc.ID,
					OrganizationID: doc.OrganizationID,
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

			res, err := db.FetchDocumentFile(context.Background(), c.File.ID, c.File.OrganizationID)
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

	res, err = db.FetchDocumentFile(context.Background(), f.ID, f.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &f, res)
}
