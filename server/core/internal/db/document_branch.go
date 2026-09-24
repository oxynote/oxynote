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
	q, args := a.selectDocumentBranch(a.builder.Select()).
		Where(sq.Eq{
			"db.id":                        branchID,
			"documents.fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	doc := &document.Document{}

	if err := sqlx.GetContext(ctx, a.sql, doc, q, args...); err != nil {
		return nil, err
	}

	return doc, nil
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

// RecordDocumentBranchHistoryEntry records the branch as it stands, with
// its live hooks, and returns the id of the entry it wrote. It locks the
// branch row before reading it, so two writers cannot race on the newest
// entry. Called inside the transaction that wrote the branch, it records
// what that transaction wrote.
//
// Ordinary entries aggregate: an edit in the same 30-minute bucket as the
// branch's newest ordinary entry updates that entry in place. A boundary
// entry (create, duplicate, fork, merge) is always its own row and closes
// the bucket. The count trim runs afterwards; age trimming lives in the
// file manager, since it has to reach branches that stopped writing.
func (a *agent) RecordDocumentBranchHistoryEntry(
	ctx context.Context,
	branchID xid.ID,
	organizationID string,
	by null.String,
	boundary bool,
) (xid.ID, error) {
	hooks, err := a.FetchDocumentHooksByBranchID(ctx, branchID, organizationID)
	if err != nil {
		return xid.ID{}, err
	}

	var id xid.ID

	if err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.selectDocumentBranch(a.builder.Select()).
			Where(sq.Eq{
				"db.id":                        branchID,
				"documents.fk_organization_id": organizationID,
			}).
			Suffix("FOR UPDATE OF db").
			MustSql()

		var doc document.Document

		if err := sqlx.GetContext(ctx, tx, &doc, q, args...); err != nil {
			return err
		}

		entry := history.NewEntry(
			doc,
			timeutil.Now(),
			by,
			history.NewHooks(hooks),
			boundary,
		)

		var err error

		id, err = a.insertDocumentBranchHistoryEntry(ctx, tx, entry)

		return err
	}); err != nil {
		return xid.ID{}, err
	}

	return id, nil
}

// insertDocumentBranchHistoryEntry writes the entry, or folds it into the
// branch's newest one, and returns the id of the row it wrote. The caller
// holds the branch row lock.
func (a *agent) insertDocumentBranchHistoryEntry(ctx context.Context, tx *sqlx.Tx, entry history.Entry) (xid.ID, error) {
	newest, err := a.fetchNewestDocumentBranchHistoryEntry(ctx, tx, entry.BranchID)
	if err != nil {
		return xid.ID{}, err
	}

	aggregates := newest != nil &&
		!newest.Boundary &&
		!entry.Boundary &&
		newest.CreatedAt.Truncate(_historyAggregationDuration).Equal(entry.CreatedAt.Truncate(_historyAggregationDuration))

	if aggregates {
		q, args := a.builder.Update("document_branch_history_entries").
			SetMap(map[string]any{
				"document_name":      entry.DocumentName,
				"icon":               entry.Icon,
				"content":            entry.Content,
				"hooks":              entry.Hooks,
				"fk_last_updated_by": entry.LastUpdatedBy,
				"created_at":         entry.CreatedAt,
			}).
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

// UpdateDocumentBranchHistoryEntryHooks replaces the hooks an entry lists.
// A boundary entry is written before its branch's hooks are copied, so it
// gets the copied ones afterwards.
func (a *agent) UpdateDocumentBranchHistoryEntryHooks(ctx context.Context, id xid.ID, hooks history.Hooks) error {
	q, args := a.builder.Update("document_branch_history_entries").
		Set("hooks", hooks).
		Where(sq.Eq{"id": id}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// fetchNewestDocumentBranchHistoryEntry retrieves the branch's newest
// history entry, or nil when the branch has none.
func (a *agent) fetchNewestDocumentBranchHistoryEntry(ctx context.Context, q sqlx.QueryerContext, branchID xid.ID) (*history.Entry, error) {
	sqlq, args := a.selectDocumentBranchHistoryEntry(a.builder.Select()).
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("created_at DESC", "id DESC").
		Limit(1).
		MustSql()

	var entry history.Entry

	err := sqlx.GetContext(ctx, q, &entry, sqlq, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil //nolint:nilnil // no entry is a regular outcome, not an error
		}

		return nil, err
	}

	return &entry, nil
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

// selectDocumentBranchHistoryEntry prepares a sql select statement for
// fetching history entries.
func (a *agent) selectDocumentBranchHistoryEntry(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`id AS "id"`,
		`fk_document_id AS "fk_document_id"`,
		`fk_branch_id AS "fk_branch_id"`,
		`document_name AS "document_name"`,
		`icon AS "icon"`,
		`content AS "content"`,
		`hooks AS "hooks"`,
		`fk_last_updated_by AS "fk_last_updated_by"`,
		`boundary AS "boundary"`,
		`created_at AS "created_at"`,
	).From("document_branch_history_entries")
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
