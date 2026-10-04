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
)

// fakePublisher records published notifications.
type fakePublisher struct {
	organizationIDs []string
	codes           []notification.Code
	userIDs         [][]string
}

func (f *fakePublisher) PublishNotifications(organizationID string, core notification.Core, userIDs ...string) {
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

// newTestManager creates a Manager with unconfigured GitHub and
// changedetection clients and the given mocks.
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

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	fetched := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := &DBMock{
		FetchPaginatedDocumentHooksFunc: func(context.Context, xid.ID, int64) ([]hook.Hook, error) {
			close(fetched)
			cancel()

			return nil, nil
		},
	}

	man := newTestManager(t, db, &fakePublisher{}, nil)

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		man.Start(ctx)
	}()

	<-fetched
	<-stopped

	assert.Len(t, db.FetchPaginatedDocumentHooksCalls(), 1)
}

func Test_Manager_processHooks(t *testing.T) {
	t.Parallel()

	branchID := xid.New()

	type check func(*testing.T, *DBMock, *fakePublisher, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(expect bool) check {
		return func(t *testing.T, _ *DBMock, _ *fakePublisher, err error) {
			if expect {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
			require.Len(t, db.DeleteDocumentHookCalls(), count)
		}
	}

	wasUpdateCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
			require.Len(t, db.UpdateDocumentHookCalls(), count)
		}
	}

	wasFetchDocumentCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
			require.Len(t, db.FetchDocumentByBranchIDCalls(), count)
		}
	}

	wasPublished := func(count int) check {
		return func(t *testing.T, _ *DBMock, pub *fakePublisher, _ error) {
			require.Len(t, pub.organizationIDs, count)
		}
	}

	wasPublishedCode := func(code notification.Code) check {
		return func(t *testing.T, _ *DBMock, pub *fakePublisher, _ error) {
			assert.Equal(t, []notification.Code{code}, pub.codes)
		}
	}

	hasUpdatedStatus := func(expected processor.Status) check {
		return func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
			ff := db.UpdateDocumentHookCalls()
			require.NotEmpty(t, ff)
			assert.Equal(t, expected, ff[0].Hk.Status)
		}
	}

	hasUpdatedScore := func(expected decimal.Decimal) check {
		return func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
			ff := db.UpdateDocumentHookCalls()
			require.NotEmpty(t, ff)
			assert.True(
				t,
				ff[0].Hk.Score.Equal(expected),
				"score %s, want %s", ff[0].Hk.Score, expected,
			)
		}
	}

	tests := map[string]struct {
		Hooks     func(t *testing.T) []hook.Hook
		FetchErr  error
		DocErr    error
		Doc       *document.Document
		MaintErr  error
		UpdateErr error
		Checks    []check
	}{
		"Hook fetch failure is propagated": {
			Hooks:    func(*testing.T) []hook.Hook { return nil },
			FetchErr: assert.AnError,
			Checks: checks(
				hasError(true),
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
		"Processing failure still persists the cleared soft deletion": {
			Hooks: func(t *testing.T) []hook.Hook {
				h := stubHook(t, branchID, time.Now().Add(time.Hour), time.Now())
				h.BlockID = null.StringFrom("b1")
				h.SoftDeletedAt = null.TimeFrom(time.Now().Add(-time.Hour))
				// unparsable settings make Process fail, which used to
				// skip the update and leave the retention clock running.
				h.Settings = processor.Settings(`{"scale": "nonsense"}`)

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
					ff := db.UpdateDocumentHookCalls()
					require.NotEmpty(t, ff)
					assert.False(t, ff[0].Hk.SoftDeletedAt.Valid)
				},
			),
		},
		// the score is kept, so the hook does not also read as outdated.
		"Hook that can no longer check notifies the maintainers": {
			Hooks: func(_ *testing.T) []hook.Hook {
				return []hook.Hook{urlWatcherHook(branchID)}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedStatus(processor.StatusUnconfigured),
				hasUpdatedScore(mathutil.Hundred),
				wasPublishedCode(notification.NotificationDocumentHookNeedsAttention),
			),
		},
		"Hook that still cannot check is not re-notified": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.Status = processor.StatusUnconfigured

				return []hook.Hook{h}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedStatus(processor.StatusUnconfigured),
				wasPublished(0),
			),
		},

		// an unconfigured changedetection must not trap the orphaned row:
		// the teardown is skipped and the row is still deleted.
		"Orphaned URL watcher is deleted when unconfigured": {
			Hooks: func(_ *testing.T) []hook.Hook {
				h := urlWatcherHook(branchID)
				h.BranchID = null.Value[xid.ID]{}

				return []hook.Hook{h}
			},
			Checks: checks(
				hasError(false),
				wasDeleteCalled(1),
				wasFetchDocumentCalled(0),
				wasUpdateCalled(0),
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
				wasPublished(0),
			),
		},
		"Full-to-zero score drop notifies the maintainers": {
			Hooks: func(t *testing.T) []hook.Hook {
				// elapsed schedule: the hook drops from 100 to 0.
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))}
			},
			Doc: stubDocument(),
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				hasUpdatedScore(decimal.Zero),
				wasPublishedCode(notification.NotificationDocumentHookTriggered),
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
				wasPublished(1),
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
				wasPublished(0),
			),
		},
		// the unpersisted score would be recomputed into the same
		// transition on the next cycle, notifying again every five minutes.
		"Failed score persist suppresses the notification": {
			Hooks: func(t *testing.T) []hook.Hook {
				return []hook.Hook{stubHook(t, branchID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))}
			},
			Doc:       stubDocument(),
			UpdateErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasUpdateCalled(1),
				wasPublished(0),
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
				wasPublished(0),
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
				func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
					ff := db.UpdateDocumentHookCalls()
					require.NotEmpty(t, ff)
					assert.True(t, ff[0].Hk.SoftDeletedAt.Valid)
				},
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
				wasPublished(0),
				func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
					ff := db.UpdateDocumentHookCalls()
					require.NotEmpty(t, ff)
					assert.True(t, ff[0].Hk.SoftDeletedAt.Valid)
				},
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
				func(t *testing.T, db *DBMock, _ *fakePublisher, _ error) {
					ff := db.UpdateDocumentHookCalls()
					require.NotEmpty(t, ff)
					assert.False(t, ff[0].Hk.SoftDeletedAt.Valid)
				},
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			hooks := tc.Hooks(t)

			db := &DBMock{
				FetchPaginatedDocumentHooksFunc: func(_ context.Context, offsetID xid.ID, _ int64) ([]hook.Hook, error) {
					if !offsetID.IsZero() {
						return nil, nil
					}

					return hooks, tc.FetchErr
				},
				FetchDocumentByBranchIDFunc: func(context.Context, xid.ID, string) (*document.Document, error) {
					return tc.Doc, tc.DocErr
				},
				FetchDocumentMaintainersFunc: func(context.Context, xid.ID, string) ([]string, error) {
					return []string{"user-1"}, tc.MaintErr
				},
				UpdateDocumentHookFunc: func(context.Context, hook.Hook) error {
					return tc.UpdateErr
				},
			}

			pub := &fakePublisher{}

			err := newTestManager(t, db, pub, nil).processHooks(context.Background())

			for _, ch := range tc.Checks {
				ch(t, db, pub, err)
			}
		})
	}
}
