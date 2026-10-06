package edit

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/oxynote/oxynote/server/core/internal/assistant/markup"
	"github.com/oxynote/oxynote/server/core/internal/document"
)

// _nodeName matches a word that may be one of the ProseMirror node types
// the realtime service names in its messages.
var _nodeName = regexp.MustCompile(`\b[a-z][A-Za-z]*\b`)

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
// inside the moved block says what to pick instead, and a node type is
// named by the element the model knows. A message with no rewrite passes
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

	return _nodeName.ReplaceAllStringFunc(e.Message, func(w string) string {
		return markup.DescribeNode(document.BlockNodeType(w))
	})
}

// Err renders the failed operation as an error, or returns nil when
// every operation landed. When the batch held more than one operation,
// the message says nothing was applied and numbers the failure from 1,
// since the reader needs to know which one failed.
func (r Result) Err(ops int) error {
	if len(r.Errors) == 0 {
		return nil
	}

	e := r.Errors[0]
	if ops == 1 {
		return errors.New(e.describe())
	}

	return fmt.Errorf("nothing was applied; operation %d: %s", e.Index+1, e.describe())
}
