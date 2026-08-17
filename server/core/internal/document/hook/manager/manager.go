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
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/shopspring/decimal"
)

const (
	// _processingBatch defines how many hooks to process in one batch.
	_processingBatch = 100

	// _processingInterval defines how often to process hooks.
	_processingInterval = time.Minute * 5

	// _hookRetentionDuration defines how long to retain inactive hooks.
	_hookRetentionDuration = time.Hour * 24
)

// _fullScorePercent is the maximum freshness score in percent.
const _fullScorePercent = 100

// _fullScore is the maximum freshness score (100%) a hook can report.
var _fullScore = decimal.NewFromInt(_fullScorePercent)

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

	tm := time.NewTimer(0)
	defer tm.Stop()

	for {
		select {
		case <-tm.C:
			err := m.processHooks(ctx)
			if err != nil {
				m.log.With("error", err).
					Error("processing document hooks")
			}

			tm.Reset(_processingInterval)
		case <-ctx.Done():
			return
		}
	}
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

			// The branch, the document or the whole organization was
			// deleted, which is the only trace left of the hook: tear down
			// the external resource it holds before the row goes away with
			// the last reference to it.
			if !h.BranchID.Valid || !h.DocumentID.Valid || !h.OrganizationID.Valid {
				m.deleteHook(ctx, &h)

				continue
			}

			// github-tracking hooks cannot make progress without a
			// configured GitHub App; skip them and leave their state
			// untouched.
			if h.Type == hook.TypeGithubTracking && !m.githubMan.Configured() {
				m.log.With("hook_id", h.ID).
					Warn("skipping github-tracking hook: github app is not configured")

				continue
			}

			previousScore := h.Score

			ok := m.ensureHook(ctx, ps, &h)
			if !ok {
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

				// the soft-deletion mark set by ensureHook is persisted even
				// here: it starts the retention clock, and a hook that keeps
				// failing to process is exactly the one that would otherwise
				// never reach it.
				m.updateHook(ctx, h)

				continue
			}

			m.updateHook(ctx, h)

			if previousScore.Equal(_fullScore) && h.Score.Equal(decimal.Zero) {
				maintainers, err := m.db.FetchDocumentMaintainers(ctx, h.DocumentID.V, h.OrganizationID.String)
				if err != nil {
					m.log.With("hook_id", h.ID).
						With("error", err).
						Error("fetching document maintainers for hook notification")

					continue
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
		}

		if len(hooks) < _processingBatch {
			break
		}

		ps.OffsetID = hooks[len(hooks)-1].ID
	}

	return nil
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

// updateHook persists the hook's current state.
func (m *Manager) updateHook(ctx context.Context, h hook.Hook) {
	err := m.db.UpdateDocumentHook(ctx, h)
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("updating document hook")
	}
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
