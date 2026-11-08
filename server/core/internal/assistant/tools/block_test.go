package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _stubContentUID is the uid of the paragraph stubContentDB answers
// with.
const _stubContentUID = "a"

// _stubMetricUID is the uid of the metric stubSimulationRoot holds.
const _stubMetricUID = "m"

// stubContentDB answers content reads with a paragraph and a callout
// holding a paragraph. err makes the branch fetch fail.
func stubContentDB(err error) *DBMock {
	return stubRootDB(stubContentRoot(), err)
}

// stubContentRoot is the content stubContentDB answers with: a
// paragraph, then a callout holding one.
func stubContentRoot() document.RootBlock {
	return document.RootBlock{
		Content: []document.Block{
			{
				Type:    document.BlockNodeParagraph,
				Attrs:   document.Attributes{document.AttrUID: _stubContentUID},
				Content: []document.Block{{Type: document.BlockNodeText, Text: "hello"}},
			},
			{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: document.Attributes{document.AttrUID: "c", document.AttrIcon: "lucide:info"},
				Content: []document.Block{{
					Type:    document.BlockNodeParagraph,
					Attrs:   document.Attributes{document.AttrUID: "cp"},
					Content: []document.Block{{Type: document.BlockNodeText, Text: "inside"}},
				}},
			},
		},
	}
}

// stubSimulationRoot is content holding one metric whose data has
// arrived: it still names its preset and its simulation is switched off.
func stubSimulationRoot() document.RootBlock {
	return document.RootBlock{
		Content: []document.Block{
			{
				Type:  document.BlockNodeMetricGrid,
				Attrs: document.Attributes{document.AttrUID: "g"},
				Content: []document.Block{
					{
						Type: document.BlockNodeMetricBlock,
						Attrs: document.Attributes{
							document.AttrUID:              _stubMetricUID,
							document.AttrSimulationPreset: "cpu_usage",
							document.AttrSimulationActive: false,
						},
					},
				},
			},
		},
	}
}

// stubRootDB answers content reads with the given tree. err makes the
// branch fetch fail.
func stubRootDB(root document.RootBlock, err error) *DBMock {
	db := stubDocumentDB()
	fetch := db.FetchDocumentFunc
	fetchByBranch := db.FetchDocumentByBranchIDFunc

	// the document fetches carry the branch's content, which is what a
	// branch read and get_document use; err makes the branch fetch fail.
	db.FetchDocumentFunc = func(ctx context.Context, id xid.ID, orgID, branchName string) (*document.Document, error) {
		doc, ferr := fetch(ctx, id, orgID, branchName)
		if ferr != nil {
			return nil, ferr
		}

		doc.Content = root

		return doc, nil
	}

	db.FetchDocumentByBranchIDFunc = func(ctx context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
		if err != nil {
			return nil, err
		}

		doc, ferr := fetchByBranch(ctx, branchID, orgID)
		if ferr != nil {
			return nil, ferr
		}

		doc.Content = root

		return doc, nil
	}

	return db
}

// targetArgs is the document_id and branch_id pair every content tool
// is called with, naming the test document on the given branch.
func targetArgs(branchID xid.ID) string {
	return `"document_id":"` + _testDocID.String() + `","branch_id":"` + branchID.String() + `"`
}

// blockCase is one call of a block write and what it should come to.
type blockCase struct {
	Tool Tool

	// Root is the stored content. Empty takes stubContentRoot.
	Root document.RootBlock

	// FetchErr, CheckErr and ApplyErr are what Input.FetchDocumentBlock,
	// Input.CheckDataSources and Input.ApplyEdit fail with.
	FetchErr error
	CheckErr error
	ApplyErr error

	// Content is pieces of the written blocks' markup.
	Content []string

	// Result, when set, is the whole expected result.
	Result any

	// Ops are the operations the write should send, in order, each
	// given by a piece of its JSON. Empty skips the check.
	Ops []string

	// FetchCalls, CheckCalls and ApplyCalls are how many times the
	// write reaches each collaborator.
	FetchCalls int
	CheckCalls int
	ApplyCalls int

	Err error
}

// runBlockCase runs a block write over a stub input and asserts the shared
// outcome.
func runBlockCase(t *testing.T, c blockCase) {
	t.Helper()

	root := c.Root
	if root.Content == nil {
		root = stubContentRoot()
	}

	inp := &InputMock{
		FetchDocumentBlockFunc: func(_, _ xid.ID, uid string) (document.Block, error) {
			if c.FetchErr != nil {
				return document.Block{}, c.FetchErr
			}

			b, ok := root.FindByUID(uid)
			if !ok {
				return document.Block{}, fmt.Errorf("block %s: %w", uid, _errUnknownBlock)
			}

			return b, nil
		},
		CheckDataSourcesFunc: func([]document.Block) error { return c.CheckErr },
		ApplyEditFunc:        func(xid.ID, xid.ID, []edit.Operation) error { return c.ApplyErr },
	}

	res, err := c.Tool.Exec(inp)
	testutil.AssertEqualError(t, c.Err, err)

	assert.Len(t, inp.FetchDocumentBlockCalls(), c.FetchCalls)
	assert.Len(t, inp.CheckDataSourcesCalls(), c.CheckCalls)
	assert.Len(t, inp.ApplyEditCalls(), c.ApplyCalls)

	if err != nil {
		return
	}

	if c.Result != nil {
		assert.Equal(t, c.Result, res)
	}

	if len(c.Content) > 0 {
		out, ok := res.(blockWriteResult)
		require.True(t, ok)

		for _, want := range c.Content {
			assert.Contains(t, out.Content, want)
		}
	}

	if len(c.Ops) == 0 {
		return
	}

	ff := inp.ApplyEditCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, _testDocID, ff[0].DocumentID)
	require.Len(t, ff[0].Ops, len(c.Ops))

	for i, want := range c.Ops {
		raw, err := json.Marshal(ff[0].Ops[i])
		require.NoError(t, err)

		assert.Contains(t, string(raw), want, "operation %d", i+1)
	}
}

func Test_insertBlocks_Info(t *testing.T) {
	t.Parallel()

	info := insertBlocks{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameInsertBlocks, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "position", "content"}, info.Required)
}

func Test_insertBlocks_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool insertBlocks
		Err  error
	}{
		"Document id is required": {
			Tool: insertBlocks{Position: positionEnd, Content: "<p>x</p>"},
			Err:  errors.New("document_id is required"),
		},
		"Position is required": {
			Tool: insertBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
			Err:  errors.New("position is required"),
		},
		"Reference block uid is required": {
			Tool: insertBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, Content: "<p>x</p>"},
			Err:  errors.New("reference_block_uid is required"),
		},
		// a reference only means something beside a block.
		"Reference block uid at an end of the document": {
			Tool: insertBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, ReferenceBlockUID: "a", Content: "<p>x</p>"},
			Err:  assert.AnError,
		},
		"Content is required": {
			Tool: insertBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd},
			Err:  errors.New("content is required"),
		},
		"Valid arguments": {
			Tool: insertBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, Content: "<p>x</p>"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_insertBlocks_Summary(t *testing.T) {
	t.Parallel()

	target := docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}

	cc := map[string]struct {
		Input  *InputMock
		Tool   insertBlocks
		Result string
		Err    error
	}{
		"At the start": {
			Input:  stubDocumentInput(),
			Tool:   insertBlocks{docTarget: target, Position: positionStart, Content: "<p>a</p><p>b</p>"},
			Result: "Insert 2 blocks at the start of Runbook",
		},
		"At the end": {
			Input:  stubDocumentInput(),
			Tool:   insertBlocks{docTarget: target, Position: positionEnd, Content: "<p>a</p>"},
			Result: "Insert 1 block at the end of Runbook",
		},
		"Beside a block": {
			Input:  stubDocumentInput(),
			Tool:   insertBlocks{docTarget: target, Position: positionAfter, ReferenceBlockUID: "a", Content: "<p>a</p>"},
			Result: "Insert 1 block after a block in Runbook",
		},
		"Malformed content": {
			Input: stubDocumentInput(),
			Tool:  insertBlocks{docTarget: target, Position: positionEnd, Content: "<p>a"},
			Err:   assert.AnError,
		},
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: insertBlocks{docTarget: target, Position: positionEnd, Content: "<p>a</p>"},
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameInsertBlocks,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_insertBlocks_Exec(t *testing.T) {
	t.Parallel()

	target := docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}
	two := "<p>one</p><p>two</p>"

	cc := map[string]blockCase{
		"Malformed content": {
			Tool: insertBlocks{docTarget: target, Position: positionEnd, Content: "<p>x"},
			Err:  assert.AnError,
		},
		// an insert writes new blocks, so an id names nothing.
		"Element with an id": {
			Tool: insertBlocks{docTarget: target, Position: positionEnd, Content: `<p id="a">x</p>`},
			Err:  assert.AnError,
		},
		"Content holding no block": {
			Tool: insertBlocks{docTarget: target, Position: positionEnd, Content: "  \n "},
			Err:  assert.AnError,
		},
		"Error returned by Input.CheckDataSources": {
			Tool:       insertBlocks{docTarget: target, Position: positionEnd, Content: two},
			CheckErr:   assert.AnError,
			CheckCalls: 1,
			Err:        assert.AnError,
		},
		"Error returned by Input.ApplyEdit": {
			Tool:       insertBlocks{docTarget: target, Position: positionEnd, Content: two},
			ApplyErr:   assert.AnError,
			CheckCalls: 1,
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		// the rest go after the first, since each append would land
		// before an empty paragraph the write itself put last.
		"Appended in order": {
			Tool:       insertBlocks{docTarget: target, Position: positionEnd, Content: two},
			Content:    []string{">one</p>\n<p id=", ">two</p>"},
			Ops:        []string{`"kind":"append"`, `"kind":"insert","position":"after"`},
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		"Prepended with the rest chained after the first": {
			Tool:       insertBlocks{docTarget: target, Position: positionStart, Content: two},
			Ops:        []string{`"kind":"prepend"`, `"kind":"insert","position":"after"`},
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		"Inserted before a block": {
			Tool:       insertBlocks{docTarget: target, Position: positionBefore, ReferenceBlockUID: "a", Content: two},
			Ops:        []string{`"position":"before","reference_uid":"a"`, `"position":"after"`},
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		"Inserted after a block": {
			Tool:       insertBlocks{docTarget: target, Position: positionAfter, ReferenceBlockUID: "a", Content: "<p>x</p>"},
			Ops:        []string{`"position":"after","reference_uid":"a"`},
			CheckCalls: 1,
			ApplyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runBlockCase(t, c)
		})
	}
}

func Test_insertBlocks_insertOps(t *testing.T) {
	t.Parallel()

	blocks := []document.Block{
		{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "x"}},
		{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "y"}},
	}

	cc := map[string]struct {
		Position          position
		ReferenceBlockUID string
		Result            []string
	}{
		"Before a block": {Position: positionBefore, ReferenceBlockUID: "r", Result: []string{"insert before r", "insert after x"}},
		"After a block":  {Position: positionAfter, ReferenceBlockUID: "r", Result: []string{"insert after r", "insert after x"}},
		"At the start":   {Position: positionStart, Result: []string{"prepend  ", "insert after x"}},
		"At the end":     {Position: positionEnd, Result: []string{"append  ", "insert after x"}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			ops := insertBlocks{Position: c.Position, ReferenceBlockUID: c.ReferenceBlockUID}.insertOps(blocks)
			kinds := make([]string, 0, len(ops))

			for _, op := range ops {
				kinds = append(kinds, op.Kind+" "+op.Position+" "+op.ReferenceUID)
			}

			assert.Equal(t, c.Result, kinds)
		})
	}
}

func Test_replaceBlocks_Info(t *testing.T) {
	t.Parallel()

	info := replaceBlocks{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true}, info.Traits)

	assert.Equal(t, NameReplaceBlocks, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "content"}, info.Required)
}

func Test_replaceBlocks_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool replaceBlocks
		Err  error
	}{
		"Document id is required": {
			Tool: replaceBlocks{BlockUID: "a", Content: "<p>x</p>"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Tool: replaceBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
			Err:  errors.New("block_uid is required"),
		},
		"Content is required": {
			Tool: replaceBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a"},
			Err:  errors.New("content is required"),
		},
		"Valid arguments": {
			Tool: replaceBlocks{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a", Content: "<p>x</p>"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_replaceBlocks_Summary(t *testing.T) {
	t.Parallel()

	target := docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}

	cc := map[string]struct {
		Input  *InputMock
		Tool   replaceBlocks
		Result string
		Err    error
	}{
		"Successful summary": {
			Input:  stubDocumentInput(),
			Tool:   replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="a">x</p><p>y</p>`},
			Result: "Replace a block in Runbook with 2 blocks",
		},
		"Content that does not build": {
			Input: stubDocumentInput(),
			Tool:  replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="nope">x</p>`},
			Err:   assert.AnError,
		},
		"Block the document does not hold": {
			Input: stubDocumentInput(),
			Tool:  replaceBlocks{docTarget: target, BlockUID: "zzz", Content: "<p>x</p>"},
			Err:   fmt.Errorf("block %s: %w", "zzz", _errUnknownBlock),
		},
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Tool: replaceBlocks{docTarget: target, BlockUID: "a", Content: "<p>x</p>"},
			Err:  assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := c.Tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, ActionSummary{
				Tool:         NameReplaceBlocks,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      c.Result,
			}, got)
		})
	}
}

func Test_replaceBlocks_Exec(t *testing.T) {
	t.Parallel()

	target := docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}

	cc := map[string]blockCase{
		"Error returned by Input.FetchDocumentBlock": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: "<p>x</p>"},
			FetchErr:   assert.AnError,
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Malformed content": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: "<p>x"},
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Content holding no block": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: "  "},
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		// the callout's paragraph stays where it is, so keeping its id
		// here would put it in the document twice.
		"Id of a block outside the one replaced": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="cp">x</p>`},
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Error returned by Input.CheckDataSources": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="a">x</p>`},
			CheckErr:   assert.AnError,
			FetchCalls: 1,
			CheckCalls: 1,
			Err:        assert.AnError,
		},
		"Error returned by Input.ApplyEdit": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="a">x</p>`},
			ApplyErr:   assert.AnError,
			FetchCalls: 1,
			CheckCalls: 1,
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Text rewritten in place": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="a">new <b>words</b></p>`},
			Content:    []string{`<p id="a">new <b>words</b></p>`},
			Ops:        []string{`"kind":"replace","block_uid":"a","block":{"type":"paragraph"`},
			FetchCalls: 1,
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		"One block becomes several": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "a", Content: `<p id="a">one</p><h2>two</h2>`},
			Ops:        []string{`"kind":"replace","block_uid":"a"`, `"kind":"insert","position":"after","reference_uid":"a"`},
			FetchCalls: 1,
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		"Nested block kept by its id": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: "c", Content: `<callout id="c" icon="lucide:info"><p id="cp">inside</p><p>more</p></callout>`},
			Content:    []string{`<p id="cp">inside</p>`},
			Ops:        []string{`"kind":"replace","block_uid":"c"`},
			FetchCalls: 1,
			CheckCalls: 1,
			ApplyCalls: 1,
		},
		// the metric's data arrived, and re-sending its preset does not
		// switch the simulation back on.
		"Metric keeps its simulation flag": {
			Tool:       replaceBlocks{docTarget: target, BlockUID: _stubMetricUID, Content: `<metrics id="m">{"simulationPreset":"cpu_usage","width":"wide"}</metrics>`},
			Root:       stubSimulationRoot(),
			Ops:        []string{`"simulationActive":false`},
			FetchCalls: 1,
			CheckCalls: 1,
			ApplyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runBlockCase(t, c)
		})
	}
}

func Test_deleteBlock_Info(t *testing.T) {
	t.Parallel()

	info := deleteBlock{}.Info()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, info.Traits)

	assert.Equal(t, NameDeleteBlock, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid"}, info.Required)
}

func Test_deleteBlock_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool deleteBlock
		Err  error
	}{
		"Document id is required": {
			Tool: deleteBlock{BlockUID: "b"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Tool: deleteBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID},
			Err:  errors.New("block_uid is required"),
		},
		"Valid arguments": {
			Tool: deleteBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_deleteBlock_Summary(t *testing.T) {
	t.Parallel()

	tool := deleteBlock{DocumentID: _testDocID, BranchID: _stubBranchID, BlockUID: "a"}

	cc := map[string]struct {
		Input  *InputMock
		Result ActionSummary
		Err    error
	}{
		"Successful summary": {
			Input: stubDocumentInput(),
			Result: ActionSummary{
				Tool:         NameDeleteBlock,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Delete a block in Runbook on branch draft",
			},
		},
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_deleteBlock_Exec(t *testing.T) {
	t.Parallel()

	tool := deleteBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a"}

	cc := map[string]blockCase{
		"Error returned by Input.ApplyEdit": {
			Tool:       tool,
			ApplyErr:   assert.AnError,
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Deleted": {
			Tool:       tool,
			Ops:        []string{`"kind":"delete","block_uid":"a"`},
			Result:     deleteBlockResult{Deleted: "a"},
			ApplyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runBlockCase(t, c)
		})
	}
}

func Test_moveBlock_Info(t *testing.T) {
	t.Parallel()

	info := moveBlock{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameMoveBlock, info.Name)
	assert.Contains(t, info.Description, "keeps its id")
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "position", "reference_block_uid"}, info.Required)
}

func Test_moveBlock_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tool moveBlock
		Err  error
	}{
		"Document id is required": {
			Tool: moveBlock{BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, ReferenceBlockUID: "r"},
			Err:  errors.New("block_uid is required"),
		},
		"Position is required": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", ReferenceBlockUID: "r"},
			Err:  errors.New("position is required"),
		},
		"Reference block uid is required": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter},
			Err:  errors.New("reference_block_uid is required"),
		},
		// a block cannot be moved relative to itself.
		"Reference block uid naming the block": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "b"},
			Err:  errors.New("reference_block_uid must differ from block_uid"),
		},
		// a move lands beside a block, never at an end of the document.
		"Position at an end of the document": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionEnd, ReferenceBlockUID: "r"},
			Err:  errors.New(`position must be "before" or "after" for a move, got "end"`),
		},
		"Valid arguments": {
			Tool: moveBlock{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Tool.Validate())
		})
	}
}

func Test_moveBlock_Summary(t *testing.T) {
	t.Parallel()

	tool := moveBlock{
		DocumentID: _testDocID, BranchID: _stubMainBranchID,
		BlockUID:          "a",
		Position:          positionAfter,
		ReferenceBlockUID: "c",
	}

	cc := map[string]struct {
		Input  *InputMock
		Result ActionSummary
		Err    error
	}{
		"Successful summary": {
			Input: stubDocumentInput(),
			Result: ActionSummary{
				Tool:         NameMoveBlock,
				DocumentID:   _testDocID,
				DocumentName: _stubDocumentName,
				Summary:      "Move a block after another block in Runbook",
			},
		},
		"Error returned by Input.FetchBranch": {
			Input: &InputMock{
				FetchBranchFunc: func(xid.ID, xid.ID) (*document.Document, error) { return nil, assert.AnError },
			},
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := tool.Summary(c.Input)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Input.FetchBranchCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_moveBlock_Exec(t *testing.T) {
	t.Parallel()

	tool := func(uid string, pos position) moveBlock {
		return moveBlock{
			DocumentID: _testDocID, BranchID: _stubMainBranchID,
			BlockUID:          uid,
			Position:          pos,
			ReferenceBlockUID: "c",
		}
	}

	cc := map[string]blockCase{
		"Error returned by Input.FetchDocumentBlock": {
			Tool:       tool("a", positionAfter),
			FetchErr:   assert.AnError,
			FetchCalls: 1,
			Err:        assert.AnError,
		},
		"Block uid the document does not hold": {
			Tool:       tool("zzz", positionAfter),
			FetchCalls: 1,
			Err:        fmt.Errorf("block zzz: %w", _errUnknownBlock),
		},
		"Error returned by Input.ApplyEdit": {
			Tool:       tool("a", positionAfter),
			ApplyErr:   assert.AnError,
			FetchCalls: 1,
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Moved before": {
			Tool:       tool("a", positionBefore),
			Content:    []string{`<p id="a">hello</p>`},
			Ops:        []string{`"kind":"move","position":"before","reference_uid":"c","block_uid":"a"`},
			FetchCalls: 1,
			ApplyCalls: 1,
		},
		"Moved after": {
			Tool:       tool("a", positionAfter),
			Content:    []string{`<p id="a">hello</p>`},
			Ops:        []string{`"kind":"move","position":"after","reference_uid":"c","block_uid":"a"`},
			FetchCalls: 1,
			ApplyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runBlockCase(t, c)
		})
	}
}

func Test_insertAfterPrevious(t *testing.T) {
	t.Parallel()

	first := edit.Delete("x")

	cc := map[string]struct {
		Blocks []document.Block
		Result int
	}{
		"One block":      {Blocks: []document.Block{{Type: document.BlockNodeParagraph}}, Result: 1},
		"Several blocks": {Blocks: make([]document.Block, 3), Result: 3},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Len(t, insertAfterPrevious(first, c.Blocks), c.Result)
		})
	}
}

func Test_docTarget_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Target docTarget
		Err    error
	}{
		"Document id is required": {
			Target: docTarget{},
			Err:    errors.New("document_id is required"),
		},
		"Branch id is required": {
			Target: docTarget{DocumentID: _testDocID},
			Err:    errors.New("branch_id is required"),
		},
		"Valid target": {
			Target: docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Target.Validate())
		})
	}
}

func Test_position_UnmarshalText(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Text   string
		Result position
		Err    error
	}{
		"Known position":   {Text: "before", Result: positionBefore},
		"Unknown position": {Text: "sideways", Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var p position

			err := p.UnmarshalText([]byte(c.Text))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, p)
		})
	}
}

func Test_position_relative(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Position position
		Result   bool
	}{
		"Before": {Position: positionBefore, Result: true},
		"After":  {Position: positionAfter, Result: true},
		"Start":  {Position: positionStart},
		"End":    {Position: positionEnd},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, c.Position.relative())
		})
	}
}
