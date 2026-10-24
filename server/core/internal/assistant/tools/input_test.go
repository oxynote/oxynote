package tools

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/internal/variable"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m, testutil.IgnoreBleveWorkers())
}

// _testDocID is the document id the tool tests address.
var _testDocID = xid.New()

// _testTagID is the tag id the tag tool tests address; _otherTagID is
// the second tag every tag stub lists, and _unknownTagID names none.
var (
	_testTagID    = xid.New()
	_otherTagID   = xid.New()
	_unknownTagID = xid.New()
)

// _stubTagName and _stubTagColor are what the stubbed tag lookups answer
// with for _testTagID.
const (
	_stubTagName  = "Production"
	_stubTagColor = "green"
)

// testDeps builds session wiring with don't-care dependencies. Pass the
// mocks a test cares about; nil leaves a don't-care in place.
func testDeps(db *DBMock, applier *EditApplierMock, tree *TreeNotifierMock) *Deps {
	if db == nil {
		db = &DBMock{}
	}

	if applier == nil {
		applier = &EditApplierMock{}
	}

	// a deployment without the GitHub app refuses every call that would
	// reach GitHub.
	gh, err := github.NewManager(nil, github.Options{})
	if err != nil {
		panic(err)
	}

	d := &Deps{
		log:         slog.New(slog.DiscardHandler),
		db:          db,
		search:      &SearcherMock{},
		searchQueue: &SearchQueueMock{},
		runners: &DataSourceRunnersMock{
			RunnerFunc: func(datasource.DataSource) datasource.Runner {
				return &datasourceMock.Runner{}
			},
		},
		githubMan:       gh,
		webchangeClient: webchange.NewClient("", ""),
		hookMan:         stubHookManager(),
		variableMan:     stubVariableManager(),
		applier:         applier,
		offload:         &offloadReaderMock{},
		orgID:           "org",
		userID:          "user",
	}

	// a nil notifier is a real case — NotifyTreeChange no-ops — so it is
	// only wired when a test asks for it.
	if tree != nil {
		d.tree = tree
	}

	return d
}

// requiredArgs builds the smallest payload the named tool's Decode
// accepts: every required argument, each with a value its type takes.
func requiredArgs(t *testing.T, name Name) string {
	t.Helper()

	e, ok := New(testDeps(nil, nil, nil)).Entry(name)
	require.True(t, ok, "%s is not in the registry", name)

	vals := map[string]any{}

	for _, key := range e.Info.Required {
		switch key {
		case "document_id", "data_source_id":
			vals[key] = _testDocID.String()
		case "tag_id":
			vals[key] = _testTagID.String()
		case "hook_id":
			vals[key] = _testHookID.String()
		case "group_id":
			vals[key] = _testVariableGroupID.String()
		case "variable_id":
			vals[key] = _testVariableID.String()
		case "type":
			vals[key] = string(variable.TypeText)
		case "settings":
			// the stubbed hook is a scheduled reminder.
			vals[key] = map[string]any{"type": string(hook.TypeScheduledReminder), "schedule": _stubSchedule}
		case "color":
			vals[key] = "green"
		case "sort_index":
			vals[key] = 0
		case "assigned":
			vals[key] = true
		case "content":
			vals[key] = "<p>x</p>"
		case "position":
			// a position beside a block needs the reference the tool
			// requires; one that requires no reference takes an end.
			vals[key] = string(positionEnd)
			if slices.Contains(e.Info.Required, "reference_block_uid") {
				vals[key] = string(positionAfter)
			}
		case "branch_id":
			vals[key] = _stubMainBranchID.String()
		case "block_uid":
			// a block the stubbed document holds, since a replace checks
			// its markup against it before anything is shown.
			vals[key] = "a"
		case "reference_block_uid":
			// distinct from the "x" other uid keys take, so a payload
			// naming both a block and a reference is not a self-move.
			vals[key] = "r"
		case "matchers":
			vals[key] = []string{"up"}
		default:
			vals[key] = "x"
		}
	}

	// update_document, update_tag and update_variable_group require
	// nothing beyond their target, but refuse a call that changes
	// nothing; a name is the smallest change.
	if name == NameUpdateDocument || name == NameUpdateTag || name == NameUpdateVariableGroup {
		vals["name"] = "x"
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
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, branchName string) (*document.Document, error) {
			return stubBranchDocument(id, orgID, _stubMainBranchID, branchName), nil
		},
		// the stub has the two branches its listing names and no other,
		// and the draft belongs to the test document alone.
		FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
			switch branchID {
			case _stubMainBranchID:
				return stubBranchDocument(_testDocID, orgID, branchID, document.DefaultBranch), nil
			case _stubBranchID:
				return stubBranchDocument(_testDocID, orgID, branchID, _stubBranchName), nil
			default:
				return nil, errutil.ErrNotFound
			}
		},
		FetchDocumentBranchesFunc: stubBranches(false, false),
		// the tag tree holds two tags: the test tag, carried by the test
		// document, and a hidden one carried by nothing.
		FetchTagTreeFunc: func(context.Context, string, string) (tag.Summaries, error) {
			return tag.Summaries{
				{
					ID:      _testTagID,
					TagName: _stubTagName,
					Color:   _stubTagColor,
					Documents: document.Summaries{{
						ID:              _testDocID,
						DocumentName:    _stubDocumentName,
						DefaultBranchID: _stubMainBranchID,
						Children:        document.Summaries{{ID: xid.New(), DocumentName: "Nested"}},
					}},
				},
				{
					ID:        _otherTagID,
					TagName:   "Staging",
					Color:     "orange",
					Hidden:    true,
					Documents: document.Summaries{},
				},
			}, nil
		},
		FetchDocumentHookFunc: func(_ context.Context, id xid.ID, _ string) (*hook.Hook, error) {
			if id != _testHookID {
				return nil, errutil.ErrNotFound
			}

			return stubHook(), nil
		},
	}
}

// stubHook builds the hook every hook stub answers with.
func stubHook() *hook.Hook {
	return &hook.Hook{
		ID:             _testHookID,
		Type:           hook.TypeScheduledReminder,
		DocumentID:     null.ValueFrom(_testDocID),
		OrganizationID: null.StringFrom("org"),
		BranchID:       null.ValueFrom(_stubBranchID),
		Settings:       processor.Settings(`{"scale":"linear","duration":"custom","schedule":"2030-01-01T00:00:00Z"}`),
		State:          null.ValueFrom(processor.State(`{"startedAt":"2026-01-01T00:00:00Z"}`)),
		Status:         processor.StatusActive,
		Score:          decimal.NewFromInt(100),
		CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// _testHookID is the hook id the hook tool tests address; _unknownHookID
// names none.
var (
	_testHookID    = xid.New()
	_unknownHookID = xid.New()
)

// stubBranchDocument builds the document the stubs answer with, on the
// named branch.
func stubBranchDocument(id xid.ID, orgID string, branchID xid.ID, branchName string) *document.Document {
	return &document.Document{
		BranchID:       branchID,
		BranchName:     branchName,
		DocumentName:   _stubDocumentName,
		Default:        branchName == document.DefaultBranch,
		ID:             id,
		OrganizationID: orgID,
	}
}

// stubBranches answers a branch listing with the default branch and one
// named draft, each protected when told so.
func stubBranches(mainProtected, draftProtected bool) func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
	return func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
		return []document.BranchSummary{
			{BranchID: _stubMainBranchID, BranchName: document.DefaultBranch, Default: true, Protected: mainProtected},
			{BranchID: _stubBranchID, BranchName: _stubBranchName, Protected: draftProtected},
		}, nil
	}
}

// _stubSchedule is the schedule every scheduled-reminder argument stub
// carries, far enough ahead that a reset scores it fresh.
const _stubSchedule = "2030-01-01T00:00:00Z"

// _stubBranchName is the non-default branch every branch stub lists.
const _stubBranchName = "draft"

// _stubMainBranchID and _stubBranchID are the ids of the two branches
// every branch stub lists; _unknownBranchID names no branch of any
// document.
var (
	_stubMainBranchID = xid.New()
	_stubBranchID     = xid.New()
	_unknownBranchID  = xid.New()
)

// protectedDocumentDB answers with a document whose branch is
// protected, which is the one branch a tool's write may not reach.
func protectedDocumentDB() *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, branchName string) (*document.Document, error) {
			doc := stubBranchDocument(id, orgID, _stubMainBranchID, branchName)
			doc.Protected = true

			return doc, nil
		},
		FetchDocumentByBranchIDFunc: func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
			doc := stubBranchDocument(_testDocID, orgID, branchID, document.DefaultBranch)
			doc.Protected = true

			return doc, nil
		},
		FetchDocumentBranchesFunc: stubBranches(true, false),
	}
}

// failingDocumentDB refuses every document lookup.
func failingDocumentDB() *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
			return nil, assert.AnError
		},
		FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
			return nil, assert.AnError
		},
	}
}

// stubApplier accepts every edit it is handed.
func stubApplier() *EditApplierMock {
	return &EditApplierMock{
		ApplyFunc: func(_ context.Context, _, _ xid.ID, _ []edit.Operation, _ string, _ bool) (edit.Result, error) {
			return edit.Result{Errors: []edit.OpError{}}, nil
		},
	}
}

func Test_NewDeps(t *testing.T) {
	t.Parallel()

	gh, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	var (
		db       = &DBMock{}
		searcher = &SearcherMock{}
		runner   = &datasourceMock.Runner{}
		applier  = &EditApplierMock{}
		tree     = &TreeNotifierMock{}
		tags     = &TagNotifierMock{}
		offload  = &offloadReaderMock{}
		wc       = webchange.NewClient("", "")
		hookMan  = &HookManagerMock{}
		varMan   = &VariableManagerMock{}
	)

	runners := &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	queue := &SearchQueueMock{}

	d := NewDeps(slog.New(slog.DiscardHandler), db, searcher, queue, runners, gh, wc, hookMan, varMan, applier, tree, tags, offload, "org", "user")
	require.NotNil(t, d)

	assert.NotNil(t, d.log)
	assert.Same(t, db, d.db)
	assert.Same(t, searcher, d.search)
	assert.Same(t, queue, d.searchQueue)
	assert.Same(t, runners, d.runners)
	assert.Same(t, gh, d.githubMan)
	assert.Same(t, wc, d.webchangeClient)
	assert.Same(t, hookMan, d.hookMan)
	assert.Same(t, varMan, d.variableMan)
	assert.Same(t, applier, d.applier)
	assert.Same(t, tree, d.tree)
	assert.Same(t, tags, d.tags)
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
	assert.Equal(t, "org", i.orgID)
	assert.Equal(t, "user", i.userID)
}

func Test_input_Context(t *testing.T) {
	t.Parallel()

	type callKey struct{}

	ctx := context.WithValue(context.Background(), callKey{}, "call")

	assert.Same(t, ctx, (&input{ctx: ctx}).Context())
}

func Test_input_Decode(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Args string
		Want deleteBlockArgs

		// ErrContains is a fragment the error has to carry, for an error
		// whose full wording json/v2 does not fix.
		ErrContains string

		Err error
	}{
		"Malformed JSON": {Args: `{`, ErrContains: "invalid input:", Err: assert.AnError},
		// a provider calling a parameterless tool sends nothing rather
		// than an empty object; that is a payload, not a decode failure.
		"Empty arguments read as none": {Args: ``, Err: errors.New("document_id is required")},
		"Blank arguments read as none": {Args: " \n", Err: errors.New("document_id is required")},
		"Invalid id names the argument": {
			Args: `{"document_id":"nope","block_uid":"b"}`,
			Err:  errors.New("document_id is not an id; copy all 20 characters exactly from the result that gave it"),
		},
		"Null id is not an argument": {
			Args: `{"document_id":null,"block_uid":"b"}`,
			Err:  errors.New("document_id is required"),
		},
		"Incomplete arguments are rejected by Validate": {
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  errors.New("block_uid is required"),
		},
		"Unknown keys are ignored": {
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b","extra":true}`,
			Want: deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
		},
		"Decoded": {
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b"}`,
			Want: deleteBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var out deleteBlockArgs

			err := testDeps(nil, nil, nil).newInput(context.Background(), NameDeleteBlock, json.RawMessage(c.Args)).Decode(&out)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Contains(t, err.Error(), c.ErrContains)

				return
			}

			assert.Equal(t, c.Want, out)
		})
	}
}

func Test_input_CreateDocument(t *testing.T) {
	t.Parallel()

	type tcase struct {
		DB       *DBMock
		Tx       *TxMock
		Queue    *SearchQueueMock
		Parent   null.Value[xid.ID]
		Exists   int
		Begins   int
		Inserts  int
		Upserts  int
		Commits  int
		Jobs     int
		Notifies int
		Err      error
	}

	stubQueue := func(enqueueErr error) *SearchQueueMock {
		return &SearchQueueMock{
			EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
				return enqueueErr
			},
		}
	}

	stubTx := func(insertErr, upsertErr, commitErr error) *TxMock {
		return &TxMock{
			InsertDocumentFunc: func(context.Context, document.Document) error {
				return insertErr
			},
			UpsertDocumentMaintainersFunc: func(context.Context, xid.ID, string, []string) error {
				return upsertErr
			},
			CommitFunc: func() error { return commitErr },
		}
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		return &DBMock{
			BeginTxFunc: testutil.StubBeginTx(tx, beginErr),
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

			return tcase{DB: db, Queue: stubQueue(nil), Parent: null.ValueFrom(xid.New()), Exists: 1, Err: assert.AnError}
		}(),
		"Unknown parent": func() tcase {
			parentID := xid.New()
			db := stubDB(nil, nil)
			db.CheckDocumentExistsFunc = func(context.Context, xid.ID, string) error {
				return sql.ErrNoRows
			}

			return tcase{DB: db, Queue: stubQueue(nil), Parent: null.ValueFrom(parentID), Exists: 1, Err: fmt.Errorf("parent %s: %w", parentID, errUnknownParent)}
		}(),
		"Error returned by db.BeginTx": func() tcase {
			return tcase{DB: stubDB(nil, assert.AnError), Queue: stubQueue(nil), Begins: 1, Err: assert.AnError}
		}(),
		"Error returned by Tx.InsertDocument": func() tcase {
			tx := stubTx(assert.AnError, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Begins: 1, Inserts: 1, Err: assert.AnError}
		}(),
		"Error returned by Tx.UpsertDocumentMaintainers": func() tcase {
			tx := stubTx(nil, assert.AnError, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Begins: 1, Inserts: 1, Upserts: 1, Err: assert.AnError}
		}(),
		"Error returned by SearchQueue.Enqueue": func() tcase {
			tx := stubTx(nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(assert.AnError), Begins: 1, Inserts: 1, Upserts: 1, Jobs: 1, Err: assert.AnError}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := stubTx(nil, nil, assert.AnError)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Begins: 1, Inserts: 1, Upserts: 1, Commits: 1, Jobs: 1, Err: assert.AnError}
		}(),
		"Every write lands together": func() tcase {
			tx := stubTx(nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Begins: 1, Inserts: 1, Upserts: 1, Commits: 1, Jobs: 1, Notifies: 1}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			deps := testDeps(c.DB, nil, tree)
			deps.searchQueue = c.Queue
			inp := deps.newInput(context.Background(), NameCreateDocument, json.RawMessage(`{}`))

			doc := document.Document{ID: xid.New(), BranchID: xid.New(), ParentID: c.Parent}

			err := inp.CreateDocument(doc)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.Exists)
			assert.Len(t, c.DB.BeginTxCalls(), c.Begins)
			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notifies)

			ff := c.Queue.EnqueueCalls()
			require.Len(t, ff, c.Jobs)

			if c.Jobs != 0 {
				assert.Same(t, c.Tx, ff[0].Tx)
				assert.Equal(t, doc.ID, ff[0].J.DocumentID.V)
				assert.Equal(t, doc.BranchID, ff[0].J.BranchID.V)
			}

			if c.Tx == nil {
				return
			}

			assert.Len(t, c.Tx.InsertDocumentCalls(), c.Inserts)
			assert.Len(t, c.Tx.UpsertDocumentMaintainersCalls(), c.Upserts)
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
	parentID := null.ValueFrom(xid.New())

	type tcase struct {
		DB      *DBMock
		Tx      *TxMock
		Queue   *SearchQueueMock
		Commits int
		Jobs    int
		Err     error
	}

	stubQueue := func(enqueueErr error) *SearchQueueMock {
		return &SearchQueueMock{
			EnqueueFunc: func(context.Context, sqlutil.Tx, search.Job) error {
				return enqueueErr
			},
		}
	}

	stubTx := func(ids []xid.ID, deleteErr, commitErr error) *TxMock {
		return &TxMock{
			DeleteDocumentFunc: func(context.Context, xid.ID, string) ([]xid.ID, error) {
				return ids, deleteErr
			},
			CommitFunc: func() error { return commitErr },
		}
	}

	stubDB := func(tx *TxMock, beginErr error) *DBMock {
		return &DBMock{
			FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
				return &document.Document{ID: docID, ParentID: parentID}, nil
			},
			BeginTxFunc: testutil.StubBeginTx(tx, beginErr),
		}
	}

	cc := map[string]tcase{
		"Error returned by db.BeginTx": func() tcase {
			return tcase{DB: stubDB(nil, assert.AnError), Queue: stubQueue(nil), Err: assert.AnError}
		}(),
		"Error returned by Tx.DeleteDocument": func() tcase {
			tx := stubTx(nil, assert.AnError, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Err: assert.AnError}
		}(),
		"Error returned by SearchQueue.Enqueue": func() tcase {
			tx := stubTx([]xid.ID{docID}, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(assert.AnError), Jobs: 1, Err: assert.AnError}
		}(),
		"Error returned by Tx.Commit": func() tcase {
			tx := stubTx([]xid.ID{docID}, nil, assert.AnError)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Commits: 1, Jobs: 1, Err: assert.AnError}
		}(),
		// a delete that matched nothing means the id names no document
		// in this organisation — a miss, not a success.
		"Empty subtree reports an unknown document": func() tcase {
			tx := stubTx(nil, nil, nil)

			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Err: ErrUnknownDocument}
		}(),
		"Delete and search removal land together": func() tcase {
			tx := stubTx([]xid.ID{docID, xid.New()}, nil, nil)

			// one job per cascade-deleted document.
			return tcase{DB: stubDB(tx, nil), Tx: tx, Queue: stubQueue(nil), Commits: 1, Jobs: 2}
		}(),
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			deps := testDeps(c.DB, nil, tree)
			deps.searchQueue = c.Queue
			inp := deps.newInput(context.Background(), NameDeleteDocument, json.RawMessage(`{}`))

			err := inp.DeleteDocument(docID)
			testutil.AssertEqualError(t, c.Err, err)

			// only a committed delete changes the subtree it sat in.
			if err != nil {
				assert.Empty(t, tree.NotifyTreeChangeCalls())
			} else {
				require.Len(t, tree.NotifyTreeChangeCalls(), 1)
				assert.Equal(t, parentID, tree.NotifyTreeChangeCalls()[0].ParentID)
			}

			assert.Len(t, c.DB.FetchDocumentCalls(), 1)
			assert.Len(t, c.DB.BeginTxCalls(), 1)

			ff := c.Queue.EnqueueCalls()
			require.Len(t, ff, c.Jobs)

			if c.Jobs != 0 {
				assert.Same(t, c.Tx, ff[0].Tx)
				assert.True(t, ff[0].J.DocumentID.Valid)
				assert.False(t, ff[0].J.BranchID.Valid)
			}

			if c.Tx == nil {
				return
			}

			assert.Len(t, c.Tx.DeleteDocumentCalls(), 1)
			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), 1)
		})
	}
}

func Test_input_RenameDocument(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(xid.New())
	doc := &document.Document{ID: _testDocID, BranchID: _stubMainBranchID, ParentID: parentID}

	failing := &EditApplierMock{
		ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
			return edit.Result{}, assert.AnError
		},
	}

	cc := map[string]struct {
		Applier *EditApplierMock
		Name    string
		Icon    string
		Ops     int
		Fetches int
		Applies int
		Notify  int
		Err     error
	}{
		"Nothing to change": {Applier: stubApplier()},
		"Error returned by applier.Apply": {
			Applier: failing,
			Name:    "Playbook",
			Fetches: 1,
			Applies: 1,
			Err:     assert.AnError,
		},
		"Renamed": {Applier: stubApplier(), Name: "Playbook", Ops: 1, Fetches: 1, Applies: 1, Notify: 1},
		"Renamed with a new icon": {
			Applier: stubApplier(),
			Name:    "Playbook",
			Icon:    "mingcute:rocket-fill",
			Ops:     2,
			Fetches: 1,
			Applies: 1,
			Notify:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := stubDocumentDB()
			tree := &TreeNotifierMock{}
			inp := testDeps(db, c.Applier, tree).newInput(context.Background(), NameUpdateDocument, json.RawMessage(`{}`))

			err := inp.RenameDocument(doc, c.Name, c.Icon)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, db.FetchDocumentByBranchIDCalls(), c.Fetches)
			assert.Empty(t, db.FetchDocumentBranchesCalls())

			calls := c.Applier.ApplyCalls()
			require.Len(t, calls, c.Applies)

			// the document's own row shows the change, under its parent.
			ff := tree.NotifyTreeChangeCalls()
			require.Len(t, ff, c.Notify)

			if c.Notify > 0 {
				assert.Equal(t, parentID, ff[0].ParentID)
			}

			if err != nil || c.Applies == 0 {
				return
			}

			assert.Len(t, calls[0].Ops, c.Ops)
		})
	}
}

func Test_input_MoveDocument(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	branchID := xid.New()
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
		DB       *DBMock
		Parent   null.Value[xid.ID]
		Checks   int
		Cycles   int
		Updates  int
		Notifies int
		Err      error
	}{
		"Error returned by db.CheckDocumentExists": {
			DB:     moveDB(assert.AnError, false, nil, nil),
			Parent: parentID,
			Checks: 1,
			Err:    assert.AnError,
		},
		"Unknown parent": {
			DB:     moveDB(sql.ErrNoRows, false, nil, nil),
			Parent: parentID,
			Checks: 1,
			Err:    fmt.Errorf("new parent %s: %w", parentID.V, errUnknownParent),
		},
		"Error returned by db.CheckDocumentCycle": {
			DB:     moveDB(nil, false, assert.AnError, nil),
			Parent: parentID,
			Checks: 1,
			Cycles: 1,
			Err:    assert.AnError,
		},
		"A move under its own subtree is refused": {
			DB:     moveDB(nil, true, nil, nil),
			Parent: parentID,
			Checks: 1,
			Cycles: 1,
			Err:    errCyclicParent,
		},
		"Error returned by db.UpdateDocumentParentID": {
			DB:      moveDB(nil, false, nil, assert.AnError),
			Parent:  parentID,
			Checks:  1,
			Cycles:  1,
			Updates: 1,
			Err:     assert.AnError,
		},
		// the old and the new subtree both change shape.
		"Moved under a parent": {
			DB:       moveDB(nil, false, nil, nil),
			Parent:   parentID,
			Checks:   1,
			Cycles:   1,
			Updates:  1,
			Notifies: 2,
		},
		// the document already sat at the root, so one subtree changes.
		"A move to the root has no destination to check": {
			DB:       moveDB(nil, false, nil, nil),
			Updates:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			inp := testDeps(c.DB, nil, tree).newInput(context.Background(), NameUpdateDocument, json.RawMessage(`{}`))

			err := inp.MoveDocument(&document.Document{ID: docID, BranchID: branchID}, c.Parent)
			testutil.AssertEqualError(t, c.Err, err)

			// nothing is written until the destination is known good.
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.Checks)
			assert.Len(t, c.DB.CheckDocumentCycleCalls(), c.Cycles)
			assert.Len(t, c.DB.UpdateDocumentParentIDCalls(), c.Updates)
			assert.Len(t, tree.NotifyTreeChangeCalls(), c.Notifies)
		})
	}
}

func Test_input_ApplyEdit(t *testing.T) {
	t.Parallel()

	docID := _testDocID

	type tcase struct {
		DB       *DBMock
		Applier  *EditApplierMock
		Branch   xid.ID
		Listings int
		Applies  int

		// Ops is how many operations the batch holds; 0 means one.
		Ops int
		Err error
	}

	cc := map[string]tcase{
		"Error returned by db.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Applier: stubApplier(),
			Err:     assert.AnError,
		},
		"Error returned by applier.Apply": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			Applies: 1,
			Err:     assert.AnError,
		},
		"Protected branch is refused before anything is applied": {
			DB:       protectedDocumentDB(),
			Applier:  stubApplier(),
			Listings: 1,
			Err:      fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		"Protected branch with no alternative says so": func() tcase {
			db := protectedDocumentDB()
			db.FetchDocumentBranchesFunc = stubBranches(true, true)

			return tcase{
				DB:       db,
				Applier:  stubApplier(),
				Listings: 1,
				Err:      fmt.Errorf("branch main: %w; the document has no unprotected branch to write to", errBranchProtected),
			}
		}(),
		"Applied to a named branch": func() tcase {
			// the branch has to belong to the document being edited.
			db := stubDocumentDB()
			db.FetchDocumentByBranchIDFunc = func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
				return stubBranchDocument(docID, orgID, branchID, _stubBranchName), nil
			}

			return tcase{
				DB:      db,
				Applier: stubApplier(),
				Branch:  _stubBranchID,
				Applies: 1,
			}
		}(),
		"Nothing applied is a failure rather than a result": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
					return edit.Result{
						Errors: []edit.OpError{{Index: 0, Message: "block_uid not found: a"}},
					}, nil
				},
			},
			Applies: 1,
			Err:     fmt.Errorf("applying edit: %w", errors.New("no block with uid a in this document; call get_document for the current uids")),
		},
		"Failed batch names the operation that failed": {
			DB: stubDocumentDB(),
			Applier: &EditApplierMock{
				ApplyFunc: func(context.Context, xid.ID, xid.ID, []edit.Operation, string, bool) (edit.Result, error) {
					return edit.Result{
						Errors: []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			Ops:     2,
			Applies: 1,
			Err:     fmt.Errorf("applying edit: %w", errors.New("nothing was applied; operation 2: uid not found")),
		},
		"Applied": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Applies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testDeps(c.DB, c.Applier, nil).newInput(context.Background(), NameInsertBlocks, json.RawMessage(`{}`))

			branch := c.Branch
			if branch.IsNil() {
				branch = _stubMainBranchID
			}

			ops := make([]edit.Operation, 0, max(c.Ops, 1))
			ops = append(ops, edit.Delete("a"))

			for range c.Ops - 1 {
				ops = append(ops, edit.Delete("b"))
			}

			err := inp.ApplyEdit(docID, branch, ops)
			testutil.AssertEqualError(t, c.Err, err)

			// an edit that never landed has changed nothing to report.

			// a refusal the tools own — a protected branch — stops
			// before the batch is shipped, rather than letting it land
			// and be undone by the persist behind it.
			assert.Len(t, c.Applier.ApplyCalls(), c.Applies)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)

			if err != nil {
				return
			}

			// a tool's write is a person's, never core's own: an
			// edit asking otherwise would land on a protected
			// branch, which is the one place a tool may not reach.
			// The persist credits the user the assistant acts for.
			if c.Applier != nil {
				for _, call := range c.Applier.ApplyCalls() {
					assert.False(t, call.System)
					assert.Equal(t, inp.userID, call.UserID)
				}
			}
		})
	}
}

func Test_input_notifyTreeChange(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(xid.New())

	cc := map[string]struct {
		Tree *TreeNotifierMock
	}{
		// a session without a notifier silently no-ops.
		"No notifier":         {},
		"Notifier configured": {Tree: &TreeNotifierMock{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testDeps(nil, nil, c.Tree).newInput(context.Background(), NameCreateDocument, json.RawMessage(`{}`)).notifyTreeChange(parentID)

			if c.Tree == nil {
				return
			}

			ff := c.Tree.NotifyTreeChangeCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, parentID, ff[0].ParentID)
		})
	}
}

func Test_input_Warn(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	d := testDeps(nil, nil, nil)
	d.log = slog.New(slog.NewTextHandler(&buf, nil))

	d.newInput(context.Background(), NameSearchDocuments, json.RawMessage(`{}`)).Warn("something degraded", slog.String("key", "value"))

	assert.Contains(t, buf.String(), "something degraded")
	assert.Contains(t, buf.String(), "search_documents")
	assert.Contains(t, buf.String(), "value")
}

func Test_input_CheckDataSources(t *testing.T) {
	t.Parallel()

	metric := func(id string) document.Block {
		return document.Block{Type: document.BlockNodeMetricBlock, Attrs: document.Attributes{document.AttrDataSourceID: id}}
	}

	grid := func(blocks ...document.Block) []document.Block {
		return []document.Block{{Type: document.BlockNodeMetricGrid, Content: blocks}}
	}

	unknown := xid.New().String()

	cc := map[string]struct {
		Blocks  []document.Block
		Fetches int
		Err     error
	}{
		"No blocks": {},
		"Owned data source nested in a grid": {
			Blocks:  grid(metric(_testDataSourceID.String())),
			Fetches: 1,
		},
		"Empty id is skipped": {
			Blocks: grid(metric("")),
		},
		"Id that is not an xid": {
			Blocks:  grid(metric(_testDataSourceID.String()), metric("wibble")),
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Id the organisation owns nothing for": {
			Blocks:  grid(metric(_testDataSourceID.String()), metric(unknown)),
			Fetches: 2,
			Err:     fmt.Errorf("metric dataSourceId %q: %w", unknown, errUnknownDataSource),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := dataSourceDeps(t, datasource.TypePrometheus, nil)
			inp := d.newInput(context.Background(), NameInsertBlocks, json.RawMessage(""))

			testutil.AssertEqualError(t, c.Err, inp.CheckDataSources(c.Blocks))

			assert.Len(t, d.db.(*DBMock).FetchDataSourceCalls(), c.Fetches)
		})
	}
}

func Test_input_FetchDataSource(t *testing.T) {
	t.Parallel()

	failing := &DBMock{
		FetchDataSourceFunc: func(context.Context, xid.ID, string) (*datasource.DataSource, error) {
			return nil, assert.AnError
		},
	}

	cc := map[string]struct {
		DB  *DBMock
		ID  xid.ID
		Err error
	}{
		"Error returned by db.FetchDataSource": {
			DB:  failing,
			ID:  _testDataSourceID,
			Err: fmt.Errorf("fetching data source: %w", assert.AnError),
		},
		// an id the organisation owns nothing for comes back as a
		// failure rather than as a zero data source.
		"Unknown data source": {
			ID:  xid.New(),
			Err: errUnknownDataSource,
		},
		"Successful fetch": {
			ID: _testDataSourceID,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := dataSourceDeps(t, datasource.TypePrometheus, nil)
			if c.DB != nil {
				d.db = c.DB
			}

			ds, err := d.newInput(context.Background(), NameQueryDataSource, json.RawMessage("")).FetchDataSource(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, d.db.(*DBMock).FetchDataSourceCalls(), 1)

			if err != nil {
				return
			}

			require.NotNil(t, ds)
			assert.Equal(t, "prod", ds.Name)
		})
	}
}

func Test_input_FetchDataSources(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := d.newInput(context.Background(), NameListDataSources, json.RawMessage("")).FetchDataSources()
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "prod", got[0].Name)
	assert.Len(t, d.db.(*DBMock).FetchDataSourcesCalls(), 1)
}

func Test_input_DataSourceRunner(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		ID      xid.ID
		Runners int
		Err     error
	}{
		// an id the organisation owns nothing for is as absent as one that
		// names nothing at all, and says so in terms the model can act on.
		"Unknown data source": {
			ID:  xid.New(),
			Err: errUnknownDataSource,
		},
		"Successful lookup": {
			ID:      _testDataSourceID,
			Runners: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			runner := &datasourceMock.Runner{}
			d := dataSourceDeps(t, datasource.TypePrometheus, runner)

			got, err := d.newInput(context.Background(), NameQueryDataSource, json.RawMessage("")).DataSourceRunner(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, d.db.(*DBMock).FetchDataSourceCalls(), 1)
			assert.Len(t, d.runners.(*DataSourceRunnersMock).RunnerCalls(), c.Runners)

			if err != nil {
				assert.Contains(t, err.Error(), "list_data_sources")
				return
			}

			assert.Same(t, runner, got)
		})
	}
}

func Test_input_FetchDocumentBlock(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Branch xid.ID
		UID    string
		Result string
		Err    error
	}{
		"Error returned by db.FetchDocumentByBranchID": {
			DB:     stubContentDB(assert.AnError),
			Branch: _stubMainBranchID,
			UID:    _stubContentUID,
			Err:    assert.AnError,
		},
		"Block found on another branch": {
			DB:     stubContentDB(nil),
			Branch: _stubBranchID,
			UID:    _stubContentUID,
			Result: "hello",
		},
		"Uid the document does not hold": {
			DB:     stubContentDB(nil),
			Branch: _stubMainBranchID,
			UID:    "zzz",
			Err:    fmt.Errorf("block zzz: %w", errUnknownBlock),
		},
		"Block found": {
			DB:     stubContentDB(nil),
			Branch: _stubMainBranchID,
			UID:    _stubContentUID,
			Result: "hello",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testDeps(c.DB, nil, nil).newInput(context.Background(), NameReplaceBlocks, json.RawMessage(`{}`))

			b, err := inp.FetchDocumentBlock(_testDocID, c.Branch, c.UID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Empty(t, c.DB.FetchDocumentBranchesCalls())

			if err != nil {
				return
			}

			require.NotEmpty(t, b.Content)
			assert.Equal(t, c.Result, b.Content[0].Text)
		})
	}
}

func Test_input_DescendantCount(t *testing.T) {
	t.Parallel()

	nestedID := xid.New()
	tree := document.Summaries{{
		ID: _testDocID,
		Children: document.Summaries{
			{ID: xid.New()},
			{ID: nestedID, Children: document.Summaries{{ID: xid.New()}}},
		},
	}}

	cc := map[string]struct {
		TreeErr error
		ID      xid.ID
		Result  int
		Err     error
	}{
		"Error returned by db.FetchDocumentTree": {
			TreeErr: assert.AnError,
			ID:      _testDocID,
			Err:     assert.AnError,
		},
		"Document at the root counts every level under it": {
			ID:     _testDocID,
			Result: 3,
		},
		"Nested document counts its own subtree": {
			ID:     nestedID,
			Result: 1,
		},
		"Document missing from the tree has none": {
			ID:     xid.New(),
			Result: 0,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					if c.TreeErr != nil {
						return nil, c.TreeErr
					}

					return tree, nil
				},
			}

			got, err := testDeps(db, nil, nil).newInput(context.Background(), NameDeleteDocument, nil).DescendantCount(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, db.FetchDocumentTreeCalls(), 1)
			assert.Equal(t, "org", db.FetchDocumentTreeCalls()[0].OrganizationID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_FetchDocumentTree(t *testing.T) {
	t.Parallel()

	tree := document.Summaries{{ID: _testDocID}}

	cc := map[string]struct {
		TreeErr error
		Result  document.Summaries
		Err     error
	}{
		"Error returned by db.FetchDocumentTree": {
			TreeErr: assert.AnError,
			Err:     assert.AnError,
		},
		"Successful fetch": {
			Result: tree,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchDocumentTreeFunc: func(context.Context, string) (document.Summaries, error) {
					if c.TreeErr != nil {
						return nil, c.TreeErr
					}

					return tree, nil
				},
			}

			got, err := testDeps(db, nil, nil).newInput(context.Background(), NameListDocuments, nil).FetchDocumentTree()
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, db.FetchDocumentTreeCalls(), 1)
			assert.Equal(t, "org", db.FetchDocumentTreeCalls()[0].OrganizationID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_FetchBranchesByDocument(t *testing.T) {
	t.Parallel()

	otherID := xid.New()

	cc := map[string]struct {
		Err    error
		Result map[xid.ID][]document.BranchSummary
	}{
		"Error returned by db.FetchDocumentBranchesByOrganizationID": {
			Err: assert.AnError,
		},
		"Branches keyed by document": {
			Result: map[xid.ID][]document.BranchSummary{
				_testDocID: {
					{BranchID: _stubMainBranchID, DocumentID: _testDocID, Default: true},
					{BranchID: _stubBranchID, DocumentID: _testDocID},
				},
				otherID: {
					{BranchID: _unknownBranchID, DocumentID: otherID, Default: true},
				},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchDocumentBranchesByOrganizationIDFunc: func(context.Context, string) ([]document.BranchSummary, error) {
					if c.Err != nil {
						return nil, c.Err
					}

					return []document.BranchSummary{
						{BranchID: _stubMainBranchID, DocumentID: _testDocID, Default: true},
						{BranchID: _stubBranchID, DocumentID: _testDocID},
						{BranchID: _unknownBranchID, DocumentID: otherID, Default: true},
					}, nil
				},
			}

			got, err := testDeps(db, nil, nil).newInput(context.Background(), NameListDocuments, nil).FetchBranchesByDocument()
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, db.FetchDocumentBranchesByOrganizationIDCalls(), 1)
			assert.Equal(t, "org", db.FetchDocumentBranchesByOrganizationIDCalls()[0].OrganizationID)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_FetchDocumentChildren(t *testing.T) {
	t.Parallel()

	parentID := null.ValueFrom(_testDocID)
	children := document.Summaries{{ID: xid.New()}}

	cc := map[string]struct {
		ChildrenErr error
		Children    document.Summaries
		CheckErr    error
		Checks      int
		Result      document.Summaries
		Err         error
	}{
		"Error returned by db.FetchDocumentTreeByDocumentParentID": {
			ChildrenErr: assert.AnError,
			Err:         assert.AnError,
		},
		// a parent with children exists, so it is not checked.
		"Successful fetch": {
			Children: children,
			Result:   children,
		},
		"Unknown parent is refused": {
			Children: document.Summaries{},
			CheckErr: sql.ErrNoRows,
			Checks:   1,
			Err:      fmt.Errorf("parent %s: %w", _testDocID, ErrUnknownDocument),
		},
		"Error returned by db.CheckDocumentExists": {
			Children: document.Summaries{},
			CheckErr: assert.AnError,
			Checks:   1,
			Err:      assert.AnError,
		},
		"Parent without children": {
			Children: document.Summaries{},
			Checks:   1,
			Result:   document.Summaries{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchDocumentTreeByDocumentParentIDFunc: func(context.Context, null.Value[xid.ID], string) (document.Summaries, error) {
					if c.ChildrenErr != nil {
						return nil, c.ChildrenErr
					}

					return c.Children, nil
				},
				CheckDocumentExistsFunc: func(context.Context, xid.ID, string) error {
					return c.CheckErr
				},
			}

			got, err := testDeps(db, nil, nil).newInput(context.Background(), NameListDocuments, nil).FetchDocumentChildren(parentID)
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, db.FetchDocumentTreeByDocumentParentIDCalls(), 1)
			assert.Equal(t, parentID, db.FetchDocumentTreeByDocumentParentIDCalls()[0].ParentID)
			assert.Equal(t, "org", db.FetchDocumentTreeByDocumentParentIDCalls()[0].OrganizationID)
			assert.Empty(t, db.FetchDocumentTreeCalls())
			assert.Len(t, db.CheckDocumentExistsCalls(), c.Checks)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_FetchDocument(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB  *DBMock
		Err error
	}{
		"Error returned by db.FetchDocument": {
			DB:  failingDocumentDB(),
			Err: assert.AnError,
		},
		"Unknown document": {
			DB: &DBMock{
				FetchDocumentFunc: func(context.Context, xid.ID, string, string) (*document.Document, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Err: ErrUnknownDocument,
		},
		"Default branch": {DB: stubDocumentDB()},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			doc, err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`)).FetchDocument(_testDocID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, document.DefaultBranch, doc.BranchName)
			assert.True(t, doc.Default)
		})
	}
}

func Test_input_FetchBranch(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Branch   xid.ID
		Result   string
		Listings int
		Err      error
	}{
		"Error returned by db.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
				},
			},
			Branch: _stubBranchID,
			Err:    assert.AnError,
		},
		"Unknown branch names the ones that exist": {
			DB:       stubDocumentDB(),
			Branch:   _unknownBranchID,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Unknown branch of an unknown document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Branch:   _unknownBranchID,
			Listings: 1,
			Err:      ErrUnknownDocument,
		},
		// a branch id resolves on its own, so another document's branch
		// is refused as unknown to this one.
		"Branch of another document": {
			DB: func() *DBMock {
				db := stubDocumentDB()
				db.FetchDocumentByBranchIDFunc = func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
					return stubBranchDocument(xid.New(), orgID, branchID, _stubBranchName), nil
				}

				return db
			}(),
			Branch:   _stubBranchID,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _stubBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Default branch": {
			DB:     stubDocumentDB(),
			Branch: _stubMainBranchID,
			Result: document.DefaultBranch,
		},
		"Another branch": {
			DB:     stubDocumentDB(),
			Branch: _stubBranchID,
			Result: _stubBranchName,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`))

			doc, err := inp.FetchBranch(_testDocID, c.Branch)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, doc.BranchName)
			assert.Equal(t, c.Branch, doc.BranchID)

			// the steps of one call share the fetch, and asking for the
			// branch as another document's is still refused.
			again, err := inp.FetchBranch(_testDocID, c.Branch)
			require.NoError(t, err)
			assert.Same(t, doc, again)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)

			_, err = inp.FetchBranch(xid.New(), c.Branch)
			require.Error(t, err)
			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 2)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), 1)
		})
	}
}

func Test_input_unknownBranch(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB  *DBMock
		Err error
	}{
		// the listing itself failing is passed on.
		"Error returned by db.FetchDocumentBranches": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		// the branches the document does have travel with the refusal.
		"Unknown branch names the ones that exist": {
			DB:  stubDocumentDB(),
			Err: fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`)).unknownBranch(_testDocID, _unknownBranchID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), 1)
		})
	}
}

func Test_input_FetchDocumentBranches(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Result []document.BranchSummary
		Err    error
	}{
		"Error returned by db.FetchDocumentBranches": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		// every document has a branch, so none means no document.
		"No branches is an unknown document": {
			DB:  &DBMock{},
			Err: ErrUnknownDocument,
		},
		"Branches listed": {
			DB: stubDocumentDB(),
			Result: []document.BranchSummary{
				{BranchID: _stubMainBranchID, BranchName: document.DefaultBranch, Default: true},
				{BranchID: _stubBranchID, BranchName: _stubBranchName},
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`)).FetchDocumentBranches(_testDocID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_protectedBranch(t *testing.T) {
	t.Parallel()

	doc := &document.Document{ID: _testDocID, BranchName: document.DefaultBranch, Protected: true}

	cc := map[string]struct {
		DB  *DBMock
		Err error
	}{
		// a listing that fails still says the branch is protected.
		"Error returned by db.FetchDocumentBranches": {
			DB: &DBMock{
				FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
					return nil, assert.AnError
				},
			},
			Err: fmt.Errorf("branch main: %w; %w", errBranchProtected, fmt.Errorf("fetching branches: %w", assert.AnError)),
		},
		"Unprotected branches are offered": {
			DB:  protectedDocumentDB(),
			Err: fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameInsertBlocks, json.RawMessage(`{}`)).protectedBranch(doc)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), 1)
		})
	}
}

func Test_branchLabels(t *testing.T) {
	t.Parallel()

	mainID, draftID := xid.New(), xid.New()
	branches := []document.BranchSummary{
		{BranchID: mainID, BranchName: "main", Default: true, Protected: true},
		{BranchID: draftID, BranchName: "draft"},
	}

	assert.Equal(t, "main ("+mainID.String()+"), draft ("+draftID.String()+")", branchLabels(branches))
	assert.Empty(t, branchLabels(nil))
}

func Test_input_FetchTagTree(t *testing.T) {
	t.Parallel()

	db := stubTagDB(nil)

	tree, err := testDeps(db, nil, nil).newInput(context.Background(), NameListTags, json.RawMessage(`{}`)).FetchTagTree()
	require.NoError(t, err)
	require.Len(t, tree, 2)
	assert.Equal(t, _testTagID, tree[0].ID)

	// the tree is the session's own: scoped to its pair, so the hidden
	// flags are the user's.
	ff := db.FetchTagTreeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, "user", ff[0].UserID)

	failing := stubTagDB(assert.AnError)

	_, err = testDeps(failing, nil, nil).newInput(context.Background(), NameListTags, json.RawMessage(`{}`)).FetchTagTree()
	require.Error(t, err)
	assert.Len(t, failing.FetchTagTreeCalls(), 1)
}

func Test_input_FetchTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		ID     xid.ID
		Result string
		Err    error
	}{
		"Error returned by db.FetchTagTree": {
			DB:  stubTagDB(assert.AnError),
			ID:  _testTagID,
			Err: assert.AnError,
		},
		"Unknown tag": {
			DB:  stubTagDB(nil),
			ID:  _unknownTagID,
			Err: fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Known tag": {
			DB:     stubTagDB(nil),
			ID:     _otherTagID,
			Result: "Staging",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tg, err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameDeleteTag, json.RawMessage(`{}`)).FetchTag(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchTagTreeCalls(), 1)

			if err != nil {
				assert.Nil(t, tg)
				return
			}

			assert.Equal(t, c.Result, tg.TagName)
		})
	}
}

func Test_input_FetchBranchTags(t *testing.T) {
	t.Parallel()

	db := &DBMock{
		FetchBranchTagsFunc: func(_ context.Context, orgID string, documentID, branchID xid.ID) ([]tag.Tag, error) {
			if orgID != "org" || documentID != _testDocID || branchID != _stubBranchID {
				return nil, assert.AnError
			}

			return []tag.Tag{{ID: _testTagID}}, nil
		},
	}

	tags, err := testDeps(db, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`)).FetchBranchTags(_testDocID, _stubBranchID)
	require.NoError(t, err)
	assert.Equal(t, []tag.Tag{{ID: _testTagID}}, tags)
	assert.Len(t, db.FetchBranchTagsCalls(), 1)

	_, err = testDeps(db, nil, nil).newInput(context.Background(), NameGetDocument, json.RawMessage(`{}`)).FetchBranchTags(_testDocID, _unknownBranchID)
	require.Error(t, err)
	assert.Len(t, db.FetchBranchTagsCalls(), 2)
}

func Test_input_CreateTag(t *testing.T) {
	t.Parallel()

	tg := tag.NewTag(tag.CreateInput{TagName: "Release", Color: "blue"}, "org", "user")

	cc := map[string]struct {
		DB       *DBMock
		Notifies int
		Err      error
	}{
		// the repository's own refusal of a duplicate name passes through
		// untouched: it already names the clash.
		"Duplicate tag name": {
			DB: &DBMock{
				InsertTagFunc: func(context.Context, tag.Tag) error { return tag.ErrDuplicateTagName },
			},
			Err: tag.ErrDuplicateTagName,
		},
		"Created": {
			DB:       &DBMock{},
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags

			err := d.newInput(context.Background(), NameCreateTag, json.RawMessage(`{}`)).CreateTag(tg)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)

			ff := c.DB.InsertTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, tg, ff[0].T)
		})
	}
}

func Test_input_UpdateTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Notifies int
		Err      error
	}{
		"Error returned by db.UpdateTag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"Unknown tag": {
			DB: &DBMock{
				UpdateTagFunc: func(context.Context, string, xid.ID, tag.UpdateInput) error {
					return errutil.ErrNotFound
				},
			},
			Err: fmt.Errorf("tag %s: %w", _testTagID, ErrUnknownTag),
		},
		"Updated": {DB: &DBMock{}, Notifies: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := tag.UpdateInput{TagName: null.StringFrom("Release")}
			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags

			err := d.newInput(context.Background(), NameUpdateTag, json.RawMessage(`{}`)).UpdateTag(_testTagID, inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)

			ff := c.DB.UpdateTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, _testTagID, ff[0].ID)
			assert.Equal(t, inp, ff[0].Inp)
		})
	}
}

func Test_input_DeleteTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Notifies int
		Err      error
	}{
		"Error returned by db.DeleteTag": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error { return assert.AnError },
			},
			Err: assert.AnError,
		},
		"Unknown tag": {
			DB: &DBMock{
				DeleteTagFunc: func(context.Context, xid.ID, string) error { return errutil.ErrNotFound },
			},
			Err: fmt.Errorf("tag %s: %w", _testTagID, ErrUnknownTag),
		},
		"Deleted": {DB: &DBMock{}, Notifies: 1},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags

			err := d.newInput(context.Background(), NameDeleteTag, json.RawMessage(`{}`)).DeleteTag(_testTagID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)

			ff := c.DB.DeleteTagCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testTagID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_AssignTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB       *DBMock
		Branch   xid.ID
		Listings int
		Assigns  int
		Notifies int
		Err      error
	}{
		"Unknown branch is refused before the write": {
			DB:       stubDocumentDB(),
			Branch:   _unknownBranchID,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Protected branch is refused before the write": {
			DB:       protectedDocumentDB(),
			Branch:   _stubMainBranchID,
			Listings: 1,
			Err:      fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		"Unknown tag": {
			DB: func() *DBMock {
				db := stubDocumentDB()
				db.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return errutil.ErrNotFound
				}

				return db
			}(),
			Branch:  _stubMainBranchID,
			Assigns: 1,
			Err:     fmt.Errorf("tag %s: %w", _testTagID, ErrUnknownTag),
		},
		"Error returned by db.AssignBranchTag": {
			DB: func() *DBMock {
				db := stubDocumentDB()
				db.AssignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				}

				return db
			}(),
			Branch:  _stubMainBranchID,
			Assigns: 1,
			Err:     assert.AnError,
		},
		"Assigned and recorded": {
			DB:       stubDocumentDB(),
			Branch:   _stubBranchID,
			Assigns:  1,
			Notifies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags
			inp := d.newInput(context.Background(), NameSetTagAssignment, json.RawMessage(`{}`))

			err := inp.AssignTag(_testDocID, c.Branch, _testTagID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)
			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)
			assert.Len(t, tags.NotifyBranchTagsChangeCalls(), c.Notifies)

			ff := c.DB.AssignBranchTagCalls()
			require.Len(t, ff, c.Assigns)

			if c.Assigns > 0 {
				assert.Equal(t, "org", ff[0].OrganizationID)
				assert.Equal(t, _testDocID, ff[0].DocumentID)
				assert.Equal(t, c.Branch, ff[0].BranchID)
				assert.Equal(t, _testTagID, ff[0].TagID)
			}
		})
	}
}

func Test_input_UnassignTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB         *DBMock
		Branch     xid.ID
		Tag        xid.ID
		Listings   int
		TagFetches int
		Unassigns  int
		Notifies   int
		Err        error
	}{
		"Unknown branch is refused before the write": {
			DB:       stubTagDB(nil),
			Branch:   _unknownBranchID,
			Tag:      _testTagID,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Protected branch is refused before the write": {
			DB:       protectedDocumentDB(),
			Branch:   _stubMainBranchID,
			Tag:      _testTagID,
			Listings: 1,
			Err:      fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		// the repository treats a tag the branch does not carry as
		// nothing to do, so an id naming nothing is caught here.
		"Unknown tag": {
			DB:         stubTagDB(nil),
			Branch:     _stubMainBranchID,
			Tag:        _unknownTagID,
			TagFetches: 1,
			Err:        fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Error returned by db.UnassignBranchTag": {
			DB: func() *DBMock {
				db := stubTagDB(nil)
				db.UnassignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				}

				return db
			}(),
			Branch:     _stubMainBranchID,
			Tag:        _testTagID,
			TagFetches: 1,
			Unassigns:  1,
			Err:        assert.AnError,
		},
		"Unassigned and recorded": {
			DB:         stubTagDB(nil),
			Branch:     _stubBranchID,
			Tag:        _testTagID,
			TagFetches: 1,
			Unassigns:  1,
			Notifies:   1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags
			inp := d.newInput(context.Background(), NameSetTagAssignment, json.RawMessage(`{}`))

			err := inp.UnassignTag(_testDocID, c.Branch, c.Tag)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)
			assert.Len(t, c.DB.FetchTagTreeCalls(), c.TagFetches)
			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)
			assert.Len(t, tags.NotifyBranchTagsChangeCalls(), c.Notifies)

			ff := c.DB.UnassignBranchTagCalls()
			require.Len(t, ff, c.Unassigns)

			if c.Unassigns > 0 {
				assert.Equal(t, "org", ff[0].OrganizationID)
				assert.Equal(t, _testDocID, ff[0].DocumentID)
				assert.Equal(t, c.Branch, ff[0].BranchID)
				assert.Equal(t, c.Tag, ff[0].TagID)
			}
		})
	}
}

func Test_input_MoveTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		Tag       xid.ID
		SortIndex int
		Order     []xid.ID
		Notifies  int
		Err       error
	}{
		"Error returned by db.FetchTagTree": {
			DB:  stubTagDB(assert.AnError),
			Tag: _testTagID,
			Err: assert.AnError,
		},
		"Unknown tag": {
			DB:  stubTagDB(nil),
			Tag: _unknownTagID,
			Err: fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Sort index past the last tag": {
			DB:        stubTagDB(nil),
			Tag:       _testTagID,
			SortIndex: 2,
			Err:       assert.AnError,
		},
		"Negative sort index": {
			DB:        stubTagDB(nil),
			Tag:       _testTagID,
			SortIndex: -1,
			Err:       assert.AnError,
		},
		"Error returned by db.UpdateTagTree": {
			DB: func() *DBMock {
				db := stubTagDB(nil)
				db.UpdateTagTreeFunc = func(context.Context, tag.Summaries, string) error {
					return assert.AnError
				}

				return db
			}(),
			Tag:       _testTagID,
			SortIndex: 1,
			Order:     []xid.ID{_otherTagID, _testTagID},
			Err:       assert.AnError,
		},
		"Moved": {
			DB:        stubTagDB(nil),
			Tag:       _testTagID,
			SortIndex: 1,
			Order:     []xid.ID{_otherTagID, _testTagID},
			Notifies:  1,
		},
		"Kept in place": {
			DB:        stubTagDB(nil),
			Tag:       _testTagID,
			SortIndex: 0,
			Order:     []xid.ID{_testTagID, _otherTagID},
			Notifies:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tags := &TagNotifierMock{}
			d := testDeps(c.DB, nil, nil)
			d.tags = tags

			err := d.newInput(context.Background(), NameUpdateTag, json.RawMessage(`{}`)).MoveTag(c.Tag, c.SortIndex)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchTagTreeCalls(), 1)
			assert.Len(t, tags.NotifyTreeChangeCalls(), c.Notifies)

			ff := c.DB.UpdateTagTreeCalls()

			if c.Order == nil {
				assert.Empty(t, ff)
				return
			}

			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)

			order := make([]xid.ID, 0, len(ff[0].Tree))
			for _, s := range ff[0].Tree {
				order = append(order, s.ID)
			}

			assert.Equal(t, c.Order, order)
		})
	}
}

func Test_input_notifyTagTreeChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tags *TagNotifierMock
	}{
		// a session without a notifier silently no-ops.
		"No notifier":         {},
		"Notifier configured": {Tags: &TagNotifierMock{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := testDeps(nil, nil, nil)
			if c.Tags != nil {
				d.tags = c.Tags
			}

			d.newInput(context.Background(), NameCreateTag, json.RawMessage(`{}`)).notifyTagTreeChange()

			if c.Tags == nil {
				return
			}

			ff := c.Tags.NotifyTreeChangeCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_notifyBranchTagsChange(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Tags *TagNotifierMock
	}{
		// a session without a notifier silently no-ops.
		"No notifier":         {},
		"Notifier configured": {Tags: &TagNotifierMock{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := testDeps(nil, nil, nil)
			if c.Tags != nil {
				d.tags = c.Tags
			}

			d.newInput(context.Background(), NameSetTagAssignment, json.RawMessage(`{}`)).notifyBranchTagsChange(_testDocID, _stubBranchID)

			if c.Tags == nil {
				return
			}

			ff := c.Tags.NotifyBranchTagsChangeCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, _stubBranchID, ff[0].BranchID)
		})
	}
}

func Test_input_FetchHooks(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB          *DBMock
		Branch      xid.ID
		Result      []hook.Hook
		Listings    int
		HookFetches int
		Err         error
	}{
		"Unknown branch": {
			DB:       stubHookDB(),
			Branch:   _unknownBranchID,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Error returned by db.FetchDocumentHooksByBranchID": {
			DB:          failing,
			Branch:      _stubBranchID,
			HookFetches: 1,
			Err:         assert.AnError,
		},
		"Branch without hooks": {
			DB:          stubHookDB(),
			Branch:      _stubMainBranchID,
			Result:      []hook.Hook{},
			HookFetches: 1,
		},
		"Branch with hooks": {
			DB:          stubHookDB(),
			Branch:      _stubBranchID,
			Result:      []hook.Hook{*stubHook()},
			HookFetches: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameListHooks, json.RawMessage(`{}`)).FetchHooks(_testDocID, c.Branch)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)

			ff := c.DB.FetchDocumentHooksByBranchIDCalls()
			require.Len(t, ff, c.HookFetches)

			if err != nil {
				assert.Nil(t, got)
				return
			}

			assert.Equal(t, c.Result, got)
			assert.Equal(t, c.Branch, ff[0].BranchID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_FetchHook(t *testing.T) {
	t.Parallel()

	otherDocument := stubHook()
	otherDocument.DocumentID = null.ValueFrom(xid.New())

	deletedDocument := stubHook()
	deletedDocument.DocumentID = null.Value[xid.ID]{}

	cc := map[string]struct {
		DB  *DBMock
		ID  xid.ID
		Err error
	}{
		"Error returned by db.FetchDocumentHook": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hook.Hook, error) {
					return nil, assert.AnError
				},
			},
			ID:  _testHookID,
			Err: assert.AnError,
		},
		"Unknown hook": {
			DB:  stubHookDB(),
			ID:  _unknownHookID,
			Err: fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook),
		},
		// the repository scopes by organisation, so another one's hook
		// is as absent as an id that names nothing.
		"Hook of another organisation": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hook.Hook, error) {
					return nil, errutil.ErrNotFound
				},
			},
			ID:  _testHookID,
			Err: fmt.Errorf("hook %s on document %s: %w", _testHookID, _testDocID, errUnknownHook),
		},
		"Hook of another document": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hook.Hook, error) {
					return otherDocument, nil
				},
			},
			ID:  _testHookID,
			Err: fmt.Errorf("hook %s on document %s: %w", _testHookID, _testDocID, errUnknownHook),
		},
		"Hook of a deleted document": {
			DB: &DBMock{
				FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hook.Hook, error) {
					return deletedDocument, nil
				},
			},
			ID:  _testHookID,
			Err: fmt.Errorf("hook %s on document %s: %w", _testHookID, _testDocID, errUnknownHook),
		},
		"Known hook": {
			DB: stubHookDB(),
			ID: _testHookID,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hk, err := testDeps(c.DB, nil, nil).newInput(context.Background(), NameUpdateHook, json.RawMessage(`{}`)).FetchHook(_testDocID, c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.FetchDocumentHookCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.ID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.Equal(t, stubHook(), hk)
		})
	}
}

func Test_input_CreateHook(t *testing.T) {
	t.Parallel()

	scheduled := processor.Settings(`{"scale":"linear","duration":"custom","schedule":"` + _stubSchedule + `"}`)

	failing := stubHookManager()
	failing.CreateHookFunc = func(context.Context, hook.CreateInput, xid.ID, string, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	blockless := stubHookManager()
	blockless.CreateHookFunc = func(context.Context, hook.CreateInput, xid.ID, string, string) (*hook.Hook, error) {
		return nil, hook.ErrBlockNotFound
	}

	cc := map[string]struct {
		DB       *DBMock
		Man      *HookManagerMock
		Branch   xid.ID
		BlockUID string
		Type     hook.Type
		Settings processor.Settings
		Listings int
		Inserts  int
		Err      error
	}{
		"Unknown branch is refused before anything is created": {
			DB:       stubHookDB(),
			Branch:   _unknownBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Listings: 1,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Protected branch is refused before anything is created": {
			DB:       protectedDocumentDB(),
			Branch:   _stubMainBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Listings: 1,
			Err:      fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		"Unknown block": {
			DB:       stubHookDB(),
			Man:      blockless,
			Branch:   _stubBranchID,
			BlockUID: "nope",
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
			Err:      fmt.Errorf("block %s: %w", "nope", errUnknownBlock),
		},
		"GitHub tracking without the app": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeGithubTracking,
			Settings: processor.Settings(`{"repository":"r","branch":"main","paths":["a"]}`),
			Err:      fmt.Errorf("%s: %w", hook.TypeGithubTracking, github.ErrNotConfigured),
		},
		"URL watcher without changedetection": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			Err:      fmt.Errorf("%s: %w", hook.TypeURLWatcher, webchange.ErrNotConfigured),
		},
		"Error returned by hookMan.CreateHook": {
			DB:       stubHookDB(),
			Man:      failing,
			Branch:   _stubBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
			Err:      assert.AnError,
		},
		"Created on the document": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
		},
		"Created on a block": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			BlockUID: _stubContentUID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameCreateHook, json.RawMessage(`{}`))

			hk, err := inp.CreateHook(_testDocID, c.Branch, c.BlockUID, c.Type, c.Settings)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentByBranchIDCalls(), 1)
			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)

			ff := d.hookMan.(*HookManagerMock).CreateHookCalls()
			require.Len(t, ff, c.Inserts)

			// the write is credited to the user the assistant acts for.
			for _, call := range ff {
				assert.Equal(t, inp.userID, call.UpdatedBy)
			}

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.Equal(t, c.Type, ff[0].Ci.Type)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, c.Type, hk.Type)
			assert.Equal(t, null.ValueFrom(_testDocID), hk.DocumentID)
			assert.Equal(t, null.ValueFrom(c.Branch), hk.BranchID)
			assert.Equal(t, null.StringFrom("org"), hk.OrganizationID)
			assert.Equal(t, null.NewString(c.BlockUID, c.BlockUID != ""), hk.BlockID)
			assert.Equal(t, c.Settings, hk.Settings)
		})
	}
}

func Test_input_UpdateHook(t *testing.T) {
	t.Parallel()

	failing := stubHookManager()
	failing.UpdateHookFunc = func(context.Context, xid.ID, xid.ID, string, hook.UpdateInput, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB       *DBMock
		Man      *HookManagerMock
		Settings processor.Settings
		Updates  int
		Err      error
	}{
		"Error returned by hookMan.UpdateHook": {
			DB:       stubHookDB(),
			Man:      failing,
			Settings: processor.Settings(`{"scale":"linear","duration":"custom","schedule":"2031-01-01T00:00:00Z"}`),
			Updates:  1,
			Err:      assert.AnError,
		},
		"Updated and recorded": {
			DB:       stubHookDB(),
			Settings: processor.Settings(`{"scale":"linear","duration":"custom","schedule":"2031-01-01T00:00:00Z"}`),
			Updates:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameUpdateHook, json.RawMessage(`{}`))
			hk := stubHook()

			err := inp.UpdateHook(hk, c.Settings)
			testutil.AssertEqualError(t, c.Err, err)

			ff := d.hookMan.(*HookManagerMock).UpdateHookCalls()
			require.Len(t, ff, c.Updates)

			// the write is credited to the user the assistant acts for.
			for _, call := range ff {
				assert.Equal(t, inp.userID, call.UpdatedBy)
			}

			if err != nil {
				return
			}

			// the hook handed in takes the stored one.
			assert.Equal(t, hk.ID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, c.Settings, ff[0].UI.Settings)
			assert.Equal(t, c.Settings, hk.Settings)
			assert.True(t, hk.UpdatedAt.Valid)
		})
	}
}

func Test_input_ResetHook(t *testing.T) {
	t.Parallel()

	failing := stubHookManager()
	failing.ResetHookFunc = func(context.Context, xid.ID, xid.ID, string) (*hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB      *DBMock
		Man     *HookManagerMock
		Hook    *hook.Hook
		Updates int
		Err     error
	}{
		"Error returned by hookMan.ResetHook": {
			DB:      stubHookDB(),
			Man:     failing,
			Hook:    stubHook(),
			Updates: 1,
			Err:     assert.AnError,
		},
		"Reset and recorded": {
			DB:      stubHookDB(),
			Hook:    stubHook(),
			Updates: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameUpdateHook, json.RawMessage(`{}`))

			err := inp.ResetHook(c.Hook)
			testutil.AssertEqualError(t, c.Err, err)

			ff := d.hookMan.(*HookManagerMock).ResetHookCalls()
			require.Len(t, ff, c.Updates)

			if err != nil {
				return
			}

			// the hook handed in takes the stored one.
			assert.Equal(t, c.Hook.ID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.True(t, c.Hook.UpdatedAt.Valid)
		})
	}
}

func Test_input_DeleteHook(t *testing.T) {
	t.Parallel()

	failing := stubHookManager()
	failing.DeleteHookFunc = func(context.Context, xid.ID, xid.ID, string, string) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB       *DBMock
		Man      *HookManagerMock
		Hook     *hook.Hook
		Listings int
		Deletes  int
		Err      error
	}{
		"Error returned by db.FetchDocumentByBranchID": {
			DB:   failingDocumentDB(),
			Hook: stubHook(),
			Err:  assert.AnError,
		},
		"Protected branch is refused before the delete": {
			DB:       protectedDocumentDB(),
			Hook:     stubHook(),
			Listings: 1,
			Err:      fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		"Hook without a branch is deleted": {
			DB: failingDocumentDB(),
			Hook: func() *hook.Hook {
				hk := stubHook()
				hk.BranchID = null.Value[xid.ID]{}

				return hk
			}(),
			Deletes: 1,
		},
		"Error returned by hookMan.DeleteHook": {
			DB:      stubHookDB(),
			Man:     failing,
			Hook:    stubHook(),
			Deletes: 1,
			Err:     assert.AnError,
		},
		"Deleted and recorded": {
			DB:      stubHookDB(),
			Hook:    stubHook(),
			Deletes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := hookDeps(c.DB, c.Man)
			inp := d.newInput(context.Background(), NameDeleteHook, json.RawMessage(`{}`))

			err := inp.DeleteHook(c.Hook)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.DB.FetchDocumentBranchesCalls(), c.Listings)

			ff := d.hookMan.(*HookManagerMock).DeleteHookCalls()
			require.Len(t, ff, c.Deletes)

			// the write is credited to the user the assistant acts for.
			for _, call := range ff {
				assert.Equal(t, inp.userID, call.UpdatedBy)
			}

			if err != nil {
				return
			}

			assert.Equal(t, _testHookID, ff[0].ID)
			assert.Equal(t, _testDocID, ff[0].DocumentID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_FetchVariableTree(t *testing.T) {
	t.Parallel()

	man := stubVariableManager()

	tree, err := variableDeps(man).newInput(context.Background(), NameListVariables, json.RawMessage(`{}`)).FetchVariableTree()
	require.NoError(t, err)
	assert.Equal(t, stubVariableTree(), tree)

	ff := man.FetchTreeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, "user", ff[0].UserID)
}

func Test_input_SubstituteVariables(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man    *VariableManagerMock
		Texts  []string
		Calls  int
		Result []string
		Err    error
	}{
		"No reference skips the tree": {
			Man:    stubVariableManager(),
			Texts:  []string{"up", "$__from"},
			Result: []string{"up", "$__from"},
		},
		"Error returned by VariableManager.FetchTree": {
			Man:   failingTreeManager(),
			Texts: []string{"$region"},
			Calls: 1,
			Err:   assert.AnError,
		},
		"Successful substitution": {
			Man:    stubVariableManager(),
			Texts:  []string{"up", "$region $metric_range"},
			Calls:  1,
			Result: []string{"up", "eu-west-1 last_7_days"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			res, err := variableDeps(c.Man).newInput(context.Background(), NameQueryDataSource, json.RawMessage(`{}`)).SubstituteVariables(c.Texts...)
			assert.Len(t, c.Man.FetchTreeCalls(), c.Calls)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, res)
		})
	}
}

func Test_input_FetchVariableGroup(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man    *VariableManagerMock
		ID     xid.ID
		Result string
		Err    error
	}{
		"Error returned by VariableManager.FetchTree": {Man: failingTreeManager(), ID: _testVariableGroupID, Err: assert.AnError},
		"Unknown group": {
			Man: stubVariableManager(),
			ID:  _unknownVariableID,
			Err: fmt.Errorf("variable group %s: %w", _unknownVariableID, ErrUnknownVariableGroup),
		},
		"Known group": {Man: stubVariableManager(), ID: _testVariableGroupID, Result: "Environment"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			g, err := variableDeps(c.Man).newInput(context.Background(), NameDeleteVariableGroup, json.RawMessage(`{}`)).FetchVariableGroup(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, g.GroupName)
		})
	}
}

func Test_input_FetchVariable(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		Man    *VariableManagerMock
		ID     xid.ID
		Result string
		Err    error
	}{
		"Error returned by VariableManager.FetchTree": {Man: failingTreeManager(), ID: _testVariableID, Err: assert.AnError},
		"Unknown variable": {
			Man: stubVariableManager(),
			ID:  _unknownVariableID,
			Err: fmt.Errorf("variable %s: %w", _unknownVariableID, ErrUnknownVariable),
		},
		"Variable of the second group": {Man: stubVariableManager(), ID: _testVariableID, Result: "region"},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v, err := variableDeps(c.Man).newInput(context.Background(), NameDeleteVariable, json.RawMessage(`{}`)).FetchVariable(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), 1)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, v.Key)
		})
	}
}

func Test_input_CreateVariableGroup(t *testing.T) {
	t.Parallel()

	man := stubVariableManager()
	inp := variable.GroupCreateInput{GroupName: "Deploys", Color: "orange"}

	g, err := variableDeps(man).newInput(context.Background(), NameCreateVariableGroup, json.RawMessage(`{}`)).CreateVariableGroup(inp)
	require.NoError(t, err)
	assert.Equal(t, _testVariableGroupID, g.ID)

	ff := man.CreateGroupCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, inp, ff[0].Inp)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, "user", ff[0].UserID)
}

func Test_input_UpdateVariableGroup(t *testing.T) {
	t.Parallel()

	stub := func(err error) *VariableManagerMock {
		return &VariableManagerMock{
			UpdateGroupFunc: func(context.Context, xid.ID, variable.GroupUpdateInput, string) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.UpdateGroup": {
			Man: stub(assert.AnError),
			Err: assert.AnError,
		},
		"Not found": {
			Man: stub(errutil.ErrNotFound),
			Err: fmt.Errorf("variable group %s: %w", _testVariableGroupID, ErrUnknownVariableGroup),
		},
		"Successful call": {
			Man: stub(nil),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := variableDeps(c.Man).newInput(context.Background(), NameUpdateVariableGroup, json.RawMessage(`{}`)).UpdateVariableGroup(_testVariableGroupID, variable.GroupUpdateInput{GroupName: null.StringFrom("Env")})
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.UpdateGroupCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableGroupID, ff[0].ID)
			assert.Equal(t, variable.GroupUpdateInput{GroupName: null.StringFrom("Env")}, ff[0].Inp)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_DeleteVariableGroup(t *testing.T) {
	t.Parallel()

	stub := func(err error) *VariableManagerMock {
		return &VariableManagerMock{
			DeleteGroupFunc: func(context.Context, xid.ID, string) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.DeleteGroup": {
			Man: stub(assert.AnError),
			Err: assert.AnError,
		},
		"Not found": {
			Man: stub(errutil.ErrNotFound),
			Err: fmt.Errorf("variable group %s: %w", _testVariableGroupID, ErrUnknownVariableGroup),
		},
		"Successful call": {
			Man: stub(nil),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := variableDeps(c.Man).newInput(context.Background(), NameDeleteVariableGroup, json.RawMessage(`{}`)).DeleteVariableGroup(_testVariableGroupID)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.DeleteGroupCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableGroupID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_CreateVariable(t *testing.T) {
	t.Parallel()

	inp := variable.Input{GroupID: _testVariableGroupID, Key: "release"}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.CreateVariable": {
			Man: &VariableManagerMock{
				CreateVariableFunc: func(context.Context, variable.Input, string, string) (*variable.Variable, error) {
					return nil, assert.AnError
				},
			},
			Err: assert.AnError,
		},
		"Created": {Man: stubVariableManager()},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			v, err := variableDeps(c.Man).newInput(context.Background(), NameCreateVariable, json.RawMessage(`{}`)).CreateVariable(inp)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.CreateVariableCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, inp, ff[0].Inp)
			assert.Equal(t, "user", ff[0].UserID)

			if err != nil {
				assert.Nil(t, v)
				return
			}

			assert.Equal(t, _testVariableID, v.ID)
		})
	}
}

func Test_input_UpdateVariable(t *testing.T) {
	t.Parallel()

	withUpdate := func(err error) *VariableManagerMock {
		m := stubVariableManager()
		m.UpdateVariableFunc = func(context.Context, xid.ID, variable.Input, string) error {
			return err
		}

		return m
	}

	cc := map[string]struct {
		Man         *VariableManagerMock
		GroupID     xid.ID
		TreeFetches int
		Err         error
	}{
		"Error returned by VariableManager.UpdateVariable": {
			Man:     withUpdate(assert.AnError),
			GroupID: _testVariableGroupID,
			Err:     assert.AnError,
		},
		"Unknown group": {
			Man:         withUpdate(errutil.ErrNotFound),
			GroupID:     _unknownVariableID,
			TreeFetches: 1,
			Err:         fmt.Errorf("variable group %s: %w", _unknownVariableID, ErrUnknownVariableGroup),
		},
		"Unknown variable": {
			Man:         withUpdate(errutil.ErrNotFound),
			GroupID:     _testVariableGroupID,
			TreeFetches: 1,
			Err:         fmt.Errorf("variable %s: %w", _testVariableID, ErrUnknownVariable),
		},
		"Updated": {
			Man:     withUpdate(nil),
			GroupID: _testVariableGroupID,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := variable.Input{GroupID: c.GroupID, Key: "zone"}

			err := variableDeps(c.Man).newInput(context.Background(), NameUpdateVariable, json.RawMessage(`{}`)).UpdateVariable(_testVariableID, inp)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, c.Man.FetchTreeCalls(), c.TreeFetches)

			ff := c.Man.UpdateVariableCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.Equal(t, inp, ff[0].Inp)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_DeleteVariable(t *testing.T) {
	t.Parallel()

	stub := func(err error) *VariableManagerMock {
		return &VariableManagerMock{
			DeleteVariableFunc: func(context.Context, xid.ID, string) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.DeleteVariable": {
			Man: stub(assert.AnError),
			Err: assert.AnError,
		},
		"Not found": {
			Man: stub(errutil.ErrNotFound),
			Err: fmt.Errorf("variable %s: %w", _testVariableID, ErrUnknownVariable),
		},
		"Successful call": {
			Man: stub(nil),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := variableDeps(c.Man).newInput(context.Background(), NameDeleteVariable, json.RawMessage(`{}`)).DeleteVariable(_testVariableID)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.DeleteVariableCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_SetVariableValue(t *testing.T) {
	t.Parallel()

	stub := func(err error) *VariableManagerMock {
		return &VariableManagerMock{
			SetValueFunc: func(context.Context, xid.ID, string, string, string) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.SetValue": {
			Man: stub(assert.AnError),
			Err: assert.AnError,
		},
		"Not found": {
			Man: stub(errutil.ErrNotFound),
			Err: fmt.Errorf("variable %s: %w", _testVariableID, ErrUnknownVariable),
		},
		"Successful call": {
			Man: stub(nil),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := variableDeps(c.Man).newInput(context.Background(), NameSetVariableValue, json.RawMessage(`{}`)).SetVariableValue(_testVariableID, "x")
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.SetValueCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.Equal(t, "x", ff[0].Value)
			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, "user", ff[0].UserID)
		})
	}
}

func Test_input_SetVariableDefault(t *testing.T) {
	t.Parallel()

	stub := func(err error) *VariableManagerMock {
		return &VariableManagerMock{
			SetDefaultValueFunc: func(context.Context, xid.ID, string, string) error {
				return err
			},
		}
	}

	cc := map[string]struct {
		Man *VariableManagerMock
		Err error
	}{
		"Error returned by VariableManager.SetDefaultValue": {
			Man: stub(assert.AnError),
			Err: assert.AnError,
		},
		"Not found": {
			Man: stub(errutil.ErrNotFound),
			Err: fmt.Errorf("variable %s: %w", _testVariableID, ErrUnknownVariable),
		},
		"Successful call": {
			Man: stub(nil),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := variableDeps(c.Man).newInput(context.Background(), NameSetVariableValue, json.RawMessage(`{}`)).SetVariableDefault(_testVariableID, "x")
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Man.SetDefaultValueCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, _testVariableID, ff[0].ID)
			assert.Equal(t, "x", ff[0].Value)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}
