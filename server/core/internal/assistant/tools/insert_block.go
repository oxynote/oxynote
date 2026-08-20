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

// insertBlock places a canonical block beside a referenced block.
type insertBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *insertBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameInsertBlock,
		"Insert a single canonical block before or after a referenced block in the document. The reference block stays in place. Block uid is generated server-side if omitted.",
		map[string]any{
			_keyDocumentID:        stringProp(_descTargetDocumentID),
			"reference_block_uid": stringProp("The uid of the block to insert relative to."),
			"position": map[string]any{
				_keyType:        _typeString,
				"enum":          []string{"before", "after"},
				_keyDescription: "Insert side relative to the reference block.",
			},
			_keyBlock: _blockSchema,
		},
		_keyDocumentID, "reference_block_uid", "position", _keyBlock,
	)
}

// Label announces which document is being updated.
func (t *insertBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm describes the insertion the model wants to make.
func (t *insertBlock) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		Position string `json:"position"`
		Block    struct {
			Type string `json:"type"`
		} `json:"block"`
	}

	t.parseToolArgs(args, &in)

	return t.summarize(ctx, NameInsertBlock, args, func(subject string) string {
		return fmt.Sprintf("Insert %s %s a block in %s",
			blockKindLabel(in.Block.Type), in.Position, subject)
	})
}

// InvokableRun validates the placement and applies the insertion.
func (t *insertBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID        string      `json:"document_id"`
		ReferenceBlockUID string      `json:"reference_block_uid"`
		Position          string      `json:"position"`
		Block             block.Block `json:"block"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("insert_block: invalid input: %w", err)
	}

	if in.ReferenceBlockUID == "" {
		return "", errors.New("insert_block: reference_block_uid is required")
	}

	if err := t.validatePlacement(ctx, in.DocumentID, in.ReferenceBlockUID, in.Block); err != nil {
		return "", fmt.Errorf("insert_block: %w", err)
	}

	var op edit.Operation

	switch in.Position {
	case "before":
		op = edit.InsertBefore(in.ReferenceBlockUID, in.Block)
	case "after":
		op = edit.InsertAfter(in.ReferenceBlockUID, in.Block)
	default:
		return "", fmt.Errorf("insert_block: position must be \"before\" or \"after\", got %q", in.Position)
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{op})
}
