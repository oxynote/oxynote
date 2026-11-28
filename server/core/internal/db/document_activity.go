package db

import (
	"context"
	"maps"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/guregu/null/v5"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/internal/document/activity"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
)

// _contentEditWindow is how long a content edit entry keeps taking its
// editor's edits.
const _contentEditWindow = 30 * time.Minute

// InsertDocumentActivityEntries inserts new activity entries.
func (a *agent) InsertDocumentActivityEntries(ctx context.Context, entries ...activity.Entry) error {
	return a.insertDocumentActivityEntries(ctx, a.sql, entries...)
}

// RecordDocumentContentEdit grows each editor's open content edit entry
// on the branch, or opens one when the editor has none.
func (a *agent) RecordDocumentContentEdit(
	ctx context.Context,
	doc document.Document,
	editors []activity.Actor,
) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		// the branch row lock keeps two persists from both opening an
		// entry for one editor.
		q, args := a.builder.Select("id").
			From("document_branches").
			Where(sq.Eq{
				"id":                 doc.BranchID,
				"fk_organization_id": doc.OrganizationID,
			}).
			Suffix("FOR UPDATE").
			MustSql()

		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}

		now := timeutil.Now()

		for _, ed := range editors {
			open := a.builder.Select("id").
				From("document_activity_entries").
				Where(sq.Eq{
					"fk_branch_id":    doc.BranchID,
					"type":            activity.TypeContentEdited,
					"actor_kind":      ed.Kind,
					"fk_actor_id":     ed.UserID,
					"actor_client_id": ed.ClientID,
				}).
				Where(sq.Gt{"created_at": now.Add(-_contentEditWindow)}).
				OrderBy("created_at DESC", "id DESC").
				Limit(1).
				Prefix("id = (").
				Suffix(")")

			q, args = a.builder.Update("document_activity_entries").
				Set("updated_at", now).
				Where(open).
				MustSql()

			res, err := tx.ExecContext(ctx, q, args...)
			if err != nil {
				return err
			}

			n, err := res.RowsAffected()
			if err != nil {
				return err
			}

			if n > 0 {
				continue
			}

			if err = a.insertDocumentActivityEntries(ctx, tx, activity.NewContentEdited(ed, doc, now)); err != nil {
				return err
			}
		}

		return nil
	})
}

// insertDocumentActivityEntries inserts the entries through the given
// executor, all in one statement.
func (a *agent) insertDocumentActivityEntries(ctx context.Context, ex sqlx.ExecerContext, entries ...activity.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	b := a.builder.Insert("document_activity_entries").Columns(
		"id",
		"fk_organization_id",
		"fk_document_id",
		"fk_branch_id",
		"actor_kind",
		"fk_actor_id",
		"actor_client_id",
		"category",
		"type",
		"target_kind",
		"target_id",
		"metadata",
		"created_at",
		"updated_at",
	)

	for _, e := range entries {
		b = b.Values(
			e.ID,
			e.OrganizationID,
			e.DocumentID,
			e.BranchID,
			e.Actor.Kind,
			e.Actor.UserID,
			e.Actor.ClientID,
			e.Category,
			e.Type,
			e.Target.Kind,
			e.Target.ID,
			e.Metadata,
			e.CreatedAt,
			e.UpdatedAt,
		)
	}

	q, args := b.MustSql()

	_, err := ex.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentActivityEntries retrieves a page of the branch's entries,
// newest first, with what each entry's target points at.
func (a *agent) FetchDocumentActivityEntries(
	ctx context.Context,
	branchID xid.ID,
	organizationID string,
	qr activity.Query,
) (activity.Page, error) {
	size := qr.PageSize()

	// one entry past the page tells whether a next page exists.
	b := a.selectDocumentActivityEntry(a.builder.Select(), organizationID).
		Where(sq.Eq{"document_activity_entries.fk_branch_id": branchID}).
		OrderBy("document_activity_entries.created_at DESC", "document_activity_entries.id DESC").
		Limit(size + 1)

	if len(qr.Categories) > 0 {
		b = b.Where(sq.Eq{"document_activity_entries.category": qr.Categories})
	}

	// a cursor from another branch matches no row, so the page is empty.
	if !qr.Before.IsNil() {
		b = b.Where(
			`(document_activity_entries.created_at, document_activity_entries.id) <
			(SELECT created_at, id FROM document_activity_entries WHERE id = ? AND fk_branch_id = ?)`,
			qr.Before,
			branchID,
		)
	}

	q, args := b.MustSql()

	page := activity.Page{Entries: []activity.Entry{}}

	if err := sqlx.SelectContext(ctx, a.sql, &page.Entries, q, args...); err != nil {
		return activity.Page{}, err
	}

	if uint64(len(page.Entries)) > size {
		page.Entries = page.Entries[:size]
		page.Next = null.ValueFrom(page.Entries[size-1].ID)
	}

	if err := a.attachDocumentActivitySubjects(ctx, page.Entries, organizationID); err != nil {
		// NOCOV: a test cannot make the subject reads fail without
		// failing the entry read first.
		return activity.Page{}, err
	}

	return page, nil
}

// attachDocumentActivitySubjects sets what each entry's target points
// at.
func (a *agent) attachDocumentActivitySubjects(ctx context.Context, entries []activity.Entry, organizationID string) error {
	ids := make(map[activity.TargetKind][]string)

	for _, e := range entries {
		if e.Target.Kind.Valid {
			ids[e.Target.Kind.V] = append(ids[e.Target.Kind.V], e.Target.ID.String)
		}
	}

	subjects := make(map[activity.Target]any)

	for kind, kindIDs := range ids {
		found, err := a.fetchDocumentActivitySubjects(ctx, kind, kindIDs, organizationID)
		if err != nil {
			return err
		}

		maps.Copy(subjects, found)
	}

	for i := range entries {
		entries[i].Subject = subjects[entries[i].Target]
	}

	return nil
}

// fetchDocumentActivitySubjects loads what the targets of one kind point
// at, keyed by target. Only comment and reply revisions load a subject.
func (a *agent) fetchDocumentActivitySubjects(
	ctx context.Context,
	kind activity.TargetKind,
	ids []string,
	organizationID string,
) (map[activity.Target]any, error) {
	res := make(map[activity.Target]any, len(ids))

	switch kind {
	case activity.TargetKindCommentRevision:
		revs, err := a.fetchDocumentCommentRevisionsByIDs(ctx, ids, organizationID)
		if err != nil {
			return nil, err
		}

		for _, rev := range revs {
			res[activity.NewTarget(kind, rev.ID.String())] = rev
		}
	case activity.TargetKindReplyRevision:
		revs, err := a.fetchDocumentCommentReplyRevisionsByIDs(ctx, ids, organizationID)
		if err != nil {
			return nil, err
		}

		for _, rev := range revs {
			res[activity.NewTarget(kind, rev.ID.String())] = rev
		}
	case activity.TargetKindDocument,
		activity.TargetKindBranch,
		activity.TargetKindUser,
		activity.TargetKindHook,
		activity.TargetKindTag:
		// the client already lists these, so the entry carries only the
		// id.
	}

	return res, nil
}

// selectDocumentActivityEntry prepares a sql select statement for
// fetching the organization's activity entries.
func (a *agent) selectDocumentActivityEntry(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		`document_activity_entries.id AS "id"`,
		`document_activity_entries.fk_organization_id AS "fk_organization_id"`,
		`document_activity_entries.fk_document_id AS "fk_document_id"`,
		`document_activity_entries.fk_branch_id AS "fk_branch_id"`,
		`document_activity_entries.actor_kind AS "actor.actor_kind"`,
		`document_activity_entries.fk_actor_id AS "actor.fk_actor_id"`,
		`document_activity_entries.actor_client_id AS "actor.actor_client_id"`,
		`document_activity_entries.category AS "category"`,
		`document_activity_entries.type AS "type"`,
		`document_activity_entries.target_kind AS "target.target_kind"`,
		`document_activity_entries.target_id AS "target.target_id"`,
		`document_activity_entries.metadata AS "metadata"`,
		`document_activity_entries.created_at AS "created_at"`,
		`document_activity_entries.updated_at AS "updated_at"`,
	).
		From("document_activity_entries").
		Where(sq.Eq{"document_activity_entries.fk_organization_id": organizationID})
}
