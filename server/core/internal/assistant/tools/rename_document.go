package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
)

// renameDocument changes a document's display name.
type renameDocument struct{ *Input }

// Info returns the tool's model-facing description.
func (t *renameDocument) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameRenameDocument,
		"Change a document's display name. The change is applied live via hocuspocus.",
		map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			"name":         stringProp("The new display name."),
		},
		_keyDocumentID, "name",
	)
}

// Label announces which document is being renamed.
func (t *renameDocument) Label(ctx context.Context, args json.RawMessage) string {
	return "Renaming " + t.subject(ctx, args)
}

// Confirm describes the rename the model wants to make.
func (t *renameDocument) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		Name string `json:"name"`
	}

	t.parseToolArgs(args, &in)

	return t.summarize(ctx, NameRenameDocument, args, func(subject string) string {
		if in.Name == "" {
			return "Rename " + subject
		}

		return fmt.Sprintf("Rename %s to %q", subject, in.Name)
	})
}

// InvokableRun renames the document and refreshes the tree.
func (t *renameDocument) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		Name       string `json:"name"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("rename_document: invalid input: %w", err)
	}

	if in.Name == "" {
		return "", errors.New("rename_document: name is required")
	}

	out, err := t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.SetName(in.Name)})
	if err != nil {
		return "", err
	}

	t.notifyTreeChangeForDocument(ctx, in.DocumentID)

	return out, nil
}
