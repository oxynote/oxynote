package manager

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStoredHook makes the DB mock answer a hook fetch with a copy of the
// hook, or fail with fetchErr.
func stubStoredHook(db *DBMock, hk hook.Hook, fetchErr error) *DBMock {
	db.FetchDocumentHookFunc = func(context.Context, xid.ID, string) (*hook.Hook, error) {
		if fetchErr != nil {
			return nil, fetchErr
		}

		return &hk, nil
	}

	return db
}

func Test_Manager_CreateHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		Type     hook.Type
		BlockID  null.String
		Settings processor.Settings
		CD       bool
		// Unreachable points the changedetection client at a closed port.
		Unreachable bool
		// OtherDocument has the branch belong to another document.
		OtherDocument bool
		Protected     bool
		FlushErr      error
		FetchErr      error
		BeginErr      error
		Tx            *TxMock
		Flushes       int
		Fetches       int
		Begins        int
		Inserts       int
		Records       int
		Commits       int
		Rollbacks     int
		Watchers      []string
		Err           error
	}{
		"Error returned by flusher.Flush": {
			Type:     hook.TypeScheduledReminder,
			Settings: processor.Settings(`{}`),
			FlushErr: assert.AnError,
			Tx:       &TxMock{},
			Flushes:  1,
			Err:      assert.AnError,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			Type:     hook.TypeScheduledReminder,
			Settings: processor.Settings(`{}`),
			FetchErr: sql.ErrNoRows,
			Tx:       &TxMock{},
			Flushes:  1,
			Fetches:  1,
			Err:      sql.ErrNoRows,
		},
		"Branch of another document is refused": {
			Type:          hook.TypeScheduledReminder,
			Settings:      processor.Settings(`{}`),
			OtherDocument: true,
			Tx:            &TxMock{},
			Flushes:       1,
			Fetches:       1,
			Err:           hook.ErrBranchMismatch,
		},
		"Protected branch is refused": {
			Type:      hook.TypeScheduledReminder,
			Settings:  processor.Settings(`{}`),
			Protected: true,
			Tx:        &TxMock{},
			Flushes:   1,
			Fetches:   1,
			Err:       document.ErrBranchProtected,
		},
		"Block the branch does not hold is refused": {
			Type:     hook.TypeScheduledReminder,
			BlockID:  null.StringFrom("nope"),
			Settings: processor.Settings(`{}`),
			Tx:       &TxMock{},
			Flushes:  1,
			Fetches:  1,
			Err:      hook.ErrBlockNotFound,
		},
		"Unknown type is refused": {
			Type:     "bogus",
			Settings: processor.Settings(`{}`),
			Tx:       &TxMock{},
			Flushes:  1,
			Fetches:  1,
			Err:      hook.ErrInvalidType,
		},
		"Hook that cannot check its target is refused": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			Tx:       &TxMock{},
			Flushes:  1,
			Fetches:  1,
			Err:      errutil.New(http.StatusUnprocessableEntity, "document_hook.unconfigured", "the integration the hook needs is not configured"),
		},
		"Service the hook checks is unreachable": {
			Type:        hook.TypeURLWatcher,
			Settings:    processor.Settings(`{"url":"https://example.com"}`),
			Unreachable: true,
			Tx:          &TxMock{},
			Flushes:     1,
			Fetches:     1,
			Err:         hook.ErrUpstreamUnavailable,
		},
		"Error returned by db.BeginTx": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			BeginErr: assert.AnError,
			Tx:       &TxMock{},
			Flushes:  1,
			Fetches:  1,
			Begins:   1,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		"Error returned by tx.RecordDocumentBranchHistoryEntry": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			Tx: &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return assert.AnError
				},
			},
			Flushes:   1,
			Fetches:   1,
			Begins:    1,
			Inserts:   1,
			Records:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		"Error returned by tx.Commit": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			Tx: &TxMock{
				CommitFunc: func() error {
					return assert.AnError
				},
			},
			Flushes:   1,
			Fetches:   1,
			Begins:    1,
			Inserts:   1,
			Records:   1,
			Commits:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		// the hook was not stored, so the watcher it created is torn down.
		"Error returned by tx.InsertDocumentHook": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			Tx: &TxMock{
				InsertDocumentHookFunc: func(context.Context, hook.Hook) error {
					return assert.AnError
				},
			},
			Flushes:   1,
			Fetches:   1,
			Begins:    1,
			Inserts:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		"Successful creation": {
			Type:      hook.TypeURLWatcher,
			BlockID:   null.StringFrom("b1"),
			Settings:  processor.Settings(`{"url":"https://example.com"}`),
			CD:        true,
			Tx:        &TxMock{},
			Flushes:   1,
			Fetches:   1,
			Begins:    1,
			Inserts:   1,
			Records:   1,
			Commits:   1,
			Rollbacks: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			var (
				cd *fakeChangeDetection
				wc *webchange.Client
			)

			if c.CD {
				cd, wc = newFakeChangeDetection(t)
			}

			if c.Unreachable {
				wc = webchange.NewClient("http://127.0.0.1:1", "key")
			}

			documentID := xid.New()

			doc := stubDocument()
			doc.Protected = c.Protected

			if !c.OtherDocument {
				doc.ID = documentID
			}

			dbm := &DBMock{
				BeginTxFunc: testutil.StubBeginTx(c.Tx, c.BeginErr),
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return doc, c.FetchErr
				},
			}
			man := newTestManager(t, dbm, &fakePublisher{}, wc)

			if c.FlushErr != nil {
				man.flusher = &FlusherMock{
					FlushFunc: func(context.Context, xid.ID, xid.ID) error {
						return c.FlushErr
					},
				}
			}

			var changes []hook.Hook

			man.BindHookChange(func(h hook.Hook) { changes = append(changes, h) })

			hk, err := man.CreateHook(context.Background(), hook.CreateInput{
				Type:     c.Type,
				BranchID: branchID,
				BlockID:  c.BlockID,
				Settings: c.Settings,
			}, documentID, "org-1", "u1")
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, man.flusher.(*FlusherMock).FlushCalls(), c.Flushes)
			assert.Len(t, dbm.FetchDocumentByBranchIDCalls(), c.Fetches)
			assert.Len(t, dbm.BeginTxCalls(), c.Begins)
			require.Len(t, c.Tx.InsertDocumentHookCalls(), c.Inserts)
			assert.Len(t, c.Tx.RecordDocumentBranchHistoryEntryCalls(), c.Records)
			assert.Len(t, c.Tx.CommitCalls(), c.Commits)
			assert.Len(t, c.Tx.RollbackCalls(), c.Rollbacks)

			if cd != nil {
				assert.Equal(t, c.Watchers, cd.deletedWatchers())
			}

			if err != nil {
				assert.Nil(t, hk)
				assert.Empty(t, changes)

				return
			}

			assert.Equal(t, []hook.Hook{*hk}, changes)
			assert.Equal(t, *hk, c.Tx.InsertDocumentHookCalls()[0].Hk)
			assert.Equal(t, null.ValueFrom(documentID), hk.DocumentID)
			assert.Equal(t, null.ValueFrom(branchID), hk.BranchID)
			assert.Equal(t, c.BlockID, hk.BlockID)
			assert.Equal(t, null.StringFrom("org-1"), hk.OrganizationID)
			assert.Equal(t, processor.StatusActive, hk.Status)
			assert.True(t, hk.State.Valid)

			ff := dbm.FetchDocumentByBranchIDCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, branchID, ff[0].BranchID)
			assert.Equal(t, "org-1", ff[0].OrganizationID)

			flushes := man.flusher.(*FlusherMock).FlushCalls()
			require.Len(t, flushes, 1)
			assert.Equal(t, documentID, flushes[0].DocumentID)
			assert.Equal(t, branchID, flushes[0].BranchID)

			rr := c.Tx.RecordDocumentBranchHistoryEntryCalls()
			require.Len(t, rr, 1)
			assert.Equal(t, null.StringFrom("u1"), rr[0].By)
		})
	}
}

func Test_Manager_UpdateHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()
	settings := processor.Settings(`{"url":"https://example.com"}`)

	cc := map[string]struct {
		// Held has another write hold the hook's lock until the update
		// gives up.
		Held          bool
		Branchless    bool
		OtherDocument bool
		Unreachable   bool
		Settings      processor.Settings
		FetchErr      error
		FlushErr      error
		BeginErr      error
		UpdateErr     error
		RecordErr     error
		CommitErr     error
		Fetches       int
		Flushes       int
		Begins        int
		Updates       int
		Records       int
		Commits       int
		Rollbacks     int
		Watchers      []string
		Err           error
	}{
		"Hook another write holds": {
			Held:     true,
			Settings: settings,
			Err:      context.DeadlineExceeded,
		},
		"Error returned by db.FetchDocumentHook": {
			Settings: settings,
			FetchErr: sql.ErrNoRows,
			Fetches:  1,
			Err:      sql.ErrNoRows,
		},
		"Hook of another document": {
			Settings:      settings,
			OtherDocument: true,
			Fetches:       1,
			Err:           hook.ErrHookMismatch,
		},
		"Error returned by flusher.Flush": {
			Settings: settings,
			FlushErr: assert.AnError,
			Fetches:  1,
			Flushes:  1,
			Err:      assert.AnError,
		},
		"Settings the processor refuses": {
			Settings: processor.Settings(`{"url":"ftp://example.com"}`),
			Fetches:  1,
			Flushes:  1,
			Err:      processor.ErrInvalidURL,
		},
		"Service the hook checks is unreachable": {
			Settings:    settings,
			Unreachable: true,
			Fetches:     1,
			Flushes:     1,
			Err:         hook.ErrUpstreamUnavailable,
		},
		"Error returned by db.BeginTx tears down the setup": {
			Settings: settings,
			BeginErr: assert.AnError,
			Fetches:  1,
			Flushes:  1,
			Begins:   1,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		"Error returned by tx.UpdateDocumentHook tears down the setup": {
			Settings:  settings,
			UpdateErr: assert.AnError,
			Fetches:   1,
			Flushes:   1,
			Begins:    1,
			Updates:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		"Error returned by tx.RecordDocumentBranchHistoryEntry tears down the setup": {
			Settings:  settings,
			RecordErr: assert.AnError,
			Fetches:   1,
			Flushes:   1,
			Begins:    1,
			Updates:   1,
			Records:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		"Error returned by tx.Commit tears down the setup": {
			Settings:  settings,
			CommitErr: assert.AnError,
			Fetches:   1,
			Flushes:   1,
			Begins:    1,
			Updates:   1,
			Records:   1,
			Commits:   1,
			Watchers:  []string{"w-new"},
			Rollbacks: 1,
			Err:       assert.AnError,
		},
		"Hook without a branch has no history": {
			Settings:   settings,
			Branchless: true,
			Fetches:    1,
			Begins:     1,
			Updates:    1,
			Commits:    1,
			Rollbacks:  1,
		},
		"Successful update": {
			Settings:  settings,
			Fetches:   1,
			Flushes:   1,
			Begins:    1,
			Updates:   1,
			Records:   1,
			Commits:   1,
			Rollbacks: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			// the stored hook is a url watcher copy that the update sets up,
			// so a failure after it has a watcher to tear down.
			stored := urlWatcherHook(branchID)
			stored.State = null.Value[processor.State]{}

			if c.Branchless {
				stored.BranchID = null.Value[xid.ID]{}
			}

			tx := &TxMock{
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.UpdateErr
				},
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return c.RecordErr
				},
				CommitFunc: func() error {
					return c.CommitErr
				},
			}

			cd, wc := newFakeChangeDetection(t)
			if c.Unreachable {
				wc = webchange.NewClient("http://127.0.0.1:1", "key")
			}

			dbm := stubStoredHook(&DBMock{BeginTxFunc: testutil.StubBeginTx(tx, c.BeginErr)}, stored, c.FetchErr)
			man := newTestManager(t, dbm, &fakePublisher{}, wc)

			if c.FlushErr != nil {
				man.flusher = &FlusherMock{
					FlushFunc: func(context.Context, xid.ID, xid.ID) error {
						return c.FlushErr
					},
				}
			}

			var changes []hook.Hook

			man.BindHookChange(func(h hook.Hook) { changes = append(changes, h) })

			ctx := context.Background()

			if c.Held {
				unlock, err := man.hookMu.Lock(ctx, stored.ID)
				require.NoError(t, err)

				defer unlock()

				var cancel context.CancelFunc

				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			documentID := stored.DocumentID.V
			if c.OtherDocument {
				documentID = xid.New()
			}

			hk, err := man.UpdateHook(ctx, stored.ID, documentID, "org-1", hook.UpdateInput{Settings: c.Settings}, "u1")
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, dbm.FetchDocumentHookCalls(), c.Fetches)
			assert.Len(t, man.flusher.(*FlusherMock).FlushCalls(), c.Flushes)
			assert.Len(t, dbm.BeginTxCalls(), c.Begins)
			assert.Len(t, tx.UpdateDocumentHookCalls(), c.Updates)
			assert.Len(t, tx.CommitCalls(), c.Commits)
			assert.Len(t, tx.RollbackCalls(), c.Rollbacks)
			assert.Equal(t, c.Watchers, cd.deletedWatchers())

			rr := tx.RecordDocumentBranchHistoryEntryCalls()
			require.Len(t, rr, c.Records)

			for _, r := range rr {
				assert.Equal(t, branchID, r.BranchID)
				assert.Equal(t, "org-1", r.OrganizationID)
				assert.Equal(t, null.StringFrom("u1"), r.By)
				assert.False(t, r.Boundary)
			}

			if err != nil {
				assert.Nil(t, hk)
				assert.Empty(t, changes)

				return
			}

			ll := dbm.FetchDocumentHookCalls()
			require.Len(t, ll, 1)
			assert.Equal(t, stored.ID, ll[0].ID)
			assert.Equal(t, "org-1", ll[0].OrganizationID)

			assert.Equal(t, *hk, tx.UpdateDocumentHookCalls()[0].Hk)
			assert.Equal(t, c.Settings, hk.Settings)
			assert.True(t, hk.State.Valid)
			assert.True(t, hk.UpdatedAt.Valid)
			assert.Len(t, tx.CommitCalls(), 1)
			assert.Equal(t, []hook.Hook{*hk}, changes)
		})
	}
}

func Test_Manager_DeleteHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		// Held has another write hold the hook's lock until the delete
		// gives up.
		Held          bool
		Hook          func(*testing.T) hook.Hook
		OtherDocument bool
		// Protected has the hook's branch protected.
		Protected bool
		// CDURL points changedetection at this address.
		CDURL          string
		FetchErr       error
		BranchFetchErr error
		FlushErr       error
		BeginErr       error
		DelErr         error
		RecordErr      error
		CommitErr      error
		Fetches        int
		BranchFetches  int
		Flushes        int
		Begins         int
		Deletes        int
		Records        int
		Commits        int
		Rollbacks      int
		Err            error
	}{
		"Hook another write holds": {
			Held: true,
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			Err: context.DeadlineExceeded,
		},
		"Error returned by db.FetchDocumentHook": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			FetchErr: sql.ErrNoRows,
			Fetches:  1,
			Err:      sql.ErrNoRows,
		},
		"Hook of another document": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			OtherDocument: true,
			Fetches:       1,
			Err:           hook.ErrHookMismatch,
		},
		"Error returned by db.FetchDocumentByBranchID": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			BranchFetchErr: sql.ErrNoRows,
			Fetches:        1,
			BranchFetches:  1,
			Err:            sql.ErrNoRows,
		},
		"Hook on a protected branch is refused": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			Protected:     true,
			Fetches:       1,
			BranchFetches: 1,
			Err:           document.ErrBranchProtected,
		},
		"Error returned by flusher.Flush": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			FlushErr:      assert.AnError,
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Err:           assert.AnError,
		},
		// the row goes only once the resource it describes is gone, so a
		// failed teardown keeps it.
		"Unreachable changedetection keeps the hook": {
			Hook: func(*testing.T) hook.Hook {
				return urlWatcherHook(branchID)
			},
			CDURL:         "http://127.0.0.1:1",
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Err:           hook.ErrUpstreamUnavailable,
		},
		// a stored state that does not decode is no service's failure.
		"Malformed state keeps the hook": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.ValueFrom(processor.State(`{`))

				return hk
			},
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Err:           assert.AnError,
		},
		"Error returned by db.BeginTx": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			BeginErr:      assert.AnError,
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Err:           assert.AnError,
		},
		"Error returned by tx.DeleteDocumentHook": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			DelErr:        assert.AnError,
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Deletes:       1,
			Rollbacks:     1,
			Err:           assert.AnError,
		},
		"Error returned by tx.RecordDocumentBranchHistoryEntry": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			RecordErr:     assert.AnError,
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Deletes:       1,
			Records:       1,
			Rollbacks:     1,
			Err:           assert.AnError,
		},
		"Error returned by tx.Commit": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			CommitErr:     assert.AnError,
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Deletes:       1,
			Records:       1,
			Commits:       1,
			Rollbacks:     1,
			Err:           assert.AnError,
		},
		"Hook never set up is deleted": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.Value[processor.State]{}

				return hk
			},
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Deletes:       1,
			Records:       1,
			Commits:       1,
			Rollbacks:     1,
		},
		"Hook without a branch has no history": {
			Hook: func(t *testing.T) hook.Hook {
				hk := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				hk.BranchID = null.Value[xid.ID]{}

				return hk
			},
			Fetches:   1,
			Begins:    1,
			Deletes:   1,
			Commits:   1,
			Rollbacks: 1,
		},
		"Successful deletion": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			Fetches:       1,
			BranchFetches: 1,
			Flushes:       1,
			Begins:        1,
			Deletes:       1,
			Records:       1,
			Commits:       1,
			Rollbacks:     1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hk := c.Hook(t)

			tx := &TxMock{
				DeleteDocumentHookFunc: func(context.Context, xid.ID) error {
					return c.DelErr
				},
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) error {
					return c.RecordErr
				},
				CommitFunc: func() error {
					return c.CommitErr
				},
			}

			var wc *webchange.Client

			if c.CDURL != "" {
				wc = webchange.NewClient(c.CDURL, "key")
			}

			dbm := stubStoredHook(&DBMock{
				BeginTxFunc: testutil.StubBeginTx(tx, c.BeginErr),
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return &document.Document{Protected: c.Protected}, c.BranchFetchErr
				},
			}, hk, c.FetchErr)
			man := newTestManager(t, dbm, &fakePublisher{}, wc)

			if c.FlushErr != nil {
				man.flusher = &FlusherMock{
					FlushFunc: func(context.Context, xid.ID, xid.ID) error {
						return c.FlushErr
					},
				}
			}

			var changes []hook.Hook

			man.BindHookChange(func(h hook.Hook) { changes = append(changes, h) })

			ctx := context.Background()

			if c.Held {
				unlock, err := man.hookMu.Lock(ctx, hk.ID)
				require.NoError(t, err)

				defer unlock()

				var cancel context.CancelFunc

				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			documentID := hk.DocumentID.V
			if c.OtherDocument {
				documentID = xid.New()
			}

			err := man.DeleteHook(ctx, hk.ID, documentID, "org-1", "u1")
			testutil.AssertEqualError(t, c.Err, err)

			// only a service the hook could not reach is reported as one.
			if c.Err != hook.ErrUpstreamUnavailable { //nolint:err113,errorlint // the sentinel is returned as is
				assert.NotEqual(t, hook.ErrUpstreamUnavailable, err)
			}

			assert.Len(t, dbm.FetchDocumentHookCalls(), c.Fetches)
			assert.Len(t, dbm.FetchDocumentByBranchIDCalls(), c.BranchFetches)
			assert.Len(t, man.flusher.(*FlusherMock).FlushCalls(), c.Flushes)
			assert.Len(t, dbm.BeginTxCalls(), c.Begins)

			ff := tx.DeleteDocumentHookCalls()
			require.Len(t, ff, c.Deletes)
			assert.Len(t, tx.RecordDocumentBranchHistoryEntryCalls(), c.Records)
			assert.Len(t, tx.CommitCalls(), c.Commits)
			assert.Len(t, tx.RollbackCalls(), c.Rollbacks)

			if err != nil {
				assert.Empty(t, changes)

				return
			}

			assert.Equal(t, hk.ID, ff[0].ID)
			assert.Len(t, changes, 1)

			if hk.BranchID.Valid {
				flushes := man.flusher.(*FlusherMock).FlushCalls()
				require.Len(t, flushes, 1)
				assert.Equal(t, hk.BranchID.V, flushes[0].BranchID)
			}
		})
	}
}

func Test_Manager_ResetHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		// Held has another write hold the hook's lock until the reset
		// gives up.
		Held          bool
		Hook          func(*testing.T) hook.Hook
		OtherDocument bool
		CD            bool
		Unreachable   bool
		FetchErr      error
		UpdateErr     error
		Fetches       int
		Updates       int
		MaintFetches  int
		Status        processor.Status
		Published     []notification.Code
		Watchers      []string
		Err           error
	}{
		"Hook another write holds": {
			Held: true,
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			Err: context.DeadlineExceeded,
		},
		"Error returned by db.FetchDocumentHook": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			FetchErr: sql.ErrNoRows,
			Fetches:  1,
			Err:      sql.ErrNoRows,
		},
		"Hook of another document": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			OtherDocument: true,
			Fetches:       1,
			Err:           hook.ErrHookMismatch,
		},
		"Service the hook checks is unreachable": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.Value[processor.State]{}

				return hk
			},
			Unreachable: true,
			Fetches:     1,
			Err:         hook.ErrUpstreamUnavailable,
		},
		// the reset set the hook up, but the row that would name the new
		// watcher is not stored.
		"Error returned by db.UpdateDocumentHook tears down the setup": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.Value[processor.State]{}

				return hk
			},
			CD:        true,
			UpdateErr: assert.AnError,
			Fetches:   1,
			Updates:   1,
			Watchers:  []string{"w-new"},
			Err:       assert.AnError,
		},
		// there is nothing to refuse: the hook is stored with the status
		// that tells why it cannot check.
		"Hook that cannot check its target is stored with its status": {
			Hook: func(*testing.T) hook.Hook {
				return urlWatcherHook(branchID)
			},
			Fetches:      1,
			Updates:      1,
			MaintFetches: 1,
			Status:       processor.StatusUnconfigured,
			Published:    []notification.Code{notification.NotificationDocumentHookNeedsAttention},
		},
		"Successful reset": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now().Add(-time.Hour))
			},
			Fetches: 1,
			Updates: 1,
			Status:  processor.StatusActive,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			stored := c.Hook(t)

			db := stubStoredHook(&DBMock{
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.UpdateErr
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"user-1"}, nil
				},
			}, stored, c.FetchErr)

			var (
				cd *fakeChangeDetection
				wc *webchange.Client
			)

			if c.CD {
				cd, wc = newFakeChangeDetection(t)
			}

			if c.Unreachable {
				wc = webchange.NewClient("http://127.0.0.1:1", "key")
			}

			pub := &fakePublisher{}
			man := newTestManager(t, db, pub, wc)

			var changes []hook.Hook

			man.BindHookChange(func(h hook.Hook) { changes = append(changes, h) })

			ctx := context.Background()

			if c.Held {
				unlock, err := man.hookMu.Lock(ctx, stored.ID)
				require.NoError(t, err)

				defer unlock()

				var cancel context.CancelFunc

				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			documentID := stored.DocumentID.V
			if c.OtherDocument {
				documentID = xid.New()
			}

			hk, err := man.ResetHook(ctx, stored.ID, documentID, "org-1")
			testutil.AssertEqualError(t, c.Err, err)

			assert.Len(t, db.FetchDocumentHookCalls(), c.Fetches)

			ff := db.UpdateDocumentHookCalls()
			require.Len(t, ff, c.Updates)

			assert.Len(t, db.FetchDocumentMaintainersCalls(), c.MaintFetches)
			assert.Equal(t, c.Published, pub.codes)

			if cd != nil {
				assert.Equal(t, c.Watchers, cd.deletedWatchers())
			}

			if err != nil {
				assert.Empty(t, changes)

				return
			}

			assert.Equal(t, *hk, ff[0].Hk)
			assert.Equal(t, c.Status, hk.Status)
			assert.True(t, hk.UpdatedAt.Valid)
			assert.Equal(t, []hook.Hook{*hk}, changes)
		})
	}
}
