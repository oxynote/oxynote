package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// deleteDocument removes a document from the organisation.
type deleteDocument struct{ *Input }

// Info returns the tool's model-facing description.
func (t *deleteDocument) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameDeleteDocument,
		"Delete a document. This is destructive — the user is always asked to confirm and there is no auto-approve.",
		documentIDProp("The document id to delete."),
		_keyDocumentID,
	)
}

// Destructive keeps this tool outside any "approve all" answer.
func (t *deleteDocument) Destructive() {}

// Label announces which document is being deleted.
func (t *deleteDocument) Label(ctx context.Context, args json.RawMessage) string {
	return "Deleting " + t.subject(ctx, args)
}

// Confirm describes the document the model wants to delete.
func (t *deleteDocument) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	return t.summarize(ctx, NameDeleteDocument, args, func(subject string) string {
		return "Delete " + subject
	})
}

// InvokableRun deletes the document and refreshes the tree.
func (t *deleteDocument) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("delete_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("delete_document: document_id is not a valid xid: %w", err)
	}

	// capture the parent before the row goes away so we can scope the
	// tree-change notification to the affected subtree.
	var parentID null.Value[xid.ID]

	if doc, ferr := t.db.FetchDocument(ctx, docID, t.orgID, document.DefaultBranch); ferr == nil && doc != nil {
		parentID = doc.ParentID
	}

	if err := t.db.DeleteDocument(ctx, docID, t.orgID); err != nil {
		return "", fmt.Errorf("delete_document: delete: %w", err)
	}

	t.notifyTreeChange(parentID)

	return result(struct {
		DocumentID string `json:"document_id"`
		Deleted    bool   `json:"deleted"`
	}{
		DocumentID: docID.String(),
		Deleted:    true,
	})
}
