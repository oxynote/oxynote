package manager

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDB wires the DB mock's BeginTx to hand out the Tx mock, or to fail
// with beginErr, as the real BeginTx does.
func stubDB(db *DBMock, tx *TxMock, beginErr error) *DBMock {
	db.BeginTxFunc = func(_ context.Context, dest any) error {
		if beginErr != nil {
			return beginErr
		}

		*dest.(*Tx) = tx

		return nil
	}

	return db
}

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
		Settings processor.Settings
		CD       bool
		Tx       *TxMock
		Inserts  int
		Commits  int
		Watchers []string
		Err      error
	}{
		"Unknown type is refused": {
			Type:     "bogus",
			Settings: processor.Settings(`{}`),
			Tx:       &TxMock{},
			Err:      hook.ErrInvalidType,
		},
		"Hook that cannot check its target is refused": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			Tx:       &TxMock{},
			Err:      errutil.New(http.StatusUnprocessableEntity, "document_hook.unconfigured", "the hook cannot check its target: %s", processor.StatusUnconfigured),
		},
		// the insert is rolled back, so the watcher the hook created is torn
		// down again.
		"Error returned by tx.InsertDocumentHook": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			Tx: &TxMock{
				InsertDocumentHookFunc: func(context.Context, hook.Hook) error {
					return assert.AnError
				},
			},
			Inserts:  1,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		"Successful creation": {
			Type:     hook.TypeURLWatcher,
			Settings: processor.Settings(`{"url":"https://example.com"}`),
			CD:       true,
			Tx:       &TxMock{},
			Inserts:  1,
			Commits:  1,
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

			man := newTestManager(t, stubDB(&DBMock{}, c.Tx, nil), &fakePublisher{}, wc)

			documentID := xid.New()

			hk, err := man.CreateHook(context.Background(), hook.CreateInput{
				Type:     c.Type,
				BranchID: branchID,
				Settings: c.Settings,
			}, documentID, "org-1", "u1")
			testutil.AssertEqualError(t, c.Err, err)

			require.Len(t, c.Tx.InsertDocumentHookCalls(), c.Inserts)
			assert.Len(t, c.Tx.CommitCalls(), c.Commits)

			if cd != nil {
				assert.Equal(t, c.Watchers, cd.deletedWatchers())
			}

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.Equal(t, *hk, c.Tx.InsertDocumentHookCalls()[0].Hk)
			assert.Equal(t, null.ValueFrom(documentID), hk.DocumentID)
			assert.Equal(t, null.ValueFrom(branchID), hk.BranchID)
			assert.Equal(t, null.StringFrom("org-1"), hk.OrganizationID)
			assert.Equal(t, processor.StatusActive, hk.Status)
			assert.True(t, hk.State.Valid)

			ff := c.Tx.RecordDocumentBranchHistoryEntryCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, null.StringFrom("u1"), ff[0].By)
		})
	}
}

func Test_Manager_UpdateHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()
	settings := processor.Settings(`{"scale":"linear","schedule":"2099-01-01T00:00:00Z"}`)

	cc := map[string]struct {
		Settings  processor.Settings
		FetchErr  error
		UpdateErr error
		Updates   int
		Err       error
	}{
		"Error returned by db.FetchDocumentHook": {
			Settings: settings,
			FetchErr: sql.ErrNoRows,
			Err:      sql.ErrNoRows,
		},
		"Processor refuses the settings": {
			Settings: processor.Settings(`{"scale":"bogus","schedule":"2099-01-01T00:00:00Z"}`),
			Err:      processor.ErrInvalidScaleType,
		},
		"Error returned by tx.UpdateDocumentHook": {
			Settings:  settings,
			UpdateErr: assert.AnError,
			Updates:   1,
			Err:       assert.AnError,
		},
		"Successful update": {
			Settings: settings,
			Updates:  1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			stored := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			tx := &TxMock{
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.UpdateErr
				},
			}
			db := stubStoredHook(stubDB(&DBMock{}, tx, nil), stored, c.FetchErr)

			man := newTestManager(t, db, &fakePublisher{}, nil)

			hk, err := man.UpdateHook(context.Background(), stored.ID, "org-1", hook.UpdateInput{Settings: c.Settings}, "u1")
			testutil.AssertEqualError(t, c.Err, err)

			ff := tx.UpdateDocumentHookCalls()
			require.Len(t, ff, c.Updates)

			ll := db.FetchDocumentHookCalls()
			require.Len(t, ll, 1)
			assert.Equal(t, stored.ID, ll[0].ID)
			assert.Equal(t, "org-1", ll[0].OrganizationID)

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.Equal(t, *hk, ff[0].Hk)
			assert.Equal(t, c.Settings, hk.Settings)
			assert.True(t, hk.UpdatedAt.Valid)
			assert.Len(t, tx.RecordDocumentBranchHistoryEntryCalls(), 1)
			assert.Len(t, tx.CommitCalls(), 1)
		})
	}
}

func Test_Manager_DeleteHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		Hook    func(*testing.T) hook.Hook
		DelErr  error
		Deletes int
		Err     error
	}{
		// the row goes only once the resource it describes is gone, so a
		// failed teardown keeps it.
		"External teardown fails": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.ValueFrom(processor.State(`{`))

				return hk
			},
			Err: hook.ErrUpstreamUnavailable,
		},
		"Error returned by tx.DeleteDocumentHook": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			DelErr:  assert.AnError,
			Deletes: 1,
			Err:     assert.AnError,
		},
		"Hook never set up is deleted": {
			Hook: func(*testing.T) hook.Hook {
				hk := urlWatcherHook(branchID)
				hk.State = null.Value[processor.State]{}

				return hk
			},
			Deletes: 1,
		},
		"Successful deletion": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			},
			Deletes: 1,
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
			}

			man := newTestManager(t, stubStoredHook(stubDB(&DBMock{}, tx, nil), hk, nil), &fakePublisher{}, nil)

			err := man.DeleteHook(context.Background(), hk.ID, "org-1", "u1")
			testutil.AssertEqualError(t, c.Err, err)

			ff := tx.DeleteDocumentHookCalls()
			require.Len(t, ff, c.Deletes)

			if err != nil {
				return
			}

			assert.Equal(t, hk.ID, ff[0].ID)
			assert.Len(t, tx.RecordDocumentBranchHistoryEntryCalls(), 1)
		})
	}
}

func Test_Manager_ResetHook(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		Hook      func(*testing.T) hook.Hook
		UpdateErr error
		Updates   int
		Status    processor.Status
		Published []notification.Code
		Err       error
	}{
		"Error returned by tx.UpdateDocumentHook": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now().Add(-time.Hour))
			},
			UpdateErr: assert.AnError,
			Updates:   1,
			Err:       assert.AnError,
		},
		// there is nothing to refuse: the hook is stored with the status
		// that tells why it cannot check.
		"Hook that cannot check its target is stored with its status": {
			Hook: func(*testing.T) hook.Hook {
				return urlWatcherHook(branchID)
			},
			Updates:   1,
			Status:    processor.StatusUnconfigured,
			Published: []notification.Code{notification.NotificationDocumentHookNeedsAttention},
		},
		"Successful reset": {
			Hook: func(t *testing.T) hook.Hook {
				return stubHook(t, branchID, time.Now().Add(time.Hour), time.Now().Add(-time.Hour))
			},
			Updates: 1,
			Status:  processor.StatusActive,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			stored := c.Hook(t)

			tx := &TxMock{
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.UpdateErr
				},
			}

			db := stubStoredHook(stubDB(&DBMock{
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"user-1"}, nil
				},
			}, tx, nil), stored, nil)
			pub := &fakePublisher{}

			man := newTestManager(t, db, pub, nil)

			hk, err := man.ResetHook(context.Background(), stored.ID, "org-1")
			testutil.AssertEqualError(t, c.Err, err)

			ff := tx.UpdateDocumentHookCalls()
			require.Len(t, ff, c.Updates)

			// history does not hold watcher state.
			assert.Empty(t, tx.RecordDocumentBranchHistoryEntryCalls())

			if err != nil {
				return
			}

			assert.Equal(t, *hk, ff[0].Hk)
			assert.Equal(t, c.Status, hk.Status)
			assert.Equal(t, c.Published, pub.codes)
		})
	}
}

func Test_Manager_change(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		// Held has another write hold the hook's lock until the change
		// gives up.
		Held      bool
		BeginErr  error
		FetchErr  error
		RunErr    error
		WriteErr  error
		CommitErr error
		// Writes is how many times the write ran; the outside run always
		// comes first, with no transaction open.
		Writes   int
		Watchers []string
		Err      error
	}{
		"Error returned by db.BeginTx tears down the setup": {
			BeginErr: assert.AnError,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		"Hook another write holds": {
			Held: true,
			Err:  context.DeadlineExceeded,
		},
		"Error returned by db.FetchDocumentHook": {
			FetchErr: assert.AnError,
			Err:      assert.AnError,
		},
		"Error returned by the run tears down the setup": {
			RunErr:   assert.AnError,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		// the run set the hook up, but the row that would name the new
		// watcher is not stored.
		"Failed write tears down the setup": {
			WriteErr: assert.AnError,
			Writes:   1,
			Watchers: []string{"w-new"},
			Err:      assert.AnError,
		},
		"Error returned by tx.Commit tears down the setup": {
			CommitErr: assert.AnError,
			Writes:    1,
			Watchers:  []string{"w-new"},
			Err:       assert.AnError,
		},
		"Successful change": {
			Writes: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			stored := urlWatcherHook(branchID)
			stored.State = null.Value[processor.State]{}

			tx := &TxMock{
				CommitFunc: func() error {
					return c.CommitErr
				},
			}

			cd, wc := newFakeChangeDetection(t)
			man := newTestManager(t, stubStoredHook(stubDB(&DBMock{}, tx, c.BeginErr), stored, c.FetchErr), &fakePublisher{}, wc)

			ctx := context.Background()

			if c.Held {
				unlock, err := man.hooks.Lock(ctx, stored.ID)
				require.NoError(t, err)

				defer unlock()

				var cancel context.CancelFunc

				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}

			var writes int

			hk, err := man.change(
				ctx,
				stored.ID,
				"org-1",
				func(ctx context.Context, hk *hook.Hook) error {
					if err := hk.Reset(ctx, man.input("org-1")); err != nil {
						return err
					}

					return c.RunErr
				},
				func(_ context.Context, wtx Tx, hk hook.Hook) error {
					writes++

					assert.Same(t, tx, wtx)
					assert.True(t, hk.State.Valid)

					return c.WriteErr
				},
			)
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Writes, writes)

			assert.Equal(t, c.Watchers, cd.deletedWatchers())

			if err != nil {
				assert.Nil(t, hk)
				return
			}

			assert.True(t, hk.State.Valid)
			assert.Len(t, tx.CommitCalls(), 1)
		})
	}
}

func Test_Manager_record(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		// Branchless cuts the hook loose from its branch.
		Branchless bool
		RecordErr  error
		Records    int
		Err        error
	}{
		"Error returned by tx.RecordDocumentBranchHistoryEntry": {
			RecordErr: assert.AnError,
			Records:   1,
			Err:       assert.AnError,
		},
		"Hook without a branch has no history": {
			Branchless: true,
		},
		"Successful record": {
			Records: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			tx := &TxMock{
				RecordDocumentBranchHistoryEntryFunc: func(context.Context, xid.ID, string, null.String, bool) (xid.ID, error) {
					return xid.ID{}, c.RecordErr
				},
			}

			hk := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
			if c.Branchless {
				hk.BranchID = null.Value[xid.ID]{}
			}

			err := newTestManager(t, &DBMock{}, &fakePublisher{}, nil).record(context.Background(), tx, hk, "u1")
			testutil.AssertEqualError(t, c.Err, err)

			ff := tx.RecordDocumentBranchHistoryEntryCalls()
			require.Len(t, ff, c.Records)

			for _, f := range ff {
				assert.Equal(t, branchID, f.BranchID)
				assert.Equal(t, "org-1", f.OrganizationID)
				assert.Equal(t, null.StringFrom("u1"), f.By)
				assert.False(t, f.Boundary)
			}
		})
	}
}

func Test_Manager_upstream(t *testing.T) {
	t.Parallel()

	man := newTestManager(t, &DBMock{}, &fakePublisher{}, nil)

	assert.Equal(t, processor.ErrInvalidURL, man.upstream(processor.ErrInvalidURL))
	assert.Equal(t, hook.ErrUpstreamUnavailable, man.upstream(assert.AnError))
}

func Test_Manager_input(t *testing.T) {
	t.Parallel()

	man := newTestManager(t, &DBMock{}, &fakePublisher{}, nil)

	got := man.input("org-1")
	require.NotNil(t, got)

	// the deployment has neither integration, and the input says so the
	// way the processors ask.
	assert.Same(t, man.webchangeClient, got.ChangeDetection())

	_, err := got.Github(context.Background())
	testutil.AssertEqualError(t, github.ErrNotConfigured, err)
}
