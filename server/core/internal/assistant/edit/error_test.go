package edit

import (
	"errors"
	"testing"

	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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
		"Node types named by their elements": {
			Msg:    "paragraph is not allowed there in calloutBlock, which takes bulletList or orderedList at that point.",
			Result: "<p> is not allowed there in <callout>, which takes <ul> or <ol> at that point.",
		},
		"Titled pre at the root is told where it goes": {
			Msg:    "titledCodeBlock is not allowed there in the document root, which takes paragraph or heading at that point. Choose a reference block in a container that takes it.",
			Result: "a titled <pre> is not allowed there in the document root, which takes <p> or a heading at that point. Choose a reference block in a container that takes it. A titled <pre> goes only in a <split_doc>'s <right>; leave the title out for code anywhere else.",
		},
		"Params at the root are told where they go": {
			Msg:    "splitDocumentationParameterList is not allowed there in the document root, which takes paragraph at that point.",
			Result: "<params> is not allowed there in the document root, which takes <p> at that point. <params> go only in a <split_doc>'s <left>, after its heading and text.",
		},
		"Text after the params is told to go before them": {
			Msg:    "paragraph is not allowed there in splitDocumentationLeftSide, which takes splitDocumentationParameterList at that point.",
			Result: "<p> is not allowed there in <left>, which takes <params> at that point. In <left>, text, lists and callouts go before the <params>.",
		},
		"Metric outside its containers is told where it goes": {
			Msg:    "metricBlock is not allowed there in calloutBlock, which takes paragraph at that point.",
			Result: "<metric> is not allowed there in <callout>, which takes <p> at that point. A <metric> goes only in <metrics> or a <split_doc>'s <right>.",
		},
		"Node type without an element named by the element holding it": {
			Msg:    "codeBlockTitle would end without text in nrk_7bSz-mJp",
			Result: "a titled <pre>'s title would end without text in nrk_7bSz-mJp",
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

func Test_Result_Err(t *testing.T) {
	t.Parallel()

	errs := []OpError{{Index: 2, Message: "block_uid not found: a"}}

	// the message is rewritten for the model. The index and the prefix
	// only say something when the batch held more than one operation.
	cc := map[string]struct {
		Result Result
		Ops    int
		Err    error
	}{
		"No errors": {
			Result: Result{},
			Ops:    3,
		},
		"Single operation": {
			Result: Result{Errors: errs},
			Ops:    1,
			Err:    errors.New("no block with uid a in this document; call get_document for the current uids"),
		},
		"Several operations": {
			Result: Result{Errors: errs},
			Ops:    3,
			Err:    errors.New("nothing was applied; operation 3: no block with uid a in this document; call get_document for the current uids"),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Result.Err(c.Ops))
		})
	}
}
