package document

import (
	"net/http"

	"github.com/guregu/null/v5"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/rs/xid"
)

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

	if err := doc.AllowsBranchRename(ui.Name); err != nil {
		httpserver.RespondError(h.log, w, err)
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
		if err := tx.InsertSearchJob(r.Context(), search.BranchScope(session.ActiveOrganizationID, ndoc.ID, ndoc.BranchID)); err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.searchTrigger.Trigger()

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

	// a persist that changes nothing, such as the Yjs seed Hocuspocus
	// sends on first load, leaves history alone. Only a persist that names
	// its editors is attributed. A system write has no user, and without
	// maintainers the branch's author is whoever edited before.
	if !doc.SnapshotEqual(ndoc) {
		var lastUpdatedBy null.String
		if !ui.System && len(ui.Maintainers) > 0 {
			lastUpdatedBy = ndoc.LastUpdatedBy
		}

		_, err = tx.RecordDocumentBranchHistoryEntry(
			r.Context(),
			doc.BranchID,
			doc.OrganizationID,
			lastUpdatedBy,
			false,
		)
		if err != nil {
			httpserver.RespondError(h.log, w, err)
			return
		}
	}

	maintainers, err := tx.FetchDocumentMaintainers(r.Context(), doc.ID, doc.OrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	maintainersAdded := ui.AddsMaintainers(maintainers)

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

	if err = tx.InsertSearchJob(r.Context(), search.BranchScope(doc.OrganizationID, doc.ID, doc.BranchID)); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	err = tx.Commit()
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.searchTrigger.Trigger()

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

// MergeBranches merges the content of a source branch into a target branch.
// The target branch's hooks are detached for the hook manager to tear down
// and replaced with copies from the source.
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

	if err = documentCore.AllowsMerge(inp.FromBranchID, inp.ToBranchID); err != nil {
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

	if fromDoc.ID != id || toDoc.ID != id {
		httpserver.RespondError(h.log, w, ErrBranchMismatch)
		return
	}

	ndoc := toDoc.MergeBranch(fromDoc.Branch, session.UserID)

	var tx Tx

	if err = h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.DetachDocumentHooksByBranchID(r.Context(), toDoc.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.DeleteDocumentCommentsByBranchID(r.Context(), toDoc.BranchID, session.ActiveOrganizationID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err = tx.UpdateDocument(r.Context(), ndoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the copies go in before the entry is recorded, so it lists them.
	err = h.hookMan.CopyHooks(
		r.Context(),
		tx,
		fromDoc.BranchID,
		toDoc.BranchID,
		toDoc.ID,
		session.ActiveOrganizationID,
		nil,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = tx.RecordDocumentBranchHistoryEntry(
		r.Context(),
		toDoc.BranchID,
		session.ActiveOrganizationID,
		null.StringFrom(session.UserID),
		true,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.ReplaceBranchTags(r.Context(), session.ActiveOrganizationID, fromDoc.BranchID, toDoc.BranchID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.InsertSearchJob(r.Context(), search.BranchScope(session.ActiveOrganizationID, toDoc.ID, toDoc.BranchID)); err != nil {
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

	h.searchTrigger.Trigger()
	h.hookMan.ProcessBranch(toDoc.BranchID, session.ActiveOrganizationID)

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

	newDoc := sourceDoc.Fork(inp.Branch, session.UserID)

	var tx Tx

	if err = h.db.BeginTx(r.Context(), &tx); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	defer tx.Rollback() //nolint:errcheck // error provides no meaningful info

	if err = tx.InsertDocumentBranch(r.Context(), newDoc); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	// the copies go in before the entry is recorded, so it lists them.
	err = h.hookMan.CopyHooks(
		r.Context(),
		tx,
		sourceDoc.BranchID,
		newDoc.BranchID,
		sourceDoc.ID,
		session.ActiveOrganizationID,
		nil,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	_, err = tx.RecordDocumentBranchHistoryEntry(
		r.Context(),
		newDoc.BranchID,
		session.ActiveOrganizationID,
		null.StringFrom(session.UserID),
		true,
	)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.CopyBranchTags(r.Context(), session.ActiveOrganizationID, sourceDoc.BranchID, newDoc.BranchID); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.InsertSearchJob(r.Context(), search.BranchScope(session.ActiveOrganizationID, newDoc.ID, newDoc.BranchID)); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.searchTrigger.Trigger()
	h.hookMan.ProcessBranch(newDoc.BranchID, session.ActiveOrganizationID)

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

	if err = branchDoc.AllowsBranchDelete(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	count, err := h.db.CountDocumentBranches(r.Context(), branchDoc.ID, session.ActiveOrganizationID)
	if err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if count <= 1 {
		httpserver.RespondError(h.log, w, documentCore.ErrLastBranchDelete)
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

	if err := tx.InsertSearchJob(r.Context(), search.BranchScope(session.ActiveOrganizationID, branchDoc.ID, branchID)); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	if err := tx.Commit(); err != nil {
		httpserver.RespondError(h.log, w, err)
		return
	}

	h.searchTrigger.Trigger()

	httpserver.Respond(
		h.log,
		w,
		nil,
		http.StatusNoContent,
	)
}
