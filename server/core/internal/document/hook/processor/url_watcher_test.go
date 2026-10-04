package processor

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
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

// watcherState marshals a URL-watcher state.
func watcherState(t *testing.T, watcherID string, lastChangedAt null.Time) State {
	t.Helper()

	raw, err := json.Marshal(URLWatcherState{
		WatcherID:     watcherID,
		LastChangedAt: lastChangedAt,
	})
	require.NoError(t, err)

	return State(raw)
}

func Test_URLWatcher_Validate(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		URL string
		Err error
	}{
		"Https URL is valid":         {URL: "https://example.com/page"},
		"Http URL is valid":          {URL: "http://example.com"},
		"Other scheme is rejected":   {URL: "ftp://example.com", Err: ErrInvalidURL},
		"Relative URL is rejected":   {URL: "/page", Err: ErrInvalidURL},
		"Missing host is rejected":   {URL: "https://", Err: ErrInvalidURL},
		"Unparsable URL is rejected": {URL: "https://exa mple.com/%zz", Err: ErrInvalidURL},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			testutil.AssertEqualError(t, c.Err, (&URLWatcher{URL: c.URL}).Validate())
		})
	}
}

func Test_URLWatcher_Process(t *testing.T) {
	t.Parallel()

	changed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		CD      *fakeChangeDetection
		State   State
		Score   decimal.Decimal
		Status  Status
		Updated [][2]string
		Err     error
	}{
		"Unchanged watch keeps the full score": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://example.com", LastChangedAt: changed},
			},
			State:  watcherState(t, "w-1", null.TimeFrom(changed)),
			Score:  decimal.NewFromInt(100),
			Status: StatusActive,
		},
		"Newer change drops the score to zero": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://example.com", LastChangedAt: changed.Add(time.Hour)},
			},
			State:  watcherState(t, "w-1", null.TimeFrom(changed)),
			Score:  decimal.Zero,
			Status: StatusActive,
		},
		"Missing baseline adopts the watch timestamp": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://example.com", LastChangedAt: changed},
			},
			State:  watcherState(t, "w-1", null.Time{}),
			Score:  decimal.NewFromInt(100),
			Status: StatusActive,
		},
		"Unreachable watch is a status": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://example.com", Unreachable: true},
			},
			State:  watcherState(t, "w-1", null.Time{}),
			Status: StatusUnreachableURL,
		},
		"Watcher on another URL is pointed back": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://old.example.com", LastChangedAt: changed},
			},
			State:   watcherState(t, "w-1", null.TimeFrom(changed)),
			Score:   decimal.NewFromInt(100),
			Status:  StatusActive,
			Updated: [][2]string{{"w-1", "https://example.com"}},
		},
		"Removed watcher is created again": {
			CD: &fakeChangeDetection{
				fetchErr:  webchange.ErrWatcherNotFound,
				createdID: "w-new",
			},
			State:  watcherState(t, "w-1", null.Time{}),
			Score:  decimal.NewFromInt(100),
			Status: StatusActive,
		},
		"Unconfigured changedetection is a status": {
			CD:     &fakeChangeDetection{fetchErr: webchange.ErrNotConfigured},
			State:  watcherState(t, "w-1", null.Time{}),
			Status: StatusUnconfigured,
		},
		"Fetch failure is propagated": {
			CD:    &fakeChangeDetection{fetchErr: assert.AnError},
			State: watcherState(t, "w-1", null.Time{}),
			Err:   assert.AnError,
		},
		"Malformed state fails": {
			CD:    &fakeChangeDetection{},
			State: State(`{not json`),
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			uw := URLWatcher{URL: "https://example.com"}

			res, err := uw.Process(context.Background(), stubInput{state: c.State, cd: c.CD})
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Status, res.Status)
			assert.True(t, res.Score.Equal(c.Score), "score %s", res.Score)

			if c.Status != StatusActive {
				assert.Nil(t, res.State)
			}

			assert.Equal(t, c.Updated, c.CD.updated)
		})
	}
}

func Test_URLWatcher_Reset(t *testing.T) {
	t.Parallel()

	changed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cc := map[string]struct {
		CD      *fakeChangeDetection
		State   State
		Created []string
		Updated [][2]string
		Result  Result
		Err     error
	}{
		"Missing watcher is created": {
			CD:      &fakeChangeDetection{createdID: "w-new"},
			Created: []string{"https://example.com"},
			Result: Result{
				Status: StatusActive,
				Score:  decimal.NewFromInt(100),
				State:  watcherState(t, "w-new", null.Time{}),
			},
		},
		"Existing watcher adopts the watch timestamp": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://example.com", LastChangedAt: changed},
			},
			State: watcherState(t, "w-1", null.Time{}),
			Result: Result{
				Status: StatusActive,
				Score:  decimal.NewFromInt(100),
				State:  watcherState(t, "w-1", null.TimeFrom(changed)),
			},
		},
		"Changed URL updates the watcher and clears the baseline": {
			CD: &fakeChangeDetection{
				watch: &webchange.Watch{URL: "https://old.example.com", LastChangedAt: changed},
			},
			State:   watcherState(t, "w-1", null.Time{}),
			Updated: [][2]string{{"w-1", "https://example.com"}},
			Result: Result{
				Status: StatusActive,
				Score:  decimal.NewFromInt(100),
				State:  watcherState(t, "w-1", null.Time{}),
			},
		},
		"Removed watcher is created again": {
			CD: &fakeChangeDetection{
				fetchErr:  webchange.ErrWatcherNotFound,
				createdID: "w-new",
			},
			State:   watcherState(t, "w-1", null.Time{}),
			Created: []string{"https://example.com"},
			Result: Result{
				Status: StatusActive,
				Score:  decimal.NewFromInt(100),
				State:  watcherState(t, "w-new", null.Time{}),
			},
		},
		"Unconfigured changedetection is a status": {
			CD:      &fakeChangeDetection{createErr: webchange.ErrNotConfigured},
			Created: []string{"https://example.com"},
			Result:  Result{Status: StatusUnconfigured},
		},
		"Error returned by ChangeDetection.UpdateWatcher": {
			CD: &fakeChangeDetection{
				watch:     &webchange.Watch{URL: "https://old.example.com"},
				updateErr: assert.AnError,
			},
			State:   watcherState(t, "w-1", null.Time{}),
			Updated: [][2]string{{"w-1", "https://example.com"}},
			Err:     assert.AnError,
		},
		"Error returned by ChangeDetection.CreateWatcher": {
			CD:      &fakeChangeDetection{createErr: assert.AnError},
			Created: []string{"https://example.com"},
			Err:     assert.AnError,
		},
		"Malformed state": {
			CD:    &fakeChangeDetection{},
			State: State(`{not json`),
			Err:   assert.AnError,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			uw := URLWatcher{URL: "https://example.com"}

			res, err := uw.Reset(context.Background(), stubInput{state: c.State, cd: c.CD})
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Created, c.CD.createdURLs)
			assert.Equal(t, c.Updated, c.CD.updated)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result.Status, res.Status)
			assert.True(t, res.Score.Equal(c.Result.Score), "score %s", res.Score)
			assert.Equal(t, c.Result.State, res.State)
		})
	}
}

func Test_URLWatcher_Delete(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		CD      *fakeChangeDetection
		State   State
		Deleted []string
		Err     error
	}{
		"Missing state is a no-op": {
			CD: &fakeChangeDetection{},
		},
		"State without a watcher is a no-op": {
			CD:    &fakeChangeDetection{},
			State: State(`{}`),
		},
		"Malformed state": {
			CD:    &fakeChangeDetection{},
			State: State(`{not json`),
			Err:   assert.AnError,
		},
		"Error returned by ChangeDetection.DeleteWatcher": {
			CD:      &fakeChangeDetection{deleteErr: assert.AnError},
			State:   watcherState(t, "w-1", null.Time{}),
			Deleted: []string{"w-1"},
			Err:     assert.AnError,
		},
		"Existing watcher is deleted": {
			CD:      &fakeChangeDetection{},
			State:   watcherState(t, "w-1", null.Time{}),
			Deleted: []string{"w-1"},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			err := (&URLWatcher{}).Delete(context.Background(), stubInput{state: c.State, cd: c.CD})
			testutil.AssertEqualError(t, c.Err, err)

			assert.Equal(t, c.Deleted, c.CD.deletedIDs)
		})
	}
}
