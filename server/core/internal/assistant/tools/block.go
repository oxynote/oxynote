package tools

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// _maxPreviewLen caps the quoted text preview shown in the confirm UI.
const _maxPreviewLen = 60

// readDocumentSummaryArgs is what read_document_summary is called with.
type readDocumentSummaryArgs struct {
	// DocumentID names the document being summarised.
	DocumentID string `json:"document_id"`
}

// readDocumentSummary returns a compact, ordered view of a document.
type readDocumentSummary struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (readDocumentSummary) Info() Info {
	return Info{
		Name:        NameReadDocumentSummary,
		Description: "Return an ordered, compact summary of a document: per-block uid, kind, flattened source_text, surrounding_context (ancestor headings, list intro, doc title), and tags. Use this as your default way to read a document — it's ~5-10x cheaper than fetching full content.",
		Properties:  documentIDProp(_descDocumentID),
		Required:    []string{_keyDocumentID},
	}
}

// Traits reports a plain read.
func (readDocumentSummary) Traits() Traits {
	return Traits{}
}

// Title announces which document is being read.
func (readDocumentSummary) Title(inp DescribeInput) (string, error) {
	var in readDocumentSummaryArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Reading " + inp.Subject(in.DocumentID), nil
}

// Execute summarises the document's default branch.
func (readDocumentSummary) Execute(inp Input) (string, error) {
	var in readDocumentSummaryArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("read_document_summary: document_id is not a valid xid: %w", err)
	}

	content, err := inp.DocumentContent(docID)
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

// readBlockArgs is what read_block is called with.
type readBlockArgs struct {
	// DocumentID names the document holding the block.
	DocumentID string `json:"document_id"`

	// BlockUID is the block being read. Required.
	BlockUID string `json:"block_uid"`
}

// readBlock returns the full canonical content of one block.
type readBlock struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (readBlock) Info() Info {
	return Info{
		Name:        NameReadBlock,
		Description: "Return the full canonical content of one block by uid, including any nested children. Use this only when read_document_summary doesn't carry enough detail (e.g. you need the full structure of a split_doc or split_doc_param_list to edit it).",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyBlockUID:   stringProp("The block uid to fetch."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBlockUID,
		},
	}
}

// Traits reports a plain read.
func (readBlock) Traits() Traits {
	return Traits{}
}

// Title announces which document the block is being read from.
func (readBlock) Title(inp DescribeInput) (string, error) {
	var in readBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Reading a block in " + inp.Subject(in.DocumentID), nil
}

// Execute fetches and compacts the named block.
func (readBlock) Execute(inp Input) (string, error) {
	var in readBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	docID, err := xid.FromString(in.DocumentID)
	if err != nil {
		return "", fmt.Errorf("read_block: document_id is not a valid xid: %w", err)
	}

	if in.BlockUID == "" {
		return "", errors.New("read_block: block_uid is required")
	}

	content, err := inp.DocumentContent(docID)
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

// insertBlock places a canonical block beside a referenced block.
type insertBlock struct{}

// Info returns the tool's model-facing description.
func (insertBlock) Info() Info {
	return Info{
		Name:        NameInsertBlock,
		Description: "Insert a single canonical block before or after a referenced block in the document. The reference block stays in place. Block uid is generated server-side if omitted.",
		Properties: map[string]any{
			_keyDocumentID:        stringProp(_descTargetDocumentID),
			"reference_block_uid": stringProp("The uid of the block to insert relative to."),
			"position": map[string]any{
				_keyType: _typeString,
				"enum": []string{
					"before",
					"after",
				},
				_keyDescription: "Insert side relative to the reference block.",
			},
			_keyBlock: _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			"reference_block_uid",
			"position",
			_keyBlock,
		},
	}
}

// insertBlockArgs is what insert_block is called with.
type insertBlockArgs struct {
	// DocumentID names the document to insert into.
	DocumentID string `json:"document_id"`

	// ReferenceBlockUID is the block the insertion is positioned
	// against. Required.
	ReferenceBlockUID string `json:"reference_block_uid"`

	// Position is the side of the reference block to insert on,
	// "before" or "after".
	Position string `json:"position"`

	// Block is the block being inserted.
	Block block.Block `json:"block"`
}

// Traits reports a write.
func (insertBlock) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (insertBlock) Title(inp DescribeInput) (string, error) {
	var in insertBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary describes the insertion the model wants to make.
func (insertBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in insertBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	out := summarize(inp, NameInsertBlock, in.DocumentID, func(subject string) string {
		// echoing a missing or garbage position onto the confirm card
		// would garble it, so fall back to an un-positioned phrase.
		if in.Position != "before" && in.Position != "after" {
			return fmt.Sprintf("Insert %s in %s",
				blockKindLabel(in.Block.Type), subject)
		}

		return fmt.Sprintf("Insert %s %s a block in %s",
			blockKindLabel(in.Block.Type), in.Position, subject)
	})

	return out, nil
}

// Execute validates the placement and applies the insertion.
func (insertBlock) Execute(inp Input) (string, error) {
	var in insertBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.ReferenceBlockUID == "" {
		return "", errors.New("insert_block: reference_block_uid is required")
	}

	// the enum check is free while ValidatePlacement may hit the
	// database, so a bad position fails before any I/O happens.
	var op edit.Operation

	switch in.Position {
	case "before":
		op = edit.InsertBefore(in.ReferenceBlockUID, in.Block)
	case "after":
		op = edit.InsertAfter(in.ReferenceBlockUID, in.Block)
	default:
		return "", fmt.Errorf("insert_block: position must be \"before\" or \"after\", got %q", in.Position)
	}

	if err := inp.ValidatePlacement(in.DocumentID, in.ReferenceBlockUID, in.Block); err != nil {
		return "", fmt.Errorf("insert_block: %w", err)
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{op})
}

// rootBlockArgs is what append_block and prepend_block are called
// with. The two differ only in which end of the document they write to,
// so they take the same arguments.
type rootBlockArgs struct {
	// DocumentID names the document to write to.
	DocumentID string `json:"document_id"`

	// Block is the block being added.
	Block block.Block `json:"block"`
}

// appendBlock adds a canonical block at the end of a document.
type appendBlock struct{}

// Info returns the tool's model-facing description.
func (appendBlock) Info() Info {
	return Info{
		Name:        NameAppendBlock,
		Description: "Append a single canonical block at the end of a document. Block uid is generated server-side if omitted.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlock:      _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			_keyBlock,
		},
	}
}

// Traits reports a write.
func (appendBlock) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (appendBlock) Title(inp DescribeInput) (string, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary describes the block the model wants to append.
func (appendBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	out := summarize(inp, NameAppendBlock, in.DocumentID, func(subject string) string {
		return fmt.Sprintf("Append %s to %s", blockKindLabel(in.Block.Type), subject)
	})

	return out, nil
}

// Execute validates the block and appends it.
func (appendBlock) Execute(inp Input) (string, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return "", fmt.Errorf("append_block: %w", err)
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.Append(in.Block)})
}

// prependBlock adds a canonical block at the start of a document.
type prependBlock struct{}

// Info returns the tool's model-facing description.
func (prependBlock) Info() Info {
	return Info{
		Name:        NamePrependBlock,
		Description: "Prepend a single canonical block at the start of a document. Block uid is generated server-side if omitted.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlock:      _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			_keyBlock,
		},
	}
}

// Traits reports a write.
func (prependBlock) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (prependBlock) Title(inp DescribeInput) (string, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary describes the block the model wants to prepend.
func (prependBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	out := summarize(inp, NamePrependBlock, in.DocumentID, func(subject string) string {
		return fmt.Sprintf("Prepend %s to %s", blockKindLabel(in.Block.Type), subject)
	})

	return out, nil
}

// Execute validates the block and prepends it.
func (prependBlock) Execute(inp Input) (string, error) {
	var in rootBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := block.ValidateAsRoot(in.Block); err != nil {
		return "", fmt.Errorf("prepend_block: %w", err)
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.Prepend(in.Block)})
}

// replaceBlockArgs is what replace_block is called with.
type replaceBlockArgs struct {
	// DocumentID names the document holding the block.
	DocumentID string `json:"document_id"`

	// BlockUID is the block being replaced. Required.
	BlockUID string `json:"block_uid"`

	// Block is what takes its place.
	Block block.Block `json:"block"`
}

// replaceBlock swaps an existing block for a new one.
type replaceBlock struct{}

// Info returns the tool's model-facing description.
func (replaceBlock) Info() Info {
	return Info{
		Name:        NameReplaceBlock,
		Description: "Replace an existing block by uid with a new block. The replacement keeps the same position in its parent. Useful for changing a block's type or its full structure. Block uid is generated server-side if omitted.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block being replaced."),
			_keyBlock:      _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			_keyBlockUID,
			_keyBlock,
		},
	}
}

// Traits reports a write.
func (replaceBlock) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (replaceBlock) Title(inp DescribeInput) (string, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary describes the replacement the model wants to make.
func (replaceBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	out := summarize(inp, NameReplaceBlock, in.DocumentID, func(subject string) string {
		return fmt.Sprintf("Replace a block in %s with %s", subject, blockKindLabel(in.Block.Type))
	})

	return out, nil
}

// Execute validates the replacement and applies it.
func (replaceBlock) Execute(inp Input) (string, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.BlockUID == "" {
		return "", errors.New("replace_block: block_uid is required")
	}

	// the replacement lands where the target sits, so the target is what
	// decides whether this is a root placement.
	if err := inp.ValidatePlacement(in.DocumentID, in.BlockUID, in.Block); err != nil {
		return "", fmt.Errorf("replace_block: %w", err)
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.Replace(in.BlockUID, in.Block)})
}

// updateBlockTextArgs is what update_block_text is called with.
type updateBlockTextArgs struct {
	// DocumentID names the document holding the block.
	DocumentID string `json:"document_id"`

	// BlockUID is the block whose text is being written. Required.
	BlockUID string `json:"block_uid"`

	// Text is the new inline content.
	Text string `json:"text"`
}

// updateBlockText replaces the inline text of a text-bearing block.
type updateBlockText struct{}

// Info returns the tool's model-facing description.
func (updateBlockText) Info() Info {
	return Info{
		Name:        NameUpdateBlockText,
		Description: "Replace the inline text of a text-bearing block (paragraph, heading, blockquote, code, mermaid, callout-shorthand). The block's type and attrs are preserved. text uses the canonical minimal-markdown subset (**bold**, *italic*, _underline_, ~~strike~~, `code`, [label](url)). For code/mermaid blocks, text is raw and markdown is not parsed.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block whose text should be replaced."),
			"text":         stringProp("New inline text in canonical markdown."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBlockUID,
			"text",
		},
	}
}

// Traits reports a write.
func (updateBlockText) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (updateBlockText) Title(inp DescribeInput) (string, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary previews the text the model wants to write.
func (updateBlockText) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	preview := textPreview(in.Text, _maxPreviewLen)

	out := summarize(inp, NameUpdateBlockText, in.DocumentID, func(subject string) string {
		if preview == "" {
			return "Update text of a block in " + subject
		}

		return fmt.Sprintf("Update a block in %s: %q", subject, preview)
	})

	return out, nil
}

// Execute writes the new text.
func (updateBlockText) Execute(inp Input) (string, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.BlockUID == "" {
		return "", errors.New("update_block_text: block_uid is required")
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.UpdateText(in.BlockUID, in.Text)})
}

// updateBlockAttrsArgs is what update_block_attrs is called with.
type updateBlockAttrsArgs struct {
	// DocumentID names the document holding the block.
	DocumentID string `json:"document_id"`

	// BlockUID is the block whose attributes are being set. Required.
	BlockUID string `json:"block_uid"`

	// Attrs are the attributes to set. Must not be empty.
	Attrs map[string]any `json:"attrs"`
}

// updateBlockAttrs sets named attributes on an existing block.
type updateBlockAttrs struct{}

// Info returns the tool's model-facing description.
func (updateBlockAttrs) Info() Info {
	return Info{
		Name:        NameUpdateBlockAttrs,
		Description: "Set or override the named attributes on an existing block. Unmentioned attributes are preserved; the uid attribute cannot be changed.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block whose attrs should be updated."),
			"attrs": map[string]any{
				_keyType:        _typeObject,
				_keyDescription: "Attribute keys and values to set (e.g. {\"level\": 2}, {\"icon\": \"lucide:warning\"}).",
			},
		},
		Required: []string{
			_keyDocumentID,
			_keyBlockUID,
			"attrs",
		},
	}
}

// Traits reports a write.
func (updateBlockAttrs) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (updateBlockAttrs) Title(inp DescribeInput) (string, error) {
	var in updateBlockAttrsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary names the attributes the model wants to set.
func (updateBlockAttrs) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateBlockAttrsArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	keys := make([]string, 0, len(in.Attrs))
	for k := range in.Attrs {
		keys = append(keys, k)
	}

	// map iteration order is random, and the confirm card must read
	// the same every time the same write is proposed.
	slices.Sort(keys)

	out := summarize(inp, NameUpdateBlockAttrs, in.DocumentID, func(subject string) string {
		if len(keys) == 0 {
			return "Update block attributes in " + subject
		}

		return fmt.Sprintf("Update block %s in %s", strings.Join(keys, ", "), subject)
	})

	return out, nil
}

// Execute applies the attribute changes.
func (updateBlockAttrs) Execute(inp Input) (string, error) {
	var in updateBlockAttrsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.BlockUID == "" {
		return "", errors.New("update_block_attrs: block_uid is required")
	}

	if len(in.Attrs) == 0 {
		return "", errors.New("update_block_attrs: attrs must not be empty")
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.UpdateAttrs(in.BlockUID, in.Attrs)})
}

// deleteBlockArgs is what delete_block is called with.
type deleteBlockArgs struct {
	// DocumentID names the document holding the block.
	DocumentID string `json:"document_id"`

	// BlockUID is the block being removed. Required.
	BlockUID string `json:"block_uid"`
}

// deleteBlock removes a block from a document.
type deleteBlock struct{}

// Info returns the tool's model-facing description.
func (deleteBlock) Info() Info {
	return Info{
		Name:        NameDeleteBlock,
		Description: "Delete a block from the document by uid. This is destructive — the user is always asked to confirm and there is no auto-approve.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBlockUID:   stringProp("The uid of the block to delete."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBlockUID,
		},
	}
}

// Traits reports a destructive write, which stays outside any "approve
// all" answer.
func (deleteBlock) Traits() Traits {
	return Traits{Write: true, Destructive: true}
}

// Title announces which document is being updated.
func (deleteBlock) Title(inp DescribeInput) (string, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	return "Updating " + inp.Subject(in.DocumentID), nil
}

// Summary describes the deletion the model wants to make.
func (deleteBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	out := summarize(inp, NameDeleteBlock, in.DocumentID, func(subject string) string {
		return "Delete a block in " + subject
	})

	return out, nil
}

// Execute removes the block.
func (deleteBlock) Execute(inp Input) (string, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.BlockUID == "" {
		return "", errors.New("delete_block: block_uid is required")
	}

	return inp.ApplyEdit(in.DocumentID, []edit.Operation{edit.Delete(in.BlockUID)})
}
