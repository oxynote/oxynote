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
		return i.interpretDocumentNewCommentNotification(ctx, n)
	case notification.NotificationDocumentNewCommentReply:
		return i.interpretDocumentNewCommentReplyNotification(ctx, n)
	}

	return nil, ErrUnknownNotificationCode
}

// interpretDocumentReviewRequestNotification interprets a document review request notification.
func (i *Interpreter) interpretDocumentReviewRequestNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	branchID, ok := metaBranchID(n)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, err := i.db.FetchDocumentByBranchID(ctx, branchID, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	orgSlug, err := i.db.FetchOrganizationSlug(ctx, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s-%s", i.appURL, orgSlug, slug.Make(doc.DocumentName), doc.ID.String())

	return &Message{
		Text: fmt.Sprintf(
			"Your review was requested on the %s branch of %s",
			doc.BranchName,
			i.fm.Link(url, doc.DocumentName),
		),
	}, nil
}

// interpretDocumentHookTriggeredNotification interprets a document hook triggered notification.
func (i *Interpreter) interpretDocumentHookTriggeredNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	tp, ok := n.Metadata["type"].(hook.Type)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	blockID, ok := n.Metadata["blockId"].(null.String)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	branchID, ok := metaBranchID(n)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, err := i.db.FetchDocumentByBranchID(ctx, branchID, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	orgSlug, err := i.db.FetchOrganizationSlug(ctx, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s-%s", i.appURL, orgSlug, slug.Make(doc.DocumentName), doc.ID.String())
	if blockID.Valid {
		url = fmt.Sprintf("%s#%s", url, blockID.String)
	}

	subject := doc.DocumentName
	if blockID.Valid {
		subject = "a block in " + doc.DocumentName
	}

	return &Message{
		Text: fmt.Sprintf(
			"%s may be outdated — %s",
			i.fm.Link(url, subject),
			tp.HumanizedString(),
		),
	}, nil
}

// interpretDocumentNewCommentNotification interprets a document new comment notification.
func (i *Interpreter) interpretDocumentNewCommentNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	userID, ok := n.Metadata["userId"].(string)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	anchorBlockID, ok := n.Metadata["anchorBlockId"].(null.String)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	branchID, ok := metaBranchID(n)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, err := i.db.FetchDocumentByBranchID(ctx, branchID, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	name, err := i.db.FetchUserName(ctx, userID)
	if err != nil {
		return nil, err
	}

	orgSlug, err := i.db.FetchOrganizationSlug(ctx, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s-%s", i.appURL, orgSlug, slug.Make(doc.DocumentName), doc.ID.String())
	if anchorBlockID.Valid {
		url = fmt.Sprintf("%s#%s", url, anchorBlockID.String)
	}

	return &Message{
		Text: fmt.Sprintf(
			"%s left a comment on %s",
			name,
			i.fm.Link(url, doc.DocumentName),
		),
	}, nil
}

// interpretDocumentNewCommentReplyNotification interprets a document new comment reply notification.
func (i *Interpreter) interpretDocumentNewCommentReplyNotification(ctx context.Context, n notification.Notification) (*Message, error) {
	userID, ok := n.Metadata["userId"].(string)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	anchorBlockID, ok := n.Metadata["anchorBlockId"].(null.String)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	branchID, ok := metaBranchID(n)
	if !ok {
		return nil, ErrInvalidNotificationMetadata
	}

	doc, err := i.db.FetchDocumentByBranchID(ctx, branchID, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	name, err := i.db.FetchUserName(ctx, userID)
	if err != nil {
		return nil, err
	}

	orgSlug, err := i.db.FetchOrganizationSlug(ctx, n.OrganizationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s-%s", i.appURL, orgSlug, slug.Make(doc.DocumentName), doc.ID.String())
	if anchorBlockID.Valid {
		url = fmt.Sprintf("%s#%s", url, anchorBlockID.String)
	}

	return &Message{
		Text: fmt.Sprintf(
			"%s replied to a comment on %s",
			name,
			i.fm.Link(url, doc.DocumentName),
		),
	}, nil
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
