// Package hook defines document freshness hooks and their processing contract.
package hook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
)

// ErrInvalidType is returned when a hook carries a type the processors do
// not implement.
var ErrInvalidType = errutil.New(http.StatusBadRequest, "document_hook.invalid_type", "invalid hook type")

// ErrInvalidSettings is returned when a hook's settings do not decode.
var ErrInvalidSettings = errutil.New(http.StatusBadRequest, "document_hook.invalid_settings", "invalid hook settings")

// ErrUpstreamUnavailable is returned when the service a hook checks could
// not be reached while the hook was created or changed.
var ErrUpstreamUnavailable = errutil.New(http.StatusFailedDependency, "document_hook.upstream_unavailable", "the service the hook checks is unavailable")

// Type represents the type of a freshness hook.
type Type string

// Validate checks whether the hook type is one of the known variants.
func (t Type) Validate() error {
	switch t {
	case TypeScheduledReminder, TypeGithubTracking, TypeURLWatcher, TypeContainerImageWatcher:
		return nil
	default:
		return ErrInvalidType
	}
}

// HumanizedString returns a human-readable string for the hook type.
func (t Type) HumanizedString() string {
	switch t {
	case TypeScheduledReminder:
		return "Scheduled Reminder"
	case TypeGithubTracking:
		return "GitHub Tracking"
	case TypeURLWatcher:
		return "Website Changes"
	case TypeContainerImageWatcher:
		return "Container Image Updates"
	default:
		return "Unknown"
	}
}

const (
	// TypeScheduledReminder represents a hook that is triggered based on
	// a scheduled time.
	TypeScheduledReminder Type = "scheduled-reminder"

	// TypeGithubTracking represents a hook that is triggered based on
	// GitHub activity.
	TypeGithubTracking Type = "github-tracking"

	// TypeURLWatcher represents a hook that watches a URL for changes.
	TypeURLWatcher Type = "url-watcher"

	// TypeContainerImageWatcher represents a hook that watches container images for updates.
	TypeContainerImageWatcher Type = "container-image-watcher"
)

// Hook represents a freshness hook that is associated with a
// document or block.
type Hook struct {
	// ID is the unique identifier for the hook.
	ID xid.ID `json:"id" db:"id"`

	// Type is the type of the hook.
	Type Type `json:"type" db:"type"`

	// DocumentID is the ID of the document that this hook is associated with.
	// Null when the document has been deleted; the manager will clean up
	// such hooks.
	DocumentID null.Value[xid.ID] `json:"documentId" db:"fk_document_id"`

	// OrganizationID is the ID of the organization that owns the hook.
	// Null when the organization has been deleted; the manager will clean up
	// such hooks, tearing down the external resource they hold first.
	OrganizationID null.String `json:"organizationId" db:"fk_organization_id"`

	// BranchID is the ID of the document branch this hook is associated with.
	// Null when the branch has been deleted; the manager will clean up such
	// hooks, tearing down the external resource they hold first.
	BranchID null.Value[xid.ID] `json:"branchId" db:"fk_branch_id"`

	// BlockID is the ID of the block that this hook is associated with.
	// If null, hook is associated with the document.
	BlockID null.String `json:"blockId" db:"block_id"`

	// Settings contains the settings for the hook in JSON format.
	Settings processor.Settings `json:"settings" db:"settings"`

	// State contains the state of the hook in JSON format. Null until the
	// hook is set up.
	State null.Value[processor.State] `json:"state" db:"state"`

	// Status tells whether the hook could check its target on its last run.
	// Score and state are left as they were while it is not active.
	Status processor.Status `json:"status" db:"status"`

	// Score is the freshness score of the hook.
	Score decimal.Decimal `json:"score" db:"score"`

	// CreatedAt is the time when the hook was created.
	CreatedAt time.Time `json:"createdAt" db:"created_at"`

	// UpdatedAt is the timestamp when the document was last updated.
	UpdatedAt null.Time `json:"updatedAt" db:"updated_at"`

	// SoftDeletedAt is the time when the hook was soft deleted (not found in the document).
	// Once some time passes after soft deletion, the hook can be permanently deleted.
	SoftDeletedAt null.Time `json:"softDeletedAt" db:"soft_deleted_at"`

	prepared bool
	runner   runner
}

// NewHook creates a new freshness hook with the given input and document ID.
// It fails unless the hook can check its target right away.
func NewHook(
	ctx context.Context,
	ci CreateInput,
	documentID xid.ID,
	branchID xid.ID,
	organizationID string,
	inp *Input,
) (*Hook, error) {
	h := Hook{
		ID:             xid.New(),
		Type:           ci.Type,
		DocumentID:     null.ValueFrom(documentID),
		OrganizationID: null.StringFrom(organizationID),
		BranchID:       null.ValueFrom(branchID),
		BlockID:        ci.BlockID,
		Settings:       ci.Settings,
		CreatedAt:      timeutil.Now(),
	}

	if err := h.validate(); err != nil {
		return nil, err
	}

	if err := h.Reset(ctx, inp); err != nil {
		return nil, err
	}

	if err := h.requireActive(); err != nil {
		return nil, err
	}

	return &h, nil
}

// NewCopy returns a copy of the hook for another branch, anchored to the
// given block. Its state is null, so it holds no external resource until
// its first run sets it up. The source's state is never copied: it can
// name the source's own watcher. Score and status are, so the first run
// does not announce a change the source already had.
func (h *Hook) NewCopy(documentID, branchID xid.ID, blockID null.String) Hook {
	return Hook{
		ID:             xid.New(),
		Type:           h.Type,
		DocumentID:     null.ValueFrom(documentID),
		OrganizationID: h.OrganizationID,
		BranchID:       null.ValueFrom(branchID),
		BlockID:        blockID,
		Settings:       h.Settings,
		Status:         h.Status,
		Score:          h.Score,
		CreatedAt:      timeutil.Now(),
	}
}

// ApplyUpdate applies the new settings and resets the hook. It fails
// unless the hook can check its target with them.
func (h *Hook) ApplyUpdate(ctx context.Context, ui UpdateInput, inp *Input) error {
	h.Settings = ui.Settings
	h.UpdatedAt = null.TimeFrom(timeutil.Now())

	// the runner is built from the settings; drop the memoized one so the
	// reset below sees the updated settings.
	h.prepared = false
	h.runner = nil

	if err := h.validate(); err != nil {
		return err
	}

	if err := h.Reset(ctx, inp); err != nil {
		return err
	}

	return h.requireActive()
}

// Process processes the hook and updates its score and state. A hook that
// was never set up is reset instead.
func (h *Hook) Process(ctx context.Context, inp *Input) error {
	if !h.State.Valid {
		return h.Reset(ctx, inp)
	}

	if err := h.ensurePrepared(); err != nil {
		return err
	}

	res, err := h.runner.Process(ctx, newStateInput(inp, h.State.V))
	if err != nil {
		return err
	}

	h.apply(res)

	return nil
}

// Reset resets the state of the hook to its initial state.
func (h *Hook) Reset(ctx context.Context, inp *Input) error {
	if err := h.ensurePrepared(); err != nil {
		return err
	}

	res, err := h.runner.Reset(ctx, newStateInput(inp, h.State.V))
	if err != nil {
		return err
	}

	h.apply(res)

	return nil
}

// Delete cleans up any external resources associated with the hook. A hook
// that was never set up holds none.
func (h *Hook) Delete(ctx context.Context, inp *Input) error {
	if !h.State.Valid {
		return nil
	}

	if err := h.ensurePrepared(); err != nil {
		return err
	}

	if err := h.runner.Delete(ctx, newStateInput(inp, h.State.V)); err != nil {
		return err
	}

	return nil
}

// ChangedFrom reports whether the hook differs from prev in what editors
// show of it.
func (h *Hook) ChangedFrom(prev Hook) bool {
	return h.Status != prev.Status ||
		!h.Score.Equal(prev.Score) ||
		h.State.Valid != prev.State.Valid ||
		h.SoftDeletedAt.Valid != prev.SoftDeletedAt.Valid
}

// apply stores a run's result. A run that could not check its target
// leaves the score and state as they were.
func (h *Hook) apply(res processor.Result) {
	h.Status = res.Status

	if res.Status != processor.StatusActive {
		return
	}

	h.Score = res.Score
	h.State = null.ValueFrom(res.State)
}

// validate checks the type and the settings before anything runs.
func (h *Hook) validate() error {
	if err := h.Type.Validate(); err != nil {
		return err
	}

	if err := h.ensurePrepared(); err != nil {
		return ErrInvalidSettings
	}

	return h.runner.Validate()
}

// requireActive fails with an error named after the status unless the hook
// could check its target.
func (h *Hook) requireActive() error {
	if h.Status == processor.StatusActive {
		return nil
	}

	return errutil.New(
		http.StatusUnprocessableEntity,
		"document_hook."+string(h.Status),
		"the hook cannot check its target: %s",
		h.Status,
	)
}

// ensurePrepared prepares the hook for processing.
func (h *Hook) ensurePrepared() error {
	if h.prepared {
		return nil
	}

	switch h.Type {
	case TypeScheduledReminder:
		var sr processor.ScheduledReminder

		if err := json.Unmarshal(h.Settings, &sr); err != nil {
			return err
		}

		h.runner = &sr
		h.prepared = true

		return nil
	case TypeGithubTracking:
		var gt processor.GithubTracking

		if err := json.Unmarshal(h.Settings, &gt); err != nil {
			return err
		}

		h.runner = &gt
		h.prepared = true

		return nil
	case TypeURLWatcher:
		var uw processor.URLWatcher

		if err := json.Unmarshal(h.Settings, &uw); err != nil {
			return err
		}

		h.runner = &uw
		h.prepared = true

		return nil
	case TypeContainerImageWatcher:
		var ciw processor.ContainerImageWatcher

		if err := json.Unmarshal(h.Settings, &ciw); err != nil {
			return err
		}

		h.runner = &ciw
		h.prepared = true

		return nil
	default:
		return errors.New("invalid processor type")
	}
}

// CreateInput represents the data required to create a new freshness hook.
type CreateInput struct {
	// Type is the type of the hook.
	Type Type `json:"type"`

	// BranchID is the ID of the document branch this hook is to be created on.
	BranchID xid.ID `json:"branchId"`

	// BlockID is the ID of the block that this hook is associated with.
	// If null, hook is associated with the document.
	BlockID null.String `json:"blockId"`

	// Settings contains the settings for the hook in JSON format.
	Settings processor.Settings `json:"settings"`
}

// UpdateInput represents the data required to update an existing freshness hook.
type UpdateInput struct {
	// Settings contains the settings for the hook in JSON format.
	Settings processor.Settings `json:"settings"`
}

// runner is an interface that defines the methods for applying a freshness hook.
type runner interface {
	// Validate checks the settings without reaching any service.
	Validate() error

	// Process calculates the freshness score for the hook.
	Process(ctx context.Context, inp processor.Input) (processor.Result, error)

	// Reset resets the state of the hook to its initial state.
	Reset(ctx context.Context, inp processor.Input) (processor.Result, error)

	// Delete cleans up any external resources associated with the hook.
	Delete(ctx context.Context, inp processor.Input) error
}
