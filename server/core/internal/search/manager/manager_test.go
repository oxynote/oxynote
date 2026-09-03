package manager

import (
	"context"
	"log/slog"
	"strconv"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jobs builds n sequential search jobs with IDs starting at start.
func jobs(start, n int) []search.DocumentSearchJob {
	jj := make([]search.DocumentSearchJob, 0, n)

	for i := range n {
		jj = append(jj, search.DocumentSearchJob{ID: int64(start + i)})
	}

	return jj
}

// docJob builds a job whose difference touches the given document.
func docJob(id int64, documentID xid.ID) search.DocumentSearchJob {
	return search.DocumentSearchJob{
		ID: id,
		BlockDiff: search.BlocksDifference{
			Added: []search.Block{{ID: "b" + strconv.FormatInt(id, 10), DocumentID: documentID}},
		},
	}
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	man := NewManager(slog.New(slog.DiscardHandler), &DBMock{}, &SearchGatewayMock{})

	require.NotNil(t, man)
}

func Test_Manager_Start(t *testing.T) {
	t.Parallel()

	fetched := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := &DBMock{
		FetchDocumentSearchJobsFunc: func(context.Context, int64, int64) ([]search.DocumentSearchJob, error) {
			close(fetched)
			cancel()

			return nil, nil
		},
	}

	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		NewManager(slog.New(slog.DiscardHandler), db, &SearchGatewayMock{}).Start(ctx)
	}()

	<-fetched
	<-stopped

	assert.Len(t, db.FetchDocumentSearchJobsCalls(), 1)
}

func Test_Manager_processJobs(t *testing.T) {
	t.Parallel()

	type check func(*testing.T, *DBMock, *SearchGatewayMock, error)

	checks := func(cc ...check) []check { return cc }

	hasError := func(expect bool) check {
		return func(t *testing.T, _ *DBMock, _ *SearchGatewayMock, err error) {
			if expect {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
		}
	}

	wasReplaceCalled := func(count int) check {
		return func(t *testing.T, _ *DBMock, gw *SearchGatewayMock, _ error) {
			require.Len(t, gw.ReplaceDocumentBlocksCalls(), count)
		}
	}

	wasDeleteCalled := func(count int) check {
		return func(t *testing.T, db *DBMock, _ *SearchGatewayMock, _ error) {
			require.Len(t, db.DeleteDocumentSearchJobCalls(), count)
		}
	}

	docA, docB := xid.New(), xid.New()

	isCancelled := func() check {
		return func(t *testing.T, _ *DBMock, _ *SearchGatewayMock, err error) {
			assert.ErrorIs(t, err, context.Canceled)
		}
	}

	tests := map[string]struct {
		Batches          [][]search.DocumentSearchJob
		CancelledContext bool
		ReplaceErr       error
		FailJobs         map[int64]bool
		DeleteErr        error
		Checks           []check
	}{
		"Cancelled context stops the run": {
			CancelledContext: true,
			Checks: checks(
				isCancelled(),
				wasReplaceCalled(0),
			),
		},
		"Fetch failure is propagated": {
			Batches: nil,
			Checks: checks(
				hasError(true),
				wasReplaceCalled(0),
			),
		},
		"Jobs are processed and deleted": {
			Batches: [][]search.DocumentSearchJob{jobs(1, 3)},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(3),
				wasDeleteCalled(3),
			),
		},
		"Full batches keep paginating past the last job ID": {
			Batches: [][]search.DocumentSearchJob{jobs(1, _processingBatch), jobs(101, 2)},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(_processingBatch+2),
				wasDeleteCalled(_processingBatch+2),
				func(t *testing.T, db *DBMock, _ *SearchGatewayMock, _ error) {
					ff := db.FetchDocumentSearchJobsCalls()
					require.Len(t, ff, 2)
					assert.Equal(t, int64(0), ff[0].OffsetID)
					assert.Equal(t, int64(_processingBatch), ff[1].OffsetID)
				},
			),
		},
		"Replace failure keeps the job for the next interval": {
			Batches:    [][]search.DocumentSearchJob{jobs(1, 2)},
			ReplaceErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasReplaceCalled(2),
				wasDeleteCalled(0),
			),
		},
		"Persistently failing full batches advance and terminate": {
			// the offset moves past failing jobs, so a wall of
			// failures cannot refetch the same batch forever.
			Batches: [][]search.DocumentSearchJob{
				jobs(1, _processingBatch),
				jobs(101, _processingBatch),
				nil,
			},
			ReplaceErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasReplaceCalled(2*_processingBatch),
				wasDeleteCalled(0),
			),
		},
		// a diff is a delta against what the previous ones left, so replaying
		// a failed one after a newer one would resurrect what the newer one
		// removed — including the blocks of a document that no longer exists.
		"A failed job holds the later diffs of its document": {
			Batches: [][]search.DocumentSearchJob{{
				docJob(1, docA),
				docJob(2, docA),
				docJob(3, docA),
			}},
			FailJobs: map[int64]bool{1: true},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(1),
				wasDeleteCalled(0),
			),
		},
		"A failure holds only its own document": {
			Batches: [][]search.DocumentSearchJob{{
				docJob(1, docA),
				docJob(2, docB),
				docJob(3, docA),
			}},
			FailJobs: map[int64]bool{1: true},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(2),
				wasDeleteCalled(1),
				func(t *testing.T, db *DBMock, _ *SearchGatewayMock, _ error) {
					ff := db.DeleteDocumentSearchJobCalls()
					require.Len(t, ff, 1)
					assert.Equal(t, int64(2), ff[0].ID, "the other document keeps moving")
				},
			),
		},
		"A branch removal waits behind its document's failed diff": {
			Batches: [][]search.DocumentSearchJob{{
				docJob(1, docA),
				{
					ID: 2,
					BlockDiff: search.BlocksDifference{
						RemovedBranches: []search.BranchRemoval{{DocumentID: docA, BranchID: xid.New()}},
					},
				},
				{
					ID: 3,
					BlockDiff: search.BlocksDifference{
						RemovedBranches: []search.BranchRemoval{{DocumentID: docB, BranchID: xid.New()}},
					},
				},
			}},
			FailJobs: map[int64]bool{1: true},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(2),
				wasDeleteCalled(1),
				func(t *testing.T, db *DBMock, _ *SearchGatewayMock, _ error) {
					ff := db.DeleteDocumentSearchJobCalls()
					require.Len(t, ff, 1)
					assert.Equal(t, int64(3), ff[0].ID, "the other document's branch removal keeps moving")
				},
			),
		},
		"An organization removal waits for anything held": {
			Batches: [][]search.DocumentSearchJob{{
				docJob(1, docA),
				{
					ID: 2,
					BlockDiff: search.BlocksDifference{
						RemovedOrganizations: []string{"org-1"},
					},
				},
				docJob(3, docB),
			}},
			FailJobs: map[int64]bool{1: true},
			Checks: checks(
				hasError(false),
				// the organization removal covers documents this run cannot
				// enumerate, so everything after it waits as well.
				wasReplaceCalled(1),
				wasDeleteCalled(0),
			),
		},
		"Delete failure still advances the offset": {
			Batches: [][]search.DocumentSearchJob{
				jobs(1, _processingBatch),
				nil,
			},
			DeleteErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasReplaceCalled(_processingBatch),
				wasDeleteCalled(_processingBatch),
			),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fetches := 0

			db := &DBMock{
				FetchDocumentSearchJobsFunc: func(context.Context, int64, int64) ([]search.DocumentSearchJob, error) {
					if fetches >= len(tc.Batches) {
						if len(tc.Batches) == 0 {
							return nil, assert.AnError
						}

						t.Fatal("unexpected extra fetch: the pagination did not terminate")
					}

					batch := tc.Batches[fetches]
					fetches++

					return batch, nil
				},
				DeleteDocumentSearchJobFunc: func(context.Context, int64) error {
					return tc.DeleteErr
				},
			}

			applied := 0

			gw := &SearchGatewayMock{
				ReplaceDocumentBlocksFunc: func(context.Context, search.BlocksDifference) error {
					applied++

					if tc.FailJobs != nil {
						// the fixture numbers jobs from one in fetch order.
						if tc.FailJobs[int64(applied)] {
							return assert.AnError
						}

						return nil
					}

					return tc.ReplaceErr
				},
			}

			man := NewManager(slog.New(slog.DiscardHandler), db, gw)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tc.CancelledContext {
				cancel()
			}

			err := man.processJobs(ctx)

			for _, ch := range tc.Checks {
				ch(t, db, gw, err)
			}
		})
	}
}
