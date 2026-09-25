// Package manager periodically processes document freshness hooks.
package manager

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/guregu/null/v5"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/internal/apps/github"
	"github.com/oxynote/oxynote/server/core/internal/apps/webchange"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/internal/document/hook/processor"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/syncutil"
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

	// _hookTimeout bounds one hook's run, and with it how long the hook's
	// lock is held.
	_hookTimeout = time.Second * 30

	// _teardownTimeout bounds tearing down a resource whose row was not
	// stored. It runs on its own clock, since the run's may be spent.
	_teardownTimeout = time.Second * 10

	// _branchesInterval defines how often queued branches are processed
	// when no trigger arrives.
	_branchesInterval = time.Minute
)

// Manager manages document freshness hooks.
type Manager struct {
	log             *slog.Logger
	db              DB
	flusher         Flusher
	githubMan       *github.Manager
	webchangeClient *webchange.Client
	notifPub        notification.Publisher
	hooks           *syncutil.KeyedMutex[xid.ID]
	branchesExec    *timeutil.PeriodicExec
	changeCallback  func(hook.Hook)

	branchesMu sync.Mutex
	branches   map[branchRef]struct{}
}

// NewManager creates a new Manager with the given database interface.
func NewManager(
	log *slog.Logger,
	db DB,
	flusher Flusher,
	githubMan *github.Manager,
	webchangeClient *webchange.Client,
	notifPub notification.Publisher,
) *Manager {
	m := &Manager{
		log:             log.With("component", "document-hooks-manager"),
		db:              db,
		flusher:         flusher,
		githubMan:       githubMan,
		webchangeClient: webchangeClient,
		notifPub:        notifPub,
		hooks:           syncutil.NewKeyedMutex[xid.ID](),
		changeCallback:  func(hook.Hook) {},
		branches:        make(map[branchRef]struct{}),
	}

	m.branchesExec = timeutil.NewPeriodicExec(
		_branchesInterval,
		0,
		m.processBranches,
		logutil.RecoveryValue(m.log, logutil.NewRecoveryPlan("recovered from a panic while processing branch hooks")),
		false,
	)

	return m
}

// BindHookChange sets the function called with every hook the manager
// stores, changes or deletes. Call it once, before Start.
func (m *Manager) BindHookChange(fn func(hook.Hook)) {
	m.changeCallback = fn
}

// Start processes document hooks periodically, and the branches
// QueueBranch queues, until the context ends.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("starting")
	defer m.log.Info("stopped")

	supv := xync.NewSupervisor(xync.WithSupervisorBaseContext(ctx))
	defer supv.Close()

	supv.Go(timeutil.NewPeriodicExec(
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
	).Start)
	supv.Go(m.branchesExec.Start)

	supv.Wait()
}

// QueueBranch has the branch's hooks run soon, without waiting for the
// next pass. Callers use it once hooks copied to the branch are committed,
// so the copies get set up.
func (m *Manager) QueueBranch(branchID xid.ID, organizationID string) {
	m.branchesMu.Lock()
	m.branches[branchRef{ID: branchID, OrganizationID: organizationID}] = struct{}{}
	m.branchesMu.Unlock()

	m.branchesExec.Trigger()
}

// processBranches processes every hook of the queued branches.
func (m *Manager) processBranches(ctx context.Context) {
	m.branchesMu.Lock()
	branches := m.branches
	m.branches = make(map[branchRef]struct{})
	m.branchesMu.Unlock()

	for b := range branches {
		hooks, err := m.db.FetchDocumentHooksByBranchID(ctx, b.ID, b.OrganizationID)
		if err != nil {
			m.log.With("branch_id", b.ID).
				With("error", err).
				Error("fetching branch hooks")

			continue
		}

		docs := make(map[xid.ID]*document.Document)

		for _, h := range hooks {
			m.processHook(ctx, docs, h)
		}
	}
}

// processHooks processes document hooks in a paginated manner.
func (m *Manager) processHooks(ctx context.Context) error {
	var offsetID xid.ID

	docs := make(map[xid.ID]*document.Document)

	for {
		hooks, err := m.db.FetchPaginatedDocumentHooks(ctx, offsetID, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document hooks: %w", err)
		}

		for _, h := range hooks {
			offsetID = h.ID

			m.processHook(ctx, docs, h)
		}

		if len(hooks) < _processingBatch {
			return nil
		}
	}
}

// processHook runs one hook, holding its lock. A hook a write holds is
// skipped until the next pass. docs caches documents by branch ID.
func (m *Manager) processHook(ctx context.Context, docs map[xid.ID]*document.Document, h hook.Hook) {
	unlock, ok := m.hooks.TryLock(h.ID)
	if !ok {
		return
	}

	defer unlock()

	ctx, cancel := context.WithTimeout(ctx, _hookTimeout)
	defer cancel()

	// the row may have changed between the page read and the lock. A hook
	// cut loose from its organization is out of any write's reach, so its
	// row is as it was read.
	if h.OrganizationID.Valid {
		stored, err := m.db.FetchDocumentHook(ctx, h.ID, h.OrganizationID.String)
		if err != nil {
			if !errutil.IsNotFound(err) {
				m.log.With("hook_id", h.ID).
					With("error", err).
					Error("fetching document hook")
			}

			return
		}

		h = *stored
	}

	prev := h

	deleted, stored := m.runHook(ctx, docs, &h)
	if !stored {
		m.undoSetup(ctx, prev, h)

		return
	}

	if deleted || h.ChangedFrom(prev) {
		m.changeCallback(h)
	}

	if !deleted {
		m.notifyTransition(ctx, prev, h)
	}
}

// runHook processes the hook and stores the result, or deletes a hook
// that has nothing left to describe.
func (m *Manager) runHook(ctx context.Context, docs map[xid.ID]*document.Document, h *hook.Hook) (deleted, stored bool) {
	// the branch, document or organization of the hook is gone.
	if !h.BranchID.Valid || !h.DocumentID.Valid || !h.OrganizationID.Valid {
		return true, m.deleteHook(ctx, h)
	}

	doc, ok := m.fetchDocument(ctx, docs, h)
	if !ok {
		return false, false
	}

	if doc == nil || (h.SoftDeletedAt.Valid && h.SoftDeletedAt.Time.Before(timeutil.Now().Add(-_hookRetentionDuration))) {
		return true, m.deleteHook(ctx, h)
	}

	if h.BlockID.Valid {
		hasBlock := doc.Content.HasBlock(h.BlockID.String)

		if !hasBlock && !h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.TimeFrom(timeutil.Now())
		} else if hasBlock && h.SoftDeletedAt.Valid {
			h.SoftDeletedAt = null.Time{}
		}
	}

	// a soft-deleted hook's block is gone, so its score describes nothing.
	// The mark is still stored, since it starts the retention clock.
	if !h.SoftDeletedAt.Valid {
		if err := h.Process(ctx, m.input(h.OrganizationID.String)); err != nil {
			// a transient failure keeps the stored state.
			m.log.With("hook_id", h.ID).
				With("error", err).
				Error("processing document hook")
		}
	}

	if err := m.db.UpdateDocumentHook(ctx, *h); err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("updating document hook")

		return false, false
	}

	return false, true
}

// fetchDocument returns the hook's document, cached in docs by branch. A
// nil document means it is gone; false means it could not be read.
func (m *Manager) fetchDocument(ctx context.Context, docs map[xid.ID]*document.Document, h *hook.Hook) (*document.Document, bool) {
	key := h.BranchID.V

	if doc, ok := docs[key]; ok {
		return doc, true
	}

	doc, err := m.db.FetchDocumentByBranchID(ctx, h.BranchID.V, h.OrganizationID.String)

	switch {
	case err == nil:
		docs[key] = doc
	case errutil.IsNotFound(err):
		docs[key] = nil
	default:
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("fetching document for hook")

		return nil, false
	}

	return docs[key], true
}

// deleteHook tears down the hook's external resource and then removes the
// row. The row is the only record of the resource, so it goes last.
func (m *Manager) deleteHook(ctx context.Context, h *hook.Hook) bool {
	if err := h.Delete(ctx, m.input(h.OrganizationID.String)); err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("deleting hook external resource")

		return false
	}

	if err := m.db.DeleteDocumentHook(ctx, h.ID); err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("deleting hook from db")

		return false
	}

	return true
}

// undoSetup tears down the resource h got when it was set up, since its
// row was not stored. Nothing else points at the resource, so a failure
// is reported.
func (m *Manager) undoSetup(ctx context.Context, prev, h hook.Hook) {
	if prev.State.Valid || !h.State.Valid {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), _teardownTimeout)
	defer cancel()

	if err := h.Delete(ctx, m.input(h.OrganizationID.String)); err != nil {
		logutil.Critical(m.log, err).Error(
			"cannot tear down the resource of a hook that was not stored",
			slog.String("hook_id", h.ID.String()),
		)
	}
}

// notifyTransition tells the document's maintainers when an active hook
// ran out of freshness, or when a hook can no longer check its target.
// Both fire on the transition only.
func (m *Manager) notifyTransition(ctx context.Context, prev, h hook.Hook) {
	var core notification.Core

	switch {
	case prev.Status == processor.StatusActive && h.Status != processor.StatusActive:
		core = notification.NewDocumentHookNeedsAttentionNotification(
			h.DocumentID.V,
			h.Type,
			h.BlockID,
			h.BranchID.V,
			h.Status,
		)
	// a scheduled reminder decays through 99…1, so the transition is the
	// arrival at zero, not a drop from full.
	case h.Status == processor.StatusActive && !prev.Score.IsZero() && h.Score.IsZero():
		core = notification.NewDocumentHookTriggeredNotification(
			h.DocumentID.V,
			h.Type,
			h.BlockID,
			h.BranchID.V,
		)
	default:
		return
	}

	maintainers, err := m.db.FetchDocumentMaintainers(ctx, h.DocumentID.V, h.OrganizationID.String)
	if err != nil {
		m.log.With("hook_id", h.ID).
			With("error", err).
			Error("fetching document maintainers for hook notification")

		return
	}

	m.notifPub.PublishNotifications(h.OrganizationID.String, core, maintainers...)
}

// input builds the processor input for the organization's hooks.
func (m *Manager) input(organizationID string) *hook.Input {
	return hook.NewInput(organizationID, m.githubMan, m.webchangeClient)
}

// branchRef names a branch queued by QueueBranch.
type branchRef struct {
	ID             xid.ID
	OrganizationID string
}

// DB defines the database operations required by the Manager.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	sqlutil.DB

	// FetchPaginatedDocumentHooks should retrieve a paginated list of document hooks
	// starting after the given offset ID, limited to the specified number of hooks.
	FetchPaginatedDocumentHooks(ctx context.Context, offsetID xid.ID, limit int64) ([]hook.Hook, error)

	// FetchDocumentHook should fetch the hook of the organization.
	FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hook.Hook, error)

	// UpdateDocumentHook should update the document hook.
	UpdateDocumentHook(ctx context.Context, hk hook.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID) error

	// FetchDocumentHooksByBranchID should fetch the hooks of a branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error)

	// FetchDocumentByBranchID should fetch the document joined against the branch
	// identified by branchID.
	FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error)

	// FetchDocumentMaintainers should fetch the document maintainers.
	FetchDocumentMaintainers(ctx context.Context, documentID xid.ID, organizationID string) ([]string, error)
}

// Tx is the transaction a hook write runs in, with its history record.
//
//go:generate ../../../../scripts/codegen/mock -t internal Tx tx
type Tx interface {
	sqlutil.Tx

	// InsertDocumentHook should insert the document hook.
	InsertDocumentHook(ctx context.Context, hk hook.Hook) error

	// UpdateDocumentHook should update the document hook.
	UpdateDocumentHook(ctx context.Context, hk hook.Hook) error

	// DeleteDocumentHook should delete the document hook for the given id.
	DeleteDocumentHook(ctx context.Context, id xid.ID) error

	// RecordDocumentBranchHistoryEntry should record the branch as it
	// stands, with its hooks, and return the id of the entry.
	RecordDocumentBranchHistoryEntry(
		ctx context.Context,
		branchID xid.ID,
		organizationID string,
		by null.String,
		boundary bool,
	) (xid.ID, error)
}

// CopyTx is the part of a caller's transaction CopyHooks runs in.
//
//go:generate ../../../../scripts/codegen/mock -t internal CopyTx copy_tx
type CopyTx interface {
	// FetchDocumentHooksByBranchID should fetch the hooks of a branch.
	FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error)

	// InsertDocumentHook should insert the document hook.
	InsertDocumentHook(ctx context.Context, hk hook.Hook) error
}

// Flusher stores what the editors of a branch hold.
//
//go:generate ../../../../scripts/codegen/mock -t internal Flusher flusher
type Flusher interface {
	// Flush should have the realtime service store what the editors of
	// the branch hold, returning once core has it.
	Flush(ctx context.Context, documentID, branchID xid.ID) error
}
