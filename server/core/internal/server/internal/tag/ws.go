package tag

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

// TreeChangeMessage represents a change in the tree of tags. The tree is
// small and always fetched whole, so the message carries no payload: it
// tells a subscriber to refetch.
type TreeChangeMessage struct{}

// BranchTagsChangeMessage represents a change in the tags one branch of a
// document carries. It names the branch so a subscriber showing another
// branch of the same document can leave its own list alone.
type BranchTagsChangeMessage struct {
	// BranchID is the branch whose tags changed.
	BranchID xid.ID `json:"branchId"`
}

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

// BindBranchTagsChange binds a branch tags change event to the given
// topic, which is scoped to one document.
func (h *Handler) BindBranchTagsChange(tpc wsserver.Topic) {
	h.branchTags.changeCallback = func(organizationID string, documentID, branchID xid.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(
			ctx,
			BranchTagsChangeMessage{BranchID: branchID},
			auth.FilterOrganizationDocument(organizationID, documentID),
		)
	}
}

// NotifyTreeChange announces a change of the tag tree to every subscriber
// in the organization. Safe to call before BindTreeChange has been invoked
// — no subscribers means it is a no-op.
func (h *Handler) NotifyTreeChange(organizationID string) {
	if h.tree.changeCallback == nil {
		return
	}

	h.tree.changeCallback(organizationID)
}

// NotifyBranchTagsChange announces that the tags a document's branch
// carries changed to every subscriber of that document in the
// organization. Safe to call before BindBranchTagsChange has been invoked
// — no subscribers means it is a no-op.
func (h *Handler) NotifyBranchTagsChange(organizationID string, documentID, branchID xid.ID) {
	if h.branchTags.changeCallback == nil {
		return
	}

	h.branchTags.changeCallback(organizationID, documentID, branchID)
}

// notifyUserTreeChange announces a change only one user's tree carries, so
// their other sessions refetch and nobody else is disturbed.
func (h *Handler) notifyUserTreeChange(organizationID, userID string) {
	if h.tree.userChangeCallback == nil {
		return
	}

	h.tree.userChangeCallback(organizationID, userID)
}
