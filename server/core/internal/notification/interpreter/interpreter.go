package interpreter

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gosimple/slug"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

var (
	// ErrUnknownNotificationCode is returned when the notification code is unknown.
	ErrUnknownNotificationCode = errutil.New(http.StatusBadRequest, "notification.unknown_code", "unknown notification code")

	// ErrInvalidNotificationMetadata is returned when the notification metadata is invalid.
	ErrInvalidNotificationMetadata = errutil.New(http.StatusBadRequest, "notification.invalid_metadata", "invalid notification metadata")
)

const (
	// _commentVerb names what the commenter did in a new comment message.
	_commentVerb = "left a comment on"

	// _commentReplyVerb names what the commenter did in a comment reply
	// message.
	_commentReplyVerb = "replied to a comment on"
)

// Interpreter is the notification interpreter.
type Interpreter struct {
	db     DB
	fm     Formatter
	appURL string
}

// NewInterpreter creates a new notification interpreter.
func NewInterpreter(db DB, fm Formatter, appURL string) *Interpreter {
	return &Interpreter{
		db:     db,
		fm:     fm,
		appURL: appURL,
	}
}

// InterpretNotification interprets a notification and returns a human-readable message.
func (i *Interpreter) InterpretNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	switch n.Code {
	case notification.NotificationDocumentReviewRequest:
		return i.interpretDocumentReviewRequestNotification(ctx, n)
	case notification.NotificationDocumentHookTriggered:
		return i.interpretDocumentHookTriggeredNotification(ctx, n)
	case notification.NotificationDocumentNewComment:
		return i.interpretDocumentCommentNotification(ctx, n, _commentVerb)
	case notification.NotificationDocumentNewCommentReply:
		return i.interpretDocumentCommentNotification(ctx, n, _commentReplyVerb)
	}

	return nil, ErrUnknownNotificationCode
}

// interpretDocumentReviewRequestNotification interprets a document review request notification.
func (i *Interpreter) interpretDocumentReviewRequestNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	doc, orgSlug, err := i.resolveDocument(ctx, n)
	if err != nil {
		return nil, err
	}

	return &Message{
		Text: fmt.Sprintf(
			"Your review was requested on the %s branch of %s",
			doc.BranchName,
			i.fm.Link(i.documentURL(doc, orgSlug, null.String{}), doc.DocumentName),
		),
	}, nil
}

// interpretDocumentHookTriggeredNotification interprets a document hook triggered notification.
func (i *Interpreter) interpretDocumentHookTriggeredNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	tp, ok := metaHookType(n)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	blockID, ok := metaNullString(n, "blockId")
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, orgSlug, err := i.resolveDocument(ctx, n)
	if err != nil {
		return nil, err
	}

	subject := doc.DocumentName
	if blockID.Valid {
		subject = "a block in " + doc.DocumentName
	}

	return &Message{
		Text: fmt.Sprintf(
			"%s may be outdated — %s",
			i.fm.Link(i.documentURL(doc, orgSlug, blockID), subject),
			tp.HumanizedString(),
		),
	}, nil
}

// interpretDocumentCommentNotification interprets both comment
// notifications: a comment and a reply to one differ only in the verb naming
// what the commenter did.
func (i *Interpreter) interpretDocumentCommentNotification(
	ctx context.Context,
	n notification.Notification,
	verb string,
) (*Message, error) {
	userID, ok := n.Metadata["userId"].(string)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	anchorBlockID, ok := metaNullString(n, "anchorBlockId")
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, orgSlug, err := i.resolveDocument(ctx, n)
	if err != nil {
		return nil, err
	}

	name, err := i.db.FetchUserName(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &Message{
		Text: fmt.Sprintf(
			"%s %s %s",
			name,
			verb,
			i.fm.Link(i.documentURL(doc, orgSlug, anchorBlockID), doc.DocumentName),
		),
	}, nil
}

// resolveDocument reads the branch the notification points at and returns the
// document together with the slug of the organization owning it, which is
// what every message needs to address its reader.
func (i *Interpreter) resolveDocument(
	ctx context.Context,
	n notification.Notification,
) (*document.Document, string, error) {
	branchID, ok := metaBranchID(n)
	if !ok {
		return nil, "", ErrInvalidNotificationMetadata
	}

	doc, err := i.db.FetchDocumentByBranchID(ctx, branchID, n.OrganizationID)
	if err != nil {
		return nil, "", err
	}

	orgSlug, err := i.db.FetchOrganizationSlug(ctx, n.OrganizationID)
	if err != nil {
		return nil, "", err
	}

	return doc, orgSlug, nil
}

// documentURL builds the front-end URL of a document, anchored at a block
// when the notification names one.
func (i *Interpreter) documentURL(doc *document.Document, orgSlug string, anchor null.String) string {
	url := fmt.Sprintf("%s/%s/%s-%s", i.appURL, orgSlug, slug.Make(doc.DocumentName), doc.ID.String())
	if anchor.Valid {
		return fmt.Sprintf("%s#%s", url, anchor.String)
	}

	return url
}

// DB is the interface for database operations needed by the interpreter.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentByBranchID should fetch a document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchOrganizationSlug should fetch the slug for a given organization ID.
	FetchOrganizationSlug(ctx context.Context, organizationID string) (string, error)

	// FetchUserName should fetch the user's name by their user ID.
	FetchUserName(ctx context.Context, userID string) (string, error)
}

// Formatter is an interface that handles platform-specific message
// formatting.
type Formatter interface {
	// Link should render a link with the given URL and display text.
	Link(url, text string) string
}

// metaHookType extracts the hook type from notification metadata. Like every
// metadata reader here it accepts both the typed value the in-memory fan-out
// carries and the plain string a stored notification decodes into.
func metaHookType(n notification.Notification) (hook.Type, bool) {
	switch v := n.Metadata["type"].(type) {
	case hook.Type:
		return v, true
	case string:
		tp := hook.Type(v)
		if tp.Validate() != nil {
			return "", false
		}

		return tp, true
	}

	return "", false
}

// metaNullString extracts an optional string from notification metadata. A
// null.String survives the fan-out as itself, decodes from JSON as a string,
// and is absent altogether when it was never set.
func metaNullString(n notification.Notification, key string) (null.String, bool) {
	value, ok := n.Metadata[key]
	if !ok || value == nil {
		return null.String{}, true
	}

	switch v := value.(type) {
	case null.String:
		return v, true
	case string:
		return null.StringFrom(v), true
	}

	return null.String{}, false
}

// metaBranchID extracts the branch ID from notification metadata.
// The value is a typed xid.ID when the notification comes from the
// in-memory fan-out and a string after a JSON round-trip.
func metaBranchID(n notification.Notification) (xid.ID, bool) {
	switch v := n.Metadata["branchId"].(type) {
	case xid.ID:
		return v, true
	case string:
		id, err := xid.FromString(v)
		if err != nil {
			return xid.ID{}, false
		}

		return id, true
	}

	return xid.ID{}, false
}
