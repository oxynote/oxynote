package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/search"
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

func Test_NewManager(t *testing.T) {
	t.Parallel()

	man := NewManager(slog.New(slog.DiscardHandler), &DBMock{}, &SearchGatewayMock{})

	require.NotNil(t, man)
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

	tests := map[string]struct {
		Batches    [][]search.DocumentSearchJob
		ReplaceErr error
		DeleteErr  error
		Checks     []check
	}{
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

			gw := &SearchGatewayMock{
				ReplaceDocumentBlocksFunc: func(context.Context, search.BlocksDifference) error {
					return tc.ReplaceErr
				},
			}

			man := NewManager(slog.New(slog.DiscardHandler), db, gw)

			err := man.processJobs(context.Background())

			for _, ch := range tc.Checks {
				ch(t, db, gw, err)
			}
		})
	}
}

func Test_Manager_processJobs_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	man := NewManager(slog.New(slog.DiscardHandler), &DBMock{}, &SearchGatewayMock{})

	assert.ErrorIs(t, man.processJobs(ctx), context.Canceled)
}
