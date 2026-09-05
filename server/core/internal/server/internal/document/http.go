// Package document provides HTTP handlers for managing documents.
package document

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// ErrInvalidSearchQuery is returned when the search query is invalid.
var ErrInvalidSearchQuery = errutil.New(http.StatusBadRequest, "document.invalid_search_query", "invalid search query")

// ErrBranchMismatch is returned when the requested branch does not belong to
// the document identified by the request path.
var ErrBranchMismatch = errutil.New(http.StatusNotFound, "document.branch_mismatch", "branch does not belong to the document")

// ErrDefaultBranchRename is returned when a rename targets the default branch.
var ErrDefaultBranchRename = errutil.New(http.StatusBadRequest, "document.default_branch_rename", "the default branch cannot be renamed")

// ErrInvalidDocumentParent is returned when a document would be moved under
// itself or one of its own descendants.
var ErrInvalidDocumentParent = errutil.New(http.StatusBadRequest, "document.invalid_parent", "document cannot be moved under itself or its descendant")

// Handler holds dependencies required for tenant app-related operations.
type Handler struct {
	log             *slog.Logger
	db              DB
	githubMan       *github.Manager
	webchangeClient *webchange.Client
	searchGateway   SearchGateway
	searchJobs      *search.Jobs
	notifPub        notification.Publisher
	storer          Storer

	tree struct {
		changeCallback func(organizationID string, parentId null.Value[xid.ID])
	}

	metadata struct {
		changeCallback func(organizationID string, doc documentCore.Document)
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
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	searchGateway SearchGateway,
	searchJobs *search.Jobs,
	notifPub notification.Publisher,
	storer Storer,
) *Handler {
	return &Handler{
		log:             log,
		db:              db,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		searchGateway:   searchGateway,
		searchJobs:      searchJobs,
		notifPub:        notifPub,
		storer:          storer,
	}
}

// RequireDocumentAccess rejects a request whose path document does not exist
// within the caller's organization. A handler that only filters its query by
// organization answers such a request with an empty success, which tells a
// caller enumerating ids that the document exists somewhere else; behind this
// every route under a document answers not found instead, the same as one
// naming an id that exists nowhere.
func (h *Handler) RequireDocumentAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session, ok := auth.RequireSession(h.log, w, r)
		if !ok {
			return
		}

		id, err := httpserver.ExtractNamedID(r, "documentId")
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if err := h.db.CheckDocumentExists(r.Context(), id, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// FetchDocumentMaintainers handles the retrieval of document maintainers by document ID.
func (h *Handler) FetchDocumentMaintainers(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
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

// VerifyDocumentAccess handles the access check for a document: it responds
// with no content when the document is visible to the caller's organization
// and with not found when it does not exist or belongs to another one.
func (h *Handler) VerifyDocumentAccess(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if _, err := h.db.FetchDocument(r.Context(), id, session.ActiveOrganizationID, documentCore.DefaultBranch); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusNoContent,
	)
}

// FetchBranchReviewers handles the retrieval of reviewers for a specific document branch.
func (h *Handler) FetchBranchReviewers(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if branchDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if branchDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
		return
	}

	var inp struct {
		UserID string `json:"userId"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	ok, err = h.db.CheckOrganizationMember(r.Context(), session.ActiveOrganizationID, inp.UserID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if !ok {
		httpserver.RespondError(h.log, w, httpserver.ErrNotPermitted)
		return
	}

	err = h.upsertBranchReviewer(
		r.Context(),
		branchDoc.BranchID,
		inp.UserID,
		session.ActiveOrganizationID,
		false,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.notifPub.PublishNotifications(
		session.ActiveOrganizationID,
		notification.NewDocumentReviewRequestNotification(session.UserID, branchDoc.ID, branchDoc.BranchID),
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
		// the reviewer is addressable only through the collection plus the
		// user it belongs to, which is the same shape its delete takes.
		httpserver.LocationHeader(r.URL.Path+"?userId="+url.QueryEscape(inp.UserID)),
	)
}

// RemoveBranchReviewer handles removing a reviewer from a specific document branch.
func (h *Handler) RemoveBranchReviewer(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	userID := r.URL.Query().Get("userId")
	if userID == "" {
		httpserver.RespondError(h.log, w, httpserver.ErrInvalidForm)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the change event is published under the path document id, so a
	// mismatched request would announce the removal to watchers of an
	// unrelated document.
	if branchDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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
		http.StatusNoContent,
	)
}

// UpdateDocumentTree handles the update of the document tree.
func (h *Handler) UpdateDocumentTree(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var data documentCore.SwapInput

	if err := httpserver.DecodeJSON(r, &data); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	var tx Tx

	err := h.db.BeginTx(r.Context(), &tx)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	doc, err := tx.FetchDocument(r.Context(), data.ID, session.ActiveOrganizationID, documentCore.DefaultBranch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if data.ParentID.Valid {
		if err = tx.CheckDocumentExists(r.Context(), data.ParentID.V, session.ActiveOrganizationID); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		var cycle bool

		cycle, err = tx.CheckDocumentCycle(r.Context(), doc.ID, data.ParentID.V, session.ActiveOrganizationID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		if cycle {
			httpserver.RespondError(h.log, w, ErrInvalidDocumentParent)
			return
		}
	}

	tree, err := tx.FetchDocumentTreeByDocumentParentID(r.Context(), data.ParentID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	changedParentID := doc.ParentID != data.ParentID

	if changedParentID { //nolint:nestif // the branching is sequential and readable
		var previousTree documentCore.Summaries

		previousTree, err = tx.FetchDocumentTreeByDocumentParentID(r.Context(), doc.ParentID, session.ActiveOrganizationID)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}

		var removedTree documentCore.Summaries

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

		tree = append(tree, documentCore.Summary{
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
		swappedTree,
		http.StatusOK,
	)
}

// FetchDocumentTree handles the retrieval of the document tree.
func (h *Handler) FetchDocumentTree(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	var di documentCore.CreateInput

	if err := httpserver.DecodeJSON(r, &di); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc := documentCore.NewDocument(di, session.ActiveOrganizationID, session.UserID)

	if err := h.insertDocumentTx(r.Context(), doc, null.Value[xid.ID]{}, session); err != nil {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if doc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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

	// the main branch is found by name in the tree and content queries, and
	// a rename would make the document look nameless and contentless.
	if doc.Default && ui.Name.Valid && ui.Name.String != doc.BranchName {
		httpserver.RespondError(h.log, w, ErrDefaultBranchRename)
		return
	}

	ndoc := doc.ApplyBranchUpdate(ui.Name, ui.Protected, session.UserID)

	var tx Tx

	if err := h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := tx.UpdateDocumentBranchMetadata(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the index carries the branch name on every entry of the branch.
	if ndoc.BranchName != doc.BranchName {
		if err := h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDiff(doc.Search(), ndoc.Search())); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
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
	id, err := httpserver.ExtractNamedID(r, "documentId")
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

	var ui documentCore.UpdateInput

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

	// the update carries the editors of one persist, so the set is only
	// ever added to. Diffing it against the stored maintainers would drop
	// everyone who happens not to be editing right now.
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

	if err = h.searchJobs.Enqueue(
		r.Context(),
		tx,
		search.BlocksDiff(doc.Search(), ndoc.Search()),
	); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the change event is published under the path document id, so a
	// mismatched request would announce the approval to watchers of an
	// unrelated document.
	if branchDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
		return
	}

	var inp struct {
		Approved bool `json:"approved"`
	}

	if err = httpserver.DecodeJSON(r, &inp); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = h.upsertBranchReviewer(
		r.Context(),
		branchDoc.BranchID,
		session.UserID,
		session.ActiveOrganizationID,
		inp.Approved,
	)
	if err != nil {
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
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

	// a self-merge would soft-delete the branch's hooks and comments and
	// then copy the hooks back from the same, now hook-less, branch —
	// permanently destroying both.
	if inp.FromBranchID == inp.ToBranchID {
		httpserver.RespondError(h.log, w, errutil.New(http.StatusBadRequest, "document.branch_self_merge", "cannot merge a branch into itself"))
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

	if toDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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

	if err := tx.UpdateDocument(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.ReplaceBranchTags(r.Context(), session.ActiveOrganizationID, fromDoc.BranchID, toDoc.BranchID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDiff(toDoc.Search(), ndoc.Search())); err != nil {
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

	// the hooks are copied after the commit: creating one creates its
	// external watcher too, and a rollback cannot take that back.
	if err := h.copyHooksToBranch(
		r.Context(),
		fromDoc.BranchID,
		toDoc.BranchID,
		toDoc.ID,
		session.ActiveOrganizationID,
		nil,
	); err != nil {
		logutil.Critical(h.log, err).Error(
			"cannot copy hooks to the merged branch",
			slog.String("branch_id", toDoc.BranchID.String()),
		)
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocument(r.Context(), id, session.ActiveOrganizationID, documentCore.DefaultBranch)
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
			h.webchangeClient,
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

	// the delete reports the ids of the document and its cascade-deleted
	// descendants; queuing their index removal from them in the same
	// transaction is this handler's job, since after the commit nothing
	// else knows what went away.
	ids, err := tx.DeleteDocument(r.Context(), doc.ID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if len(ids) != 0 {
		if err = h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDifference{
			RemovedDocuments: ids,
		}); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	doc, err := h.db.FetchDocument(r.Context(), id, session.ActiveOrganizationID, documentCore.DefaultBranch)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	duplDoc, files, uids := doc.Duplicate(session.UserID)

	if err = h.insertDocumentTx(r.Context(), duplDoc, null.ValueFrom(doc.BranchID), session); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the hooks are copied after the commit: creating one creates its
	// external watcher too, and a rollback cannot take that back.
	if err := h.copyHooksToBranch(
		r.Context(),
		doc.BranchID,
		duplDoc.BranchID,
		duplDoc.ID,
		session.ActiveOrganizationID,
		uids,
	); err != nil {
		logutil.Critical(h.log, err).Error(
			"cannot copy hooks to the duplicated document",
			slog.String("branch_id", duplDoc.BranchID.String()),
		)
	}

	h.copyDocumentFiles(r.Context(), files, id, duplDoc.ID, session.ActiveOrganizationID)

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

// copyDocumentFiles gives the duplicated document its own copies of the
// source document's files.
//
// It runs after the document is committed, and each copy writes its row
// before the object: a crash can then only leave a row without an object,
// which the file manager reaps, whereas an object without a row would be
// invisible to every cleanup path. A failure here leaves the duplicate with
// broken images rather than failing the duplication outright — the document
// exists, and deleting it reclaims whatever was copied.
func (h *Handler) copyDocumentFiles(
	ctx context.Context,
	files map[string]string,
	fromDocumentID, toDocumentID xid.ID,
	organizationID string,
) {
	for oldID, newID := range files {
		f, err := h.db.FetchDocumentFile(ctx, oldID, organizationID)
		if err != nil {
			// the content referenced a file that has no row, so there is
			// nothing to copy; the duplicate keeps the dangling reference
			// its source already had.
			h.log.Warn(
				"cannot fetch document file for duplication",
				slog.String("error", err.Error()),
				slog.String("file_id", oldID),
			)

			continue
		}

		fromFolder := file.Folder(organizationID, fromDocumentID)
		toFolder := file.Folder(organizationID, toDocumentID)

		err = h.db.InsertDocumentFile(ctx, file.NewFile(
			newID,
			f.Location,
			file.Key(organizationID, toDocumentID, newID),
			toDocumentID,
			organizationID,
		))
		if err != nil {
			logutil.Critical(h.log, err).Error(
				"cannot insert duplicated document file",
				slog.String("file_id", newID),
			)

			continue
		}

		err = h.storer.Copy(ctx, fromFolder, oldID, toFolder, newID)
		if err != nil {
			logutil.Critical(h.log, err).Error(
				"cannot copy duplicated document file",
				slog.String("file_id", newID),
			)
		}
	}
}

// FetchDocumentBranches handles the retrieval of all branches for a document.
func (h *Handler) FetchDocumentBranches(w http.ResponseWriter, r *http.Request) {
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	docID, err := httpserver.ExtractNamedID(r, "documentId")
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
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

	if sourceDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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

	if err := tx.CopyBranchTags(r.Context(), session.ActiveOrganizationID, sourceDoc.BranchID, newDoc.BranchID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDiff(nil, newDoc.Search())); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the hooks are copied after the commit: creating one creates its
	// external watcher too, and a rollback cannot take that back.
	if err := h.copyHooksToBranch(
		r.Context(),
		sourceDoc.BranchID,
		newDoc.BranchID,
		sourceDoc.ID,
		session.ActiveOrganizationID,
		nil,
	); err != nil {
		logutil.Critical(h.log, err).Error(
			"cannot copy hooks to the forked branch",
			slog.String("branch_id", newDoc.BranchID.String()),
		)
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
	session, ok := auth.RequireSession(h.log, w, r)
	if !ok {
		return
	}

	id, err := httpserver.ExtractNamedID(r, "documentId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchID, err := httpserver.ExtractNamedID(r, "branchId")
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	branchDoc, err := h.db.FetchDocumentByBranchID(r.Context(), branchID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if branchDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
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

	var tx Tx

	if err := h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err := tx.DeleteDocumentBranchByID(r.Context(), branchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := h.searchJobs.Enqueue(r.Context(), tx, search.BlocksDifference{
		RemovedBranches: []search.BranchRemoval{{DocumentID: branchDoc.ID, BranchID: branchID}},
	}); err != nil {
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
		nil,
		http.StatusNoContent,
	)
}

// copyHooksToBranch fetches all hooks from fromBranchID and re-creates them on
// toBranchID with fresh state. This is handler-level business logic; the DB
// layer is not involved in the re-creation decision.
//
// Creating a hook creates its external resource as a side effect, so this runs
// outside the caller's transaction: a rollback cannot take a changedetection.io
// watcher back, and the row that would have pointed at it is gone. A failed
// insert tears the watcher down again for the same reason.
//
// A branch whose content was duplicated carries fresh block uids, so the
// caller passes the old-to-new uid map and a hook anchored to a block is
// re-anchored through it; a hook whose block the map does not name has
// nothing to point at on the target and is dropped. A nil map keeps every
// block id as it is, which is right for a fork or a merge.
func (h *Handler) copyHooksToBranch(
	ctx context.Context,
	fromBranchID, toBranchID, documentID xid.ID,
	organizationID string,
	uids map[string]string,
) error {
	hooks, err := h.db.FetchDocumentHooksByBranchID(ctx, fromBranchID, organizationID)
	if err != nil {
		return err
	}

	inp := hook.NewInput(organizationID, h.githubMan, h.webchangeClient)

	for _, hk := range hooks {
		// a url-watcher cannot get its changedetection.io watcher without
		// the integration configured; dropping it from the copy beats
		// failing the whole fork or merge over it.
		if hk.Type == hook.TypeURLWatcher && !h.webchangeClient.Configured() {
			h.log.With("hook_id", hk.ID).
				Warn("skipping url-watcher hook copy: changedetection is not configured")

			continue
		}

		blockID := hk.BlockID

		if uids != nil && blockID.Valid {
			uid, ok := uids[blockID.String]
			if !ok {
				continue
			}

			blockID = null.StringFrom(uid)
		}

		newHk, err := hook.NewHook(ctx, hook.CreateInput{
			Type:     hk.Type,
			BranchID: toBranchID,
			BlockID:  blockID,
			Settings: hk.Settings,
		}, documentID, toBranchID, organizationID, inp)
		if err != nil {
			return err
		}

		if err := h.db.InsertDocumentHook(ctx, *newHk); err != nil {
			if derr := newHk.Delete(ctx, inp); derr != nil {
				logutil.Critical(h.log, derr).Error(
					"cannot delete the hook external resource after a failed insert",
					slog.String("hook_id", newHk.ID.String()),
				)
			}

			return err
		}
	}

	return nil
}

// upsertBranchReviewer records the reviewer's approval state, inserting the
// row when the user is not yet a reviewer of the branch. PreviouslyApproved
// is never written here: UpdateBranchReviewer persists currently_approved
// alone, and the promotion on merge owns the other column.
func (h *Handler) upsertBranchReviewer(
	ctx context.Context,
	branchID xid.ID,
	userID string,
	organizationID string,
	approved bool,
) error {
	reviewer := documentCore.BranchReviewer{
		BranchID:          branchID,
		UserID:            userID,
		OrganizationID:    organizationID,
		CurrentlyApproved: approved,
	}

	_, err := h.db.FetchBranchReviewer(ctx, branchID, userID, organizationID)

	switch {
	case err == nil:
		return h.db.UpdateBranchReviewer(ctx, reviewer)
	case errutil.IsNotFound(err):
		return h.db.InsertBranchReviewer(ctx, reviewer)
	default:
		return err
	}
}

// insertDocumentTx inserts a new document together with its maintainer and
// its search job, and slots it at the top of its parent's tree, all in one
// transaction. A document duplicated from a branch also takes that branch's
// tags. The tree-change notification is left to the caller, since it must
// not fire before the commit.
func (h *Handler) insertDocumentTx(
	ctx context.Context,
	doc documentCore.Document,
	sourceBranchID null.Value[xid.ID],
	session auth.Session,
) error {
	var tx Tx

	if err := h.db.BeginTx(ctx, &tx); err != nil {
		return err
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if doc.ParentID.Valid {
		if err := tx.CheckDocumentExists(ctx, doc.ParentID.V, session.ActiveOrganizationID); err != nil {
			return err
		}
	}

	if err := tx.InsertDocument(ctx, doc); err != nil {
		return err
	}

	if sourceBranchID.Valid {
		if err := tx.CopyBranchTags(ctx, session.ActiveOrganizationID, sourceBranchID.V, doc.BranchID); err != nil {
			return err
		}
	}

	if err := tx.UpsertDocumentMaintainers(
		ctx,
		doc.ID,
		session.ActiveOrganizationID,
		[]string{session.UserID},
	); err != nil {
		return err
	}

	if err := h.searchJobs.Enqueue(ctx, tx, search.BlocksDiff(nil, doc.Search())); err != nil {
		return err
	}

	tree, err := tx.FetchDocumentTreeByDocumentParentID(ctx, doc.ParentID, session.ActiveOrganizationID)
	if err != nil {
		return err
	}

	swappedTree, err := tree.Swap(doc.ID, 0)
	if err != nil {
		return err
	}

	if err = tx.UpdateDocumentTree(ctx, swappedTree, session.ActiveOrganizationID); err != nil {
		return err
	}

	return tx.Commit()
}

// DB is an interface that combines sqlutil.DB and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB
	DBAgent
}

// Tx is an interface that combines sqlutil.Tx and DBAgent.
//
//go:generate ../../../../scripts/codegen/mock -t internal Tx tx
type Tx interface {
	sqlutil.Tx
	DBAgent
}

// DBAgent is an interface that handles communication with the document database.
type DBAgent interface {
	HooksDBAgent
	ReviewersDBAgent
	DocumentsDBAgent
	BranchesDBAgent
	TreeDBAgent
	MaintainersDBAgent
	FilesDBAgent

	// DeleteDocumentCommentsByBranchID should delete all comments for a branch.
	DeleteDocumentCommentsByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error
}

// HooksDBAgent is an interface that handles communication with the document
// hooks database, covering the hook operations needed by document and branch
// lifecycle handlers (branch forking, merging, and document deletion).
type HooksDBAgent interface {
	// InsertDocumentHook should insert the document hook.
	InsertDocumentHook(ctx context.Context, hk hook.Hook) error

	// FetchDocumentHooksByDocumentID should fetch all hooks for a document across all branches.
	// Used for full document cleanup (e.g. on document deletion).
	FetchDocumentHooksByDocumentID(ctx context.Context, documentID xid.ID, organizationID string) ([]hook.Hook, error)

	// FetchDocumentHooksByBranchID should fetch all hooks for a specific branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error)

	// SoftDeleteDocumentHooksByBranchID should mark all hooks for a branch as soft-deleted
	// so the hook manager job can handle external resource cleanup.
	SoftDeleteDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error
}

// DocumentsDBAgent is an interface that handles communication with the
// document entity database.
type DocumentsDBAgent interface {
	// InsertDocument should insert the document.
	InsertDocument(ctx context.Context, doc documentCore.Document) error

	// InsertDocumentSearchJob should insert the document search job.
	InsertDocumentSearchJob(ctx context.Context, diff search.BlocksDifference) error

	// CheckDocumentExists returns nil if the document exists and belongs to the given organization.
	CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error

	// CheckDocumentCycle should report whether making parentID the parent of id
	// would create a cycle in the document tree.
	CheckDocumentCycle(ctx context.Context, id, parentID xid.ID, organizationID string) (bool, error)

	// FetchDocument should fetch a document joined against the given branch for the given id.
	FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*documentCore.Document, error)

	// FetchDocumentByBranchID should fetch a document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*documentCore.Document, error)

	// FetchDocumentUnsafeByBranchID should fetch the document joined against
	// the branch identified by branchID without checking organization ownership.
	// This is intended only for internal system use cases.
	FetchDocumentUnsafeByBranchID(ctx context.Context, branchID xid.ID) (*documentCore.Document, error)

	// UpdateDocument should update the document.
	UpdateDocument(ctx context.Context, doc documentCore.Document) error

	// DeleteDocument should delete the document and report the ids of
	// the document and of every cascade-deleted descendant.
	DeleteDocument(ctx context.Context, id xid.ID, organizationID string) ([]xid.ID, error)
}

// BranchesDBAgent is an interface that handles communication with the
// document branches database.
type BranchesDBAgent interface {
	// FetchDocumentBranchesUnsafe should fetch all branches for a document as
	// lightweight summaries without checking organization ownership.
	// This is intended only for internal system use cases.
	FetchDocumentBranchesUnsafe(ctx context.Context, docID xid.ID) ([]documentCore.BranchSummary, error)

	// ForkDocumentBranch should create a new branch by copying the contents of an existing source branch.
	ForkDocumentBranch(ctx context.Context, docID xid.ID, orgID, sourceBranch, targetBranch, createdBy string) error

	// FetchDocumentBranches should fetch all branches for a document as lightweight summaries.
	FetchDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) ([]documentCore.BranchSummary, error)

	// CountDocumentBranches should return the number of branches for a document.
	CountDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) (int, error)

	// DeleteDocumentBranchByID should delete a branch identified by its ID.
	DeleteDocumentBranchByID(ctx context.Context, branchID xid.ID, organizationID string) error

	// UpdateDocumentBranchMetadata should update the name and protection status of a branch
	// without modifying content or inserting a history entry.
	UpdateDocumentBranchMetadata(ctx context.Context, doc documentCore.Document) error

	// CopyBranchTags should make the target branch carry every tag the
	// source branch carries, on top of its own.
	CopyBranchTags(ctx context.Context, organizationID string, fromBranchID, toBranchID xid.ID) error

	// ReplaceBranchTags should make the target branch carry exactly the
	// tags the source branch carries.
	ReplaceBranchTags(ctx context.Context, organizationID string, fromBranchID, toBranchID xid.ID) error
}

// TreeDBAgent is an interface that handles communication with the document
// tree database.
type TreeDBAgent interface {
	// FetchDocumentTree should fetch the document tree.
	FetchDocumentTree(ctx context.Context, organizationID string) (documentCore.Summaries, error)

	// FetchDocumentTreeByParentID should fetch the document tree for the given parent id.
	FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (documentCore.Summaries, error)

	// UpdateDocumentTree should update the tree of the document childrens.
	UpdateDocumentTree(ctx context.Context, ss documentCore.Summaries, organizationID string) error

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
	FetchBranchReviewers(ctx context.Context, branchID xid.ID, organizationID string) ([]documentCore.BranchReviewer, error)

	// FetchBranchReviewer should fetch a single reviewer for a branch by user ID.
	FetchBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) (*documentCore.BranchReviewer, error)

	// InsertBranchReviewer should insert a reviewer for a branch.
	InsertBranchReviewer(ctx context.Context, reviewer documentCore.BranchReviewer) error

	// UpdateBranchReviewer should update the approval state of a branch reviewer.
	UpdateBranchReviewer(ctx context.Context, reviewer documentCore.BranchReviewer) error

	// DeleteBranchReviewer should remove a reviewer from a branch.
	DeleteBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) error

	// PromoteBranchApprovals should promote reviewer approvals from one branch to another.
	PromoteBranchApprovals(ctx context.Context, fromBranchID, toBranchID xid.ID, organizationID string) error
}

// FilesDBAgent is an interface that handles communication with the document
// files database, covering the file operations needed when a document is
// duplicated.
type FilesDBAgent interface {
	// FetchDocumentFile should fetch the document file for the given block id.
	FetchDocumentFile(ctx context.Context, id, organizationID string) (*file.File, error)

	// InsertDocumentFile should insert the document file.
	InsertDocumentFile(ctx context.Context, f file.File) error
}

// Storer is an interface that defines the object storage operations needed
// when a document is duplicated.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Copy should copy an object within the storage.
	Copy(ctx context.Context, srcFolder, srcID, dstFolder, dstID string) error
}

// SearchGateway is an interface that handles communication with the search engine.
//
//go:generate ../../../../scripts/codegen/mock -t internal SearchGateway
type SearchGateway interface {
	// Configured should report whether search is configured on this
	// deployment.
	Configured() bool

	// SearchDocuments should find the documents matching the query.
	SearchDocuments(ctx context.Context, organizationID, query string) ([]byte, error)
}
