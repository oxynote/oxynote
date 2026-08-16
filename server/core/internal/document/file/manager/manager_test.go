package manager

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/guregu/null/v5"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// stubFile builds a file created long enough ago to be past the grace
// period, which is what the interesting cases need.
func stubFile() file.File {
	f := file.NewFile("file-1", file.LocationDocument, "folder/file-1", xid.New(), "org-1")
	f.CreatedAt = timeutil.Now().Add(-_fileRetentionDuration * 2)

	return f
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	db := &DBMock{}
	storer := &StorerMock{}
	opts := Options{ChangelogRetention: time.Hour}

	m := NewManager(slog.New(slog.DiscardHandler), db, storer, opts)
	require.NotNil(t, m)
	assert.NotNil(t, m.log)
	assert.Same(t, db, m.db)
	assert.Same(t, storer, m.storer)
	assert.Equal(t, opts, m.opts)
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	fetched := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := &DBMock{
		FetchPaginatedDocumentFilesFunc: func(context.Context, string, int64) ([]file.File, error) {
			close(fetched)
			cancel()

			return nil, nil
		},
	}

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		NewManager(slog.New(slog.DiscardHandler), db, &StorerMock{}, Options{}).Start(ctx)
	}()

	<-fetched
	<-stopped

	assert.Len(t, db.FetchPaginatedDocumentFilesCalls(), 1)
}

func Test_Manager_processFiles(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(exp error) check {
		return func(t *testing.T, _ *DBMock, _ *StorerMock, err error) {
			testutil.AssertEqualError(t, exp, err)
		}
	}

	wasFetchCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ error) {
			require.Len(t, db.FetchPaginatedDocumentFilesCalls(), count)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ error) {
			assert.Len(t, db.DeleteDocumentFileCalls(), count)
		}
	}

	wasTrimCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock, _ error) {
			assert.Len(t, db.DeleteExpiredDocumentBranchChangelogsCalls(), count)
		}
	}

	// a full batch followed by a partial one, which is what makes the
	// loop page a second time.
	pagedFiles := func() []file.File {
		ff := make([]file.File, _processingBatch)

		for i := range ff {
			f := stubFile()
			f.DocumentID = null.Value[xid.ID]{}
			ff[i] = f
		}

		return ff
	}

	cc := map[string]struct {
		DB     *DBMock
		Storer *StorerMock
		Opts   Options
		Checks []check
	}{
		"Error returned by db.DeleteExpiredDocumentBranchChangelogs": {
			DB: &DBMock{
				DeleteExpiredDocumentBranchChangelogsFunc: func(context.Context, time.Time) error {
					return assert.AnError
				},
			},
			Storer: &StorerMock{},
			Opts:   Options{ChangelogRetention: time.Hour},
			Checks: checks(
				hasError(assert.AnError),
				wasTrimCalled(1),
				wasFetchCalled(0),
			),
		},
		"Error returned by db.FetchPaginatedDocumentFiles": {
			DB: &DBMock{
				FetchPaginatedDocumentFilesFunc: func(context.Context, string, int64) ([]file.File, error) {
					return nil, assert.AnError
				},
			},
			Storer: &StorerMock{},
			Checks: checks(
				hasError(assert.AnError),
				wasFetchCalled(1),
			),
		},
		"Full batch pages once more": {
			DB: func() *DBMock {
				db := &DBMock{}

				calls := 0
				db.FetchPaginatedDocumentFilesFunc = func(context.Context, string, int64) ([]file.File, error) {
					calls++

					if calls == 1 {
						return pagedFiles(), nil
					}

					return nil, nil
				}

				return db
			}(),
			Storer: &StorerMock{},
			Checks: checks(
				hasError(nil),
				wasFetchCalled(2),
				wasDeleteCalled(_processingBatch),
			),
		},
		"Nothing to process": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			Checks: checks(
				hasError(nil),
				wasFetchCalled(1),
				wasTrimCalled(0),
				wasDeleteCalled(0),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewManager(slog.New(slog.DiscardHandler), c.DB, c.Storer, c.Opts)

			err := m.processFiles(context.Background())

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer, err)
			}
		})
	}
}

func Test_Manager_processFile(t *testing.T) {
	type check func(*testing.T, *DBMock, *StorerMock)

	checks := func(cc ...check) []check { return cc }

	wasCheckCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock) {
			assert.Len(t, db.CheckDocumentFileReferencedCalls(), count)
		}
	}

	wasMarkCalled := func(at null.Time) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock) {
			ff := db.UpdateDocumentFileUnreferencedAtCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "file-1", ff[0].ID)
			assert.Equal(t, at.Valid, ff[0].At.Valid)
		}
	}

	wasNotMarked := func() check {
		return func(t *testing.T, db *DBMock, _ *StorerMock) {
			assert.Empty(t, db.UpdateDocumentFileUnreferencedAtCalls())
		}
	}

	wasObjectDeleted := func(count int) check {
		return func(t *testing.T, _ *DBMock, storer *StorerMock) {
			ff := storer.DeleteCalls()
			require.Len(t, ff, count)

			if count == 0 {
				return
			}

			assert.Equal(t, "folder/", ff[0].Folder)
			assert.Equal(t, "file-1", ff[0].ID)
		}
	}

	wasRowDeleted := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *StorerMock) {
			assert.Len(t, db.DeleteDocumentFileCalls(), count)
		}
	}

	referencedDB := func(referenced bool) *DBMock {
		return &DBMock{
			CheckDocumentFileReferencedFunc: func(context.Context, string, xid.ID) (bool, error) {
				return referenced, nil
			},
		}
	}

	cc := map[string]struct {
		DB     *DBMock
		Storer *StorerMock
		File   func() file.File
		Checks []check
	}{
		"Orphaned file is reclaimed without waiting": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			File: func() file.File {
				f := stubFile()
				f.CreatedAt = timeutil.Now()
				f.DocumentID = null.Value[xid.ID]{}

				return f
			},
			Checks: checks(
				wasCheckCalled(0),
				wasObjectDeleted(1),
				wasRowDeleted(1),
			),
		},
		"File within the grace period is left alone": {
			DB:     &DBMock{},
			Storer: &StorerMock{},
			File: func() file.File {
				f := stubFile()
				f.CreatedAt = timeutil.Now()

				return f
			},
			Checks: checks(
				wasCheckCalled(0),
				wasNotMarked(),
				wasObjectDeleted(0),
				wasRowDeleted(0),
			),
		},
		"Error returned by db.CheckDocumentFileReferenced": {
			DB: &DBMock{
				CheckDocumentFileReferencedFunc: func(context.Context, string, xid.ID) (bool, error) {
					return false, assert.AnError
				},
			},
			Storer: &StorerMock{},
			File:   stubFile,
			Checks: checks(
				wasCheckCalled(1),
				wasNotMarked(),
				wasObjectDeleted(0),
			),
		},
		"Referenced file keeps its clean state": {
			DB:     referencedDB(true),
			Storer: &StorerMock{},
			File:   stubFile,
			Checks: checks(
				wasCheckCalled(1),
				wasNotMarked(),
				wasObjectDeleted(0),
			),
		},
		"Referenced file loses its unreferenced mark": {
			DB:     referencedDB(true),
			Storer: &StorerMock{},
			File: func() file.File {
				f := stubFile()
				f.UnreferencedAt = null.TimeFrom(timeutil.Now())

				return f
			},
			Checks: checks(
				wasMarkCalled(null.Time{}),
				wasObjectDeleted(0),
			),
		},
		"Unreferenced file is marked": {
			DB:     referencedDB(false),
			Storer: &StorerMock{},
			File:   stubFile,
			Checks: checks(
				wasMarkCalled(null.TimeFrom(timeutil.Now())),
				wasObjectDeleted(0),
				wasRowDeleted(0),
			),
		},
		"Marked file within the retention window survives": {
			DB:     referencedDB(false),
			Storer: &StorerMock{},
			File: func() file.File {
				f := stubFile()
				f.UnreferencedAt = null.TimeFrom(timeutil.Now())

				return f
			},
			Checks: checks(
				wasNotMarked(),
				wasObjectDeleted(0),
				wasRowDeleted(0),
			),
		},
		"Marked file past the retention window is reclaimed": {
			DB:     referencedDB(false),
			Storer: &StorerMock{},
			File: func() file.File {
				f := stubFile()
				f.UnreferencedAt = null.TimeFrom(timeutil.Now().Add(-_fileRetentionDuration * 2))

				return f
			},
			Checks: checks(
				wasNotMarked(),
				wasObjectDeleted(1),
				wasRowDeleted(1),
			),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewManager(slog.New(slog.DiscardHandler), c.DB, c.Storer, Options{})

			m.processFile(context.Background(), c.File())

			for _, ch := range c.Checks {
				ch(t, c.DB, c.Storer)
			}
		})
	}
}

func Test_Manager_markFile(t *testing.T) {
	cc := map[string]struct {
		DB *DBMock
		At null.Time
	}{
		"Error returned by db.UpdateDocumentFileUnreferencedAt": {
			DB: &DBMock{
				UpdateDocumentFileUnreferencedAtFunc: func(context.Context, string, null.Time) error {
					return assert.AnError
				},
			},
			At: null.TimeFrom(timeutil.Now()),
		},
		"Successful update": {
			DB: &DBMock{},
			At: null.Time{},
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewManager(slog.New(slog.DiscardHandler), c.DB, &StorerMock{}, Options{})

			m.markFile(context.Background(), stubFile(), c.At)

			ff := c.DB.UpdateDocumentFileUnreferencedAtCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "file-1", ff[0].ID)
			assert.Equal(t, c.At, ff[0].At)
		})
	}
}

func Test_Manager_deleteFile(t *testing.T) {
	cc := map[string]struct {
		DB       *DBMock
		Storer   *StorerMock
		RowCount int
	}{
		"Error returned by storer.Delete keeps the row": {
			DB: &DBMock{},
			Storer: &StorerMock{
				DeleteFunc: func(context.Context, string, string) error {
					return assert.AnError
				},
			},
			RowCount: 0,
		},
		"Error returned by db.DeleteDocumentFile": {
			DB: &DBMock{
				DeleteDocumentFileFunc: func(context.Context, string) error {
					return assert.AnError
				},
			},
			Storer:   &StorerMock{},
			RowCount: 1,
		},
		"Successful deletion": {
			DB:       &DBMock{},
			Storer:   &StorerMock{},
			RowCount: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewManager(slog.New(slog.DiscardHandler), c.DB, c.Storer, Options{})

			m.deleteFile(context.Background(), stubFile())

			ff := c.Storer.DeleteCalls()
			require.Len(t, ff, 1)
			assert.Equal(t, "folder/", ff[0].Folder)
			assert.Equal(t, "file-1", ff[0].ID)

			assert.Len(t, c.DB.DeleteDocumentFileCalls(), c.RowCount)
		})
	}
}

func Test_Manager_trimChangelogs(t *testing.T) {
	cc := map[string]struct {
		DB    *DBMock
		Opts  Options
		Calls int
		Err   error
	}{
		"Zero retention disables trimming": {
			DB:    &DBMock{},
			Opts:  Options{},
			Calls: 0,
		},
		"Error returned by db.DeleteExpiredDocumentBranchChangelogs": {
			DB: &DBMock{
				DeleteExpiredDocumentBranchChangelogsFunc: func(context.Context, time.Time) error {
					return assert.AnError
				},
			},
			Opts:  Options{ChangelogRetention: time.Hour},
			Calls: 1,
			Err:   assert.AnError,
		},
		"Successful trim": {
			DB:    &DBMock{},
			Opts:  Options{ChangelogRetention: time.Hour},
			Calls: 1,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			m := NewManager(slog.New(slog.DiscardHandler), c.DB, &StorerMock{}, c.Opts)

			err := m.trimChangelogs(context.Background())

			testutil.AssertEqualError(t, c.Err, err)

			ff := c.DB.DeleteExpiredDocumentBranchChangelogsCalls()
			require.Len(t, ff, c.Calls)

			if c.Calls == 0 {
				return
			}

			assert.WithinDuration(t, timeutil.Now().Add(-time.Hour), ff[0].Before, time.Minute)
		})
	}
}
