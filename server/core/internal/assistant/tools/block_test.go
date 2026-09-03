package tools

import (
	"context"
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

// stubContentDB answers content reads with a single paragraph the
// placement checks can find.
func stubContentDB(err error) *DBMock {
	db := stubDocumentDB()
	db.FetchMainBranchContentFunc = func(context.Context, xid.ID, string) (document.Content, error) {
		if err != nil {
			return document.Content{}, err
		}

		return document.Content{
			DocumentName: "Runbook",
			Content: document.RootBlock{
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
			},
		}, nil
	}

	return db
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
	db := stubContentDB(nil)
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
	Err      error
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
}

func Test_readBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	assertValidate(t, readBlockArgs{DocumentID: _testDocID, BlockUID: "b"}, map[string]Args{
		"document_id": readBlockArgs{BlockUID: "b"},
		"block_uid":   readBlockArgs{DocumentID: _testDocID},
	})
}

func Test_readBlock_Info(t *testing.T) {
	t.Parallel()

	info := readBlock{}.Info()

	assert.Equal(t, NameReadBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID}, info.Required)
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
			Args: `{"document_id":"` + _testDocID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Block is absent": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"missing"}`,
			Err:  assert.AnError,
		},
		"Block is returned": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID.String() + `","block_uid":"a"}`,
			Contains: `"paragraph"`,
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

	assertValidate(t, insertBlockArgs{DocumentID: _testDocID, ReferenceBlockUID: "r", Position: positionAfter, Block: para}, map[string]Args{
		"document_id":         insertBlockArgs{ReferenceBlockUID: "r", Position: positionAfter, Block: para},
		"reference_block_uid": insertBlockArgs{DocumentID: _testDocID, Position: positionAfter, Block: para},
		"position":            insertBlockArgs{DocumentID: _testDocID, ReferenceBlockUID: "r", Block: para},
		"block":               insertBlockArgs{DocumentID: _testDocID, ReferenceBlockUID: "r", Position: positionAfter},
	})

	// an end of the document needs no reference, and refuses one: a
	// reference given with start or end is a contradiction, not a hint.
	require.NoError(t, insertBlockArgs{DocumentID: _testDocID, Position: positionEnd, Block: para}.Validate())

	err := insertBlockArgs{DocumentID: _testDocID, ReferenceBlockUID: "r", Position: positionStart, Block: para}.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reference_block_uid applies to before and after only")
}

func Test_insertBlock_Info(t *testing.T) {
	t.Parallel()

	info := insertBlock{}.Info()

	assert.Equal(t, NameInsertBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, "position", _keyBlock}, info.Required)

	// the four positions are what the model is shown.
	pos, ok := info.Properties["position"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []string{"before", "after", "start", "end"}, pos[_keyEnum])
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
	beside := `{"document_id":"` + _testDocID.String() + `","reference_block_uid":"a","block":` + _paragraphArgs
	atEnd := `{"document_id":"` + _testDocID.String() + `","block":` + _paragraphArgs

	cc := map[string]struct {
		Args   string
		Result string
	}{
		"Beside a block": {Args: beside + `,"position":"after"}`, Result: "Insert a paragraph after a block in Runbook"},
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

	beside := `{"document_id":"` + _testDocID.String() + `","reference_block_uid":"a","block":` + _paragraphArgs
	atEnd := `{"document_id":"` + _testDocID.String() + `","block":` + _paragraphArgs
	paragraphRow := []string{`"kind":"paragraph"`, `"text":"hello"`, `"depth":0`}

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Reference uid is required beside a block": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","position":"after","block":` + _paragraphArgs + `}`,
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
			Args: `{"document_id":"` + _testDocID.String() + `","reference_block_uid":"a","position":"after",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"A type the root refuses": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","position":"end","block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err:  assert.AnError,
		},
		"Inserted before":       {DB: stubContentDB(nil), Args: beside + `,"position":"before"}`, Contains: paragraphRow},
		"Inserted after":        {DB: stubContentDB(nil), Args: beside + `,"position":"after"}`, Contains: paragraphRow},
		"Inserted at the start": {DB: stubContentDB(nil), Args: atEnd + `,"position":"start"}`, Contains: paragraphRow},
		"Inserted at the end":   {DB: stubContentDB(nil), Args: atEnd + `,"position":"end"}`, Contains: paragraphRow},
		// a list carries its entries' uids too, so a follow-up edit can
		// target an entry without reading the document back.
		"A list reports its entries": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID.String() + `","position":"end","block":{"type":"bullet_list","items":[{"type":"paragraph","text":"one"}]}}`,
			Contains: []string{`"kind":"bullet_list"`, `"kind":"list_item"`, `"depth":1`, `"parent_uid"`},
		},
		"A metric naming a data source the organisation owns": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","reference_block_uid":"` + _stubContentUID +
				`","position":"after","block":` + metricBlockArgs(_testDataSourceID.String()) + `}`,
			Contains: []string{`"kind":"metric_grid"`, `"kind":"metric"`},
		},
		"A metric naming a data source it does not": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","reference_block_uid":"` + _stubContentUID +
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

	assertValidate(t, replaceBlockArgs{DocumentID: _testDocID, BlockUID: "b", Block: block.Block{Type: block.BlockParagraph}}, map[string]Args{
		"document_id": replaceBlockArgs{BlockUID: "b", Block: block.Block{Type: block.BlockParagraph}},
		"block_uid":   replaceBlockArgs{DocumentID: _testDocID, Block: block.Block{Type: block.BlockParagraph}},
		"block":       replaceBlockArgs{DocumentID: _testDocID, BlockUID: "b"},
	})
}

func Test_replaceBlock_Info(t *testing.T) {
	t.Parallel()

	info := replaceBlock{}.Info()

	assert.Equal(t, NameReplaceBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, _keyBlock}, info.Required)
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
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a","block":`+_paragraphArgs+`}`,
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
			Args: `{"document_id":"` + _testDocID.String() + `","block":` + _paragraphArgs + `}`,
			Err:  assert.AnError,
		},
		"Error returned by the placement check": {
			DB: stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"Replaced": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID.String() + `","block_uid":"a","block":` + _paragraphArgs + `}`,
			Contains: []string{`"kind":"paragraph"`, `"text":"hello"`},
		},
		"A metric grid naming a data source the organisation owns": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"` + _stubContentUID +
				`","block":` + metricBlockArgs(_testDataSourceID.String()) + `}`,
		},
		"A metric grid naming a data source it does not": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"` + _stubContentUID +
				`","block":` + metricBlockArgs(_unknownDataSourceID) + `}`,
			Err: assert.AnError,
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

	assertValidate(t, updateBlockTextArgs{DocumentID: _testDocID, BlockUID: "b", Text: "t"}, map[string]Args{
		"document_id": updateBlockTextArgs{BlockUID: "b", Text: "t"},
		"block_uid":   updateBlockTextArgs{DocumentID: _testDocID, Text: "t"},
		"text":        updateBlockTextArgs{DocumentID: _testDocID, BlockUID: "b"},
	})
}

func Test_updateBlockText_Info(t *testing.T) {
	t.Parallel()

	info := updateBlockText{}.Info()

	assert.Equal(t, NameUpdateBlockText, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, "text"}, info.Required)
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
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a","text":"a new intro"}`))
	require.NoError(t, err)
	assert.Equal(t, `Update a block in Runbook: "a new intro"`, got.Summary)

	// an empty preview leaves a card that still reads.
	got, err = updateBlockText{}.Summary(testInput(d, NameUpdateBlockText,
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a","text":"  "}`))
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
			Args: `{"document_id":"` + _testDocID.String() + `","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"zzz","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Text written": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID.String() + `","block_uid":"a","text":"hi"}`,
			Contains: []string{`"uid":"a"`, `"text":"hi"`},
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

	assertValidate(t, updateBlockAttrsArgs{DocumentID: _testDocID, BlockUID: "b", Attrs: map[string]any{"level": 2}}, map[string]Args{
		"document_id": updateBlockAttrsArgs{BlockUID: "b", Attrs: map[string]any{"level": 2}},
		"block_uid":   updateBlockAttrsArgs{DocumentID: _testDocID, Attrs: map[string]any{"level": 2}},
		"attrs":       updateBlockAttrsArgs{DocumentID: _testDocID, BlockUID: "b"},
	})
}

func Test_updateBlockAttrs_Info(t *testing.T) {
	t.Parallel()

	info := updateBlockAttrs{}.Info()

	assert.Equal(t, NameUpdateBlockAttrs, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, "attrs"}, info.Required)
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
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a","attrs":{"level":2,"icon":"lucide:warning"}}`))
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
			Args: `{"document_id":"` + _testDocID.String() + `","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Attrs must not be empty": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"h","attrs":{}}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"h","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"zzz","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Attrs applied": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID.String() + `","block_uid":"h","attrs":{"level":2}}`,
			Contains: []string{`"uid":"h"`, `"kind":"heading"`, `"level":2`},
		},
		// the payload names attributes, not a block type, so a metric's
		// data source arrives on its own rather than inside a block.
		"A data source the organisation owns": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","attrs":{"dataSourceId":"` +
				_testDataSourceID.String() + `"}}`,
		},
		"A data source it does not": {
			DB: stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","attrs":{"dataSourceId":"` +
				_unknownDataSourceID + `"}}`,
			Err: assert.AnError,
		},
		// an empty data source is the editor's "unset", not a reference
		// to check, and no other attribute names one at all.
		"An empty data source is not looked up": {
			DB:   stubMetricDB(),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","attrs":{"dataSourceId":""}}`,
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

	assertValidate(t, deleteBlockArgs{DocumentID: _testDocID, BlockUID: "b"}, map[string]Args{
		"document_id": deleteBlockArgs{BlockUID: "b"},
		"block_uid":   deleteBlockArgs{DocumentID: _testDocID},
	})
}

func Test_deleteBlock_Info(t *testing.T) {
	t.Parallel()

	info := deleteBlock{}.Info()

	assert.Equal(t, NameDeleteBlock, info.Name)
	assert.Contains(t, info.Description, "cannot be restored")
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID}, info.Required)
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
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a"}`,
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
			Args: `{"document_id":"` + _testDocID.String() + `"}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:       stubDocumentDB(),
			Args:     `{"document_id":"` + _testDocID.String() + `","block_uid":"a"}`,
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

	ok := moveBlockArgs{DocumentID: _testDocID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"}

	assertValidate(t, ok, map[string]Args{
		"document_id":         moveBlockArgs{BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
		"block_uid":           moveBlockArgs{DocumentID: _testDocID, Position: positionAfter, ReferenceBlockUID: "r"},
		"position":            moveBlockArgs{DocumentID: _testDocID, BlockUID: "b", ReferenceBlockUID: "r"},
		"reference_block_uid": moveBlockArgs{DocumentID: _testDocID, BlockUID: "b", Position: positionAfter},
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
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, "position", "reference_block_uid"}, info.Required)
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
		`{"document_id":"`+_testDocID.String()+`","block_uid":"a","position":"after","reference_block_uid":"b"}`,
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

	base := `{"document_id":"` + _testDocID.String() + `","block_uid":"a","reference_block_uid":"b"`

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Reference uid is required": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","position":"after"}`,
			Err:  assert.AnError,
		},
		"Position must be before or after": {
			DB:   stubContentDB(nil),
			Args: base + `,"position":"end"}`,
			Err:  assert.AnError,
		},
		"Reference must differ from the block": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"a","position":"after","reference_block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: base + `,"position":"after"}`,
			Err:  assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"zzz","position":"after","reference_block_uid":"b"}`,
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
