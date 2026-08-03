// Package liveedit is the Go client for the Node-side edit
// operations endpoint that mutates live Y.Docs. AI-generated edits
// flow:
//
//	aiblock.Block  --Expand-->  document.Block (PM JSON)
//	                                |
//	                                v
//	liveedit.Operation  --Apply-->  POST /api/internal/.../operations
//	                                |
//	                                v
//	Node hocuspocus opens a direct connection, mutates the Y.Doc in
//	one transaction, broadcasts to subscribers, and disconnects.
//
// Use the package-level constructors (InsertAfter, Append,
// UpdateText, …) to build operations; Client.Apply ships them to
// Node and returns a per-op outcome.
package liveedit

import (
	"github.com/oxynote/heimdall/internal/document"
	"github.com/oxynote/heimdall/internal/document/aiblock"
)

// Operation is one edit to apply to a live document. Operations are
// constructed via the package-level helpers (InsertAfter, Append,
// Prepend, Replace, UpdateText, UpdateAttrs, Delete, SetName,
// SetIcon) and applied as a batch via Client.Apply.
type Operation interface {
	// op marks an implementation as an Operation. The method is
	// unexported so that the type set stays closed to this
	// package — external code cannot invent new op kinds.
	op()

	// toWire returns the JSON shape expected by the Node endpoint.
	// Failures (typically canonical expansion errors) are reported
	// as errors so Apply can fail fast before the round-trip.
	toWire() (wireOp, error)
}

// InsertAfter builds an operation that inserts block immediately
// after the block identified by referenceUID. The block argument is
// expanded into ProseMirror JSON at send time.
func InsertAfter(referenceUID string, block aiblock.Block) Operation {
	return &insertOp{position: "after", referenceUID: referenceUID, block: block}
}

// InsertBefore builds an operation that inserts block immediately
// before the block identified by referenceUID.
func InsertBefore(referenceUID string, block aiblock.Block) Operation {
	return &insertOp{position: "before", referenceUID: referenceUID, block: block}
}

// Append builds an operation that adds block at the end of the
// document's top-level content.
func Append(block aiblock.Block) Operation {
	return &appendOp{block: block}
}

// Prepend builds an operation that adds block at the start of the
// document's top-level content.
func Prepend(block aiblock.Block) Operation {
	return &prependOp{block: block}
}

// Replace builds an operation that replaces the block identified by
// blockUID with block. The replacement preserves the original
// position in its parent; its uid is taken from block (or freshly
// generated when block.UID is empty).
func Replace(blockUID string, block aiblock.Block) Operation {
	return &replaceOp{blockUID: blockUID, block: block}
}

// UpdateText builds an operation that replaces the inline text of a
// text-bearing block, leaving its type and attrs unchanged. text
// uses the canonical minimal-markdown subset.
func UpdateText(blockUID, text string) Operation {
	return &updateTextOp{blockUID: blockUID, text: text}
}

// UpdateAttrs builds an operation that sets the named attributes on
// an existing block. Unmentioned attrs are preserved; the uid attr
// cannot be changed and is silently ignored on the Node side.
func UpdateAttrs(blockUID string, attrs map[string]any) Operation {
	return &updateAttrsOp{blockUID: blockUID, attrs: attrs}
}

// Delete builds an operation that removes the block identified by
// blockUID from the document.
func Delete(blockUID string) Operation {
	return &deleteOp{blockUID: blockUID}
}

// SetName builds an operation that updates the document's display
// name. Empty name leaves the document title empty.
func SetName(name string) Operation {
	return &setNameOp{name: name}
}

// SetIcon builds an operation that updates the document's icon
// identifier.
func SetIcon(icon string) Operation {
	return &setIconOp{icon: icon}
}

// wireOp is the JSON shape the Node endpoint expects for one
// operation. Only the fields meaningful to a given kind are set;
// omitempty keeps the wire payload tight.
type wireOp struct {
	Kind         string           `json:"kind"`
	Position     string           `json:"position,omitempty"`
	ReferenceUID string           `json:"reference_uid,omitempty"`
	BlockUID     string           `json:"block_uid,omitempty"`
	Block        *document.Block  `json:"block,omitempty"`
	Content      []document.Block `json:"content,omitempty"`
	Attrs        map[string]any   `json:"attrs,omitempty"`
	Name         string           `json:"name,omitempty"`
	Icon         string           `json:"icon,omitempty"`
}

// insertOp inserts block before or after the reference block in
// the document content fragment.
type insertOp struct {
	// position is "before" or "after" relative to referenceUID.
	position string

	// referenceUID identifies the existing block to anchor on.
	referenceUID string

	// block is the new block to insert. Expanded to ProseMirror
	// at wire time so a re-extracted aiblock doesn't ship a stale
	// payload.
	block aiblock.Block
}

func (*insertOp) op() {}

func (o *insertOp) toWire() (wireOp, error) {
	expanded, err := aiblock.Expand(o.block)
	if err != nil {
		return wireOp{}, err
	}

	return wireOp{
		Kind:         "insert",
		Position:     o.position,
		ReferenceUID: o.referenceUID,
		Block:        &expanded,
	}, nil
}

// appendOp adds block as the last top-level child of the document.
type appendOp struct {
	// block is the new block to append.
	block aiblock.Block
}

func (*appendOp) op() {}

func (o *appendOp) toWire() (wireOp, error) {
	expanded, err := aiblock.Expand(o.block)
	if err != nil {
		return wireOp{}, err
	}

	return wireOp{Kind: "append", Block: &expanded}, nil
}

// prependOp adds block as the first top-level child of the document.
type prependOp struct {
	// block is the new block to prepend.
	block aiblock.Block
}

func (*prependOp) op() {}

func (o *prependOp) toWire() (wireOp, error) {
	expanded, err := aiblock.Expand(o.block)
	if err != nil {
		return wireOp{}, err
	}

	return wireOp{Kind: "prepend", Block: &expanded}, nil
}

// replaceOp swaps an existing block by uid for a new block,
// preserving the position in the parent.
type replaceOp struct {
	// blockUID is the uid of the block being replaced.
	blockUID string

	// block is the replacement. Its own uid is taken from
	// block.UID; if empty, Node generates a fresh one.
	block aiblock.Block
}

func (*replaceOp) op() {}

func (o *replaceOp) toWire() (wireOp, error) {
	expanded, err := aiblock.Expand(o.block)
	if err != nil {
		return wireOp{}, err
	}

	return wireOp{Kind: "replace", BlockUID: o.blockUID, Block: &expanded}, nil
}

// updateTextOp replaces the inline content of a text-bearing block
// without touching its type, attrs, or uid. text is parsed as the
// minimal-markdown subset at wire time.
type updateTextOp struct {
	// blockUID is the uid of the block whose text is updated.
	blockUID string

	// text is the new inline content in canonical markdown.
	text string
}

func (*updateTextOp) op() {}

func (o *updateTextOp) toWire() (wireOp, error) {
	return wireOp{
		Kind:     "update_text",
		BlockUID: o.blockUID,
		Content:  aiblock.ParseInlineMarkdown(o.text),
	}, nil
}

// updateAttrsOp merges new attribute values onto an existing block.
// Unmentioned attrs are preserved; the uid attr is ignored by Node.
type updateAttrsOp struct {
	// blockUID is the uid of the block being mutated.
	blockUID string

	// attrs is the partial attribute map to merge.
	attrs map[string]any
}

func (*updateAttrsOp) op() {}

func (o *updateAttrsOp) toWire() (wireOp, error) {
	return wireOp{Kind: "update_attrs", BlockUID: o.blockUID, Attrs: o.attrs}, nil
}

// deleteOp removes an existing block by uid.
type deleteOp struct {
	// blockUID is the uid of the block to delete.
	blockUID string
}

func (*deleteOp) op() {}

func (o *deleteOp) toWire() (wireOp, error) {
	return wireOp{Kind: "delete", BlockUID: o.blockUID}, nil
}

// setNameOp overwrites the document's display name via the Y.Doc's
// name fragment.
type setNameOp struct {
	// name is the new display name.
	name string
}

func (*setNameOp) op() {}

func (o *setNameOp) toWire() (wireOp, error) {
	return wireOp{Kind: "set_name", Name: o.name}, nil
}

// setIconOp overwrites the document's icon identifier via the
// Y.Doc's icon text field.
type setIconOp struct {
	// icon is the new icon identifier (e.g. "lucide:rocket").
	icon string
}

func (*setIconOp) op() {}

func (o *setIconOp) toWire() (wireOp, error) {
	return wireOp{Kind: "set_icon", Icon: o.icon}, nil
}
