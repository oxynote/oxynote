package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// insertDocumentBranch inserts a branch row for a newly created document.
// Called inside the same transaction as InsertDocument.
func (a *agent) insertDocumentBranch(ctx context.Context, tx *sqlx.Tx, doc document.Document) error {
	q, args := a.builder.Insert("document_branches").
		SetMap(map[string]any{
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
		}).
		Suffix("ON CONFLICT (fk_document_id, branch_name) DO NOTHING").
		MustSql()

	_, err := tx.ExecContext(ctx, q, args...)

	return err
}

// upsertDocumentBranch upserts the branch row for a document through the
// given executor. Called inside the same transaction as UpdateDocument.
func (a *agent) upsertDocumentBranch(ctx context.Context, ex sqlx.ExecerContext, doc document.Document) error {
	q, args := a.builder.Insert("document_branches").
		SetMap(map[string]any{
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
		}).
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

// ForkDocumentBranch creates a new branch by copying the contents of an
// existing source branch. A taken target name surfaces as the duplicate-name
// unique violation — swallowing it would hand the caller the pre-existing
// branch as if it were freshly forked.
func (a *agent) ForkDocumentBranch(
	ctx context.Context,
	docID xid.ID,
	orgID string,
	sourceBranch string,
	targetBranch string,
	createdBy string,
) error {
	now := timeutil.Now()

	q := `
		INSERT INTO document_branches (
			id, fk_document_id, fk_organization_id, branch_name, document_name, icon,
			content, raw_content, protected, "default",
			created_at, fk_created_by, updated_at, fk_last_updated_by
		)
		SELECT $1, $2, $3, $4, document_name, icon, content, raw_content, protected, false, $5, $6, $5, $6
		FROM document_branches
		WHERE fk_document_id = $2 AND branch_name = $7
	`

	_, err := a.sql.ExecContext(ctx, q, xid.New(), docID, orgID, targetBranch, now, createdBy, sourceBranch)

	return err
}

// FetchMainBranchContent returns the parsed content and display name
// of a document's main branch, scoped to the given org. Used by the
// assistant's read tools, which only need these fields and would
// otherwise pay to deserialize raw_content and the rest of the
// branch row.
func (a *agent) FetchMainBranchContent(ctx context.Context, docID xid.ID, organizationID string) (document.Content, error) {
	q, args := a.builder.Select(
		`document_name AS "document_name"`,
		`content AS "content"`,
	).From("document_branches").
		Where(sq.Eq{
			"fk_document_id":     docID,
			"fk_organization_id": organizationID,
			// the flag, not the name: the name is user-facing and the
			// document would read as contentless the moment it changed.
			`"default"`: true,
		}).
		Limit(1).
		MustSql()

	out := document.Content{
		OrganizationID: organizationID,
		DocumentID:     docID,
	}
	if err := sqlx.GetContext(ctx, a.sql, &out, q, args...); err != nil {
		return document.Content{}, err
	}

	return out, nil
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
// It does not modify content or insert a changelog entry.
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

// insertDocumentBranchChangelog inserts a changelog entry for a branch update
// through the given executor. The entry is keyed by branch id rather than
// branch name so that renaming a branch cannot break the reference.
func (a *agent) insertDocumentBranchChangelog(
	ctx context.Context,
	ex sqlx.ExecerContext,
	docID, branchID xid.ID,
	clog document.Changelog,
) error {
	q, args := a.builder.Insert("document_branch_changelogs").
		SetMap(map[string]any{
			"id":             clog.ID,
			"fk_document_id": docID,
			"fk_branch_id":   branchID,
			"content":        clog.Content,
			"raw_content":    clog.RawContent,
			"created_at":     clog.CreatedAt,
		}).
		Suffix("ON CONFLICT (id) DO UPDATE SET " +
			"content = excluded.content, raw_content = excluded.raw_content, created_at = excluded.created_at").
		MustSql()

	if _, err := ex.ExecContext(ctx, q, args...); err != nil {
		return err
	}

	return a.trimDocumentBranchChangelogs(ctx, ex, branchID)
}

// trimDocumentBranchChangelogs keeps only the newest snapshots of a branch.
// Age trimming lives in the file manager instead, since it has to reach
// branches that stopped inserting altogether.
func (a *agent) trimDocumentBranchChangelogs(ctx context.Context, ex sqlx.ExecerContext, branchID xid.ID) error {
	// a zero limit means unlimited retention; without this guard the
	// subquery below would emit LIMIT 0 and the delete would drop every
	// snapshot of the branch.
	if a.opts.MaxDocumentChangelogs == 0 {
		return nil
	}

	b := a.builder.Select("id").
		From("document_branch_changelogs").
		Where(sq.Eq{"fk_branch_id": branchID}).
		OrderBy("created_at DESC").
		Limit(a.opts.MaxDocumentChangelogs).
		Prefix("id NOT IN (").
		Suffix(")")

	q, args := a.builder.Delete("document_branch_changelogs").Where(sq.And{
		b,
		sq.Eq{"fk_branch_id": branchID},
	}).MustSql()

	_, err := ex.ExecContext(ctx, q, args...)

	return err
}
