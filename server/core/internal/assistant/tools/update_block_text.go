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

// updateBlockText replaces the inline text of a text-bearing block.
type updateBlockText struct{ *Input }

// Info returns the tool's model-facing description.
func (t *updateBlockText) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameUpdateBlockText,
		"Replace the inline text of a text-bearing block (paragraph, heading, blockquote, code, mermaid, callout-shorthand). The block's type and attrs are preserved. text uses the canonical minimal-markdown subset (**bold**, *italic*, _underline_, ~~strike~~, `code`, [label](url)). For code/mermaid blocks, text is raw and markdown is not parsed.",
		map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block whose text should be replaced."),
			"text":         stringProp("New inline text in canonical markdown."),
		},
		_keyDocumentID, _keyBlockUID, "text",
	)
}

// Label announces which document is being updated.
func (t *updateBlockText) Label(ctx context.Context, args json.RawMessage) string {
	return "Updating " + t.subject(ctx, args)
}

// Confirm previews the text the model wants to write.
func (t *updateBlockText) Confirm(ctx context.Context, args json.RawMessage) ConfirmActionSummary {
	var in struct {
		Text string `json:"text"`
	}

	t.parseToolArgs(args, &in)

	preview := textPreview(in.Text, _maxPreviewLen)

	return t.summarize(ctx, NameUpdateBlockText, args, func(subject string) string {
		if preview == "" {
			return "Update text of a block in " + subject
		}

		return fmt.Sprintf("Update a block in %s: %q", subject, preview)
	})
}

// InvokableRun writes the new text.
func (t *updateBlockText) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
		Text       string `json:"text"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("update_block_text: invalid input: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("update_block_text: block_uid is required")
	}

	return t.applyEdit(ctx, in.DocumentID, []edit.Operation{edit.UpdateText(in.BlockUID, in.Text)})
}
