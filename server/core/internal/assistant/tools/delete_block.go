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

// deleteBlock removes a block from a document.
type deleteBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *deleteBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameDeleteBlock,
		"Delete a block from the document by uid. This is destructive — the user is always asked to confirm and there is no auto-approve.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block to delete."),
		},
		_keyDocumentID, _keyBlockUID,
	)
}

// Destructive keeps this tool outside any "approve all" answer.
func (t *deleteBlock) Destructive() {}

// Label announces which document is being updated.
func (t *deleteBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm describes the deletion the model wants to make.
func (t *deleteBlock) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	return t.summarize(ctx, NameDeleteBlock, args, func(subject string) string {
		return "Delete a block in " + subject
	})
}

// InvokableRun removes the block.
func (t *deleteBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("delete_block: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("delete_block: block_uid is required")
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Delete(in.BlockUID)})
}
