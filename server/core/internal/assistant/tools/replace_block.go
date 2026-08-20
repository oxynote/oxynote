package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
)

// replaceBlock swaps an existing block for a new one.
type replaceBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *replaceBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameReplaceBlock,
		"Replace an existing block by uid with a new block. The replacement keeps the same position in its parent. Useful for changing a block's type or its full structure. Block uid is generated server-side if omitted.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block being replaced."),
			_keyBlock:      _blockSchema,
		},
		_keyDocumentID, _keyBlockUID, _keyBlock,
	)
}

// Label announces which document is being updated.
func (t *replaceBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm describes the replacement the model wants to make.
func (t *replaceBlock) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	kind := blockKindLabel(t.blockType(args))

	return t.summarize(ctx, NameReplaceBlock, args, func(subject string) string {
		return fmt.Sprintf("Replace a block in %s with %s", subject, kind)
	})
}

// InvokableRun validates the replacement and applies it.
func (t *replaceBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string      `json:"document_id"`
		BlockUID   string      `json:"block_uid"`
		Block      block.Block `json:"block"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("replace_block: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("replace_block: block_uid is required")
	}

	// the replacement lands where the target sits, so the target is what
	// decides whether this is a root placement.
	if err := t.validatePlacement(ctx, in.DocumentID, in.BlockUID, in.Block); err != nil {
		return "", fmt.Errorf("replace_block: %w", err)
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.Replace(in.BlockUID, in.Block)})
}
