// Package manager runs the background document search-indexing jobs.
package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

const (
	// _processingBatch defines how many search jobs to process in one batch.
	_processingBatch = 100

	// _processingInterval defines how often to process search jobs.
	_processingInterval = time.Second * 10
)

// Manager manages document search index update jobs.
type Manager struct {
	log           *slog.Logger
	db            DB
	searchGateway SearchGateway
}

// NewManager creates a new Manager with the given database and search gateway.
func NewManager(log *slog.Logger, db DB, searchGateway SearchGateway) *Manager {
	return &Manager{
		log:           log.With("component", "document-search-job-manager"),
		db:            db,
		searchGateway: searchGateway,
	}
}

// Start begins the periodic processing of document search jobs.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("starting")
	defer m.log.Info("stopped")

	timeutil.NewPeriodicExec(
		_processingInterval,
		0,
		func(ctx context.Context) {
			if err := m.processJobs(ctx); err != nil {
				m.log.With("error", err).
					Error("processing document search jobs")
			}
		},
		logutil.RecoveryValue(m.log, logutil.NewRecoveryPlan("recovered from a panic while processing document search jobs")),
		true,
	).Start(ctx)
}

// processJobs processes document search jobs in a paginated manner. Failed
// jobs stay in the database and are retried on the next processing interval,
// together with every later job of the same document: a diff describes a
// change against the state its predecessors left behind, so applying one out
// of order resurrects blocks a newer diff removed.
func (m *Manager) processJobs(ctx context.Context) error {
	var (
		offsetID int64
		held     = newHoldSet()
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		jobs, err := m.db.FetchDocumentSearchJobs(ctx, offsetID, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document search jobs: %w", err)
		}

		for _, job := range jobs {
			offsetID = job.ID

			documentIDs, global := jobScope(job.BlockDiff)

			if held.blocks(documentIDs, global) {
				held.hold(documentIDs, global)

				continue
			}

			err := m.searchGateway.ReplaceDocumentBlocks(ctx, job.BlockDiff)
			if err != nil {
				m.log.With("job_id", job.ID).
					With("error", err).
					Error("processing document search job")

				held.hold(documentIDs, global)

				continue
			}

			err = m.db.DeleteDocumentSearchJob(ctx, job.ID)
			if err != nil {
				m.log.With("job_id", job.ID).
					With("error", err).
					Error("deleting document search job")

				// the diff is applied but the job survives, so it runs again
				// next interval — after anything newer, unless held.
				held.hold(documentIDs, global)
			}
		}

		if len(jobs) < _processingBatch {
			break
		}
	}

	return nil
}

// jobScope reports the documents a difference touches. An organization
// removal is global: it clears the entries of every document the
// organization owns, which no per-document bound can enumerate.
func jobScope(bd search.BlocksDifference) (documentIDs []xid.ID, global bool) {
	if len(bd.RemovedOrganizations) > 0 {
		return nil, true
	}

	seen := make(map[xid.ID]struct{})

	add := func(id xid.ID) {
		if _, ok := seen[id]; ok {
			return
		}

		seen[id] = struct{}{}

		documentIDs = append(documentIDs, id)
	}

	for _, blocks := range [][]search.Block{bd.Added, bd.Updated, bd.Removed} {
		for _, b := range blocks {
			add(b.DocumentID)
		}
	}

	for _, id := range bd.RemovedDocuments {
		add(id)
	}

	return documentIDs, false
}

// holdSet tracks the documents whose diffs have to wait for the next
// interval, in the order-preserving sense: once one diff of a document is
// held back, every later one is held with it.
type holdSet struct {
	documentIDs map[xid.ID]struct{}
	all         bool
}

// newHoldSet creates an empty hold set.
func newHoldSet() *holdSet {
	return &holdSet{documentIDs: make(map[xid.ID]struct{})}
}

// blocks reports whether a difference of the given scope has to wait.
func (h *holdSet) blocks(documentIDs []xid.ID, global bool) bool {
	if h.all {
		return true
	}

	// a global difference covers documents this run knows nothing about, so
	// anything held at all can be one of them.
	if global {
		return len(h.documentIDs) > 0
	}

	for _, id := range documentIDs {
		if _, ok := h.documentIDs[id]; ok {
			return true
		}
	}

	return false
}

// hold makes every later difference of the same scope wait too.
func (h *holdSet) hold(documentIDs []xid.ID, global bool) {
	if global {
		h.all = true

		return
	}

	for _, id := range documentIDs {
		h.documentIDs[id] = struct{}{}
	}
}

// DB defines the database operations required by the Manager.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentSearchJobs should retrieve a batch of document search
	// jobs with IDs greater than the given offset ID, limited to the
	// specified number of jobs.
	FetchDocumentSearchJobs(ctx context.Context, offsetID, limit int64) ([]search.DocumentSearchJob, error)

	// DeleteDocumentSearchJob should delete the given document search job from the database.
	DeleteDocumentSearchJob(ctx context.Context, id int64) error
}

// SearchGateway defines the search operations required by the Manager.
//
//go:generate ../../../scripts/codegen/mock -t internal SearchGateway search_gateway
type SearchGateway interface {
	// ReplaceDocumentBlocks should replace document blocks based on the provided differences.
	ReplaceDocumentBlocks(ctx context.Context, bd search.BlocksDifference) error
}
