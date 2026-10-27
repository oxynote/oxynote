package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
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
		From           search.ViewFrom
		ViewedAt       time.Time
		Results        map[search.ViewFrom][]search.RecentDocument
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
				From:           search.ViewFromAll,
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
				From:           search.ViewFromAll,
				ViewedAt:       now,
				Results: map[search.ViewFrom][]search.RecentDocument{
					search.ViewFromAll:    {{Document: *doc, ViewedAt: now}},
					search.ViewFromSearch: {},
				},
			}
		},
		"Successful insert from search": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				UserID:         prepUsers(t, db, 1)[0],
				OrganizationID: doc.OrganizationID,
				BranchID:       doc.BranchID,
				From:           search.ViewFromSearch,
				ViewedAt:       now,
				Results: map[search.ViewFrom][]search.RecentDocument{
					search.ViewFromAll:    {{Document: *doc, ViewedAt: now}},
					search.ViewFromSearch: {{Document: *doc, ViewedAt: now}},
				},
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
					search.ViewFromSearch,
					now.Add(-time.Duration(len(docs)-i)*time.Hour),
				))
			}

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				BranchID:       docs[0].BranchID,
				From:           search.ViewFromSearch,
				ViewedAt:       now,
				Results: map[search.ViewFrom][]search.RecentDocument{
					search.ViewFromAll: {
						{Document: *docs[0], ViewedAt: now},
						{Document: *docs[1], ViewedAt: now.Add(-time.Hour)},
					},
					search.ViewFromSearch: {
						{Document: *docs[0], ViewedAt: now},
						{Document: *docs[1], ViewedAt: now.Add(-time.Hour)},
					},
				},
			}
		},
		"Successful update that leaves the other origin as it was": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			require.NoError(t, db.UpsertDocumentBranchView(
				context.Background(),
				user,
				doc.OrganizationID,
				doc.BranchID,
				search.ViewFromSearch,
				now.Add(-time.Hour),
			))

			return tcase{
				UserID:         user,
				OrganizationID: doc.OrganizationID,
				BranchID:       doc.BranchID,
				From:           search.ViewFromAll,
				ViewedAt:       now,
				Results: map[search.ViewFrom][]search.RecentDocument{
					search.ViewFromAll:    {{Document: *doc, ViewedAt: now}},
					search.ViewFromSearch: {{Document: *doc, ViewedAt: now.Add(-time.Hour)}},
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.UpsertDocumentBranchView(context.Background(), c.UserID, c.OrganizationID, c.BranchID, c.From, c.ViewedAt)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			for from, exp := range c.Results {
				res, err := db.FetchRecentlyViewedDocuments(context.Background(), c.UserID, c.OrganizationID, from, 10)
				require.NoError(t, err)
				testutil.AssertFilterEqual(t, exp, res)
			}
		})
	}
}

func Test_agent_FetchRecentlyViewedDocuments(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		UserID           string
		OrganizationID   string
		From             search.ViewFrom
		Limit            int
		Result           []search.RecentDocument
		Err              error
	}

	now := timeutil.Now().Truncate(time.Second)

	// view records the user's views of the documents from the origin, the
	// first one viewed most recently.
	view := func(t *testing.T, db *DB, user string, from search.ViewFrom, docs ...*document.Document) {
		t.Helper()

		for i, doc := range docs {
			require.NoError(t, db.UpsertDocumentBranchView(
				context.Background(),
				user,
				doc.OrganizationID,
				doc.BranchID,
				from,
				now.Add(-time.Duration(i)*time.Minute),
			))
		}
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				UserID:           "user",
				OrganizationID:   "org",
				From:             search.ViewFromAll,
				Limit:            10,
				Err:              assert.AnError,
			}
		},
		"No views": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				UserID:         prepUsers(t, db, 1)[0],
				OrganizationID: doc.OrganizationID,
				From:           search.ViewFromAll,
				Limit:          10,
				Result:         []search.RecentDocument{},
			}
		},
		"Most recent first within the limit": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 3, nil)
			user := prepUsers(t, db, 1)[0]

			view(t, db, user, search.ViewFromAll, docs[2], docs[0], docs[1])

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				From:           search.ViewFromAll,
				Limit:          2,
				Result: []search.RecentDocument{
					{Document: *docs[2], ViewedAt: now},
					{Document: *docs[0], ViewedAt: now.Add(-time.Minute)},
				},
			}
		},
		"Views from other origins are left out": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 2, nil)
			user := prepUsers(t, db, 1)[0]

			view(t, db, user, search.ViewFromAll, docs[0])
			view(t, db, user, search.ViewFromSearch, docs[1])

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				From:           search.ViewFromSearch,
				Limit:          10,
				Result: []search.RecentDocument{
					{Document: *docs[1], ViewedAt: now},
				},
			}
		},
		"Views of other users and organizations are left out": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 1, nil)
			other := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)

			view(t, db, users[0], search.ViewFromAll, docs[0], other)
			view(t, db, users[1], search.ViewFromAll, docs[0])

			return tcase{
				UserID:         users[0],
				OrganizationID: docs[0].OrganizationID,
				From:           search.ViewFromAll,
				Limit:          10,
				Result: []search.RecentDocument{
					{Document: *docs[0], ViewedAt: now},
				},
			}
		},
		"Views of deleted branches are gone": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 2, nil)
			user := prepUsers(t, db, 1)[0]

			view(t, db, user, search.ViewFromAll, docs[0], docs[1])

			require.NoError(t, db.DeleteDocumentBranchByID(context.Background(), docs[0].BranchID, docs[0].OrganizationID))

			return tcase{
				UserID:         user,
				OrganizationID: docs[0].OrganizationID,
				From:           search.ViewFromAll,
				Limit:          10,
				Result: []search.RecentDocument{
					{Document: *docs[1], ViewedAt: now.Add(-time.Minute)},
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

			res, err := db.FetchRecentlyViewedDocuments(ctx, c.UserID, c.OrganizationID, c.From, c.Limit)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchDocumentBranchViews(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		Result           []document.BranchView
		Err              error
	}

	now := timeutil.Now().Truncate(time.Second)

	// view records a view of the branch by each user from the origin, the
	// first user's the most recent.
	view := func(t *testing.T, db *DB, doc *document.Document, from search.ViewFrom, users ...string) {
		t.Helper()

		for i, user := range users {
			require.NoError(t, db.UpsertDocumentBranchView(
				context.Background(),
				user,
				doc.OrganizationID,
				doc.BranchID,
				from,
				now.Add(-time.Duration(i)*time.Minute),
			))
		}
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				BranchID:         xid.New(),
				OrganizationID:   "org",
				Err:              assert.AnError,
			}
		},
		"No views": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Result:         []document.BranchView{},
			}
		},
		"Most recent first": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 3)

			view(t, db, doc, search.ViewFromAll, users[1], users[2], users[0])

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Result: []document.BranchView{
					{UserID: users[1], ViewedAt: now},
					{UserID: users[2], ViewedAt: now.Add(-time.Minute)},
					{UserID: users[0], ViewedAt: now.Add(-2 * time.Minute)},
				},
			}
		},
		"Views at the same time ordered by user": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 2)
			slices.Sort(users)

			view(t, db, doc, search.ViewFromAll, users[1])
			view(t, db, doc, search.ViewFromAll, users[0])

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Result: []document.BranchView{
					{UserID: users[0], ViewedAt: now},
					{UserID: users[1], ViewedAt: now},
				},
			}
		},
		"One view per user whatever the origin": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			user := prepUsers(t, db, 1)[0]

			view(t, db, doc, search.ViewFromSearch, user)

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: doc.OrganizationID,
				Result: []document.BranchView{
					{UserID: user, ViewedAt: now},
				},
			}
		},
		"Views of other branches are left out": func(t *testing.T, db *DB) tcase {
			docs := prepDocumentBranches(t, db, 2, nil)
			users := prepUsers(t, db, 2)

			view(t, db, docs[0], search.ViewFromAll, users[0])
			view(t, db, docs[1], search.ViewFromAll, users[1])

			return tcase{
				BranchID:       docs[0].BranchID,
				OrganizationID: docs[0].OrganizationID,
				Result: []document.BranchView{
					{UserID: users[0], ViewedAt: now},
				},
			}
		},
		"Another organization sees no views": func(t *testing.T, db *DB) tcase {
			doc := prepDocuments(t, db, 1, nil)[0]
			other := prepDocuments(t, db, 1, nil)[0]

			view(t, db, doc, search.ViewFromAll, prepUsers(t, db, 1)...)

			return tcase{
				BranchID:       doc.BranchID,
				OrganizationID: other.OrganizationID,
				Result:         []document.BranchView{},
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

			res, err := db.FetchDocumentBranchViews(ctx, c.BranchID, c.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)

				return
			}

			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}
