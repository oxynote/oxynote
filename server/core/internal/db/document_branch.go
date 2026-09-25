package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/history"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _historyAggregationDuration is the length of the time bucket that
// ordinary history entries of one branch collapse into.
var _historyAggregationDuration = 30 * time.Minute

// insertDocumentBranch inserts a branch row for a newly created document.
// Called inside the same transaction as InsertDocument.
func (a *agent) insertDocumentBranch(ctx context.Context, tx *sqlx.Tx, doc document.Document) error {
	q, args := a.builder.Insert("document_branches").
		SetMap(documentBranchRow(doc)).
		Suffix("ON CONFLICT (fk_document_id, branch_name) DO NOTHING").
		MustSql()

	_, err := tx.ExecContext(ctx, q, args...)

	return err
}

// upsertDocumentBranch upserts the branch row for a document through the
// given executor. Called inside the same transaction as UpdateDocument.
func (a *agent) upsertDocumentBranch(ctx context.Context, ex sqlx.ExecerContext, doc document.Document) error {
	q, args := a.builder.Insert("document_branches").
		SetMap(documentBranchRow(doc)).
		// "default" is intentionally omitted from the SET clause — it is
		// set once at branch creation and must never change via upsert.
		Suffix("ON CONFLICT (fk_document_id, branch_name) DO UPDATE SET " +
			"document_name = excluded.document_name, icon = excluded.icon, " +
			"content = excluded.content, raw_content = excluded.raw_content, " +
			"protected = excluded.protected, updated_at = excluded.updated_at, " +
			"fk_last_updated_by = excluded.fk_last_updated_by").
		MustSql()

	_, err := ex.ExecContext(ctx, q, args...)

	return err
}

// InsertDocumentBranch inserts a branch row for an existing document. A
// taken branch name surfaces as the duplicate-name unique violation —
// swallowing it would hand the caller the pre-existing branch as if it were
// freshly created.
func (a *agent) InsertDocumentBranch(ctx context.Context, doc document.Document) error {
	q, args := a.builder.Insert("document_branches").
		SetMap(documentBranchRow(doc)).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentBranches fetches all branches for a document as lightweight summaries.
func (a *agent) FetchDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) ([]document.BranchSummary, error) {
	q, args := a.selectBranchSummary(a.builder.Select()).
		Where(sq.Eq{
			"fk_document_id":     docID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	branches := []document.BranchSummary{}

	if err := sqlx.SelectContext(ctx, a.sql, &branches, q, args...); err != nil {
		return nil, err
	}

	return branches, nil
}

// CountDocumentBranches returns the number of branches for a document.
// Used to enforce the minimum of one branch before deletion.
func (a *agent) CountDocumentBranches(ctx context.Context, docID xid.ID, organizationID string) (int, error) {
	q, args := a.builder.Select("COUNT(*)").
		From("document_branches").
		Where(sq.Eq{
			"fk_document_id":     docID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	var count int

	if err := sqlx.GetContext(ctx, a.sql, &count, q, args...); err != nil {
		return 0, err
	}

	return count, nil
}

// DeleteDocumentBranchByID deletes a branch identified by its ID.
// The caller is responsible for ensuring at least one branch remains.
func (a *agent) DeleteDocumentBranchByID(ctx context.Context, branchID xid.ID, organizationID string) error {
	q, args := a.builder.Delete("document_branches").
		Where(sq.Eq{
			"id":                 branchID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateDocumentBranchMetadata updates the name and protection status of a branch.
// It does not modify content or insert a history entry.
func (a *agent) UpdateDocumentBranchMetadata(ctx context.Context, doc document.Document) error {
	q, args := a.builder.Update("document_branches").
		SetMap(map[string]any{
			"branch_name":        doc.BranchName,
			"protected":          doc.Protected,
			"updated_at":         doc.UpdatedAt,
			"fk_last_updated_by": doc.LastUpdatedBy,
		}).
		Where(sq.Eq{
			"id":                 doc.BranchID,
			"fk_organization_id": doc.OrganizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentByBranchID fetches a document joined against the branch identified by branchID.
func (a *agent) FetchDocumentByBranchID(ctx context.Context, branchID xid.ID, organizationID string) (*document.Document, error) {
	return a.fetchDocumentBranchByID(ctx, a.sql, branchID, organizationID, false)
}

// FetchDocumentBranchesUnsafe fetches all branches for a document as lightweight
// summaries without checking organization ownership.
// This is intended only for internal system use cases.
func (a *agent) FetchDocumentBranchesUnsafe(ctx context.Context, docID xid.ID) ([]document.BranchSummary, error) {
	q, args := a.selectBranchSummary(a.builder.Select()).
		Where(sq.Eq{
			"fk_document_id": docID,
		}).
		MustSql()

	branches := []document.BranchSummary{}

	if err := sqlx.SelectContext(ctx, a.sql, &branches, q, args...); err != nil {
		return nil, err
	}

	return branches, nil
}

// FetchDocumentUnsafeByBranchID fetches a document joined against the branch
// identified by branchID without checking organization ownership.
// This is intended only for internal system use cases.
func (a *agent) FetchDocumentUnsafeByBranchID(ctx context.Context, branchID xid.ID) (*document.Document, error) {
	q, args := a.selectDocumentBranch(a.builder.Select()).
		Where(sq.Eq{
			"db.id": branchID,
		}).
		Limit(1).
		MustSql()

	doc := &document.Document{}

	if err := sqlx.GetContext(ctx, a.sql, doc, q, args...); err != nil {
		return nil, err
	}

	return doc, nil
}

// FetchDocumentBranchesAfter fetches up to limit branches whose id sorts
// after the given one, joined against their document, in id order and
// across every organization.
func (a *agent) FetchDocumentBranchesAfter(ctx context.Context, after xid.ID, limit int) ([]document.Document, error) {
	// a zero id is compared as its string form, since its driver value
	// would be a null the comparison cannot take.
	q, args := a.selectDocumentBranch(a.builder.Select()).
		Where(sq.Gt{
			"db.id": after.String(),
		}).
		OrderBy("db.id ASC").
		Limit(uint64(limit)).
		MustSql()

	docs := []document.Document{}

	if err := sqlx.SelectContext(ctx, a.sql, &docs, q, args...); err != nil {
		return nil, err
	}

	return docs, nil
}

// selectDocumentBranch prepares a select statement joining a document against
// one of its branches. The organization scope is left to the caller, since
// the unsafe variants exist precisely to go without it.
func (a *agent) selectDocumentBranch(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`db.id AS "branch_id"`,
		`documents.id AS "id"`,
		`documents.fk_organization_id AS "fk_organization_id"`,
		`documents.fk_parent_id AS "fk_parent_id"`,
		`db.branch_name AS "branch_name"`,
		`db.document_name AS "document_name"`,
		`db.icon AS "icon"`,
		`db.content AS "content"`,
		`db.raw_content AS "raw_content"`,
		`db.protected AS "protected"`,
		`db."default" AS "default"`,
		`db.created_at AS "created_at"`,
		`db.fk_created_by AS "fk_created_by"`,
		`db.updated_at AS "updated_at"`,
		`db.fk_last_updated_by AS "fk_last_updated_by"`,
	).From("documents").
		Join("document_branches db ON db.fk_document_id = documents.id")
}

// selectBranchSummary prepares a select statement for the lightweight branch
// summary columns. The organization scope is left to the caller, since the
// unsafe variants exist precisely to go without it.
func (a *agent) selectBranchSummary(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`id AS "branch_id"`,
		`branch_name AS "branch_name"`,
		`document_name AS "document_name"`,
		`icon AS "icon"`,
		`protected AS "protected"`,
		`"default" AS "default"`,
		`created_at AS "created_at"`,
		`updated_at AS "updated_at"`,
	).From("document_branches")
}

// RecordDocumentBranchHistoryEntry records the branch and its hooks and
// returns the id of the entry that holds them. It locks the branch row, so
// writers cannot race on the newest entry.
func (a *agent) RecordDocumentBranchHistoryEntry(
	ctx context.Context,
	branchID xid.ID,
	organizationID string,
	by null.String,
	boundary bool,
) (xid.ID, error) {
	var id xid.ID

	if err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		doc, err := a.fetchDocumentBranchByID(ctx, tx, branchID, organizationID, true)
		if err != nil {
			return err
		}

		hooks, err := a.fetchDocumentBranchHistoryHooks(ctx, tx, branchID)
		if err != nil {
			return err
		}

		entry := history.NewEntry(
			*doc,
			timeutil.Now(),
			by,
			history.NewHooks(doc.Content, hooks),
			boundary,
		)

		id, err = a.insertDocumentBranchHistoryEntry(ctx, tx, entry)

		return err
	}); err != nil {
		return xid.ID{}, err
	}

	return id, nil
}

// insertDocumentBranchHistoryEntry writes the entry, folds it into the
// branch's newest one, or skips it when it matches the newest one, and
// returns the id of the row that holds it. The caller holds the branch row
// lock.
func (a *agent) insertDocumentBranchHistoryEntry(ctx context.Context, tx *sqlx.Tx, entry history.Entry) (xid.ID, error) {
	newest, err := a.fetchNewestDocumentBranchHistoryEntry(ctx, tx, entry)
	if err != nil {
		return xid.ID{}, err
	}

	if newest != nil && newest.Same && !entry.Boundary {
		return newest.ID, nil
	}

	aggregates := newest != nil &&
		!newest.Boundary &&
		!entry.Boundary &&
		newest.CreatedAt.Truncate(_historyAggregationDuration).Equal(entry.CreatedAt.Truncate(_historyAggregationDuration))

	if aggregates {
		set := map[string]any{
			"document_name": entry.DocumentName,
			"icon":          entry.Icon,
			"content":       entry.Content,
			"hooks":         entry.Hooks,
			"updated_at":    entry.UpdatedAt,
		}

		// a system write has no author, and the entry still holds the
		// edits of the one it has.
		if entry.LastUpdatedBy.Valid {
			set["fk_last_updated_by"] = entry.LastUpdatedBy
		}

		q, args := a.builder.Update("document_branch_history_entries").
			SetMap(set).
			Where(sq.Eq{"id": newest.ID}).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return xid.ID{}, err
		}

		return newest.ID, nil
	}

	q, args := a.builder.Insert("document_branch_history_entries").
		SetMap(map[string]any{
			"id":                 entry.ID,
			"fk_document_id":     entry.DocumentID,
			"fk_branch_id":       entry.BranchID,
			"document_name":      entry.DocumentName,
			"icon":               entry.Icon,
			"content":            entry.Content,
			"hooks":              entry.Hooks,
			"fk_last_updated_by": entry.LastUpdatedBy,
			"boundary":           entry.Boundary,
			"created_at":         entry.CreatedAt,
			"updated_at":         entry.UpdatedAt,
		}).
		MustSql()

	if _, err := tx.ExecContext(ctx, q, args...); err != nil {
		return xid.ID{}, err
	}

	if err := a.trimDocumentBranchHistoryEntries(ctx, tx, entry.BranchID); err != nil {
		return xid.ID{}, err
	}

	return entry.ID, nil
}

// fetchDocumentBranchByID reads the branch joined against its document.
// With lock, it also locks the branch row until the transaction ends.
func (a *agent) fetchDocumentBranchByID(
	ctx context.Context,
	q sqlx.QueryerContext,
	branchID xid.ID,
	organizationID string,
	lock bool,
) (*document.Document, error) {
	b := a.selectDocumentBranch(a.builder.Select()).
		Where(sq.Eq{
			"db.id":                        branchID,
			"documents.fk_organization_id": organizationID,
		})

	if lock {
		b = b.Suffix("FOR UPDATE OF db")
	}

	sqlq, args := b.MustSql()

	doc := &document.Document{}

	if err := sqlx.GetContext(ctx, q, doc, sqlq, args...); err != nil {
		return nil, err
	}

	return doc, nil
}

// fetchDocumentBranchHistoryHooks fetches every hook of the branch a
// history entry may list, soft-deleted ones included, in a stable order so
// equal hook sets compare equal. history.NewHooks picks the ones the
// entry's content holds.
func (a *agent) fetchDocumentBranchHistoryHooks(ctx context.Context, q sqlx.QueryerContext, branchID xid.ID) ([]hook.Hook, error) {
	sqlq, args := a.selectDocumentHook(a.builder.Select()).
		Where(sq.Eq{"document_hooks.fk_branch_id": branchID}).
		OrderBy("document_hooks.created_at ASC", "document_hooks.id ASC").
		MustSql()

	hooks := []hook.Hook{}

	if err := sqlx.SelectContext(ctx, q, &hooks, sqlq, args...); err != nil {
		return nil, err
	}

	return hooks, nil
}

// fetchNewestDocumentBranchHistoryEntry retrieves the newest history entry
// of the given entry's branch, or nil when the branch has none. The
// comparison with the entry runs in Postgres, so the stored content never
// leaves it.
func (a *agent) fetchNewestDocumentBranchHistoryEntry(ctx context.Context, q sqlx.QueryerContext, entry history.Entry) (*historyHead, error) {
	sqlq, args := a.builder.Select("id", "boundary", "created_at").
		Column(sq.Alias(sq.Expr(
			"document_name = ? AND icon = ? AND content = ?::jsonb AND hooks = ?::jsonb",
			entry.DocumentName,
			entry.Icon,
			entry.Content,
			entry.Hooks,
		), "same")).
		From("document_branch_history_entries").
		Where(sq.Eq{"fk_branch_id": entry.BranchID}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		MustSql()

	var newest historyHead

	err := sqlx.GetContext(ctx, q, &newest, sqlq, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil // no entry is a regular outcome, not an error
		}

		return nil, err
	}

	return &newest, nil
}

// trimDocumentBranchHistoryEntries keeps only the newest entries of a branch.
func (a *agent) trimDocumentBranchHistoryEntries(ctx context.Context, ex sqlx.ExecerContext, branchID xid.ID) error {
	// a zero limit means unlimited retention; without this guard the
	// subquery below would emit LIMIT 0 and the delete would drop every
	// snapshot of the branch.
	if a.opts.MaxDocumentHistoryEntries == 0 {
		return nil
	}

	b := a.builder.Select("id").
		From("document_branch_history_entries").
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("created_at DESC", "id DESC").
		Limit(a.opts.MaxDocumentHistoryEntries).
		Prefix("id NOT IN (").
		Suffix(")")

	q, args := a.builder.Delete("document_branch_history_entries").Where(sq.And{
		b,
		sq.Eq{"fk_branch_id": branchID},
	}).MustSql()

	_, err := ex.ExecContext(ctx, q, args...)

	return err
}

// historyHead is what placing a new history entry needs to know about
// the branch's newest one.
type historyHead struct {
	// ID is the unique identifier for the entry.
	ID xid.ID `db:"id"`

	// Boundary indicates whether the entry closes its bucket.
	Boundary bool `db:"boundary"`

	// CreatedAt is the timestamp when the entry was taken.
	CreatedAt time.Time `db:"created_at"`

	// Same reports whether the entry records the same name, icon, content
	// and hooks as the one being placed.
	Same bool `db:"same"`
}

// documentBranchRow maps a document's branch onto the document_branches
// columns.
func documentBranchRow(doc document.Document) map[string]any {
	return map[string]any{
		"id":                 doc.BranchID,
		"fk_document_id":     doc.ID,
		"fk_organization_id": doc.OrganizationID,
		"branch_name":        doc.BranchName,
		"document_name":      doc.DocumentName,
		"icon":               doc.Icon,
		"content":            doc.Content,
		"protected":          doc.Protected,
		`"default"`:          doc.Default,
		"raw_content":        doc.RawContent,
		"created_at":         doc.CreatedAt,
		"fk_created_by":      doc.CreatedBy,
		"updated_at":         doc.UpdatedAt,
		"fk_last_updated_by": doc.LastUpdatedBy,
	}
}
