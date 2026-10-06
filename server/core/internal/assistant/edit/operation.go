// Package edit is the Go client for the Node-side edit
// operations endpoint that mutates live Y.Docs. AI-generated edits
// flow:
//
//	markup  --markup.Build-->  document.Block (PM JSON)
//	                                    |
//	                                    v
//	edit.Operation  --Apply-->  POST /api/internal/.../operations
//	                              |
//	                              v
//	Node hocuspocus opens a direct connection, mutates the Y.Doc in
//	one transaction, broadcasts to subscribers, and disconnects.
//
// Use the package-level constructors (InsertAfter, Append,
// Replace, …) to build operations; Client.Apply ships them to
// Node, which applies the whole batch or none of it.
package edit

import (
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Operation is one edit to apply to a live document, in the JSON shape
// the Node endpoint expects. Only the fields meaningful to its kind are
// set; omitempty keeps the wire payload tight. Operations are built with
// the package-level constructors and applied as a batch via
// Client.Apply. A block-carrying operation takes the expanded editor
// tree, uids already resolved, so the caller can report the uids the
// write lands with.
type Operation struct {
	// Kind names the operation ("insert", "append", …); set on
	// every op.
	Kind string `json:"kind"`

	// Position is "after" or "before" relative to ReferenceUID;
	// insert and move only.
	Position string `json:"position,omitempty"`

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
}

// InsertAfter builds an operation that inserts b immediately after
// the block identified by referenceUID.
func InsertAfter(referenceUID string, b document.Block) Operation {
	return Operation{
		Kind:         "insert",
		Position:     "after",
		ReferenceUID: referenceUID,
		Block:        &b,
	}
}

// InsertBefore builds an operation that inserts b immediately
// before the block identified by referenceUID.
func InsertBefore(referenceUID string, b document.Block) Operation {
	return Operation{
		Kind:         "insert",
		Position:     "before",
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

// MoveAfter builds an operation that repositions the block identified
// by blockUID to immediately after the block identified by
// referenceUID, keeping the block's uid, attrs, and nested content.
func MoveAfter(blockUID, referenceUID string) Operation {
	return Operation{
		Kind:         "move",
		Position:     "after",
		BlockUID:     blockUID,
		ReferenceUID: referenceUID,
	}
}

// MoveBefore builds an operation that repositions the block
// identified by blockUID to immediately before the block identified
// by referenceUID.
func MoveBefore(blockUID, referenceUID string) Operation {
	return Operation{
		Kind:         "move",
		Position:     "before",
		BlockUID:     blockUID,
		ReferenceUID: referenceUID,
	}
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
