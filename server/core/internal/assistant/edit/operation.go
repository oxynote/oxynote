// Package edit is the Go client for the Node-side edit
// operations endpoint that mutates live Y.Docs. AI-generated edits
// flow:
//
//	block.Block  --Expand-->  document.Block (PM JSON)
//	                              |
//	                              v
//	edit.Operation  --Apply-->  POST /api/internal/.../operations
//	                              |
//	                              v
//	Node hocuspocus opens a direct connection, mutates the Y.Doc in
//	one transaction, broadcasts to subscribers, and disconnects.
//
// Use the package-level constructors (InsertAfter, Append,
// UpdateText, …) to build operations; Client.Apply ships them to
// Node and returns a per-op outcome.
package edit

import (
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// Operation is one edit to apply to a live document. An Operation
// returns the JSON shape expected by the Node endpoint; failures
// (typically canonical expansion errors) are reported as errors so
// Apply can fail fast before the round-trip. Operations are
// constructed via the package-level helpers (InsertAfter,
// InsertBefore, Append, Prepend, Replace, UpdateText, UpdateAttrs,
// Delete, SetName, SetIcon) and applied as a batch via Client.Apply. Blocks are
// expanded to ProseMirror at wire time so a re-extracted block
// doesn't ship a stale payload.
type Operation func() (wireOp, error)

// InsertAfter builds an operation that inserts b immediately after
// the block identified by referenceUID. The block argument is
// expanded into ProseMirror JSON at send time.
func InsertAfter(referenceUID string, b block.Block) Operation {
	return insert("after", referenceUID, b)
}

// InsertBefore builds an operation that inserts b immediately
// before the block identified by referenceUID.
func InsertBefore(referenceUID string, b block.Block) Operation {
	return insert("before", referenceUID, b)
}

// insert builds the shared wire closure behind InsertAfter and
// InsertBefore.
func insert(position, referenceUID string, b block.Block) Operation {
	return withBlock(b, func(expanded *document.Block) wireOp {
		return wireOp{
			Kind:         "insert",
			Position:     position,
			ReferenceUID: referenceUID,
			Block:        expanded,
		}
	})
}

// Append builds an operation that adds b at the end of the
// document's top-level content.
func Append(b block.Block) Operation {
	return withBlock(b, func(expanded *document.Block) wireOp {
		return wireOp{Kind: "append", Block: expanded}
	})
}

// Prepend builds an operation that adds b at the start of the
// document's top-level content.
func Prepend(b block.Block) Operation {
	return withBlock(b, func(expanded *document.Block) wireOp {
		return wireOp{Kind: "prepend", Block: expanded}
	})
}

// Replace builds an operation that replaces the block identified by
// blockUID with b. The replacement preserves the original position
// in its parent; its uid is taken from b (or freshly generated when
// b.UID is empty).
func Replace(blockUID string, b block.Block) Operation {
	return withBlock(b, func(expanded *document.Block) wireOp {
		return wireOp{Kind: "replace", BlockUID: blockUID, Block: expanded}
	})
}

// withBlock builds an operation that expands b to ProseMirror at
// wire time and hands the result to wrap for the kind-specific wire
// form.
func withBlock(b block.Block, wrap func(expanded *document.Block) wireOp) Operation {
	return func() (wireOp, error) {
		expanded, err := block.Expand(b)
		if err != nil {
			return wireOp{}, err
		}

		return wrap(&expanded), nil
	}
}

// UpdateText builds an operation that replaces the inline text of a
// text-bearing block, leaving its type and attrs unchanged. text
// uses the canonical minimal-markdown subset and is parsed at wire
// time.
func UpdateText(blockUID, text string) Operation {
	return func() (wireOp, error) {
		return wireOp{
			Kind:     "update_text",
			BlockUID: blockUID,
			Content:  block.ParseInlineMarkdown(text),
		}, nil
	}
}

// UpdateAttrs builds an operation that sets the named attributes on
// an existing block. Unmentioned attrs are preserved; the uid attr
// cannot be changed and is silently ignored on the Node side.
func UpdateAttrs(blockUID string, attrs map[string]any) Operation {
	return func() (wireOp, error) {
		return wireOp{
			Kind:     "update_attrs",
			BlockUID: blockUID,
			Attrs:    attrs,
		}, nil
	}
}

// Delete builds an operation that removes the block identified by
// blockUID from the document.
func Delete(blockUID string) Operation {
	return func() (wireOp, error) {
		return wireOp{
			Kind:     "delete",
			BlockUID: blockUID,
		}, nil
	}
}

// SetName builds an operation that updates the document's display
// name. The op is sent even when name is empty: omitempty drops the
// name field from the wire form, and Node treats the missing name as
// falsy and clears the title.
func SetName(name string) Operation {
	return func() (wireOp, error) {
		return wireOp{
			Kind: "set_name",
			Name: name,
		}, nil
	}
}

// SetIcon builds an operation that updates the document's icon
// identifier (e.g. "lucide:rocket").
func SetIcon(icon string) Operation {
	return func() (wireOp, error) {
		return wireOp{
			Kind: "set_icon",
			Icon: icon,
		}, nil
	}
}

// wireOp is the JSON shape the Node endpoint expects for one
// operation. Only the fields meaningful to a given kind are set;
// omitempty keeps the wire payload tight.
type wireOp struct {
	// Kind names the operation ("insert", "append", …); set on
	// every op.
	Kind string `json:"kind"`

	// Position is "after" or "before" relative to ReferenceUID;
	// insert only.
	Position string `json:"position,omitempty"`

	// ReferenceUID identifies the block an insert is anchored to;
	// insert only.
	ReferenceUID string `json:"reference_uid,omitempty"`

	// BlockUID identifies the block the operation targets; replace,
	// update_text, update_attrs, and delete.
	BlockUID string `json:"block_uid,omitempty"`

	// Block is the expanded ProseMirror payload; insert, append,
	// prepend, and replace.
	Block *document.Block `json:"block,omitempty"`

	// Content is the parsed inline replacement text; update_text
	// only.
	Content []document.Block `json:"content,omitempty"`

	// Attrs holds the attributes to set; update_attrs only.
	Attrs map[string]any `json:"attrs,omitempty"`

	// Name is the new document display name; set_name only.
	Name string `json:"name,omitempty"`

	// Icon is the new document icon identifier; set_icon only.
	Icon string `json:"icon,omitempty"`
}
