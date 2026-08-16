package processor

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/shopspring/decimal"
)

// _fullScorePercent is the maximum freshness score in percent.
const _fullScorePercent = 100

// _fullScore is the maximum freshness score (100%) a hook can report.
var _fullScore = decimal.NewFromInt(_fullScorePercent)

// Input represents an input that provides state information
// for processing freshness hooks.
type Input interface {
	// State returns the state of the input in JSON format.
	State() State

	// Github should return a GitHub client interface.
	Github(ctx context.Context) (Github, error)

	// ChangeDetection should return a ChangeDetection client interface.
	ChangeDetection() ChangeDetection
}

// State represents the state of a processor in JSON format.
type State json.RawMessage

// MarshalJSON returns the JSON representation of the state.
func (s State) MarshalJSON() ([]byte, error) {
	// mirror json.RawMessage: a nil value marshals as JSON null instead
	// of failing the entire enclosing marshal.
	if s == nil {
		return []byte("null"), nil
	}

	return s, nil
}

// UnmarshalJSON sets the state from the given JSON data.
func (s *State) UnmarshalJSON(data []byte) error {
	*s = data

	return nil
}

// Value transforms state into a database entry.
func (s State) Value() (driver.Value, error) {
	return []byte(s), nil
}

// Scan transforms a database entry into a state type.
func (s *State) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*s = State(v)
	case []byte:
		// the driver may reuse the slice after Scan returns, so it has to be
		// copied rather than aliased.
		*s = append(State(nil), v...)
	default:
		return errors.New("invalid state type")
	}

	return nil
}

// Settings represents the settings of a processor in JSON format.
type Settings json.RawMessage

// MarshalJSON returns the JSON representation of the settings.
func (s Settings) MarshalJSON() ([]byte, error) {
	// mirror json.RawMessage: a nil value marshals as JSON null instead
	// of failing the entire enclosing marshal.
	if s == nil {
		return []byte("null"), nil
	}

	return s, nil
}

// UnmarshalJSON sets the settings from the given JSON data.
func (s *Settings) UnmarshalJSON(data []byte) error {
	*s = data

	return nil
}

// Value transforms settings into a database entry.
func (s Settings) Value() (driver.Value, error) {
	return []byte(s), nil
}

// Scan transforms a database entry into a settings type.
func (s *Settings) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*s = Settings(v)
	case []byte:
		// the driver may reuse the slice after Scan returns, so it has to be
		// copied rather than aliased.
		*s = append(Settings(nil), v...)
	default:
		return errors.New("invalid settings type")
	}

	return nil
}

// Github represents a GitHub client interface.
type Github interface {
	// FetchRepositoryTree fetches the file tree of a repository at a specific branch.
	FetchRepositoryTree(ctx context.Context, repository, branch string) (github.Tree, error)
}

// ChangeDetection represents an interface for detecting changes in URLs.
type ChangeDetection interface {
	// CreateWatcher creates a new watcher for change detection.
	CreateWatcher(ctx context.Context, url string) (string, error)

	// FetchWatcher fetches the watcher with the given watch ID.
	FetchWatcher(ctx context.Context, watchID string) (*webchange.Watch, error)

	// UpdateWatcher updates a watcher for change detection.
	UpdateWatcher(ctx context.Context, watchID, url string) error

	// DeleteWatcher deletes the watcher with the given watch ID.
	DeleteWatcher(ctx context.Context, watchID string) error
}
