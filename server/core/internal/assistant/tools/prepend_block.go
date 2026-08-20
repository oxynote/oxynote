package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
)

// prependBlock adds a canonical block at the start of a document.
type prependBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *prependBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NamePrependBlock,
		"Prepend a single canonical block at the start of a document.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlock:      _blockSchema,
		},
		_keyDocumentID, _keyBlock,
	)
}

// Label announces which document is being updated.
func (t *prependBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm describes the block the model wants to prepend.
func (t *prependBlock) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	kind := blockKindLabel(blockType(t.Input, args))

	return t.summarize(ctx, NamePrependBlock, args, func(subject string) string {
		return fmt.Sprintf("Prepend %s to %s", kind, subject)
	})
}

// InvokableRun validates the block and prepends it.
func (t *prependBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("prepend_block: invalid input: %w", err)
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return "", fmt.Errorf("prepend_block: %w", err)
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Prepend(in.Block)})
}
