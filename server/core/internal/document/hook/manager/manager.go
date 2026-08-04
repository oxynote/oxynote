package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/apps/githubapp"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchanges"
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

// Manager manages document freshness hooks.
type Manager struct {
	log              *slog.Logger
	db               DB
	githubMan        *githubapp.Manager
	webchangesClient *webchanges.Client
	notifPub         notification.NotificationPublisher
}

// NewManager creates a new Manager with the given database interface.
func NewManager(
	log *slog.Logger,
	db DB,
	githubMan *githubapp.Manager,
	webchangesClient *webchanges.Client,
	notifPub notification.NotificationPublisher,
) *Manager {
	return &Manager{
		log:              log.With("component", "document-hooks-manager"),
		db:               db,
		githubMan:        githubMan,
		webchangesClient: webchangesClient,
		notifPub:         notifPub,
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

// documentBranchKey is the cache key for the documents map in ProcessingState.
type documentBranchKey struct {
	// ID is the document ID.
	ID xid.ID

	// BranchID is the document branch ID.
	BranchID null.Value[xid.ID]
}

// ProcessingState holds the state during hook processing.
type ProcessingState struct {
	// OffsetID is the last processed hook ID.
	OffsetID xid.ID

	// Documents caches documents by (ID, branchID) to avoid redundant fetches.
	Documents map[documentBranchKey]*document.Document
}

// processHooks processes document hooks in a paginated manner.
func (m *Manager) processHooks(ctx context.Context) error {
	ps := &ProcessingState{
		Documents: make(map[documentBranchKey]*document.Document),
	}

	for {
		hooks, err := m.db.FetchPaginatedDocumentHooks(ctx, ps.OffsetID, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document hooks: %w", err)
		}

		for _, h := range hooks {
			ps.OffsetID = h.ID

			// Branch was deleted; clean up the orphaned hook immediately.
			if !h.BranchID.Valid {
				if err := m.db.DeleteDocumentHook(ctx, h.ID, h.OrganizationID); err != nil {
					m.log.With("hook_id", h.ID).
						With("error", err).
						Error("deleting orphaned hook for deleted branch")
				}

				continue
			}

			previousScore := h.Score

			ok := m.ensureHook(ctx, ps, &h)
			if !ok {
				continue
			}

			err = h.Process(ctx, hook.NewInput(
				h.OrganizationID,
				m.githubMan,
				m.webchangesClient,
			))
			if err != nil {
				m.log.With("hook_id", h.ID).
					With("error", err).
					Error("processing document hook")

				continue
			}

			err = m.db.UpdateDocumentHook(ctx, h)
			if err != nil {
				m.log.With("hook_id", h.ID).
					With("error", err).
					Error("updating document hook")
			}

			if previousScore.Equal(decimal.NewFromInt(100)) && h.Score.Equal(decimal.Zero) {
				maintainers, err := m.db.FetchDocumentMaintainers(ctx, h.DocumentID, h.OrganizationID)
				if err != nil {
					m.log.With("hook_id", h.ID).
						With("error", err).
						Error("fetching document maintainers for hook notification")

					continue
				}

				m.notifPub.PublishNotifications(
					h.OrganizationID,
					notification.NewDocumentHookTriggeredNotification(
						h.DocumentID,
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
	key := documentBranchKey{
		ID:       h.DocumentID,
		BranchID: h.BranchID,
	}

	doc, ok := state.Documents[key]
	if !ok {
		var err error

		doc, err = m.db.FetchDocumentByBranchID(ctx, h.BranchID.V, h.OrganizationID)

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
		err := h.Delete(ctx, hook.NewInput(
			h.OrganizationID,
			m.githubMan,
			m.webchangesClient,
		))
		if err != nil {
			m.log.With("hook_id", h.ID).
				With("error", err).
				Error("deleting hook for missing document")

			return false
		}

		err = m.db.DeleteDocumentHook(ctx, h.ID, h.OrganizationID)
		if err != nil {
			m.log.With("hook_id", h.ID).
				With("error", err).
				Error("deleting hook for missing document from db")
		}

		return false
	}

	if h.BlockID.Valid {
		hasBlock := doc.Content.FindBlock(h.BlockID.String)

		if !hasBlock && !h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.TimeFrom(timeutil.Now())
		} else if hasBlock && h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.Time{}
		}
	}

	return true
}

// DB defines the database operations required by the Manager.
type DB interface {
	// FetchPaginatedDocumentHooks should retrieve a paginated list of document hooks
	// starting after the given offset ID, limited to the specified number of hooks.
	FetchPaginatedDocumentHooks(ctx context.Context, offsetID xid.ID, limit int64) ([]hook.Hook, error)

	// UpdateDocumentHook should update the given document hook in the database.
	UpdateDocumentHook(ctx context.Context, hk hook.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID, organizationID string) error

	// FetchDocumentByBranchID should fetch the document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentMaintainers should fetch the document maintainers.
	FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error)
}
