package document

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	documentCore "github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
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

	if fromDoc.ID != id || toDoc.ID != id {
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

	if err := tx.DetachDocumentHooksByBranchID(r.Context(), toDoc.BranchID, session.ActiveOrganizationID); err != nil {
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

	h.copyHooksToBranch(
		r.Context(),
		fromDoc.BranchID,
		toDoc.BranchID,
		toDoc.ID,
		session.ActiveOrganizationID,
		nil,
	)

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

	h.copyHooksToBranch(
		r.Context(),
		sourceDoc.BranchID,
		newDoc.BranchID,
		sourceDoc.ID,
		session.ActiveOrganizationID,
		nil,
	)

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
// toBranchID with fresh state, once the target is committed. Creating a hook
// creates its external resource as a side effect, so the copy cannot run
// inside the caller's transaction: a rollback cannot take a changedetection.io
// watcher back, and the row that would have pointed at it is gone. A failed
// insert tears the just-created resource down again for the same reason. A
// failure stops the copy and leaves the target standing with fewer hooks than
// its source, which the critical log reports.
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
) {
	hooks, err := h.db.FetchDocumentHooksByBranchID(ctx, fromBranchID, organizationID)
	if err != nil {
		logutil.Critical(h.log, err).Error(
			"cannot fetch the hooks to copy",
			slog.String("branch_id", fromBranchID.String()),
		)

		return
	}

	inp := hook.NewInput(organizationID, h.githubMan, h.webchangeClient)

	for _, hk := range hooks {
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
			// a hook whose integration is not configured cannot get its
			// external resource; dropping it from the copy beats failing
			// the whole fork or merge over it.
			if errors.Is(err, webchange.ErrNotConfigured) {
				h.log.With("hook_id", hk.ID).
					Warn("skipping hook copy: its integration is not configured")

				continue
			}

			logutil.Critical(h.log, err).Error(
				"cannot re-create the hook on the target branch",
				slog.String("hook_id", hk.ID.String()),
				slog.String("branch_id", toBranchID.String()),
			)

			return
		}

		if err := h.db.InsertDocumentHook(ctx, *newHk); err != nil {
			if derr := newHk.Delete(ctx, inp); derr != nil {
				logutil.Critical(h.log, derr).Error(
					"cannot delete the hook external resource after a failed insert",
					slog.String("hook_id", newHk.ID.String()),
				)
			}

			logutil.Critical(h.log, err).Error(
				"cannot insert the copied hook",
				slog.String("hook_id", newHk.ID.String()),
				slog.String("branch_id", toBranchID.String()),
			)

			return
		}
	}
}
