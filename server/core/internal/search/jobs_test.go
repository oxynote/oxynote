package search

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertRecorder records InsertDocumentSearchJob calls for Jobs tests.
type insertRecorder struct {
	diffs []BlocksDifference
	err   error
}

func (ir *insertRecorder) InsertDocumentSearchJob(_ context.Context, diff BlocksDifference) error {
	ir.diffs = append(ir.diffs, diff)

	return ir.err
}

func Test_NewJobs(t *testing.T) {
	t.Parallel()

	j := NewJobs(true)
	require.NotNil(t, j)
	assert.True(t, j.enabled)
}

func Test_Jobs_Enqueue(t *testing.T) {
	t.Parallel()

	diff := BlocksDifference{
		RemovedOrganizations: []string{"org1"},
	}

	// disabled drops the job without touching the inserter.
	ir := &insertRecorder{}
	require.NoError(t, NewJobs(false).Enqueue(context.Background(), ir, diff))
	assert.Empty(t, ir.diffs)

	// enabled writes through and propagates the inserter's error.
	ir = &insertRecorder{err: assert.AnError}
	err := NewJobs(true).Enqueue(context.Background(), ir, diff)
	assert.Equal(t, assert.AnError, err)
	require.Len(t, ir.diffs, 1)
	assert.Equal(t, diff, ir.diffs[0])
}
