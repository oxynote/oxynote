// Package comment provides HTTP handlers for document comments.
package comment

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/oxynote/oxynote/server/core/internal/document"
	commentCore "github.com/oxynote/oxynote/server/core/internal/document/comment"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// Handler holds dependencies required for document comment operations.
type Handler struct {
	log      *slog.Logger
	db       DB
	notifPub notification.Publisher

	comments struct {
		changeCallback func(organizationID string, documentID xid.ID, msg ChangeMessage)
	}
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	notifPub notification.Publisher,
) *Handler {
	return &Handler{
		log:      log,
		db:       db,
		notifPub: notifPub,
	}
}

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

	var ci commentCore.Input

	if err = httpserver.DecodeJSON(r, &ci); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), ci.BranchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	c := commentCore.NewComment(
		ci,
		documentID,
		branchDoc.BranchID,
		session.UserID,
		session.ActiveOrganizationID,
	)

	if err = h.db.InsertDocumentComment(r.Context(), c); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeCreated,
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

	var ri commentCore.ReplyInput

	if err := httpserver.DecodeJSON(r, &ri); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	reply := commentCore.NewReply(
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
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeUpdated,
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

	var ui commentCore.Input

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
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeUpdated,
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

	var ui commentCore.ReplyInput

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
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeUpdated,
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

	// resolving currently destroys the comment and its replies, so it needs
	// the same ownership guard the delete route applies.
	if !c.UserID.Valid || c.UserID.String != session.UserID {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	if err := h.db.DeleteDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// TODO: Once comment history is implemented, soft delete (resolve) the
	// comment via c.Resolve and publish an update message instead of
	// removing it.

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeDeleted,
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
// func (h *Handler) UnresolveDocumentComment(w http.ResponseWriter, r *http.Request) {
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
//		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
//			Type:      ChangeTypeUpdated,
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

	if err = h.db.BeginTx(r.Context(), &tx); err != nil {
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

	var cct ChangeType

	if nc, ok := c.Replace(); ok {
		reply := c.Replies[0]

		if err := tx.ReplaceDocumentComment(r.Context(), nc); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if err := tx.DeleteDocumentCommentReply(r.Context(), reply.ID, commentID, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		cct = ChangeTypeUpdated
		c = &nc
	} else {
		if err := tx.DeleteDocumentComment(r.Context(), commentID, documentID, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		cct = ChangeTypeDeleted
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.comments.changeCallback != nil {
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
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
		h.comments.changeCallback(session.ActiveOrganizationID, documentID, ChangeMessage{
			Type:      ChangeTypeUpdated,
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

// extractDocumentParameter extracts the document ID from the request parameters.
func (h *Handler) extractDocumentParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "documentId")
}

// extractReplyParameter extracts the reply ID from the request parameters.
func (h *Handler) extractReplyParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "replyId")
}

// DB is an interface that combines sqlutil.DB and DBAgent.
//
//go:generate ../../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx is an interface that combines sqlutil.Tx and DBAgent.
//
//go:generate ../../../../../scripts/codegen/mock -t internal Tx tx
type Tx interface {
	sqlutil.Tx
	DBAgent
}

// DBAgent is an interface that handles communication with the document comments database.
type DBAgent interface {
	CommentsDBAgent

	// FetchDocumentMaintainers should fetch the document maintainers.
	FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error)
}

// CommentsDBAgent is an interface that handles communication with the document comments database.
type CommentsDBAgent interface {
	RepliesDBAgent

	// FetchDocumentByBranchID should fetch the document joined against the branch identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// InsertDocumentComment should insert a new comment.
	InsertDocumentComment(ctx context.Context, c commentCore.Comment) error

	// FetchDocumentComment should fetch a comment by its ID along with all its replies.
	FetchDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) (*commentCore.Comment, error)

	// FetchDocumentCommentsByBranchID should fetch all comments for a branch with their replies.
	FetchDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]commentCore.Comment, error)

	// UpdateDocumentComment should update an existing comment.
	UpdateDocumentComment(ctx context.Context, c commentCore.Comment) error

	// ReplaceDocumentComment should replace a comment's content and author with the given reply's data.
	ReplaceDocumentComment(ctx context.Context, c commentCore.Comment) error

	// DeleteDocumentComment should delete a comment.
	DeleteDocumentComment(ctx context.Context, id, documentID xid.ID, organizationID string) error

	// DeleteDocumentCommentsByBranchID should delete all comments for a branch.
	DeleteDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error
}

// RepliesDBAgent is an interface that handles communication with the
// document comment replies database.
type RepliesDBAgent interface {
	// InsertDocumentCommentReply should insert a new reply to a comment.
	InsertDocumentCommentReply(ctx context.Context, r commentCore.Reply) error

	// FetchDocumentCommentReply should fetch a reply by its ID.
	FetchDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) (*commentCore.Reply, error)

	// UpdateDocumentCommentReply should update an existing reply.
	UpdateDocumentCommentReply(ctx context.Context, r commentCore.Reply) error

	// DeleteDocumentCommentReply should delete a reply.
	DeleteDocumentCommentReply(ctx context.Context, id, commentID xid.ID, organizationID string) error
}
