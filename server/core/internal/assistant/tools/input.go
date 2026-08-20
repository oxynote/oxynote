package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// Input carries everything a tool needs to do its work. Every tool
// embeds one, so a tool file names only the arguments it takes and the
// work it does, never the plumbing it needs to get there.
//
// It is built per session and scoped to a single (organization, user)
// pair, so cross-org access is impossible by construction.
type Input struct {
	// log is scoped to the session's (org, user) and used to record
	// per-tool outcomes so we can diagnose AI loops without
	// re-running the conversation.
	log *slog.Logger

	// db is the persistence used by read tools and the non-content
	// write tools (create/delete/move).
	db DB

	// search is the full-text index behind search_documents.
	search Searcher

	// applier is the edit client for content mutations and the
	// rename/set-icon ops that must propagate to connected editors.
	applier EditApplier

	// tree notifies tree-change subscribers after the assistant
	// mutates the document tree.
	tree TreeNotifier

	// orgID scopes every tool call to one organization.
	orgID string

	// userID identifies the user the assistant is acting for. Used
	// when a tool creates audit-relevant rows (the created_by fields
	// on a new document, for instance).
	userID string
}

// NewInput creates a fresh instance of Input. Every dependency is
// required; nil values surface as nil-pointer panics on the first tool
// call rather than at startup, but in practice the cmd-level wiring
// passes all of them.
func NewInput(
	log *slog.Logger,
	db DB,
	searcher Searcher,
	applier EditApplier,
	tree TreeNotifier,
	orgID, userID string,
) *Input {
	return &Input{
		log: log.With(
			"component", "assistant-tools",
			"org_id", orgID,
			"user_id", userID,
		),
		db:      db,
		search:  searcher,
		applier: applier,
		tree:    tree,
		orgID:   orgID,
		userID:  userID,
	}
}

// docRef wraps the (documentID, branchID) pair the edit client needs to
// address a live Y.Doc. The branch is resolved to the document's
// default branch — multi-branch editing is out of scope for the
// assistant.
type docRef struct {
	DocumentID string
	BranchID   string
	OrgID      string
}

// resolveDoc loads the default branch of the given document and returns
// the ids the edit client needs. The lookup also acts as the cross-org
// safety check — db.FetchDocument scopes by orgID so a docID from
// another organisation surfaces as NotFound.
func (i *Input) resolveDoc(ctx context.Context, documentID string) (docRef, error) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return docRef{}, fmt.Errorf("document_id is not a valid xid: %w", err)
	}

	doc, err := i.db.FetchDocument(ctx, docID, i.orgID, document.DefaultBranch)
	if err != nil {
		return docRef{}, fmt.Errorf("fetch document: %w", err)
	}

	return docRef{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
		OrgID:      doc.OrganizationID,
	}, nil
}

// applyEdit is the shared tail of every content-mutating write tool: it
// resolves the document to a (documentID, branchID) pair, ships the
// operation batch to Node, and surfaces the per-op result. Outcomes are
// logged so partial failures on the Node side (uid not found, malformed
// block) are visible without re-running the conversation.
func (i *Input) applyEdit(
	ctx context.Context,
	documentID string,
	ops []edit.Operation,
) (string, error) {
	ref, err := i.resolveDoc(ctx, documentID)
	if err != nil {
		i.log.Warn(
			"edit resolve failed",
			slog.String("document_id", documentID),
			slog.String("error", err.Error()),
		)

		return "", err
	}

	res, err := i.applier.Apply(ctx, ref.DocumentID, ref.BranchID, ops)
	if err != nil {
		i.log.Error(
			"edit apply failed",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("op_count", len(ops)),
			slog.String("error", err.Error()),
		)

		return "", fmt.Errorf("apply edit: %w", err)
	}

	if len(res.Errors) > 0 {
		i.log.Warn(
			"edit partial failure",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("applied", res.Applied),
			slog.Any("errors", res.Errors),
		)
	} else {
		i.log.Debug(
			"edit applied",
			slog.String("document_id", ref.DocumentID),
			slog.String("branch_id", ref.BranchID),
			slog.Int("applied", res.Applied),
		)
	}

	return result(res)
}

// validatePlacement validates a block that is about to land next to, or
// in place of, the block referenceUID names. Types the document root
// accepts are legal wherever their parent takes them, so only a macro
// internal — a titled_code, metric or param_list, which the editor's
// schema binds to its container — has to look at where it is going.
func (i *Input) validatePlacement(
	ctx context.Context,
	documentID, referenceUID string,
	b block.Block,
) error {
	if err := block.Validate(b); err != nil {
		return err
	}

	if block.ValidateAsRoot(b) == nil {
		return nil
	}

	docID, err := xid.FromString(documentID)
	if err != nil {
		return fmt.Errorf("document_id is not a valid xid: %w", err)
	}

	content, err := i.db.FetchMainBranchContent(ctx, docID, i.orgID)
	if err != nil {
		return fmt.Errorf("fetch content: %w", err)
	}

	for _, rb := range content.Content.Content {
		if uid, ok := rb.UID(); ok && uid == referenceUID {
			return block.ValidateAsRoot(b)
		}
	}

	return nil
}

// notifyTreeChange invokes the tree notifier when one is configured.
// Safe to call from any tool — a nil notifier silently no-ops so tests
// that don't wire one don't trip.
func (i *Input) notifyTreeChange(parentID null.Value[xid.ID]) {
	if i.tree == nil {
		return
	}

	i.tree.NotifyTreeChange(i.orgID, parentID)
}

// notifyTreeChangeForDocument looks up the document's current parent
// and fires a tree-change for that parent. Used by rename/icon ops
// which don't carry a parent in their args. Failures (e.g. doc fetched
// after delete) silently skip the notification.
func (i *Input) notifyTreeChangeForDocument(ctx context.Context, documentID string) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return
	}

	doc, err := i.db.FetchDocument(ctx, docID, i.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return
	}

	i.notifyTreeChange(doc.ParentID)
}

// lookupDocumentName fetches the document's display name. Failures (bad
// id, not found, transient) return an empty string so the confirm UI
// gracefully falls back to the id alone.
func (i *Input) lookupDocumentName(ctx context.Context, documentID string) string {
	id, err := xid.FromString(documentID)
	if err != nil {
		return ""
	}

	doc, err := i.db.FetchDocument(ctx, id, i.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return ""
	}

	return doc.DocumentName
}

// parseToolArgs unmarshals a tool's JSON args into dst for the
// best-effort label and summary helpers: a malformed args payload
// should degrade the label, not abort the surrounding flow. So we log a
// warning and let dst keep its zero value rather than propagating the
// error. The provider enforces the input schema at the tool-call
// boundary, so a failure here is unusual and worth knowing about.
func (i *Input) parseToolArgs(args json.RawMessage, dst any) {
	if err := json.Unmarshal(args, dst); err != nil {
		i.log.Warn("tool args unmarshal failed",
			slog.String("error", err.Error()),
		)
	}
}

// subject returns the display subject for a tool's label and summary:
// the named document when it can be resolved, a generic fallback
// otherwise.
func (i *Input) subject(ctx context.Context, args json.RawMessage) string {
	var probe struct {
		DocumentID string `json:"document_id"`
	}

	i.parseToolArgs(args, &probe)

	if probe.DocumentID == "" {
		return subjectFor("")
	}

	return subjectFor(i.lookupDocumentName(ctx, probe.DocumentID))
}

// summarize builds a confirmation for a write that targets an existing
// document: the document is resolved once, and the tool supplies only
// the phrasing for its own change.
func (i *Input) summarize(
	ctx context.Context,
	name Name,
	args json.RawMessage,
	phrase func(subject string) string,
) ConfirmActionSummary {
	out := ConfirmActionSummary{Tool: string(name)}

	if docID := i.documentID(args); docID != "" {
		out.DocumentID = docID
		out.DocumentName = i.lookupDocumentName(ctx, docID)
	}

	out.Summary = phrase(subjectFor(out.DocumentName))

	return out
}

// documentID returns the document a tool's args target, or an empty
// string when the args name none.
func (i *Input) documentID(args json.RawMessage) string {
	var probe struct {
		DocumentID string `json:"document_id"`
	}

	i.parseToolArgs(args, &probe)

	return probe.DocumentID
}
