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
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
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
				"resolved_at":           c.ResolvedAt,
				"anchor_block_id":       c.AnchorBlockID,
				"content":               c.Content,
				"diff_deletion_context": c.DiffDeletionContext,
				"created_at":            c.CreatedAt,
				"updated_at":            c.UpdatedAt,
				"deleted_at":            c.DeletedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		// a live comment has a revision of its text. A deleted one has no
		// text to record.
		if c.Content != nil {
			require.NoError(t, sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				return db.recordDocumentCommentRevision(context.Background(), tx, c)
			}))
		}
	}

	return res
}

// fetchStoredDocumentComment reads a comment row as stored, deleted or not,
// without its replies.
func fetchStoredDocumentComment(t *testing.T, db *DB, id xid.ID) comment.Comment {
	t.Helper()

	q, args := db.selectDocumentComment(db.builder.Select()).
		Where(sq.Eq{"document_comments.id": id}).
		MustSql()

	var c comment.Comment
	require.NoError(t, sqlx.GetContext(context.Background(), db.sql, &c, q, args...))

	return c
}

// fetchStoredDocumentCommentReply reads a reply row as stored, deleted or
// not.
func fetchStoredDocumentCommentReply(t *testing.T, db *DB, id xid.ID) comment.Reply {
	t.Helper()

	q, args := db.selectDocumentCommentReply(db.builder.Select()).
		Where(sq.Eq{"document_comment_replies.id": id}).
		MustSql()

	var r comment.Reply
	require.NoError(t, sqlx.GetContext(context.Background(), db.sql, &r, q, args...))

	return r
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
				"deleted_at":             r.DeletedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)

		// a live reply has a revision of its text. A deleted one has no
		// text to record.
		if r.Content != nil {
			require.NoError(t, sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				return db.recordDocumentCommentReplyRevision(context.Background(), tx, r)
			}))
		}
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
	type tcase struct {
		Agent          *agent
		ID             xid.ID
		DocumentID     xid.ID
		OrganizationID string
		Result         *comment.Comment
		Err            error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Not found": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				ID:             xid.New(),
				DocumentID:     xid.New(),
				OrganizationID: "non-existent-org-id",
				Err:            sql.ErrNoRows,
			}
		},
		"Error returned by the replies query": func(t *testing.T, _ *DB) tcase {
			a, mock := prepMockDB(t)

			id := xid.New()

			mock.ExpectQuery("SELECT .* FROM document_comments").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id.String()))
			mock.ExpectQuery("SELECT .* FROM document_comment_replies").
				WillReturnError(assert.AnError)

			return tcase{
				Agent:          a,
				ID:             id,
				DocumentID:     xid.New(),
				OrganizationID: "org-id",
				Err:            assert.AnError,
			}
		},
		"Deleted comment without live replies": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
				c.Content = nil
				c.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			})[0]

			prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
				r.Content = nil
				r.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			})

			return tcase{
				ID:             c.ID,
				DocumentID:     c.DocumentID,
				OrganizationID: c.OrganizationID,
				Err:            sql.ErrNoRows,
			}
		},
		"Successful fetch of a deleted comment kept by a live reply": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
				c.Content = nil
				c.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			})[0]

			replies := prepDocumentCommentReplies(t, db, 2, func(i int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID

				if i == 0 {
					r.Content = nil
					r.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
				}
			})
			c.Replies = replies[1:]

			return tcase{
				ID:             c.ID,
				DocumentID:     c.DocumentID,
				OrganizationID: c.OrganizationID,
				Result:         &c,
			}
		},
		"Successful fetch with replies": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			c.Replies = prepDocumentCommentReplies(t, db, 2, func(_ int, r *comment.Reply) {
				r.CommentID = c.ID
				r.OrganizationID = c.OrganizationID
			})

			return tcase{
				ID:             c.ID,
				DocumentID:     c.DocumentID,
				OrganizationID: c.OrganizationID,
				Result:         &c,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ag := &db.agent

			if c.Agent != nil {
				ag = c.Agent
			}

			res, err := ag.FetchDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchDocumentCommentReply(t *testing.T) {
	db := prepTempDB(t)

	// error - not found
	res, err := db.FetchDocumentCommentReply(context.Background(), xid.New(), xid.New(), "non-existent-org-id")
	testutil.AssertEqualError(t, sql.ErrNoRows, err)
	assert.Nil(t, res)

	// error - deleted
	deleted := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
		r.Content = nil
		r.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
	})[0]

	res, err = db.FetchDocumentCommentReply(context.Background(), deleted.ID, deleted.CommentID, deleted.OrganizationID)
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
		"Deleted comment": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			require.NoError(t, db.DeleteDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID))

			c.Content = comment.Content{"text": "edited after the delete"}

			return tcase{
				Comment: c,
				Err:     errutil.ErrNotFound,
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
			c.ResolvedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
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

func Test_agent_UpdateDocumentCommentStatus(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comment          comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Comment:          prepDocumentComments(t, db, 1, nil)[0],
				Err:              assert.AnError,
			}
		},
		"Successful update": func(t *testing.T, db *DB) tcase {
			users := prepUsers(t, db, 1)
			c := prepDocumentComments(t, db, 1, nil)[0]
			c.Resolved = true
			c.ResolvedBy = null.StringFrom(users[0])
			c.ResolvedAt = null.TimeFrom(timeutil.Now().Truncate(time.Microsecond))
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

			err := db.UpdateDocumentCommentStatus(ctx, c.Comment)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			res, err := db.FetchDocumentComment(context.Background(), c.Comment.ID, c.Comment.DocumentID, c.Comment.OrganizationID)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, &c.Comment, res)

			// the text did not change, so only the revision from the
			// comment's creation exists.
			var revisions int

			require.NoError(t, sqlx.GetContext(
				context.Background(),
				db.sql,
				&revisions,
				`SELECT COUNT(*) FROM document_comment_revisions WHERE fk_document_comment_id = $1`,
				c.Comment.ID,
			))
			assert.Equal(t, 1, revisions)
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
		"Deleted reply": func(t *testing.T, db *DB) tcase {
			r := prepDocumentCommentReplies(t, db, 1, nil)[0]
			require.NoError(t, db.DeleteDocumentCommentReply(context.Background(), r.ID, r.CommentID, r.OrganizationID))

			r.Content = comment.Content{"text": "edited after the delete"}

			return tcase{
				Reply: r,
				Err:   errutil.ErrNotFound,
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

	// prepDeleted stores a live comment and returns it as deleted.
	prepDeleted := func(t *testing.T, db *DB) comment.Comment {
		t.Helper()

		c := prepDocumentComments(t, db, 1, nil)[0]
		c.Content = nil
		c.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Microsecond))

		return c
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Comment:          prepDeleted(t, db),
				Err:              assert.AnError,
			}
		},
		"Already deleted": func(t *testing.T, db *DB) tcase {
			c := prepDeleted(t, db)
			require.NoError(t, db.DeleteDocumentComment(context.Background(), c.ID, c.DocumentID, c.OrganizationID))

			return tcase{
				Comment: c,
				Err:     errutil.ErrNotFound,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			return tcase{
				Comment: prepDeleted(t, db),
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

			// the row stays, without its text, and the comment leaves
			// every read.
			// the delete stamps its own time.
			stored := fetchStoredDocumentComment(t, db, c.Comment.ID)
			assert.WithinDuration(t, timeutil.Now(), stored.DeletedAt.Time, time.Minute)
			c.Comment.DeletedAt = stored.DeletedAt
			testutil.AssertFilterEqual(t, c.Comment, stored)

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

	// prepDeleted stores a live reply and returns it as deleted.
	prepDeleted := func(t *testing.T, db *DB) comment.Reply {
		t.Helper()

		r := prepDocumentCommentReplies(t, db, 1, nil)[0]
		r.Content = nil
		r.DeletedAt = null.TimeFrom(timeutil.Now().Truncate(time.Microsecond))

		return r
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Reply:            prepDeleted(t, db),
				Err:              assert.AnError,
			}
		},
		"Already deleted": func(t *testing.T, db *DB) tcase {
			r := prepDeleted(t, db)
			require.NoError(t, db.DeleteDocumentCommentReply(context.Background(), r.ID, r.CommentID, r.OrganizationID))

			return tcase{
				Reply: r,
				Err:   errutil.ErrNotFound,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			return tcase{
				Reply: prepDeleted(t, db),
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

			// the delete stamps its own time.
			stored := fetchStoredDocumentCommentReply(t, db, c.Reply.ID)
			assert.WithinDuration(t, timeutil.Now(), stored.DeletedAt.Time, time.Minute)
			c.Reply.DeletedAt = stored.DeletedAt
			testutil.AssertFilterEqual(t, c.Reply, stored)

			res, err := db.FetchDocumentCommentReply(context.Background(), c.Reply.ID, c.Reply.CommentID, c.Reply.OrganizationID)
			testutil.AssertEqualError(t, sql.ErrNoRows, err)
			assert.Nil(t, res)
		})
	}
}

func Test_agent_FetchDocumentCommentsByBranchID(t *testing.T) {
	type tcase struct {
		Agent            *agent
		CancelledContext bool
		BranchID         xid.ID
		OrganizationID   string
		Resolved         bool
		Result           []comment.Comment
		Err              error
	}

	// prepBranchComments creates three unresolved comments of one branch,
	// the first two with replies, and one resolved comment of the same
	// branch.
	prepBranchComments := func(t *testing.T, db *DB) ([]comment.Comment, []comment.Comment) {
		t.Helper()

		comments := prepDocumentComments(t, db, 3, nil)
		resolved := prepDocumentComments(t, db, 1, func(_ int, c *comment.Comment) {
			c.BranchID = comments[0].BranchID
			c.DocumentID = comments[0].DocumentID
			c.OrganizationID = comments[0].OrganizationID
			c.Resolved = true
			c.ResolvedAt = null.TimeFrom(timeutil.Now().Truncate(time.Second))
			c.Replies = make([]comment.Reply, 0)
		})
		comments[0].Replies = prepDocumentCommentReplies(t, db, 2, func(_ int, r *comment.Reply) {
			r.CommentID = comments[0].ID
			r.OrganizationID = comments[0].OrganizationID
		})
		comments[1].Replies = prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
			r.CommentID = comments[1].ID
			r.OrganizationID = comments[1].OrganizationID
		})
		comments[2].Replies = make([]comment.Reply, 0)

		return comments, resolved
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Error returned by the comments query": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				CancelledContext: true,
				BranchID:         xid.New(),
				OrganizationID:   "org-id",
				Err:              assert.AnError,
			}
		},
		"Error returned by the replies query": func(t *testing.T, _ *DB) tcase {
			a, mock := prepMockDB(t)

			id := xid.New()

			mock.ExpectQuery("SELECT .* FROM document_comments").
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id.String()))
			mock.ExpectQuery("SELECT .* FROM document_comment_replies").
				WillReturnError(assert.AnError)

			return tcase{
				Agent:          a,
				BranchID:       xid.New(),
				OrganizationID: "org-id",
				Err:            assert.AnError,
			}
		},
		"Successful fetch without comments": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				BranchID:       xid.New(),
				OrganizationID: "non-existent-org-id",
				Result:         []comment.Comment{},
			}
		},
		"Successful fetch of unresolved comments": func(t *testing.T, db *DB) tcase {
			// newest comment first, replies in creation order and
			// attached to the comment they belong to rather than the first
			// one; a comment with no replies carries an empty slice, not
			// nil, matching FetchDocumentComment; a resolved comment of the
			// same branch is left out.
			comments, _ := prepBranchComments(t, db)

			return tcase{
				BranchID:       comments[0].BranchID,
				OrganizationID: comments[0].OrganizationID,
				Result:         comments,
			}
		},
		"Successful fetch of deleted threads": func(t *testing.T, db *DB) tcase {
			// a deleted comment stays listed only while it has a live
			// reply, and deleted replies are left out.
			deletedAt := null.TimeFrom(timeutil.Now().Truncate(time.Second))

			comments := prepDocumentComments(t, db, 4, func(i int, c *comment.Comment) {
				if i == 1 || i == 2 {
					c.Content = nil
					c.DeletedAt = deletedAt
				}
			})

			kept := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = comments[1].ID
				r.OrganizationID = comments[1].OrganizationID
			})

			prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = comments[3].ID
				r.OrganizationID = comments[3].OrganizationID
				r.Content = nil
				r.DeletedAt = deletedAt
			})

			comments[0].Replies = make([]comment.Reply, 0)
			comments[1].Replies = kept
			comments[3].Replies = make([]comment.Reply, 0)

			return tcase{
				BranchID:       comments[0].BranchID,
				OrganizationID: comments[0].OrganizationID,
				Result:         []comment.Comment{comments[0], comments[1], comments[3]},
			}
		},
		"Successful fetch of resolved comments": func(t *testing.T, db *DB) tcase {
			comments, resolved := prepBranchComments(t, db)

			return tcase{
				BranchID:       comments[0].BranchID,
				OrganizationID: comments[0].OrganizationID,
				Resolved:       true,
				Result:         resolved,
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

			ag := &db.agent

			if c.Agent != nil {
				ag = c.Agent
			}

			res, err := ag.FetchDocumentCommentsByBranchID(ctx, c.BranchID, c.OrganizationID, c.Resolved)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, res)
				return
			}

			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_DeleteDocumentCommentsByBranchID(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		Comments         []comment.Comment
		Replies          []comment.Reply
		Other            comment.Comment
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			return tcase{
				CancelledContext: true,
				Comments:         prepDocumentComments(t, db, 1, nil),
				Err:              assert.AnError,
			}
		},
		"Successful delete": func(t *testing.T, db *DB) tcase {
			comments := prepDocumentComments(t, db, 2, nil)
			replies := prepDocumentCommentReplies(t, db, 1, func(_ int, r *comment.Reply) {
				r.CommentID = comments[0].ID
				r.OrganizationID = comments[0].OrganizationID
			})

			return tcase{
				Comments: comments,
				Replies:  replies,
				Other:    prepDocumentComments(t, db, 1, nil)[0],
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

			res, err := db.FetchDocumentCommentsByBranchID(context.Background(), c.Comments[0].BranchID, c.Comments[0].OrganizationID, false)
			require.NoError(t, err)
			assert.Empty(t, res)

			// the delete stamps its own time, one for every row.
			deletedAt := fetchStoredDocumentComment(t, db, c.Comments[0].ID).DeletedAt
			assert.WithinDuration(t, timeutil.Now(), deletedAt.Time, time.Minute)

			for _, cmt := range c.Comments {
				cmt.Content = nil
				cmt.DeletedAt = deletedAt
				testutil.AssertFilterEqual(t, cmt, fetchStoredDocumentComment(t, db, cmt.ID))
			}

			for _, r := range c.Replies {
				r.Content = nil
				r.DeletedAt = deletedAt
				testutil.AssertFilterEqual(t, r, fetchStoredDocumentCommentReply(t, db, r.ID))
			}

			// another branch keeps its comments.
			testutil.AssertFilterEqual(t, c.Other, fetchStoredDocumentComment(t, db, c.Other.ID))
		})
	}
}
