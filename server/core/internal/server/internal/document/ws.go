package document

import (
	"context"
	"time"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/wetsocks/wsserver"
	"github.com/rs/xid"
)

// _publishTimeout bounds each WebSocket publish triggered by a domain
// callback.
const _publishTimeout = 5 * time.Second

// TreeChangeMessage represents a change in the tree of a document.
type TreeChangeMessage struct {
	// ParentID is the id of the parent document that has changed.
	ParentID null.Value[xid.ID] `json:"parentId"`
}

// BindTreeChange binds a tree change event to the given topic.
func (h *Handler) BindTreeChange(tpc wsserver.Topic) {
	h.tree.changeCallback = func(organizationID string, parentId null.Value[xid.ID]) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, TreeChangeMessage{
			ParentID: parentId,
		}, auth.FilterOrganization(organizationID))
	}
}

// NotifyTreeChange publishes a tree-change event to all WebSocket
// subscribers in the org. Use from callers outside the HTTP handler
// (the AI assistant) that mutate the document tree directly. Safe to
// call before BindTreeChange has been invoked — no subscribers means
// it's a no-op.
func (h *Handler) NotifyTreeChange(organizationID string, parentID null.Value[xid.ID]) {
	if h.tree.changeCallback == nil {
		return
	}

	h.tree.changeCallback(organizationID, parentID)
}

// BranchMetadata represents the metadata of a document branch published over WebSocket.
type BranchMetadata struct {
	// BranchID is the unique identifier of the branch.
	BranchID xid.ID `json:"branchId"`

	// DocumentName is the display name of the document on this branch.
	DocumentName string `json:"documentName"`

	// Protected indicates whether the branch is protected.
	Protected bool `json:"protected"`

	// CreatedAt is the timestamp when the branch was created.
	CreatedAt time.Time `json:"createdAt"`

	// CreatedBy is the identifier of the user who created the branch.
	CreatedBy null.String `json:"createdBy"`

	// UpdatedAt is the timestamp when the branch was last updated.
	UpdatedAt time.Time `json:"updatedAt"`

	// LastUpdatedBy is the identifier of the user who last updated the branch.
	LastUpdatedBy null.String `json:"lastUpdatedBy"`
}

// BindReviewersChange binds a reviewers change event to the given topic.
func (h *Handler) BindReviewersChange(tpc wsserver.Topic) {
	h.reviewers.changeCallback = func(organizationID string, documentID xid.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, struct{}{}, func(ctx context.Context, rawTopic string) bool {
			if wsserver.TopicParamFromContext(ctx, "documentId") != documentID.String() {
				return false
			}

			return auth.FilterOrganization(organizationID)(ctx, rawTopic)
		})
	}
}

// BindMaintainersChange binds a maintainers change event to the given topic.
func (h *Handler) BindMaintainersChange(tpc wsserver.Topic) {
	h.maintainers.changeCallback = func(organizationID string, documentID xid.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer cancel()

		tpc.PublishMany(ctx, struct{}{}, func(ctx context.Context, rawTopic string) bool {
			if wsserver.TopicParamFromContext(ctx, "documentId") != documentID.String() {
				return false
			}

			return auth.FilterOrganization(organizationID)(ctx, rawTopic)
		})
	}
}

// BindMetadataChange binds a metadata change event to the given topic.
func (h *Handler) BindMetadataChange(tpc wsserver.Topic) {
	// We sync initial state on subscription.
	tpc.OnSub(func(ctx context.Context) {
		session, err := auth.ExtractSessionFromContext(ctx)
		if err != nil {
			return
		}

		documentID, err := xid.FromString(wsserver.TopicParamFromContext(ctx, "documentId"))
		if err != nil {
			return
		}

		branches, err := h.db.FetchDocumentBranches(ctx, documentID, session.ActiveOrganizationID)
		if err != nil {
			h.log.With("error", err).
				Error("fetching document branches for metadata WS subscription")

			return
		}

		for _, branch := range branches {
			doc, err := h.db.FetchDocumentByBranchID(ctx, branch.BranchID, session.ActiveOrganizationID)
			if err != nil {
				h.log.With("error", err).
					Error("fetching document branch for metadata WS subscription")

				continue
			}

			tpc.PublishOne(ctx, branchMetadataFrom(*doc))
		}
	})

	h.metadata.changeCallback = func(organizationID string, doc documentCore.Document) {
		pubCtx, pubCancel := context.WithTimeout(context.Background(), _publishTimeout)
		defer pubCancel()

		tpc.PublishMany(pubCtx, branchMetadataFrom(doc), func(ctx context.Context, rawTopic string) bool {
			if wsserver.TopicParamFromContext(ctx, "documentId") != doc.ID.String() {
				return false
			}

			return auth.FilterOrganization(organizationID)(ctx, rawTopic)
		})
	}
}

// branchMetadataFrom builds a BranchMetadata from a document.
func branchMetadataFrom(doc documentCore.Document) BranchMetadata {
	return BranchMetadata{
		BranchID:      doc.BranchID,
		DocumentName:  doc.DocumentName,
		Protected:     doc.Protected,
		CreatedAt:     doc.CreatedAt,
		CreatedBy:     doc.CreatedBy,
		UpdatedAt:     doc.UpdatedAt,
		LastUpdatedBy: doc.LastUpdatedBy,
	}
}
