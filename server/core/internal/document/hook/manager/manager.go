// Package manager periodically processes document freshness hooks.
package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

const (
	// _processingBatch defines how many hooks to process in one batch.
	_processingBatch = 100

	// _processingInterval defines how often to process hooks.
	_processingInterval = time.Minute * 5

	// _hookRetentionDuration defines how long to retain inactive hooks.
	_hookRetentionDuration = time.Hour * 24
)

// Manager manages document freshness hooks.
type Manager struct {
	log             *slog.Logger
	db              DB
	githubMan       *github.Manager
	webchangeClient *webchange.Client
	notifPub        notification.Publisher
}

// NewManager creates a new Manager with the given database interface.
func NewManager(
	log *slog.Logger,
	db DB,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	notifPub notification.Publisher,
) *Manager {
	return &Manager{
		log:             log.With("component", "document-hooks-manager"),
		db:              db,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		notifPub:        notifPub,
	}
}

// Start begins the periodic processing of document hooks.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("starting")
	defer m.log.Info("stopped")

	timeutil.NewPeriodicExec(
		_processingInterval,
		0,
		func(ctx context.Context) {
			if err := m.processHooks(ctx); err != nil {
				m.log.With("error", err).
					Error("processing document hooks")
			}
		},
		logutil.RecoveryValue(m.log, logutil.NewRecoveryPlan("recovered from a panic while processing document hooks")),
		true,
	).Start(ctx)
}

// ProcessingState holds the state during hook processing.
type ProcessingState struct {
	// OffsetID is the last processed hook ID.
	OffsetID xid.ID

	// Documents caches documents by branch ID to avoid redundant fetches;
	// a branch belongs to exactly one document.
	Documents map[xid.ID]*document.Document
}

// processHooks processes document hooks in a paginated manner.
func (m *Manager) processHooks(ctx context.Context) error {
	ps := &ProcessingState{
		Documents: make(map[xid.ID]*document.Document),
	}

	for {
		hooks, err := m.db.FetchPaginatedDocumentHooks(ctx, ps.OffsetID, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document hooks: %w", err)
		}

		for _, h := range hooks {
			ps.OffsetID = h.ID

			// The hook was cut loose — its branch, document or whole
			// organization was deleted, or a merge replaced the branch's
			// hooks — and the row is the only trace left of it: tear down
			// the external resource it holds before the row goes away with
			// the last reference to it.
			if !h.BranchID.Valid || !h.DocumentID.Valid || !h.OrganizationID.Valid {
				m.deleteHook(ctx, &h)

				continue
			}

			if m.skipsUnconfigured(&h) {
				continue
			}

			previousScore := h.Score

			ok := m.ensureHook(ctx, ps, &h)
			if !ok {
				continue
			}

			// a soft-deleted hook's block is gone from the document, so its
			// score describes nothing and a zero-score notification would
			// point at a block that no longer exists. The mark is still
			// persisted — it starts the retention clock — and processing
			// resumes when the block reappears and ensureHook clears it.
			if h.SoftDeletedAt.Valid {
				m.updateHook(ctx, h)

				continue
			}

			err = h.Process(ctx, hook.NewInput(
				h.OrganizationID.String,
				m.githubMan,
				m.webchangeClient,
			))
			if err != nil {
				m.log.With("hook_id", h.ID).
					With("error", err).
					Error("processing document hook")

				// a soft-deletion mark ensureHook just cleared for a
				// reappeared block is persisted even here: leaving it stored
				// would keep the retention clock running toward deleting a
				// hook whose block is back.
				m.updateHook(ctx, h)

				continue
			}

			// a notification for a score that was not persisted would be
			// published again on every cycle, since the next one recomputes
			// the same transition from the same stored score.
			if !m.updateHook(ctx, h) {
				continue
			}

			// the score decays gradually — a scheduled reminder walks down
			// through 99…1 — so the transition to watch for is the arrival
			// at zero, not an exact full-to-zero jump within one cycle.
			if !previousScore.IsZero() && h.Score.IsZero() {
				m.notifyMaintainers(ctx, h)
			}
		}

		if len(hooks) < _processingBatch {
			break
		}
	}

	return nil
}

// notifyMaintainers tells the document's maintainers that the hook ran out of
// freshness.
func (m *Manager) notifyMaintainers(ctx context.Context, h hook.Hook) {
	maintainers, err := m.db.FetchDocumentMaintainers(ctx, h.DocumentID.V, h.OrganizationID.String)
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("fetching document maintainers for hook notification")

		return
	}

	m.notifPub.PublishNotifications(
		h.OrganizationID.String,
		notification.NewDocumentHookTriggeredNotification(
			h.DocumentID.V,
			h.Type,
			h.BlockID,
			h.BranchID.V,
		),
		maintainers...,
	)
}

// ensureHook ensures the hook is valid and handles deletions if necessary.
func (m *Manager) ensureHook(
	ctx context.Context,
	state *ProcessingState,
	h *hook.Hook,
) bool {
	key := h.BranchID.V

	doc, ok := state.Documents[key]
	if !ok {
		var err error

		doc, err = m.db.FetchDocumentByBranchID(ctx, h.BranchID.V, h.OrganizationID.String)

		switch {
		case err == nil:
			state.Documents[key] = doc
		case errutil.IsNotFound(err):
			state.Documents[key] = nil
		default:
			m.log.With("hook_id", h.ID).
				With("error", err).
				Error("fetching document for hook")

			return false
		}
	}

	if doc == nil || (h.SoftDeletedAt.Valid && h.SoftDeletedAt.Time.Before(timeutil.Now().Add(-_hookRetentionDuration))) {
		m.deleteHook(ctx, h)

		return false
	}

	if h.BlockID.Valid {
		hasBlock := doc.Content.HasBlock(h.BlockID.String)

		if !hasBlock && !h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.TimeFrom(timeutil.Now())
		} else if hasBlock && h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.Time{}
		}
	}

	return true
}

// skipsUnconfigured reports whether the hook depends on an integration
// this deployment does not have. Such a hook cannot make progress, so
// the processing pass skips it and leaves its state untouched.
func (m *Manager) skipsUnconfigured(h *hook.Hook) bool {
	switch {
	case h.Type == hook.TypeGithubTracking && !m.githubMan.Configured():
		m.log.With("hook_id", h.ID).
			Warn("skipping github-tracking hook: github app is not configured")

		return true
	case h.Type == hook.TypeURLWatcher && !m.webchangeClient.Configured():
		m.log.With("hook_id", h.ID).
			Warn("skipping url-watcher hook: changedetection is not configured")

		return true
	default:
		return false
	}
}

// deleteHook tears down the hook's external resource and then removes the
// row describing it. The external teardown goes first: the row is the only
// record of the resource, so dropping it first would strand the watcher
// with nothing left to find it by.
func (m *Manager) deleteHook(ctx context.Context, h *hook.Hook) {
	err := h.Delete(ctx, hook.NewInput(
		h.OrganizationID.String,
		m.githubMan,
		m.webchangeClient,
	))
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("deleting hook external resource")

		return
	}

	err = m.db.DeleteDocumentHook(ctx, h.ID)
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("deleting hook from db")
	}
}

// updateHook persists the hook's current state and reports whether it stuck.
func (m *Manager) updateHook(ctx context.Context, h hook.Hook) bool {
	err := m.db.UpdateDocumentHook(ctx, h)
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("updating document hook")

		return false
	}

	return true
}

// DB defines the database operations required by the Manager.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchPaginatedDocumentHooks should retrieve a paginated list of document hooks
	// starting after the given offset ID, limited to the specified number of hooks.
	FetchPaginatedDocumentHooks(ctx context.Context, offsetID xid.ID, limit int64) ([]hook.Hook, error)

	// UpdateDocumentHook should update the given document hook in the database.
	UpdateDocumentHook(ctx context.Context, hk hook.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID) error

	// FetchDocumentByBranchID should fetch the document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentMaintainers should fetch the document maintainers.
	FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error)
}
