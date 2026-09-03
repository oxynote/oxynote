package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// _testDocID is the document id the tool tests address.
var _testDocID = xid.New()

// discardLog returns a logger that writes nowhere.
func discardLog() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// testDeps builds session wiring with don't-care dependencies. Pass the
// mocks a test cares about; nil leaves a don't-care in place.
func testDeps(db *DBMock, applier *EditApplierMock, tree *TreeNotifierMock) *Deps {
	if db == nil {
		db = &DBMock{}
	}

	if applier == nil {
		applier = &EditApplierMock{}
	}

	d := &Deps{
		log: discardLog(),
		db:  db,
		search: &SearcherMock{
			ConfiguredFunc: func() bool { return true },
		},
		jobs: search.NewJobs(true),
		runners: &DataSourceRunnersMock{
			RunnerFunc: func(datasource.DataSource) datasource.Runner {
				return &datasourceMock.Runner{}
			},
		},
		applier: applier,
		offload: &offloadReaderMock{},
		orgID:   "org",
		userID:  "user",
	}

	// a nil notifier is a real case — NotifyTreeChange no-ops — so it is
	// only wired when a test asks for it.
	if tree != nil {
		d.tree = tree
	}

	return d
}

// testInput builds the per-call input a tool is handed.
func testInput(d *Deps, name Name, args string) *input {
	if args == "" {
		args = "{}"
	}

	return d.newInput(context.Background(), name, json.RawMessage(args))
}

// infoOf returns the named tool's description, or a bare one for a name
// the registry does not know.
func infoOf(name Name) Info {
	if e, ok := New(testDeps(nil, nil, nil)).Entry(name); ok {
		return e.Info
	}

	return Info{Name: name}
}

// assertValidate checks that ok passes Validate and that every entry of
// missing — ok with one required argument blanked — is refused naming
// that argument.
func assertValidate(t *testing.T, ok Args, missing map[string]Args) {
	t.Helper()

	require.NoError(t, ok.Validate())

	for key, a := range missing {
		err := a.Validate()
		require.Error(t, err, "%s should be required", key)
		assert.Contains(t, err.Error(), key+" is required")
	}
}

// requiredArgs builds the smallest payload the named tool's Decode
// accepts: every required argument, each with a value its type takes.
func requiredArgs(t *testing.T, name Name) string {
	t.Helper()

	vals := map[string]any{}

	for _, key := range infoOf(name).Required {
		switch key {
		case _keyDocumentID, _keyDataSourceID:
			vals[key] = _testDocID.String()
		case _keyBlock:
			vals[key] = map[string]any{_keyType: string(block.BlockParagraph)}
		case "position":
			// a position beside a block needs the reference the tool
			// requires; one that requires no reference takes an end.
			vals[key] = string(positionEnd)
			if slices.Contains(infoOf(name).Required, "reference_block_uid") {
				vals[key] = string(positionAfter)
			}
		case "reference_block_uid":
			// distinct from the "x" other uid keys take, so a payload
			// naming both a block and a reference is not a self-move.
			vals[key] = "r"
		case _keyMatchers:
			vals[key] = []string{"up"}
		case "attrs":
			vals[key] = map[string]any{"level": 2}
		default:
			vals[key] = "x"
		}
	}

	// update_document requires nothing beyond the document, but refuses
	// a call that changes nothing; a name is the smallest change.
	if name == NameUpdateDocument {
		vals[_keyName] = "x"
	}

	raw, err := json.Marshal(vals)
	require.NoError(t, err)

	return string(raw)
}

// offloadReaderMock is a stub offload reader; the offload tool's own
// test replaces it.
type offloadReaderMock struct {
	// read answers every retrieval.
	read func(path string) (string, error)
}

// Read returns the stubbed payload.
func (m *offloadReaderMock) Read(_ context.Context, path string) (string, error) {
	if m.read == nil {
		return "", nil
	}

	return m.read(path)
}

// _stubDocumentName is the display name every stubbed document lookup
// answers with, so a tool can resolve and name its target.
const _stubDocumentName = "Runbook"

// stubDocumentDB answers every document lookup with a resolvable
// document, so a tool can name its target.
func stubDocumentDB() *DBMock {
	branchID := xid.New()

	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			return &document.Document{
				BranchID:       branchID,
				DocumentName:   _stubDocumentName,
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
	}
}

// protectedDocumentDB answers with a document whose branch is
// protected, which is the one branch a tool's write may not reach.
func protectedDocumentDB() *DBMock {
	branchID := xid.New()

	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			return &document.Document{
				BranchID:       branchID,
				DocumentName:   _stubDocumentName,
				ID:             id,
				OrganizationID: orgID,
				Protected:      true,
			}, nil
		},
	}
}

// failingDocumentDB refuses every document lookup.
func failingDocumentDB() *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
			return nil, assert.AnError
		},
	}
}

// stubApplier accepts every edit it is handed.
func stubApplier() *EditApplierMock {
	return &EditApplierMock{
		ApplyFunc: func(_ context.Context, _, _ xid.ID, _ []edit.Operation, _ bool) (edit.Result, error) {
			return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
		},
	}
}

func Test_NewDeps(t *testing.T) {
	t.Parallel()

	var (
		db       = &DBMock{}
		searcher = &SearcherMock{}
		runner   = &datasourceMock.Runner{}
		applier  = &EditApplierMock{}
		tree     = &TreeNotifierMock{}
		offload  = &offloadReaderMock{}
	)

	runners := &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	jobs := search.NewJobs(true)

	d := NewDeps(discardLog(), db, searcher, jobs, runners, applier, tree, offload, "org", "user")
	require.NotNil(t, d)

	assert.NotNil(t, d.log)
	assert.Same(t, db, d.db)
	assert.Same(t, searcher, d.search)
	assert.Same(t, jobs, d.jobs)
	assert.Same(t, runners, d.runners)
	assert.Same(t, applier, d.applier)
	assert.Same(t, tree, d.tree)
	assert.Same(t, offload, d.offload)
	assert.Equal(t, "org", d.orgID)
	assert.Equal(t, "user", d.userID)
}

func Test_Deps_newInput(t *testing.T) {
	t.Parallel()

	d := testDeps(nil, nil, nil)
	ctx := context.Background()

	i := d.newInput(ctx, NameGetDocument, json.RawMessage(`{"a":1}`))
	require.NotNil(t, i)

	assert.Same(t, d, i.Deps)
	assert.Equal(t, NameGetDocument, i.name)
	assert.Equal(t, ctx, i.Context())
	assert.JSONEq(t, `{"a":1}`, string(i.args))
	assert.Equal(t, "org", i.OrganizationID())
	assert.Equal(t, "user", i.UserID())
}

func Test_input_Decode(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args string
		Err  string
		Want readBlockArgs
	}{
		"Malformed JSON": {Args: `{`, Err: "read_block: invalid input:"},
		// NOTE: json/v2 randomizes the modal verb of its error messages per
		// process ("cannot" / "unable to") to keep callers off the exact
		// wording, so the expectation starts after it.
		"Invalid id names the argument": {
			Args: `{"document_id":"nope","block_uid":"b"}`,
			Err:  `unmarshal JSON string into Go xid.ID within "/document_id": xid: invalid ID`,
		},
		"Null id is not an argument": {
			Args: `{"document_id":null,"block_uid":"b"}`,
			Err:  "read_block: document_id is required",
		},
		"Incomplete arguments are rejected by Validate": {
			Args: `{"document_id":"` + _testDocID.String() + `"}`,
			Err:  "read_block: block_uid is required",
		},
		"Unknown keys are ignored": {
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"b","extra":true}`,
			Want: readBlockArgs{DocumentID: _testDocID, BlockUID: "b"},
		},
		"Decoded": {
			Args: `{"document_id":"` + _testDocID.String() + `","block_uid":"b"}`,
			Want: readBlockArgs{DocumentID: _testDocID, BlockUID: "b"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var out readBlockArgs

			err := testInput(testDeps(nil, nil, nil), NameReadBlock, c.Args).Decode(&out)
			if c.Err != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), c.Err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, c.Want, out)
		})
	}
}

func Test_input_CreateDocument(t *testing.T) {
	t.Parallel()

	type tcase struct {
		DB      *DBMock
		Tx      *TxMock
		Parent  null.Value[xid.ID]
		Commits int
		Err     error
	}

	stubTx := func(insertErr, upsertErr, jobErr, commitErr error) *TxMock {
		return &TxMock{
			InsertDocumentFunc: func(context.Context, document.Document) error {
				return insertErr
			},
			UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
				return upsertErr
			},
			InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
				return jobErr
			},
			CommitFunc: func() error { return commitErr },
		}
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		return &DBMock{
			BeginTxFunc: func(_ context.Context, dest any) error {
				if beginErr != nil {
					return beginErr
				}

				reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(tx))

				return nil
			},
		}
	}

	cc := map[string]tcase{
		"Error returned by db.CheckDocumentExists": func() tcase {
			// the parent is checked here rather than by the caller, so
			// the invariant travels with the write that depends on it.
			db := stubDB(nil, nil)
			db.CheckDocumentExistsFunc = func(context.Context, xid.ID, string) error {
				return assert.AnError
			}

			return tcase{DB: db, Parent: null.ValueFrom(xid.New()), Err: assert.AnError}
		}(),
		"Error returned by db.BeginTx": func() tcase {
			return tcase{DB: stubDB(nil, assert.AnError), Err: assert.AnError}
		}(),
		"Error returned by Tx.InsertDocument": func() tcase {
			tx := stubTx(assert.AnError, nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Err: assert.AnError}
		}(),
		"Error returned by Tx.UpsertDocumentMaintainers": func() tcase {
			tx := stubTx(nil, assert.AnError, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Err: assert.AnError}
		}(),
		"Error returned by Tx.InsertDocumentSearchJob": func() tcase {
			tx := stubTx(nil, nil, assert.AnError, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Err: assert.AnError}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := stubTx(nil, nil, nil, assert.AnError)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Commits: 1, Err: assert.AnError}
		}(),
		"Every write lands together": func() tcase {
			tx := stubTx(nil, nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Commits: 1}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameCreateDocument, `{}`)

			doc := document.Document{ID: xid.New(), ParentID: c.Parent}

			err := inp.CreateDocument(doc)
			testutil.AssertEqualError(t, c.Err, err)

			// only a committed document exists to be pointed at.
			if err == nil {
				assert.Equal(t, []xid.ID{doc.ID}, inp.touched)
			} else {
				assert.Empty(t, inp.touched)
			}

			if c.Tx == nil {
				return
			}

			assert.Len(t, c.Tx.CommitCalls(), c.Commits)

			// the rollback is deferred unconditionally; after a
			// successful commit it is a harmless no-op.
			assert.Len(t, c.Tx.RollbackCalls(), 1)
		})
	}
}

func Test_input_DeleteDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	type tcase struct {
		DB      *DBMock
		Tx      *TxMock
		Commits int
		Jobs    int
		Err     error
	}

	stubTx := func(ids []xid.ID, deleteErr, jobErr, commitErr error) *TxMock {
		return &TxMock{
			DeleteDocumentFunc: func(context.Context, xid.ID, string) ([]xid.ID, error) {
				return ids, deleteErr
			},
			InsertDocumentSearchJobFunc: func(context.Context, search.BlocksDifference) error {
				return jobErr
			},
			CommitFunc: func() error { return commitErr },
		}
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		return &DBMock{
			BeginTxFunc: func(_ context.Context, dest any) error {
				if beginErr != nil {
					return beginErr
				}

				reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(tx))

				return nil
			},
		}
	}

	cc := map[string]tcase{
		"Error returned by db.BeginTx": func() tcase {
			return tcase{DB: stubDB(nil, assert.AnError), Err: assert.AnError}
		}(),
		"Error returned by Tx.DeleteDocument": func() tcase {
			tx := stubTx(nil, assert.AnError, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Err: assert.AnError}
		}(),
		"Error returned by Tx.InsertDocumentSearchJob": func() tcase {
			tx := stubTx([]xid.ID{docID}, nil, assert.AnError, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Jobs: 1, Err: assert.AnError}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := stubTx([]xid.ID{docID}, nil, nil, assert.AnError)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Commits: 1, Jobs: 1, Err: assert.AnError}
		}(),
		// a delete that matched nothing means the id names no document
		// in this organisation — a miss, not a success.
		"Empty subtree reports an unknown document": func() tcase {
			tx := stubTx(nil, nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Err: ErrUnknownDocument}
		}(),
		"Delete and search removal land together": func() tcase {
			tx := stubTx([]xid.ID{docID, xid.New()}, nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Commits: 1, Jobs: 1}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameDeleteDocument, `{}`)

			err := inp.DeleteDocument(docID)
			testutil.AssertEqualError(t, c.Err, err)

			// the document is gone, so nothing is recorded as touched —
			// a link to it would only point at something that no longer
			// exists.
			assert.Empty(t, inp.touched)

			if c.Tx == nil {
				return
			}

			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), 1)

			ff := c.Tx.InsertDocumentSearchJobCalls()
			require.Len(t, ff, c.Jobs)

			if c.Jobs != 0 {
				assert.NotEmpty(t, ff[0].Diff.RemovedDocuments)
			}
		})
	}
}

func Test_input_recordTouched(t *testing.T) {
	t.Parallel()

	inp := testInput(testDeps(nil, nil, nil), NameInsertBlock, `{}`)

	// a call that changed nothing reports nothing.
	assert.Empty(t, inp.touched)

	a, b := xid.New(), xid.New()

	inp.recordTouched(a)
	inp.recordTouched(b)

	// a document changed twice in one call is still one document, and
	// the order the call touched them is preserved.
	inp.recordTouched(a)

	// nothing to record is not something to record.
	inp.recordTouched(xid.NilID())

	assert.Equal(t, []xid.ID{a, b}, inp.touched)
}

func Test_input_MoveDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	parentID := null.ValueFrom(xid.New())

	moveDB := func(exists error, cycle bool, cycleErr, updateErr error) *DBMock {
		return &DBMock{
			CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error { return exists },
			CheckDocumentCycleFunc: func(context.Context, xid.ID, xid.ID, string) (bool, error) {
				return cycle, cycleErr
			},
			UpdateDocumentParentIDFunc: func(context.Context, xid.ID, null.Value[xid.ID], string) error {
				return updateErr
			},
		}
	}

	cc := map[string]struct {
		DB      *DBMock
		Parent  null.Value[xid.ID]
		Checks  int
		Updates int
		Err     error
	}{
		"Error returned by db.CheckDocumentExists": {
			DB:     moveDB(assert.AnError, false, nil, nil),
			Parent: parentID,
			Checks: 1,
			Err:    assert.AnError,
		},
		"Error returned by db.CheckDocumentCycle": {
			DB:     moveDB(nil, false, assert.AnError, nil),
			Parent: parentID,
			Checks: 1,
			Err:    assert.AnError,
		},
		"A move under the document's own subtree is refused": {
			DB:     moveDB(nil, true, nil, nil),
			Parent: parentID,
			Checks: 1,
			Err:    errCyclicParent,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:      moveDB(nil, false, nil, assert.AnError),
			Parent:  parentID,
			Checks:  1,
			Updates: 1,
			Err:     assert.AnError,
		},
		"Moved under a parent": {
			DB:      moveDB(nil, false, nil, nil),
			Parent:  parentID,
			Checks:  1,
			Updates: 1,
		},
		"A move to the root has no destination to check": {
			DB:      moveDB(nil, false, nil, nil),
			Updates: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameUpdateDocument, `{}`)

			err := inp.MoveDocument(docID, c.Parent)
			testutil.AssertEqualError(t, c.Err, err)

			// nothing is written until the destination is known good.
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.Checks)
			assert.Len(t, c.DB.UpdateDocumentParentIDCalls(), c.Updates)

			// the parents' own content is unchanged by a re-parent, so
			// the moved document is the only one recorded.
			if err == nil {
				assert.Equal(t, []xid.ID{docID}, inp.touched)
			} else {
				assert.Empty(t, inp.touched)
			}
		})
	}
}

func Test_input_ApplyEdit(t *testing.T) {
	t.Parallel()

	docID := xid.New()

	cc := map[string]struct {
		DB      *DBMock
		Applier *EditApplierMock
		Touched []xid.ID
		Applies int
		Err     error
	}{
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Applier: stubApplier(),
			Err:     assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, bool) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			Applies: 1,
			Err:     assert.AnError,
		},
		"Protected branch is refused before anything is applied": {
			DB:      protectedDocumentDB(),
			Applier: stubApplier(),
			Err:     errBranchProtected,
		},
		"Nothing applied is a failure, not a result": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, bool) (edit.Result, error) {
					return edit.Result{
						Errors: []edit.OpError{{Index: 0, Message: "block_uid not found: a"}},
					}, nil
				},
			},
			Applies: 1,
			Err:     errors.New("applying edit: no block with uid a in this document; call get_document for the current uids"),
		},
		"Partial failure still reports the outcome": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, bool) (edit.Result, error) {
					return edit.Result{
						Applied: 1,
						Errors:  []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			Touched: []xid.ID{docID},
			Applies: 1,
		},
		"Applied": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Touched: []xid.ID{docID},
			Applies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, c.Applier, nil), NameInsertBlock, `{}`)

			err := inp.ApplyEdit(docID, []edit.Operation{edit.Delete("a")})
			testutil.AssertEqualError(t, c.Err, err)

			// an edit that never landed has changed nothing to report.
			assert.Equal(t, c.Touched, inp.touched)

			// a refusal the tools own — a protected branch — stops
			// before the batch is shipped, rather than letting it land
			// and be undone by the persist behind it.
			assert.Len(t, c.Applier.ApplyCalls(), c.Applies)

			if err != nil {
				return
			}

			// a tool's write is a person's, never core's own: an
			// edit asking otherwise would land on a protected
			// branch, which is the one place a tool may not reach.
			if c.Applier != nil {
				for _, call := range c.Applier.ApplyCalls() {
					assert.False(t, call.System)
				}
			}
		})
	}
}

// contentWithBlocks builds a DB whose main-branch content is the given
// top-level blocks, for the validators that read the document to decide.
func contentWithBlocks(blocks ...document.Block) *DBMock {
	return &DBMock{
		FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
			return document.Content{
				Content: document.RootBlock{Content: blocks},
			}, nil
		},
	}
}

func Test_input_ValidateAttrUpdate(t *testing.T) {
	t.Parallel()

	// a heading at level 2, a metric inside its grid, and a task item —
	// a typed block with attr rules, a typed block with enum rules, and
	// a wrapper item the canonical model does not name.
	contentDB := contentWithBlocks(
		document.Block{
			Type: document.BlockNodeHeading,
			Attrs: document.Attributes{
				document.AttrUID:   "head",
				document.AttrLevel: 2,
			},
		},
		document.Block{
			Type:  document.BlockNodeMetricGrid,
			Attrs: document.Attributes{document.AttrUID: "grid"},
			Content: []document.Block{{
				Type: document.BlockNodeMetricBlock,
				Attrs: document.Attributes{
					document.AttrUID:              "metric",
					document.AttrSimulationPreset: "cpu_usage",
				},
			}},
		},
		document.Block{
			Type:  document.BlockNodeTaskList,
			Attrs: document.Attributes{document.AttrUID: "tasks"},
			Content: []document.Block{{
				Type:  document.BlockNodeTaskItem,
				Attrs: document.Attributes{document.AttrUID: "task"},
			}},
		},
	)

	cc := map[string]struct {
		DB    *DBMock
		UID   string
		Attrs map[string]any
		Err   error
	}{
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			UID:   "head",
			Attrs: map[string]any{document.AttrLevel: 2},
			Err:   assert.AnError,
		},
		"Unresolved uid is left to the backend": {
			DB:    contentDB,
			UID:   "missing",
			Attrs: map[string]any{document.AttrLevel: 9},
		},
		"Wrapper item carries attrs the canonical model does not name": {
			DB:    contentDB,
			UID:   "task",
			Attrs: map[string]any{"checked": true},
		},
		"Allowed heading level": {
			DB:    contentDB,
			UID:   "head",
			Attrs: map[string]any{document.AttrLevel: 3},
		},
		"Heading level outside the range": {
			DB:    contentDB,
			UID:   "head",
			Attrs: map[string]any{document.AttrLevel: 9},
			Err:   assert.AnError,
		},
		"Unrelated attr keeps the level already on the block": {
			DB:    contentDB,
			UID:   "head",
			Attrs: map[string]any{document.AttrIcon: "lucide:hash"},
		},
		"Unknown simulation preset": {
			DB:    contentDB,
			UID:   "metric",
			Attrs: map[string]any{document.AttrSimulationPreset: "solar_flares"},
			Err:   assert.AnError,
		},
		"Known simulation preset": {
			DB:    contentDB,
			UID:   "metric",
			Attrs: map[string]any{document.AttrSimulationPreset: "error_rate"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameUpdateBlockAttrs, `{}`)

			err := inp.ValidateAttrUpdate(_testDocID, c.UID, c.Attrs)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_input_ValidateMove(t *testing.T) {
	t.Parallel()

	// two lists, so a wrapper item has both a same-kind destination and
	// a root one to be tested against, plus a split_doc whose right side
	// holds the one block type that may not sit at the root.
	contentDB := contentWithBlocks(
		document.Block{
			Type:  document.BlockNodeParagraph,
			Attrs: document.Attributes{document.AttrUID: "root-p"},
		},
		document.Block{
			Type:  document.BlockNodeBulletList,
			Attrs: document.Attributes{document.AttrUID: "list-a"},
			Content: []document.Block{
				{
					Type:  document.BlockNodeListItem,
					Attrs: document.Attributes{document.AttrUID: "item-a1"},
				},
				{
					Type:  document.BlockNodeListItem,
					Attrs: document.Attributes{document.AttrUID: "item-a2"},
				},
			},
		},
		document.Block{
			Type:  document.BlockNodeBulletList,
			Attrs: document.Attributes{document.AttrUID: "list-b"},
			Content: []document.Block{{
				Type:  document.BlockNodeListItem,
				Attrs: document.Attributes{document.AttrUID: "item-b1"},
			}},
		},
		document.Block{
			Type:  document.BlockNodeSplitDoc,
			Attrs: document.Attributes{document.AttrUID: "split"},
			Content: []document.Block{{
				Type: document.BlockNodeSplitDocRight,
				Content: []document.Block{{
					Type:  document.BlockNodeTitledCodeBlock,
					Attrs: document.Attributes{document.AttrUID: "right-code"},
				}},
			}},
		},
	)

	cc := map[string]struct {
		DB  *DBMock
		UID string
		Ref string
		Err error
	}{
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			UID: "root-p",
			Ref: "list-a",
			Err: assert.AnError,
		},
		"Unresolved moved uid is left to the backend": {
			DB:  contentDB,
			UID: "missing",
			Ref: "root-p",
		},
		"Unresolved reference is left to the backend": {
			DB:  contentDB,
			UID: "root-p",
			Ref: "missing",
		},
		"Root block beside a root reference": {
			DB:  contentDB,
			UID: "root-p",
			Ref: "list-a",
		},
		"Macro internal out to the document root is refused": {
			DB:  contentDB,
			UID: "right-code",
			Ref: "root-p",
			Err: assert.AnError,
		},
		"Root block onto a split_doc right side is refused": {
			DB:  contentDB,
			UID: "root-p",
			Ref: "right-code",
			Err: assert.AnError,
		},
		"List item reordered within its own list": {
			DB:  contentDB,
			UID: "item-a1",
			Ref: "item-a2",
		},
		"List item moved into another list of the same kind": {
			DB:  contentDB,
			UID: "item-a1",
			Ref: "item-b1",
		},
		"List item out to the document root is refused": {
			DB:  contentDB,
			UID: "item-a1",
			Ref: "root-p",
			Err: assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameMoveBlock, `{}`)

			err := inp.ValidateMove(_testDocID, c.UID, c.Ref)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_input_ValidatePlacement(t *testing.T) {
	t.Parallel()

	rootBlock := block.Block{Type: block.BlockParagraph, Text: "hi"}
	macroBlock := block.Block{
		Type:  block.BlockTitledCode,
		Text:  "code",
		Attrs: map[string]any{"title": "Request"},
	}

	// a document with a reference at every container kind the tools can
	// land a block next to: the root ("root-p"), a callout item
	// ("callout-p") and a split_doc right side ("right-code").
	contentDB := &DBMock{
		FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
			return document.Content{
				Content: document.RootBlock{
					Content: []document.Block{
						{
							Type:  document.BlockNodeParagraph,
							Attrs: document.Attributes{document.AttrUID: "root-p"},
						},
						{
							Type:  document.BlockNodeCalloutBlock,
							Attrs: document.Attributes{document.AttrUID: "callout"},
							Content: []document.Block{{
								Type:  document.BlockNodeParagraph,
								Attrs: document.Attributes{document.AttrUID: "callout-p"},
							}},
						},
						{
							Type:  document.BlockNodeSplitDoc,
							Attrs: document.Attributes{document.AttrUID: "split"},
							Content: []document.Block{{
								Type: document.BlockNodeSplitDocRight,
								Content: []document.Block{{
									Type:  document.BlockNodeTitledCodeBlock,
									Attrs: document.Attributes{document.AttrUID: "right-code"},
								}},
							}},
						},
					},
				},
			}, nil
		},
	}

	cc := map[string]struct {
		DB    *DBMock
		Ref   string
		Block block.Block
		Err   error
	}{
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			Ref:   "root-p",
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Invalid block with an unresolved reference": {
			DB:    contentDB,
			Ref:   "missing",
			Block: block.Block{Type: "nonsense"},
			Err:   assert.AnError,
		},
		"Valid block with an unresolved reference is left to the backend": {
			DB:    contentDB,
			Ref:   "missing",
			Block: macroBlock,
		},
		"Invalid block beside a resolved reference": {
			DB:    contentDB,
			Ref:   "root-p",
			Block: block.Block{Type: "nonsense"},
			Err:   assert.AnError,
		},
		"Root block beside a root reference is allowed": {
			DB:    contentDB,
			Ref:   "root-p",
			Block: rootBlock,
		},
		"Macro internal beside a root reference is refused": {
			DB:    contentDB,
			Ref:   "root-p",
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Root block inside a callout is allowed": {
			DB:    contentDB,
			Ref:   "callout-p",
			Block: rootBlock,
		},
		"Macro internal inside a callout is refused": {
			DB:    contentDB,
			Ref:   "callout-p",
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Macro internal on a split_doc right side is allowed": {
			DB:    contentDB,
			Ref:   "right-code",
			Block: macroBlock,
		},
		"Root block on a split_doc right side is refused": {
			DB:    contentDB,
			Ref:   "right-code",
			Block: rootBlock,
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameInsertBlock, `{}`)

			err := inp.ValidatePlacement(_testDocID, c.Ref, c.Block)
			testutil.AssertEqualError(t, c.Err, err)
		})
	}
}

func Test_input_NotifyTreeChange(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(xid.New())

	// a session without a notifier silently no-ops.
	testInput(testDeps(nil, nil, nil), NameCreateDocument, `{}`).NotifyTreeChange(parentID)

	tree := &TreeNotifierMock{}
	testInput(testDeps(nil, nil, tree), NameCreateDocument, `{}`).NotifyTreeChange(parentID)

	ff := tree.NotifyTreeChangeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, parentID, ff[0].ParentID)
}

func Test_input_NotifyTreeChangeForDocument(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Notify int
	}{
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
		},
		"Parent is announced": {DB: stubDocumentDB(), Notify: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			testInput(testDeps(c.DB, nil, tree), NameUpdateDocument, `{}`).
				NotifyTreeChangeForDocument(_testDocID)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notify)
		})
	}
}

func Test_input_Warn(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	d := testDeps(nil, nil, nil)
	d.log = slog.New(slog.NewTextHandler(&buf, nil))

	testInput(d, NameSearchDocuments, `{}`).Warn("something degraded", slog.String("key", "value"))

	assert.Contains(t, buf.String(), "something degraded")
	assert.Contains(t, buf.String(), "search_documents")
	assert.Contains(t, buf.String(), "value")
}

func Test_input_CheckDataSources(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	inp := testInput(d, NameInsertBlock, "")

	require.NoError(t, inp.CheckDataSources(nil))
	require.NoError(t, inp.CheckDataSources([]string{_testDataSourceID.String()}))

	// an id that is not an xid and one the organisation owns nothing for
	// are both refused, named by the attribute they arrived in.
	for _, id := range []string{"wibble", xid.New().String()} {
		err := inp.CheckDataSources([]string{_testDataSourceID.String(), id})
		require.Error(t, err, "id %q should be refused", id)
		assert.Contains(t, err.Error(), "dataSourceId")
	}
}

func Test_input_DataSource(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	inp := testInput(d, NameQueryPrometheus, "")

	ds, err := inp.DataSource(_testDataSourceID)
	require.NoError(t, err)
	require.NotNil(t, ds)
	assert.Equal(t, "prod", ds.Name)

	// an id the organisation owns nothing for comes back as a failure
	// rather than as a zero data source.
	_, err = inp.DataSource(xid.New())
	require.Error(t, err)
}

func Test_input_DataSources(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := testInput(d, NameListDataSources, "").DataSources()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "prod", got[0].Name)
}

func Test_input_DataSourceRunner(t *testing.T) {
	t.Parallel()

	runner := &datasourceMock.Runner{}
	inp := testInput(dataSourceDeps(t, datasource.TypePrometheus, runner), NameQueryPrometheus, "")

	got, err := inp.DataSourceRunner(_testDataSourceID)
	require.NoError(t, err)
	assert.Same(t, runner, got)

	// an id the organisation owns nothing for is as absent as one that
	// names nothing at all, and says so in terms the model can act on.
	_, err = inp.DataSourceRunner(xid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list_data_sources")
}

func Test_input_DocumentBlock(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		UID    string
		Result string
		Err    error
	}{
		"Error returned by db.FetchMainBranchContent": {
			DB:  stubContentDB(assert.AnError),
			UID: _stubContentUID,
			Err: assert.AnError,
		},
		"Uid the document does not hold": {
			DB:  stubContentDB(nil),
			UID: "zzz",
			Err: fmt.Errorf("block zzz: %w", errUnknownBlock),
		},
		"Block found": {
			DB:     stubContentDB(nil),
			UID:    _stubContentUID,
			Result: "hello",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameUpdateBlockText, `{}`)

			b, err := inp.DocumentBlock(_testDocID, c.UID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.Text)
		})
	}
}
