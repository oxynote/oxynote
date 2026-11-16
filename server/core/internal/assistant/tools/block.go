package tools

import (
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/org"
	"github.com/rs/xid"
)

const (
	// _positionBefore places ahead of the reference block.
	_positionBefore position = "before"

	// _positionAfter places behind the reference block.
	_positionAfter position = "after"

	// _positionAbove places above a reference block in a grid.
	_positionAbove position = "above"

	// _positionBelow places below a reference block in a grid.
	_positionBelow position = "below"

	// _positionStart places at the start of the document.
	_positionStart position = "start"

	// _positionEnd places at the end of the document.
	_positionEnd position = "end"
)

const (
	// _layoutWide places a block on the columns of its grid.
	_layoutWide layout = "wide"

	// _layoutStacked places a block in the one-column order narrow pages
	// show a grid in.
	_layoutStacked layout = "stacked"
)

// _contentDescription describes the XML a block write takes. Only
// insert_blocks carries it, and the others refer to it there.
var _contentDescription = "The blocks as XML, one element per block, with all text inside an element such as <p>.\n" +
	"- <p> holds text with inline marks. <h1> to <h3> hold plain text.\n" +
	"- <ul> and <ol start> hold <li>, and <tasks> holds <task checked>. An entry holds its text, then any blocks nested under it.\n" +
	"- <blockquote> and <callout icon> hold blocks. A callout's icon takes the ids create_document's icon does.\n" +
	"- <pre language> holds raw code. It takes a title only in a split_doc's <right>.\n" +
	"- <mermaid> holds raw source. Mermaid ends a gantt task name or timeline period at its first colon, so write a colon inside one as #58;, never as &#58;.\n" +
	"- <hr/>, <img src alt title width/> and <figma src width height/> are empty.\n" +
	"- <grid> sits at the top level and holds <metrics> (a chart) and <logs> in the order the grid shows them, each a JSON object of attributes. A row fits 3 compact, 2 standard or 1 wide, never a standard beside a compact. Fill every row. A tall block spans two rows, and the blocks after it fill the space beside it. get_document shows three read-only attributes on each block: row and column, counted from 0 on 24 columns, and stack, its place in the one-column order of narrow pages. A <grid> holds no text, so text about the charts, when there is some, goes in a block before it.\n" +
	"- <split_doc inversed> holds <left> then <right>. <left> has an <h1>, then <p>, lists or <callout>, then any <params header> of <param name type>description</param>. " +
	"<right> holds a titled <pre>, a <metrics> or a <mermaid>.\n" +
	"Inline text takes <b>, <i>, <u>, <s>, <code> and <a href>. " +
	"Outside <pre>, <mermaid>, <metrics> and <logs>, write & as &amp; and < as &lt;. " +
	"Inside them content is raw, but raw content with no < at all is decoded as escaped text, so write a literal &lt; there as &amp;lt;.\n" +
	"A metric block's attributes: " + markup.MetricReference() + ".\n" +
	"A log block's attributes: " + markup.LogReference() + "."

// _gridPositionDescription says what the positions mean beside a block in
// a grid. insert_blocks and move_block share it.
var _gridPositionDescription = "above and below need a reference block in a <grid>, where before and after are its left and right."

// _errGridReplaced is what replace_blocks reports for a grid. A list of
// blocks cannot say whether a block sits beside or below another, so a
// grid changes one block at a time. The message names the tools that do.
var _errGridReplaced = errors.New("a <grid> is not replaced whole; change one of its blocks with replace_blocks on that block, add one with insert_blocks, move one with move_block or remove one with delete_block")

// insertBlocks places new blocks in a document: beside a referenced
// block, or at either end of the document.
type insertBlocks struct {
	docTarget

	// ReferenceBlockUID is the block the insertion is positioned
	// against. Required for before, after, above and below, and refused
	// otherwise.
	ReferenceBlockUID string `json:"reference_block_uid"`

	// Position is where the blocks land.
	Position position `json:"position"`

	// Content is the blocks, as markup.
	Content string `json:"content"`

	// ReturnContent includes the written XML instead of only block ids.
	ReturnContent bool `json:"return_content"`
}

// Info returns the tool's model-facing description.
func (insertBlocks) Info() Info {
	return Info{
		Name:        NameInsertBlocks,
		Traits:      Traits{Write: true, Role: org.RoleEditor},
		Description: "Insert new blocks as XML at a document's start or end or beside reference_block_uid. Omit id. Use replace_blocks for existing ones. A <metrics> or <logs> belongs in a <grid>. Invalid placement is refused, naming the allowed blocks. Returns {block_uids: [id]} in document order, including nested blocks, or {content: normalised XML} when return_content is true.",
		Properties: map[string]any{
			"document_id":         map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":           map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"reference_block_uid": map[string]any{"type": "string", "description": "The id of the block to insert beside. Give it with before, after, above and below, and leave it out with start and end."},
			"position": map[string]any{
				"type":        "string",
				"enum":        []string{string(_positionBefore), string(_positionAfter), string(_positionAbove), string(_positionBelow), string(_positionStart), string(_positionEnd)},
				"description": "Where the blocks land. before, after, above and below are taken against the reference block, and start and end against the whole document. " + _gridPositionDescription,
			},
			"content":        map[string]any{"type": "string", "description": _contentDescription},
			"return_content": map[string]any{"type": "boolean", "description": "Return the written XML instead of block_uids.", "default": false},
		},
		Required: []string{"document_id", "branch_id", "position", "content"},
	}
}

// Validate checks the arguments are complete and consistent.
func (ib insertBlocks) Validate() error {
	if err := ib.docTarget.Validate(); err != nil {
		return err
	}

	if ib.Position == "" {
		return errRequired("position")
	}

	if ib.Position.relative() && ib.ReferenceBlockUID == "" {
		return errRequired("reference_block_uid")
	}

	if !ib.Position.relative() && ib.ReferenceBlockUID != "" {
		return errors.New("reference_block_uid applies to before, after, above and below only; use one of them, or leave it out")
	}

	if ib.Content == "" {
		return errRequired("content")
	}

	return nil
}

// Summary describes the insertion the model wants to make.
func (ib insertBlocks) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(ib.DocumentID, ib.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	blocks, err := markup.Build(ib.Content, document.RootBlock{})
	if err != nil {
		return ActionSummary{}, err
	}

	var summary string

	switch what := countPhrase(len(blocks), "block"); ib.Position {
	case _positionStart:
		summary = fmt.Sprintf("Insert %s at the start of %s", what, doc.Title())
	case _positionEnd:
		summary = fmt.Sprintf("Insert %s at the end of %s", what, doc.Title())
	default:
		summary = fmt.Sprintf("Insert %s %s a block in %s", what, ib.Position, doc.Title())
	}

	return ActionSummary{
		Tool:         NameInsertBlocks,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Exec builds the blocks and inserts them in one batch.
func (ib insertBlocks) Exec(inp Input) (any, error) {
	// an empty stored document gives no id a block to keep, which is
	// what makes every element a new block.
	blocks, err := markup.Build(ib.Content, document.RootBlock{})
	if err != nil {
		return nil, err
	}

	if err := inp.CheckDataSources(blocks); err != nil {
		return nil, err
	}

	if err := inp.ApplyEdit(ib.DocumentID, ib.BranchID, ib.insertOps(blocks)); err != nil {
		return nil, err
	}

	return writtenBlocks(blocks, ib.ReturnContent), nil
}

// insertOps returns the operations that insert blocks, at least one, at
// the position, in order: the first lands at the position and each next
// one after the one before it.
func (ib insertBlocks) insertOps(blocks []document.Block) []edit.Operation {
	first := edit.Append(blocks[0])

	switch {
	case ib.Position.relative():
		first = edit.Insert(edit.Position(ib.Position), ib.ReferenceBlockUID, blocks[0])
	case ib.Position == _positionStart:
		first = edit.Prepend(blocks[0])
	default:
	}

	return insertAfterPrevious(first, blocks)
}

// replaceBlocks swaps one block for the blocks markup describes.
type replaceBlocks struct {
	docTarget

	// BlockUID is the block being replaced. Required.
	BlockUID string `json:"block_uid"`

	// Content is what takes its place, as markup.
	Content string `json:"content"`

	// ReturnContent includes the written XML instead of only block ids.
	ReturnContent bool `json:"return_content"`
}

// Info returns the tool's model-facing description.
func (replaceBlocks) Info() Info {
	return Info{
		Name:        NameReplaceBlocks,
		Traits:      Traits{Write: true, Overwrites: true, Role: org.RoleEditor},
		Description: "Replace one block with content, in the XML insert_blocks describes. Read it with get_document. Keep ids to preserve comments, hooks and files. Unchanged content stays as stored. Omit id for a new block. Ids outside the replaced block and replacing a whole <grid> are refused. Returns {block_uids: [id]} in document order, including nested blocks, or {content: normalised XML} when return_content is true.",
		Properties: map[string]any{
			"document_id":    map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":      map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"block_uid":      map[string]any{"type": "string", "description": "The id of the block being replaced."},
			"content":        map[string]any{"type": "string", "description": "The blocks that take its place, in the XML insert_blocks describes."},
			"return_content": map[string]any{"type": "boolean", "default": false, "description": "Return the written XML instead of block_uids."},
		},
		Required: []string{"document_id", "branch_id", "block_uid", "content"},
	}
}

// Validate checks the arguments are complete.
func (rb replaceBlocks) Validate() error {
	if err := rb.docTarget.Validate(); err != nil {
		return err
	}

	if rb.BlockUID == "" {
		return errRequired("block_uid")
	}

	if rb.Content == "" {
		return errRequired("content")
	}

	return nil
}

// Summary describes the replacement the model wants to make.
func (rb replaceBlocks) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(rb.DocumentID, rb.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	// the markup is built as Exec builds it, against the replaced
	// block only, so the user is not asked to approve a write that is
	// then refused.
	target, ok := doc.Content.FindByUID(rb.BlockUID)
	if !ok {
		return ActionSummary{}, fmt.Errorf("block %s: %w", rb.BlockUID, _errUnknownBlock)
	}

	if target.Type.IsGrid() {
		return ActionSummary{}, _errGridReplaced
	}

	blocks, err := markup.Build(rb.Content, document.RootBlock{Content: []document.Block{target}})
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

// Exec builds the replacement and writes it where the block was.
func (rb replaceBlocks) Exec(inp Input) (any, error) {
	target, err := inp.FetchDocumentBlock(rb.DocumentID, rb.BranchID, rb.BlockUID)
	if err != nil {
		return nil, err
	}

	if target.Type.IsGrid() {
		return nil, _errGridReplaced
	}

	// only the replaced block's ids can be kept. Another block stays
	// where it is, so keeping its id would put it in the document twice.
	blocks, err := markup.Build(rb.Content, document.RootBlock{Content: []document.Block{target}})
	if err != nil {
		return nil, err
	}

	if err := inp.CheckDataSources(blocks); err != nil {
		return nil, err
	}

	if err := inp.ApplyEdit(rb.DocumentID, rb.BranchID, insertAfterPrevious(edit.Replace(rb.BlockUID, blocks[0]), blocks)); err != nil {
		return nil, err
	}

	return writtenBlocks(blocks, rb.ReturnContent), nil
}

// deleteBlock removes a block from a document.
type deleteBlock struct {
	docTarget

	// BlockUID is the block being removed. Required.
	BlockUID string `json:"block_uid"`
}

// Info returns the tool's model-facing description.
func (deleteBlock) Info() Info {
	return Info{
		Name:        NameDeleteBlock,
		Traits:      Traits{Write: true, Destructive: true, Role: org.RoleEditor},
		Description: "Delete one block by id, with everything nested inside it. Returns {deleted: id}. Its comments, hooks and files go with it and cannot be restored. So to reorder blocks, use move_block instead, and to give a block new content, use replace_blocks.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of the block to delete."},
		},
		Required: []string{"document_id", "branch_id", "block_uid"},
	}
}

// Validate checks the arguments are complete.
func (db deleteBlock) Validate() error {
	if err := db.docTarget.Validate(); err != nil {
		return err
	}

	if db.BlockUID == "" {
		return errRequired("block_uid")
	}

	return nil
}

// Summary describes the deletion the model wants to make.
func (db deleteBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(db.DocumentID, db.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	return ActionSummary{
		Tool:         NameDeleteBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      "Delete a block in " + doc.Title(),
	}, nil
}

// Exec removes the block.
func (db deleteBlock) Exec(inp Input) (any, error) {
	if err := inp.ApplyEdit(db.DocumentID, db.BranchID, []edit.Operation{edit.Delete(db.BlockUID)}); err != nil {
		return nil, err
	}

	return deleteBlockResult{Deleted: db.BlockUID}, nil
}

// deleteBlockResult is what delete_block returns.
type deleteBlockResult struct {
	// Deleted is the uid of the block that was removed.
	Deleted string `json:"deleted"`
}

// moveBlock repositions an existing block within a document.
type moveBlock struct {
	docTarget

	// BlockUID is the block being moved. Required.
	BlockUID string `json:"block_uid"`

	// Position is the side of the reference block the move lands on.
	Position position `json:"position"`

	// ReferenceBlockUID is the block the move is positioned against.
	// Required.
	ReferenceBlockUID string `json:"reference_block_uid"`

	// Layout is the grid layout the move changes. Wide when left out.
	Layout layout `json:"layout"`
}

// Info returns the tool's model-facing description.
func (moveBlock) Info() Info {
	return Info{
		Name:        NameMoveBlock,
		Traits:      Traits{Write: true, Role: org.RoleEditor},
		Description: "Move a block beside another block of the same document. Returns the moved block as XML. It keeps its id and nested content, and with them its comments, hooks and files, which a delete and insert would lose. A block not allowed where it lands is refused.",
		Properties: map[string]any{
			"document_id": map[string]any{"type": "string", "description": _documentIDDescription},
			"branch_id":   map[string]any{"type": "string", "description": _writeBranchIDDescription},
			"block_uid":   map[string]any{"type": "string", "description": "The id of the block to move."},
			"position": map[string]any{
				"type":        "string",
				"enum":        []string{string(_positionBefore), string(_positionAfter), string(_positionAbove), string(_positionBelow)},
				"description": "Which side of the reference block it lands on. " + _gridPositionDescription,
			},
			"reference_block_uid": map[string]any{"type": "string", "description": "The id of the block it lands beside."},
			"layout": map[string]any{
				"type":        "string",
				"enum":        []string{string(_layoutWide), string(_layoutStacked)},
				"default":     string(_layoutWide),
				"description": "Which layout of a <grid> the move changes. wide moves the block on the columns, and narrow pages follow until a stacked move gives them their own one-column order. stacked moves it only in that order, where above means before and below means after.",
			},
		},
		Required: []string{"document_id", "branch_id", "block_uid", "position", "reference_block_uid"},
	}
}

// Validate checks the arguments are complete.
func (mb moveBlock) Validate() error {
	if err := mb.docTarget.Validate(); err != nil {
		return err
	}

	if mb.BlockUID == "" {
		return errRequired("block_uid")
	}

	if mb.Position == "" {
		return errRequired("position")
	}

	if mb.ReferenceBlockUID == "" {
		return errRequired("reference_block_uid")
	}

	if mb.ReferenceBlockUID == mb.BlockUID {
		return errors.New("reference_block_uid must differ from block_uid")
	}

	if !mb.Position.relative() {
		return fmt.Errorf("position must be %q, %q, %q or %q for a move, got %q", _positionBefore, _positionAfter, _positionAbove, _positionBelow, mb.Position)
	}

	return nil
}

// Summary describes the move the model wants to make.
func (mb moveBlock) Summary(inp DescribeInput) (ActionSummary, error) {
	doc, err := inp.FetchBranch(mb.DocumentID, mb.BranchID)
	if err != nil {
		return ActionSummary{}, fmt.Errorf("fetching document: %w", err)
	}

	summary := fmt.Sprintf("Move a block %s another block in %s", mb.Position, doc.Title())
	if mb.Layout == _layoutStacked {
		summary += " on narrow pages"
	}

	return ActionSummary{
		Tool:         NameMoveBlock,
		DocumentID:   doc.ID,
		DocumentName: doc.DocumentName,
		Summary:      summary,
	}, nil
}

// Exec applies the move.
func (mb moveBlock) Exec(inp Input) (any, error) {
	var op edit.Operation

	switch mb.Layout {
	case _layoutStacked:
		op = edit.MoveStacked(mb.BlockUID, edit.Position(mb.Position), mb.ReferenceBlockUID)
	default:
		// an omitted layout is the wide one
		op = edit.Move(mb.BlockUID, edit.Position(mb.Position), mb.ReferenceBlockUID)
	}

	b, err := inp.FetchDocumentBlock(mb.DocumentID, mb.BranchID, mb.BlockUID)
	if err != nil {
		return nil, err
	}

	if err := inp.ApplyEdit(mb.DocumentID, mb.BranchID, []edit.Operation{op}); err != nil {
		return nil, err
	}

	return blockWriteResult{Content: markup.Render([]document.Block{b})}, nil
}

// blockWriteResult reports the written block ids or their full markup.
type blockWriteResult struct {
	// BlockUIDs are the written block ids in document order.
	BlockUIDs []string `json:"block_uids,omitempty"`

	// Content is the blocks as markup, ids included, when requested.
	Content string `json:"content,omitempty"`
}

// writtenBlocks reports the built tree without fetching the document again.
func writtenBlocks(blocks []document.Block, returnContent bool) blockWriteResult {
	if returnContent {
		return blockWriteResult{Content: markup.Render(blocks)}
	}

	var ids []string

	var visit func([]document.Block)

	visit = func(bb []document.Block) {
		for _, b := range bb {
			if uid, ok := b.UID(); ok {
				ids = append(ids, uid)
			}

			visit(b.Content)
		}
	}

	visit(blocks)

	return blockWriteResult{BlockUIDs: ids}
}

// insertAfterPrevious returns first, the operation that writes the first
// block, then the operations that insert each later block after the one
// before it.
func insertAfterPrevious(first edit.Operation, blocks []document.Block) []edit.Operation {
	ops := make([]edit.Operation, 0, len(blocks))
	ops = append(ops, first)

	for i := 1; i < len(blocks); i++ {
		prev, _ := blocks[i-1].UID()
		ops = append(ops, edit.Insert(edit.PositionAfter, prev, blocks[i]))
	}

	return ops
}

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

// position is where a block lands: a side of a reference block, or an
// end of the document.
type position string

// UnmarshalText parses the position, refusing anything outside the
// six values. The schema enum is what the model was shown; the decoder
// is where a value outside it gets reported, named by argument.
func (p *position) UnmarshalText(text []byte) error {
	switch v := position(text); v {
	case _positionBefore, _positionAfter, _positionAbove, _positionBelow, _positionStart, _positionEnd:
		*p = v

		return nil
	default:
		return fmt.Errorf("position must be one of %q, %q, %q, %q, %q or %q, got %q",
			_positionBefore, _positionAfter, _positionAbove, _positionBelow, _positionStart, _positionEnd, text)
	}
}

// relative reports whether the position is taken against a reference
// block rather than an end of the document.
func (p position) relative() bool {
	return p != _positionStart && p != _positionEnd
}

// layout is the grid layout a move changes. The zero value is wide.
type layout string

// UnmarshalText parses the layout, refusing anything but the two
// values.
func (l *layout) UnmarshalText(text []byte) error {
	switch v := layout(text); v {
	case _layoutWide, _layoutStacked:
		*l = v

		return nil
	default:
		return fmt.Errorf("layout must be %q or %q, got %q", _layoutWide, _layoutStacked, text)
	}
}
