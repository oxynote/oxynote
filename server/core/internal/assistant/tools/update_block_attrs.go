package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
)

// updateBlockAttrs sets named attributes on an existing block.
type updateBlockAttrs struct{ *Input }

// Info returns the tool's model-facing description.
func (t *updateBlockAttrs) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameUpdateBlockAttrs,
		"Set or override the named attributes on an existing block. Unmentioned attributes are preserved; the uid attribute cannot be changed.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block whose attrs should be updated."),
			"attrs": map[string]any{
				_keyType:        _typeObject,
				_keyDescription: "Attribute keys and values to set (e.g. {\"level\": 2}, {\"icon\": \"lucide:warning\"}).",
			},
		},
		_keyDocumentID, _keyBlockUID, "attrs",
	)
}

// Label announces which document is being updated.
func (t *updateBlockAttrs) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm names the attributes the model wants to set.
func (t *updateBlockAttrs) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		Attrs map[string]any `json:"attrs"`
	}

	t.parseToolArgs(args, &in)

	keys := make([]string, 0, len(in.Attrs))
	for k := range in.Attrs {
		keys = append(keys, k)
	}

	return t.summarize(ctx, NameUpdateBlockAttrs, args, func(subject string) string {
		if len(keys) == 0 {
			return "Update block attributes in " + subject
		}

		return fmt.Sprintf("Update block %s in %s", strings.Join(keys, ", "), subject)
	})
}

// InvokableRun applies the attribute changes.
func (t *updateBlockAttrs) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string         `json:"document_id"`
		BlockUID   string         `json:"block_uid"`
		Attrs      map[string]any `json:"attrs"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("update_block_attrs: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("update_block_attrs: block_uid is required")
	}

	if len(in.Attrs) == 0 {
		return "", errors.New("update_block_attrs: attrs must not be empty")
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.UpdateAttrs(in.BlockUID, in.Attrs)})
}
