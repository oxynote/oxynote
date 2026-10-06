package db

import (
	"context"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_agent_UpsertDocumentBranchView(t *testing.T) {
	type tcase struct {
		UserID         string
		OrganizationID string
		BranchID       xid.ID
		ViewedAt       time.Time
		Result         []document.Document
		Err            error
	}

	now := timeutil.Now().Truncate(time.Second)

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				UserID:         prepUsers(t, db, 1)[0],
				OrganizationID: doc.OrganizationID,
				BranchID:       xid.New(),
				ViewedAt:       now,
				Err:            assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				UserID:         prepUsers(t, db, 1)[0],
				OrganizationID: doc.OrganizationID,
				BranchID:       doc.BranchID,
				ViewedAt:       now,
				Result:         []document.Document{*doc},
			}
		},
		"Successful update of an earlier view": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 2, nil)
			user := prepUsers(t, db, 1)[0]

			for i, doc := range docs {
				require.NoError(t, db.UpsertDocumentBranchView(
					context.Background(),
					user,
					doc.OrganizationID,
					doc.BranchID,
					now.Add(-time.Duration(len(docs)-i)*time.Hour),
				))
			}

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				BranchID:       docs[0].BranchID,
				ViewedAt:       now,
				Result:         []document.Document{*docs[0], *docs[1]},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpsertDocumentBranchView(context.Background(), c.UserID, c.OrganizationID, c.BranchID, c.ViewedAt)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchRecentlyViewedDocuments(context.Background(), c.UserID, c.OrganizationID, 10)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchRecentlyViewedDocuments(t *testing.T) {
	type tcase struct {
		UserID         string
		OrganizationID string
		Limit          int
		Result         []document.Document
	}

	now := timeutil.Now().Truncate(time.Second)

	// view records the user's views of the documents, the first one
	// viewed most recently.
	view := func(t *testing.T, db *DB, user string, docs ...*document.Document) {
		t.Helper()

		for i, doc := range docs {
			require.NoError(t, db.UpsertDocumentBranchView(
				context.Background(),
				user,
				doc.OrganizationID,
				doc.BranchID,
				now.Add(-time.Duration(i)*time.Minute),
			))
		}
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"No views": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				UserID:         prepUsers(t, db, 1)[0],
				OrganizationID: doc.OrganizationID,
				Limit:          10,
				Result:         []document.Document{},
			}
		},
		"Most recent first within the limit": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 3, nil)
			user := prepUsers(t, db, 1)[0]

			view(t, db, user, docs[2], docs[0], docs[1])

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				Limit:          2,
				Result:         []document.Document{*docs[2], *docs[0]},
			}
		},
		"Views of other users and organizations are left out": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 1, nil)
			other := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			view(t, db, users[0], docs[0], other)
			view(t, db, users[1], docs[0])

			return tcase{
				UserID:         users[0],
				OrganizationID: docs[0].OrganizationID,
				Limit:          10,
				Result:         []document.Document{*docs[0]},
			}
		},
		"Views of deleted branches are gone": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 2, nil)
			user := prepUsers(t, db, 1)[0]

			view(t, db, user, docs[0], docs[1])

			require.NoError(t, db.DeleteDocumentBranchByID(context.Background(), docs[0].BranchID, docs[0].OrganizationID))

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				Limit:          10,
				Result:         []document.Document{*docs[1]},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			res, err := db.FetchRecentlyViewedDocuments(context.Background(), c.UserID, c.OrganizationID, c.Limit)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}

	// error - cancelled context
	db := prepTempDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchRecentlyViewedDocuments(ctx, "user", "org", 10)
	require.Error(t, err)
	assert.Nil(t, res)
}
