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

func Test_agent_recordDocumentCommentReplyRevision(t *testing.T) {
	type tcase struct {
		Reply comment.Reply
		// Texts lists the reply's revisions afterwards, oldest first.
		Texts []comment.Content
		Err   error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Non-existent reply": func(_ *testing.T, _ *DB) tcase {
			return tcase{
				Reply: comment.Reply{
					ID:             xid.New(),
					OrganizationID: "non-existent-org-id",
					Content:        comment.Content{"text": "hello"},
					CreatedAt:      timeutil.Now().Truncate(time.Microsecond),
				},
				Err: assert.AnError,
			}
		},
		"Successful insert": func(t *testing.T, db *DB) tcase {
			c := prepDocumentCommentReplies(t, db, 1, nil)[0]
			first := c.Content

			c.Content = comment.Content{"text": "edited"}
			c.UpdatedAt = null.TimeFrom(c.CreatedAt.Add(time.Minute))

			return tcase{
				Reply: c,
				Texts: []comment.Content{first, c.Content},
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			err := sqlutil.WrapTx(context.Background(), db.sql, func(tx *sqlx.Tx) error {
				return db.recordDocumentCommentReplyRevision(context.Background(), tx, c.Reply)
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
				`SELECT content FROM document_comment_reply_revisions
				WHERE fk_document_comment_reply_id = $1
				ORDER BY created_at, id`,
				c.Reply.ID,
			))
			assert.Equal(t, c.Texts, texts)
		})
	}
}
