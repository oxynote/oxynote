package processor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChangeDetection is a ChangeDetection stub with per-method behavior
// and call recording.
type fakeChangeDetection struct {
	watch     *webchange.Watch
	fetchErr  error
	createdID string
	createErr error
	updateErr error
	deleteErr error

	createdURLs []string
	updated     [][2]string
	deletedIDs  []string
}

func (f *fakeChangeDetection) CreateWatcher(_ context.Context, url string) (string, error) {
	f.createdURLs = append(f.createdURLs, url)

	return f.createdID, f.createErr
}

func (f *fakeChangeDetection) FetchWatcher(_ context.Context, _ string) (*webchange.Watch, error) {
	return f.watch, f.fetchErr
}

func (f *fakeChangeDetection) UpdateWatcher(_ context.Context, watchID, url string) error {
	f.updated = append(f.updated, [2]string{watchID, url})

	return f.updateErr
}

func (f *fakeChangeDetection) DeleteWatcher(_ context.Context, watchID string) error {
	f.deletedIDs = append(f.deletedIDs, watchID)

	return f.deleteErr
}

// watcherState marshals a URL-watcher state for the watcher "w-1".
func watcherState(t *testing.T, lastChangedAt null.Time) State {
	t.Helper()

	raw, err := json.Marshal(URLWatcherState{
		WatcherID:     "w-1",
		LastChangedAt: lastChangedAt,
	})
	require.NoError(t, err)

	return State(raw)
}

func Test_URLWatcher_Process(t *testing.T) {
	t.Parallel()

	changed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		CD             *fakeChangeDetection
		State          State
		ExpectErr      bool
		ExpectedScore  decimal.Decimal
		ExpectedStatus URLWatcherStatus
	}{
		"Unchanged watch keeps the full score": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{LastChangedAt: changed},
			},
			State:          watcherState(t, null.TimeFrom(changed)),
			ExpectedScore:  decimal.NewFromInt(100),
			ExpectedStatus: URLWatcherStatusActive,
		},
		"Newer change drops the score to zero": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{LastChangedAt: changed.Add(time.Hour)},
			},
			State:          watcherState(t, null.TimeFrom(changed)),
			ExpectedScore:  decimal.Zero,
			ExpectedStatus: URLWatcherStatusActive,
		},
		"Missing baseline adopts the watch timestamp": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{LastChangedAt: changed},
			},
			State:          watcherState(t, null.Time{}),
			ExpectedScore:  decimal.NewFromInt(100),
			ExpectedStatus: URLWatcherStatusActive,
		},
		"Unreachable watch scores zero": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{Unreachable: true},
			},
			State:          watcherState(t, null.Time{}),
			ExpectedScore:  decimal.Zero,
			ExpectedStatus: URLWatcherStatusUnreachableURL,
		},
		"Fetch failure is propagated": {
			CD:        &fakeChangeDetection{fetchErr: assert.AnError},
			State:     watcherState(t, null.Time{}),
			ExpectErr: true,
		},
		"Malformed state fails": {
			CD:        &fakeChangeDetection{},
			State:     State(`{not json`),
			ExpectErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			uw := URLWatcher{URL: "https://example.com"}

			score, state, err := uw.Process(context.Background(), stubInput{state: tc.State, cd: tc.CD})

			if tc.ExpectErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.True(t, score.Equal(tc.ExpectedScore), "score %s", score)

			var uws URLWatcherState

			require.NoError(t, json.Unmarshal(state, &uws))
			assert.Equal(t, tc.ExpectedStatus, uws.Status)
		})
	}
}

func Test_URLWatcher_Reset(t *testing.T) {
	t.Parallel()

	changed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("Missing watcher is created", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{createdID: "w-new"}
		uw := URLWatcher{URL: "https://example.com"}

		score, state, err := uw.Reset(context.Background(), stubInput{cd: cd})
		require.NoError(t, err)

		assert.True(t, score.Equal(decimal.NewFromInt(100)))
		assert.Equal(t, []string{"https://example.com"}, cd.createdURLs)

		var uws URLWatcherState

		require.NoError(t, json.Unmarshal(state, &uws))
		assert.Equal(t, "w-new", uws.WatcherID)
		assert.Equal(t, URLWatcherStatusActive, uws.Status)
		assert.False(t, uws.LastChangedAt.Valid)
	})

	t.Run("Existing watcher adopts the watch timestamp", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{
			watch: &webchange.Watch{URL: "https://example.com", LastChangedAt: changed},
		}
		uw := URLWatcher{URL: "https://example.com"}

		_, state, err := uw.Reset(context.Background(), stubInput{
			state: watcherState(t, null.Time{}),
			cd:    cd,
		})
		require.NoError(t, err)

		var uws URLWatcherState

		require.NoError(t, json.Unmarshal(state, &uws))
		assert.Equal(t, null.TimeFrom(changed), uws.LastChangedAt)
		assert.Empty(t, cd.updated)
	})

	t.Run("Changed URL updates the watcher and clears the baseline", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{
			watch: &webchange.Watch{URL: "https://old.example.com", LastChangedAt: changed},
		}
		uw := URLWatcher{URL: "https://example.com"}

		_, state, err := uw.Reset(context.Background(), stubInput{
			state: watcherState(t, null.Time{}),
			cd:    cd,
		})
		require.NoError(t, err)

		assert.Equal(t, [][2]string{{"w-1", "https://example.com"}}, cd.updated)

		var uws URLWatcherState

		require.NoError(t, json.Unmarshal(state, &uws))
		assert.False(t, uws.LastChangedAt.Valid)
	})

	t.Run("Create failure is propagated", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{createErr: assert.AnError}

		_, _, err := (&URLWatcher{URL: "https://example.com"}).Reset(context.Background(), stubInput{cd: cd})
		require.Error(t, err)
	})
}

func Test_URLWatcher_Delete(t *testing.T) {
	t.Parallel()

	t.Run("Existing watcher is deleted", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{}

		err := (&URLWatcher{}).Delete(context.Background(), stubInput{
			state: watcherState(t, null.Time{}),
			cd:    cd,
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"w-1"}, cd.deletedIDs)
	})

	t.Run("Missing state is a no-op", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{}

		require.NoError(t, (&URLWatcher{}).Delete(context.Background(), stubInput{cd: cd}))
		assert.Empty(t, cd.deletedIDs)
	})

	t.Run("Delete failure is propagated", func(t *testing.T) {
		t.Parallel()

		cd := &fakeChangeDetection{deleteErr: assert.AnError}

		err := (&URLWatcher{}).Delete(context.Background(), stubInput{
			state: watcherState(t, null.Time{}),
			cd:    cd,
		})
		require.Error(t, err)
	})
}
