package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _stubContentUID is the uid of the paragraph stubContentDB answers
// with.
const _stubContentUID = "a"

// _stubMetricUID is the uid of the metric stubSimulationDB answers with.
const _stubMetricUID = "m"

// text is a text node holding s.
func text(s string) []document.Block {
	return []document.Block{{Type: document.BlockNodeText, Text: s}}
}

// stubContentDB answers content reads with a paragraph and a callout
// holding a paragraph. err makes the branch fetch fail.
func stubContentDB(err error) *DBMock {
	return stubRootDB(document.RootBlock{
		Content: []document.Block{
			{
				Type:    document.BlockNodeParagraph,
				Attrs:   document.Attributes{document.AttrUID: _stubContentUID},
				Content: text("hello"),
			},
			{
				Type:  document.BlockNodeCalloutBlock,
				Attrs: document.Attributes{document.AttrUID: "c", document.AttrIcon: "lucide:info"},
				Content: []document.Block{{
					Type:    document.BlockNodeParagraph,
					Attrs:   document.Attributes{document.AttrUID: "cp"},
					Content: text("inside"),
				}},
			},
		},
	}, err)
}

// stubSimulationDB answers content reads with one metric whose data has
// arrived: it still names its preset and its simulation is switched off.
func stubSimulationDB() *DBMock {
	return stubRootDB(document.RootBlock{
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
	}, nil)
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

// _unknownDataSourceID names no data source in any organisation.
var _unknownDataSourceID = xid.New().String()

// metricContent is a metric grid holding one metric that names the
// given data source, as a JSON string the content argument takes.
func metricContent(dataSourceID string) string {
	return `"<metrics><metric>{\"dataSourceId\":\"` + dataSourceID +
		`\",\"visualizationType\":\"line_chart\",\"queries\":[{\"name\":\"Q\",\"query\":\"up\",\"legendFormat\":\"\"}]}</metric></metrics>"`
}

// stubDataSource makes db resolve exactly one data source.
func stubDataSource(db *DBMock) *DBMock {
	db.FetchDataSourceFunc = func(_ context.Context, id xid.ID, orgID string) (*datasource.DataSource, error) {
		if id != _testDataSourceID || orgID != "org" {
			return nil, errutil.ErrNotFound
		}

		return &datasource.DataSource{
			ID:   _testDataSourceID,
			Name: "prod",
			Type: datasource.TypePrometheus,
		}, nil
	}

	return db
}

// editCase is one call of a block write and what it should come to.
type editCase struct {
	DB       *DBMock
	Runner   *datasourceMock.Runner
	Args     string
	Contains []string

	// Ops are the operations the write should send, in order, each
	// given by a piece of its JSON. Empty skips the check.
	Ops []string

	Err error
}

// runEdit executes a block tool and asserts the shared outcome.
func runEdit(t *testing.T, tl Tool, name Name, c editCase) {
	t.Helper()

	applier := stubApplier()

	d := testDeps(c.DB, applier, nil)
	if c.Runner != nil {
		d.runners = &DataSourceRunnersMock{
			RunnerFunc: func(datasource.DataSource) datasource.Runner { return c.Runner },
		}
	}

	res, err := tl.Execute(testInput(d, name, c.Args))
	testutil.AssertEqualError(t, c.Err, err)

	if err != nil {
		// a write that was refused never reached the document: every
		// check runs before the edit is shipped, so a rejected call
		// leaves nothing half-applied.
		assert.Empty(t, applier.ApplyCalls(), "a refused write must not reach the document")

		return
	}

	require.NotEmpty(t, res)

	for _, want := range c.Contains {
		assert.Contains(t, res, want)
	}

	if len(c.Ops) == 0 {
		return
	}

	require.Len(t, applier.ApplyCalls(), 1)
	require.Len(t, applier.ApplyCalls()[0].Ops, len(c.Ops))

	for i, want := range c.Ops {
		raw, err := json.Marshal(applier.ApplyCalls()[0].Ops[i])
		require.NoError(t, err)

		assert.Contains(t, string(raw), want, "operation %d", i+1)
	}
}

func Test_docTarget_Validate(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, docTarget{}.Validate(), "document_id is required")
	assert.EqualError(t, docTarget{DocumentID: _testDocID}.Validate(), "branch_id is required")
	assert.NoError(t, docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}.Validate())
}

func Test_position_UnmarshalText(t *testing.T) {
	t.Parallel()

	var p position

	require.NoError(t, p.UnmarshalText([]byte("before")))
	assert.Equal(t, positionBefore, p)
	assert.Error(t, p.UnmarshalText([]byte("sideways")))
}

func Test_position_relative(t *testing.T) {
	t.Parallel()

	assert.True(t, positionBefore.relative())
	assert.True(t, positionAfter.relative())
	assert.False(t, positionStart.relative())
	assert.False(t, positionEnd.relative())
}

func Test_insertBlocksArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, Content: "<p>x</p>"}, map[string]Args{
		"document_id":         insertBlocksArgs{Position: positionEnd, Content: "<p>x</p>"},
		"position":            insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
		"reference_block_uid": insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, Content: "<p>x</p>"},
		"content":             insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd},
	})

	// a reference only means something beside a block.
	err := insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, ReferenceBlockUID: "a", Content: "<p>x</p>"}.Validate()
	assert.Error(t, err)
}

func Test_insertBlocks_Info(t *testing.T) {
	t.Parallel()

	info := insertBlocks{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameInsertBlocks, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "position", "content"}, info.Required)
}

func Test_insertBlocks_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	cc := map[string]struct {
		Args   string
		Result string
		Err    error
	}{
		"At the start": {
			Args:   `{` + targetArgs(_stubMainBranchID) + `,"position":"start","content":"<p>a</p><p>b</p>"}`,
			Result: "Insert 2 blocks at the start of Runbook",
		},
		"At the end": {
			Args:   `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p>a</p>"}`,
			Result: "Insert 1 block at the end of Runbook",
		},
		"Beside a block": {
			Args:   `{` + targetArgs(_stubMainBranchID) + `,"position":"after","reference_block_uid":"a","content":"<p>a</p>"}`,
			Result: "Insert 1 block after a block in Runbook",
		},
		"Malformed content": {
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p>a"}`,
			Err:  assert.AnError,
		},
		"Unreadable arguments": {Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := insertBlocks{}.Summary(testInput(d, NameInsertBlocks, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}

	_, err := insertBlocks{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameInsertBlocks, requiredArgs(t, NameInsertBlocks)))
	require.Error(t, err)
}

func Test_insertBlocks_Execute(t *testing.T) {
	t.Parallel()

	two := `"<p>one</p><p>two</p>"`

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Malformed content": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p>x"}`,
			Err:  assert.AnError,
		},
		// an insert writes new blocks, so an id names nothing.
		"Element with an id": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p id=\"a\">x</p>"}`,
			Err:  assert.AnError,
		},
		"Data source the organisation does not own": {
			DB:   stubDataSource(stubContentDB(nil)),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + metricContent(_unknownDataSourceID) + `}`,
			Err:  assert.AnError,
		},
		"Data source the organisation owns": {
			DB:       stubDataSource(stubContentDB(nil)),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + metricContent(_testDataSourceID.String()) + `}`,
			Contains: []string{`<metrics id=`},
			Ops:      []string{`"kind":"append"`},
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + two + `}`,
			Err:  assert.AnError,
		},
		"Content holding no block": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"  \n "}`,
			Err:  assert.AnError,
		},
		// the rest go after the first, since each append would land
		// before an empty paragraph the write itself put last.
		"Appended in order": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + two + `}`,
			Contains: []string{`>one</p>\n<p id=`, `>two</p>`},
			Ops:      []string{`"kind":"append"`, `"kind":"insert","position":"after"`},
		},
		"Prepended, the rest chained after the first": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"start","content":` + two + `}`,
			Ops:  []string{`"kind":"prepend"`, `"kind":"insert","position":"after"`},
		},
		"Inserted before a block": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"before","reference_block_uid":"a","content":` + two + `}`,
			Ops:  []string{`"position":"before","reference_uid":"a"`, `"position":"after"`},
		},
		"Inserted after a block on another branch": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubBranchID) + `,"position":"after","reference_block_uid":"a","content":"<p>x</p>"}`,
			Ops:  []string{`"position":"after","reference_uid":"a"`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, insertBlocks{}, NameInsertBlocks, c)
		})
	}
}

func Test_replaceBlocksArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a", Content: "<p>x</p>"}, map[string]Args{
		"document_id": replaceBlocksArgs{BlockUID: "a", Content: "<p>x</p>"},
		"block_uid":   replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
		"content":     replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a"},
	})
}

func Test_replaceBlocks_Info(t *testing.T) {
	t.Parallel()

	info := replaceBlocks{}.Info()

	assert.Equal(t, Traits{Write: true, Overwrites: true}, info.Traits)

	assert.Equal(t, NameReplaceBlocks, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "content"}, info.Required)
}

func Test_replaceBlocks_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubContentDB(nil), nil, nil)

	got, err := replaceBlocks{}.Summary(testInput(d, NameReplaceBlocks,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","content":"<p id=\"a\">x</p><p>y</p>"}`))
	require.NoError(t, err)
	assert.Equal(t, "Replace a block in Runbook with 2 blocks", got.Summary)

	// content that does not build is refused before anything is shown.
	_, err = replaceBlocks{}.Summary(testInput(d, NameReplaceBlocks,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","content":"<p id=\"nope\">x</p>"}`))
	require.Error(t, err)

	// the markup is built against the replaced block only, as the write
	// builds it, so an id from elsewhere in the document is refused here.
	_, err = replaceBlocks{}.Summary(testInput(d, NameReplaceBlocks,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","content":"<p id=\"cp\">x</p>"}`))
	require.Error(t, err)

	_, err = replaceBlocks{}.Summary(testInput(d, NameReplaceBlocks,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"zzz","content":"<p>x</p>"}`))
	testutil.AssertEqualError(t, fmt.Errorf("block %s: %w", "zzz", errUnknownBlock), err)

	_, err = replaceBlocks{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameReplaceBlocks, requiredArgs(t, NameReplaceBlocks)))
	require.Error(t, err)

	_, err = replaceBlocks{}.Summary(testInput(d, NameReplaceBlocks, `{`))
	require.Error(t, err)
}

func Test_replaceBlocks_Execute(t *testing.T) {
	t.Parallel()

	args := func(uid, content string) string {
		return `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + uid + `","content":` + content + `}`
	}

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: args("a", `"<p>x</p>"`),
			Err:  assert.AnError,
		},
		"Block the document does not hold": {
			DB:   stubContentDB(nil),
			Args: args("zzz", `"<p>x</p>"`),
			Err:  assert.AnError,
		},
		"Malformed content": {
			DB:   stubContentDB(nil),
			Args: args("a", `"<p>x"`),
			Err:  assert.AnError,
		},
		"Content holding no block": {
			DB:   stubContentDB(nil),
			Args: args("a", `"  "`),
			Err:  assert.AnError,
		},
		// the callout's paragraph stays where it is, so keeping its id
		// here would put it in the document twice.
		"Id of a block outside the one replaced": {
			DB:   stubContentDB(nil),
			Args: args("a", `"<p id=\"cp\">x</p>"`),
			Err:  assert.AnError,
		},
		"Data source the organisation does not own": {
			DB:   stubDataSource(stubSimulationDB()),
			Args: args("g", metricContent(_unknownDataSourceID)),
			Err:  assert.AnError,
		},
		"Text rewritten in place": {
			DB:       stubContentDB(nil),
			Args:     args("a", `"<p id=\"a\">new <b>words</b></p>"`),
			Contains: []string{`<p id=\"a\">new <b>words</b></p>`},
			Ops:      []string{`"kind":"replace","block_uid":"a","block":{"type":"paragraph"`},
		},
		"One block becomes several": {
			DB:   stubContentDB(nil),
			Args: args("a", `"<p id=\"a\">one</p><h2>two</h2>"`),
			Ops:  []string{`"kind":"replace","block_uid":"a"`, `"kind":"insert","position":"after","reference_uid":"a"`},
		},
		"Nested block kept by its id": {
			DB:       stubContentDB(nil),
			Args:     args("c", `"<callout id=\"c\" icon=\"lucide:info\"><p id=\"cp\">inside</p><p>more</p></callout>"`),
			Contains: []string{`<p id=\"cp\">inside</p>`},
			Ops:      []string{`"kind":"replace","block_uid":"c"`},
		},
		// the metric's data arrived, and re-sending its preset does not
		// switch the simulation back on.
		"Metric keeps its simulation flag": {
			DB:   stubSimulationDB(),
			Args: args(_stubMetricUID, `"<metric id=\"m\">{\"simulationPreset\":\"cpu_usage\",\"width\":\"wide\"}</metric>"`),
			Ops:  []string{`"simulationActive":false`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, replaceBlocks{}, NameReplaceBlocks, c)
		})
	}
}

func Test_insertBlocksArgs_insertOps(t *testing.T) {
	t.Parallel()

	blocks := []document.Block{
		{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "x"}},
		{Type: document.BlockNodeParagraph, Attrs: document.Attributes{document.AttrUID: "y"}},
	}

	kinds := func(ops []edit.Operation) []string {
		out := make([]string, 0, len(ops))

		for _, op := range ops {
			out = append(out, op.Kind+" "+op.Position+" "+op.ReferenceUID)
		}

		return out
	}

	ops := func(pos position, ref string) []edit.Operation {
		return insertBlocksArgs{Position: pos, ReferenceBlockUID: ref}.insertOps(blocks)
	}

	assert.Equal(t, []string{"insert before r", "insert after x"}, kinds(ops(positionBefore, "r")))
	assert.Equal(t, []string{"insert after r", "insert after x"}, kinds(ops(positionAfter, "r")))
	assert.Equal(t, []string{"prepend  ", "insert after x"}, kinds(ops(positionStart, "")))
	assert.Equal(t, []string{"append  ", "insert after x"}, kinds(ops(positionEnd, "")))
}

func Test_insertAfterPrevious(t *testing.T) {
	t.Parallel()

	first := edit.Delete("x")

	assert.Len(t, insertAfterPrevious(first, []document.Block{{Type: document.BlockNodeParagraph}}), 1)
	assert.Len(t, insertAfterPrevious(first, make([]document.Block, 3)), 3)
}

func Test_deleteBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"}, map[string]Args{
		"document_id": deleteBlockArgs{BlockUID: "b"},
		"block_uid":   deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID},
	})
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

func Test_deleteBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := deleteBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Delete a block in Runbook", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = deleteBlock{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameDeleteBlock, requiredArgs(t, NameDeleteBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = deleteBlock{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock, `{`))
	require.Error(t, err)
}

func Test_deleteBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubDocumentDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:       stubDocumentDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			Contains: []string{`"deleted":"a"`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, deleteBlock{}, NameDeleteBlock, c)
		})
	}
}

func Test_moveBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	ok := moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"}

	assertValidate(t, ok, map[string]Args{
		"document_id":         moveBlockArgs{BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
		"block_uid":           moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, ReferenceBlockUID: "r"},
		"position":            moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", ReferenceBlockUID: "r"},
		"reference_block_uid": moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter},
	})

	// a block cannot be moved relative to itself.
	self := ok
	self.ReferenceBlockUID = ok.BlockUID

	err := self.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must differ")

	// a move lands beside a block, never at an end of the document.
	end := ok
	end.Position = positionEnd

	err = end.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "for a move")
}

func Test_moveBlock_Info(t *testing.T) {
	t.Parallel()

	info := moveBlock{}.Info()

	assert.Equal(t, Traits{Write: true}, info.Traits)

	assert.Equal(t, NameMoveBlock, info.Name)
	assert.Contains(t, info.Description, "keeps its id")
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "position", "reference_block_uid"}, info.Required)
}

func Test_moveBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := moveBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameMoveBlock,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","position":"after","reference_block_uid":"b"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Move a block after another block in Runbook", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = moveBlock{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameMoveBlock, requiredArgs(t, NameMoveBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = moveBlock{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameMoveBlock, `{`))
	require.Error(t, err)
}

func Test_moveBlock_Execute(t *testing.T) {
	t.Parallel()

	base := `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","reference_block_uid":"b"`

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Reference uid is required": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","position":"after"}`,
			Err:  assert.AnError,
		},
		"Position must be before or after": {
			DB:   stubContentDB(nil),
			Args: base + `,"position":"end"}`,
			Err:  assert.AnError,
		},
		"Reference must differ from the block": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","position":"after","reference_block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: base + `,"position":"after"}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","position":"after","reference_block_uid":"b"}`,
			Err:  assert.AnError,
		},
		"Moved before": {DB: stubContentDB(nil), Args: base + `,"position":"before"}`, Contains: []string{`<p id=\"a\">hello</p>`}},
		"Moved after":  {DB: stubContentDB(nil), Args: base + `,"position":"after"}`, Contains: []string{`<p id=\"a\">hello</p>`}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, moveBlock{}, NameMoveBlock, c)
		})
	}
}
