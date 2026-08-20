package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// getDocument returns one document's metadata.
type getDocument struct{ *Input }

// Info returns the tool's model-facing description.
func (t *getDocument) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameGetDocument,
		"Fetch metadata for one document: name, icon, parent_id, default branch id, protected flag, updated_at.",
		documentIDProp(_descDocumentID),
		_keyDocumentID,
	)
}

// Label returns no status line: a metadata read is too noisy to announce.
func (t *getDocument) Label(_ context.Context, _ json.RawMessage) string {
	return ""
}

// InvokableRun fetches the document's metadata.
func (t *getDocument) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("get_document: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("get_document: document_id is not a valid xid: %w", err)
	}

	doc, err := t.db.FetchDocument(ctx, docID, t.orgID, document.DefaultBranch)
	if err != nil {
		return "", fmt.Errorf("get_document: fetch: %w", err)
	}

	out := struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Icon       string `json:"icon"`
		ParentID   string `json:"parent_id,omitempty"`
		BranchID   string `json:"branch_id"`
		BranchName string `json:"branch_name"`
		Protected  bool   `json:"protected"`
		UpdatedAt  string `json:"updated_at"`
	}{
		ID:         doc.ID.String(),
		Name:       doc.DocumentName,
		Icon:       doc.Icon,
		BranchID:   doc.BranchID.String(),
		BranchName: doc.BranchName,
		Protected:  doc.Protected,
		UpdatedAt:  doc.UpdatedAt.UTC().Format(time.RFC3339),
	}

	if doc.ParentID.Valid {
		out.ParentID = doc.ParentID.V.String()
	}

	return result(out)
}
