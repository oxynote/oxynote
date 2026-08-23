package search

import "context"

// Jobs is the way into the document search-job queue: every enqueue in
// the codebase goes through it, so whether a deployment indexes at all
// is decided here rather than by each call site or the database layer.
type Jobs struct {
	enabled bool
}

// NewJobs creates a fresh instance of Jobs. When enabled is false —
// search is not configured on this deployment — every enqueued job is
// dropped instead of piling up in a table nothing consumes; documents
// created meanwhile were never indexed, so there is nothing stale to
// clean up either.
func NewJobs(enabled bool) *Jobs {
	return &Jobs{
		enabled: enabled,
	}
}

// Enqueue queues the diff through the given inserter, which is the
// caller's own database handle so the job commits atomically with the
// content change it describes.
func (j *Jobs) Enqueue(ctx context.Context, ins JobInserter, diff BlocksDifference) error {
	if !j.enabled {
		return nil
	}

	return ins.InsertDocumentSearchJob(ctx, diff)
}

// JobInserter is the database surface Enqueue writes through.
type JobInserter interface {
	// InsertDocumentSearchJob should insert the document search job.
	InsertDocumentSearchJob(ctx context.Context, diff BlocksDifference) error
}
