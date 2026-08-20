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

// appendBlock adds a canonical block at the end of a document.
type appendBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *appendBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameAppendBlock,
		"Append a single canonical block at the end of a document. Block uid is generated server-side if omitted.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlock:      _blockSchema,
		},
		_keyDocumentID, _keyBlock,
	)
}

// Label announces which document is being updated.
func (t *appendBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm describes the block the model wants to append.
func (t *appendBlock) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	kind := blockKindLabel(t.blockType(args))

	return t.summarize(ctx, NameAppendBlock, args, func(subject string) string {
		return fmt.Sprintf("Append %s to %s", kind, subject)
	})
}

// InvokableRun validates the block and appends it.
func (t *appendBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("append_block: invalid input: %w", err)
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return "", fmt.Errorf("append_block: %w", err)
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Append(in.Block)})
}
