package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _defaultDocumentIcon is the icon assigned to assistant-created
// documents when the model doesn't pick one.
const _defaultDocumentIcon = "lucide:file"

// docRef wraps the (documentID, branchID) pair the edit client
// needs to address a live Y.Doc. The branch is resolved to the
// document's default branch — multi-branch editing is out of scope
// for the assistant.
type docRef struct {
	DocumentID string
	BranchID   string
	OrgID      string
}

// resolveDoc loads the default branch of the given document and
// returns the ids the edit client needs. The lookup also acts
// as the cross-org safety check — db.FetchDocument scopes by orgID
// so a docID from another organisation surfaces as NotFound.
func (m *Manager) resolveDoc(ctx context.Context, documentID string) (docRef, error) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return docRef{}, fmt.Errorf("document_id is not a valid xid: %w", err)
	}

	doc, err := m.db.FetchDocument(ctx, docID, m.orgID, document.DefaultBranch)
	if err != nil {
		return docRef{}, fmt.Errorf("fetch document: %w", err)
	}

	return docRef{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
		OrgID:      doc.OrganizationID,
	}, nil
}

func (m *Manager) createDocument(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		Name     string `json:"name"`
		Icon     string `json:"icon"`
		ParentID string `json:"parent_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("create_document: invalid input: %w", err)
	}

	if in.Name == "" {
		return nil, errors.New("create_document: name is required")
	}

	icon := in.Icon
	if icon == "" {
		icon = _defaultDocumentIcon
	}

	var parentID null.Value[xid.ID]

	if in.ParentID != "" {
		pid, err := xid.FromString(in.ParentID)
		if err != nil {
			return nil, fmt.Errorf("create_document: parent_id is not a valid xid: %w", err)
		}

		if err := m.db.CheckDocumentExists(ctx, pid, m.orgID); err != nil {
			return nil, fmt.Errorf("create_document: parent not found: %w", err)
		}

		parentID = null.ValueFrom(pid)
	}

	doc := document.NewDocument(document.CreateInput{
		Name:     in.Name,
		Icon:     icon,
		ParentID: parentID,
	}, m.orgID, m.userID)

	if err := m.db.InsertDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("create_document: insert: %w", err)
	}

	// Mirror the HTTP create path: whoever asked for the document
	// becomes its first maintainer so they own it from the start.
	if err := m.db.UpsertDocumentMaintainers(ctx, doc.ID, m.orgID, []string{m.userID}); err != nil {
		return nil, fmt.Errorf("create_document: upsert maintainers: %w", err)
	}

	m.notifyTreeChange(doc.ParentID)

	return marshalResult(struct {
		DocumentID string `json:"document_id"`
		BranchID   string `json:"branch_id"`
	}{
		DocumentID: doc.ID.String(),
		BranchID:   doc.BranchID.String(),
	})
}

func (m *Manager) deleteDocument(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("delete_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("delete_document: document_id is not a valid xid: %w", err)
	}

	// Capture the parent before the row goes away so we can scope
	// the tree-change notification to the affected subtree.
	var parentID null.Value[xid.ID]

	if doc, ferr := m.db.FetchDocument(ctx, docID, m.orgID, document.DefaultBranch); ferr == nil && doc != nil {
		parentID = doc.ParentID
	}

	if err := m.db.DeleteDocument(ctx, docID, m.orgID); err != nil {
		return nil, fmt.Errorf("delete_document: delete: %w", err)
	}

	m.notifyTreeChange(parentID)

	return marshalResult(struct {
		DocumentID string `json:"document_id"`
		Deleted    bool   `json:"deleted"`
	}{
		DocumentID: docID.String(),
		Deleted:    true,
	})
}

func (m *Manager) renameDocument(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Name       string `json:"name"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("rename_document: invalid input: %w", err)
	}

	if in.Name == "" {
		return nil, errors.New("rename_document: name is required")
	}

	out, err := m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.SetName(in.Name)})
	if err != nil {
		return nil, err
	}

	m.notifyTreeChangeForDocument(ctx, in.DocumentID)

	return out, nil
}

func (m *Manager) setDocumentIcon(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Icon       string `json:"icon"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("set_document_icon: invalid input: %w", err)
	}

	if in.Icon == "" {
		return nil, errors.New("set_document_icon: icon is required")
	}

	out, err := m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.SetIcon(in.Icon)})
	if err != nil {
		return nil, err
	}

	m.notifyTreeChangeForDocument(ctx, in.DocumentID)

	return out, nil
}

func (m *Manager) moveDocument(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID  string `json:"document_id"`
		NewParentID string `json:"new_parent_id"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("move_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("move_document: document_id is not a valid xid: %w", err)
	}

	doc, err := m.db.FetchDocument(ctx, docID, m.orgID, document.DefaultBranch)
	if err != nil {
		return nil, fmt.Errorf("move_document: document not found: %w", err)
	}

	oldParent := doc.ParentID

	var newParent null.Value[xid.ID]

	if in.NewParentID != "" {
		pid, err := xid.FromString(in.NewParentID)
		if err != nil {
			return nil, fmt.Errorf("move_document: new_parent_id is not a valid xid: %w", err)
		}

		if err := m.db.CheckDocumentExists(ctx, pid, m.orgID); err != nil {
			return nil, fmt.Errorf("move_document: new parent not found: %w", err)
		}

		cycle, cerr := m.db.CheckDocumentCycle(ctx, docID, pid, m.orgID)
		if cerr != nil {
			return nil, fmt.Errorf("move_document: parent check: %w", cerr)
		}

		if cycle {
			return nil, errors.New(
				"move_document: a document cannot be moved under itself or one of its descendants",
			)
		}

		newParent = null.ValueFrom(pid)
	}

	if err := m.db.UpdateDocumentParentID(ctx, docID, newParent, m.orgID); err != nil {
		return nil, fmt.Errorf("move_document: update: %w", err)
	}

	// Both the source and destination subtrees changed shape; tell
	// subscribers about each. When the move is within the same
	// parent the second call is a duplicate but harmless.
	m.notifyTreeChange(oldParent)

	if oldParent != newParent {
		m.notifyTreeChange(newParent)
	}

	out := struct {
		DocumentID  string `json:"document_id"`
		NewParentID string `json:"new_parent_id,omitempty"`
	}{
		DocumentID: docID.String(),
	}

	if newParent.Valid {
		out.NewParentID = newParent.V.String()
	}

	return marshalResult(out)
}

func (m *Manager) insertBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID        string      `json:"document_id"`
		ReferenceBlockUID string      `json:"reference_block_uid"`
		Position          string      `json:"position"`
		Block             block.Block `json:"block"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("insert_block: invalid input: %w", err)
	}

	if in.ReferenceBlockUID == "" {
		return nil, errors.New("insert_block: reference_block_uid is required")
	}

	if err := block.Validate(in.Block); err != nil {
		return nil, fmt.Errorf("insert_block: %w", err)
	}

	var op edit.Operation

	switch in.Position {
	case "before":
		op = edit.InsertBefore(in.ReferenceBlockUID, in.Block)
	case "after":
		op = edit.InsertAfter(in.ReferenceBlockUID, in.Block)
	default:
		return nil, fmt.Errorf("insert_block: position must be \"before\" or \"after\", got %q", in.Position)
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{op})
}

func (m *Manager) appendBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("append_block: invalid input: %w", err)
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return nil, fmt.Errorf("append_block: %w", err)
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Append(in.Block)})
}

func (m *Manager) prependBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("prepend_block: invalid input: %w", err)
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return nil, fmt.Errorf("prepend_block: %w", err)
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Prepend(in.Block)})
}

func (m *Manager) replaceBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		BlockUID   string      `json:"block_uid"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("replace_block: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return nil, errors.New("replace_block: block_uid is required")
	}

	if err := block.Validate(in.Block); err != nil {
		return nil, fmt.Errorf("replace_block: %w", err)
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Replace(in.BlockUID, in.Block)})
}

func (m *Manager) updateBlockText(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
		Text       string `json:"text"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("update_block_text: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return nil, errors.New("update_block_text: block_uid is required")
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.UpdateText(in.BlockUID, in.Text)})
}

func (m *Manager) updateBlockAttrs(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string         `json:"document_id"`
		BlockUID   string         `json:"block_uid"`
		Attrs      map[string]any `json:"attrs"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("update_block_attrs: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return nil, errors.New("update_block_attrs: block_uid is required")
	}

	if len(in.Attrs) == 0 {
		return nil, errors.New("update_block_attrs: attrs must not be empty")
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.UpdateAttrs(in.BlockUID, in.Attrs)})
}

func (m *Manager) deleteBlock(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
	}

	if err := json.Unmarshal(args, &in); err != nil {
		return nil, fmt.Errorf("delete_block: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return nil, errors.New("delete_block: block_uid is required")
	}

	return m.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Delete(in.BlockUID)})
}

// notifyTreeChange invokes the tree notifier when one is configured.
// Safe to call from any tool — a nil notifier silently no-ops so
// tests that don't wire one don't trip.
func (m *Manager) notifyTreeChange(parentID null.Value[xid.ID]) {
	if m.tree == nil {
		return
	}

	m.tree.NotifyTreeChange(m.orgID, parentID)
}

// notifyTreeChangeForDocument looks up the document's current parent
// and fires a tree-change for that parent. Used by rename/icon ops
// which don't carry a parent in their args. Failures (e.g. doc
// fetched after delete) silently skip the notification.
func (m *Manager) notifyTreeChangeForDocument(ctx context.Context, documentID string) {
	docID, err := xid.FromString(documentID)
	if err != nil {
		return
	}

	doc, err := m.db.FetchDocument(ctx, docID, m.orgID, document.DefaultBranch)
	if err != nil || doc == nil {
		return
	}

	m.notifyTreeChange(doc.ParentID)
}

// applyEdit is the shared tail of every content-mutating write
// tool: it resolves the document to a (documentID, branchID) pair,
// ships the operation batch to Node, and surfaces the per-op
// result. Outcomes are logged so partial failures on the Node side
// (uid not found, malformed block) are visible without re-running
// the conversation.
func (m *Manager) applyEdit(ctx context.Context, documentID string, ops []edit.Operation) (json.RawMessage, error) {
	ref, err := m.resolveDoc(ctx, documentID)
	if err != nil {
		m.log.Warn("edit resolve failed",
			"document_id", documentID,
			"error", err.Error(),
		)

		return nil, err
	}

	res, err := m.applier.Apply(ctx, ref.DocumentID, ref.BranchID, ops)
	if err != nil {
		m.log.Error("edit apply failed",
			"document_id", ref.DocumentID,
			"branch_id", ref.BranchID,
			"op_count", len(ops),
			"error", err.Error(),
		)

		return nil, fmt.Errorf("apply edit: %w", err)
	}

	if len(res.Errors) > 0 {
		m.log.Warn("edit partial failure",
			"document_id", ref.DocumentID,
			"branch_id", ref.BranchID,
			"applied", res.Applied,
			"errors", res.Errors,
		)
	} else {
		m.log.Debug("edit applied",
			"document_id", ref.DocumentID,
			"branch_id", ref.BranchID,
			"applied", res.Applied,
		)
	}

	return marshalResult(res)
}
