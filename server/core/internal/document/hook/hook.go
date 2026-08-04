package hook

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
)

// Type represents the type of a freshness hook.
type Type string

// HumanizedString returns a human-readable string for the hook type.
func (t Type) HumanizedString() string {
	switch t {
	case TypeScheduledReminder:
		return "Scheduled Reminder"
	case TypeGithubTracking:
		return "GitHub Tracking"
	case TypeURLWatcher:
		return "Website Changes"
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
	DocumentID xid.ID `json:"documentId" db:"fk_document_id"`

	// OrganizationID is the ID of the organization that owns the hook.
	OrganizationID string `json:"organizationId" db:"fk_organization_id"`

	// BranchID is the ID of the document branch this hook is associated with.
	// Null when the branch has been deleted; the manager will clean up such hooks.
	BranchID null.Value[xid.ID] `json:"branchId" db:"fk_branch_id"`

	// BlockID is the ID of the block that this hook is associated with.
	// If null, hook is associated with the document.
	BlockID null.String `json:"blockId" db:"block_id"`

	// Settings contains the settings for the hook in JSON format.
	Settings processor.Settings `json:"settings" db:"settings"`

	// State contains the state of the hook in JSON format.
	State processor.State `json:"state" db:"state"`

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
		DocumentID:     documentID,
		OrganizationID: organizationID,
		BranchID:       null.ValueFrom(branchID),
		BlockID:        ci.BlockID,
		Settings:       ci.Settings,
		CreatedAt:      timeutil.Now(),
	}

	if err := h.Reset(ctx, inp); err != nil {
		return nil, err
	}

	return &h, nil
}

// ApplyUpdate updates the freshness hook with the given input.
func (h *Hook) ApplyUpdate(ctx context.Context, ui UpdateInput, inp *Input) error {
	h.Settings = ui.Settings
	h.UpdatedAt = null.TimeFrom(timeutil.Now())

	if err := h.Reset(ctx, inp); err != nil {
		return err
	}

	return nil
}

// Process processes the hook and updates its score and state.
func (h *Hook) Process(ctx context.Context, inp *Input) error {
	if err := h.ensurePrepared(); err != nil {
		return err
	}

	score, state, err := h.runner.Process(ctx, newStateInput(inp, h.State))
	if err != nil {
		return err
	}

	h.Score = score
	h.State = state

	return nil
}

// Reset resets the state of the hook to its initial state.
func (h *Hook) Reset(ctx context.Context, inp *Input) error {
	if err := h.ensurePrepared(); err != nil {
		return err
	}

	score, state, err := h.runner.Reset(ctx, newStateInput(inp, h.State))
	if err != nil {
		return err
	}

	h.Score = score
	h.State = state

	return nil
}

// Delete cleans up any external resources associated with the hook.
func (h *Hook) Delete(ctx context.Context, inp *Input) error {
	if err := h.ensurePrepared(); err != nil {
		return err
	}

	if err := h.runner.Delete(ctx, newStateInput(inp, h.State)); err != nil {
		return err
	}

	return nil
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
	// Process calculates the freshness score for the hook.
	Process(ctx context.Context, inp processor.Input) (decimal.Decimal, processor.State, error)

	// Reset resets the state of the hook to its initial state.
	Reset(ctx context.Context, inp processor.Input) (decimal.Decimal, processor.State, error)

	// Delete cleans up any external resources associated with the hook.
	Delete(ctx context.Context, inp processor.Input) error
}
