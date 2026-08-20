package tools

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/assistant/block"
	"github.com/oxynote/oxynote/server/core/internal/assistant/edit"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// testInput builds an Input with don't-care dependencies.
func testInput() *Input {
	return &Input{
		log:     slog.New(slog.DiscardHandler),
		db:      &DBMock{},
		search:  &SearcherMock{},
		applier: &EditApplierMock{},
		tree:    &TreeNotifierMock{},
		orgID:   "org",
		userID:  "user",
	}
}

func Test_NewInput(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	inp := NewInput(slog.New(slog.DiscardHandler), db, &SearcherMock{}, &EditApplierMock{}, &TreeNotifierMock{}, "org", "user")

	require.NotNil(t, inp)
	assert.NotNil(t, inp.log)
	assert.Equal(t, db, inp.db)
	assert.NotNil(t, inp.search)
	assert.NotNil(t, inp.applier)
	assert.NotNil(t, inp.tree)
	assert.Equal(t, "org", inp.orgID)
	assert.Equal(t, "user", inp.userID)
}

// stubEditInput builds an Input whose db resolves documents to branchID
// and whose applier succeeds, for the edit-based tools.
func stubEditInput(db *DBMock, applier *EditApplierMock, tree *TreeNotifierMock) *Input {
	inp := &Input{
		log:     slog.New(slog.DiscardHandler),
		db:      db,
		applier: applier,
		orgID:   "org",
		userID:  "user",
	}

	if tree != nil {
		inp.tree = tree
	}

	return inp
}

// stubResolvingDB returns a DB mock whose FetchDocument succeeds
// with the given parent, or fails with err.
func stubResolvingDB(branchID xid.ID, parent null.Value[xid.ID], err error) *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			if err != nil {
				return nil, err
			}

			return &document.Document{
				Branch:         document.Branch{BranchID: branchID},
				ID:             id,
				ParentID:       parent,
				OrganizationID: orgID,
			}, nil
		},
	}
}

// stubOKApplier returns an applier that reports one applied op.
func stubOKApplier() *EditApplierMock {
	return &EditApplierMock{
		ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
			return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
		},
	}
}

func stubNamedDB(name string, err error) *DBMock {
	return &DBMock{
		FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
			if err != nil {
				return nil, err
			}

			return &document.Document{
				Branch:         document.Branch{DocumentName: name},
				ID:             id,
				OrganizationID: orgID,
			}, nil
		},
	}
}

func Test_Input_resolveDoc(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB         *DBMock
		DocumentID string
		Result     docRef
		Err        error
	}{
		"Invalid document id": {
			DB:         &DBMock{},
			DocumentID: "nope",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:         stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Successful resolve": {
			DB:         stubResolvingDB(branchID, null.Value[xid.ID]{}, nil),
			DocumentID: docID.String(),
			Result: docRef{
				DocumentID: docID.String(),
				BranchID:   branchID.String(),
			},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := stubEditInput(c.DB, nil, nil)

			ref, err := inp.resolveDoc(context.Background(), c.DocumentID)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, ref)
		})
	}
}

func Test_Input_applyEdit(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	stubDB := func(err error) *DBMock {
		return &DBMock{
			FetchDocumentFunc: func(_ context.Context, id xid.ID, orgID, _ string) (*document.Document, error) {
				if err != nil {
					return nil, err
				}

				return &document.Document{
					Branch:         document.Branch{BranchID: branchID},
					ID:             id,
					OrganizationID: orgID,
				}, nil
			},
		}
	}

	cc := map[string]struct {
		DB         *DBMock
		Applier    *EditApplierMock
		DocumentID string
		ApplyCalls int
		RespJSON   string
		Err        error
	}{
		"Invalid document id": {
			DB:         stubDB(nil),
			Applier:    &EditApplierMock{},
			DocumentID: "not-an-xid",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchDocument": {
			DB:         stubDB(assert.AnError),
			Applier:    &EditApplierMock{},
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Error returned by EditApplier.Apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{}, assert.AnError
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			Err:        assert.AnError,
		},
		"Partial failure surfaces per-op errors": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{
						Applied: 1,
						Errors:  []edit.OpError{{Index: 1, Message: "uid not found"}},
					}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[{"index":1,"message":"uid not found"}]}`,
		},
		"Successful apply": {
			DB: stubDB(nil),
			Applier: &EditApplierMock{
				ApplyFunc: func(_ context.Context, _, _ string, _ []edit.Operation) (edit.Result, error) {
					return edit.Result{Applied: 1, Errors: []edit.OpError{}}, nil
				},
			},
			DocumentID: docID.String(),
			ApplyCalls: 1,
			RespJSON:   `{"applied":1,"errors":[]}`,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{
				log:     slog.New(slog.DiscardHandler),
				db:      c.DB,
				applier: c.Applier,
				orgID:   "org",
				userID:  "user",
			}

			res, err := inp.applyEdit(context.Background(), c.DocumentID, []edit.Operation{edit.Delete("target")})
			testutil.AssertEqualError(t, c.Err, err)

			ff := c.Applier.ApplyCalls()
			require.Len(t, ff, c.ApplyCalls)

			if c.ApplyCalls > 0 {
				assert.Equal(t, docID.String(), ff[0].DocumentID)
				assert.Equal(t, branchID.String(), ff[0].BranchID)
				assert.Len(t, ff[0].Ops, 1)
			}

			if err != nil {
				return
			}

			assert.JSONEq(t, c.RespJSON, res)
		})
	}
}

func Test_Input_validatePlacement(t *testing.T) {
	docID := xid.New()

	// a titled_code block is a macro internal: legal only inside a
	// split_doc, so it forces the placement check to look at the
	// reference block's position.
	macro := block.Block{
		Type:  block.BlockTitledCode,
		Text:  "x",
		Attrs: document.Attributes{"title": "t"},
	}

	cc := map[string]struct {
		Block      block.Block
		DocumentID string
		Content    document.RootBlock
		FetchErr   error
		Fetches    int
		Err        error
	}{
		"Invalid block": {
			Block:      block.Block{Type: "wibble"},
			DocumentID: docID.String(),
			Err:        assert.AnError,
		},
		"Root-legal block skips the content fetch": {
			Block:      block.Block{Type: block.BlockParagraph, Text: "hi"},
			DocumentID: docID.String(),
		},
		"Invalid document id": {
			Block:      macro,
			DocumentID: "nope",
			Err:        assert.AnError,
		},
		"Error returned by db.FetchMainBranchContent": {
			Block:      macro,
			DocumentID: docID.String(),
			FetchErr:   assert.AnError,
			Fetches:    1,
			Err:        assert.AnError,
		},
		"Macro internal beside a root block": {
			Block:      macro,
			DocumentID: docID.String(),
			Content: document.RootBlock{
				Type:    document.BlockNodeDoc,
				Content: []document.Block{pmBlock(document.BlockNodeParagraph, "r", nil)},
			},
			Fetches: 1,
			Err:     assert.AnError,
		},
		"Macro internal beside a nested block": {
			Block:      macro,
			DocumentID: docID.String(),
			Content: document.RootBlock{
				Type:    document.BlockNodeDoc,
				Content: []document.Block{pmBlock(document.BlockNodeParagraph, "other", nil)},
			},
			Fetches: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := &DBMock{
				FetchMainBranchContentFunc: func(_ context.Context, _ xid.ID, _ string) (document.Content, error) {
					return document.Content{Content: c.Content}, c.FetchErr
				},
			}

			inp := stubEditInput(db, nil, nil)

			err := inp.validatePlacement(context.Background(), c.DocumentID, "r", c.Block)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, db.FetchMainBranchContentCalls(), c.Fetches)
		})
	}
}

func Test_Input_notifyTreeChange(t *testing.T) {
	t.Parallel()

	// nil notifier is a silent no-op.
	inp := stubEditInput(&DBMock{}, nil, nil)
	inp.notifyTreeChange(null.Value[xid.ID]{})

	// configured notifier receives the parent.
	tree := &TreeNotifierMock{}
	inp = stubEditInput(&DBMock{}, nil, tree)

	parent := null.ValueFrom(xid.New())
	inp.notifyTreeChange(parent)

	ff := tree.NotifyTreeChangeCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, "org", ff[0].OrganizationID)
	assert.Equal(t, parent, ff[0].ParentID)
}

func Test_Input_notifyTreeChangeForDocument(t *testing.T) {
	docID := xid.New()
	branchID := xid.New()

	cc := map[string]struct {
		DB          *DBMock
		DocumentID  string
		NotifyCalls int
	}{
		"Invalid document id skips": {
			DB:          &DBMock{},
			DocumentID:  "nope",
			NotifyCalls: 0,
		},
		"Fetch failure skips": {
			DB:          stubResolvingDB(branchID, null.Value[xid.ID]{}, assert.AnError),
			DocumentID:  docID.String(),
			NotifyCalls: 0,
		},
		"Successful notify": {
			DB:          stubResolvingDB(branchID, null.ValueFrom(xid.New()), nil),
			DocumentID:  docID.String(),
			NotifyCalls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tree := &TreeNotifierMock{}
			inp := stubEditInput(c.DB, nil, tree)

			inp.notifyTreeChangeForDocument(context.Background(), c.DocumentID)

			assert.Len(t, tree.NotifyTreeChangeCalls(), c.NotifyCalls)
		})
	}
}

func Test_Input_parseToolArgs(t *testing.T) {
	t.Parallel()

	inp := &Input{log: slog.New(slog.DiscardHandler)}

	// valid args populate dst.
	var dst struct {
		Name string `json:"name"`
	}

	inp.parseToolArgs(json.RawMessage(`{"name":"x"}`), &dst)
	assert.Equal(t, "x", dst.Name)

	// malformed args leave dst zeroed.
	dst.Name = ""

	inp.parseToolArgs(json.RawMessage(`{broken`), &dst)
	assert.Empty(t, dst.Name)
}

func Test_Input_lookupDocumentName(t *testing.T) {
	docID := xid.New().String()

	cc := map[string]struct {
		DB     *DBMock
		ID     string
		Result string
	}{
		"Invalid xid": {
			DB:     &DBMock{},
			ID:     "not-an-xid",
			Result: "",
		},
		"Error returned by db.FetchDocument": {
			DB:     stubNamedDB("", assert.AnError),
			ID:     docID,
			Result: "",
		},
		"Nil document": {
			DB: &DBMock{
				FetchDocumentFunc: func(_ context.Context, _ xid.ID, _, _ string) (*document.Document, error) {
					return nil, nil //nolint:nilnil // the nil-doc guard is the path under test
				},
			},
			ID:     docID,
			Result: "",
		},
		"Successful lookup": {
			DB:     stubNamedDB("Cat Facts", nil),
			ID:     docID,
			Result: "Cat Facts",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := &Input{log: slog.New(slog.DiscardHandler), db: c.DB, orgID: "org"}

			assert.Equal(t, c.Result, inp.lookupDocumentName(context.Background(), c.ID))
		})
	}
}

func Test_Input_subject(t *testing.T) {
	docID := xid.New().String()

	cc := map[string]struct {
		DB      *DBMock
		Args    string
		Lookups int
		Result  string
	}{
		"No document id skips the lookup": {
			DB:     &DBMock{},
			Args:   `{}`,
			Result: "document",
		},
		"Unresolvable document falls back": {
			DB:      stubNamedDB("", assert.AnError),
			Args:    `{"document_id":"` + docID + `"}`,
			Lookups: 1,
			Result:  "document",
		},
		"Resolvable document is named": {
			DB:      stubNamedDB("Runbook", nil),
			Args:    `{"document_id":"` + docID + `"}`,
			Lookups: 1,
			Result:  "Runbook",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			inp := stubEditInput(c.DB, nil, nil)

			assert.Equal(t, c.Result, inp.subject(context.Background(), json.RawMessage(c.Args)))
			assert.Len(t, c.DB.FetchDocumentCalls(), c.Lookups)
		})
	}
}

func Test_Input_summarize(t *testing.T) {
	t.Parallel()

	docID := xid.New()
	inp := stubEditInput(stubNamedDB("Runbook", nil), nil, nil)

	// a write that names its target carries the id and resolved name.
	got := inp.summarize(
		context.Background(),
		NameDeleteBlock,
		json.RawMessage(`{"document_id":"`+docID.String()+`"}`),
		func(subject string) string { return "Delete a block in " + subject },
	)

	assert.Equal(t, ConfirmActionSummary{
		Tool:         string(NameDeleteBlock),
		DocumentID:   docID.String(),
		DocumentName: "Runbook",
		Summary:      "Delete a block in Runbook",
	}, got)

	// args without a document id leave the target empty.
	got = inp.summarize(
		context.Background(),
		NameDeleteBlock,
		json.RawMessage(`{}`),
		func(subject string) string { return "Delete a block in " + subject },
	)

	assert.Equal(t, ConfirmActionSummary{
		Tool:    string(NameDeleteBlock),
		Summary: "Delete a block in document",
	}, got)
}

func Test_Input_documentID(t *testing.T) {
	t.Parallel()

	inp := &Input{log: slog.New(slog.DiscardHandler)}

	assert.Equal(t, "d1", inp.documentID(json.RawMessage(`{"document_id":"d1"}`)))
	assert.Empty(t, inp.documentID(json.RawMessage(`{}`)))
}

func Test_Input_blockType(t *testing.T) {
	t.Parallel()

	inp := &Input{log: slog.New(slog.DiscardHandler)}

	assert.Equal(t, "callout", inp.blockType(json.RawMessage(`{"block":{"type":"callout"}}`)))
	assert.Empty(t, inp.blockType(json.RawMessage(`{}`)))
}
