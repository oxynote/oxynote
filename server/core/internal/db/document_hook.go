package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document/hook"
	"github.com/rs/xid"
)

// InsertDocumentHook inserts a new document hook into the database.
func (a *agent) InsertDocumentHook(ctx context.Context, hk hook.Hook) error {
	q, args := a.builder.Insert("document_hooks").
		SetMap(map[string]any{
			"id":                 hk.ID,
			"type":               hk.Type,
			"fk_document_id":     hk.DocumentID,
			"fk_organization_id": hk.OrganizationID,
			"fk_branch_id":       hk.BranchID,
			"block_id":           hk.BlockID,
			"settings":           hk.Settings,
			"state":              hk.State,
			"score":              hk.Score,
			"created_at":         hk.CreatedAt,
			"updated_at":         hk.UpdatedAt,
			"soft_deleted_at":    hk.SoftDeletedAt,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentHook retrieves a document hook by its ID from the database.
func (a *agent) FetchDocumentHook(ctx context.Context, id xid.ID, organizationID string) (*hook.Hook, error) {
	q, args := a.selectDocumentHook(a.builder.Select()).
		Where(sq.Eq{
			"document_hooks.id":                 id,
			"document_hooks.fk_organization_id": organizationID,
		}).
		Limit(1).
		MustSql()

	hk := &hook.Hook{}
	if err := sqlx.GetContext(ctx, a.sql, hk, q, args...); err != nil {
		return nil, err
	}

	return hk, nil
}

// FetchDocumentHooksByDocumentID retrieves all document hooks associated
// with a specific document ID from the database.
func (a *agent) FetchDocumentHooksByDocumentID(ctx context.Context, documentID xid.ID, organizationID string) ([]hook.Hook, error) {
	q, args := a.selectDocumentHook(a.builder.Select()).
		Where(sq.Eq{
			"document_hooks.fk_document_id":     documentID,
			"document_hooks.fk_organization_id": organizationID,
		}).
		MustSql()

	hooks := []hook.Hook{}
	if err := sqlx.SelectContext(ctx, a.sql, &hooks, q, args...); err != nil {
		return nil, err
	}

	return hooks, nil
}

// FetchDocumentHooksByOrganizationID retrieves every hook of an organization.
func (a *agent) FetchDocumentHooksByOrganizationID(ctx context.Context, organizationID string) ([]hook.Hook, error) {
	q, args := a.selectDocumentHook(a.builder.Select()).
		Where(sq.Eq{
			"document_hooks.fk_organization_id": organizationID,
		}).
		MustSql()

	hooks := []hook.Hook{}
	if err := sqlx.SelectContext(ctx, a.sql, &hooks, q, args...); err != nil {
		return nil, err
	}

	return hooks, nil
}

// FetchPaginatedDocumentHooks retrieves a list of document hooks
// from the database with pagination support.
func (a *agent) FetchPaginatedDocumentHooks(ctx context.Context, offsetID xid.ID, limit int64) ([]hook.Hook, error) {
	b := a.selectDocumentHook(a.builder.Select()).
		OrderBy("document_hooks.id ASC").
		Limit(uint64(limit))

	if !offsetID.IsNil() {
		b = b.Where(sq.Gt{"document_hooks.id": offsetID})
	}

	q, args := b.MustSql()

	hooks := []hook.Hook{}
	if err := sqlx.SelectContext(ctx, a.sql, &hooks, q, args...); err != nil {
		return nil, err
	}

	return hooks, nil
}

// UpdateDocumentHook updates an existing document hook in the database.
func (a *agent) UpdateDocumentHook(ctx context.Context, hk hook.Hook) error {
	q, args := a.builder.Update("document_hooks").
		SetMap(map[string]any{
			"settings":        hk.Settings,
			"state":           hk.State,
			"score":           hk.Score,
			"updated_at":      hk.UpdatedAt,
			"soft_deleted_at": hk.SoftDeletedAt,
		}).
		Where(sq.Eq{
			"id":                 hk.ID,
			"fk_organization_id": hk.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}

	return nil
}

// DeleteDocumentHook deletes a document hook from the database.
func (a *agent) DeleteDocumentHook(ctx context.Context, id xid.ID) error {
	q, args := a.builder.Delete("document_hooks").
		Where(sq.Eq{
			"id": id,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchDocumentHooksByBranchID retrieves all document hooks for a specific branch.
func (a *agent) FetchDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) ([]hook.Hook, error) {
	q, args := a.selectDocumentHook(a.builder.Select()).
		Where(sq.Eq{
			"document_hooks.fk_branch_id":       branchID,
			"document_hooks.fk_organization_id": organizationID,
		}).
		Where(sq.Eq{"soft_deleted_at": nil}).
		MustSql()

	hooks := []hook.Hook{}

	if err := sqlx.SelectContext(ctx, a.sql, &hooks, q, args...); err != nil {
		return nil, err
	}

	return hooks, nil
}

// DetachDocumentHooksByBranchID cuts every hook of a branch loose from the
// branch and its document. The hook manager job tears down the external
// resources and deletes the rows, as it does for a deleted branch. A
// soft-delete mark is not used because the job lifts it whenever the
// hook's block is still in the branch, which is the case when the hooks
// are being replaced rather than their blocks removed.
func (a *agent) DetachDocumentHooksByBranchID(ctx context.Context, branchID xid.ID, organizationID string) error {
	q, args := a.builder.Update("document_hooks").
		SetMap(map[string]any{
			"fk_branch_id":   nil,
			"fk_document_id": nil,
		}).
		Where(sq.Eq{
			"fk_branch_id":       branchID,
			"fk_organization_id": organizationID,
		}).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// selectDocumentHook prepares a sql select statement for fetching document hooks.
func (a *agent) selectDocumentHook(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`document_hooks.id AS "id"`,
		`document_hooks.type AS "type"`,
		`document_hooks.fk_document_id AS "fk_document_id"`,
		`document_hooks.fk_organization_id AS "fk_organization_id"`,
		`document_hooks.fk_branch_id AS "fk_branch_id"`,
		`document_hooks.block_id AS "block_id"`,
		`document_hooks.settings AS "settings"`,
		`document_hooks.state AS "state"`,
		`document_hooks.score AS "score"`,
		`document_hooks.created_at AS "created_at"`,
		`document_hooks.updated_at AS "updated_at"`,
		`document_hooks.soft_deleted_at AS "soft_deleted_at"`,
	).From("document_hooks")
}
