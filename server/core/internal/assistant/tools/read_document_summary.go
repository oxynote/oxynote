package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/rs/xid"
)

// readDocumentSummary returns a compact, ordered view of a document.
type readDocumentSummary struct{ *Input }

// Info returns the tool's model-facing description.
func (t *readDocumentSummary) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameReadDocumentSummary,
		"Return an ordered, compact summary of a document: per-block uid, kind, flattened source_text, surrounding_context (ancestor headings, list intro, doc title), and tags. Use this as your default way to read a document — it's ~5-10x cheaper than fetching full content.",
		documentIDProp(_descDocumentID),
		_keyDocumentID,
	)
}

// Label announces which document is being read.
func (t *readDocumentSummary) Label(ctx context.Context, args json.RawMessage) string {
	return "Reading " + t.subject(ctx, args)
}

// InvokableRun summarises the document's default branch.
func (t *readDocumentSummary) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("read_document_summary: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("read_document_summary: document_id is not a valid xid: %w", err)
	}

	content, err := t.db.FetchMainBranchContent(ctx, docID, t.orgID)
	if err != nil {
		return "", fmt.Errorf("read_document_summary: fetch content: %w", err)
	}

	return result(struct {
		DocumentID   string            `json:"document_id"`
		DocumentName string            `json:"document_name"`
		Blocks       []docSummaryEntry `json:"blocks"`
	}{
		DocumentID:   docID.String(),
		DocumentName: content.DocumentName,
		Blocks:       walkDocForAssistant(content.Content.Content),
	})
}
