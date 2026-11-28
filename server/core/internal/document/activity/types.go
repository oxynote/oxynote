package activity

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
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

// NewDocumentCreated creates the entry of a new document, on its default
// branch.
func NewDocumentCreated(actor Actor, doc document.Document) Entry {
	return newBranchEntry(actor, doc, TypeDocumentCreated, Target{}, Metadata{})
}

// NewDocumentDuplicated creates the entry of a document duplicated from
// another, on the new document's default branch.
func NewDocumentDuplicated(actor Actor, doc document.Document, sourceID xid.ID) Entry {
	return newBranchEntry(
		actor,
		doc,
		TypeDocumentDuplicated,
		NewTarget(TargetKindDocument, sourceID.String()),
		Metadata{},
	)
}

// NewBranchForked creates the entries of a fork, one on each branch, each
// pointing at the other.
func NewBranchForked(actor Actor, source, fork document.Document) []Entry {
	return newBranchPair(actor, TypeBranchForked, source, fork)
}

// NewBranchMerged creates the entries of a merge, one on each branch, each
// pointing at the other.
func NewBranchMerged(actor Actor, source, target document.Document) []Entry {
	return newBranchPair(actor, TypeBranchMerged, source, target)
}

// NewBranchUpdated creates an entry for each branch setting the update
// changed: its name and its protection.
func NewBranchUpdated(actor Actor, before, after document.Document) []Entry {
	var entries []Entry

	if before.BranchName != after.BranchName {
		entries = append(entries, newBranchEntry(actor, after, TypeBranchRenamed, Target{}, Metadata{
			MetaKeyFrom: before.BranchName,
			MetaKeyTo:   after.BranchName,
		}))
	}

	if before.Protected != after.Protected {
		tp := TypeBranchUnprotected
		if after.Protected {
			tp = TypeBranchProtected
		}

		entries = append(entries, newBranchEntry(actor, after, tp, Target{}, Metadata{}))
	}

	return entries
}

// NewReviewRequested creates the entry of a reviewer requested on the
// branch. A request clears the reviewer's approval, and the entry says so
// when there was one.
func NewReviewRequested(actor Actor, doc document.Document, reviewerID string, approvalReset bool) Entry {
	md := Metadata{}
	if approvalReset {
		md[MetaKeyApprovalReset] = true
	}

	return newBranchEntry(actor, doc, TypeReviewRequested, NewTarget(TargetKindUser, reviewerID), md)
}

// NewReviewWithdrawn creates the entry of a reviewer removed from the
// branch.
func NewReviewWithdrawn(actor Actor, doc document.Document, reviewerID string) Entry {
	return newBranchEntry(actor, doc, TypeReviewWithdrawn, NewTarget(TargetKindUser, reviewerID), Metadata{})
}

// NewReviewEntry creates the entry of a review given or taken back on the
// branch.
func NewReviewEntry(actor Actor, tp Type, doc document.Document) Entry {
	return newBranchEntry(actor, doc, tp, Target{}, Metadata{})
}

// NewCommentEntry creates the entry of a change to a comment, pointing at
// the revision of its text the change left.
func NewCommentEntry(actor Actor, tp Type, c comment.Comment, rev comment.Revision) Entry {
	return newEntry(
		actor,
		c.OrganizationID,
		c.DocumentID,
		c.BranchID,
		tp,
		NewTarget(TargetKindCommentRevision, rev.ID.String()),
		Metadata{MetaKeyCommentID: c.ID.String()},
		timeutil.Now(),
	)
}

// NewReplyEntry creates the entry of a change to a reply, pointing at the
// revision of its text the change left.
func NewReplyEntry(actor Actor, tp Type, c comment.Comment, rev comment.ReplyRevision) Entry {
	return newEntry(
		actor,
		c.OrganizationID,
		c.DocumentID,
		c.BranchID,
		tp,
		NewTarget(TargetKindReplyRevision, rev.ID.String()),
		Metadata{
			MetaKeyCommentID: c.ID.String(),
			MetaKeyReplyID:   rev.ReplyID.String(),
		},
		timeutil.Now(),
	)
}

// NewTagEntry creates the entry of a tag assigned to or unassigned from
// the branch. The tag's name is kept, so the entry reads after the tag is
// deleted.
func NewTagEntry(actor Actor, tp Type, doc document.Document, t tag.Tag) Entry {
	return newBranchEntry(
		actor,
		doc,
		tp,
		NewTarget(TargetKindTag, t.ID.String()),
		Metadata{MetaKeyTagName: t.TagName},
	)
}

// NewHookEntry creates the entry of a change to a hook on its branch. The
// hook's type is kept, so the entry reads after the hook is deleted.
func NewHookEntry(actor Actor, tp Type, h hook.Hook) Entry {
	return newEntry(
		actor,
		h.OrganizationID.String,
		h.DocumentID.V,
		h.BranchID.V,
		tp,
		NewTarget(TargetKindHook, h.ID.String()),
		Metadata{MetaKeyHookType: h.Type},
		timeutil.Now(),
	)
}

// newBranchEntry creates an entry on the document's branch, made now.
func newBranchEntry(actor Actor, doc document.Document, tp Type, target Target, md Metadata) Entry {
	return newEntry(actor, doc.OrganizationID, doc.ID, doc.BranchID, tp, target, md, timeutil.Now())
}

// newBranchPair creates the two entries of a change between two branches
// of a document, one on each branch, each pointing at the other.
func newBranchPair(actor Actor, tp Type, from, to document.Document) []Entry {
	md := Metadata{
		MetaKeyFromBranchID: from.BranchID.String(),
		MetaKeyToBranchID:   to.BranchID.String(),
	}

	return []Entry{
		newBranchEntry(actor, from, tp, NewTarget(TargetKindBranch, to.BranchID.String()), md),
		newBranchEntry(actor, to, tp, NewTarget(TargetKindBranch, from.BranchID.String()), md),
	}
}
