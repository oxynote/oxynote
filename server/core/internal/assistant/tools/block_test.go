package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _paragraphArgs is a canonical paragraph a write tool can be handed.
const _paragraphArgs = `{"type":"paragraph","text":"hello"}`

// _stubContentUID is the uid of the paragraph stubContentDB answers
// with; the write cases reference it as their edit target.
const _stubContentUID = "a"

// _stubHeadingUID is the uid of the heading stubContentDB answers with,
// for the cases that need a block with attributes of its own.
const _stubHeadingUID = "h"

// _stubMetricUID is the uid of the metric stubSimulationDB answers with.
const _stubMetricUID = "m"

// stubContentDB answers content reads with a single paragraph the
// placement checks can find.
func stubContentDB(err error) *DBMock {
	return stubRootDB(document.RootBlock{
		Content: []document.Block{
			{
				Type:  document.BlockNodeParagraph,
				Text:  "hello",
				Attrs: document.Attributes{document.AttrUID: _stubContentUID},
			},
			{
				Type:  document.BlockNodeHeading,
				Text:  "Title",
				Attrs: document.Attributes{document.AttrUID: _stubHeadingUID, document.AttrLevel: 1},
			},
		},
	}, err)
}

// stubNestedDB answers content reads with blocks that hold their text
// or their parts one level down: a code block, a split_doc whose right
// side holds a titled code block, and a list whose entry holds a
// nested list.
func stubNestedDB() *DBMock {
	text := func(s string) []document.Block {
		return []document.Block{{Type: document.BlockNodeText, Text: s}}
	}

	uid := func(id string) document.Attributes {
		return document.Attributes{document.AttrUID: id}
	}

	return stubRootDB(document.RootBlock{
		Content: []document.Block{
			{Type: document.BlockNodeCodeBlock, Attrs: uid("c"), Content: text("old")},
			{
				Type:  document.BlockNodeSplitDoc,
				Attrs: uid("sd"),
				Content: []document.Block{
					{
						Type:    document.BlockNodeSplitDocLeft,
						Attrs:   uid("ls"),
						Content: []document.Block{{Type: document.BlockNodeHeading, Attrs: uid("sh"), Content: text("API")}},
					},
					{
						Type:  document.BlockNodeSplitDocRight,
						Attrs: uid("rs"),
						Content: []document.Block{{
							Type:  document.BlockNodeTitledCodeBlock,
							Attrs: uid("tc"),
							Content: []document.Block{
								{Type: document.BlockNodeCodeBlockTitle, Attrs: uid("tt"), Content: text("GET /x")},
								{Type: document.BlockNodeCodeBlock, Attrs: document.Attributes{document.AttrUID: "cb", document.AttrLanguage: "go"}, Content: text("old")},
							},
						}},
					},
				},
			},
			{
				Type:  document.BlockNodeBulletList,
				Attrs: uid("bl"),
				Content: []document.Block{{
					Type:  document.BlockNodeListItem,
					Attrs: uid("li"),
					Content: []document.Block{
						{Type: document.BlockNodeParagraph, Attrs: uid("lp"), Content: text("one")},
						{
							Type:  document.BlockNodeBulletList,
							Attrs: uid("nl"),
							Content: []document.Block{{
								Type:    document.BlockNodeListItem,
								Attrs:   uid("ni"),
								Content: []document.Block{{Type: document.BlockNodeParagraph, Attrs: uid("np"), Content: text("two")}},
							}},
						},
					},
				}},
			},
		},
	}, nil)
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

// metricBlockArgs is a metric_grid holding one metric that names the
// given data source, as the model would send it.
func metricBlockArgs(dataSourceID string) string {
	return `{"type":"metric_grid","items":[{"type":"metric","attrs":{"dataSourceId":"` + dataSourceID +
		`","visualizationType":"line_chart","queries":[{"name":"Query 1","query":"up","legendFormat":""}]}}]}`
}

// stubMetricDB answers content reads and resolves exactly one data
// source, so only the id under test decides a metric write's outcome.
func stubMetricDB() *DBMock {
	return stubDataSource(stubContentDB(nil))
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

// editCases are the argument-validation cases every block write shares.
type editCase struct {
	DB       *DBMock
	Runner   *datasourceMock.Runner
	Args     string
	Contains []string

	// Op is the single operation the write should send, as the realtime
	// service receives it. Empty skips the check.
	Op string

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

	if c.Op == "" {
		return
	}

	require.Len(t, applier.ApplyCalls(), 1)
	require.Len(t, applier.ApplyCalls()[0].Ops, 1)

	op, err := applier.ApplyCalls()[0].Ops[0]()
	require.NoError(t, err)

	raw, err := json.Marshal(op)
	require.NoError(t, err)

	assert.JSONEq(t, c.Op, string(raw))
}

func Test_readBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, readBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"}, map[string]Args{
		"document_id": readBlockArgs{BlockUID: "b"},
		"block_uid":   readBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID},
	})
}

func Test_readBlock_Info(t *testing.T) {
	t.Parallel()

	info := readBlock{}.Info()

	assert.Equal(t, NameReadBlock, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid"}, info.Required)
}

func Test_readBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, readBlock{}.Traits())
}

func Test_readBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := readBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameReadBlock, requiredArgs(t, NameReadBlock)))
	require.NoError(t, err)
	assert.Equal(t, "Reading a block in Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = readBlock{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameReadBlock, requiredArgs(t, NameReadBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = readBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameReadBlock, `{`))
	require.Error(t, err)
}

func Test_readBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Args     string
		Contains string
		Err      error
	}{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Document id is not a valid xid": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"nope","block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Block uid is required": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Block is absent": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"missing"}`,
			Err:  assert.AnError,
		},
		"Block is returned": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			Contains: `"paragraph"`,
		},
		"Block is returned from a named branch": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `,"block_uid":"a"}`,
			Contains: `"paragraph"`,
		},
		"Unknown branch is refused": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,"block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"List entry is returned as its paragraph with its nested list": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"li"}`,
			Contains: `{"type":"paragraph","uid":"lp","text":"one","children":[{"type":"bullet_list","uid":"nl"`,
		},
		"Code block title returns the titled code holding it": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"tt"}`,
			Contains: `{"type":"titled_code","uid":"tc"`,
		},
		"Split doc side returns the split doc": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"ls"}`,
			Contains: `{"type":"split_doc","uid":"sd"`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := readBlock{}.Execute(testInput(testDeps(c.DB, nil, nil), NameReadBlock, c.Args))
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Contains(t, res, c.Contains)
		})
	}
}

func Test_insertBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	para := block.Block{Type: block.BlockParagraph}

	assertValidate(t, insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, ReferenceBlockUID: "r", Position: positionAfter, Block: para}, map[string]Args{
		"document_id":         insertBlockArgs{ReferenceBlockUID: "r", Position: positionAfter, Block: para},
		"reference_block_uid": insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, Block: para},
		"position":            insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, ReferenceBlockUID: "r", Block: para},
		"block":               insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, ReferenceBlockUID: "r", Position: positionAfter},
	})

	// an end of the document needs no reference, and refuses one: a
	// reference given with start or end is a contradiction, not a hint.
	require.NoError(t, insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, Block: para}.Validate())

	err := insertBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, ReferenceBlockUID: "r", Position: positionStart, Block: para}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference_block_uid applies to before and after only")
}

func Test_insertBlock_Info(t *testing.T) {
	t.Parallel()

	info := insertBlock{}.Info()

	assert.Equal(t, NameInsertBlock, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "position", "block"}, info.Required)

	// the four positions are what the model is shown.
	pos, ok := info.Properties["position"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"before", "after", "start", "end"}, pos["enum"])
}

func Test_insertBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, insertBlock{}.Traits())
}

func Test_insertBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := insertBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameInsertBlock, requiredArgs(t, NameInsertBlock)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// a write aimed at a branch says so, since the card is what the user
	// approves.
	got, err = insertBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameInsertBlock,
		`{`+targetArgs(_stubBranchID)+`,"position":"end","block":`+_paragraphArgs+`}`))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook on branch draft", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = insertBlock{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameInsertBlock, requiredArgs(t, NameInsertBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = insertBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameInsertBlock, `{`))
	require.Error(t, err)
}

func Test_insertBlock_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)
	beside := `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"a","block":` + _paragraphArgs
	atEnd := `{` + targetArgs(_stubMainBranchID) + `,"block":` + _paragraphArgs

	cc := map[string]struct {
		Args   string
		Result string
	}{
		"Beside a block": {Args: beside + `,"position":"after"}`, Result: "Insert a paragraph after a block in Runbook"},
		"On a branch":    {Args: `{` + targetArgs(_stubBranchID) + `,"block":` + _paragraphArgs + `,"position":"end"}`, Result: "Append a paragraph to Runbook on branch draft"},
		"At the start":   {Args: atEnd + `,"position":"start"}`, Result: "Prepend a paragraph to Runbook"},
		"At the end":     {Args: atEnd + `,"position":"end"}`, Result: "Append a paragraph to Runbook"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := insertBlock{}.Summary(testInput(d, NameInsertBlock, c.Args))
			require.NoError(t, err)
			assert.Equal(t, c.Result, got.Summary)
		})
	}

	// a garbage position is refused at decode, named by argument, so
	// it never reaches the card.
	_, err := insertBlock{}.Summary(testInput(d, NameInsertBlock, beside+`,"position":"sideways"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"/position"`)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = insertBlock{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameInsertBlock, requiredArgs(t, NameInsertBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = insertBlock{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameInsertBlock, `{`))
	require.Error(t, err)
}

func Test_insertBlock_Execute(t *testing.T) {
	t.Parallel()

	beside := `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"a","block":` + _paragraphArgs
	atEnd := `{` + targetArgs(_stubMainBranchID) + `,"block":` + _paragraphArgs
	paragraphRow := []string{`"kind":"paragraph"`, `"text":"hello"`, `"depth":0`}

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Reference uid is required beside a block": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"after","block":` + _paragraphArgs + `}`,
			Err:  assert.AnError,
		},
		"Reference uid is refused at an end": {
			DB:   stubContentDB(nil),
			Args: beside + `,"position":"end"}`,
			Err:  assert.AnError,
		},
		"Position must be one of the four": {
			DB:   stubContentDB(nil),
			Args: beside + `,"position":"sideways"}`,
			Err:  assert.AnError,
		},
		"Error returned by the placement check": {
			DB: stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"a","position":"after",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"A type the root refuses": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err:  assert.AnError,
		},
		"Inserted before": {DB: stubContentDB(nil), Args: beside + `,"position":"before"}`, Contains: paragraphRow},
		"Inserted on another branch": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `,"block":` + _paragraphArgs + `,"position":"end"}`,
			Contains: paragraphRow,
		},
		"Inserted on an unknown branch": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_unknownBranchID) + `,"block":` + _paragraphArgs + `,"position":"end"}`,
			Err:  assert.AnError,
		},
		"Inserted after":        {DB: stubContentDB(nil), Args: beside + `,"position":"after"}`, Contains: paragraphRow},
		"Inserted at the start": {DB: stubContentDB(nil), Args: atEnd + `,"position":"start"}`, Contains: paragraphRow},
		"Inserted at the end":   {DB: stubContentDB(nil), Args: atEnd + `,"position":"end"}`, Contains: paragraphRow},
		// a list carries its entries' uids too, so a follow-up edit can
		// target an entry without reading the document back.
		"A list reports its entries": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"position":"end","block":{"type":"bullet_list","items":[{"type":"paragraph","text":"one"}]}}`,
			Contains: []string{`"kind":"bullet_list"`, `"kind":"list_item"`, `"depth":1`, `"parent_uid"`},
		},
		"A metric naming a data source the organisation owns": {
			DB: stubMetricDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"` + _stubContentUID +
				`","position":"after","block":` + metricBlockArgs(_testDataSourceID.String()) + `}`,
			Contains: []string{`"kind":"metric_grid"`, `"kind":"metric"`},
		},
		"A metric naming a data source it does not": {
			DB: stubMetricDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"reference_block_uid":"` + _stubContentUID +
				`","position":"after","block":` + metricBlockArgs(_unknownDataSourceID) + `}`,
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, insertBlock{}, NameInsertBlock, c)
		})
	}
}

func Test_replaceBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, replaceBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Block: block.Block{Type: block.BlockParagraph}}, map[string]Args{
		"document_id": replaceBlockArgs{BlockUID: "b", Block: block.Block{Type: block.BlockParagraph}},
		"block_uid":   replaceBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Block: block.Block{Type: block.BlockParagraph}},
		"block":       replaceBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
	})
}

func Test_replaceBlock_Info(t *testing.T) {
	t.Parallel()

	info := replaceBlock{}.Info()

	assert.Equal(t, NameReplaceBlock, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "block"}, info.Required)
}

func Test_replaceBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true, Overwrites: true}, replaceBlock{}.Traits())
}

func Test_replaceBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := replaceBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock, requiredArgs(t, NameReplaceBlock)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = replaceBlock{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameReplaceBlock, requiredArgs(t, NameReplaceBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = replaceBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock, `{`))
	require.Error(t, err)
}

func Test_replaceBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := replaceBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","block":`+_paragraphArgs+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Replace a block in Runbook with a paragraph", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = replaceBlock{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameReplaceBlock, requiredArgs(t, NameReplaceBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = replaceBlock{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock, `{`))
	require.Error(t, err)
}

func Test_replaceBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block":` + _paragraphArgs + `}`,
			Err:  assert.AnError,
		},
		"Error returned by the placement check": {
			DB: stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"Replaced": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","block":` + _paragraphArgs + `}`,
			Contains: []string{`"kind":"paragraph"`, `"text":"hello"`},
		},
		"A metric grid naming a data source the organisation owns": {
			DB: stubMetricDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubContentUID +
				`","block":` + metricBlockArgs(_testDataSourceID.String()) + `}`,
		},
		"A metric grid naming a data source it does not": {
			DB: stubMetricDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubContentUID +
				`","block":` + metricBlockArgs(_unknownDataSourceID) + `}`,
			Err: assert.AnError,
		},
		// read_block shows a metric with its preset even after its data
		// arrived. Re-sending it must not start the simulation again.
		"A metric re-sent with its preset keeps its simulation switched off": {
			DB: stubSimulationDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","block":` +
				`{"type":"metric","uid":"` + _stubMetricUID + `","attrs":{"title":"CPU","simulationPreset":"cpu_usage"}}}`,
			Op: `{"kind":"replace","block_uid":"m","block":{"type":"metricBlock","attrs":` +
				`{"uid":"m","title":"CPU","simulationPreset":"cpu_usage","simulationActive":false}}}`,
		},
		"A metric re-sent with another preset simulates": {
			DB: stubSimulationDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","block":` +
				`{"type":"metric","uid":"` + _stubMetricUID + `","attrs":{"simulationPreset":"error_rate"}}}`,
			Op: `{"kind":"replace","block_uid":"m","block":{"type":"metricBlock","attrs":` +
				`{"uid":"m","simulationPreset":"error_rate","simulationActive":true}}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, replaceBlock{}, NameReplaceBlock, c)
		})
	}
}

func Test_updateBlockTextArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, updateBlockTextArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Text: "t"}, map[string]Args{
		"document_id": updateBlockTextArgs{BlockUID: "b", Text: "t"},
		"block_uid":   updateBlockTextArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Text: "t"},
		"text":        updateBlockTextArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
	})

	// text over the cap is refused before anything parses it.
	err := updateBlockTextArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Text: strings.Repeat("x", block.MaxTextLength+1)}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "longer than")
}

func Test_updateBlockText_Info(t *testing.T) {
	t.Parallel()

	info := updateBlockText{}.Info()

	assert.Equal(t, NameUpdateBlockText, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "text"}, info.Required)
}

func Test_updateBlockText_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true, Overwrites: true}, updateBlockText{}.Traits())
}

func Test_updateBlockText_Title(t *testing.T) {
	t.Parallel()

	got, err := updateBlockText{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockText, requiredArgs(t, NameUpdateBlockText)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateBlockText{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateBlockText, requiredArgs(t, NameUpdateBlockText)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateBlockText{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockText, `{`))
	require.Error(t, err)
}

func Test_updateBlockText_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	got, err := updateBlockText{}.Summary(testInput(d, NameUpdateBlockText,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","text":"a new intro"}`))
	require.NoError(t, err)
	assert.Equal(t, `Update a block in Runbook: "a new intro"`, got.Summary)

	// an empty preview leaves a card that still reads.
	got, err = updateBlockText{}.Summary(testInput(d, NameUpdateBlockText,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","text":"  "}`))
	require.NoError(t, err)
	assert.Equal(t, "Update text of a block in Runbook", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateBlockText{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateBlockText, requiredArgs(t, NameUpdateBlockText)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateBlockText{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockText, `{`))
	require.Error(t, err)
}

func Test_updateBlockText_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"text":"hi"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Text written": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","text":"hi"}`,
			Contains: []string{`"uid":"a"`, `"text":"hi"`},
		},
		"Text written on a named branch": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubBranchID) + `,"block_uid":"a","text":"hi"}`,
			Contains: []string{`"uid":"a"`, `"text":"hi"`},
		},
		"Paragraph text is read as markdown": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","text":"say **hi**"}`,
			Contains: []string{`"text":"say hi"`},
			Op:       `{"kind":"update_text","block_uid":"a","content":[{"type":"text","text":"say "},{"type":"text","text":"hi","marks":[{"type":"bold"}]}]}`,
		},
		"Code text is sent raw": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"c","text":"a_b \\c **1**"}`,
			Contains: []string{`"uid":"c"`, `"text":"a_b \\c **1**"`},
			Op:       `{"kind":"update_text","block_uid":"c","content":[{"type":"text","text":"a_b \\c **1**"}]}`,
		},
		"Titled code text lands in its code": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"tc","text":"new_code"}`,
			Contains: []string{`"kind":"titled_code"`, `"text":"GET /x new_code"`},
			Op:       `{"kind":"update_text","block_uid":"tc","content":[{"type":"text","text":"new_code"}]}`,
		},
		"List entry keeps its nested list": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"li","text":"uno"}`,
			Contains: []string{`"uid":"li"`, `"text":"uno two"`, `"has_children":true`},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, updateBlockText{}, NameUpdateBlockText, c)
		})
	}
}

func Test_updateBlockAttrsArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, updateBlockAttrsArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Attrs: map[string]any{"level": 2}}, map[string]Args{
		"document_id": updateBlockAttrsArgs{BlockUID: "b", Attrs: map[string]any{"level": 2}},
		"block_uid":   updateBlockAttrsArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Attrs: map[string]any{"level": 2}},
		"attrs":       updateBlockAttrsArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
	})
}

func Test_updateBlockAttrs_Info(t *testing.T) {
	t.Parallel()

	info := updateBlockAttrs{}.Info()

	assert.Equal(t, NameUpdateBlockAttrs, info.Name)
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "attrs"}, info.Required)
}

func Test_updateBlockAttrs_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateBlockAttrs{}.Traits())
}

func Test_updateBlockAttrs_Title(t *testing.T) {
	t.Parallel()

	got, err := updateBlockAttrs{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockAttrs, requiredArgs(t, NameUpdateBlockAttrs)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateBlockAttrs{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateBlockAttrs, requiredArgs(t, NameUpdateBlockAttrs)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateBlockAttrs{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockAttrs, `{`))
	require.Error(t, err)
}

func Test_updateBlockAttrs_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	// keys are sorted, because the card must read the same every time
	// the same write is proposed.
	got, err := updateBlockAttrs{}.Summary(testInput(d, NameUpdateBlockAttrs,
		`{`+targetArgs(_stubMainBranchID)+`,"block_uid":"a","attrs":{"level":2,"icon":"lucide:warning"}}`))
	require.NoError(t, err)
	assert.Equal(t, "Update block icon, level in Runbook", got.Summary)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = updateBlockAttrs{}.Summary(testInput(testDeps(failingDocumentDB(), nil, nil), NameUpdateBlockAttrs, requiredArgs(t, NameUpdateBlockAttrs)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = updateBlockAttrs{}.Summary(testInput(testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockAttrs, `{`))
	require.Error(t, err)
}

func Test_updateBlockAttrs_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Attrs must not be empty": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"h","attrs":{}}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   stubContentDB(assert.AnError),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"h","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Attrs applied": {
			DB:       stubContentDB(nil),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"h","attrs":{"level":2}}`,
			Contains: []string{`"uid":"h"`, `"kind":"heading"`, `"level":2`},
		},
		// the payload names attributes, not a block type, so a metric's
		// data source arrives on its own rather than inside a block.
		"A data source the organisation owns": {
			DB: stubDataSource(stubSimulationDB()),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","attrs":{"dataSourceId":"` +
				_testDataSourceID.String() + `"}}`,
		},
		"A data source it does not": {
			DB: stubDataSource(stubSimulationDB()),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","attrs":{"dataSourceId":"` +
				_unknownDataSourceID + `"}}`,
			Err: assert.AnError,
		},
		// an empty data source is the editor's "unset", not a reference
		// to check, and no other attribute names one at all.
		"An empty data source is not looked up": {
			DB:   stubDataSource(stubSimulationDB()),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","attrs":{"dataSourceId":""}}`,
		},
		"A simulation flag the caller sent is dropped": {
			DB: stubSimulationDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID +
				`","attrs":{"title":"CPU","simulationActive":true}}`,
			Op: `{"kind":"update_attrs","block_uid":"m","attrs":{"title":"CPU"}}`,
		},
		"Another preset switches the simulation on": {
			DB: stubSimulationDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID +
				`","attrs":{"simulationPreset":"error_rate"}}`,
			Op: `{"kind":"update_attrs","block_uid":"m","attrs":{"simulationPreset":"error_rate","simulationActive":true}}`,
		},
		"A null preset switches the simulation off": {
			DB: stubSimulationDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID +
				`","attrs":{"simulationPreset":null}}`,
			Op: `{"kind":"update_attrs","block_uid":"m","attrs":{"simulationPreset":null,"simulationActive":false}}`,
		},
		"Titled code title lands on its title row": {
			DB:       stubNestedDB(),
			Args:     `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"tc","attrs":{"title":"POST /y"}}`,
			Contains: []string{`"uid":"tc"`, `"text":"POST /y old"`},
			Op:       `{"kind":"update_attrs","block_uid":"tc","attrs":{"title":"POST /y"}}`,
		},
		"Titled code language alone is accepted": {
			DB:   stubNestedDB(),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"tc","attrs":{"language":"python"}}`,
			Op:   `{"kind":"update_attrs","block_uid":"tc","attrs":{"language":"python"}}`,
		},
		// only metric blocks have the flag, so any other block is sent
		// unchanged.
		"A block that is not a metric is sent as given": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"h","attrs":{"level":2}}`,
			Op:   `{"kind":"update_attrs","block_uid":"h","attrs":{"level":2}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, updateBlockAttrs{}, NameUpdateBlockAttrs, c)
		})
	}
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

	assert.Equal(t, NameDeleteBlock, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid"}, info.Required)
}

func Test_deleteBlock_Traits(t *testing.T) {
	t.Parallel()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, deleteBlock{}.Traits())
}

func Test_deleteBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := deleteBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock, requiredArgs(t, NameDeleteBlock)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = deleteBlock{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameDeleteBlock, requiredArgs(t, NameDeleteBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = deleteBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock, `{`))
	require.Error(t, err)
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

	assert.Equal(t, NameMoveBlock, info.Name)
	assert.Contains(t, info.Description, "keeps its uid")
	assert.Equal(t, []string{"document_id", "branch_id", "block_uid", "position", "reference_block_uid"}, info.Required)
}

func Test_moveBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, moveBlock{}.Traits())
}

func Test_moveBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := moveBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameMoveBlock, requiredArgs(t, NameMoveBlock)))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)

	// the document it names has to resolve; the failure is passed on
	// rather than described around.
	_, err = moveBlock{}.Title(testInput(testDeps(failingDocumentDB(), nil, nil), NameMoveBlock, requiredArgs(t, NameMoveBlock)))
	require.Error(t, err)

	// unreadable arguments are refused before anything is looked up.
	_, err = moveBlock{}.Title(testInput(testDeps(stubDocumentDB(), nil, nil), NameMoveBlock, `{`))
	require.Error(t, err)
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
		"Moved before": {DB: stubContentDB(nil), Args: base + `,"position":"before"}`, Contains: []string{`"uid":"a"`, `"kind":"paragraph"`}},
		"Moved after":  {DB: stubContentDB(nil), Args: base + `,"position":"after"}`, Contains: []string{`"uid":"a"`, `"kind":"paragraph"`}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, moveBlock{}, NameMoveBlock, c)
		})
	}
}

func Test_findReadable(t *testing.T) {
	t.Parallel()

	db := stubNestedDB()
	doc, err := db.FetchDocumentByBranchIDFunc(context.Background(), _stubMainBranchID, "org")
	require.NoError(t, err)

	cc := map[string]struct {
		UID      string
		Expected string
		Found    bool
	}{
		"Block named by its uid":            {UID: "c", Expected: "c", Found: true},
		"Nested block named by its uid":     {UID: "np", Expected: "np", Found: true},
		"List entry named by its uid":       {UID: "li", Expected: "li", Found: true},
		"Part of a block returns its block": {UID: "tt", Expected: "tc", Found: true},
		"Part of a macro returns the macro": {UID: "rs", Expected: "sd", Found: true},
		"Unknown uid":                       {UID: "zzz"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, ok := findReadable(doc.Content.Content, c.UID)
			assert.Equal(t, c.Found, ok)

			uid, _ := got.UID()
			assert.Equal(t, c.Expected, uid)
		})
	}
}

func Test_withText(t *testing.T) {
	t.Parallel()

	content := []document.Block{{Type: document.BlockNodeText, Text: "new"}}

	paragraph := func(uid, text string) document.Block {
		return document.Block{
			Type:    document.BlockNodeParagraph,
			Attrs:   document.Attributes{document.AttrUID: uid},
			Content: []document.Block{{Type: document.BlockNodeText, Text: text}},
		}
	}

	cc := map[string]struct {
		Block    document.Block
		Expected string
	}{
		"Text leaf takes the text itself": {
			Block:    paragraph("p", "old"),
			Expected: "new",
		},
		"Titled code takes it in its code": {
			Block: document.Block{
				Type: document.BlockNodeTitledCodeBlock,
				Content: []document.Block{
					{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: document.BlockNodeText, Text: "GET /x"}}},
					{Type: document.BlockNodeCodeBlock, Content: []document.Block{{Type: document.BlockNodeText, Text: "old"}}},
				},
			},
			Expected: "GET /x new",
		},
		"List entry takes it in its paragraph and keeps the rest": {
			Block: document.Block{
				Type: document.BlockNodeListItem,
				Content: []document.Block{
					paragraph("p", "old"),
					{Type: document.BlockNodeBulletList, Content: []document.Block{paragraph("q", "kept")}},
				},
			},
			Expected: "new kept",
		},
		"Wrapper without a paragraph is left as it is": {
			Block: document.Block{
				Type:    document.BlockNodeCalloutBlock,
				Content: []document.Block{{Type: document.BlockNodeBulletList, Content: []document.Block{paragraph("q", "kept")}}},
			},
			Expected: "kept",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			before := c.Block.Flatten()

			assert.Equal(t, c.Expected, withText(c.Block, content).Flatten())

			// the fetched block is left as it was.
			assert.Equal(t, before, c.Block.Flatten())
		})
	}
}

func Test_withAttrs(t *testing.T) {
	t.Parallel()

	titled := document.Block{
		Type:  document.BlockNodeTitledCodeBlock,
		Attrs: document.Attributes{document.AttrUID: "tc"},
		Content: []document.Block{
			{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: document.BlockNodeText, Text: "GET /x"}}},
			{Type: document.BlockNodeCodeBlock, Attrs: document.Attributes{document.AttrLanguage: "go"}},
			{Type: "unknownPart"},
		},
	}

	cc := map[string]struct {
		Block    document.Block
		Attrs    map[string]any
		Expected document.Block
	}{
		"Attrs are laid over the block's own": {
			Block: document.Block{Type: document.BlockNodeHeading, Attrs: document.Attributes{document.AttrUID: "h", document.AttrLevel: 1}},
			Attrs: map[string]any{document.AttrLevel: 2},
			Expected: document.Block{
				Type:  document.BlockNodeHeading,
				Attrs: document.Attributes{document.AttrUID: "h", document.AttrLevel: 2},
			},
		},
		"Block without attrs gets them": {
			Block:    document.Block{Type: document.BlockNodeCalloutBlock},
			Attrs:    map[string]any{document.AttrIcon: "lucide:info"},
			Expected: document.Block{Type: document.BlockNodeCalloutBlock, Attrs: document.Attributes{document.AttrIcon: "lucide:info"}},
		},
		"Titled code takes its title and language on its children": {
			Block: titled,
			Attrs: map[string]any{document.AttrTitle: "POST /y", document.AttrLanguage: "python"},
			Expected: document.Block{
				Type:  document.BlockNodeTitledCodeBlock,
				Attrs: document.Attributes{document.AttrUID: "tc"},
				Content: []document.Block{
					{Type: document.BlockNodeCodeBlockTitle, Content: []document.Block{{Type: document.BlockNodeText, Text: "POST /y"}}},
					{Type: document.BlockNodeCodeBlock, Attrs: document.Attributes{document.AttrLanguage: "python"}},
					{Type: "unknownPart"},
				},
			},
		},
		"Titled code keeps what the update does not name": {
			Block:    titled,
			Attrs:    map[string]any{},
			Expected: titled,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Expected, withAttrs(c.Block, c.Attrs))
		})
	}

	// the fetched block is left as it was.
	assert.Equal(t, "go", titled.Content[1].Attrs[document.AttrLanguage])
}

func Test_sanitizeBlock(t *testing.T) {
	t.Parallel()

	metric := func(attrs document.Attributes) block.Block {
		return block.Block{Type: block.BlockMetric, UID: _stubMetricUID, Attrs: attrs}
	}

	cc := map[string]struct {
		Block  block.Block
		Stored document.Block
		Result document.Attributes
		Err    error
	}{
		"Error returned by inp.CheckDataSources": {
			Block: metric(document.Attributes{document.AttrDataSourceID: _unknownDataSourceID}),
			Err:   assert.AnError,
		},
		"Error returned by block.Expand": {
			Block: block.Block{Type: "nope"},
			Err:   assert.AnError,
		},
		"A new metric naming a preset simulates": {
			Block: metric(document.Attributes{document.AttrSimulationPreset: "cpu_usage"}),
			Result: document.Attributes{
				document.AttrUID:              _stubMetricUID,
				document.AttrSimulationPreset: "cpu_usage",
				document.AttrSimulationActive: true,
			},
		},
		"A stored metric keeps its simulation flag": {
			Block: metric(document.Attributes{document.AttrSimulationPreset: "cpu_usage"}),
			Stored: document.Block{
				Type: document.BlockNodeMetricBlock,
				Attrs: document.Attributes{
					document.AttrUID:              _stubMetricUID,
					document.AttrSimulationPreset: "cpu_usage",
					document.AttrSimulationActive: false,
				},
			},
			Result: document.Attributes{
				document.AttrUID:              _stubMetricUID,
				document.AttrSimulationPreset: "cpu_usage",
				document.AttrSimulationActive: false,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(stubMetricDB(), nil, nil), NameReplaceBlock, `{}`)

			res, err := sanitizeBlock(inp, c.Block, c.Stored)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, document.BlockNodeMetricBlock, res.Type)
			assert.Equal(t, c.Result, res.Attrs)
		})
	}
}

func Test_sanitizeBlockAttrs(t *testing.T) {
	t.Parallel()

	stored := document.Block{
		Type: document.BlockNodeMetricBlock,
		Attrs: document.Attributes{
			document.AttrUID:              _stubMetricUID,
			document.AttrSimulationPreset: "cpu_usage",
			document.AttrSimulationActive: false,
		},
	}

	cc := map[string]struct {
		Attrs  map[string]any
		Stored document.Block
		Result map[string]any
		Err    error
	}{
		"Error returned by inp.CheckDataSources": {
			Attrs:  map[string]any{document.AttrDataSourceID: _unknownDataSourceID},
			Stored: stored,
			Err:    assert.AnError,
		},
		"A block that is not a metric is left as given": {
			Attrs: map[string]any{
				document.AttrDataSourceID:     _unknownDataSourceID,
				document.AttrSimulationActive: true,
			},
			Stored: document.Block{Type: document.BlockNodeParagraph},
			Result: map[string]any{
				document.AttrDataSourceID:     _unknownDataSourceID,
				document.AttrSimulationActive: true,
			},
		},
		"An empty data source is not looked up": {
			Attrs:  map[string]any{document.AttrDataSourceID: ""},
			Stored: stored,
			Result: map[string]any{document.AttrDataSourceID: ""},
		},
		"A metric has its simulation flag derived": {
			Attrs: map[string]any{
				document.AttrDataSourceID:     _testDataSourceID.String(),
				document.AttrSimulationPreset: "error_rate",
				document.AttrSimulationActive: false,
			},
			Stored: stored,
			Result: map[string]any{
				document.AttrDataSourceID:     _testDataSourceID.String(),
				document.AttrSimulationPreset: "error_rate",
				document.AttrSimulationActive: true,
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(stubMetricDB(), nil, nil), NameUpdateBlockAttrs, `{}`)

			err := sanitizeBlockAttrs(inp, c.Attrs, c.Stored)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, c.Attrs)
		})
	}
}

func Test_docTarget_validate(t *testing.T) {
	t.Parallel()

	// error: each half is required, the document first.
	err := docTarget{BranchID: _stubMainBranchID}.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "document_id is required")

	err = docTarget{DocumentID: _testDocID}.validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch_id is required")

	// success
	require.NoError(t, docTarget{DocumentID: _testDocID, BranchID: _stubMainBranchID}.validate())
}

func Test_position_UnmarshalText(t *testing.T) {
	t.Parallel()

	for _, want := range []position{positionBefore, positionAfter, positionStart, positionEnd} {
		var got position

		require.NoError(t, got.UnmarshalText([]byte(want)))
		assert.Equal(t, want, got)
	}

	// anything else is refused by name, so the decoder can report the
	// argument rather than the model guessing what went wrong.
	var got position

	err := got.UnmarshalText([]byte("sideways"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "position must be one of")
}

func Test_position_relative(t *testing.T) {
	t.Parallel()

	assert.True(t, positionBefore.relative())
	assert.True(t, positionAfter.relative())
	assert.False(t, positionStart.relative())
	assert.False(t, positionEnd.relative())
}
