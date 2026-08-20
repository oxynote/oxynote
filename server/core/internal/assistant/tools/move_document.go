package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// moveDocument re-parents a document within the organisation.
type moveDocument struct{ *Input }

// Info returns the tool's model-facing description.
func (t *moveDocument) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameMoveDocument,
		"Re-parent a document. Omit new_parent_id to move the document to the org root.",
		map[string]any{
			_keyDocumentID:  stringProp("The document to move."),
			"new_parent_id": stringProp("Optional new parent document id; omit to move to the root."),
		},
		_keyDocumentID,
	)
}

// Label announces which document is being moved.
func (t *moveDocument) Label(ctx context.Context, args json.RawMessage) string {
	return "Moving " + t.subject(ctx, args)
}

// Confirm describes the move the model wants to make.
func (t *moveDocument) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		NewParentID string `json:"new_parent_id"`
	}

	t.parseToolArgs(args, &in)

	return t.summarize(ctx, NameMoveDocument, args, func(subject string) string {
		if in.NewParentID == "" {
			return fmt.Sprintf("Move %s to the org root", subject)
		}

		return fmt.Sprintf("Move %s under another document", subject)
	})
}

// InvokableRun re-parents the document, refusing to create a cycle.
func (t *moveDocument) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID  string `json:"document_id"`
		NewParentID string `json:"new_parent_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("move_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("move_document: document_id is not a valid xid: %w", err)
	}

	doc, err := t.db.FetchDocument(ctx, docID, t.orgID, document.DefaultBranch)
	if err != nil {
		return "", fmt.Errorf("move_document: document not found: %w", err)
	}

	oldParent := doc.ParentID

	var newParent null.Value[xid.ID]

	if in.NewParentID != "" {
		pid, perr := xid.FromString(in.NewParentID)
		if perr != nil {
			return "", fmt.Errorf("move_document: new_parent_id is not a valid xid: %w", perr)
		}

		if cerr := t.db.CheckDocumentExists(ctx, pid, t.orgID); cerr != nil {
			return "", fmt.Errorf("move_document: new parent not found: %w", cerr)
		}

		cycle, cerr := t.db.CheckDocumentCycle(ctx, docID, pid, t.orgID)
		if cerr != nil {
			return "", fmt.Errorf("move_document: parent check: %w", cerr)
		}

		if cycle {
			return "", errors.New(
				"move_document: a document cannot be moved under itself or one of its descendants",
			)
		}

		newParent = null.ValueFrom(pid)
	}

	if err := t.db.UpdateDocumentParentID(ctx, docID, newParent, t.orgID); err != nil {
		return "", fmt.Errorf("move_document: update: %w", err)
	}

	// both the source and destination subtrees changed shape; tell
	// subscribers about each. When the move is within the same parent
	// the second call is a duplicate but harmless.
	t.notifyTreeChange(oldParent)

	if oldParent != newParent {
		t.notifyTreeChange(newParent)
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

	return result(out)
}
