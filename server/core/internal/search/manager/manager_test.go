package manager

import (
	"context"
	"log/slog"
	"testing"

	"github.com/oxynote/oxynote/server/core/internal/search"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jobs builds n sequential search jobs.
func jobs(n int) []search.DocumentSearchJob {
	jj := make([]search.DocumentSearchJob, 0, n)

	for i := range n {
		jj = append(jj, search.DocumentSearchJob{ID: int64(i + 1)})
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
			Batches: [][]search.DocumentSearchJob{jobs(3)},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(3),
				wasDeleteCalled(3),
			),
		},
		"Full batches keep paginating": {
			Batches: [][]search.DocumentSearchJob{jobs(_processingBatch), jobs(2)},
			Checks: checks(
				hasError(false),
				wasReplaceCalled(_processingBatch+2),
				wasDeleteCalled(_processingBatch+2),
			),
		},
		"Replace failure keeps the job for the next pass": {
			Batches:    [][]search.DocumentSearchJob{jobs(2)},
			ReplaceErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasReplaceCalled(2),
				wasDeleteCalled(0),
			),
		},
		"Persistently failing full batch terminates": {
			// without the no-progress guard this would refetch the
			// same failing batch forever.
			Batches:    [][]search.DocumentSearchJob{jobs(_processingBatch), jobs(_processingBatch)},
			ReplaceErr: assert.AnError,
			Checks: checks(
				hasError(false),
				wasReplaceCalled(_processingBatch),
				wasDeleteCalled(0),
			),
		},
		"Delete failure does not count as progress": {
			Batches:   [][]search.DocumentSearchJob{jobs(_processingBatch), jobs(_processingBatch)},
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
				FetchDocumentSearchJobsFunc: func(context.Context, int64) ([]search.DocumentSearchJob, error) {
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
