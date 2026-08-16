// Package manager periodically reclaims document files that nothing
// references any more.
package manager

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

const (
	// _processingBatch defines how many files to process in one batch.
	_processingBatch = 100

	// _processingInterval defines how often to reclaim files.
	_processingInterval = time.Minute * 5

	// _fileRetentionDuration defines how long a file is kept after it was
	// first observed to be referenced by nothing. It covers the window
	// between an upload and the first content persist that mentions it,
	// during which a brand new file legitimately looks unreferenced.
	_fileRetentionDuration = time.Hour * 24
)

// Manager reclaims the objects of files no document refers to.
type Manager struct {
	log    *slog.Logger
	db     DB
	storer Storer
	opts   Options
}

// Options holds all file manager options.
type Options struct {
	// ChangelogRetention specifies how long document changelog snapshots
	// are kept. Snapshots pin the files they reference, so trimming them
	// is what releases a file removed from live content.
	// Zero disables age-based trimming.
	ChangelogRetention time.Duration
}

// NewManager creates a new Manager with the given database interface.
func NewManager(
	log *slog.Logger,
	db DB,
	storer Storer,
	opts Options,
) *Manager {
	return &Manager{
		log:    log.With("component", "document-files-manager"),
		db:     db,
		storer: storer,
		opts:   opts,
	}
}

// Start begins the periodic reclamation of document files.
func (m *Manager) Start(ctx context.Context) {
	m.log.Info("starting")
	defer m.log.Info("stopped")

	tm := time.NewTimer(0)
	defer tm.Stop()

	for {
		select {
		case <-tm.C:
			err := m.processFiles(ctx)
			if err != nil {
				m.log.With("error", err).
					Error("processing document files")
			}

			tm.Reset(_processingInterval)
		case <-ctx.Done():
			return
		}
	}
}

// processFiles trims expired changelog snapshots and then reclaims every
// file that no longer has anything referring to it.
//
// The snapshots go first: a file released by an expiring snapshot is then
// reclaimed in the same pass instead of waiting for the next one.
func (m *Manager) processFiles(ctx context.Context) error {
	if err := m.trimChangelogs(ctx); err != nil {
		return err
	}

	var offsetID string

	for {
		files, err := m.db.FetchPaginatedDocumentFiles(ctx, offsetID, _processingBatch)
		if err != nil {
			return fmt.Errorf("fetching paginated document files: %w", err)
		}

		for _, f := range files {
			offsetID = f.ID

			m.processFile(ctx, f)
		}

		if len(files) < _processingBatch {
			break
		}
	}

	return nil
}

// processFile reclaims a single file, marks it as unreferenced, or leaves
// it alone.
func (m *Manager) processFile(ctx context.Context, f file.File) {
	// the owner is gone, so no content can reference the file any more and
	// there is no upload in flight to protect: the retention window would
	// only delay the inevitable.
	if f.Orphaned() {
		m.deleteFile(ctx, f)
		return
	}

	if timeutil.Now().Sub(f.CreatedAt) < _fileRetentionDuration {
		return
	}

	referenced, err := m.db.CheckDocumentFileReferenced(ctx, f.ID, f.DocumentID.V)
	if err != nil {
		m.log.With("file_id", f.ID).
			With("error", err).
			Error("checking whether document file is referenced")

		return
	}

	if referenced {
		if f.UnreferencedAt.Valid {
			m.markFile(ctx, f, null.Time{})
		}

		return
	}

	if !f.UnreferencedAt.Valid {
		m.markFile(ctx, f, null.TimeFrom(timeutil.Now()))
		return
	}

	if f.UnreferencedAt.Time.After(timeutil.Now().Add(-_fileRetentionDuration)) {
		return
	}

	m.deleteFile(ctx, f)
}

// markFile records or clears the moment the file was observed to be
// referenced by nothing.
func (m *Manager) markFile(ctx context.Context, f file.File, at null.Time) {
	err := m.db.UpdateDocumentFileUnreferencedAt(ctx, f.ID, at)
	if err != nil {
		m.log.With("file_id", f.ID).
			With("error", err).
			Error("updating document file unreferenced timestamp")
	}
}

// deleteFile removes the object and then the row that describes it. The
// object goes first so that a failure in between leaves a row pointing at
// an object that is already gone, which the next pass resolves; the other
// order would leave an object nothing knows about.
func (m *Manager) deleteFile(ctx context.Context, f file.File) {
	folder, id := path.Split(f.StorageKey)

	err := m.storer.Delete(ctx, folder, id)
	if err != nil {
		m.log.With("file_id", f.ID).
			With("error", err).
			Error("deleting document file object")

		return
	}

	err = m.db.DeleteDocumentFile(ctx, f.ID)
	if err != nil {
		m.log.With("file_id", f.ID).
			With("error", err).
			Error("deleting document file")
	}
}

// trimChangelogs removes changelog snapshots that outlived their retention.
// Snapshot count is trimmed on insert instead; age has to be handled here,
// because a branch that stopped being edited never inserts again and its
// snapshots would pin their files forever.
func (m *Manager) trimChangelogs(ctx context.Context) error {
	if m.opts.ChangelogRetention == 0 {
		return nil
	}

	err := m.db.DeleteExpiredDocumentBranchChangelogs(
		ctx,
		timeutil.Now().Add(-m.opts.ChangelogRetention),
	)
	if err != nil {
		return fmt.Errorf("deleting expired document branch changelogs: %w", err)
	}

	return nil
}

// DB defines the database operations required by the Manager.
//
//go:generate ../../../../scripts/codegen/mock -t internal DB db
type DB interface {
	// FetchPaginatedDocumentFiles should retrieve a batch of document files
	// following the given offset.
	FetchPaginatedDocumentFiles(ctx context.Context, offsetID string, limit int64) ([]file.File, error)

	// CheckDocumentFileReferenced should report whether the file is still
	// referenced by the document's content, changelogs or comments.
	CheckDocumentFileReferenced(ctx context.Context, id string, documentID xid.ID) (bool, error)

	// UpdateDocumentFileUnreferencedAt should set or clear the moment the
	// file was observed to be referenced by nothing.
	UpdateDocumentFileUnreferencedAt(ctx context.Context, id string, at null.Time) error

	// DeleteDocumentFile should remove the document file.
	DeleteDocumentFile(ctx context.Context, id string) error

	// DeleteExpiredDocumentBranchChangelogs should remove changelog
	// snapshots created before the given time.
	DeleteExpiredDocumentBranchChangelogs(ctx context.Context, before time.Time) error
}

// Storer defines the object storage operations required by the Manager.
//
//go:generate ../../../../scripts/codegen/mock -t internal Storer
type Storer interface {
	// Delete should delete an object by its ID.
	Delete(ctx context.Context, folder, id string) error
}
