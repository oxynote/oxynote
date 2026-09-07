package hook

import (
	"context"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// _publishTimeout bounds each WebSocket publish triggered by a domain
// callback.
const _publishTimeout = 5 * time.Second

// HooksChangeMessage represents a change in the hooks one branch of a
// document carries. It names the branch so a subscriber showing another
// branch of the same document can leave its own list alone.
type HooksChangeMessage struct {
	// BranchID is the branch whose hooks changed.
	BranchID xid.ID `json:"branchId"`
}

// BindHooksChange binds a hooks change event to the given topic, which
// is scoped to one document.
func (h *Handler) BindHooksChange(tpc wsserver.Topic) {
	h.hooks.changeCallback = func(organizationID string, documentID, branchID xid.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(
			ctx,
			HooksChangeMessage{BranchID: branchID},
			auth.FilterOrganizationDocument(organizationID, documentID),
		)
	}
}

// NotifyHooksChange announces that the hooks on a document's branch
// changed to every subscriber of that document in the organization. Safe
// to call before BindHooksChange has been invoked: no subscribers means
// it is a no-op. A hook whose document or branch is gone has no editor
// to redraw, so a null id is a no-op too.
func (h *Handler) NotifyHooksChange(organizationID string, documentID, branchID null.Value[xid.ID]) {
	if h.hooks.changeCallback == nil || !documentID.Valid || !branchID.Valid {
		return
	}

	h.hooks.changeCallback(organizationID, documentID.V, branchID.V)
}
