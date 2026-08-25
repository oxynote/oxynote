package db

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// _insertDocumentSortIndexConstraint names the unique constraint on
// (fk_organization_id, fk_parent_id, sort_index). Concurrent InsertDocument
// calls under the same parent race on it; InsertDocument catches the
// violation and retries.
const _insertDocumentSortIndexConstraint = "documents_sort_index_key"

// _insertDocumentMaxAttempts caps the retry loop in InsertDocument
// when concurrent inserts race on sort_index. Five attempts covers
// the AI assistant's typical fan-out (a handful of parallel
// create_document calls) without risking livelock.
const _insertDocumentMaxAttempts = 5

// _insertDocumentSavepoint names the savepoint isolating one insert attempt.
const _insertDocumentSavepoint = "document_insert"

// InsertDocument inserts a new document and its first branch atomically.
// Concurrent inserts under the same parent race on the sort_index unique
// constraint; the constraint guarantees correctness and a short retry loop
// smooths over the lost race. Each attempt runs inside a savepoint so a
// conflict can be retried even when the agent is already running in a
// caller's transaction, where a failed statement would otherwise abort it.
func (a *agent) InsertDocument(ctx context.Context, doc document.Document) error {
	for range _insertDocumentMaxAttempts {
		err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
			return withSavepoint(ctx, tx, _insertDocumentSavepoint, func() error {
				if ierr := a.insertDocumentRow(ctx, tx, doc); ierr != nil {
					return ierr
				}

				return a.insertDocumentBranch(ctx, tx, doc)
			})
		})
		if err == nil {
			return nil
		}

		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == _insertDocumentSortIndexConstraint {
			continue
		}

		return err
	}

	return fmt.Errorf(
		"InsertDocument: exhausted %d retries on sort_index race",
		_insertDocumentMaxAttempts,
	)
}

// withSavepoint runs fn inside a savepoint so a failed attempt rolls back on
// its own instead of aborting the surrounding transaction.
func withSavepoint(ctx context.Context, tx *sqlx.Tx, name string, fn func() error) error {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+name); err != nil {
		return err
	}

	if err := fn(); err != nil {
		if _, rerr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+name); rerr != nil {
			return rerr
		}

		return err
	}

	_, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+name)

	return err
}

// insertDocumentRow performs one attempt: pick the next sort_index as one
// past the highest sibling index within the organization, then insert.
// MAX rather than COUNT, so the gaps deletions leave behind are skipped
// over instead of colliding with a surviving sibling. Caller is expected
// to retry on a unique-violation on the sort_index constraint.
func (a *agent) insertDocumentRow(ctx context.Context, tx *sqlx.Tx, doc document.Document) error {
	var pos int64

	q, args := a.builder.Select("COALESCE(MAX(sort_index) + 1, 0)").From("documents").
		Where(sq.Eq{
			"fk_parent_id":       doc.ParentID,
			"fk_organization_id": doc.OrganizationID,
		}).MustSql()

	if err := sqlx.GetContext(ctx, tx, &pos, q, args...); err != nil {
		return err
	}

	q, args = a.builder.Insert("documents").
		SetMap(map[string]any{
			"id":                 doc.ID,
			"sort_index":         pos,
			"fk_organization_id": doc.OrganizationID,
			"fk_parent_id":       doc.ParentID,
			"created_at":         doc.CreatedAt,
			"fk_created_by":      doc.CreatedBy,
			"updated_at":         doc.UpdatedAt,
			"fk_last_updated_by": doc.LastUpdatedBy,
		}).MustSql()

	_, err := tx.ExecContext(ctx, q, args...)

	return err
}

// CheckDocumentExists returns nil if the document exists and belongs to the
// given organization, or sql.ErrNoRows if not found.
func (a *agent) CheckDocumentExists(ctx context.Context, id xid.ID, organizationID string) error {
	var n int

	q, args := a.builder.Select("1").From("documents").
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	return sqlx.GetContext(ctx, a.sql, &n, q, args...)
}

// CheckDocumentCycle reports whether making parentID the parent of id would
// create a cycle in the document tree, i.e. whether parentID is the document
// itself or one of its descendants. It walks the candidate parent's ancestor
// chain, so the work is bounded by the tree depth rather than its size. A
// parent outside the organization yields false; callers check existence
// separately.
func (a *agent) CheckDocumentCycle(ctx context.Context, id, parentID xid.ID, organizationID string) (bool, error) {
	const q = `
		WITH RECURSIVE ancestors AS (
			SELECT id, fk_parent_id
			FROM documents
			WHERE id = $1 AND fk_organization_id = $3
			UNION ALL
			SELECT d.id, d.fk_parent_id
			FROM documents d
			JOIN ancestors a ON d.id = a.fk_parent_id
			WHERE d.fk_organization_id = $3
		)
		SELECT EXISTS (SELECT 1 FROM ancestors WHERE id = $2)
	`

	var cycle bool

	if err := sqlx.GetContext(ctx, a.sql, &cycle, q, parentID, id, organizationID); err != nil {
		return false, err
	}

	return cycle, nil
}

// FetchDocument retrieves a document by its ID and organization ID,
// joined against the specified branch.
func (a *agent) FetchDocument(ctx context.Context, id xid.ID, organizationID, branchName string) (*document.Document, error) {
	q, args := a.selectDocumentWithBranch(a.builder.Select(), branchName).
		Where(sq.Eq{
			"documents.id":                 id,
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

// UpdateDocumentTree updates the tree of a document childrens in the database.
func (a *agent) UpdateDocumentTree(ctx context.Context, ss document.Summaries, organizationID string) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		// renumbering walks positions that transiently collide with rows not
		// yet updated, so the constraint is checked at commit here.
		if _, err := tx.ExecContext(ctx, "SET CONSTRAINTS "+_insertDocumentSortIndexConstraint+" DEFERRED"); err != nil {
			return err
		}

		for i, st := range ss {
			q, args := a.builder.Update("documents").
				SetMap(map[string]any{
					"sort_index": i,
				}).
				Where(sq.Eq{
					"id":                 st.ID,
					"fk_organization_id": organizationID,
				}).
				MustSql()

			_, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// FetchDocumentTree fetches the document tree.
func (a *agent) FetchDocumentTree(ctx context.Context, organizationID string) (document.Summaries, error) {
	q, args := a.selectDocumentTree(a.builder.Select(), organizationID).
		MustSql()

	return a.fetchDocumentTree(ctx, null.Value[xid.ID]{}, q, args...)
}

// FetchDocumentTreeByDocumentParentID fetchs the document tree for the given parent id.
func (a *agent) FetchDocumentTreeByDocumentParentID(ctx context.Context, parentID null.Value[xid.ID], organizationID string) (document.Summaries, error) {
	q, args := a.selectDocumentTree(a.builder.Select(), organizationID).
		Where(sq.Eq{
			"documents.fk_parent_id": parentID,
		}).
		MustSql()

	return a.fetchDocumentTree(ctx, parentID, q, args...)
}

// UpdateDocument updates the branch of an existing document in the database.
func (a *agent) UpdateDocument(ctx context.Context, doc document.Document) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Update("documents").
			SetMap(map[string]any{
				"updated_at":         doc.UpdatedAt,
				"fk_last_updated_by": doc.LastUpdatedBy,
			}).
			Where(sq.Eq{
				"id":                 doc.ID,
				"fk_organization_id": doc.OrganizationID,
			}).MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		if err := a.upsertDocumentBranch(ctx, tx, doc); err != nil {
			return err
		}

		return a.insertDocumentBranchChangelog(ctx, tx, doc.ID, doc.BranchID, doc.Changelog())
	})
}

// UpdateDocumentParentID re-parents a document, placing it last among its new
// siblings. The sort index is recomputed rather than carried over: it belongs
// to the previous parent's sequence and would otherwise collide with an
// existing sibling at the destination. One past the highest sibling index,
// not the sibling count — a destination whose sequence has gaps from
// deletions would make the count collide with a surviving sibling.
func (a *agent) UpdateDocumentParentID(ctx context.Context, id xid.ID, parentID null.Value[xid.ID], organizationID string) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		var pos int64

		q, args := a.builder.Select("COALESCE(MAX(sort_index) + 1, 0)").From("documents").
			Where(sq.Eq{
				"fk_parent_id":       parentID,
				"fk_organization_id": organizationID,
			}).MustSql()

		if err := sqlx.GetContext(ctx, tx, &pos, q, args...); err != nil {
			return err
		}

		q, args = a.builder.Update("documents").
			SetMap(map[string]any{
				"fk_parent_id": parentID,
				"sort_index":   pos,
			}).
			Where(sq.Eq{
				"id":                 id,
				"fk_organization_id": organizationID,
			}).MustSql()

		_, err := tx.ExecContext(ctx, q, args...)

		return err
	})
}

// DeleteDocument deletes a document from the database, returning the ids
// of the document and of every descendant the delete cascaded to.
// Deleting the row destroys the subtree, so the ids are collected here
// rather than by the caller: after the delete there is nothing left to
// tell the search index which documents went away. The caller queues the
// index removal from them, in the same transaction when it runs in one.
func (a *agent) DeleteDocument(ctx context.Context, id xid.ID, organizationID string) ([]xid.ID, error) {
	var ids []xid.ID

	err := sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		var err error

		ids, err = a.fetchDocumentSubtreeIDs(ctx, tx, id, organizationID)
		if err != nil {
			return err
		}

		q, args := a.builder.Delete("documents").
			Where(sq.Eq{
				"id":                 id,
				"fk_organization_id": organizationID,
			}).MustSql()

		_, err = tx.ExecContext(ctx, q, args...)

		return err
	})
	if err != nil {
		return nil, err
	}

	return ids, nil
}

// fetchDocumentSubtreeIDs retrieves the IDs of the document and of every
// document below it in the tree.
func (a *agent) fetchDocumentSubtreeIDs(
	ctx context.Context,
	q sqlx.QueryerContext,
	id xid.ID,
	organizationID string,
) ([]xid.ID, error) {
	const query = `
		WITH RECURSIVE subtree AS (
			SELECT id
			FROM documents
			WHERE id = $1 AND fk_organization_id = $2
			UNION ALL
			SELECT d.id
			FROM documents d
			JOIN subtree s ON d.fk_parent_id = s.id
			WHERE d.fk_organization_id = $2
		)
		SELECT id FROM subtree
	`

	var ids []xid.ID

	if err := sqlx.SelectContext(ctx, q, &ids, query, id, organizationID); err != nil {
		return nil, err
	}

	return ids, nil
}

// fetchDocumentTree fetchs the document tree for the given query and arguments.
// Rows arrive ordered by sort index alone, so parents and children interleave.
// The tree is therefore indexed first and materialised afterwards: attaching
// children while scanning would mean holding pointers into child slices that
// a later append can reallocate, silently dropping whole subtrees.
func (a *agent) fetchDocumentTree(ctx context.Context, rootID null.Value[xid.ID], q string, args ...any) (document.Summaries, error) {
	var data []struct {
		*document.Summary

		ParentID null.Value[xid.ID] `db:"fk_parent_id"`
	}

	if err := sqlx.SelectContext(ctx, a.sql, &data, q, args...); err != nil {
		return nil, err
	}

	var (
		root      []xid.ID
		summaries = make(map[xid.ID]*document.Summary, len(data))
		children  = make(map[xid.ID][]xid.ID)
	)

	for _, d := range data {
		summaries[d.ID] = d.Summary

		if d.ParentID == rootID {
			root = append(root, d.ID)

			continue
		}

		children[d.ParentID.V] = append(children[d.ParentID.V], d.ID)
	}

	res := make(document.Summaries, 0, len(root))

	for _, id := range root {
		res = append(res, buildDocumentSubtree(id, summaries, children))
	}

	return res, nil
}

// buildDocumentSubtree materialises the subtree rooted at the given id.
// Documents whose parent is absent from the result set are unreachable from
// any root and stay out of the tree, as they did before.
func buildDocumentSubtree(
	id xid.ID,
	summaries map[xid.ID]*document.Summary,
	children map[xid.ID][]xid.ID,
) document.Summary {
	res := *summaries[id]

	ids := children[id]
	if len(ids) == 0 {
		return res
	}

	res.Children = make(document.Summaries, 0, len(ids))

	for _, childID := range ids {
		res.Children = append(res.Children, buildDocumentSubtree(childID, summaries, children))
	}

	return res
}

// selectDocumentTree prepares a sql select statement for fetching the document tree.
func (a *agent) selectDocumentTree(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		`documents.id AS "id"`,
		`db.document_name AS "document_name"`,
		`db.icon AS "icon"`,
		`db.protected AS "protected"`,
		`documents.fk_parent_id AS "fk_parent_id"`,
	).From("documents").
		// joined on the flag rather than the name, which is user-facing:
		// a renamed main branch would leave every row nameless.
		LeftJoin(`document_branches db ON db.fk_document_id = documents.id AND db."default"`).
		Where(sq.Eq{
			"documents.fk_organization_id": organizationID,
		}).
		OrderBy("documents.sort_index")
}

// selectDocumentWithBranch narrows the document-branch select down to the
// branch carrying the given name.
func (a *agent) selectDocumentWithBranch(b sq.SelectBuilder, branchName string) sq.SelectBuilder {
	return a.selectDocumentBranch(b).Where(sq.Eq{"db.branch_name": branchName})
}
