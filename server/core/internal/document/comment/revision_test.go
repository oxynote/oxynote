package comment

import (
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
)

func Test_NewRevision(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)

	cc := map[string]struct {
		UpdatedAt null.Time
		Result    time.Time
	}{
		"New comment": {
			Result: created,
		},
		"Edited comment": {
			UpdatedAt: null.TimeFrom(updated),
			Result:    updated,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			cmt := stubComment()
			cmt.CreatedAt = created
			cmt.UpdatedAt = c.UpdatedAt

			rev := NewRevision(cmt)
			assert.False(t, rev.ID.IsZero())
			assert.Equal(t, cmt.ID, rev.CommentID)
			assert.Equal(t, cmt.OrganizationID, rev.OrganizationID)
			assert.Equal(t, cmt.Content, rev.Content)
			assert.Equal(t, cmt.UserID, rev.UserID)
			assert.Equal(t, c.Result, rev.CreatedAt)
		})
	}
}

func Test_NewReplyRevision(t *testing.T) {
	t.Parallel()

	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)

	cc := map[string]struct {
		UpdatedAt null.Time
		Result    time.Time
	}{
		"New reply": {
			Result: created,
		},
		"Edited reply": {
			UpdatedAt: null.TimeFrom(updated),
			Result:    updated,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			r := Reply{
				ID:             xid.New(),
				CommentID:      xid.New(),
				OrganizationID: "org-1",
				UserID:         null.StringFrom("user-1"),
				Content:        Content{"text": "reply"},
				CreatedAt:      created,
				UpdatedAt:      c.UpdatedAt,
			}

			rev := NewReplyRevision(r)
			assert.False(t, rev.ID.IsZero())
			assert.Equal(t, r.ID, rev.ReplyID)
			assert.Equal(t, r.OrganizationID, rev.OrganizationID)
			assert.Equal(t, r.Content, rev.Content)
			assert.Equal(t, r.UserID, rev.UserID)
			assert.Equal(t, c.Result, rev.CreatedAt)
		})
	}
}
