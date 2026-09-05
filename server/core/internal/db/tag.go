package db

import (
	"context"
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/tag"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// InsertTag inserts a new tag at the end of its organization's tags. The
// position is one past the highest index rather than the count, so the gaps
// deletions leave behind are skipped over; sort_index carries no uniqueness
// constraint, so two concurrent inserts landing on the same index are
// tolerated and ordered by id.
func (a *agent) InsertTag(ctx context.Context, t tag.Tag) error {
	const q = `
		INSERT INTO tags (id, fk_organization_id, tag_name, color, sort_index, created_at, fk_created_by)
		SELECT $1, $2, $3, $4, COALESCE(MAX(sort_index) + 1, 0), $5, $6
		FROM tags
		WHERE fk_organization_id = $2
	`

	_, err := a.sql.ExecContext(
		ctx,
		q,
		t.ID,
		t.OrganizationID,
		t.TagName,
		t.Color,
		t.CreatedAt,
		t.CreatedBy,
	)

	return err
}

// FetchTagTree retrieves an organization's tags in their display order,
// each with the documents whose default branch carries it. Hidden reflects
// the given user's own setting; a tag the user never touched reads as
// visible. Documents are listed in tree order with their own subtrees.
func (a *agent) FetchTagTree(ctx context.Context, organizationID, userID string) (tag.Summaries, error) {
	tree, err := a.FetchDocumentTree(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	q, args := a.selectTagSummary(a.builder.Select(), organizationID, userID).MustSql()

	summaries := tag.Summaries{}

	if err = sqlx.SelectContext(ctx, a.sql, &summaries, q, args...); err != nil {
		return nil, err
	}

	if len(summaries) == 0 {
		return summaries, nil
	}

	q, args = a.builder.Select(
		`dbt.fk_tag_id AS "tag_id"`,
		`db.fk_document_id AS "document_id"`,
	).From("document_branch_tags dbt").
		Join(`document_branches db ON db.id = dbt.fk_branch_id AND db."default"`).
		Where(sq.Eq{"dbt.fk_organization_id": organizationID}).
		MustSql()

	var assignments []struct {
		TagID      xid.ID `db:"tag_id"`
		DocumentID xid.ID `db:"document_id"`
	}

	if err = sqlx.SelectContext(ctx, a.sql, &assignments, q, args...); err != nil {
		return nil, err
	}

	assigned := make(map[xid.ID]map[xid.ID]bool, len(summaries))

	for _, as := range assignments {
		if assigned[as.TagID] == nil {
			assigned[as.TagID] = map[xid.ID]bool{}
		}

		assigned[as.TagID][as.DocumentID] = true
	}

	descendants := tree.Descendants()

	for i, s := range summaries {
		docs := document.Summaries{}

		for _, doc := range descendants {
			if assigned[s.ID][doc.ID] {
				docs = append(docs, doc)
			}
		}

		summaries[i].Documents = docs
	}

	return summaries, nil
}

// FetchBranchTagIDs retrieves the ids of the tags a document's branch
// carries, in the tags' display order.
func (a *agent) FetchBranchTagIDs(ctx context.Context, organizationID string, documentID, branchID xid.ID) ([]xid.ID, error) {
	q, args := a.builder.Select("dbt.fk_tag_id").
		From("document_branch_tags dbt").
		Join("tags t ON t.id = dbt.fk_tag_id").
		Join("document_branches db ON db.id = dbt.fk_branch_id").
		Where(sq.Eq{
			"dbt.fk_branch_id":       branchID,
			"dbt.fk_organization_id": organizationID,
			"db.fk_document_id":      documentID,
		}).
		OrderBy("t.sort_index", "t.id").
		MustSql()

	ids := []xid.ID{}

	if err := sqlx.SelectContext(ctx, a.sql, &ids, q, args...); err != nil {
		return nil, err
	}

	return ids, nil
}

// UpdateTagTree rewrites the display order of an organization's tags to the
// order of the given tree. A tag outside the organization aborts the whole
// rewrite.
func (a *agent) UpdateTagTree(ctx context.Context, tree tag.Summaries, organizationID string) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		for i, s := range tree {
			q, args := a.builder.Update("tags").
				SetMap(map[string]any{
					"sort_index": i,
				}).
				Where(sq.Eq{
					"id":                 s.ID,
					"fk_organization_id": organizationID,
				}).
				MustSql()

			res, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			if n == 0 {
				return errutil.ErrNotFound
			}
		}

		return nil
	})
}

// SetTagVisibility records whether one user keeps a tag out of their
// sidebar, leaving every other user's setting untouched.
func (a *agent) SetTagVisibility(
	ctx context.Context,
	organizationID, userID string,
	id xid.ID,
	inp tag.VisibilityInput,
) error {
	// the row is selected out of tags rather than inserted outright so a
	// tag outside the organization inserts nothing instead of a setting
	// pointing across tenants.
	const q = `
		INSERT INTO user_tag_settings (fk_user_id, fk_tag_id, hidden)
		SELECT $1, id, $2
		FROM tags
		WHERE id = $3 AND fk_organization_id = $4
		ON CONFLICT (fk_user_id, fk_tag_id) DO UPDATE SET hidden = EXCLUDED.hidden
	`

	res, err := a.sql.ExecContext(ctx, q, userID, inp.Hidden, id, organizationID)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// DeleteTag removes a tag; its branch assignments and every user's setting
// for it go with it through the cascades.
func (a *agent) DeleteTag(ctx context.Context, id xid.ID, organizationID string) error {
	q, args := a.builder.Delete("tags").
		Where(sq.Eq{
			"id":                 id,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	res, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n == 0 {
		return errutil.ErrNotFound
	}

	return nil
}

// AssignBranchTag makes a document's branch carry a tag. Assigning a tag
// the branch already carries changes nothing; a branch or tag outside the
// organization, or a branch of another document, is not found.
func (a *agent) AssignBranchTag(ctx context.Context, organizationID string, documentID, branchID, tagID xid.ID) error {
	const q = `
		INSERT INTO document_branch_tags (fk_branch_id, fk_tag_id, fk_organization_id)
		SELECT b.id, t.id, b.fk_organization_id
		FROM document_branches b
		JOIN tags t ON t.fk_organization_id = b.fk_organization_id
		WHERE b.id = $1 AND b.fk_document_id = $2 AND t.id = $3 AND b.fk_organization_id = $4
		ON CONFLICT DO NOTHING
	`

	res, err := a.sql.ExecContext(ctx, q, branchID, documentID, tagID, organizationID)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}

	if n > 0 {
		return nil
	}

	// nothing inserted means either the pair already exists or one side
	// is missing, and only the former is a success.
	var exists int

	eq, eargs := a.builder.Select("1").
		From("document_branch_tags").
		Where(sq.Eq{
			"fk_branch_id":       branchID,
			"fk_tag_id":          tagID,
			"fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	if err = sqlx.GetContext(ctx, a.sql, &exists, eq, eargs...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errutil.ErrNotFound
		}

		return err
	}

	return nil
}

// UnassignBranchTag stops a document's branch carrying a tag. Removing a
// tag the branch does not carry changes nothing.
func (a *agent) UnassignBranchTag(ctx context.Context, organizationID string, documentID, branchID, tagID xid.ID) error {
	const q = `
		DELETE FROM document_branch_tags dbt
		USING document_branches b
		WHERE b.id = dbt.fk_branch_id
			AND dbt.fk_branch_id = $1
			AND b.fk_document_id = $2
			AND dbt.fk_tag_id = $3
			AND dbt.fk_organization_id = $4
	`

	_, err := a.sql.ExecContext(ctx, q, branchID, documentID, tagID, organizationID)

	return err
}

// CopyBranchTags makes the target branch carry every tag the source branch
// carries, on top of the ones it already has.
func (a *agent) CopyBranchTags(ctx context.Context, organizationID string, fromBranchID, toBranchID xid.ID) error {
	return a.copyBranchTags(ctx, a.sql, organizationID, fromBranchID, toBranchID)
}

// ReplaceBranchTags makes the target branch carry exactly the tags the
// source branch carries.
func (a *agent) ReplaceBranchTags(ctx context.Context, organizationID string, fromBranchID, toBranchID xid.ID) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Delete("document_branch_tags").
			Where(sq.Eq{
				"fk_branch_id":       toBranchID,
				"fk_organization_id": organizationID,
			}).
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		return a.copyBranchTags(ctx, tx, organizationID, fromBranchID, toBranchID)
	})
}

// copyBranchTags inserts the source branch's assignments under the target
// branch, skipping the ones the target already carries.
func (a *agent) copyBranchTags(
	ctx context.Context,
	ex sqlx.ExecerContext,
	organizationID string,
	fromBranchID, toBranchID xid.ID,
) error {
	const q = `
		INSERT INTO document_branch_tags (fk_branch_id, fk_tag_id, fk_organization_id)
		SELECT $1, fk_tag_id, fk_organization_id
		FROM document_branch_tags
		WHERE fk_branch_id = $2 AND fk_organization_id = $3
		ON CONFLICT DO NOTHING
	`

	_, err := ex.ExecContext(ctx, q, toBranchID, fromBranchID, organizationID)

	return err
}

// selectTagSummary prepares a select statement for tag summaries in display
// order, carrying the organization scope and the requesting user's own
// visibility setting.
func (a *agent) selectTagSummary(b sq.SelectBuilder, organizationID, userID string) sq.SelectBuilder {
	return b.Columns(
		`tags.id AS "id"`,
		`tags.tag_name AS "tag_name"`,
		`tags.color AS "color"`,
		`COALESCE(uts.hidden, FALSE) AS "hidden"`,
	).From("tags").
		LeftJoin("user_tag_settings uts ON uts.fk_tag_id = tags.id AND uts.fk_user_id = ?", userID).
		Where(sq.Eq{"tags.fk_organization_id": organizationID}).
		OrderBy("tags.sort_index", "tags.id")
}
