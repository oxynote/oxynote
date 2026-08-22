package tools

import (
	"context"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// _paragraphArgs is a canonical paragraph a write tool can be handed.
const _paragraphArgs = `{"type":"paragraph","text":"hello"}`

// _stubContentUID is the uid of the one paragraph stubContentDB
// answers with; the write cases reference it as their edit target.
const _stubContentUID = "a"

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
				Content: []document.Block{{
					Type:  document.BlockNodeParagraph,
					Text:  "hello",
					Attrs: document.Attributes{document.AttrUID: _stubContentUID},
				}},
			},
		}, nil
	}

	return db
}

// editCases are the argument-validation cases every block write shares.
type editCase struct {
	DB     *DBMock
	Args   string
	Result string
	Err    error
}

// runEdit executes a block tool and asserts the shared outcome.
func runEdit(t *testing.T, tl Tool, name Name, c editCase) {
	t.Helper()

	res, err := tl.Execute(testInput(testDeps(c.DB, stubApplier(), nil), name, c.Args))
	testutil.AssertEqualError(t, c.Err, err)

	if err != nil {
		return
	}

	assert.JSONEq(t, `{"applied":1,"errors":[]}`, res)
}

func Test_readDocumentSummary_Info(t *testing.T) {
	t.Parallel()

	info := readDocumentSummary{}.Info()

	assert.Equal(t, NameReadDocumentSummary, info.Name)
	assert.Equal(t, []string{_keyDocumentID}, info.Required)
}

func Test_readDocumentSummary_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{}, readDocumentSummary{}.Traits())
}

func Test_readDocumentSummary_Title(t *testing.T) {
	t.Parallel()

	got, err := readDocumentSummary{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameReadDocumentSummary,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Reading Runbook", got)
}

func Test_readDocumentSummary_Execute(t *testing.T) {
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
			Args: `{"document_id":"nope"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID + `"}`,
			Err:  assert.AnError,
		},
		"Summary is returned": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID + `"}`,
			Contains: `"document_name":"Runbook"`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := readDocumentSummary{}.Execute(
				testInput(testDeps(c.DB, nil, nil), NameReadDocumentSummary, c.Args),
			)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Contains(t, res, c.Contains)
		})
	}
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

	got, err := readBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameReadBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Reading a block in Runbook", got)
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
			Args: `{"document_id":"` + _testDocID + `"}`,
			Err:  assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB:   stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a"}`,
			Err:  assert.AnError,
		},
		"Block is absent": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"missing"}`,
			Err:  assert.AnError,
		},
		"Block is returned": {
			DB:       stubContentDB(nil),
			Args:     `{"document_id":"` + _testDocID + `","block_uid":"a"}`,
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

func Test_insertBlock_Info(t *testing.T) {
	t.Parallel()

	info := insertBlock{}.Info()

	assert.Equal(t, NameInsertBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, "reference_block_uid", "position", _keyBlock}, info.Required)
}

func Test_insertBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, insertBlock{}.Traits())
}

func Test_insertBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := insertBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameInsertBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_insertBlock_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)
	args := `{"document_id":"` + _testDocID + `","block":` + _paragraphArgs

	got, err := insertBlock{}.Summary(testInput(d, NameInsertBlock, args+`,"position":"after"}`))
	require.NoError(t, err)
	assert.Equal(t, "Insert a paragraph after a block in Runbook", got.Summary)

	// a missing or garbage position would garble the card, so it falls
	// back to an un-positioned phrase.
	got, err = insertBlock{}.Summary(testInput(d, NameInsertBlock, args+`,"position":"sideways"}`))
	require.NoError(t, err)
	assert.Equal(t, "Insert a paragraph in Runbook", got.Summary)
}

func Test_insertBlock_Execute(t *testing.T) {
	t.Parallel()

	base := `{"document_id":"` + _testDocID + `","reference_block_uid":"a","block":` + _paragraphArgs

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Reference uid is required": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID + `","position":"after","block":` + _paragraphArgs + `}`,
			Err:  assert.AnError,
		},
		"Position must be before or after": {
			DB:   stubContentDB(nil),
			Args: base + `,"position":"sideways"}`,
			Err:  assert.AnError,
		},
		"Error returned by the placement check": {
			DB: stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID + `","reference_block_uid":"a","position":"after",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"Inserted before": {DB: stubContentDB(nil), Args: base + `,"position":"before"}`},
		"Inserted after":  {DB: stubContentDB(nil), Args: base + `,"position":"after"}`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, insertBlock{}, NameInsertBlock, c)
		})
	}
}

func Test_appendBlock_Info(t *testing.T) {
	t.Parallel()

	info := appendBlock{}.Info()

	assert.Equal(t, NameAppendBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlock}, info.Required)
}

func Test_appendBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, appendBlock{}.Traits())
}

func Test_appendBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := appendBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameAppendBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_appendBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := appendBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameAppendBlock,
		`{"document_id":"`+_testDocID+`","block":`+_paragraphArgs+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Append a paragraph to Runbook", got.Summary)
}

func Test_appendBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block is not legal at the root": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err:  assert.AnError,
		},
		"Appended": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block":` + _paragraphArgs + `}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, appendBlock{}, NameAppendBlock, c)
		})
	}
}

func Test_prependBlock_Info(t *testing.T) {
	t.Parallel()

	info := prependBlock{}.Info()

	assert.Equal(t, NamePrependBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlock}, info.Required)
}

func Test_prependBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, prependBlock{}.Traits())
}

func Test_prependBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := prependBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NamePrependBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_prependBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := prependBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NamePrependBlock,
		`{"document_id":"`+_testDocID+`","block":`+_paragraphArgs+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Prepend a paragraph to Runbook", got.Summary)
}

func Test_prependBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block is not legal at the root": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err:  assert.AnError,
		},
		"Prepended": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block":` + _paragraphArgs + `}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, prependBlock{}, NamePrependBlock, c)
		})
	}
}

func Test_replaceBlock_Info(t *testing.T) {
	t.Parallel()

	info := replaceBlock{}.Info()

	assert.Equal(t, NameReplaceBlock, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, _keyBlock}, info.Required)
}

func Test_replaceBlock_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, replaceBlock{}.Traits())
}

func Test_replaceBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := replaceBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_replaceBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := replaceBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameReplaceBlock,
		`{"document_id":"`+_testDocID+`","block":`+_paragraphArgs+`}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Replace a block in Runbook with a paragraph", got.Summary)
}

func Test_replaceBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID + `","block":` + _paragraphArgs + `}`,
			Err:  assert.AnError,
		},
		"Error returned by the placement check": {
			DB: stubContentDB(assert.AnError),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a",` +
				`"block":{"type":"titled_code","text":"x","attrs":{"title":"T"}}}`,
			Err: assert.AnError,
		},
		"Replaced": {
			DB:   stubContentDB(nil),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a","block":` + _paragraphArgs + `}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, replaceBlock{}, NameReplaceBlock, c)
		})
	}
}

func Test_updateBlockText_Info(t *testing.T) {
	t.Parallel()

	info := updateBlockText{}.Info()

	assert.Equal(t, NameUpdateBlockText, info.Name)
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID, "text"}, info.Required)
}

func Test_updateBlockText_Traits(t *testing.T) {
	t.Parallel()

	assert.Equal(t, Traits{Write: true}, updateBlockText{}.Traits())
}

func Test_updateBlockText_Title(t *testing.T) {
	t.Parallel()

	got, err := updateBlockText{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockText,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_updateBlockText_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	got, err := updateBlockText{}.Summary(testInput(d, NameUpdateBlockText,
		`{"document_id":"`+_testDocID+`","text":"a new intro"}`))
	require.NoError(t, err)
	assert.Equal(t, `Update a block in Runbook: "a new intro"`, got.Summary)

	// an empty preview leaves a card that still reads.
	got, err = updateBlockText{}.Summary(testInput(d, NameUpdateBlockText,
		`{"document_id":"`+_testDocID+`","text":"  "}`))
	require.NoError(t, err)
	assert.Equal(t, "Update text of a block in Runbook", got.Summary)
}

func Test_updateBlockText_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","text":"hi"}`,
			Err:  assert.AnError,
		},
		"Text written": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a","text":"hi"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, updateBlockText{}, NameUpdateBlockText, c)
		})
	}
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

	got, err := updateBlockAttrs{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameUpdateBlockAttrs,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_updateBlockAttrs_Summary(t *testing.T) {
	t.Parallel()

	d := testDeps(stubDocumentDB(), nil, nil)

	// keys are sorted, because the card must read the same every time
	// the same write is proposed.
	got, err := updateBlockAttrs{}.Summary(testInput(d, NameUpdateBlockAttrs,
		`{"document_id":"`+_testDocID+`","attrs":{"level":2,"icon":"lucide:warning"}}`))
	require.NoError(t, err)
	assert.Equal(t, "Update block icon, level in Runbook", got.Summary)

	got, err = updateBlockAttrs{}.Summary(testInput(d, NameUpdateBlockAttrs,
		`{"document_id":"`+_testDocID+`"}`))
	require.NoError(t, err)
	assert.Equal(t, "Update block attributes in Runbook", got.Summary)
}

func Test_updateBlockAttrs_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","attrs":{"level":2}}`,
			Err:  assert.AnError,
		},
		"Attrs must not be empty": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a","attrs":{}}`,
			Err:  assert.AnError,
		},
		"Attrs applied": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a","attrs":{"level":2}}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, updateBlockAttrs{}, NameUpdateBlockAttrs, c)
		})
	}
}

func Test_deleteBlock_Info(t *testing.T) {
	t.Parallel()

	info := deleteBlock{}.Info()

	assert.Equal(t, NameDeleteBlock, info.Name)
	assert.Contains(t, info.Description, "destructive")
	assert.Equal(t, []string{_keyDocumentID, _keyBlockUID}, info.Required)
}

func Test_deleteBlock_Traits(t *testing.T) {
	t.Parallel()

	// a delete stays outside any "approve all" answer.
	assert.Equal(t, Traits{Write: true, Destructive: true}, deleteBlock{}.Traits())
}

func Test_deleteBlock_Title(t *testing.T) {
	t.Parallel()

	got, err := deleteBlock{}.Title(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock,
		`{"document_id":"`+_testDocID+`"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Updating Runbook", got)
}

func Test_deleteBlock_Summary(t *testing.T) {
	t.Parallel()

	got, err := deleteBlock{}.Summary(testInput(
		testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock,
		`{"document_id":"`+_testDocID+`","block_uid":"a"}`,
	))
	require.NoError(t, err)
	assert.Equal(t, "Delete a block in Runbook", got.Summary)
}

func Test_deleteBlock_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Block uid is required": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `"}`,
			Err:  assert.AnError,
		},
		"Deleted": {
			DB:   stubDocumentDB(),
			Args: `{"document_id":"` + _testDocID + `","block_uid":"a"}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, deleteBlock{}, NameDeleteBlock, c)
		})
	}
}

func Test_summarize(t *testing.T) {
	t.Parallel()

	// a resolvable document is named on the card.
	got := summarize(
		testInput(testDeps(stubDocumentDB(), nil, nil), NameDeleteBlock, `{}`),
		NameDeleteBlock,
		_testDocID,
		func(subject string) string { return "Touch " + subject },
	)

	assert.Equal(t, string(NameDeleteBlock), got.Tool)
	assert.Equal(t, _testDocID, got.DocumentID)
	assert.Equal(t, "Runbook", got.DocumentName)
	assert.Equal(t, "Touch Runbook", got.Summary)

	// a write that names no document still produces a readable card.
	got = summarize(
		testInput(testDeps(nil, nil, nil), NameDeleteBlock, `{}`),
		NameDeleteBlock,
		"",
		func(subject string) string { return "Touch " + subject },
	)

	assert.Empty(t, got.DocumentID)
	assert.Equal(t, "Touch document", got.Summary)
}
