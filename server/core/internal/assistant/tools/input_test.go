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
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/datasource"
	datasourceMock "github.com/oxynote/oxynote/server/core/internal/datasource/_mock"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
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
	_stubTagColor = "#22c55e"
)

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
		githubMan:       unconfiguredGithub(),
		webchangeClient: webchange.NewClient("", ""),
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

// unconfiguredGithub builds the GitHub manager of a deployment without
// the app, which refuses every call that would reach GitHub.
func unconfiguredGithub() *github.Manager {
	man, err := github.NewManager(nil, github.Options{})
	if err != nil {
		panic(err)
	}

	return man
}

// testInput builds the per-call input a tool is handed.
func testInput(d *Deps, name Name, args string) *input {
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
		case "document_id", "data_source_id":
			vals[key] = _testDocID.String()
		case "tag_id":
			vals[key] = _testTagID.String()
		case "hook_id":
			vals[key] = _testHookID.String()
		case "settings":
			// the stubbed hook is a scheduled reminder.
			vals[key] = map[string]any{"type": string(hook.TypeScheduledReminder), "schedule": _stubSchedule}
		case "color":
			vals[key] = _stubTagColor
		case "sort_index":
			vals[key] = 0
		case "block":
			vals[key] = map[string]any{"type": string(block.BlockParagraph)}
		case "position":
			// a position beside a block needs the reference the tool
			// requires; one that requires no reference takes an end.
			vals[key] = string(positionEnd)
			if slices.Contains(infoOf(name).Required, "reference_block_uid") {
				vals[key] = string(positionAfter)
			}
		case "branch_id":
			vals[key] = _stubMainBranchID.String()
		case "reference_block_uid":
			// distinct from the "x" other uid keys take, so a payload
			// naming both a block and a reference is not a self-move.
			vals[key] = "r"
		case "matchers":
			vals[key] = []string{"up"}
		case "attrs":
			vals[key] = map[string]any{"level": 2}
		default:
			vals[key] = "x"
		}
	}

	// update_document and update_tag require nothing beyond their
	// target, but refuse a call that changes nothing; a name is the
	// smallest change.
	if name == NameUpdateDocument || name == NameUpdateTag {
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
		FetchTagTreeFunc:          stubTagTree,
		FetchDocumentHookFunc:     stubHookLookup,
	}
}

// stubHookLookup answers a hook lookup with the test hook, a scheduled
// reminder on the draft branch of the test document, for the test hook
// id and with not found for any other.
func stubHookLookup(_ context.Context, id xid.ID, _ string) (*hook.Hook, error) {
	if id != _testHookID {
		return nil, errutil.ErrNotFound
	}

	return stubHook(), nil
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
		State:          processor.State(`{"startedAt":"2026-01-01T00:00:00Z"}`),
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

// stubTagTree answers a tag tree listing with two tags: the test tag,
// carried by the test document, and a hidden one carried by nothing.
func stubTagTree(context.Context, string, string) (tag.Summaries, error) {
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
			Color:     "#f97316",
			Hidden:    true,
			Documents: document.Summaries{},
		},
	}, nil
}

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
		tags     = &TagNotifierMock{}
		hooks    = &HookNotifierMock{}
		offload  = &offloadReaderMock{}
		gh       = unconfiguredGithub()
		wc       = webchange.NewClient("", "")
	)

	runners := &DataSourceRunnersMock{
		RunnerFunc: func(datasource.DataSource) datasource.Runner { return runner },
	}

	jobs := search.NewJobs(true)

	d := NewDeps(discardLog(), db, searcher, jobs, runners, gh, wc, applier, tree, tags, hooks, offload, "org", "user")
	require.NotNil(t, d)

	assert.NotNil(t, d.log)
	assert.Same(t, db, d.db)
	assert.Same(t, searcher, d.search)
	assert.Same(t, jobs, d.jobs)
	assert.Same(t, runners, d.runners)
	assert.Same(t, gh, d.githubMan)
	assert.Same(t, wc, d.webchangeClient)
	assert.Same(t, applier, d.applier)
	assert.Same(t, tree, d.tree)
	assert.Same(t, tags, d.tags)
	assert.Same(t, hooks, d.hooks)
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
		// a provider calling a parameterless tool sends nothing rather
		// than an empty object; that is a payload, not a decode failure.
		"Empty arguments read as none": {Args: ``, Err: "read_block: document_id is required"},
		"Blank arguments read as none": {Args: " \n", Err: "read_block: document_id is required"},
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
			Args: `{` + targetArgs(_stubMainBranchID) + `}`,
			Err:  "read_block: block_uid is required",
		},
		"Unknown keys are ignored": {
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b","extra":true}`,
			Want: readBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
		},
		"Decoded": {
			Args: `{` + targetArgs(_stubMainBranchID) + `,"block_uid":"b"}`,
			Want: readBlockArgs{DocumentID: _testDocID, BranchID: _stubMainBranchID, BlockUID: "b"},
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
				assert.Equal(t, []Touched{{DocumentID: doc.ID, BranchID: doc.BranchID}}, inp.touched)
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

	a, b, branch := xid.New(), xid.New(), xid.New()

	inp.recordTouched(a, branch)
	inp.recordTouched(b, branch)

	// a branch changed twice in one call is still one branch, and the
	// order the call touched them is preserved.
	inp.recordTouched(a, branch)

	// nothing to record is not something to record.
	inp.recordTouched(xid.NilID(), branch)

	assert.Equal(t, []Touched{{DocumentID: a, BranchID: branch}, {DocumentID: b, BranchID: branch}}, inp.touched)
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

			err := inp.MoveDocument(&document.Document{ID: docID, BranchID: branchID}, c.Parent)
			testutil.AssertEqualError(t, c.Err, err)

			// nothing is written until the destination is known good.
			assert.Len(t, c.DB.CheckDocumentExistsCalls(), c.Checks)
			assert.Len(t, c.DB.UpdateDocumentParentIDCalls(), c.Updates)

			// the parents' own content is unchanged by a re-parent, so
			// the moved document is the only one recorded.
			if err == nil {
				assert.Equal(t, []Touched{{DocumentID: docID, BranchID: branchID}}, inp.touched)
			} else {
				assert.Empty(t, inp.touched)
			}
		})
	}
}

func Test_input_ApplyEdit(t *testing.T) {
	t.Parallel()

	docID := _testDocID

	cc := map[string]struct {
		DB      *DBMock
		Applier *EditApplierMock
		Branch  xid.ID
		Touched []Touched
		Applies int
		Err     error
	}{
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
			Err:     fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID),
		},
		"Protected branch with no alternative says so": func() struct {
			DB      *DBMock
			Applier *EditApplierMock
			Branch  xid.ID
			Touched []Touched
			Applies int
			Err     error
		} {
			db := protectedDocumentDB()
			db.FetchDocumentBranchesFunc = stubBranches(true, true)

			return struct {
				DB      *DBMock
				Applier *EditApplierMock
				Branch  xid.ID
				Touched []Touched
				Applies int
				Err     error
			}{
				DB:      db,
				Applier: stubApplier(),
				Err:     fmt.Errorf("branch main: %w; the document has no unprotected branch to write to", errBranchProtected),
			}
		}(),
		"Applied to a named branch": func() struct {
			DB      *DBMock
			Applier *EditApplierMock
			Branch  xid.ID
			Touched []Touched
			Applies int
			Err     error
		} {
			// the branch has to belong to the document being edited.
			db := stubDocumentDB()
			db.FetchDocumentByBranchIDFunc = func(_ context.Context, branchID xid.ID, orgID string) (*document.Document, error) {
				return stubBranchDocument(docID, orgID, branchID, _stubBranchName), nil
			}

			return struct {
				DB      *DBMock
				Applier *EditApplierMock
				Branch  xid.ID
				Touched []Touched
				Applies int
				Err     error
			}{
				DB:      db,
				Applier: stubApplier(),
				Branch:  _stubBranchID,
				Touched: []Touched{{DocumentID: docID, BranchID: _stubBranchID}},
				Applies: 1,
			}
		}(),
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
			Touched: []Touched{{DocumentID: docID, BranchID: _stubMainBranchID}},
			Applies: 1,
		},
		"Applied": {
			DB:      stubDocumentDB(),
			Applier: stubApplier(),
			Touched: []Touched{{DocumentID: docID, BranchID: _stubMainBranchID}},
			Applies: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, c.Applier, nil), NameInsertBlock, `{}`)

			branch := c.Branch
			if branch.IsNil() {
				branch = _stubMainBranchID
			}

			err := inp.ApplyEdit(docID, branch, []edit.Operation{edit.Delete("a")})
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

// contentWithBlocks builds a DB whose branch content is the given
// top-level blocks, for the validators that read the document to decide.
func contentWithBlocks(blocks ...document.Block) *DBMock {
	return &DBMock{
		FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
			return &document.Document{
				ID:      _testDocID,
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
		"Error returned by db.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
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

			err := inp.ValidateAttrUpdate(_testDocID, _stubMainBranchID, c.UID, c.Attrs)
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
		"Error returned by db.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
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

			err := inp.ValidateMove(_testDocID, _stubMainBranchID, c.UID, c.Ref)
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
		FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
			return &document.Document{
				ID: _testDocID,
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
		"Error returned by db.FetchDocumentByBranchID": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, assert.AnError
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

			err := inp.ValidatePlacement(_testDocID, _stubMainBranchID, c.Ref, c.Block)
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

func Test_input_FetchDataSource(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)
	inp := testInput(d, NameQueryPrometheus, "")

	ds, err := inp.FetchDataSource(_testDataSourceID)
	require.NoError(t, err)
	require.NotNil(t, ds)
	assert.Equal(t, "prod", ds.Name)

	// an id the organisation owns nothing for comes back as a failure
	// rather than as a zero data source.
	_, err = inp.FetchDataSource(xid.New())
	require.Error(t, err)
}

func Test_input_FetchDataSources(t *testing.T) {
	t.Parallel()

	d := dataSourceDeps(t, datasource.TypePrometheus, nil)

	got, err := testInput(d, NameListDataSources, "").FetchDataSources()
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

			inp := testInput(testDeps(c.DB, nil, nil), NameUpdateBlockText, `{}`)

			b, err := inp.FetchDocumentBlock(_testDocID, c.Branch, c.UID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b.Text)
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

			doc, err := testInput(testDeps(c.DB, nil, nil), NameGetDocument, `{}`).FetchDocument(_testDocID)
			testutil.AssertEqualError(t, c.Err, err)

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
		DB     *DBMock
		Branch xid.ID
		Result string
		Err    error
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
			DB:     stubDocumentDB(),
			Branch: _unknownBranchID,
			Err:    fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Unknown branch of an unknown document": {
			DB: &DBMock{
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return nil, errutil.ErrNotFound
				},
			},
			Branch: _unknownBranchID,
			Err:    ErrUnknownDocument,
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
			Branch: _stubBranchID,
			Err:    fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _stubBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
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

			doc, err := testInput(testDeps(c.DB, nil, nil), NameGetDocument, `{}`).FetchBranch(_testDocID, c.Branch)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, doc.BranchName)
			assert.Equal(t, c.Branch, doc.BranchID)
		})
	}
}

func Test_input_unknownBranch(t *testing.T) {
	t.Parallel()

	// error: the listing itself failing is passed on.
	db := &DBMock{
		FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
			return nil, assert.AnError
		},
	}

	err := testInput(testDeps(db, nil, nil), NameGetDocument, `{}`).unknownBranch(_testDocID, _unknownBranchID)
	require.Error(t, err)

	// success: the branches the document does have travel with the refusal.
	err = testInput(testDeps(stubDocumentDB(), nil, nil), NameGetDocument, `{}`).unknownBranch(_testDocID, _unknownBranchID)
	assert.Equal(t, fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID), err)
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

			got, err := testInput(testDeps(c.DB, nil, nil), NameGetDocument, `{}`).FetchDocumentBranches(_testDocID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, got)
		})
	}
}

func Test_input_FetchDocumentContent(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB     *DBMock
		Branch xid.ID
		Result string
		Err    error
	}{
		"Error returned by db.FetchDocumentByBranchID": {
			DB:     stubContentDB(assert.AnError),
			Branch: _stubMainBranchID,
			Err:    assert.AnError,
		},
		"Unknown branch": {
			DB:     stubContentDB(nil),
			Branch: _unknownBranchID,
			Err:    assert.AnError,
		},
		"Default branch content": {
			DB:     stubContentDB(nil),
			Branch: _stubMainBranchID,
			Result: "hello",
		},
		"Another branch's content": {
			DB:     stubContentDB(nil),
			Branch: _stubBranchID,
			Result: "hello",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			content, err := testInput(testDeps(c.DB, nil, nil), NameGetDocument, `{}`).FetchDocumentContent(_testDocID, c.Branch)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			require.NotEmpty(t, content.Content.Content)
			assert.Equal(t, c.Result, content.Content.Content[0].Text)
			assert.Equal(t, _testDocID, content.DocumentID)
		})
	}
}

func Test_input_protectedBranch(t *testing.T) {
	t.Parallel()

	doc := &document.Document{ID: _testDocID, BranchName: document.DefaultBranch, Protected: true}

	// error: a listing that fails still says the branch is protected.
	db := &DBMock{
		FetchDocumentBranchesFunc: func(context.Context, xid.ID, string) ([]document.BranchSummary, error) {
			return nil, assert.AnError
		},
	}

	err := testInput(testDeps(db, nil, nil), NameInsertBlock, `{}`).protectedBranch(doc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), errBranchProtected.Error())

	// success: the unprotected branches are offered.
	err = testInput(testDeps(protectedDocumentDB(), nil, nil), NameInsertBlock, `{}`).protectedBranch(doc)
	assert.Equal(t, fmt.Errorf("branch main: %w; write to one of draft (%s)", errBranchProtected, _stubBranchID), err)
}

func Test_branchLabels(t *testing.T) {
	t.Parallel()

	mainID, draftID := xid.New(), xid.New()
	branches := []document.BranchSummary{
		{BranchID: mainID, BranchName: "main", Default: true, Protected: true},
		{BranchID: draftID, BranchName: "draft"},
	}

	assert.Equal(t, []string{"main (" + mainID.String() + ")", "draft (" + draftID.String() + ")"}, branchLabels(branches, nil))
	assert.Equal(t, []string{"draft (" + draftID.String() + ")"}, branchLabels(branches, func(b document.BranchSummary) bool { return !b.Protected }))
	assert.Empty(t, branchLabels(nil, nil))
}

func Test_input_FetchTagTree(t *testing.T) {
	t.Parallel()

	db := stubTagDB(nil)

	tree, err := testInput(testDeps(db, nil, nil), NameListTags, `{}`).FetchTagTree()
	require.NoError(t, err)
	require.Len(t, tree, 2)
	assert.Equal(t, _testTagID, tree[0].ID)

	// the tree is the session's own: scoped to its pair, so the hidden
	// flags are the user's.
	ff := db.FetchTagTreeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, "user", ff[0].UserID)

	_, err = testInput(testDeps(stubTagDB(assert.AnError), nil, nil), NameListTags, `{}`).FetchTagTree()
	require.Error(t, err)
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

			tg, err := testInput(testDeps(c.DB, nil, nil), NameDeleteTag, `{}`).FetchTag(c.ID)
			testutil.AssertEqualError(t, c.Err, err)

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

	tags, err := testInput(testDeps(db, nil, nil), NameGetDocument, `{}`).FetchBranchTags(_testDocID, _stubBranchID)
	require.NoError(t, err)
	assert.Equal(t, []tag.Tag{{ID: _testTagID}}, tags)

	_, err = testInput(testDeps(db, nil, nil), NameGetDocument, `{}`).FetchBranchTags(_testDocID, _unknownBranchID)
	require.Error(t, err)
}

func Test_input_CreateTag(t *testing.T) {
	t.Parallel()

	tg := tag.NewTag(tag.CreateInput{TagName: "Release", Color: "#3b82f6"}, "org", "user")

	db := &DBMock{}
	require.NoError(t, testInput(testDeps(db, nil, nil), NameCreateTag, `{}`).CreateTag(tg))

	ff := db.InsertTagCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, tg, ff[0].T)

	// the repository's own refusal of a duplicate name passes through
	// untouched: it already names the clash.
	db = &DBMock{
		InsertTagFunc: func(context.Context, tag.Tag) error { return tag.ErrDuplicateTagName },
	}
	assert.Equal(t, tag.ErrDuplicateTagName, testInput(testDeps(db, nil, nil), NameCreateTag, `{}`).CreateTag(tg))
}

func Test_input_UpdateTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB  *DBMock
		Err error
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
		"Updated": {DB: &DBMock{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := tag.UpdateInput{TagName: null.StringFrom("Release")}

			err := testInput(testDeps(c.DB, nil, nil), NameUpdateTag, `{}`).UpdateTag(_testTagID, inp)
			testutil.AssertEqualError(t, c.Err, err)

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
		DB  *DBMock
		Err error
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
		"Deleted": {DB: &DBMock{}},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := testInput(testDeps(c.DB, nil, nil), NameDeleteTag, `{}`).DeleteTag(_testTagID)
			testutil.AssertEqualError(t, c.Err, err)

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
		DB      *DBMock
		Branch  xid.ID
		Assigns int
		Touched []Touched
		Err     error
	}{
		"Unknown branch is refused before the write": {
			DB:     stubDocumentDB(),
			Branch: _unknownBranchID,
			Err:    fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
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
			DB:      stubDocumentDB(),
			Branch:  _stubBranchID,
			Assigns: 1,
			Touched: []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameAssignTag, `{}`)

			err := inp.AssignTag(_testDocID, c.Branch, _testTagID)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.AssignBranchTagCalls()
			require.Len(t, ff, c.Assigns)

			if c.Assigns > 0 {
				assert.Equal(t, "org", ff[0].OrganizationID)
				assert.Equal(t, _testDocID, ff[0].DocumentID)
				assert.Equal(t, c.Branch, ff[0].BranchID)
				assert.Equal(t, _testTagID, ff[0].TagID)
			}

			assert.Equal(t, c.Touched, inp.touched)
		})
	}
}

func Test_input_UnassignTag(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB        *DBMock
		Branch    xid.ID
		Tag       xid.ID
		Unassigns int
		Touched   []Touched
		Err       error
	}{
		"Unknown branch is refused before the write": {
			DB:     stubTagDB(nil),
			Branch: _unknownBranchID,
			Tag:    _testTagID,
			Err:    fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		// the repository treats a tag the branch does not carry as
		// nothing to do, so an id naming nothing is caught here.
		"Unknown tag": {
			DB:     stubTagDB(nil),
			Branch: _stubMainBranchID,
			Tag:    _unknownTagID,
			Err:    fmt.Errorf("tag %s: %w", _unknownTagID, ErrUnknownTag),
		},
		"Error returned by db.UnassignBranchTag": {
			DB: func() *DBMock {
				db := stubTagDB(nil)
				db.UnassignBranchTagFunc = func(context.Context, string, xid.ID, xid.ID, xid.ID) error {
					return assert.AnError
				}

				return db
			}(),
			Branch:    _stubMainBranchID,
			Tag:       _testTagID,
			Unassigns: 1,
			Err:       assert.AnError,
		},
		"Unassigned and recorded": {
			DB:        stubTagDB(nil),
			Branch:    _stubBranchID,
			Tag:       _testTagID,
			Unassigns: 1,
			Touched:   []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := testInput(testDeps(c.DB, nil, nil), NameUnassignTag, `{}`)

			err := inp.UnassignTag(_testDocID, c.Branch, c.Tag)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.UnassignBranchTagCalls()
			require.Len(t, ff, c.Unassigns)

			if c.Unassigns > 0 {
				assert.Equal(t, "org", ff[0].OrganizationID)
				assert.Equal(t, _testDocID, ff[0].DocumentID)
				assert.Equal(t, c.Branch, ff[0].BranchID)
				assert.Equal(t, c.Tag, ff[0].TagID)
			}

			assert.Equal(t, c.Touched, inp.touched)
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
		},
		"Kept in place": {
			DB:        stubTagDB(nil),
			Tag:       _testTagID,
			SortIndex: 0,
			Order:     []xid.ID{_testTagID, _otherTagID},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := testInput(testDeps(c.DB, nil, nil), NameMoveTag, `{}`).MoveTag(c.Tag, c.SortIndex)
			testutil.AssertEqualError(t, c.Err, err)

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

func Test_input_NotifyTagTreeChange(t *testing.T) {
	t.Parallel()

	// a session without a notifier silently no-ops.
	testInput(testDeps(nil, nil, nil), NameCreateTag, `{}`).NotifyTagTreeChange()

	d, tags := tagDeps(nil)
	testInput(d, NameCreateTag, `{}`).NotifyTagTreeChange()

	ff := tags.NotifyTreeChangeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
}

func Test_input_NotifyBranchTagsChange(t *testing.T) {
	t.Parallel()

	// a session without a notifier silently no-ops.
	testInput(testDeps(nil, nil, nil), NameAssignTag, `{}`).NotifyBranchTagsChange(_testDocID, _stubBranchID)

	d, tags := tagDeps(nil)
	testInput(d, NameAssignTag, `{}`).NotifyBranchTagsChange(_testDocID, _stubBranchID)

	ff := tags.NotifyBranchTagsChangeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, _testDocID, ff[0].DocumentID)
	assert.Equal(t, _stubBranchID, ff[0].BranchID)
}

func Test_input_hookInput(t *testing.T) {
	t.Parallel()

	inp := testInput(testDeps(nil, nil, nil), NameCreateHook, `{}`)

	got := inp.hookInput()
	require.NotNil(t, got)

	// the deployment has neither integration, and the input says so
	// the way the processors ask.
	assert.Same(t, inp.webchangeClient, got.ChangeDetection())

	_, err := got.Github(context.Background())
	assert.Equal(t, github.ErrNotConfigured, err)
}

func Test_input_FetchHooks(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.FetchDocumentHooksByBranchIDFunc = func(context.Context, xid.ID, string) ([]hook.Hook, error) {
		return nil, assert.AnError
	}

	cc := map[string]struct {
		DB     *DBMock
		Branch xid.ID
		Result []hook.Hook
		Err    error
	}{
		"Unknown branch": {
			DB:     stubHookDB(),
			Branch: _unknownBranchID,
			Err:    fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Error returned by db.FetchDocumentHooksByBranchID": {
			DB:     failing,
			Branch: _stubBranchID,
			Err:    assert.AnError,
		},
		"Branch without hooks": {
			DB:     stubHookDB(),
			Branch: _stubMainBranchID,
			Result: []hook.Hook{},
		},
		"Branch with hooks": {
			DB:     stubHookDB(),
			Branch: _stubBranchID,
			Result: []hook.Hook{*stubHook()},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			got, err := testInput(testDeps(c.DB, nil, nil), NameListHooks, `{}`).FetchHooks(_testDocID, c.Branch)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				assert.Nil(t, got)
				return
			}

			assert.Equal(t, c.Result, got)

			ff := c.DB.FetchDocumentHooksByBranchIDCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, c.Branch, ff[0].BranchID)
			assert.Equal(t, "org", ff[0].OrganizationID)
		})
	}
}

func Test_input_FetchHook(t *testing.T) {
	t.Parallel()

	// withHook answers the lookup with the given hook.
	withHook := func(hk *hook.Hook) *DBMock {
		return &DBMock{
			FetchDocumentHookFunc: func(context.Context, xid.ID, string) (*hook.Hook, error) {
				return hk, nil
			},
		}
	}

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
			DB:  withHook(otherDocument),
			ID:  _testHookID,
			Err: fmt.Errorf("hook %s on document %s: %w", _testHookID, _testDocID, errUnknownHook),
		},
		"Hook of a deleted document": {
			DB:  withHook(deletedDocument),
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

			hk, err := testInput(testDeps(c.DB, nil, nil), NameResetHook, `{}`).FetchHook(_testDocID, c.ID)
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

func Test_input_unknownHook(t *testing.T) {
	t.Parallel()

	err := testInput(testDeps(nil, nil, nil), NameResetHook, `{}`).unknownHook(_testDocID, _unknownHookID)

	// the message names the document the call addressed, so a hook that
	// exists elsewhere is not mistaken for a typo.
	assert.Equal(t, fmt.Errorf("hook %s on document %s: %w", _unknownHookID, _testDocID, errUnknownHook), err)
}

func Test_input_CreateHook(t *testing.T) {
	t.Parallel()

	scheduled := processor.Settings(`{"scale":"linear","duration":"custom","schedule":"` + _stubSchedule + `"}`)

	failing := stubHookDB()
	failing.InsertDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB       *DBMock
		Branch   xid.ID
		BlockUID string
		Type     hook.Type
		Settings processor.Settings
		Inserts  int
		Notify   int
		Touched  []Touched
		Err      error
	}{
		"Unknown branch is refused before anything is created": {
			DB:       stubHookDB(),
			Branch:   _unknownBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Err:      fmt.Errorf("branch %s: %w; the branches are main (%s), draft (%s)", _unknownBranchID, ErrUnknownBranch, _stubMainBranchID, _stubBranchID),
		},
		"Unknown block": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			BlockUID: "nope",
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Err:      fmt.Errorf("block %s: %w", "nope", errUnknownBlock),
		},
		"GitHub tracking without the app": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeGithubTracking,
			Settings: processor.Settings(`{"repository":"o/r","branch":"main","paths":["a"]}`),
			Err:      fmt.Errorf("%s: %w", hook.TypeGithubTracking, github.ErrNotConfigured),
		},
		"URL watcher without changedetection": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			Err:      fmt.Errorf("%s: %w", hook.TypeURLWatcher, webchange.ErrNotConfigured),
		},
		"Settings the processor refuses": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: processor.Settings(`{"scale":"bogus"}`),
			Err:      processor.ErrInvalidScaleType,
		},
		"Error returned by db.InsertDocumentHook": {
			DB:       failing,
			Branch:   _stubBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
			Err:      fmt.Errorf("insert: %w", assert.AnError),
		},
		"Created on the document": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
			Notify:   1,
			Touched:  []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
		"Created on a block": {
			DB:       stubHookDB(),
			Branch:   _stubBranchID,
			BlockUID: _stubContentUID,
			Type:     hook.TypeScheduledReminder,
			Settings: scheduled,
			Inserts:  1,
			Notify:   1,
			Touched:  []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameCreateHook, `{}`)

			hk, err := inp.CreateHook(_testDocID, c.Branch, c.BlockUID, c.Type, c.Settings)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.InsertDocumentHookCalls()
			require.Len(t, ff, c.Inserts)
			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)
			assert.Equal(t, c.Touched, inp.touched)

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.Equal(t, *hk, ff[0].Hk)
			assert.Equal(t, c.Type, hk.Type)
			assert.Equal(t, null.ValueFrom(_testDocID), hk.DocumentID)
			assert.Equal(t, null.ValueFrom(c.Branch), hk.BranchID)
			assert.Equal(t, null.StringFrom("org"), hk.OrganizationID)
			assert.Equal(t, null.NewString(c.BlockUID, c.BlockUID != ""), hk.BlockID)
			assert.Equal(t, c.Settings, hk.Settings)
			assert.Equal(t, "100", hk.Score.String())
			assert.NotEmpty(t, hk.State)
		})
	}
}

func Test_input_UpdateHook(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.UpdateDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	cc := map[string]struct {
		DB       *DBMock
		Settings processor.Settings
		Updates  int
		Notify   int
		Touched  []Touched
		Err      error
	}{
		"Settings the processor refuses": {
			DB:       stubHookDB(),
			Settings: processor.Settings(`{"scale":"bogus"}`),
			Err:      processor.ErrInvalidScaleType,
		},
		"Error returned by db.UpdateDocumentHook": {
			DB:       failing,
			Settings: processor.Settings(`{"scale":"linear","duration":"custom","schedule":"2031-01-01T00:00:00Z"}`),
			Updates:  1,
			Err:      fmt.Errorf("update: %w", assert.AnError),
		},
		"Updated and recorded": {
			DB:       stubHookDB(),
			Settings: processor.Settings(`{"scale":"linear","duration":"custom","schedule":"2031-01-01T00:00:00Z"}`),
			Updates:  1,
			Notify:   1,
			Touched:  []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameUpdateHook, `{}`)
			hk := stubHook()

			err := inp.UpdateHook(hk, c.Settings)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.UpdateDocumentHookCalls()
			require.Len(t, ff, c.Updates)
			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)
			assert.Equal(t, c.Touched, inp.touched)

			if err != nil {
				return
			}

			// the hook handed in is the one written, with the settings
			// replaced and a fresh state.
			assert.Equal(t, *hk, ff[0].Hk)
			assert.Equal(t, c.Settings, hk.Settings)
			assert.NotContains(t, string(hk.State), "2026-01-01")
			assert.True(t, hk.UpdatedAt.Valid)
		})
	}
}

func Test_input_ResetHook(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.UpdateDocumentHookFunc = func(context.Context, hook.Hook) error {
		return assert.AnError
	}

	invalid := stubHook()
	invalid.Settings = processor.Settings(`{"scale":"bogus"}`)

	cc := map[string]struct {
		DB      *DBMock
		Hook    *hook.Hook
		Updates int
		Notify  int
		Touched []Touched
		Err     error
	}{
		"Settings the processor refuses": {
			DB:   stubHookDB(),
			Hook: invalid,
			Err:  processor.ErrInvalidScaleType,
		},
		"Error returned by db.UpdateDocumentHook": {
			DB:      failing,
			Hook:    stubHook(),
			Updates: 1,
			Err:     fmt.Errorf("update: %w", assert.AnError),
		},
		"Reset and recorded": {
			DB:      stubHookDB(),
			Hook:    stubHook(),
			Updates: 1,
			Notify:  1,
			Touched: []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameResetHook, `{}`)

			err := inp.ResetHook(c.Hook)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.UpdateDocumentHookCalls()
			require.Len(t, ff, c.Updates)
			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)
			assert.Equal(t, c.Touched, inp.touched)

			if err != nil {
				return
			}

			// the settings stay; only the state starts over.
			assert.Equal(t, *c.Hook, ff[0].Hk)
			assert.Equal(t, stubHook().Settings, c.Hook.Settings)
			assert.NotContains(t, string(c.Hook.State), "2026-01-01")
		})
	}
}

func Test_input_DeleteHook(t *testing.T) {
	t.Parallel()

	failing := stubHookDB()
	failing.DeleteDocumentHookFunc = func(context.Context, xid.ID) error {
		return assert.AnError
	}

	// a url watcher tears its changedetection.io watcher down from its
	// state, and a state it cannot read stops the delete before the row.
	watcher := stubHook()
	watcher.Type = hook.TypeURLWatcher
	watcher.Settings = processor.Settings(`{"url":"https://example.com"}`)
	watcher.State = processor.State(`{`)

	cc := map[string]struct {
		DB      *DBMock
		Hook    *hook.Hook
		Deletes int
		Notify  int
		Touched []Touched
		Err     error
	}{
		"External teardown failed": {
			DB:   stubHookDB(),
			Hook: watcher,
			Err:  assert.AnError,
		},
		"Error returned by db.DeleteDocumentHook": {
			DB:      failing,
			Hook:    stubHook(),
			Deletes: 1,
			Err:     fmt.Errorf("delete: %w", assert.AnError),
		},
		"Deleted and recorded": {
			DB:      stubHookDB(),
			Hook:    stubHook(),
			Deletes: 1,
			Notify:  1,
			Touched: []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d, hooks := hookDeps(c.DB)
			inp := testInput(d, NameDeleteHook, `{}`)

			err := inp.DeleteHook(c.Hook)
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.DeleteDocumentHookCalls()
			require.Len(t, ff, c.Deletes)
			assert.Len(t, hooks.NotifyHooksChangeCalls(), c.Notify)
			assert.Equal(t, c.Touched, inp.touched)

			if err != nil {
				return
			}

			assert.Equal(t, _testHookID, ff[0].ID)
		})
	}
}

func Test_input_hookChanged(t *testing.T) {
	t.Parallel()

	branchless := stubHook()
	branchless.BranchID = null.Value[xid.ID]{}

	cc := map[string]struct {
		Hook     *hook.Hook
		Notifier bool
		Notify   int
		Touched  []Touched
	}{
		// a hook whose branch is gone has no branch to link to; the
		// notifier is told and decides for itself.
		"Hook whose branch is gone": {Hook: branchless, Notifier: true, Notify: 1},
		"Without a notifier":        {Hook: stubHook(), Touched: []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}}},
		"With a notifier": {
			Hook:     stubHook(),
			Notifier: true,
			Notify:   1,
			Touched:  []Touched{{DocumentID: _testDocID, BranchID: _stubBranchID}},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			d := testDeps(nil, nil, nil)
			hooks := &HookNotifierMock{}

			if c.Notifier {
				d.hooks = hooks
			}

			inp := testInput(d, NameResetHook, `{}`)
			inp.hookChanged(c.Hook)

			assert.Equal(t, c.Touched, inp.touched)

			ff := hooks.NotifyHooksChangeCalls()
			require.Len(t, ff, c.Notify)

			if c.Notify == 0 {
				return
			}

			assert.Equal(t, "org", ff[0].OrganizationID)
			assert.Equal(t, c.Hook.DocumentID, ff[0].DocumentID)
			assert.Equal(t, c.Hook.BranchID, ff[0].BranchID)
		})
	}
}
