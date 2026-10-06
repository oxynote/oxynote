package tools

import (
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/rs/xid"
)

// _contentDescription describes the XML a block write takes. Only
// insert_blocks carries it; the others refer to it there.
var _contentDescription = "The blocks as XML, one element per block. " +
	"Elements: <p>; <h1> to <h3>, plain text; <ul> and <ol start> of <li>, and <tasks> of <task checked>, where an entry holds its text and then any blocks nested under it; " +
	"<blockquote> and <callout icon> holding blocks; <pre language> with raw code, or with a title the titled code a split_doc's right side takes; <mermaid> with raw source; " +
	"<hr/>; <img src alt title width/>; <figma src width height/>; <metrics> of <metric>, which holds a JSON object of its attributes; " +
	"<split_doc inversed> holding <left>, an <h1> then <p>, lists or <callout> then any <params header> of <param name type>description</param>, and <right>, a titled <pre>, <metric> or <mermaid>. " +
	"Inline text takes <b>, <i>, <u>, <s>, <code> and <a href>; outside <pre>, <mermaid> and <metric>, whose content is raw, write & as &amp; and < as &lt;. " +
	"Mermaid ends a gantt task name or timeline period at its first colon, so write a colon inside one as #58;. " +
	"A metric's attributes: " + markup.MetricReference() + "."

// docTarget names the document, and the branch of it, a content tool
// reads or writes. Both are required: the default branch is a branch
// like any other, and its id comes with every document listing.
type docTarget struct {
	// DocumentID names the document.
	DocumentID xid.ID `json:"document_id"`

	// BranchID names the branch.
	BranchID xid.ID `json:"branch_id"`
}

// Validate checks the target names a document and a branch.
func (t docTarget) Validate() error {
	if t.DocumentID.IsNil() {
		return errRequired("document_id")
	}

	if t.BranchID.IsNil() {
		return errRequired("branch_id")
	}

	return nil
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

// insertBlocksArgs is what insert_blocks is called with.
type insertBlocksArgs struct {
	docTarget

	// ReferenceBlockUID is the block the insertion is positioned
	// against. Required for before and after, and refused otherwise.
	ReferenceBlockUID string `json:"reference_block_uid"`

	// Position is where the blocks land.
	Position position `json:"position"`

	// Content is the blocks, as markup.
	Content string `json:"content"`
}

// Validate checks the arguments are complete and consistent.
func (a insertBlocksArgs) Validate() error {
	if err := a.docTarget.Validate(); err != nil {
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

	if a.Content == "" {
		return errRequired("content")
	}

	return nil
}

// insertOps returns the operations that insert blocks, at least one, at
// the position, in order: the first lands at the position and each next
// one after the one before it.
func (a insertBlocksArgs) insertOps(blocks []document.Block) []edit.Operation {
	first := edit.Append(blocks[0])

	switch a.Position {
	case positionBefore:
		first = edit.InsertBefore(a.ReferenceBlockUID, blocks[0])
	case positionAfter:
		first = edit.InsertAfter(a.ReferenceBlockUID, blocks[0])
	case positionStart:
		first = edit.Prepend(blocks[0])
	default:
	}

	return insertAfterPrevious(first, blocks)
}

// insertBlocks places new blocks in a document: beside a referenced
// block, or at either end of the document.
type insertBlocks struct{}

// Info returns the tool's model-facing description.
func (insertBlocks) Info() Info {
	return Info{
		Name:        NameInsertBlocks,
		Traits:      Traits{Write: true},
		Description: "Insert blocks into a document, written as XML, all in one call: at its start or end, or before or after the block reference_block_uid names. Every element is a new block, so leave id out; replace_blocks changes existing ones. A block not allowed where it lands is refused, naming what the place takes. Returns the written blocks as XML with their ids.",
		Properties: map[string]any{
			"document_id":         map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":           map[string]any{"type": "string", "description": _branchIDDescription},
			"reference_block_uid": map[string]any{"type": "string", "description": "The id of the block to insert beside. Required with before and after; leave it out with start and end."},
			"position": map[string]any{
				"type":        "string",
				"enum":        []string{string(positionBefore), string(positionAfter), string(positionStart), string(positionEnd)},
				"description": "Where the blocks land: before or after the reference block, or at the start or end of the document.",
			},
			"content": map[string]any{"type": "string", "description": _contentDescription},
		},
		Required: []string{"document_id", "branch_id", "position", "content"},
	}
}

// Summary describes the insertion the model wants to make.
func (insertBlocks) Summary(inp DescribeInput) (ActionSummary, error) {
	var in insertBlocksArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	blocks, err := markup.Build(in.Content, document.RootBlock{})
	if err != nil {
		return ActionSummary{}, err
	}

	var summary string

	switch what := countPhrase(len(blocks), "block"); in.Position {
	case positionStart:
		summary = fmt.Sprintf("Insert %s at the start of %s", what, doc.Title())
	case positionEnd:
		summary = fmt.Sprintf("Insert %s at the end of %s", what, doc.Title())
	default:
		summary = fmt.Sprintf("Insert %s %s a block in %s", what, in.Position, doc.Title())
	}

	return ActionSummary{
		Tool:         NameInsertBlocks,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Execute builds the blocks and inserts them in one batch.
func (insertBlocks) Execute(inp *input) (string, error) {
	var in insertBlocksArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	// an empty stored document gives no id a block to keep, which is
	// what makes every element a new block.
	blocks, err := markup.Build(in.Content, document.RootBlock{})
	if err != nil {
		return "", err
	}

	if err := inp.CheckDataSources(blocks); err != nil {
		return "", err
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, in.insertOps(blocks)); err != nil {
		return "", err
	}

	return result(blockWriteResult{Content: markup.Render(blocks)})
}

// replaceBlocksArgs is what replace_blocks is called with.
type replaceBlocksArgs struct {
	docTarget

	// BlockUID is the block being replaced. Required.
	BlockUID string `json:"block_uid"`

	// Content is what takes its place, as markup.
	Content string `json:"content"`
}

// Validate checks the arguments are complete.
func (a replaceBlocksArgs) Validate() error {
	if err := a.docTarget.Validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired("block_uid")
	}

	if a.Content == "" {
		return errRequired("content")
	}

	return nil
}

// replaceBlocks swaps one block for the blocks markup describes.
type replaceBlocks struct{}

// Info returns the tool's model-facing description.
func (replaceBlocks) Info() Info {
	return Info{
		Name:        NameReplaceBlocks,
		Traits:      Traits{Write: true, Overwrites: true},
		Description: "Replace one block with the blocks content holds, in the XML insert_blocks describes. To change a block, read it with get_document (block_uid narrows the read), edit the XML and send it back. Keep the id of every element that stays: it keeps its comments, hooks and files, and unchanged content stays exactly as stored. An element without an id is new; an id from outside the replaced block is refused. Returns the written blocks as XML.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of the block being replaced."},
			"content":     map[string]any{"type": "string", "description": "The blocks taking its place, as XML in the format insert_blocks describes."},
		},
		Required: []string{"document_id", "branch_id", "block_uid", "content"},
	}
}

// Summary describes the replacement the model wants to make.
func (replaceBlocks) Summary(inp DescribeInput) (ActionSummary, error) {
	var in replaceBlocksArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	// the markup is built as Execute builds it, against the replaced
	// block only, so the user is not asked to approve a write that is
	// then refused.
	target, ok := doc.Content.FindByUID(in.BlockUID)
	if !ok {
		return ActionSummary{}, fmt.Errorf("block %s: %w", in.BlockUID, errUnknownBlock)
	}

	blocks, err := markup.Build(in.Content, document.RootBlock{Content: []document.Block{target}})
	if err != nil {
		return ActionSummary{}, err
	}

	return ActionSummary{
		Tool:         NameReplaceBlocks,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Replace a block in %s with %s", doc.Title(), countPhrase(len(blocks), "block")),
	}, nil
}

// Execute builds the replacement and writes it where the block was.
func (replaceBlocks) Execute(inp *input) (string, error) {
	var in replaceBlocksArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	target, err := inp.FetchDocumentBlock(in.DocumentID, in.BranchID, in.BlockUID)
	if err != nil {
		return "", err
	}

	// only the replaced block's ids can be kept. Another block stays
	// where it is, so keeping its id would put it in the document twice.
	blocks, err := markup.Build(in.Content, document.RootBlock{Content: []document.Block{target}})
	if err != nil {
		return "", err
	}

	if err := inp.CheckDataSources(blocks); err != nil {
		return "", err
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, insertAfterPrevious(edit.Replace(in.BlockUID, blocks[0]), blocks)); err != nil {
		return "", err
	}

	return result(blockWriteResult{Content: markup.Render(blocks)})
}

// deleteBlockArgs is what delete_block is called with.
type deleteBlockArgs struct {
	docTarget

	// BlockUID is the block being removed. Required.
	BlockUID string `json:"block_uid"`
}

// Validate checks the arguments are complete.
func (a deleteBlockArgs) Validate() error {
	if err := a.docTarget.Validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired("block_uid")
	}

	return nil
}

// deleteBlock removes a block from a document.
type deleteBlock struct{}

// Info returns the tool's model-facing description.
func (deleteBlock) Info() Info {
	return Info{
		Name:        NameDeleteBlock,
		Traits:      Traits{Write: true, Destructive: true},
		Description: "Delete one block by id, including anything nested inside it. Its comments, hooks and files go with it and cannot be restored, so use move_block when the aim is to reorder and replace_blocks when the aim is new content. Returns {deleted: id}.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of the block to delete."},
		},
		Required: []string{"document_id", "branch_id", "block_uid"},
	}
}

// Summary describes the deletion the model wants to make.
func (deleteBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in deleteBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	return ActionSummary{
		Tool:         NameDeleteBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Delete a block in " + doc.Title(),
	}, nil
}

// Execute removes the block.
func (deleteBlock) Execute(inp *input) (string, error) {
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
	if err := a.docTarget.Validate(); err != nil {
		return err
	}

	if a.BlockUID == "" {
		return errRequired("block_uid")
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
		Traits:      Traits{Write: true},
		Description: "Move a block before or after another block of the same document. It keeps its id and nested content, and with them its comments, hooks and files, which a delete and insert would lose. The landing spot has to accept the block. Returns the moved block as XML.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _branchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of the block to move."},
			"position": map[string]any{
				"type":        "string",
				"enum":        []string{string(positionBefore), string(positionAfter)},
				"description": "Which side of the reference block it lands on.",
			},
			"reference_block_uid": map[string]any{"type": "string", "description": "The id of the block it lands beside."},
		},
		Required: []string{"document_id", "branch_id", "block_uid", "position", "reference_block_uid"},
	}
}

// Summary describes the move the model wants to make.
func (moveBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	var in moveBlockArgs

	if err := inp.Decode(&in); err != nil {
		return ActionSummary{}, err
	}

	doc, err := inp.FetchBranch(in.DocumentID, in.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetch document: %w", err)
	}

	return ActionSummary{
		Tool:         NameMoveBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      fmt.Sprintf("Move a block %s another block in %s", in.Position, doc.Title()),
	}, nil
}

// Execute applies the move.
func (moveBlock) Execute(inp *input) (string, error) {
	var in moveBlockArgs

	if err := inp.Decode(&in); err != nil {
		return "", err
	}

	op := edit.MoveAfter(in.BlockUID, in.ReferenceBlockUID)
	if in.Position == positionBefore {
		op = edit.MoveBefore(in.BlockUID, in.ReferenceBlockUID)
	}

	b, err := inp.FetchDocumentBlock(in.DocumentID, in.BranchID, in.BlockUID)
	if err != nil {
		return "", err
	}

	if err := inp.ApplyEdit(in.DocumentID, in.BranchID, []edit.Operation{op}); err != nil {
		return "", err
	}

	return result(blockWriteResult{Content: markup.Render([]document.Block{b})})
}

// blockWriteResult is what a block write returns: the blocks it wrote
// or moved, as markup.
type blockWriteResult struct {
	// Content is the blocks as markup, ids included.
	Content string `json:"content"`
}

// insertAfterPrevious returns first, the operation that writes the first
// block, then the operations that insert each later block after the one
// before it.
func insertAfterPrevious(first edit.Operation, blocks []document.Block) []edit.Operation {
	ops := make([]edit.Operation, 0, len(blocks))
	ops = append(ops, first)

	for i := 1; i < len(blocks); i++ {
		prev, _ := blocks[i-1].UID()
		ops = append(ops, edit.InsertAfter(prev, blocks[i]))
	}

	return ops
}
