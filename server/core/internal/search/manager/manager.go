// Package manager runs the background document search-indexing jobs.
package manager

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/oxynote/oxynote/server/core/internal/search"
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

	tm := time.NewTimer(0)
	defer tm.Stop()

	for {
		select {
		case <-tm.C:
			err := m.processJobs(ctx)
			if err != nil {
				m.log.With("error", err).
					Error("processing document search jobs")
			}

			tm.Reset(_processingInterval)
		case <-ctx.Done():
			return
		}
	}
}

// processJobs processes document search jobs in a paginated manner.
func (m *Manager) processJobs(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		jobs, err := m.db.FetchDocumentSearchJobs(ctx, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document search jobs: %w", err)
		}

		var deleted int

		for _, job := range jobs {
			err := m.searchGateway.ReplaceDocumentBlocks(ctx, job.BlockDiff)
			if err != nil {
				m.log.With("job_id", job.ID).
					With("error", err).
					Error("processing document search job")

				continue
			}

			err = m.db.DeleteDocumentSearchJob(ctx, job.ID)
			if err != nil {
				m.log.With("job_id", job.ID).
					With("error", err).
					Error("deleting document search job")

				continue
			}

			deleted++
		}

		// the fetch has no offset, so without progress the next pass
		// would see the same failing jobs again — leave them for the
		// next processing interval instead of spinning on them.
		if len(jobs) < _processingBatch || deleted == 0 {
			break
		}
	}

	return nil
}

// DB defines the database operations required by the Manager.
//
//go:generate ../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchDocumentSearchJobs should retrieve a batch of document search
	// jobs, limited to the specified number of jobs.
	FetchDocumentSearchJobs(ctx context.Context, limit int64) ([]search.DocumentSearchJob, error)

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
