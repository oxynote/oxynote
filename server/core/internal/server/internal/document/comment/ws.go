package comment

import (
	"context"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// _publishTimeout bounds each WebSocket publish triggered by a domain
// callback.
const _publishTimeout = 5 * time.Second

// ChangeType represents the type of comment change.
type ChangeType string

const (
	// ChangeTypeCreated indicates a comment was created.
	ChangeTypeCreated ChangeType = "created"

	// ChangeTypeUpdated indicates a comment was updated.
	ChangeTypeUpdated ChangeType = "updated"

	// ChangeTypeDeleted indicates a comment was deleted.
	ChangeTypeDeleted ChangeType = "deleted"
)

// ChangeMessage represents a comment change message.
type ChangeMessage struct {
	// Type is the type of comment change.
	Type ChangeType `json:"type"`

	// CommentID is the id of the comment that has changed.
	CommentID xid.ID `json:"commentId"`
}

// BindCommentsChange binds a comment change event to the given topic.
func (h *Handler) BindCommentsChange(tpc wsserver.Topic) {
	h.comments.changeCallback = func(organizationID string, documentId xid.ID, msg ChangeMessage) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, msg, func(ctx context.Context, rawTopic string) bool {
			if wsserver.TopicParamFromContext(ctx, "documentId") != documentId.String() {
				return false
			}

			return auth.FilterOrganization(organizationID)(ctx, rawTopic)
		})
	}
}
