package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// readBlock returns the full canonical content of one block.
type readBlock struct{ *Input }

// Info returns the tool's model-facing description.
func (t *readBlock) Info(_ context.Context) (*schema.ToolInfo, error) {
	return toolInfo(
		NameReadBlock,
		"Return the full canonical content of one block by uid, including any nested children. Use this only when read_document_summary doesn't carry enough detail (e.g. you need the full structure of a split_doc or split_doc_param_list to edit it).",
		map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyBlockUID:   stringProp("The block uid to fetch."),
		},
		_keyDocumentID, _keyBlockUID,
	)
}

// Label announces which document the block is being read from.
func (t *readBlock) Label(ctx context.Context, args json.RawMessage) string {
	return "Reading a block in " + t.subject(ctx, args)
}

// InvokableRun fetches and compacts the named block.
func (t *readBlock) InvokableRun(
	ctx context.Context,
	argumentsInJSON string,
	_ ...tool.Option,
) (string, error) {
	var in struct {
		DocumentID string `json:"document_id"`
		BlockUID   string `json:"block_uid"`
	}

	if err := json.Unmarshal([]byte(argumentsInJSON), &in); err != nil {
		return "", fmt.Errorf("read_block: invalid input: %w", err)
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("read_block: document_id is not a valid xid: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("read_block: block_uid is required")
	}

	content, err := t.db.FetchMainBranchContent(ctx, docID, t.orgID)
	if err != nil {
		return "", fmt.Errorf("read_block: fetch content: %w", err)
	}

	blk, ok := content.Content.FindByUID(in.BlockUID)
	if !ok {
		return "", fmt.Errorf("read_block: block %q not found: %w", in.BlockUID, errutil.ErrNotFound)
	}

	canon, err := block.Compact(blk)
	if err != nil {
		return "", fmt.Errorf("read_block: compact: %w", err)
	}

	return result(canon)
}
