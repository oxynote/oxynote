package processor

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/shopspring/decimal"
)

// All available Status constants.
const (
	// StatusActive indicates that the processor checked what it watches.
	StatusActive Status = "active"

	// StatusUnconfigured indicates that the integration the processor needs
	// is not configured on this deployment.
	StatusUnconfigured Status = "unconfigured"

	// StatusMissingInstallation indicates that the organization has no
	// GitHub App installation.
	StatusMissingInstallation Status = "missing_installation"

	// StatusMissingRepository indicates that the tracked GitHub repository
	// was not found.
	StatusMissingRepository Status = "missing_repository"

	// StatusMissingBranch indicates that the tracked branch was not found
	// in the GitHub repository.
	StatusMissingBranch Status = "missing_branch"

	// StatusTreeTruncated indicates that the repository tree is too large
	// for GitHub to return in full, so freshness cannot be determined.
	StatusTreeTruncated Status = "tree_truncated"

	// StatusUnreachableURL indicates that the watched URL is unreachable.
	StatusUnreachableURL Status = "unreachable_url"

	// StatusUnauthorized indicates that the container registry refused
	// access to the watched image.
	StatusUnauthorized Status = "unauthorized"

	// StatusImageNotFound indicates that the registry has no such image or
	// tag.
	StatusImageNotFound Status = "image_not_found"
)

// Status tells whether a processor could check what it watches. Only an
// active result carries a score and a state.
type Status string

// Result is the outcome of a processor run.
type Result struct {
	// Score is the freshness score. It is meaningful only when active.
	Score decimal.Decimal

	// State is the processor's new state. It is meaningful only when
	// active.
	State State

	// Status tells whether the check could run.
	Status Status
}

// inactive returns the result of a run that could not check its target.
func inactive(status Status) Result {
	return Result{Status: status}
}

// active returns the result of a run that checked its target.
func active(score decimal.Decimal, state any) (Result, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return Result{}, fmt.Errorf("marshaling state: %w", err)
	}

	return Result{
		Score:  score,
		State:  raw,
		Status: StatusActive,
	}, nil
}

// Input represents an input that provides state information
// for processing freshness hooks.
type Input interface {
	// State should return the state of the input in JSON format, or nil
	// for a hook that was never set up.
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
	return marshalRawJSON(s)
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
	raw, ok := rawJSONBytes(src)
	if !ok {
		return errors.New("invalid state type")
	}

	*s = State(raw)

	return nil
}

// Settings represents the settings of a processor in JSON format.
type Settings json.RawMessage

// MarshalJSON returns the JSON representation of the settings.
func (s Settings) MarshalJSON() ([]byte, error) {
	return marshalRawJSON(s)
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
	raw, ok := rawJSONBytes(src)
	if !ok {
		return errors.New("invalid settings type")
	}

	*s = Settings(raw)

	return nil
}

// marshalRawJSON mirrors json.RawMessage: a nil value marshals as JSON null
// instead of failing the entire enclosing marshal.
func marshalRawJSON(raw []byte) ([]byte, error) {
	if raw == nil {
		return []byte("null"), nil
	}

	return raw, nil
}

// rawJSONBytes copies a raw JSON database entry into a slice of its own,
// since the driver may reuse the one it hands over once Scan returns. The
// second return value reports whether src carried raw JSON at all.
func rawJSONBytes(src any) ([]byte, bool) {
	switch v := src.(type) {
	case string:
		return []byte(v), true
	case []byte:
		return append([]byte(nil), v...), true
	default:
		return nil, false
	}
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
