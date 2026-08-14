package processor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/guregu/null/v5"
	"github.com/shopspring/decimal"
)

// URLWatcherStatus represents the status of the
// URL watcher processor.
type URLWatcherStatus string

const (
	// URLWatcherStatusActive indicates that the URL watcher
	// processor is active.
	URLWatcherStatusActive URLWatcherStatus = "active"

	// URLWatcherStatusUnreachableURL indicates that the URL being
	// watched is unreachable.
	URLWatcherStatusUnreachableURL URLWatcherStatus = "unreachable_url"
)

// URLWatcher specifies a processor that watches a URL for changes.
type URLWatcher struct {
	// URL is the URL to watch.
	URL string `json:"url"`
}

// Process processes the URL watcher hook.
func (uw *URLWatcher) Process(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	var uws URLWatcherState

	if err := json.Unmarshal(inp.State(), &uws); err != nil {
		return decimal.Zero, nil, fmt.Errorf("unmarshaling url watcher state: %w", err)
	}

	watch, err := inp.ChangeDetection().FetchWatcher(ctx, uws.WatcherID)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("fetching url watcher: %w", err)
	}

	if watch.Unreachable {
		uws.Status = URLWatcherStatusUnreachableURL

		state, merr := json.Marshal(uws)
		if merr != nil {
			return decimal.Zero, nil, fmt.Errorf("marshaling url watcher state: %w", merr)
		}

		return decimal.Zero, state, nil
	}

	uws.Status = URLWatcherStatusActive

	score := _fullScore

	if !uws.LastChangedAt.Valid {
		uws.LastChangedAt = null.TimeFrom(watch.LastChangedAt)
	} else if watch.LastChangedAt.After(uws.LastChangedAt.Time) {
		score = decimal.Zero
	}

	state, err := json.Marshal(uws)
	if err != nil {
		return decimal.Zero, nil, fmt.Errorf("marshaling url watcher state: %w", err)
	}

	return score, state, nil
}

// Reset resets the state of the URL watcher processor.
func (uw *URLWatcher) Reset(ctx context.Context, inp Input) (decimal.Decimal, State, error) {
	var uws URLWatcherState

	if state := inp.State(); state != nil {
		if err := json.Unmarshal(state, &uws); err != nil {
			return decimal.Zero, nil, fmt.Errorf("unmarshaling url watcher state: %w", err)
		}
	}

	if uws.WatcherID == "" { //nolint:nestif // the branching is sequential and readable
		watcherID, err := inp.ChangeDetection().CreateWatcher(ctx, uw.URL)
		if err != nil {
			return decimal.Zero, nil, fmt.Errorf("creating url watcher: %w", err)
		}

		uws.WatcherID = watcherID
		uws.LastChangedAt = null.Time{}
	} else {
		watch, err := inp.ChangeDetection().FetchWatcher(ctx, uws.WatcherID)
		if err != nil {
			return decimal.Zero, nil, fmt.Errorf("fetching url watcher: %w", err)
		}

		uws.LastChangedAt = null.TimeFrom(watch.LastChangedAt)

		if watch.URL != uw.URL {
			err := inp.ChangeDetection().UpdateWatcher(ctx, uws.WatcherID, uw.URL)
			if err != nil {
				return decimal.Zero, nil, fmt.Errorf("updating url watcher: %w", err)
			}

			uws.LastChangedAt = null.Time{}
		}
	}

	uws.Status = URLWatcherStatusActive

	state, err := json.Marshal(uws)
	if err != nil {
		return decimal.Zero, nil, err
	}

	return _fullScore, state, nil
}

// Delete deletes the URL watcher processor's state.
func (uw *URLWatcher) Delete(ctx context.Context, inp Input) error {
	var uws URLWatcherState

	if inp.State() == nil {
		return nil
	}

	if err := json.Unmarshal(inp.State(), &uws); err != nil {
		return fmt.Errorf("unmarshaling url watcher state: %w", err)
	}

	err := inp.ChangeDetection().DeleteWatcher(ctx, uws.WatcherID)
	if err != nil {
		return fmt.Errorf("deleting url watcher: %w", err)
	}

	return nil
}

// URLWatcherState represents the state of the URL watcher processor.
type URLWatcherState struct {
	// Status is the current status of the URL watcher.
	Status URLWatcherStatus `json:"status"`

	// WatcherID is the ID of the URL watcher.
	WatcherID string `json:"watcherId"`

	// LastChangedAt is the timestamp of the last change detected.
	LastChangedAt null.Time `json:"lastChangedAt"`
}
