package db

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepDocumentComments(t *testing.T, db *DB, count int, fn func(int, *comment.Comment)) []comment.Comment {
	t.Helper()

	res := make([]comment.Comment, count)

	now := timeutil.Now().Truncate(time.Second)

	// comments missing a branch after fn ran share one lazily created
	// branch, so a single call produces comments of the same branch.
	var branch *document.Document

	for i := range count {
		c := comment.Comment{
			ID: xid.New(),
			Content: comment.Content{
				"type": "paragraph",
				"text": "Comment " + strconv.Itoa(i),
			},
			// distinct timestamps keep the fetch order deterministic.
			CreatedAt: now.Add(-time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, &c)
		}

		if c.BranchID.IsZero() {
			if branch == nil {
				branch = prepDocumentBranches(t, db, 1, nil)[0]
			}

			c.BranchID = branch.BranchID
			c.DocumentID = branch.ID
			c.OrganizationID = branch.OrganizationID
		}

		res[i] = c

		q, args := db.builder.Insert("document_comments").
			SetMap(map[string]any{
				"id":                    c.ID,
				"fk_document_id":        c.DocumentID,
				"fk_organization_id":    c.OrganizationID,
				"fk_user_id":            c.UserID,
				"fk_branch_id":          c.BranchID,
				"resolved":              c.Resolved,
				"fk_resolved_by":        c.ResolvedBy,
				"anchor_block_id":       c.AnchorBlockID,
				"content":               c.Content,
				"diff_deletion_context": c.DiffDeletionContext,
				"created_at":            c.CreatedAt,
				"updated_at":            c.UpdatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func prepDocumentCommentReplies(t *testing.T, db *DB, count int, fn func(int, *comment.Reply)) []comment.Reply {
	t.Helper()

	res := make([]comment.Reply, count)

	now := timeutil.Now().Truncate(time.Second)

	// replies missing a comment after fn ran share one lazily created
	// comment, so a single call produces replies of the same comment.
	var parent *comment.Comment

	for i := range count {
		r := comment.Reply{
			ID: xid.New(),
			Content: comment.Content{
				"type": "paragraph",
				"text": "Reply " + strconv.Itoa(i),
			},
			// distinct ascending timestamps keep the fetch order
			// deterministic.
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, &r)
		}

		if r.CommentID.IsZero() {
			if parent == nil {
				parent = &prepDocumentComments(t, db, 1, nil)[0]
			}

			r.CommentID = parent.ID
			r.OrganizationID = parent.OrganizationID
		}

		res[i] = r

		q, args := db.builder.Insert("document_comment_replies").
			SetMap(map[string]any{
				"id":                     r.ID,
				"fk_document_comment_id": r.CommentID,
				"fk_organization_id":     r.OrganizationID,
				"fk_user_id":             r.UserID,
				"content":                r.Content,
				"created_at":             r.CreatedAt,
				"updated_at":             r.UpdatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_InsertDocumentComment(t *testing.T) {
	type tcase struct {
		Comment comment.Comment
		Err     error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent branch": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Comment: comment.Comment{
					ID:             xid.New(),
					DocumentID:     xid.New(),
					OrganizationID: "non-existent-org-id",
					BranchID:       xid.New(),
					Content: comment.Content{
						"text": "hello",
					},
					CreatedAt: timeutil.Now().Truncate(time.Second),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			branch := prepDocumentBranches(t, db, 1, nil)[0]
			users := prepUsers(t, db, 1)

			return tcase{
				Comment: comment.Comment{
					ID:             xid.New(),
					DocumentID:     branch.ID,
					OrganizationID: branch.OrganizationID,
					BranchID:       branch.BranchID,
					AnchorBlockID:  null.StringFrom("block-1"),
					UserID:         null.StringFrom(users[0]),
					Content: comment.Content{
						"type": "paragraph",
						"text": "A fresh comment",
					},
					CreatedAt: timeutil.Now().Truncate(time.Second),
					Replies:   []comment.Reply{},
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertDocumentComment(context.Background(), c.Comment)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentComment(context.Background(), c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Comment, res)
		})
	}
}

func Test_agent_InsertDocumentCommentReply(t *testing.T) {
	type tcase struct {
		Reply comment.Reply
		Err   error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent comment": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Reply: comment.Reply{
					ID:             xid.New(),
					CommentID:      xid.New(),
					OrganizationID: "non-existent-org-id",
					Content: comment.Content{
						"text": "hello",
					},
					CreatedAt: timeutil.Now().Truncate(time.Second),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			users := prepUsers(t, db, 1)

			return tcase{
				Reply: comment.Reply{
					ID:             xid.New(),
					CommentID:      c.ID,
					OrganizationID: c.OrganizationID,
					UserID:         null.StringFrom(users[0]),
					Content: comment.Content{
						"type": "paragraph",
						"text": "A fresh reply",
					},
					CreatedAt: timeutil.Now().Truncate(time.Second),
				},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := db.InsertDocumentCommentReply(context.Background(), c.Reply)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentCommentReply(context.Background(), c.Reply.ID, c.Reply.CommentID, c.Reply.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Reply, res)
		})
	}
}

func Test_agent_FetchDocumentComment(t *testing.T) {
	t.Run("Not found", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		res, err := db.FetchDocumentComment(context.Background(), xid.New(), xid.New(), "non-existent-org-id")
		testutil.AssertEqualError(t, sql.ErrNoRows, err)
		assert.Nil(t, res)
	})

	t.Run("Error returned by the replies query", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		id := xid.New()

		mock.ExpectQuery("SELECT .* FROM document_comments").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id.String()))
		mock.ExpectQuery("SELECT .* FROM document_comment_replies").
			WillReturnError(assert.AnError)

		res, err := a.FetchDocumentComment(context.Background(), id, xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
		assert.Nil(t, res)
	})

	t.Run("Successful fetch with replies", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		c := prepDocumentComments(t, db, 1, nil)[0]
		c.Replies = prepDocumentCommentReplies(t, db, 2, func(_ int, r *comment.Reply) {
			r.CommentID = c.ID
			r.OrganizationID = c.OrganizationID
		})

		res, err := db.FetchDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID)
		assert.NoError(t, err)
		testutil.AssertFilterEqual(t, &c, res)
	})
}

func Test_agent_FetchDocumentCommentReply(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentCommentReply(context.Background(), xid.New(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// success
	r := prepDocumentCommentReplies(t, db, 1, nil)[0]

	res, err = db.FetchDocumentCommentReply(context.Background(), r.ID, r.CommentID, r.OrganizationID)
	assert.NoError(t, err)
	testutil.AssertFilterEqual(t, &r, res)
}

func Test_agent_UpdateDocumentComment(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comment          comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Comment:          c,
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			c := prepDocumentComments(t, db, 1, nil)[0]
			c.Content = comment.Content{
				"type": "paragraph",
				"text": "Updated comment",
			}
			c.Resolved = true
			c.ResolvedBy = null.StringFrom(users[0])
			c.AnchorBlockID = null.StringFrom("block-2")
			c.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			c.Replies = []comment.Reply{}

			return tcase{
				Comment: c,
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

			err := db.UpdateDocumentComment(ctx, c.Comment)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentComment(context.Background(), c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Comment, res)
		})
	}
}

func Test_agent_UpdateDocumentCommentReply(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Reply            comment.Reply
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Reply:            r,
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]
			r.Content = comment.Content{
				"type": "paragraph",
				"text": "Updated reply",
			}
			r.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))

			return tcase{
				Reply: r,
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

			err := db.UpdateDocumentCommentReply(ctx, c.Reply)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentCommentReply(context.Background(), c.Reply.ID, c.Reply.CommentID, c.Reply.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Reply, res)
		})
	}
}

func Test_agent_DeleteDocumentComment(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comment          comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Comment:          c,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]

			return tcase{
				Comment: c,
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

			err := db.DeleteDocumentComment(ctx, c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentComment(context.Background(), c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_DeleteDocumentCommentReply(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Reply            comment.Reply
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Reply:            r,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]

			return tcase{
				Reply: r,
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

			err := db.DeleteDocumentCommentReply(ctx, c.Reply.ID, c.Reply.CommentID, c.Reply.OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentCommentReply(context.Background(), c.Reply.ID, c.Reply.CommentID, c.Reply.OrganizationID)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_ReplaceDocumentComment(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comment          comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				Comment:          c,
				Err:              assert.AnError,
			}
		},
		"Successful replace": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			c := prepDocumentComments(t, db, 1, nil)[0]
			c.Content = comment.Content{
				"type": "paragraph",
				"text": "Promoted reply content",
			}
			c.UserID = null.StringFrom(users[0])
			c.AnchorBlockID = null.StringFrom("block-3")
			c.CreatedAt = c.CreatedAt.Add(time.Hour)
			c.UpdatedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			c.Replies = []comment.Reply{}

			return tcase{
				Comment: c,
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

			err := db.ReplaceDocumentComment(ctx, c.Comment)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentComment(context.Background(), c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Comment, res)
		})
	}
}

func Test_agent_FetchDocumentCommentsByBranchID(t *testing.T) {
	t.Run("Error returned by the comments query", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		res, err := db.FetchDocumentCommentsByBranchID(ctx, xid.New(), "org-id")
		require.Error(t, err)
		assert.Nil(t, res)
	})

	t.Run("Error returned by the replies query", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)

		id := xid.New()

		mock.ExpectQuery("SELECT .* FROM document_comments").
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id.String()))
		mock.ExpectQuery("SELECT .* FROM document_comment_replies").
			WillReturnError(assert.AnError)

		res, err := a.FetchDocumentCommentsByBranchID(context.Background(), xid.New(), "org-id")
		assert.Equal(t, assert.AnError, err)
		assert.Nil(t, res)
	})

	t.Run("Successful fetch", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		// success - no comments
		res, err := db.FetchDocumentCommentsByBranchID(context.Background(), xid.New(), "non-existent-org-id")
		require.NoError(t, err)
		assert.Empty(t, res)

		// success - newest comment first, replies in creation order and
		// attached to the comment they belong to rather than the first
		// one; a comment with no replies carries an empty slice, not
		// nil, matching FetchDocumentComment
		comments := prepDocumentComments(t, db, 3, nil)
		comments[0].Replies = prepDocumentCommentReplies(t, db, 2, func(_ int, r *comment.Reply) {
			r.CommentID = comments[0].ID
			r.OrganizationID = comments[0].OrganizationID
		})
		comments[1].Replies = prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
			r.CommentID = comments[1].ID
			r.OrganizationID = comments[1].OrganizationID
		})
		comments[2].Replies = make([]comment.Reply, 0)

		res, err = db.FetchDocumentCommentsByBranchID(context.Background(), comments[0].BranchID, comments[0].OrganizationID)
		assert.NoError(t, err)
		testutil.AssertFilterEqual(t, comments, res)
	})
}

func Test_agent_DeleteDocumentCommentsByBranchID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comments         []comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			comments := prepDocumentComments(t, db, 1, nil)

			return tcase{
				CancelledContext: true,
				Comments:         comments,
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			return tcase{
				Comments: prepDocumentComments(t, db, 2, nil),
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

			err := db.DeleteDocumentCommentsByBranchID(ctx, c.Comments[0].BranchID, c.Comments[0].OrganizationID)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentCommentsByBranchID(context.Background(), c.Comments[0].BranchID, c.Comments[0].OrganizationID)
			require.NoError(t, err)
			assert.Empty(t, res)
		})
	}
}
