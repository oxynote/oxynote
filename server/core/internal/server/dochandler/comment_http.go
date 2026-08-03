package dochandler

import (
	"context"
	"net/http"
	"slices"

	"github.com/oxynote/heimdall/internal/document"
	"github.com/oxynote/heimdall/internal/document/comment"
	"github.com/oxynote/heimdall/internal/notification"
	"github.com/oxynote/heimdall/internal/server/auth"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/rs/xid"
)

// CreateDocumentComment handles the creation of a new comment on a document.
func (h *Handler) CreateDocumentComment(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ci comment.CommentInput

	if err := httpserver.DecodeJSON(r, &ci); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), ci.BranchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c := comment.NewComment(
		ci,
		documentID,
		branchDoc.BranchID,
		session.UserID,
		session.ActiveOrganizationID,
	)

	if err := h.db.InsertDocumentComment(r.Context(), c); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeCreated,
			CommentID: c.ID,
		})
	}

	maintainers, err := h.db.FetchDocumentMaintainers(r.Context(), documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var userIDs []string

	for _, userID := range maintainers {
		if userID == session.UserID {
			continue
		}

		if !slices.Contains(userIDs, userID) {
			userIDs = append(userIDs, userID)
		}
	}

	h.notifPub.PublishNotifications(
		session.ActiveOrganizationID,
		notification.NewDocumentNewCommentNotification(
			session.UserID,
			documentID,
			c.ID,
			c.AnchorBlockID,
			c.BranchID,
		),
		userIDs...,
	)

	httpserver.Respond(
		h.log,
		w,
		c,
		http.StatusCreated,
	)
}

// CreateDocumentCommentReply handles the creation of a new reply to a comment.
func (h *Handler) CreateDocumentCommentReply(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c, err := h.db.FetchDocumentComment(
		r.Context(),
		commentID,
		documentID,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ri comment.ReplyInput

	if err := httpserver.DecodeJSON(r, &ri); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	reply := comment.NewReply(
		ri,
		commentID,
		session.UserID,
		session.ActiveOrganizationID,
	)

	if err := h.db.InsertDocumentCommentReply(r.Context(), reply); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeUpdated,
			CommentID: commentID,
		})
	}

	var userIDs []string

	if c.UserID.Valid && !slices.Contains(userIDs, c.UserID.String) && c.UserID.String != session.UserID {
		userIDs = append(userIDs, c.UserID.String)
	}

	for _, r := range c.Replies {
		if !r.UserID.Valid || r.UserID.String == session.UserID {
			continue
		}

		if !slices.Contains(userIDs, r.UserID.String) {
			userIDs = append(userIDs, r.UserID.String)
		}
	}

	h.notifPub.PublishNotifications(
		session.ActiveOrganizationID,
		notification.NewDocumentNewCommentReplyNotification(
			session.UserID,
			documentID,
			c.ID,
			reply.ID,
			c.AnchorBlockID,
			c.BranchID,
		),
		userIDs...,
	)

	httpserver.Respond(
		h.log,
		w,
		reply,
		http.StatusCreated,
	)
}

// FetchDocumentComment handles the retrieval of a single comment with its replies.
func (h *Handler) FetchDocumentComment(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c, err := h.db.FetchDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		c,
		http.StatusOK,
	)
}

// FetchDocumentComments handles the retrieval of all comments for a document branch.
// Requires a "branchId" query parameter to identify which branch's comments to return.
func (h *Handler) FetchDocumentComments(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := xid.FromString(r.URL.Query().Get("branchId"))
	if err != nil {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	comments, err := h.db.FetchDocumentCommentsByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		comments,
		http.StatusOK,
	)
}

// UpdateDocumentComment handles the update of a comment's content.
func (h *Handler) UpdateDocumentComment(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c, err := h.db.FetchDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !c.UserID.Valid || c.UserID.String != session.UserID {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	var ui comment.CommentInput

	if err := httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	nc := c.ApplyUpdate(ui)

	if err := h.db.UpdateDocumentComment(r.Context(), nc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeUpdated,
			CommentID: commentID,
		})
	}

	httpserver.Respond(
		h.log,
		w,
		nc,
		http.StatusOK,
	)
}

// UpdateDocumentCommentReply handles the update of a reply's content.
func (h *Handler) UpdateDocumentCommentReply(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	replyID, err := h.extractReplyParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	reply, err := h.db.FetchDocumentCommentReply(
		r.Context(),
		replyID,
		commentID,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !reply.UserID.Valid || reply.UserID.String != session.UserID {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	var ui comment.ReplyInput

	if err := httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	nr := reply.ApplyUpdate(ui)

	if err := h.db.UpdateDocumentCommentReply(r.Context(), nr); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeUpdated,
			CommentID: commentID,
		})
	}

	httpserver.Respond(
		h.log,
		w,
		nr,
		http.StatusOK,
	)
}

// ResolveDocumentComment handles marking a comment as resolved.
func (h *Handler) ResolveDocumentComment(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c, err := h.db.FetchDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.db.DeleteDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// TODO: Once comment history is implemented, use soft delete (resolve) here.
	// nc := c.Resolve(session.UserID)

	// if err := h.db.UpdateDocumentComment(r.Context(), nc); err != nil {
	// 	httpserver.RespondError(h.log, w, err)
	// 	return
	// }

	// if h.comments.changeCallback != nil {
	// 	h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
	// 		Type:      CommentChangeTypeUpdated,
	// 		CommentID: commentID,
	// 	})
	// }

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeDeleted,
			CommentID: commentID,
		})
	}

	httpserver.Respond(
		h.log,
		w,
		c,
		http.StatusOK,
	)
}

// TODO: Uncomment once comment history is implemented.
// UnresolveDocumentComment handles marking a comment as unresolved.
//func (h *Handler) UnresolveDocumentComment(w http.ResponseWriter, r *http.Request) {
//	session, err := auth.ExtractSessionFromContext(r.Context())
//	if err != nil {
//		httpserver.RespondError(h.log, w, err)
//		return
//	}
//
//	documentID, err := h.extractDocumentParameter(r)
//	if err != nil {
//		httpserver.RespondError(h.log, w, err)
//		return
//	}
//
//	commentID, err := h.extractCommentParameter(r)
//	if err != nil {
//		httpserver.RespondError(h.log, w, err)
//		return
//	}
//
//	c, err := h.db.FetchDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID)
//	if err != nil {
//		httpserver.RespondError(h.log, w, err)
//		return
//	}
//
//	nc := c.Unresolve()
//
//	if err := h.db.UpdateDocumentComment(r.Context(), nc); err != nil {
//		httpserver.RespondError(h.log, w, err)
//		return
//	}
//
//	if h.comments.changeCallback != nil {
//		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
//			Type:      CommentChangeTypeUpdated,
//			CommentID: commentID,
//		})
//	}
//
//	httpserver.Respond(
//		h.log,
//		w,
//		nc,
//		http.StatusOK,
//	)
//}

// DeleteDocumentComment handles the deletion of a comment.
// If the comment has replies, the first reply is promoted to become the main comment.
// If there are no replies, the comment is deleted.
func (h *Handler) DeleteDocumentComment(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var tx Tx

	if err := h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	c, err := tx.FetchDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !c.UserID.Valid || c.UserID.String != session.UserID {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	var cct CommentChangeType

	if len(c.Replies) > 0 {
		reply := c.Replies[0]

		nc := c.Replace(reply)

		if err := tx.ReplaceDocumentComment(r.Context(), nc); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if err := tx.DeleteDocumentCommentReply(r.Context(), reply.ID, commentID, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		cct = CommentChangeTypeUpdated
		c = &nc
	} else {
		if err := tx.DeleteDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		cct = CommentChangeTypeDeleted
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      cct,
			CommentID: commentID,
		})
	}

	httpserver.Respond(
		h.log,
		w,
		c,
		http.StatusOK,
	)
}

// DeleteDocumentCommentReply handles the deletion of a reply.
func (h *Handler) DeleteDocumentCommentReply(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	documentID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	commentID, err := h.extractCommentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	replyID, err := h.extractReplyParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	reply, err := h.db.FetchDocumentCommentReply(r.Context(), replyID, commentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !reply.UserID.Valid || reply.UserID.String != session.UserID {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	if err := h.db.DeleteDocumentCommentReply(r.Context(), replyID, commentID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, CommentChangeMessage{
			Type:      CommentChangeTypeUpdated,
			CommentID: commentID,
		})
	}

	httpserver.Respond(
		h.log,
		w,
		reply,
		http.StatusOK,
	)
}

// extractCommentParameter extracts the comment ID from the request parameters.
func (h *Handler) extractCommentParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "commentId")
}

// extractReplyParameter extracts the reply ID from the request parameters.
func (h *Handler) extractReplyParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "replyId")
}

// CommentsDBAgent is an interface that handles communication with the document comments database.
type CommentsDBAgent interface {
	// FetchDocumentByBranchID should fetch the document joined against the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// InsertDocumentComment should insert a new comment.
	InsertDocumentComment(ctx context.Context, c comment.Comment) error

	// InsertDocumentCommentReply should insert a new reply to a comment.
	InsertDocumentCommentReply(ctx context.Context, r comment.Reply) error

	// FetchDocumentComment should fetch a comment by its ID along with all its replies.
	FetchDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) (*comment.Comment, error)

	// FetchDocumentCommentsByBranchID should fetch all comments for a branch with their replies.
	FetchDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]comment.Comment, error)

	// FetchDocumentCommentReply should fetch a reply by its ID.
	FetchDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) (*comment.Reply, error)

	// UpdateDocumentComment should update an existing comment.
	UpdateDocumentComment(ctx context.Context, c comment.Comment) error

	// UpdateDocumentCommentReply should update an existing reply.
	UpdateDocumentCommentReply(ctx context.Context, r comment.Reply) error

	// ReplaceDocumentComment should replace a comment's content and author with the given reply's data.
	ReplaceDocumentComment(ctx context.Context, c comment.Comment) error

	// DeleteDocumentComment should delete a comment.
	DeleteDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) error

	// DeleteDocumentCommentsByBranchID should delete all comments for a branch.
	DeleteDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error

	// DeleteDocumentCommentReply should delete a reply.
	DeleteDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) error
}
