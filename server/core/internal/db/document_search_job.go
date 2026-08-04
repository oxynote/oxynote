package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/searchgw"
)

// InsertDocumentSearchJob inserts a new document search job into the database.
func (a *agent) InsertDocumentSearchJob(ctx context.Context, blockDiff searchgw.BlocksDifference) error {
	q, args := a.builder.Insert("document_search_jobs").
		SetMap(map[string]any{
			"block_diff": blockDiff,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentSearchJobs retrieves a batch of document search jobs.
func (a *agent) FetchDocumentSearchJobs(ctx context.Context, limit int64) ([]searchgw.DocumentSearchJob, error) {
	q, args := a.builder.Select(
		"id",
		"block_diff",
	).From("document_search_jobs").
		OrderBy("id ASC").
		Limit(uint64(limit)).
		MustSql()

	var jobs []searchgw.DocumentSearchJob

	err := sqlx.SelectContext(ctx, a.sql, &jobs, q, args...)
	if err != nil {
		return nil, err
	}

	return jobs, nil
}

// DeleteDocumentSearchJob deletes a document search job from the database.
func (a *agent) DeleteDocumentSearchJob(ctx context.Context, id int64) error {
	q, args := a.builder.Delete("document_search_jobs").
		Where(sq.Eq{
			"id": id,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}
