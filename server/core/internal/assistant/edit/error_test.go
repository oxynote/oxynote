package edit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_OpError_describe(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Msg    string
		Result string
	}{
		"Reference uid not found": {
			Msg:    "reference_uid not found: r1",
			Result: "no block with uid r1 in this document; call get_document for the current uids",
		},
		"Block uid not found": {
			Msg:    "block_uid not found: b1",
			Result: "no block with uid b1 in this document; call get_document for the current uids",
		},
		"Reference inside the moved block": {
			Msg:    "reference_uid is inside the moved block: r1",
			Result: "reference block r1 sits inside the block being moved; choose a reference outside it",
		},
		"Operation kind named as its tool": {
			Msg:    "update_text does not apply to calloutBlock: use replace_block to rewrite it whole, or update_text on the block holding the text.",
			Result: "update_block_text does not apply to calloutBlock: use replace_block to rewrite it whole, or update_block_text on the block holding the text.",
		},
		"Unknown message passes through": {
			Msg:    "cannot move a block relative to itself: a",
			Result: "cannot move a block relative to itself: a",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, OpError{Message: c.Msg}.describe())
		})
	}
}

func Test_JoinOpErrors(t *testing.T) {
	t.Parallel()

	got := JoinOpErrors([]OpError{
		{Index: 0, Message: "block_uid not found: a"},
		{Index: 1, Message: "something else"},
	})

	// each message is rewritten for the model and the index is left out,
	// since a tool ships one operation.
	assert.Equal(t, "no block with uid a in this document; call get_document for the current uids; something else", got)
}
