package manager

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fakePublisher records published notifications.
type fakePublisher struct {
	mu sync.Mutex

	organizationIDs []string
	codes           []notification.Code
	userIDs         [][]string
}

func (f *fakePublisher) PublishNotifications(organizationID string, core notification.Core, userIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.organizationIDs = append(f.organizationIDs, organizationID)
	f.codes = append(f.codes, core.Code)
	f.userIDs = append(f.userIDs, userIDs)
}

// changeRecorder records the hooks a manager announces as changed.
type changeRecorder struct {
	hooks []hook.Hook
}

func (r *changeRecorder) record(h hook.Hook) {
	r.hooks = append(r.hooks, h)
}

// fakeChangeDetection is a changedetection.io server that creates the
// watcher "w-new" and records the watchers it deletes.
type fakeChangeDetection struct {
	mu sync.Mutex

	deleted []string
}

// newFakeChangeDetection starts a fake changedetection.io server and
// returns a client for it.
func newFakeChangeDetection(t *testing.T) (*fakeChangeDetection, *webchange.Client) {
	t.Helper()

	f := &fakeChangeDetection{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)

			_, err := w.Write([]byte(`{"uuid":"w-new"}`))
			assert.NoError(t, err)
		case http.MethodDelete:
			f.mu.Lock()
			f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/watch/"))
			f.mu.Unlock()

			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return f, webchange.NewClient(srv.URL, "key")
}

// deletedWatchers returns the watchers the server deleted.
func (f *fakeChangeDetection) deletedWatchers() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.deleted
}

// stubHook builds a scheduled-reminder hook attached to the given branch,
// due at the given schedule with the state started at startedAt.
func stubHook(t *testing.T, branchID xid.ID, schedule, startedAt time.Time) hook.Hook {
	t.Helper()

	settings, err := json.Marshal(map[string]any{
		"scale":    "linear",
		"schedule": schedule,
	})
	require.NoError(t, err)

	state, err := json.Marshal(processor.ScheduledReminderState{StartedAt: startedAt})
	require.NoError(t, err)

	return hook.Hook{
		ID:             xid.New(),
		Type:           hook.TypeScheduledReminder,
		DocumentID:     null.ValueFrom(xid.New()),
		OrganizationID: null.StringFrom("org-1"),
		BranchID:       null.ValueFrom(branchID),
		Settings:       processor.Settings(settings),
		State:          null.ValueFrom(processor.State(state)),
		Status:         processor.StatusActive,
		Score:          mathutil.Hundred,
	}
}

// stubDocument builds a document containing a single block "b1".
func stubDocument() *document.Document {
	return &document.Document{
		ID:             xid.New(),
		OrganizationID: "org-1",
		Content: document.RootBlock{
			Type: document.BlockNodeDoc,
			Content: []document.Block{
				{Type: document.BlockNodeParagraph, Attrs: document.Attributes{"uid": "b1"}},
			},
		},
	}
}

// newTestManager creates a Manager with an unconfigured GitHub client, the
// given changedetection client (unconfigured when nil) and the given mocks.
func newTestManager(t *testing.T, db *DBMock, pub *fakePublisher, wc *webchange.Client) *Manager {
	t.Helper()

	githubMan, err := github.NewManager(nil, github.Options{})
	require.NoError(t, err)

	if wc == nil {
		wc = webchange.NewClient("", "")
	}

	return NewManager(slog.New(slog.DiscardHandler), db, &FlusherMock{}, githubMan, wc, pub)
}

// urlWatcherHook builds a url-watcher hook that already holds a
// changedetection.io watcher in its state.
func urlWatcherHook(branchID xid.ID) hook.Hook {
	return hook.Hook{
		ID:             xid.New(),
		Type:           hook.TypeURLWatcher,
		DocumentID:     null.ValueFrom(xid.New()),
		OrganizationID: null.StringFrom("org-1"),
		BranchID:       null.ValueFrom(branchID),
		Settings:       processor.Settings(`{"url":"https://example.com"}`),
		State:          null.ValueFrom(processor.State(`{"watcherId":"w1"}`)),
		Status:         processor.StatusActive,
		Score:          mathutil.Hundred,
	}
}

// stubStoredHooks makes the DB mock answer a hook fetch with a copy of
// the matching hook.
func stubStoredHooks(db *DBMock, hooks []hook.Hook) *DBMock {
	db.FetchDocumentHookFunc = func(_ context.Context, id xid.ID, _ string) (*hook.Hook, error) {
		for _, h := range hooks {
			if h.ID == id {
				return &h, nil
			}
		}

		return nil, sql.ErrNoRows
	}

	return db
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	branchID := xid.New()

	db := &DBMock{
		FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hook.Hook, error) {
			cancel()

			return nil, nil
		},
	}

	man := newTestManager(t, db, &fakePublisher{}, nil)

	// the trigger is pending before Start, so the branch loop serves it
	// on its first turn, while the pass runs once on start.
	man.QueueBranch(branchID, "org-1")
	man.Start(ctx)

	assert.Len(t, db.FetchPaginatedDocumentHooksCalls(), 1)

	ff := db.FetchDocumentHooksByBranchIDCalls()
	require.Len(t, ff, 1)
	assert.Equal(t, branchID, ff[0].BranchID)
	assert.Equal(t, "org-1", ff[0].OrganizationID)
}

func Test_Manager_processBranches(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	cc := map[string]struct {
		FetchErr error
		Updates  int
	}{
		"Error returned by db.FetchDocumentHooksByBranchID": {
			FetchErr: assert.AnError,
		},
		"Queued branch hooks are processed": {
			Updates: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hooks := []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}

			db := stubStoredHooks(&DBMock{
				FetchDocumentHooksByBranchIDFunc: func(context.Context, xid.ID, string) ([]hook.Hook, error) {
					return hooks, c.FetchErr
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return stubDocument(), nil
				},
			}, hooks)

			man := newTestManager(t, db, &fakePublisher{}, nil)
			man.QueueBranch(branchID, "org-1")
			man.processBranches(context.Background())

			assert.Len(t, db.UpdateDocumentHookCalls(), c.Updates)

			// the queue is drained.
			man.processBranches(context.Background())
			assert.Len(t, db.FetchDocumentHooksByBranchIDCalls(), 1)
		})
	}
}

func Test_Manager_processHooks(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	type check func(*testing.T, *DBMock, *changeRecorder, *fakePublisher, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(expect bool) check {
		return func(t *testing.T, _ *DBMock, _ *changeRecorder, _ *fakePublisher, err error) {
			if expect {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *changeRecorder, _ *fakePublisher, _ error) {
			require.Len(t, db.DeleteDocumentHookCalls(), count)
		}
	}

	wasUpdateCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *changeRecorder, _ *fakePublisher, _ error) {
			require.Len(t, db.UpdateDocumentHookCalls(), count)
		}
	}

	wasNotified := func(count int) check {
		return func(t *testing.T, _ *DBMock, notifier *changeRecorder, _ *fakePublisher, _ error) {
			require.Len(t, notifier.hooks, count)
		}
	}

	wasFetchDocumentCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *changeRecorder, _ *fakePublisher, _ error) {
			require.Len(t, db.FetchDocumentByBranchIDCalls(), count)
		}
	}

	wasPublished := func(codes ...notification.Code) check {
		return func(t *testing.T, _ *DBMock, _ *changeRecorder, pub *fakePublisher, _ error) {
			assert.Equal(t, codes, pub.codes)
		}
	}

	hasUpdated := func(fn func(*testing.T, hook.Hook)) check {
		return func(t *testing.T, db *DBMock, _ *changeRecorder, _ *fakePublisher, _ error) {
			ff := db.UpdateDocumentHookCalls()
			require.NotEmpty(t, ff)
			fn(t, ff[0].Hk)
		}
	}

	hasUpdatedScore := func(expected decimal.Decimal) check {
		return hasUpdated(func(t *testing.T, h hook.Hook) {
			assert.True(t, h.Score.Equal(expected), "score %s, want %s", h.Score, expected)
		})
	}

	hasUpdatedStatus := func(expected processor.Status) check {
		return hasUpdated(func(t *testing.T, h hook.Hook) {
			assert.Equal(t, expected, h.Status)
		})
	}

	hasUpdatedSoftDeletion := func(expected bool) check {
		return hasUpdated(func(t *testing.T, h hook.Hook) {
			assert.Equal(t, expected, h.SoftDeletedAt.Valid)
		})
	}

	cc := map[string]struct {
		Hooks    func(t *testing.T) []hook.Hook
		FetchErr error
		// Held has a write hold the hook's lock during the pass.
		Held      bool
		StoredErr error
		DocErr    error
		Doc       *document.Document
		MaintErr  error
		UpdateErr error
		// CD makes the changedetection client a configured fake, whose
		// deleted watchers Watchers expects.
		CD       bool
		Watchers []string
		Checks   []check
	}{
		"Hook fetch failure is propagated": {
			Hooks:    func(*testing.T) []hook.Hook { return nil },
			FetchErr: assert.AnError,
			Checks: checks(
				hasError(true),
				wasUpdateCalled(0),
			),
		},
		"Error returned by db.FetchDocumentHook skips the hook": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}
			},
			StoredErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
			),
		},
		// a write holds the hook, so it waits for the next pass.
		"Hook a write holds is skipped": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}
			},
			Held: true,
			Checks: checks(
				hasError(false),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
			),
		},
		"Hook deleted meanwhile is skipped": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}
			},
			StoredErr: sql.ErrNoRows,
			Checks: checks(
				hasError(false),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
			),
		},
		"Orphaned hook without a branch is deleted": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BranchID = null.Value[xid.ID]{}

				return []hook.Hook{h}
			},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
				wasNotified(1),
			),
		},
		"Orphaned hook without a document is deleted": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.DocumentID = null.Value[xid.ID]{}

				return []hook.Hook{h}
			},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
			),
		},
		// the organization row is deleted outside of core, taking the
		// documents with it; the hook row is what is left of the watcher.
		"Orphaned hook without an organization is deleted": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.OrganizationID = null.String{}

				return []hook.Hook{h}
			},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
			),
		},
		"Orphaned URL watcher has its watcher torn down": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.BranchID = null.Value[xid.ID]{}

				return []hook.Hook{h}
			},
			CD:       true,
			Watchers: []string{"w1"},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
			),
		},
		// a copy cut loose before it was set up holds nothing outside.
		"Orphaned URL watcher never set up is deleted": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.State = null.Value[processor.State]{}
				h.BranchID = null.Value[xid.ID]{}

				return []hook.Hook{h}
			},
			CD: true,
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
			),
		},
		"Hook never set up is set up": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.State = null.Value[processor.State]{}

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			CD:  true,
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdated(func(t *testing.T, h hook.Hook) {
					require.True(t, h.State.Valid)
					assert.JSONEq(t, `{"watcherId":"w-new","lastChangedAt":null}`, string(h.State.V))
				}),
				wasPublished(),
			),
		},
		"Failed update of a setup tears down the new watcher": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.State = null.Value[processor.State]{}

				return []hook.Hook{h}
			},
			Doc:       stubDocument(),
			CD:        true,
			UpdateErr: assert.AnError,
			Watchers:  []string{"w-new"},
			Checks: checks(
				hasError(false),
			),
		},
		"Processing failure still persists the cleared soft deletion": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BlockID = null.StringFrom("b1")
				h.SoftDeletedAt = null.TimeFrom(time.Now().Add(-time.Hour))
				// unparsable settings make Process fail.
				h.Settings = processor.Settings(`{"scale": 1}`)

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedSoftDeletion(false),
			),
		},
		"Unconfigured integration is a status and notifies once": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.Score = decimal.NewFromInt(40)

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedStatus(processor.StatusUnconfigured),
				hasUpdatedScore(decimal.NewFromInt(40)),
				wasPublished(notification.NotificationDocumentHookNeedsAttention),
			),
		},
		"Hook already not active is not re-notified": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.Status = processor.StatusUnconfigured

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				wasPublished(),
			),
		},
		"Missing document deletes the hook": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}
			},
			DocErr: sql.ErrNoRows,
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasUpdateCalled(0),
			),
		},
		"Document fetch failure skips the hook": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())}
			},
			DocErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasDeleteCalled(0),
				wasUpdateCalled(0),
			),
		},
		"Healthy hook is processed and updated": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(1000*time.Hour), time.Now())}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedScore(decimal.NewFromInt(100)),
				wasPublished(),
				wasNotified(0),
			),
		},
		"Full-to-zero score drop notifies the maintainers": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedScore(decimal.Zero),
				wasPublished(notification.NotificationDocumentHookTriggered),
				wasNotified(1),
			),
		},
		// the score decays gradually, so by the time it reaches zero the
		// previous one is somewhere below full: the notification has to
		// trigger on the arrival at zero, not on a full-to-zero jump.
		"Partly decayed score reaching zero notifies the maintainers": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
				h.Score = decimal.NewFromInt(40)

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				hasUpdatedScore(decimal.Zero),
				wasPublished(notification.NotificationDocumentHookTriggered),
			),
		},
		"Already zero score is not re-notified": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
				h.Score = decimal.Zero

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				wasPublished(),
			),
		},
		// the unpersisted score would be recomputed into the same
		// transition on the next cycle, notifying again every five minutes.
		"Failed update suppresses the notification": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))}
			},
			Doc:       stubDocument(),
			UpdateErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				wasPublished(),
			),
		},
		// the hook kept its score while it could not check, so its return at
		// zero is the arrival to announce.
		"Inactive hook back at zero notifies the maintainers": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
				h.Status = processor.StatusUnconfigured
				h.Score = decimal.NewFromInt(40)

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				hasUpdatedStatus(processor.StatusActive),
				hasUpdatedScore(decimal.Zero),
				wasPublished(notification.NotificationDocumentHookTriggered),
				wasNotified(1),
			),
		},
		"Maintainer fetch failure suppresses the notification": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))}
			},
			Doc:      stubDocument(),
			MaintErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				wasPublished(),
			),
		},
		"Documents are fetched once per branch": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{
					stubHook(t, branchID, time.Now().Add(time.Hour), time.Now()),
					stubHook(t, branchID, time.Now().Add(time.Hour), time.Now()),
				}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasFetchDocumentCalled(1),
				wasUpdateCalled(2),
			),
		},
		"Missing block soft-deletes the hook": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BlockID = null.StringFrom("missing-block")

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedSoftDeletion(true),
			),
		},
		// the hook's block is gone, so its score would describe nothing: the
		// elapsed schedule must not drop the score to zero, and no
		// notification about the vanished block may go out.
		"Soft-deleted hook is not processed or notified": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
				h.BlockID = null.StringFrom("missing-block")

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedScore(decimal.NewFromInt(100)),
				hasUpdatedSoftDeletion(true),
				wasPublished(),
			),
		},
		"Reappearing block clears the soft deletion": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BlockID = null.StringFrom("b1")
				h.SoftDeletedAt = null.TimeFrom(time.Now().Add(-time.Hour))

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedSoftDeletion(false),
			),
		},
		"Retention-expired soft-deleted hook is deleted": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BlockID = null.StringFrom("missing-block")
				h.SoftDeletedAt = null.TimeFrom(time.Now().Add(-_hookRetentionDuration - time.Hour))

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasUpdateCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			hooks := c.Hooks(t)

			db := stubStoredHooks(&DBMock{
				FetchPaginatedDocumentHooksFunc: func(_ context.Context, offsetID xid.ID, _ int64) ([]hook.Hook, error) {
					if !offsetID.IsZero() {
						return nil, nil
					}

					return hooks, c.FetchErr
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return c.Doc, c.DocErr
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"user-1"}, c.MaintErr
				},
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return c.UpdateErr
				},
			}, hooks)

			if c.StoredErr != nil {
				db.FetchDocumentHookFunc = func(context.Context, xid.ID, string) (*hook.Hook, error) {
					return nil, c.StoredErr
				}
			}

			var (
				cd *fakeChangeDetection
				wc *webchange.Client
			)

			if c.CD {
				cd, wc = newFakeChangeDetection(t)
			}

			pub := &fakePublisher{}
			notifier := &changeRecorder{}

			man := newTestManager(t, db, pub, wc)
			man.BindHookChange(notifier.record)

			if c.Held {
				unlock, err := man.hooks.Lock(context.Background(), hooks[0].ID)
				require.NoError(t, err)

				defer unlock()
			}

			err := man.processHooks(context.Background())

			for _, ch := range c.Checks {
				ch(t, db, notifier, pub, err)
			}

			if cd != nil {
				assert.Equal(t, c.Watchers, cd.deletedWatchers())
			}
		})
	}
}
