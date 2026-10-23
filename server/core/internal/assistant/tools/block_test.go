package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
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

// stubContentDB answers content reads with a paragraph and a callout
// holding a paragraph. err makes the branch fetch fail.
func stubContentDB(err error) *DBMock {
	return stubRootDB(document.RootBlock{
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
	DB *DBMock

	// ApplyErr is what the applier fails with. Nil accepts the edit.
	ApplyErr error

	Args     string
	Contains []string

	// Ops are the operations the write should send, in order, each
	// given by a piece of its JSON. Empty skips the check.
	Ops []string

	// FetchDataSourceCalls, FetchDocumentByBranchIDCalls and ApplyCalls
	// are how many times the write reaches each collaborator.
	FetchDataSourceCalls         int
	FetchDocumentByBranchIDCalls int
	ApplyCalls                   int

	Err error
}

// runEdit executes a block tool and asserts the shared outcome.
func runEdit(t *testing.T, tl Tool, name Name, c editCase) {
	t.Helper()

	applier := &EditApplierMock{
		ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
			return edit.Result{Errors: []edit.OpError{}}, c.ApplyErr
		},
	}

	res, err := tl.Execute(testDeps(c.DB, applier, nil).newInput(context.Background(), name, json.RawMessage(c.Args)))
	testutil.AssertEqualError(t, c.Err, err)

	assert.Len(t, c.DB.FetchDataSourceCalls(), c.FetchDataSourceCalls)
	assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
	assert.Empty(t, c.DB.FetchDocumentBranchesCalls())
	assert.Len(t, applier.ApplyCalls(), c.ApplyCalls)

	if err != nil {
		// a write that was refused never reached the document: every
		// check runs before the edit is shipped, so a rejected call
		// leaves nothing half-applied. Only the applier's own failure
		// comes after it.
		if c.ApplyErr == nil {
			assert.Empty(t, applier.ApplyCalls(), "a refused write must not reach the document")
		}

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

	assert.True(t, positionBefore.relative())
	assert.True(t, positionAfter.relative())
	assert.False(t, positionStart.relative())
	assert.False(t, positionEnd.relative())
}

func Test_insertBlocksArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args insertBlocksArgs
		Err  error
	}{
		"Document id is required": {
			Args: insertBlocksArgs{Position: positionEnd, Content: "<p>x</p>"},
			Err:  errors.New("document_id is required"),
		},
		"Position is required": {
			Args: insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
			Err:  errors.New("position is required"),
		},
		"Reference block uid is required": {
			Args: insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, Content: "<p>x</p>"},
			Err:  errors.New("reference_block_uid is required"),
		},
		// a reference only means something beside a block.
		"Reference block uid at an end of the document": {
			Args: insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, ReferenceBlockUID: "a", Content: "<p>x</p>"},
			Err:  assert.AnError,
		},
		"Content is required": {
			Args: insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd},
			Err:  errors.New("content is required"),
		},
		"Valid arguments": {
			Args: insertBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionEnd, Content: "<p>x</p>"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
		})
	}
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

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string

		// FetchDocumentByBranchIDCalls is how many times the summary
		// reads the branch.
		FetchDocumentByBranchIDCalls int

		Err error
	}{
		"At the start": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"start","content":"<p>a</p><p>b</p>"}`,
			Result:                       "Insert 2 blocks at the start of Runbook",
			FetchDocumentByBranchIDCalls: 1,
		},
		"At the end": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p>a</p>"}`,
			Result:                       "Insert 1 block at the end of Runbook",
			FetchDocumentByBranchIDCalls: 1,
		},
		"Beside a block": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"after","reference_block_uid":"a","content":"<p>a</p>"}`,
			Result:                       "Insert 1 block after a block in Runbook",
			FetchDocumentByBranchIDCalls: 1,
		},
		"Malformed content": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"<p>a"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Unreadable arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           failingDocumentDB(),
			Args:                         requiredArgs(t, NameInsertBlocks),
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := insertBlocks{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameInsertBlocks, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}
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
			DB:                   stubDataSource(stubContentDB(nil)),
			Args:                 `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + metricContent(_unknownDataSourceID) + `}`,
			FetchDataSourceCalls: 1,
			Err:                  assert.AnError,
		},
		"Data source the organisation owns": {
			DB:                           stubDataSource(stubContentDB(nil)),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + metricContent(_testDataSourceID.String()) + `}`,
			Contains:                     []string{`<metrics id=`},
			Ops:                          []string{`"kind":"append"`},
			FetchDataSourceCalls:         1,
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           stubContentDB(assert.AnError),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + two + `}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Content holding no block": {
			DB:   stubContentDB(nil),
			Args: `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":"  \n "}`,
			Err:  assert.AnError,
		},
		// the rest go after the first, since each append would land
		// before an empty paragraph the write itself put last.
		"Appended in order": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"end","content":` + two + `}`,
			Contains:                     []string{`>one</p>\n<p id=`, `>two</p>`},
			Ops:                          []string{`"kind":"append"`, `"kind":"insert","position":"after"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Prepended with the rest chained after the first": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"start","content":` + two + `}`,
			Ops:                          []string{`"kind":"prepend"`, `"kind":"insert","position":"after"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Inserted before a block": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"position":"before","reference_block_uid":"a","content":` + two + `}`,
			Ops:                          []string{`"position":"before","reference_uid":"a"`, `"position":"after"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Inserted after a block on another branch": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubBranchID) + `,"position":"after","reference_block_uid":"a","content":"<p>x</p>"}`,
			Ops:                          []string{`"position":"after","reference_uid":"a"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
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

	cc := map[string]struct {
		Args replaceBlocksArgs
		Err  error
	}{
		"Document id is required": {
			Args: replaceBlocksArgs{BlockUID: "a", Content: "<p>x</p>"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Args: replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Content: "<p>x</p>"},
			Err:  errors.New("block_uid is required"),
		},
		"Content is required": {
			Args: replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a"},
			Err:  errors.New("content is required"),
		},
		"Valid arguments": {
			Args: replaceBlocksArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "a", Content: "<p>x</p>"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
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

func Test_replaceBlocks_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string

		// FetchDocumentByBranchIDCalls is how many times the summary
		// reads the branch.
		FetchDocumentByBranchIDCalls int

		Err error
	}{
		"Successful summary": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">x</p><p>y</p>"}`,
			Result:                       "Replace a block in Runbook with 2 blocks",
			FetchDocumentByBranchIDCalls: 1,
		},
		// content that does not build is refused before anything is shown.
		"Content that does not build": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"nope\">x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		// the markup is built against the replaced block only, as the write
		// builds it, so an id from elsewhere in the document is refused here.
		"Id of a block outside the one replaced": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"cp\">x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Block the document does not hold": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","content":"<p>x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          fmt.Errorf("block %s: %w", "zzz", errUnknownBlock),
		},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           failingDocumentDB(),
			Args:                         requiredArgs(t, NameReplaceBlocks),
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Unreadable arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := replaceBlocks{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameReplaceBlocks, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}
}

func Test_replaceBlocks_Execute(t *testing.T) {
	t.Parallel()

	cc := map[string]editCase{
		"Malformed arguments": {DB: stubContentDB(nil), Args: `{`, Err: assert.AnError},
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           stubContentDB(assert.AnError),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p>x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Block the document does not hold": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","content":"<p>x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Malformed content": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p>x"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Content holding no block": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"  "}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		// the callout's paragraph stays where it is, so keeping its id
		// here would put it in the document twice.
		"Id of a block outside the one replaced": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"cp\">x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Data source the organisation does not own": {
			DB:                           stubDataSource(stubSimulationDB()),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"g","content":` + metricContent(_unknownDataSourceID) + `}`,
			FetchDataSourceCalls:         1,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB:                           stubContentDB(nil),
			ApplyErr:                     assert.AnError,
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">x</p>"}`,
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
			Err:                          assert.AnError,
		},
		"Text rewritten in place": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">new <b>words</b></p>"}`,
			Contains:                     []string{`<p id=\"a\">new <b>words</b></p>`},
			Ops:                          []string{`"kind":"replace","block_uid":"a","block":{"type":"paragraph"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"One block becomes several": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","content":"<p id=\"a\">one</p><h2>two</h2>"}`,
			Ops:                          []string{`"kind":"replace","block_uid":"a"`, `"kind":"insert","position":"after","reference_uid":"a"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Nested block kept by its id": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"c","content":"<callout id=\"c\" icon=\"lucide:info\"><p id=\"cp\">inside</p><p>more</p></callout>"}`,
			Contains:                     []string{`<p id=\"cp\">inside</p>`},
			Ops:                          []string{`"kind":"replace","block_uid":"c"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		// the metric's data arrived, and re-sending its preset does not
		// switch the simulation back on.
		"Metric keeps its simulation flag": {
			DB:                           stubSimulationDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"` + _stubMetricUID + `","content":"<metric id=\"m\">{\"simulationPreset\":\"cpu_usage\",\"width\":\"wide\"}</metric>"}`,
			Ops:                          []string{`"simulationActive":false`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
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

			ops := insertBlocksArgs{Position: c.Position, ReferenceBlockUID: c.ReferenceBlockUID}.insertOps(blocks)
			kinds := make([]string, 0, len(ops))

			for _, op := range ops {
				kinds = append(kinds, op.Kind+" "+op.Position+" "+op.ReferenceUID)
			}

			assert.Equal(t, c.Result, kinds)
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

func Test_deleteBlockArgs_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args deleteBlockArgs
		Err  error
	}{
		"Document id is required": {
			Args: deleteBlockArgs{BlockUID: "b"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Args: deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID},
			Err:  errors.New("block_uid is required"),
		},
		"Valid arguments": {
			Args: deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
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

func Test_deleteBlock_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string

		// FetchDocumentByBranchIDCalls is how many times the summary
		// reads the branch.
		FetchDocumentByBranchIDCalls int

		Err error
	}{
		"Successful summary": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			Result:                       "Delete a block in Runbook",
			FetchDocumentByBranchIDCalls: 1,
		},
		// the document it names has to resolve; the failure is passed on
		// rather than described around.
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           failingDocumentDB(),
			Args:                         requiredArgs(t, NameDeleteBlock),
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		// unreadable arguments are refused before anything is looked up.
		"Unreadable arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := deleteBlock{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameDeleteBlock, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}
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
		"Error returned by applier.Apply": {
			DB:                           stubDocumentDB(),
			ApplyErr:                     assert.AnError,
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
			Err:                          assert.AnError,
		},
		"Deleted": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a"}`,
			Contains:                     []string{`"deleted":"a"`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
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

	cc := map[string]struct {
		Args moveBlockArgs
		Err  error
	}{
		"Document id is required": {
			Args: moveBlockArgs{BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
			Err:  errors.New("document_id is required"),
		},
		"Block uid is required": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, Position: positionAfter, ReferenceBlockUID: "r"},
			Err:  errors.New("block_uid is required"),
		},
		"Position is required": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", ReferenceBlockUID: "r"},
			Err:  errors.New("position is required"),
		},
		"Reference block uid is required": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter},
			Err:  errors.New("reference_block_uid is required"),
		},
		// a block cannot be moved relative to itself.
		"Reference block uid naming the block": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "b"},
			Err:  errors.New("reference_block_uid must differ from block_uid"),
		},
		// a move lands beside a block, never at an end of the document.
		"Position at an end of the document": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionEnd, ReferenceBlockUID: "r"},
			Err:  errors.New(`position must be "before" or "after" for a move, got "end"`),
		},
		"Valid arguments": {
			Args: moveBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b", Position: positionAfter, ReferenceBlockUID: "r"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, c.Args.Validate())
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

func Test_moveBlock_Summary(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string

		// FetchDocumentByBranchIDCalls is how many times the summary
		// reads the branch.
		FetchDocumentByBranchIDCalls int

		Err error
	}{
		"Successful summary": {
			DB:                           stubDocumentDB(),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"a","position":"after","reference_block_uid":"b"}`,
			Result:                       "Move a block after another block in Runbook",
			FetchDocumentByBranchIDCalls: 1,
		},
		// the document it names has to resolve; the failure is passed on
		// rather than described around.
		"Error returned by db.FetchDocumentByBranchID": {
			DB:                           failingDocumentDB(),
			Args:                         requiredArgs(t, NameMoveBlock),
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		// unreadable arguments are refused before anything is looked up.
		"Unreadable arguments": {DB: stubDocumentDB(), Args: `{`, Err: assert.AnError},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := moveBlock{}.Summary(testDeps(c.DB, nil, nil).newInput(context.Background(), NameMoveBlock, json.RawMessage(c.Args)))
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), c.FetchDocumentByBranchIDCalls)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got.Summary)
		})
	}
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
			DB:                           stubContentDB(assert.AnError),
			Args:                         base + `,"position":"after"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Block uid the document does not hold": {
			DB:                           stubContentDB(nil),
			Args:                         `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"zzz","position":"after","reference_block_uid":"b"}`,
			FetchDocumentByBranchIDCalls: 1,
			Err:                          assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB:                           stubContentDB(nil),
			ApplyErr:                     assert.AnError,
			Args:                         base + `,"position":"after"}`,
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
			Err:                          assert.AnError,
		},
		"Moved before": {
			DB:                           stubContentDB(nil),
			Args:                         base + `,"position":"before"}`,
			Contains:                     []string{`<p id=\"a\">hello</p>`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
		"Moved after": {
			DB:                           stubContentDB(nil),
			Args:                         base + `,"position":"after"}`,
			Contains:                     []string{`<p id=\"a\">hello</p>`},
			FetchDocumentByBranchIDCalls: 1,
			ApplyCalls:                   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runEdit(t, moveBlock{}, NameMoveBlock, c)
		})
	}
}
