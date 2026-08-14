// Package dochandler provides HTTP handlers for managing documents.
package dochandler

import (
	"context"
	"log/slog"
	"net/http"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchanges"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// ErrInvalidSearchQuery is returned when the search query is invalid.
var ErrInvalidSearchQuery = errutil.New(http.StatusBadRequest, "document.invalid_search_query", "invalid search query")

// Handler holds dependencies required for tenant app-related operations.
type Handler struct {
	log                *slog.Logger
	db                 DB
	storer             Storer
	fileLocationFormat string
	githubMan          *githubapp.Manager
	webchangesClient   *webchanges.Client
	searchGateway      SearchGateway
	notifPub           notification.Publisher

	tree struct {
		changeCallback func(organizationID string, parentId null.Value[xid.ID])
	}

	comments struct {
		changeCallback func(organizationID string, documentID xid.ID, msg CommentChangeMessage)
	}

	metadata struct {
		changeCallback func(organizationID string, doc document.Document)
	}

	reviewers struct {
		changeCallback func(organizationID string, documentID xid.ID)
	}

	maintainers struct {
		changeCallback func(organizationID string, documentID xid.ID)
	}
}

// NewHandler creates a new handler instance with the provided logger and database.
func NewHandler(
	log *slog.Logger,
	db DB,
	storer Storer,
	fileLocationFormat string,
	githubMan *githubapp.Manager,
	webchangesClient *webchanges.Client,
	searchGateway SearchGateway,
	notifPub notification.Publisher,
) *Handler {
	return &Handler{
		log:                log,
		db:                 db,
		storer:             storer,
		fileLocationFormat: fileLocationFormat,
		githubMan:          githubMan,
		webchangesClient:   webchangesClient,
		searchGateway:      searchGateway,
		notifPub:           notifPub,
	}
}

// FetchDocumentMaintainers handles the retrieval of document maintainers by document ID.
func (h *Handler) FetchDocumentMaintainers(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	maintainers, err := h.db.FetchDocumentMaintainers(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		maintainers,
		http.StatusOK,
	)
}

// FetchBranchReviewers handles the retrieval of reviewers for a specific document branch.
func (h *Handler) FetchBranchReviewers(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	reviewers, err := h.db.FetchBranchReviewers(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		reviewers,
		http.StatusOK,
	)
}

// RequestBranchReviewer handles requesting a reviewer for a specific document branch.
func (h *Handler) RequestBranchReviewer(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp struct {
		UserID string `json:"userId"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ok, err := h.db.CheckOrganizationMember(r.Context(), session.ActiveOrganizationID, inp.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !ok {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	_, err = h.db.FetchBranchReviewer(r.Context(), branchID, inp.UserID, session.ActiveOrganizationID)

	switch {
	case err == nil:
		if err = h.db.UpdateBranchReviewer(r.Context(), document.BranchReviewer{
			BranchID:          branchID,
			UserID:            inp.UserID,
			OrganizationID:    session.ActiveOrganizationID,
			CurrentlyApproved: false,
		}); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	case errutil.IsNotFound(err):
		if err = h.db.InsertBranchReviewer(r.Context(), document.BranchReviewer{
			BranchID:       branchID,
			UserID:         inp.UserID,
			OrganizationID: session.ActiveOrganizationID,
		}); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	default:
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifPub.PublishNotifications(
		session.ActiveOrganizationID,
		notification.NewDocumentReviewRequestNotification(session.UserID, id, branchID),
		inp.UserID,
	)

	if h.reviewers.changeCallback != nil {
		h.reviewers.changeCallback(session.ActiveOrganizationID, id)
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusCreated,
	)
}

// RemoveBranchReviewer handles removing a reviewer from a specific document branch.
func (h *Handler) RemoveBranchReviewer(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	userID := r.URL.Query().Get("userId")
	if userID == "" {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	if err := h.db.DeleteBranchReviewer(r.Context(), branchID, userID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.reviewers.changeCallback != nil {
		h.reviewers.changeCallback(session.ActiveOrganizationID, id)
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusOK,
	)
}

// UpdateDocumentTree handles the update of the document tree.
func (h *Handler) UpdateDocumentTree(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var data document.SwapInput

	if err = httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	doc, err := tx.FetchDocument(r.Context(), data.ID, session.ActiveOrganizationID, document.DefaultBranch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tree, err := tx.FetchDocumentTreeByDocumentParentID(r.Context(), data.ParentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	changedParentID := doc.ParentID != data.ParentID

	if changedParentID { //nolint:nestif // the branching is sequential and readable
		var previousTree document.Summaries

		previousTree, err = tx.FetchDocumentTreeByDocumentParentID(r.Context(), doc.ParentID, session.ActiveOrganizationID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		var removedTree document.Summaries

		removedTree, err = previousTree.Remove(data.ID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if len(removedTree) != 1 {
			err = tx.UpdateDocumentTree(r.Context(), removedTree, session.ActiveOrganizationID)
			if err != nil {
				httpserver.RespondError(h.log, w, err)
				return
			}
		}

		err = tx.UpdateDocumentParentID(r.Context(), doc.ID, data.ParentID, session.ActiveOrganizationID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		tree = append(tree, document.Summary{
			ID:           doc.ID,
			DocumentName: doc.DocumentName,
			Icon:         doc.Icon,
			Protected:    doc.Protected,
		})
	}

	swappedTree, err := tree.Swap(data.ID, data.SortIndex)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.UpdateDocumentTree(r.Context(), swappedTree, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.tree.changeCallback != nil {
		if changedParentID {
			h.tree.changeCallback(session.ActiveOrganizationID, doc.ParentID)
		}

		h.tree.changeCallback(session.ActiveOrganizationID, data.ParentID)
	}

	httpserver.Respond(
		h.log,
		w,
		tree,
		http.StatusOK,
	)
}

// FetchDocumentTree handles the retrieval of the document tree.
func (h *Handler) FetchDocumentTree(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ds, err := h.db.FetchDocumentTree(r.Context(), session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		ds,
		http.StatusOK,
	)
}

// SearchDocuments handles the search for documents matching a query.
func (h *Handler) SearchDocuments(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	q := r.URL.Query().Get("q")
	if q == "" {
		httpserver.RespondError(
			h.log,
			w,
			ErrInvalidSearchQuery,
		)

		return
	}

	data, err := h.searchGateway.SearchDocuments(r.Context(), session.ActiveOrganizationID, q)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data) //nolint:errcheck,gosec // client write errors provide no meaningful info
}

// CreateDocument handles the creation of a new document.
func (h *Handler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var di document.CreateInput

	if err = httpserver.DecodeJSON(r, &di); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	doc := document.NewDocument(di, session.ActiveOrganizationID, session.UserID)

	if err = tx.InsertDocument(r.Context(), doc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.UpsertDocumentMaintainers(r.Context(), doc.ID, session.ActiveOrganizationID, []string{session.UserID}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.InsertDocumentSearchJob(r.Context(), searchgw.BlocksDiff(nil, doc.Search())); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tree, err := tx.FetchDocumentTreeByDocumentParentID(
		r.Context(),
		di.ParentID,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	swappedTree, err := tree.Swap(doc.ID, 0)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.UpdateDocumentTree(r.Context(), swappedTree, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.tree.changeCallback != nil {
		h.tree.changeCallback(session.ActiveOrganizationID, doc.ParentID)
	}

	httpserver.Respond(
		h.log,
		w,
		doc,
		http.StatusCreated,
	)
}

// UpdateDocumentBranch handles updating the name and protection status of a document branch.
func (h *Handler) UpdateDocumentBranch(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ui struct {
		Name      null.String `json:"name"`
		Protected null.Bool   `json:"protected"`
	}

	if err := httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ndoc := doc.ApplyBranchUpdate(ui.Name, ui.Protected, session.UserID)

	if err := h.db.UpdateDocumentBranchMetadata(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.metadata.changeCallback != nil {
		h.metadata.changeCallback(session.ActiveOrganizationID, ndoc)
	}

	httpserver.Respond(
		h.log,
		w,
		ndoc,
		http.StatusOK,
	)
}

// FetchDocumentBranchesUnsafe handles the retrieval of all branches for a document
// without organization ownership checks. Intended for internal system use only.
func (h *Handler) FetchDocumentBranchesUnsafe(w http.ResponseWriter, r *http.Request) {
	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branches, err := h.db.FetchDocumentBranchesUnsafe(r.Context(), id)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		branches,
		http.StatusOK,
	)
}

// FetchDocumentBranchByIDUnsafe handles the retrieval of a document branch by its ID
// without organization ownership checks. Intended for internal system use only.
func (h *Handler) FetchDocumentBranchByIDUnsafe(w http.ResponseWriter, r *http.Request) {
	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocumentUnsafeByBranchID(r.Context(), branchID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		doc,
		http.StatusOK,
	)
}

// UpdateDocumentBranchByIDUnsafe handles the update of a document branch content by branch ID
// without organization ownership checks. Intended for internal system use only.
func (h *Handler) UpdateDocumentBranchByIDUnsafe(w http.ResponseWriter, r *http.Request) {
	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocumentUnsafeByBranchID(r.Context(), branchID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var ui document.UpdateInput

	if err = httpserver.DecodeJSON(r, &ui); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ui.Branch = doc.BranchName

	ndoc, err := doc.ApplyUpdate(ui)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.UpdateDocument(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	maintainers, err := tx.FetchDocumentMaintainers(r.Context(), doc.ID, doc.OrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var maintainersAdded bool

	for _, maintainer := range ui.Maintainers {
		if !slices.Contains(maintainers, maintainer) {
			maintainersAdded = true
			break
		}
	}

	if maintainersAdded {
		if err = tx.UpsertDocumentMaintainers(
			r.Context(),
			doc.ID,
			doc.OrganizationID,
			ui.Maintainers,
		); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	if ndoc.BranchName == document.DefaultBranch {
		if err = tx.InsertDocumentSearchJob(
			r.Context(),
			searchgw.BlocksDiff(doc.Search(), ndoc.Search()),
		); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.metadata.changeCallback != nil {
		h.metadata.changeCallback(doc.OrganizationID, ndoc)
	}

	if maintainersAdded && h.maintainers.changeCallback != nil {
		h.maintainers.changeCallback(doc.OrganizationID, doc.ID)
	}

	httpserver.Respond(
		h.log,
		w,
		ndoc,
		http.StatusOK,
	)
}

// UpdateBranchReviewApproval handles setting the current user's approval state on a branch.
func (h *Handler) UpdateBranchReviewApproval(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp struct {
		Approved bool `json:"approved"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	existing, err := h.db.FetchBranchReviewer(r.Context(), branchDoc.BranchID, session.UserID, session.ActiveOrganizationID)

	switch {
	case err == nil:
		if err = h.db.UpdateBranchReviewer(r.Context(), document.BranchReviewer{
			BranchID:           branchDoc.BranchID,
			UserID:             session.UserID,
			OrganizationID:     session.ActiveOrganizationID,
			CurrentlyApproved:  inp.Approved,
			PreviouslyApproved: existing.PreviouslyApproved,
		}); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	case errutil.IsNotFound(err):
		if err = h.db.InsertBranchReviewer(r.Context(), document.BranchReviewer{
			BranchID:          branchDoc.BranchID,
			UserID:            session.UserID,
			OrganizationID:    session.ActiveOrganizationID,
			CurrentlyApproved: inp.Approved,
		}); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	default:
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.reviewers.changeCallback != nil {
		h.reviewers.changeCallback(session.ActiveOrganizationID, id)
	}

	httpserver.Respond(
		h.log,
		w,
		branchDoc,
		http.StatusOK,
	)
}

// MergeBranches merges the content of a source branch into a target branch.
// The target branch's hooks are soft-deleted and replaced with copies from the source.
// Target branch comments are cleared. Source branch reviewer approvals are promoted.
func (h *Handler) MergeBranches(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp struct {
		FromBranchID xid.ID `json:"fromBranchId"`
		ToBranchID   xid.ID `json:"toBranchId"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	fromDoc, err := h.db.FetchDocumentByBranchID(r.Context(), inp.FromBranchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	toDoc, err := h.db.FetchDocumentByBranchID(r.Context(), inp.ToBranchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if fromDoc.ID != toDoc.ID {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "document.branch_mismatch", "branches must belong to the same document"))
		return
	}

	ndoc := toDoc.MergeBranch(fromDoc.Branch, session.UserID)

	var tx Tx

	if err := h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := tx.SoftDeleteDocumentHooksByBranchID(r.Context(), toDoc.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.DeleteDocumentCommentsByBranchID(r.Context(), toDoc.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.copyHooksToBranch(r.Context(), tx, fromDoc.BranchID, toDoc.BranchID, toDoc.ID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.UpdateDocument(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.InsertDocumentSearchJob(r.Context(), searchgw.BlocksDiff(toDoc.Search(), ndoc.Search())); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.PromoteBranchApprovals(r.Context(), fromDoc.BranchID, toDoc.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.metadata.changeCallback != nil {
		h.metadata.changeCallback(session.ActiveOrganizationID, ndoc)
	}

	if h.reviewers.changeCallback != nil {
		h.reviewers.changeCallback(session.ActiveOrganizationID, toDoc.ID)
	}

	httpserver.Respond(
		h.log,
		w,
		ndoc,
		http.StatusOK,
	)
}

// DeleteDocument handles the deletion of a document by its ID.
func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocument(r.Context(), id, session.ActiveOrganizationID, document.DefaultBranch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	hooks, err := h.db.FetchDocumentHooksByDocumentID(r.Context(), id, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	for _, hk := range hooks {
		err = hk.Delete(r.Context(), hook.NewInput(
			session.ActiveOrganizationID,
			h.githubMan,
			h.webchangesClient,
		))
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.DeleteDocument(r.Context(), doc.ID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.InsertDocumentSearchJob(r.Context(), searchgw.BlocksDiff(doc.Search(), nil)); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.tree.changeCallback != nil {
		h.tree.changeCallback(session.ActiveOrganizationID, doc.ParentID)
	}

	httpserver.Respond(
		h.log,
		w,
		doc,
		http.StatusOK,
	)
}

// DuplicateDocument handles the duplication of a document by its ID.
func (h *Handler) DuplicateDocument(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	id, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocument(r.Context(), id, session.ActiveOrganizationID, document.DefaultBranch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	duplDoc := doc.Duplicate(session.UserID)

	var tx Tx

	err = h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.InsertDocument(r.Context(), duplDoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.UpsertDocumentMaintainers(r.Context(), duplDoc.ID, session.ActiveOrganizationID, []string{session.UserID}); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.InsertDocumentSearchJob(r.Context(), searchgw.BlocksDiff(nil, duplDoc.Search())); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	tree, err := tx.FetchDocumentTreeByDocumentParentID(
		r.Context(),
		duplDoc.ParentID,
		session.ActiveOrganizationID,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	swappedTree, err := tree.Swap(duplDoc.ID, 0)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.UpdateDocumentTree(r.Context(), swappedTree, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if h.tree.changeCallback != nil {
		h.tree.changeCallback(session.ActiveOrganizationID, duplDoc.ParentID)
	}

	httpserver.Respond(
		h.log,
		w,
		duplDoc,
		http.StatusCreated,
	)
}

// FetchDocumentBranches handles the retrieval of all branches for a document.
func (h *Handler) FetchDocumentBranches(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	docID, err := h.extractDocumentParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branches, err := h.db.FetchDocumentBranches(r.Context(), docID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		branches,
		http.StatusOK,
	)
}

// CreateDocumentBranch handles the creation of a new branch forked from an existing source branch.
// Hooks from the source branch are copied to the new branch.
func (h *Handler) CreateDocumentBranch(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var inp struct {
		Branch         string `json:"branch"`
		SourceBranchID xid.ID `json:"sourceBranchId"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if inp.Branch == "" {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	sourceDoc, err := h.db.FetchDocumentByBranchID(r.Context(), inp.SourceBranchID, session.ActiveOrganizationID)
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

	if err = tx.ForkDocumentBranch(
		r.Context(),
		sourceDoc.ID,
		session.ActiveOrganizationID,
		sourceDoc.BranchName,
		inp.Branch,
		session.UserID,
	); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	newDoc, err := tx.FetchDocument(r.Context(), sourceDoc.ID, session.ActiveOrganizationID, inp.Branch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.copyHooksToBranch(r.Context(), tx, sourceDoc.BranchID, newDoc.BranchID, sourceDoc.ID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		newDoc,
		http.StatusCreated,
	)
}

// DeleteDocumentBranch handles the deletion of a specific document branch by ID.
// Returns 409 if it is the last remaining branch for the document.
func (h *Handler) DeleteDocumentBranch(w http.ResponseWriter, r *http.Request) {
	session, err := auth.ExtractSessionFromContext(r.Context())
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := h.extractBranchParameter(r)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if branchDoc.Default {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusConflict, "document.default_branch", "cannot delete the default branch"))
		return
	}

	count, err := h.db.CountDocumentBranches(r.Context(), branchDoc.ID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if count <= 1 {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusConflict, "document.last_branch", "cannot delete the last branch"))
		return
	}

	if err := h.db.DeleteDocumentBranchByID(r.Context(), branchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusOK,
	)
}

// copyHooksToBranch fetches all hooks from fromBranchID and re-creates them on
// toBranchID with fresh state. This is handler-level business logic; the DB
// layer is not involved in the re-creation decision.
func (h *Handler) copyHooksToBranch(ctx context.Context, db HooksDBAgent, fromBranchID, toBranchID, documentID xid.ID, organizationID string) error {
	hooks, err := db.FetchDocumentHooksByBranchID(ctx, fromBranchID, organizationID)
	if err != nil {
		return err
	}

	inp := hook.NewInput(organizationID, h.githubMan, h.webchangesClient)

	for _, hk := range hooks {
		newHk, err := hook.NewHook(ctx, hook.CreateInput{
			Type:     hk.Type,
			BranchID: toBranchID,
			BlockID:  hk.BlockID,
			Settings: hk.Settings,
		}, documentID, toBranchID, organizationID, inp)
		if err != nil {
			return err
		}

		if err := db.InsertDocumentHook(ctx, *newHk); err != nil {
			return err
		}
	}

	return nil
}

// extractDocumentParameter extracts the document ID from the request parameters.
func (h *Handler) extractDocumentParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "documentId")
}

// extractBranchParameter extracts the branch ID from the request parameters.
func (h *Handler) extractBranchParameter(r *http.Request) (xid.ID, error) {
	return httpserver.ExtractNamedID(r, "branchId")
}

// DB is an interface that combines sqlutil.DB and DBAgent.
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx is an interface that combines sqlutil.Tx and DBAgent.
type Tx interface {
	sqlutil.Tx
	DBAgent
}

// DBAgent is an interface that handles communication with the document database.
type DBAgent interface {
	HooksDBAgent
	CommentsDBAgent
	ReviewersDBAgent
	FilesDBAgent
	DocumentsDBAgent
	BranchesDBAgent
	TreeDBAgent
	MaintainersDBAgent
}

// DocumentsDBAgent is an interface that handles communication with the
// document entity database.
type DocumentsDBAgent interface {
	// InsertDocument should insert the document.
	InsertDocument(ctx context.Context, doc document.Document) error

	// InsertDocumentSearchJob should insert the document search job.
	InsertDocumentSearchJob(ctx context.Context, diff searchgw.BlocksDifference) error

	// CheckDocumentExists returns nil if the document exists and belongs to the given organization.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error

	// FetchDocument should fetch a document joined against the given branch for the given id.
	FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*document.Document, error)

	// FetchDocumentByBranchID should fetch a document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentUnsafeByBranchID should fetch the document joined against
	// the branch identified by branchID without checking organization ownership.
	// This is intended only for internal system use cases.
	FetchDocumentUnsafeByBranchID(ctx context.Context, branchID xid.ID) (*document.Document, error)

	// UpdateDocument should update the document.
	UpdateDocument(ctx context.Context, doc document.Document) error

	// DeleteDocument should delete the document.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) error
}

// BranchesDBAgent is an interface that handles communication with the
// document branches database.
type BranchesDBAgent interface {
	// FetchDocumentBranchesUnsafe should fetch all branches for a document as
	// lightweight summaries without checking organization ownership.
	// This is intended only for internal system use cases.
	FetchDocumentBranchesUnsafe(ctx context.Context, docID xid.ID) ([]document.BranchSummary, error)

	// ForkDocumentBranch should create a new branch by copying the contents of an existing source branch.
	ForkDocumentBranch(ctx context.Context, docID xid.ID, orgID, sourceBranch, targetBranch, createdBy string) error

	// FetchDocumentBranches should fetch all branches for a document as lightweight summaries.
	FetchDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) ([]document.BranchSummary, error)

	// CountDocumentBranches should return the number of branches for a document.
	CountDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) (int, error)

	// DeleteDocumentBranchByID should delete a branch identified by its ID.
	DeleteDocumentBranchByID(ctx context.Context, branchID xid.ID, organizationID string) error

	// UpdateDocumentBranchMetadata should update the name and protection status of a branch
	// without modifying content or inserting a changelog entry.
	UpdateDocumentBranchMetadata(ctx context.Context, doc document.Document) error
}

// TreeDBAgent is an interface that handles communication with the document
// tree database.
type TreeDBAgent interface {
	// FetchDocumentTree should fetch the document tree.
	FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error)

	// FetchDocumentTreeByParentID should fetch the document tree for the given parent id.
	FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (document.Summaries, error)

	// UpdateDocumentTree should update the tree of the document childrens.
	UpdateDocumentTree(ctx context.Context, ss document.Summaries, organizationID string) error

	// UpdateDocumentParentID should update the parent id of the document.
	UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error
}

// MaintainersDBAgent is an interface that handles communication with the
// document maintainers database.
type MaintainersDBAgent interface {
	// UpsertDocumentMaintainers should upsert the document maintainers.
	UpsertDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string, maintainerIDs []string) error

	// FetchDocumentMaintainers should fetch the document maintainers.
	FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error)

	// CheckOrganizationMember should check if a user is a member of the organization.
	CheckOrganizationMember(ctx context.Context, organizationID, userID string) (bool, error)
}

// ReviewersDBAgent is an interface that handles communication with the branch reviewers database.
type ReviewersDBAgent interface {
	// FetchBranchReviewers should fetch all reviewers for a branch.
	FetchBranchReviewers(ctx context.Context, branchID xid.ID, organizationID string) ([]document.BranchReviewer, error)

	// FetchBranchReviewer should fetch a single reviewer for a branch by user ID.
	FetchBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) (*document.BranchReviewer, error)

	// InsertBranchReviewer should insert a reviewer for a branch.
	InsertBranchReviewer(ctx context.Context, reviewer document.BranchReviewer) error

	// UpdateBranchReviewer should update the approval state of a branch reviewer.
	UpdateBranchReviewer(ctx context.Context, reviewer document.BranchReviewer) error

	// DeleteBranchReviewer should remove a reviewer from a branch.
	DeleteBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) error

	// PromoteBranchApprovals should promote reviewer approvals from one branch to another.
	PromoteBranchApprovals(ctx context.Context, fromBranchID, toBranchID xid.ID, organizationID string) error
}

// SearchGateway is an interface that handles communication with the search engine.
type SearchGateway interface {
	// SearchDocuments should find the documents matching the query.
	SearchDocuments(ctx context.Context, organizationID, query string) ([]byte, error)
}
