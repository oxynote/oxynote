package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
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
var _testDocID = xid.New().String()

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
		log:     discardLog(),
		db:      db,
		search:  &SearcherMock{},
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
				Branch: document.Branch{
					BranchID:     branchID,
					DocumentName: _stubDocumentName,
				},
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
	}
}

// stubApplier accepts every edit it is handed.
func stubApplier() *EditApplierMock {
	return &EditApplierMock{
		ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
			return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
		},
	}
}

func Test_NewDeps(t *testing.T) {
	t.Parallel()

	var (
		db       = &DBMock{}
		searcher = &SearcherMock{}
		applier  = &EditApplierMock{}
		tree     = &TreeNotifierMock{}
		offload  = &offloadReaderMock{}
	)

	d := NewDeps(discardLog(), db, searcher, applier, tree, offload, "org", "user")
	require.NotNil(t, d)

	assert.NotNil(t, d.log)
	assert.Same(t, db, d.db)
	assert.Same(t, searcher, d.search)
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

	var out struct {
		Name string `json:"name"`
	}

	// success
	require.NoError(t, testInput(testDeps(nil, nil, nil), NameCreateDocument, `{"name":"Runbook"}`).Decode(&out))
	assert.Equal(t, "Runbook", out.Name)

	// error: the message names the tool that rejected the arguments
	err := testInput(testDeps(nil, nil, nil), NameCreateDocument, `{`).Decode(&out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create_document: invalid input")
}

func Test_input_Probe(t *testing.T) {
	t.Parallel()

	var out struct {
		Name string `json:"name"`
	}

	// success
	testInput(testDeps(nil, nil, nil), NameCreateDocument, `{"name":"Runbook"}`).Probe(&out)
	assert.Equal(t, "Runbook", out.Name)

	// a malformed payload degrades the description rather than aborting
	// it, so the target keeps its zero value and nothing is returned.
	out.Name = ""

	testInput(testDeps(nil, nil, nil), NameCreateDocument, `{`).Probe(&out)
	assert.Empty(t, out.Name)
}

func Test_input_DocumentID(t *testing.T) {
	t.Parallel()

	docID := xid.New().String()

	cc := map[string]struct {
		Args   string
		Result string
	}{
		"Arguments name a document": {Args: `{"document_id":"` + docID + `"}`, Result: docID},
		"Arguments name none":       {Args: `{}`},
		"Malformed arguments":       {Args: `{`},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, testInput(testDeps(nil, nil, nil), NameGetDocument, c.Args).DocumentID())
		})
	}
}

func Test_input_Subject(t *testing.T) {
	t.Parallel()

	docID := xid.New().String()

	cc := map[string]struct {
		DB     *DBMock
		Args   string
		Result string
	}{
		"Named document": {
			DB:     stubDocumentDB(),
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "Runbook",
		},
		"Arguments name no document": {
			DB:     stubDocumentDB(),
			Args:   `{}`,
			Result: "document",
		},
		"Unresolvable document falls back": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Args:   `{"document_id":"` + docID + `"}`,
			Result: "document",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, c.Result, testInput(testDeps(c.DB, nil, nil), NameGetDocument, c.Args).Subject())
		})
	}
}

func Test_input_DocumentName(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		ID     string
		Result string
	}{
		"Resolved": {DB: stubDocumentDB(), ID: xid.New().String(), Result: "Runbook"},
		"Not a valid xid": {
			DB: stubDocumentDB(),
			ID: "nope",
		},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			ID: xid.New().String(),
		},
		"Document is absent": {
			DB: &DBMock{
				//nolint:nilnil // the case under test is a store with no value and no error
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, nil
				},
			},
			ID: xid.New().String(),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got := testInput(testDeps(c.DB, nil, nil), NameGetDocument, `{}`).DocumentName(c.ID)
			assert.Equal(t, c.Result, got)
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

			err := inp.CreateDocument(document.Document{ParentID: c.Parent})
			testutil.AssertEqualError(t, c.Err, err)

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
			Err:    errors.New("a document cannot be moved under itself or one of its descendants"),
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

			inp := testInput(testDeps(c.DB, nil, nil), NameMoveDocument, `{}`)

			err := inp.MoveDocument(docID, c.Parent)
			testutil.AssertEqualError(t, c.Err, err)

			// nothing is written until the destination is known good.
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.Checks)
			assert.Len(t, c.DB.UpdateDocumentParentIDCalls(), c.Updates)
		})
	}
}

func Test_input_ApplyEdit(t *testing.T) {
	t.Parallel()

	docID := xid.New().String()

	cc := map[string]struct {
		DB      *DBMock
		Applier *EditApplierMock
		DocID   string
		Result  string
		Err     error
	}{
		"Document cannot be resolved": {
			DB:      &DBMock{},
			Applier: stubApplier(),
			DocID:   "not-an-xid",
			Err:     assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Applier: stubApplier(),
			DocID:   docID,
			Err:     assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, string, string, []edit.Operation) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			DocID: docID,
			Err:   assert.AnError,
		},
		"Partial failure still reports the outcome": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, string, string, []edit.Operation) (edit.Result, error) {
					return edit.Result{
						Applied: 1,
						Errors:  []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			DocID:  docID,
			Result: `{"applied":1,"errors":[{"index":1,"message":"uid not found"}]}`,
		},
		"Applied": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			DocID:   docID,
			Result:  `{"applied":1,"errors":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, c.Applier, nil), NameAppendBlock, `{}`)

			res, err := inp.ApplyEdit(c.DocID, []edit.Operation{edit.Delete("a")})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.JSONEq(t, c.Result, res)
		})
	}
}

func Test_input_ValidatePlacement(t *testing.T) {
	t.Parallel()

	docID := xid.New().String()

	rootBlock := block.Block{Type: block.BlockParagraph, Text: "hi"}
	macroBlock := block.Block{
		Type:  block.BlockTitledCode,
		Text:  "code",
		Attrs: map[string]any{"title": "Request"},
	}

	contentDB := func(uid string) *DBMock {
		return &DBMock{
			FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
				return document.Content{
					Content: document.RootBlock{
						Content: []document.Block{{
							Type:  document.BlockNodeParagraph,
							Attrs: document.Attributes{document.AttrUID: uid},
						}},
					},
				}, nil
			},
		}
	}

	cc := map[string]struct {
		DB    *DBMock
		DocID string
		Block block.Block
		Err   error
	}{
		"Invalid block": {
			DB:    &DBMock{},
			DocID: docID,
			Block: block.Block{Type: "nonsense"},
			Err:   assert.AnError,
		},
		"Root-legal block needs no lookup": {
			DB:    &DBMock{},
			DocID: docID,
			Block: rootBlock,
		},
		"Macro internal with an unparseable document id": {
			DB:    &DBMock{},
			DocID: "not-an-xid",
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			DB: &DBMock{
				FetchMainBranchContentFunc: func(context.Context, xid.ID, string) (document.Content, error) {
					return document.Content{}, assert.AnError
				},
			},
			DocID: docID,
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Macro internal beside a root block is refused": {
			DB:    contentDB("ref"),
			DocID: docID,
			Block: macroBlock,
			Err:   assert.AnError,
		},
		"Macro internal nested inside a macro is allowed": {
			// the reference uid is not a root child, so the block is
			// landing inside a container that accepts it.
			DB:    contentDB("other"),
			DocID: docID,
			Block: macroBlock,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameInsertBlock, `{}`)

			err := inp.ValidatePlacement(c.DocID, "ref", c.Block)
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
		DocID  string
		Notify int
	}{
		"Not a valid xid": {DB: stubDocumentDB(), DocID: "nope"},
		"Error returned by db.FetchDocument": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			DocID: xid.New().String(),
		},
		"Parent is announced": {DB: stubDocumentDB(), DocID: xid.New().String(), Notify: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}

			testInput(testDeps(c.DB, nil, tree), NameRenameDocument, `{}`).
				NotifyTreeChangeForDocument(c.DocID)

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
