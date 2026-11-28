package comment

import (
	"time"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

// Revision is the text of a comment as one write left it.
type Revision struct {
	// ID is the unique identifier of the revision.
	ID xid.ID `json:"id" db:"id"`

	// CommentID is the identifier of the comment the text belongs to.
	CommentID xid.ID `json:"commentId" db:"fk_document_comment_id"`

	// OrganizationID is the identifier of the organization the comment
	// belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// Content is the text of the comment.
	Content Content `json:"content" db:"content"`

	// UserID is the identifier of the user who wrote the text.
	UserID null.String `json:"userId" db:"fk_user_id"`

	// CreatedAt is the time of the write that left this text.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// NewRevision returns the revision of the comment's text as its latest
// write left it.
func NewRevision(c Comment) Revision {
	at := c.CreatedAt
	if c.UpdatedAt.Valid {
		at = c.UpdatedAt.Time
	}

	return Revision{
		ID:             xid.New(),
		CommentID:      c.ID,
		OrganizationID: c.OrganizationID,
		Content:        c.Content,
		UserID:         c.UserID,
		CreatedAt:      at,
	}
}

// ReplyRevision is the text of a reply as one write left it.
type ReplyRevision struct {
	// ID is the unique identifier of the revision.
	ID xid.ID `json:"id" db:"id"`

	// ReplyID is the identifier of the reply the text belongs to.
	ReplyID xid.ID `json:"replyId" db:"fk_document_comment_reply_id"`

	// OrganizationID is the identifier of the organization the reply
	// belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// Content is the text of the reply.
	Content Content `json:"content" db:"content"`

	// UserID is the identifier of the user who wrote the text.
	UserID null.String `json:"userId" db:"fk_user_id"`

	// CreatedAt is the time of the write that left this text.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

// NewReplyRevision returns the revision of the reply's text as its latest
// write left it.
func NewReplyRevision(r Reply) ReplyRevision {
	at := r.CreatedAt
	if r.UpdatedAt.Valid {
		at = r.UpdatedAt.Time
	}

	return ReplyRevision{
		ID:             xid.New(),
		ReplyID:        r.ID,
		OrganizationID: r.OrganizationID,
		Content:        r.Content,
		UserID:         r.UserID,
		CreatedAt:      at,
	}
}
