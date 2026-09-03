package tag

import (
	"context"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
)

// _publishTimeout bounds each WebSocket publish triggered by a domain
// callback.
const _publishTimeout = 5 * time.Second

// TreeChangeMessage represents a change in the tree of tags. The tree is
// small and always fetched whole, so the message carries no payload: it
// tells a subscriber to refetch.
type TreeChangeMessage struct{}

// BindTreeChange binds a tag tree change event to the given topic.
func (h *Handler) BindTreeChange(tpc wsserver.Topic) {
	h.tree.changeCallback = func(organizationID string) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, TreeChangeMessage{}, auth.FilterOrganization(organizationID))
	}
	h.tree.userChangeCallback = func(organizationID, userID string) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, TreeChangeMessage{}, auth.FilterUser(organizationID, userID))
	}
}

// notifyTreeChange announces a change of the tag tree to every subscriber
// in the organization. Safe to call before BindTreeChange has been invoked
// — no subscribers means it is a no-op.
func (h *Handler) notifyTreeChange(organizationID string) {
	if h.tree.changeCallback == nil {
		return
	}

	h.tree.changeCallback(organizationID)
}

// notifyUserTreeChange announces a change only one user's tree carries, so
// their other sessions refetch and nobody else is disturbed.
func (h *Handler) notifyUserTreeChange(organizationID, userID string) {
	if h.tree.userChangeCallback == nil {
		return
	}

	h.tree.userChangeCallback(organizationID, userID)
}
