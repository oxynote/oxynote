// Package activity defines the entries of a branch's activity log.
package activity

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"github.com/guregu/null/v5"
	"github.com/rs/xid"
)

// Metadata keys used by the entries.
const (
	MetaKeyCommentID     = "commentId"
	MetaKeyReplyID       = "replyId"
	MetaKeyFrom          = "from"
	MetaKeyTo            = "to"
	MetaKeyFromBranchID  = "fromBranchId"
	MetaKeyToBranchID    = "toBranchId"
	MetaKeyTagName       = "tagName"
	MetaKeyHookType      = "hookType"
	MetaKeyApprovalReset = "approvalReset"
)

// Page sizes.
const (
	// _defaultLimit is the number of entries a page holds when the query
	// sets no limit.
	_defaultLimit = 50

	// _maxLimit caps the number of entries one page holds.
	_maxLimit = 100
)

// All available ActorKind constants.
const (
	ActorKindUser   ActorKind = "user"
	ActorKindMCP    ActorKind = "mcp"
	ActorKindSystem ActorKind = "system"
)

// All available TargetKind constants.
const (
	TargetKindDocument        TargetKind = "document"
	TargetKindBranch          TargetKind = "branch"
	TargetKindUser            TargetKind = "user"
	TargetKindCommentRevision TargetKind = "comment-revision"
	TargetKindReplyRevision   TargetKind = "reply-revision"
	TargetKindHook            TargetKind = "hook"
	TargetKindTag             TargetKind = "tag"
)

// ActorKind specifies who made a change.
type ActorKind string

// Actor is who made a change.
type Actor struct {
	// Kind specifies who made the change.
	Kind ActorKind `json:"kind" db:"actor_kind"`

	// UserID specifies the user, or the user an agent acted for. It is
	// null for the system and after the user is deleted.
	UserID null.String `json:"userId" db:"fk_actor_id"`

	// ClientID specifies the OAuth client of an MCP actor.
	ClientID null.String `json:"clientId" db:"actor_client_id"`
}

// UserActor returns the actor of a change the user made.
func UserActor(userID string) Actor {
	return Actor{
		Kind:   ActorKindUser,
		UserID: null.StringFrom(userID),
	}
}

// SystemActor returns the actor of a change no person made.
func SystemActor() Actor {
	return Actor{Kind: ActorKindSystem}
}

// TargetKind specifies what kind of entity an entry is about.
type TargetKind string

// Target is the entity an entry is about.
type Target struct {
	// Kind specifies what kind of entity it is. It is null when the
	// entry is about no entity.
	Kind null.Value[TargetKind] `json:"kind" db:"target_kind"`

	// ID specifies the entity.
	ID null.String `json:"id" db:"target_id"`
}

// NewTarget creates the target of the entity with the given kind and id.
func NewTarget(kind TargetKind, id string) Target {
	return Target{
		Kind: null.ValueFrom(kind),
		ID:   null.StringFrom(id),
	}
}

// Entry is a single change on a branch.
type Entry struct {
	// ID is the unique identifier of the entry.
	ID xid.ID `json:"id" db:"id"`

	// OrganizationID is the organization identifier of the entry.
	OrganizationID string `json:"-" db:"fk_organization_id"`

	// DocumentID specifies the document the change was made to.
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// BranchID specifies the branch whose log holds the entry.
	BranchID xid.ID `json:"branchId" db:"fk_branch_id"`

	// Actor specifies who made the change.
	Actor Actor `json:"actor" db:"actor"`

	// Type specifies what changed.
	Type Type `json:"type" db:"type"`

	// Category specifies the group the type is filtered by.
	Category Category `json:"category" db:"category"`

	// Target specifies the entity the change is about.
	Target Target `json:"target" db:"target"`

	// Metadata specifies facts about the change itself.
	Metadata Metadata `json:"metadata" db:"metadata"`

	// Subject specifies what the target points at, loaded on read: a
	// comment.Revision or a comment.ReplyRevision. It is nil for a kind
	// that loads nothing.
	Subject any `json:"subject,omitempty" db:"-"`

	// CreatedAt is when the change was made.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is when a content edit entry last grew. It equals
	// CreatedAt for every other type.
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

// newEntry creates an entry of the given type on the branch.
func newEntry(
	actor Actor,
	organizationID string,
	documentID xid.ID,
	branchID xid.ID,
	tp Type,
	target Target,
	md Metadata,
	at time.Time,
) Entry {
	return Entry{
		ID:             xid.NewWithTime(at),
		OrganizationID: organizationID,
		DocumentID:     documentID,
		BranchID:       branchID,
		Actor:          actor,
		Type:           tp,
		Category:       tp.Category(),
		Target:         target,
		Metadata:       md,
		CreatedAt:      at,
		UpdatedAt:      at,
	}
}

// Metadata is a collection of facts about a change.
type Metadata map[string]any

// Value transforms metadata into a database entry.
func (md Metadata) Value() (driver.Value, error) {
	// NOCOV: error case cannot happen since the entries only store
	// plain values.
	return json.Marshal(md)
}

// Scan transforms a database entry into metadata.
func (md *Metadata) Scan(src any) error {
	var pv []byte

	switch v := src.(type) {
	case []byte:
		pv = v
	case string:
		pv = []byte(v)
	default:
		return errors.New("invalid metadata type")
	}

	data := &Metadata{}
	if err := json.Unmarshal(pv, data); err != nil {
		return err
	}

	*md = *data

	return nil
}

// Query selects a page of a branch's entries.
type Query struct {
	// Categories specifies the categories to keep. Empty keeps every
	// category.
	Categories []Category `schema:"category"`

	// Before specifies the entry the page starts after. A nil id starts
	// at the newest entry.
	Before xid.ID `schema:"before"`

	// Limit specifies the number of entries the page holds, see PageSize.
	Limit uint64 `schema:"limit"`
}

// PageSize returns the number of entries the page holds: the default when
// the query sets no limit, and at most the maximum.
func (qr Query) PageSize() uint64 {
	if qr.Limit == 0 {
		return _defaultLimit
	}

	return min(qr.Limit, _maxLimit)
}

// Page is a page of a branch's entries, newest first.
type Page struct {
	// Entries specifies the entries on the page.
	Entries []Entry `json:"entries"`

	// Next specifies the cursor of the following page. It is null on the
	// last page.
	Next null.Value[xid.ID] `json:"next"`
}
