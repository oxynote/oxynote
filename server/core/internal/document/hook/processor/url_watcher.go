package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/mathutil"
	"github.com/shopspring/decimal"
)

// ErrInvalidURL is returned when a URL watcher's URL is not an absolute
// http(s) URL.
var ErrInvalidURL = errutil.New(http.StatusBadRequest, "document_hook.invalid_url", "invalid url")

// URLWatcher specifies a processor that watches a URL for changes.
type URLWatcher struct {
	// URL is the URL to watch.
	URL string `json:"url"`
}

// Validate checks that the URL is an absolute http(s) URL. Reachability is
// left to changedetection, so core never fetches user URLs itself.
func (uw *URLWatcher) Validate() error {
	u, err := url.Parse(uw.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ErrInvalidURL
	}

	return nil
}

// Process processes the URL watcher hook.
func (uw *URLWatcher) Process(ctx context.Context, inp Input) (Result, error) {
	var uws URLWatcherState

	if err := json.Unmarshal(inp.State(), &uws); err != nil {
		return Result{}, fmt.Errorf("unmarshaling url watcher state: %w", err)
	}

	watch, err := inp.ChangeDetection().FetchWatcher(ctx, uws.WatcherID)

	switch {
	case err == nil:
		// OK.
	case errors.Is(err, webchange.ErrWatcherNotFound):
		// a watcher removed on the changedetection.io side would otherwise
		// fail this hook on every cycle forever. Recreate it.
		return uw.Reset(ctx, inp)
	case errors.Is(err, webchange.ErrNotConfigured):
		return inactive(StatusUnconfigured), nil
	default:
		return Result{}, fmt.Errorf("fetching url watcher: %w", err)
	}

	// a write that failed after moving the watcher leaves it on a URL the
	// settings no longer name. Point it back.
	if watch.URL != uw.URL {
		return uw.Reset(ctx, inp)
	}

	if watch.Unreachable {
		return inactive(StatusUnreachableURL), nil
	}

	score := mathutil.Hundred

	if !uws.LastChangedAt.Valid {
		uws.LastChangedAt = null.TimeFrom(watch.LastChangedAt)
	} else if watch.LastChangedAt.After(uws.LastChangedAt.Time) {
		score = decimal.Zero
	}

	return active(score, uws)
}

// Reset points the watcher at the current URL and takes its last change
// as the baseline.
func (uw *URLWatcher) Reset(ctx context.Context, inp Input) (Result, error) {
	var uws URLWatcherState

	if state := inp.State(); state != nil {
		if err := json.Unmarshal(state, &uws); err != nil {
			return Result{}, fmt.Errorf("unmarshaling url watcher state: %w", err)
		}
	}

	uws, err := uw.syncWatcher(ctx, inp.ChangeDetection(), uws.WatcherID)

	switch {
	case err == nil:
		return active(mathutil.Hundred, uws)
	case errors.Is(err, webchange.ErrNotConfigured):
		return inactive(StatusUnconfigured), nil
	default:
		return Result{}, err
	}
}

// syncWatcher points the watcher at the URL and returns the state to start
// from. A watcher that is not named or no longer exists is created.
func (uw *URLWatcher) syncWatcher(ctx context.Context, cd ChangeDetection, watcherID string) (URLWatcherState, error) {
	if watcherID != "" {
		watch, err := cd.FetchWatcher(ctx, watcherID)

		switch {
		case err == nil && watch.URL == uw.URL:
			return URLWatcherState{
				WatcherID:     watcherID,
				LastChangedAt: null.TimeFrom(watch.LastChangedAt),
			}, nil
		case err == nil:
			if err = cd.UpdateWatcher(ctx, watcherID, uw.URL); err != nil {
				return URLWatcherState{}, fmt.Errorf("updating url watcher: %w", err)
			}

			return URLWatcherState{WatcherID: watcherID}, nil
		case !errors.Is(err, webchange.ErrWatcherNotFound):
			return URLWatcherState{}, fmt.Errorf("fetching url watcher: %w", err)
		}
	}

	watcherID, err := cd.CreateWatcher(ctx, uw.URL)
	if err != nil {
		return URLWatcherState{}, fmt.Errorf("creating url watcher: %w", err)
	}

	return URLWatcherState{WatcherID: watcherID}, nil
}

// Delete deletes the watcher the state names.
func (uw *URLWatcher) Delete(ctx context.Context, inp Input) error {
	var uws URLWatcherState

	if inp.State() == nil {
		return nil
	}

	if err := json.Unmarshal(inp.State(), &uws); err != nil {
		return fmt.Errorf("unmarshaling url watcher state: %w", err)
	}

	if uws.WatcherID == "" {
		return nil
	}

	err := inp.ChangeDetection().DeleteWatcher(ctx, uws.WatcherID)
	if err != nil {
		return fmt.Errorf("deleting url watcher: %w", err)
	}

	return nil
}

// URLWatcherState represents the state of the URL watcher processor.
type URLWatcherState struct {
	// WatcherID is the ID of the URL watcher.
	WatcherID string `json:"watcherId"`

	// LastChangedAt is the timestamp of the last change detected.
	LastChangedAt null.Time `json:"lastChangedAt"`
}
