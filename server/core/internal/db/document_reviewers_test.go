package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepBranchReviewers(t *testing.T, db *DB, count int, fn func(int, *document.BranchReviewer)) []document.BranchReviewer {
	t.Helper()

	res := make([]document.BranchReviewer, count)

	// reviewers missing a branch after fn ran share one lazily
	// created branch, so a single call produces co-reviewers.
	var branch *document.Document

	for i := range count {
		reviewer := document.BranchReviewer{}

		if fn != nil {
			fn(i, &reviewer)
		}

		if reviewer.BranchID.IsZero() {
			if branch == nil {
				branch = prepDocumentBranches(t, db, 1, nil)[0]
			}

			reviewer.BranchID = branch.BranchID
			reviewer.OrganizationID = branch.OrganizationID
		}

		if reviewer.UserID == "" {
			reviewer.UserID = prepUsers(t, db, 1)[0]
		}

		res[i] = reviewer

		q, args := db.builder.Insert("branch_reviewers").
			SetMap(map[string]any{
				"fk_branch_id":        reviewer.BranchID,
				"fk_user_id":          reviewer.UserID,
				"fk_organization_id":  reviewer.OrganizationID,
				"currently_approved":  reviewer.CurrentlyApproved,
				"previously_approved": reviewer.PreviouslyApproved,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_FetchBranchReviewers(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, err := db.FetchBranchReviewers(ctx, xid.New(), "org-id")
	require.Error(t, err)
	assert.Nil(t, res)

	// success - no reviewers
	res, err = db.FetchBranchReviewers(context.Background(), xid.New(), "non-existent-org-id")
	require.NoError(t, err)
	assert.Empty(t, res)

	// success
	reviewers := prepBranchReviewers(t, db, 2, nil)

	res, err = db.FetchBranchReviewers(context.Background(), reviewers[0].BranchID, reviewers[0].OrganizationID)
	assert.NoError(t, err)
	assert.ElementsMatch(t, reviewers, res)
}

func Test_agent_FetchBranchReviewer(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchBranchReviewer(context.Background(), xid.New(), "user-id", "org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	reviewer := prepBranchReviewers(t, db, 1, func(_ int, r *document.BranchReviewer) {
		r.CurrentlyApproved = true
	})[0]

	res, err = db.FetchBranchReviewer(context.Background(), reviewer.BranchID, reviewer.UserID, reviewer.OrganizationID)
	assert.NoError(t, err)
	assert.Equal(t, &reviewer, res)
}

func Test_agent_InsertBranchReviewer(t *testing.T) {
	type tcase struct {
		Reviewer document.BranchReviewer
		Result   document.BranchReviewer
		Err      error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)

			return tcase{
				Reviewer: document.BranchReviewer{
					BranchID:       xid.New(),
					UserID:         users[0],
					OrganizationID: "non-existent-org-id",
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			users := prepUsers(t, db, 1)

			reviewer := document.BranchReviewer{
				BranchID:       branch.BranchID,
				UserID:         users[0],
				OrganizationID: branch.OrganizationID,
			}

			return tcase{
				Reviewer: reviewer,
				Result:   reviewer,
			}
		},
		"Existing reviewer is left untouched": func(t *testing.T, db *DB) tcase {
			original := prepBranchReviewers(t, db, 1, func(_ int, r *document.BranchReviewer) {
				r.CurrentlyApproved = true
			})[0]

			reviewer := original
			reviewer.CurrentlyApproved = false

			return tcase{
				Reviewer: reviewer,
				Result:   original,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertBranchReviewer(context.Background(), c.Reviewer)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchBranchReviewer(context.Background(), c.Reviewer.BranchID, c.Reviewer.UserID, c.Reviewer.OrganizationID)
			require.NoError(t, err)
			assert.Equal(t, &c.Result, res)
		})
	}
}

func Test_agent_UpdateBranchReviewer(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Reviewer         document.BranchReviewer
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			reviewer := prepBranchReviewers(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Reviewer:         reviewer,
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			reviewer := prepBranchReviewers(t, db, 1, nil)[0]
			reviewer.CurrentlyApproved = true

			return tcase{
				Reviewer: reviewer,
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

			err := db.UpdateBranchReviewer(ctx, c.Reviewer)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchBranchReviewer(context.Background(), c.Reviewer.BranchID, c.Reviewer.UserID, c.Reviewer.OrganizationID)
			require.NoError(t, err)
			assert.Equal(t, &c.Reviewer, res)
		})
	}
}

func Test_agent_DeleteBranchReviewer(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Reviewer         document.BranchReviewer
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			reviewer := prepBranchReviewers(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Reviewer:         reviewer,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			reviewer := prepBranchReviewers(t, db, 1, nil)[0]

			return tcase{
				Reviewer: reviewer,
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

			err := db.DeleteBranchReviewer(ctx, c.Reviewer.BranchID, c.Reviewer.UserID, c.Reviewer.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchBranchReviewer(context.Background(), c.Reviewer.BranchID, c.Reviewer.UserID, c.Reviewer.OrganizationID)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_PromoteBranchApprovals(t *testing.T) {
	t.Run("Error returned by the stale reviewer cleanup", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM branch_reviewers").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.PromoteBranchApprovals(context.Background(), xid.New(), xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Error returned by the reviewer update", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE branch_reviewers").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.PromoteBranchApprovals(context.Background(), xid.New(), xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Error returned by the reviewer upsert", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO branch_reviewers").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.PromoteBranchApprovals(context.Background(), xid.New(), xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Error returned by the source cleanup", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		mock.ExpectBegin()
		mock.ExpectExec("DELETE FROM branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("UPDATE branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("INSERT INTO branch_reviewers").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec("DELETE FROM branch_reviewers").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.PromoteBranchApprovals(context.Background(), xid.New(), xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Cancelled context", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := db.PromoteBranchApprovals(ctx, xid.New(), xid.New(), "org-id")
		require.Error(t, err)
	})

	t.Run("Successful promotion", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		doc := prepDocuments(t, db, 1, nil)[0]
		target := prepDocumentBranches(t, db, 1, func(_ int, ndoc *document.Document) {
			ndoc.ID = doc.ID
			ndoc.OrganizationID = doc.OrganizationID
		})[0]
		users := prepUsers(t, db, 4)

		// the target branch holds one reviewer who never approved
		// (dropped), one currently approved (demoted to previously
		// approved and shared with the source branch), and one with
		// only a stale previous approval.
		prepBranchReviewers(t, db, 3, func(i int, r *document.BranchReviewer) {
			r.BranchID = target.BranchID
			r.OrganizationID = target.OrganizationID
			r.UserID = users[i]
			r.CurrentlyApproved = i == 1
			r.PreviouslyApproved = i == 2
		})

		// the source branch holds the shared reviewer with a fresh
		// approval and a brand new reviewer without one.
		prepBranchReviewers(t, db, 2, func(i int, r *document.BranchReviewer) {
			r.BranchID = doc.BranchID
			r.OrganizationID = doc.OrganizationID
			r.UserID = users[2*i+1]
			r.CurrentlyApproved = i == 0
		})

		err := db.PromoteBranchApprovals(context.Background(), doc.BranchID, target.BranchID, doc.OrganizationID)
		require.NoError(t, err)

		// the source branch is cleared.
		res, err := db.FetchBranchReviewers(context.Background(), doc.BranchID, doc.OrganizationID)
		require.NoError(t, err)
		assert.Empty(t, res)

		// users[0] never approved and is dropped; users[1] is demoted
		// to previously approved and re-approves via the source branch;
		// users[2] loses the stale previous approval; users[3] joins
		// without an approval.
		res, err = db.FetchBranchReviewers(context.Background(), target.BranchID, target.OrganizationID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []document.BranchReviewer{
			{
				BranchID:           target.BranchID,
				UserID:             users[1],
				OrganizationID:     target.OrganizationID,
				CurrentlyApproved:  true,
				PreviouslyApproved: true,
			},
			{
				BranchID:           target.BranchID,
				UserID:             users[2],
				OrganizationID:     target.OrganizationID,
				CurrentlyApproved:  false,
				PreviouslyApproved: false,
			},
			{
				BranchID:           target.BranchID,
				UserID:             users[3],
				OrganizationID:     target.OrganizationID,
				CurrentlyApproved:  false,
				PreviouslyApproved: false,
			},
		}, res)
	})
}
