package db

import (
	"context"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/file"
	"github.com/rs/xid"
)

// InsertDocumentFile inserts a new document file into the database. The id
// is client-chosen and unique across organizations: an id that already has
// a row is refused, so an object a history entry points at is never
// overwritten in place.
func (a *agent) InsertDocumentFile(ctx context.Context, f file.File) error {
	q, args := a.builder.Insert("document_files").
		SetMap(map[string]any{
			"id":                 f.ID,
			"location":           f.Location,
			"storage_key":        f.StorageKey,
			"fk_document_id":     f.DocumentID,
			"fk_organization_id": f.OrganizationID,
			"name":               f.Name,
			"size":               f.Size,
			"content_type":       f.ContentType,
			"created_at":         f.CreatedAt,
			"unreferenced_at":    f.UnreferencedAt,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentFile retrieves a document file by its block ID from the database.
func (a *agent) FetchDocumentFile(ctx context.Context, id, organizationID string) (*file.File, error) {
	q, args := a.selectDocumentFile(a.builder.Select()).
		Where(sq.Eq{
			"document_files.id":                 id,
			"document_files.fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	f := &file.File{}
	if err := sqlx.GetContext(ctx, a.sql, f, q, args...); err != nil {
		return nil, err
	}

	return f, nil
}

// FetchPaginatedDocumentFiles retrieves a list of document files from
// the database with pagination support.
func (a *agent) FetchPaginatedDocumentFiles(ctx context.Context, offsetID string, limit int64) ([]file.File, error) {
	b := a.selectDocumentFile(a.builder.Select()).
		OrderBy("document_files.id ASC").
		Limit(uint64(limit))

	if offsetID != "" {
		b = b.Where(sq.Gt{"document_files.id": offsetID})
	}

	q, args := b.MustSql()

	files := []file.File{}
	if err := sqlx.SelectContext(ctx, a.sql, &files, q, args...); err != nil {
		return nil, err
	}

	return files, nil
}

// UpdateDocumentFileUnreferencedAt sets or clears the timestamp marking
// when the file was first observed to be referenced by nothing.
func (a *agent) UpdateDocumentFileUnreferencedAt(ctx context.Context, id string, at null.Time) error {
	q, args := a.builder.Update("document_files").
		SetMap(map[string]any{
			"unreferenced_at": at,
		}).
		Where(sq.Eq{"id": id}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteDocumentFile removes a document file row from the database.
// The row is not scoped to an organization: the sweep deletes rows whose
// organization is already gone.
func (a *agent) DeleteDocumentFile(ctx context.Context, id string) error {
	q, args := a.builder.Delete("document_files").
		Where(sq.Eq{"id": id}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// CheckDocumentFileReferenced reports whether the file id still appears in
// the document's live branch content, in a retained history entry, or
// in a comment or comment reply.
//
// The id is matched as a substring of the serialized content rather than by
// walking the block tree, which keeps the whole check inside Postgres. The
// imprecision is deliberate and one-directional: a spurious match retains a
// file that could have been deleted, and can never delete a live one.
func (a *agent) CheckDocumentFileReferenced(ctx context.Context, id string, documentID xid.ID) (bool, error) {
	q := `
		SELECT EXISTS (
			SELECT 1 FROM document_branches
			WHERE fk_document_id = $1 AND position($2 in content::text) > 0
			UNION ALL
			SELECT 1 FROM document_branch_history_entries
			WHERE fk_document_id = $1 AND position($2 in content::text) > 0
			UNION ALL
			SELECT 1 FROM document_comments
			WHERE fk_document_id = $1 AND position($2 in content::text) > 0
			UNION ALL
			SELECT 1 FROM document_comment_replies r
			JOIN document_comments c ON c.id = r.fk_document_comment_id
			WHERE c.fk_document_id = $1 AND position($2 in r.content::text) > 0
		)
	`

	var referenced bool
	if err := sqlx.GetContext(ctx, a.sql, &referenced, q, documentID, id); err != nil {
		return false, err
	}

	return referenced, nil
}

// DeleteExpiredDocumentBranchHistoryEntries removes history entries created
// before the given time, except the newest entry of each branch. Age
// trimming happens here rather than on insert because an idle branch never
// inserts again, and its entries would otherwise pin the files they
// reference forever. The newest entry pins nothing the live content does
// not.
func (a *agent) DeleteExpiredDocumentBranchHistoryEntries(ctx context.Context, before time.Time) error {
	q, args := a.builder.Delete("document_branch_history_entries e").
		Where(sq.Lt{"e.created_at": before}).
		Where(`EXISTS (
			SELECT 1 FROM document_branch_history_entries n
			WHERE n.fk_branch_id = e.fk_branch_id
			AND (n.created_at, n.id) > (e.created_at, e.id)
		)`).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// selectDocumentFile prepares a sql select statement for fetching document files.
func (a *agent) selectDocumentFile(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_files.id AS "id"`,
		`document_files.location AS "location"`,
		`document_files.storage_key AS "storage_key"`,
		`document_files.fk_document_id AS "fk_document_id"`,
		`document_files.fk_organization_id AS "fk_organization_id"`,
		`document_files.name AS "name"`,
		`document_files.size AS "size"`,
		`document_files.content_type AS "content_type"`,
		`document_files.created_at AS "created_at"`,
		`document_files.unreferenced_at AS "unreferenced_at"`,
	).From("document_files")
}
