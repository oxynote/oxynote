// Package comment provides structures and methods for handling document comments and replies.
package comment

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// Comment represents a comment on a document.
type Comment struct {
	// ID is the unique identifier for the comment.
	ID xid.ID `json:"id" db:"id"`

	// DocumentID is the identifier for the document this comment belongs to.
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// OrganizationID is the identifier for the organization this comment belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// BranchID is the ID of the document branch on which this comment was created.
	BranchID xid.ID `json:"branchId" db:"fk_branch_id"`

	// AnchorBlockID is the optional block ID.
	AnchorBlockID null.String `json:"anchorBlockId" db:"anchor_block_id"`

	// UserID is the identifier for the user who created this comment.
	UserID null.String `json:"userId" db:"fk_user_id"`

	// Resolved indicates whether this comment has been resolved.
	Resolved bool `json:"resolved" db:"resolved"`

	// ResolvedBy is the identifier for the user who resolved this comment.
	ResolvedBy null.String `json:"resolvedBy" db:"fk_resolved_by"`

	// Content is the content of the comment.
	Content Content `json:"content" db:"content"`

	// DiffDeletionContext stores contextual information when comment is in deleted content diff.
	DiffDeletionContext null.Value[json.RawMessage] `json:"diffDeletionContext" db:"diff_deletion_context"`

	// CreatedAt is the timestamp when the comment was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when the comment was last updated.
	UpdatedAt null.Time `json:"updatedAt" db:"updated_at"`

	// Replies contains all replies to this comment (not included in database queries by default).
	Replies []Reply `json:"replies" db:"-"`
}

// Reply represents a reply to a comment.
type Reply struct {
	// ID is the unique identifier for the reply.
	ID xid.ID `json:"id" db:"id"`

	// CommentID is the identifier for the comment this reply belongs to.
	CommentID xid.ID `json:"commentId" db:"fk_document_comment_id"`

	// OrganizationID is the identifier for the organization this comment belongs to.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// UserID is the identifier for the user who created this reply.
	UserID null.String `json:"userId" db:"fk_user_id"`

	// Content is the content of the reply.
	Content Content `json:"content" db:"content"`

	// CreatedAt is the timestamp when the reply was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when the reply was last updated.
	UpdatedAt null.Time `json:"updatedAt" db:"updated_at"`
}

// Content represents the structured content of a comment or reply.
type Content map[string]any

// Value implements the driver.Valuer interface for database storage.
// It converts the Content to JSON for storage in the database.
func (c Content) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil //nolint:nilnil // a nil driver.Value stores SQL NULL
	}

	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for database retrieval.
// It converts JSON data from the database back to Content.
func (c *Content) Scan(value any) error {
	if value == nil {
		*c = nil
		return nil
	}

	var data []byte

	switch v := value.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into Content", value)
	}

	// unmarshal into a fresh map: json.Unmarshal merges into an existing
	// one, which would keep stale keys when scanning into a reused value.
	var nc Content

	if err := json.Unmarshal(data, &nc); err != nil {
		return err
	}

	*c = nc

	return nil
}

// NewComment creates a new comment instance with the provided input.
func NewComment(inp Input, documentID, branchID xid.ID, userID, organizationID string) Comment {
	return Comment{
		ID:                  xid.New(),
		DocumentID:          documentID,
		OrganizationID:      organizationID,
		BranchID:            branchID,
		AnchorBlockID:       inp.AnchorBlockID,
		UserID:              null.StringFrom(userID),
		Resolved:            false,
		Content:             inp.Content,
		DiffDeletionContext: inp.DiffDeletionContext,
		CreatedAt:           timeutil.Now(),
	}
}

// ApplyUpdate updates comment instance with the input data.
func (c Comment) ApplyUpdate(inp Input) Comment {
	nc := c
	nc.AnchorBlockID = inp.AnchorBlockID
	nc.Content = inp.Content
	nc.UpdatedAt = null.TimeFrom(timeutil.Now())

	return nc
}

// Resolve marks the comment as resolved.
func (c Comment) Resolve(resolvedBy string) Comment {
	nc := c
	nc.Resolved = true
	nc.ResolvedBy = null.StringFrom(resolvedBy)

	return nc
}

// Unresolve marks the comment as unresolved.
func (c Comment) Unresolve() Comment {
	nc := c
	nc.Resolved = false
	nc.ResolvedBy = null.String{}

	return nc
}

// Replace returns a new comment with content and author replaced by the given
// reply, which is removed from the head of the reply list.
func (c Comment) Replace(r Reply) Comment {
	nc := c
	nc.UserID = r.UserID
	nc.Content = r.Content
	nc.CreatedAt = r.CreatedAt
	nc.UpdatedAt = r.UpdatedAt
	nc.Replies = c.Replies[1:]

	return nc
}

// NewReply creates a new reply instance with the provided input.
func NewReply(inp ReplyInput, commentID xid.ID, userID, organizationID string) Reply {
	return Reply{
		ID:             xid.New(),
		CommentID:      commentID,
		OrganizationID: organizationID,
		UserID:         null.StringFrom(userID),
		Content:        inp.Content,
		CreatedAt:      timeutil.Now(),
	}
}

// ApplyUpdate updates reply instance with the input data.
func (r Reply) ApplyUpdate(inp ReplyInput) Reply {
	nr := r
	nr.Content = inp.Content
	nr.UpdatedAt = null.TimeFrom(timeutil.Now())

	return nr
}

// Input is the input structure for a comment.
type Input struct {
	// Content is the content of the comment.
	Content Content `json:"content"`

	// AnchorBlockID is the optional block ID to anchor the comment to
	// a specific part of the document. Used for scroll
	// positioning in the UI.
	AnchorBlockID null.String `json:"anchorBlockId"`

	// DiffDeletionContext stores contextual information when comment is in deleted content diff.
	DiffDeletionContext null.Value[json.RawMessage] `json:"diffDeletionContext"`

	// BranchID is the ID of the document branch on which the comment is to be created.
	BranchID xid.ID `json:"branchId"`
}

// ReplyInput is the input structure for a reply.
type ReplyInput struct {
	// Content is the content of the reply.
	Content Content `json:"content"`
}
