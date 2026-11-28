package db

import (
	"context"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_agent_recordDocumentCommentRevision(t *testing.T) {
	type tcase struct {
		Comment comment.Comment
		// Texts lists the comment's revisions afterwards, oldest first.
		Texts []comment.Content
		Err   error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent comment": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Comment: comment.Comment{
					ID:             xid.New(),
					OrganizationID: "non-existent-org-id",
					Content:        comment.Content{"text": "hello"},
					CreatedAt:      timeutil.Now().Truncate(time.Microsecond),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			c := prepDocumentComments(t, db, 1, nil)[0]
			first := c.Content

			c.Content = comment.Content{"text": "edited"}
			c.UpdatedAt = null.TimeFrom(c.CreatedAt.Add(time.Minute))

			return tcase{
				Comment: c,
				Texts:   []comment.Content{first, c.Content},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				return db.recordDocumentCommentRevision(context.Background(), tx, c.Comment)
			})
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			var texts []comment.Content

			require.NoError(t, sqlx.SelectContext(
				context.Background(),
				db.sql,
				&texts,
				`SELECT content FROM document_comment_revisions
				WHERE fk_document_comment_id = $1
				ORDER BY created_at, id`,
				c.Comment.ID,
			))
			assert.Equal(t, c.Texts, texts)
		})
	}
}

func Test_agent_fetchDocumentCommentRevisionsByIDs(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		IDs              []string
		OrganizationID   string
		Result           []comment.Revision
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Error returned by the select": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentRevision(t, db, prepDocumentComments(t, db, 1, nil)[0].ID)

			return tcase{
				CancelledContext: true,
				IDs:              []string{rev.ID.String()},
				OrganizationID:   rev.OrganizationID,
				Err:              assert.AnError,
			}
		},
		"No ids": func(t *testing.T, db *DB) tcase {
			return tcase{
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         []comment.Revision{},
			}
		},
		"Another organization": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentRevision(t, db, prepDocumentComments(t, db, 1, nil)[0].ID)

			return tcase{
				IDs:            []string{rev.ID.String()},
				OrganizationID: prepOrganizations(t, db, 1)[0],
				Result:         []comment.Revision{},
			}
		},
		"Successful fetch": func(t *testing.T, db *DB) tcase {
			rev := fetchFirstDocumentCommentRevision(t, db, prepDocumentComments(t, db, 1, nil)[0].ID)

			return tcase{
				IDs:            []string{rev.ID.String(), xid.New().String()},
				OrganizationID: rev.OrganizationID,
				Result:         []comment.Revision{rev},
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

			res, err := db.fetchDocumentCommentRevisionsByIDs(ctx, c.IDs, c.OrganizationID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}
