package activity

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// All available Category constants.
const (
	CategoryDocument Category = "document"
	CategoryContent  Category = "content"
	CategoryBranch   Category = "branch"
	CategoryReview   Category = "review"
	CategoryComment  Category = "comment"
	CategoryHook     Category = "hook"
	CategoryTag      Category = "tag"
)

// All available Type constants.
const (
	TypeDocumentCreated    Type = "document.created"
	TypeDocumentDuplicated Type = "document.duplicated"
	TypeDocumentRenamed    Type = "document.renamed"

	TypeContentEdited Type = "content.edited"

	TypeBranchForked      Type = "branch.forked"
	TypeBranchRenamed     Type = "branch.renamed"
	TypeBranchProtected   Type = "branch.protected"
	TypeBranchUnprotected Type = "branch.unprotected"
	TypeBranchMerged      Type = "branch.merged"

	TypeReviewRequested  Type = "review.requested"
	TypeReviewWithdrawn  Type = "review.withdrawn"
	TypeReviewApproved   Type = "review.approved"
	TypeReviewUnapproved Type = "review.unapproved"

	TypeCommentCreated      Type = "comment.created"
	TypeCommentEdited       Type = "comment.edited"
	TypeCommentResolved     Type = "comment.resolved"
	TypeCommentReopened     Type = "comment.reopened"
	TypeCommentDeleted      Type = "comment.deleted"
	TypeCommentReplied      Type = "comment.replied"
	TypeCommentReplyEdited  Type = "comment.reply-edited"
	TypeCommentReplyDeleted Type = "comment.reply-deleted"

	TypeHookCreated        Type = "hook.created"
	TypeHookUpdated        Type = "hook.updated"
	TypeHookDeleted        Type = "hook.deleted"
	TypeHookReset          Type = "hook.reset"
	TypeHookTriggered      Type = "hook.triggered"
	TypeHookNeedsAttention Type = "hook.needs-attention"

	TypeTagAssigned   Type = "tag.assigned"
	TypeTagUnassigned Type = "tag.unassigned"
)

// ErrInvalidCategory is returned when a query names an unknown category.
var ErrInvalidCategory = errutil.New(http.StatusBadRequest, "activity.invalid_category", "invalid category")

// Category specifies a group of entry types.
type Category string

// Validate reports whether the category is a known one.
func (c Category) Validate() error {
	switch c {
	case CategoryDocument,
		CategoryContent,
		CategoryBranch,
		CategoryReview,
		CategoryComment,
		CategoryHook,
		CategoryTag:
		return nil
	default:
		return ErrInvalidCategory
	}
}

// MarshalText returns the category as text.
func (c Category) MarshalText() ([]byte, error) {
	return []byte(c), nil
}

// UnmarshalText parses a category, refusing an unknown one so a bad value
// is reported where it is decoded.
func (c *Category) UnmarshalText(text []byte) error {
	v := Category(strings.ToLower(strings.TrimSpace(string(text))))
	if err := v.Validate(); err != nil {
		return fmt.Errorf("unknown category %q", text)
	}

	*c = v

	return nil
}

// Type specifies what changed.
type Type string

// Category returns the group the type is filtered by.
func (t Type) Category() Category {
	switch t {
	case TypeDocumentCreated,
		TypeDocumentDuplicated,
		TypeDocumentRenamed:
		return CategoryDocument
	case TypeContentEdited:
		return CategoryContent
	case TypeBranchForked,
		TypeBranchRenamed,
		TypeBranchProtected,
		TypeBranchUnprotected,
		TypeBranchMerged:
		return CategoryBranch
	case TypeReviewRequested,
		TypeReviewWithdrawn,
		TypeReviewApproved,
		TypeReviewUnapproved:
		return CategoryReview
	case TypeCommentCreated,
		TypeCommentEdited,
		TypeCommentResolved,
		TypeCommentReopened,
		TypeCommentDeleted,
		TypeCommentReplied,
		TypeCommentReplyEdited,
		TypeCommentReplyDeleted:
		return CategoryComment
	case TypeHookCreated,
		TypeHookUpdated,
		TypeHookDeleted,
		TypeHookReset,
		TypeHookTriggered,
		TypeHookNeedsAttention:
		return CategoryHook
	case TypeTagAssigned, TypeTagUnassigned:
		return CategoryTag
	}

	return ""
}

// NewContentEdited creates the entry of an editor's edits to the branch,
// starting at the given time.
func NewContentEdited(actor Actor, doc document.Document, at time.Time) Entry {
	return newEntry(
		actor,
		doc.OrganizationID,
		doc.ID,
		doc.BranchID,
		TypeContentEdited,
		Target{},
		Metadata{},
		at,
	)
}
