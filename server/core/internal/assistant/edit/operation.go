// Package edit is the Go client for the Node endpoint that edits live
// Y.Docs. An assistant edit flows like this:
//
//	markup  --markup.Build-->  document.Block (PM JSON)
//	                                    |
//	                                    v
//	edit.Operation  --Apply-->  POST /api/internal/.../operations
//	                              |
//	                              v
//	Node opens the Y.Doc, changes it in one transaction, broadcasts
//	the change and disconnects.
//
// Build operations with the constructors (Insert, Append, Replace).
// Client.Apply ships them, and Node applies the whole batch or none.
package edit

import (
	"github.com/oxynote/oxynote/server/core/internal/document"
)

const (
	// PositionBefore lands ahead of the reference block. In a grid it lands
	// on the reference block's left.
	PositionBefore Position = "before"

	// PositionAfter lands behind the reference block. In a grid it lands
	// on the reference block's right.
	PositionAfter Position = "after"

	// PositionAbove lands above the reference block. It applies only next
	// to a block in a grid.
	PositionAbove Position = "above"

	// PositionBelow lands below the reference block. It applies only next
	// to a block in a grid.
	PositionBelow Position = "below"
)

// LayoutStacked moves a block only in the one-column order narrow pages
// show its grid in.
const LayoutStacked Layout = "stacked"

// Operation is one edit of a live document, in the JSON the Node
// endpoint expects. Only the fields its kind uses are set. A block
// carries the expanded editor tree with its uids resolved, so the caller
// can report the uids the write lands with.
type Operation struct {
	// Kind names the operation ("insert", "append", …); set on
	// every op.
	Kind string `json:"kind"`

	// Position is where the block lands next to ReferenceUID; insert
	// and move only.
	Position Position `json:"position,omitempty"`

	// ReferenceUID identifies the block an insert or move is anchored
	// to; insert and move only.
	ReferenceUID string `json:"reference_uid,omitempty"`

	// BlockUID identifies the block the operation targets; replace,
	// update_attrs, delete, and move.
	BlockUID string `json:"block_uid,omitempty"`

	// Block is the expanded ProseMirror payload; insert, append,
	// prepend, and replace.
	Block *document.Block `json:"block,omitempty"`

	// Attrs holds the attributes to set; update_attrs only.
	Attrs map[string]any `json:"attrs,omitempty"`

	// Name is the new document display name; set_name only.
	Name string `json:"name,omitempty"`

	// Icon is the new document icon identifier; set_icon only.
	Icon string `json:"icon,omitempty"`

	// Layout is the grid layout a move changes. It is empty for a move in
	// the wide layout; move only.
	Layout Layout `json:"layout,omitempty"`
}

// Position is where an insert or move lands next to its reference block.
type Position string

// Layout is the grid layout a move changes. The zero value is the wide
// layout.
type Layout string

// Insert builds an operation that inserts b at position next to the
// block identified by referenceUID.
func Insert(position Position, referenceUID string, b document.Block) Operation {
	return Operation{
		Kind:         "insert",
		Position:     position,
		ReferenceUID: referenceUID,
		Block:        &b,
	}
}

// Append builds an operation that adds b at the end of the
// document's top-level content.
func Append(b document.Block) Operation {
	return Operation{
		Kind:  "append",
		Block: &b,
	}
}

// Prepend builds an operation that adds b at the start of the
// document's top-level content.
func Prepend(b document.Block) Operation {
	return Operation{
		Kind:  "prepend",
		Block: &b,
	}
}

// Replace builds an operation that replaces the block identified by
// blockUID with b. The replacement preserves the original position
// in its parent and lands with the uids b carries.
func Replace(blockUID string, b document.Block) Operation {
	return Operation{
		Kind:     "replace",
		BlockUID: blockUID,
		Block:    &b,
	}
}

// UpdateAttrs builds an operation that sets the named attributes on
// an existing block. Unmentioned attrs are preserved; the uid attr
// cannot be changed and is silently ignored on the Node side.
func UpdateAttrs(blockUID string, attrs map[string]any) Operation {
	return Operation{
		Kind:     "update_attrs",
		BlockUID: blockUID,
		Attrs:    attrs,
	}
}

// Delete builds an operation that removes the block identified by
// blockUID from the document.
func Delete(blockUID string) Operation {
	return Operation{
		Kind:     "delete",
		BlockUID: blockUID,
	}
}

// Move builds an operation that repositions the block identified by
// blockUID at position next to the block identified by referenceUID.
// The block keeps its uid, attrs and nested content. position takes the
// same values as Insert's.
func Move(blockUID string, position Position, referenceUID string) Operation {
	return Operation{
		Kind:         "move",
		Position:     position,
		BlockUID:     blockUID,
		ReferenceUID: referenceUID,
	}
}

// MoveStacked builds an operation that moves the block identified by
// blockUID in its grid's stacked layout, before or after the block
// identified by referenceUID. The grid's columns stay as they are.
// PositionAbove lands as PositionBefore does, and PositionBelow as
// PositionAfter.
func MoveStacked(blockUID string, position Position, referenceUID string) Operation {
	op := Move(blockUID, position, referenceUID)
	op.Layout = LayoutStacked

	return op
}

// SetName builds an operation that updates the document's display
// name. The op is sent even when name is empty: omitempty drops the
// name field from the wire form, and Node treats the missing name as
// falsy and clears the title.
func SetName(name string) Operation {
	return Operation{
		Kind: "set_name",
		Name: name,
	}
}

// SetIcon builds an operation that updates the document's icon
// identifier (e.g. "lucide:rocket").
func SetIcon(icon string) Operation {
	return Operation{
		Kind: "set_icon",
		Icon: icon,
	}
}
