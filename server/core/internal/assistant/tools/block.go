package tools

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/rs/xid"
)

// _maxPreviewLen caps the quoted text preview shown in the confirm UI.
const _maxPreviewLen = 60

// docTarget names the document, and the branch of it, a content tool
// reads or writes. Both are required: the default branch is a branch
// like any other, and its id comes with every document listing.
type docTarget struct {
	// DocumentID names the document.
	DocumentID xid.ID `json:"document_id"`

	// BranchID names the branch.
	BranchID xid.ID `json:"branch_id"`
}

// validate checks the target names a document and a branch.
func (t docTarget) validate() error {
	if t.DocumentID.IsNil() {
		return errRequired(_keyDocumentID)
	}

	if t.BranchID.IsNil() {
		return errRequired(_keyBranchID)
	}

	return nil
}

// readBlockArgs is what read_block is called with.
type readBlockArgs struct {
	docTarget

	// BlockUID is the block being read. Required.
	BlockUID string `json:"block_uid"`
}

// Validate checks the arguments are complete.
func (a readBlockArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	return nil
}

// readBlock returns the full canonical content of one block.
type readBlock struct {
	plainSummary
}

// Info returns the tool's model-facing description.
func (readBlock) Info() Info {
	return Info{
		Name:        NameReadBlock,
		Description: "Return the full canonical content of one block by uid, including any nested children. Use it only when get_document's rows are not enough: to edit a split_doc, a nested list or a split_doc_param_list, whose inner structure has to be written back in full. Fails when the uid is not in the document.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The block uid to fetch."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
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

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameReadBlock, err)
	}

	return "Reading a block in " + doc.DocumentName, nil
}

// Execute fetches and compacts the named block.
func (readBlock) Execute(inp Input) (string, error) {
	var in readBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	content, err := inp.DocumentContent(in.DocumentID, in.BranchID)
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

// insertBlock places a canonical block in a document: beside a
// referenced block, or at either end of the document.
type insertBlock struct{}

// Info returns the tool's model-facing description.
func (insertBlock) Info() Info {
	return Info{
		Name:        NameInsertBlock,
		Description: "Insert one canonical block into a document. position start or end puts it at the document's start or end; before or after puts it beside the block reference_block_uid names, which stays in place. The block has to be legal where it lands: the document root takes every type except titled_code, metric and split_doc_param_list, and a reference inside a split_doc or metric_grid takes only what that container holds. Returns the summary rows of the new block and everything nested in it, uids included, with depth counted from the block itself.",
		Properties: map[string]any{
			_keyDocumentID:        stringProp(_descTargetDocumentID),
			_keyBranchID:          stringProp(_descBranchID),
			"reference_block_uid": stringProp("The uid of the block to insert beside. Required with before and after; leave it out with start and end."),
			"position": map[string]any{
				_keyType: _typeString,
				_keyEnum: []string{
					string(positionBefore),
					string(positionAfter),
					string(positionStart),
					string(positionEnd),
				},
				_keyDescription: "Where the block lands: before or after the reference block, or at the start or end of the document.",
			},
			_keyBlock: _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
			"position",
			_keyBlock,
		},
	}
}

const (
	// positionBefore places ahead of the reference block.
	positionBefore position = "before"

	// positionAfter places behind the reference block.
	positionAfter position = "after"

	// positionStart places at the start of the document.
	positionStart position = "start"

	// positionEnd places at the end of the document.
	positionEnd position = "end"
)

// position is where a block lands: a side of a reference block, or an
// end of the document.
type position string

// UnmarshalText parses the position, refusing anything outside the
// four values. The schema enum is what the model was shown; the decoder
// is where a value outside it gets reported, named by argument.
func (p *position) UnmarshalText(text []byte) error {
	switch v := position(text); v {
	case positionBefore, positionAfter, positionStart, positionEnd:
		*p = v

		return nil
	default:
		return fmt.Errorf("position must be one of %q, %q, %q or %q, got %q",
			positionBefore, positionAfter, positionStart, positionEnd, text)
	}
}

// relative reports whether the position is taken against a reference
// block rather than an end of the document.
func (p position) relative() bool {
	return p == positionBefore || p == positionAfter
}

// insertBlockArgs is what insert_block is called with.
type insertBlockArgs struct {
	docTarget

	// ReferenceBlockUID is the block the insertion is positioned
	// against. Required for before and after, and refused otherwise.
	ReferenceBlockUID string `json:"reference_block_uid"`

	// Position is where the block lands.
	Position position `json:"position"`

	// Block is the block being inserted.
	Block block.Block `json:"block"`
}

// Validate checks the arguments are complete and consistent.
func (a insertBlockArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.Position == "" {
		return errRequired("position")
	}

	if a.Position.relative() && a.ReferenceBlockUID == "" {
		return errRequired("reference_block_uid")
	}

	if !a.Position.relative() && a.ReferenceBlockUID != "" {
		return errors.New("reference_block_uid applies to before and after only; use before or after, or leave it out")
	}

	if a.Block.Type == "" {
		return errRequired(_keyBlock)
	}

	return nil
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

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameInsertBlock, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary describes the insertion the model wants to make.
func (insertBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in insertBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameInsertBlock, err)
	}

	kind := blockKindLabel(in.Block.Type)

	var summary string

	switch in.Position {
	case positionStart:
		summary = fmt.Sprintf("Prepend %s to %s", kind, docLabel(doc))
	case positionEnd:
		summary = fmt.Sprintf("Append %s to %s", kind, docLabel(doc))
	default:
		summary = fmt.Sprintf("Insert %s %s a block in %s", kind, in.Position, docLabel(doc))
	}

	return ActionSummary{
		Tool:         NameInsertBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute validates the placement and applies the insertion.
func (insertBlock) Execute(inp Input) (string, error) {
	var in insertBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if in.Position.relative() {
		if err := inp.ValidatePlacement(in.DocumentID, in.BranchID, in.ReferenceBlockUID, in.Block); err != nil {
			return "", fmt.Errorf("insert_block: %w", err)
		}
	} else if err := block.ValidateAsRoot(in.Block); err != nil {
		return "", fmt.Errorf("insert_block: %w", err)
	}

	if err := inp.CheckDataSources(in.Block.CollectAttributeValues(document.AttrDataSourceID)); err != nil {
		return "", fmt.Errorf("insert_block: %w", err)
	}

	// uids are resolved at expansion, so expanding here rather than at
	// wire time is what lets the tool report what it wrote without
	// reading the document back, which the debounced persist would not
	// reliably show yet.
	expanded, err := block.Expand(in.Block)
	if err != nil {
		return "", fmt.Errorf("insert_block: %w", err)
	}

	var op edit.Operation

	switch in.Position {
	case positionBefore:
		op = edit.InsertBefore(in.ReferenceBlockUID, expanded)
	case positionAfter:
		op = edit.InsertAfter(in.ReferenceBlockUID, expanded)
	case positionStart:
		op = edit.Prepend(expanded)
	default:
		op = edit.Append(expanded)
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{op}); err != nil {
		return "", err
	}

	return result(blockWriteResult{Blocks: blockRows(expanded)})
}

// blockWriteResult is what a block write returns: the rows of the block
// it wrote or touched, in the shape get_document lists them.
type blockWriteResult struct {
	// Blocks is the written block and everything nested in it, depth
	// counted from the block itself.
	Blocks []docSummaryEntry `json:"blocks"`
}

// replaceBlockArgs is what replace_block is called with.
type replaceBlockArgs struct {
	docTarget

	// BlockUID is the block being replaced. Required.
	BlockUID string `json:"block_uid"`

	// Block is what takes its place.
	Block block.Block `json:"block"`
}

// Validate checks the arguments are complete.
func (a replaceBlockArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	if a.Block.Type == "" {
		return errRequired(_keyBlock)
	}

	return nil
}

// replaceBlock swaps an existing block for a new one.
type replaceBlock struct{}

// Info returns the tool's model-facing description.
func (replaceBlock) Info() Info {
	return Info{
		Name:        NameReplaceBlock,
		Description: "Replace a block by uid with a new block in the same position. The old block's uid, content and children are all gone unless the new block carries them, so use it to change a block's type or its whole structure. For a wording change use update_block_text, and for an attribute change update_block_attrs; both keep the uid, which comments, hooks and files hang off. Returns the summary rows of the new block and everything nested in it, uids included, with depth counted from the block itself.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The uid of the block being replaced."),
			_keyBlock:      _blockSchema,
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
			_keyBlockUID,
			_keyBlock,
		},
	}
}

// Traits reports a write that overwrites: the replacement takes the
// target's place whole, so every nested block and uid under it goes.
func (replaceBlock) Traits() Traits {
	return Traits{Write: true, Overwrites: true}
}

// Title announces which document is being updated.
func (replaceBlock) Title(inp DescribeInput) (string, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameReplaceBlock, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary describes the replacement the model wants to make.
func (replaceBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameReplaceBlock, err)
	}

	return ActionSummary{
		Tool:         NameReplaceBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Replace a block in %s with %s", doc.DocumentName, blockKindLabel(in.Block.Type)),
	}, nil
}

// Execute validates the replacement and applies it.
func (replaceBlock) Execute(inp Input) (string, error) {
	var in replaceBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// the replacement lands where the target sits, so the target is what
	// decides whether this is a root placement.
	if err := inp.ValidatePlacement(in.DocumentID, in.BranchID, in.BlockUID, in.Block); err != nil {
		return "", fmt.Errorf("replace_block: %w", err)
	}

	if err := inp.CheckDataSources(in.Block.CollectAttributeValues(document.AttrDataSourceID)); err != nil {
		return "", fmt.Errorf("replace_block: %w", err)
	}

	expanded, err := block.Expand(in.Block)
	if err != nil {
		return "", fmt.Errorf("replace_block: %w", err)
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{edit.Replace(in.BlockUID, expanded)}); err != nil {
		return "", err
	}

	return result(blockWriteResult{Blocks: blockRows(expanded)})
}

// updateBlockTextArgs is what update_block_text is called with.
type updateBlockTextArgs struct {
	docTarget

	// BlockUID is the block whose text is being written. Required.
	BlockUID string `json:"block_uid"`

	// Text is the new inline content.
	Text string `json:"text"`
}

// Validate checks the arguments are complete.
func (a updateBlockTextArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	if a.Text == "" {
		return errRequired("text")
	}

	return nil
}

// updateBlockText replaces the inline text of a text-bearing block.
type updateBlockText struct{}

// Info returns the tool's model-facing description.
func (updateBlockText) Info() Info {
	return Info{
		Name:        NameUpdateBlockText,
		Description: "Replace the inline text of one text-bearing block: paragraph, heading, blockquote, code, titled_code, mermaid, or a callout written with text. Type, attrs and uid are kept, so this is the tool for wording changes. Text follows the canonical markdown subset (**bold**, *italic*, _underline_, ~~strike~~, backtick code, [label](url)), and is raw in code, titled_code and mermaid. One block is one paragraph; to add a paragraph, insert a block instead. Returns the block's summary row with the new text.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The uid of the block whose text should be replaced."),
			"text":         stringProp("New inline text in canonical markdown."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
			_keyBlockUID,
			"text",
		},
	}
}

// Traits reports a write that overwrites: the new text replaces the
// block's whole content, nested blocks and their uids included.
func (updateBlockText) Traits() Traits {
	return Traits{Write: true, Overwrites: true}
}

// Title announces which document is being updated.
func (updateBlockText) Title(inp DescribeInput) (string, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameUpdateBlockText, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary previews the text the model wants to write.
func (updateBlockText) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	preview := textPreview(in.Text, _maxPreviewLen)

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameUpdateBlockText, err)
	}

	summary := fmt.Sprintf("Update a block in %s: %q", doc.DocumentName, preview)
	if preview == "" {
		summary = "Update text of a block in " + docLabel(doc)
	}

	return ActionSummary{
		Tool:         NameUpdateBlockText,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute writes the new text.
func (updateBlockText) Execute(inp Input) (string, error) {
	var in updateBlockTextArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	b, err := inp.DocumentBlock(in.DocumentID, in.BranchID, in.BlockUID)
	if err != nil {
		return "", fmt.Errorf("update_block_text: %w", err)
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{edit.UpdateText(in.BlockUID, in.Text)}); err != nil {
		return "", err
	}

	// the write replaces the block's whole content with the text, so
	// the row describes the block as it now stands: the new text, read
	// the way every row reads (flattened, markers dropped), and nothing
	// nested under it.
	rows := blockRows(b)[:1]
	rows[0].Text = document.Block{Type: document.BlockNodeParagraph, Content: block.ParseInlineMarkdown(in.Text)}.Flatten()
	rows[0].HasChildren = false

	return result(blockWriteResult{Blocks: rows})
}

// updateBlockAttrsArgs is what update_block_attrs is called with.
type updateBlockAttrsArgs struct {
	docTarget

	// BlockUID is the block whose attributes are being set. Required.
	BlockUID string `json:"block_uid"`

	// Attrs are the attributes to set. Must not be empty.
	Attrs map[string]any `json:"attrs"`
}

// Validate checks the arguments are complete.
func (a updateBlockAttrsArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	if len(a.Attrs) == 0 {
		return errRequired("attrs")
	}

	return nil
}

// updateBlockAttrs sets named attributes on an existing block.
type updateBlockAttrs struct{}

// Info returns the tool's model-facing description.
func (updateBlockAttrs) Info() Info {
	return Info{
		Name:        NameUpdateBlockAttrs,
		Description: "Set or override named attributes on an existing block, such as a heading's level or a callout's icon. Attributes not mentioned are kept and uid cannot change. Values are validated for the block's type, so a level outside 1 to 3 or a metric width other than compact, standard or wide is rejected. Use replace_block when the type itself has to change. Returns the summary rows of the block and everything nested in it, attrs as they now stand.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The uid of the block whose attrs should be updated."),
			"attrs": map[string]any{
				_keyType:        _typeObject,
				_keyDescription: "Attribute keys and values to set (e.g. {\"level\": 2}, {\"icon\": \"lucide:warning\"}).",
			},
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
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

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameUpdateBlockAttrs, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary names the attributes the model wants to set.
func (updateBlockAttrs) Summary(inp DescribeInput) (ActionSummary, error) {
	var in updateBlockAttrsArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	// map iteration order is random, and the confirm card must read
	// the same every time the same write is proposed.
	keys := slices.Sorted(maps.Keys(in.Attrs))

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameUpdateBlockAttrs, err)
	}

	return ActionSummary{
		Tool:         NameUpdateBlockAttrs,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Update block %s in %s", strings.Join(keys, ", "), docLabel(doc)),
	}, nil
}

// Execute applies the attribute changes.
func (updateBlockAttrs) Execute(inp Input) (string, error) {
	var in updateBlockAttrsArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// the payload names attributes, not a block type, so a metric's
	// data source arrives here on its own rather than inside a block
	// the walk could find it in.
	if id, ok := in.Attrs[document.AttrDataSourceID].(string); ok && id != "" {
		if err := inp.CheckDataSources([]string{id}); err != nil {
			return "", fmt.Errorf("update_block_attrs: %w", err)
		}
	}

	if err := inp.ValidateAttrUpdate(in.DocumentID, in.BranchID, in.BlockUID, in.Attrs); err != nil {
		return "", fmt.Errorf("update_block_attrs: %w", err)
	}

	b, err := inp.DocumentBlock(in.DocumentID, in.BranchID, in.BlockUID)
	if err != nil {
		return "", fmt.Errorf("update_block_attrs: %w", err)
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{edit.UpdateAttrs(in.BlockUID, in.Attrs)}); err != nil {
		return "", err
	}

	// the rows describe the block as it now stands: its attrs with the
	// update laid over them.
	attrs := maps.Clone(b.Attrs)
	if attrs == nil {
		attrs = document.Attributes{}
	}

	maps.Copy(attrs, in.Attrs)
	b.Attrs = attrs

	return result(blockWriteResult{Blocks: blockRows(b)})
}

// deleteBlockArgs is what delete_block is called with.
type deleteBlockArgs struct {
	docTarget

	// BlockUID is the block being removed. Required.
	BlockUID string `json:"block_uid"`
}

// Validate checks the arguments are complete.
func (a deleteBlockArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	return nil
}

// deleteBlock removes a block from a document.
type deleteBlock struct{}

// Info returns the tool's model-facing description.
func (deleteBlock) Info() Info {
	return Info{
		Name:        NameDeleteBlock,
		Description: "Delete one block by uid, including anything nested inside it. Its comments, hooks and files go with it and cannot be restored, so use move_block when the aim is to reorder and update_block_text when the aim is new wording. Returns {deleted: uid}.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The uid of the block to delete."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
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

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameDeleteBlock, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary describes the deletion the model wants to make.
func (deleteBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameDeleteBlock, err)
	}

	return ActionSummary{
		Tool:         NameDeleteBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Delete a block in " + docLabel(doc),
	}, nil
}

// Execute removes the block.
func (deleteBlock) Execute(inp Input) (string, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{edit.Delete(in.BlockUID)}); err != nil {
		return "", err
	}

	return result(struct {
		Deleted string `json:"deleted"`
	}{Deleted: in.BlockUID})
}

// moveBlockArgs is what move_block is called with.
type moveBlockArgs struct {
	docTarget

	// BlockUID is the block being moved. Required.
	BlockUID string `json:"block_uid"`

	// Position is the side of the reference block the move lands on.
	Position position `json:"position"`

	// ReferenceBlockUID is the block the move is positioned against.
	// Required.
	ReferenceBlockUID string `json:"reference_block_uid"`
}

// Validate checks the arguments are complete.
func (a moveBlockArgs) Validate() error {
	if err := a.validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired(_keyBlockUID)
	}

	if a.Position == "" {
		return errRequired("position")
	}

	if a.ReferenceBlockUID == "" {
		return errRequired("reference_block_uid")
	}

	if a.ReferenceBlockUID == a.BlockUID {
		return errors.New("reference_block_uid must differ from block_uid")
	}

	if !a.Position.relative() {
		return fmt.Errorf("position must be %q or %q for a move, got %q", positionBefore, positionAfter, a.Position)
	}

	return nil
}

// moveBlock repositions an existing block within a document.
type moveBlock struct{}

// Info returns the tool's model-facing description.
func (moveBlock) Info() Info {
	return Info{
		Name:        NameMoveBlock,
		Description: "Move an existing block before or after another block in the same document. The block keeps its uid, attrs and nested content, so comments, hooks and files attached to it stay attached; deleting it and inserting a copy would lose them. The landing spot has to accept the block's type, by the same rule insert_block applies. Returns the summary rows of the moved block and everything nested in it.",
		Properties: map[string]any{
			_keyDocumentID: stringProp(_descTargetDocumentID),
			_keyBranchID:   stringProp(_descBranchID),
			_keyBlockUID:   stringProp("The uid of the block to move."),
			"position": map[string]any{
				_keyType: _typeString,
				_keyEnum: []string{
					string(positionBefore),
					string(positionAfter),
				},
				_keyDescription: "Landing side relative to the reference block.",
			},
			"reference_block_uid": stringProp("The uid of the block to move relative to."),
		},
		Required: []string{
			_keyDocumentID,
			_keyBranchID,
			_keyBlockUID,
			"position",
			"reference_block_uid",
		},
	}
}

// Traits reports a write.
func (moveBlock) Traits() Traits {
	return Traits{Write: true}
}

// Title announces which document is being updated.
func (moveBlock) Title(inp DescribeInput) (string, error) {
	var in moveBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return "", fmt.Errorf("%s: fetch document: %w", NameMoveBlock, err)
	}

	return "Updating " + docLabel(doc), nil
}

// Summary describes the move the model wants to make.
func (moveBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in moveBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.Branch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("%s: fetch document: %w", NameMoveBlock, err)
	}

	return ActionSummary{
		Tool:         NameMoveBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Move a block %s another block in %s", in.Position, docLabel(doc)),
	}, nil
}

// Execute applies the move.
func (moveBlock) Execute(inp Input) (string, error) {
	var in moveBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	op := edit.MoveAfter(in.BlockUID, in.ReferenceBlockUID)
	if in.Position == positionBefore {
		op = edit.MoveBefore(in.BlockUID, in.ReferenceBlockUID)
	}

	if err := inp.ValidateMove(in.DocumentID, in.BranchID, in.BlockUID, in.ReferenceBlockUID); err != nil {
		return "", fmt.Errorf("move_block: %w", err)
	}

	b, err := inp.DocumentBlock(in.DocumentID, in.BranchID, in.BlockUID)
	if err != nil {
		return "", fmt.Errorf("move_block: %w", err)
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{op}); err != nil {
		return "", err
	}

	return result(blockWriteResult{Blocks: blockRows(b)})
}
