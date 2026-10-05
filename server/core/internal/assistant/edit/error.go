package edit

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _nodeName matches a camel-cased word, the shape of the ProseMirror node
// types the realtime service names in its messages.
var _nodeName = regexp.MustCompile(`\b[a-z]+[A-Z][A-Za-z]*\b`)

// OpError describes one operation's failure on the Node side.
type OpError struct {
	// Index is the position of the failing op in the request's
	// operations array.
	Index int `json:"index"`

	// Message is the short reason from Node.
	Message string `json:"message"`
}

// describe rewrites the Node-side message into what the model can act
// on: a uid it holds no block for points at get_document, a reference
// inside the moved block says what to pick instead, and an operation
// kind is named as the tool the model knows it by, and a node type by
// the canonical name the model knows. A message with no rewrite passes
// through as it is.
func (e OpError) describe() string {
	for _, prefix := range []string{"reference_uid not found: ", "block_uid not found: "} {
		if uid, ok := strings.CutPrefix(e.Message, prefix); ok {
			return fmt.Sprintf("no block with uid %s in this document; call get_document for the current uids", uid)
		}
	}

	if uid, ok := strings.CutPrefix(e.Message, "reference_uid is inside the moved block: "); ok {
		return fmt.Sprintf("reference block %s sits inside the block being moved; choose a reference outside it", uid)
	}

	msg := strings.ReplaceAll(e.Message, "update_text", "update_block_text")

	return _nodeName.ReplaceAllStringFunc(msg, func(w string) string {
		return block.DescribeNode(document.BlockNodeType(w))
	})
}

// JoinOpErrors renders per-operation failures as one message. The
// index is left out: a tool sends a single operation, so naming its
// position says nothing the reader can act on.
func JoinOpErrors(errs []OpError) string {
	msgs := make([]string, 0, len(errs))

	for _, e := range errs {
		msgs = append(msgs, e.describe())
	}

	return strings.Join(msgs, "; ")
}
