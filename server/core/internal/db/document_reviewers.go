package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/document"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/rs/xid"
)

// FetchBranchReviewers retrieves all reviewers for a specific branch.
func (a *agent) FetchBranchReviewers(ctx context.Context, branchID xid.ID, organizationID string) ([]document.BranchReviewer, error) {
	q, args := a.selectBranchReviewer(a.builder.Select(), organizationID).
		Where(sq.Eq{
			"fk_branch_id": branchID,
		}).MustSql()

	reviewers := []document.BranchReviewer{}

	if err := sqlx.SelectContext(ctx, a.sql, &reviewers, q, args...); err != nil {
		return nil, err
	}

	return reviewers, nil
}

// FetchBranchReviewer retrieves a single reviewer for a branch by user ID.
func (a *agent) FetchBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) (*document.BranchReviewer, error) {
	q, args := a.selectBranchReviewer(a.builder.Select(), organizationID).
		Where(sq.Eq{
			"fk_branch_id": branchID,
			"fk_user_id":   userID,
		}).
		Limit(1).
		MustSql()

	var reviewer document.BranchReviewer

	if err := sqlx.GetContext(ctx, a.sql, &reviewer, q, args...); err != nil {
		return nil, err
	}

	return &reviewer, nil
}

// InsertBranchReviewer inserts a new reviewer for a branch. On conflict, does nothing.
func (a *agent) InsertBranchReviewer(ctx context.Context, reviewer document.BranchReviewer) error {
	q, args := a.builder.Insert("branch_reviewers").
		SetMap(map[string]any{
			"fk_branch_id":        reviewer.BranchID,
			"fk_user_id":          reviewer.UserID,
			"fk_organization_id":  reviewer.OrganizationID,
			"currently_approved":  reviewer.CurrentlyApproved,
			"previously_approved": reviewer.PreviouslyApproved,
		}).
		Suffix(`ON CONFLICT (fk_branch_id, fk_user_id) DO NOTHING`).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// UpdateBranchReviewer updates the approval state of an existing branch reviewer.
func (a *agent) UpdateBranchReviewer(ctx context.Context, reviewer document.BranchReviewer) error {
	q, args := a.builder.Update("branch_reviewers").
		SetMap(map[string]any{
			"currently_approved": reviewer.CurrentlyApproved,
		}).
		Where(sq.Eq{
			"fk_branch_id":       reviewer.BranchID,
			"fk_user_id":         reviewer.UserID,
			"fk_organization_id": reviewer.OrganizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// DeleteBranchReviewer removes a reviewer from a branch.
func (a *agent) DeleteBranchReviewer(ctx context.Context, branchID xid.ID, userID, organizationID string) error {
	q, args := a.builder.Delete("branch_reviewers").
		Where(sq.Eq{
			"fk_branch_id":       branchID,
			"fk_user_id":         userID,
			"fk_organization_id": organizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// PromoteBranchApprovals promotes reviewer approvals from one branch to another.
// It copies reviewers from fromBranchID into toBranchID, moving current approvals
// to previously_approved and resetting currently_approved. The fromBranch reviewers
// are cleared afterwards.
func (a *agent) PromoteBranchApprovals(ctx context.Context, fromBranchID, toBranchID xid.ID, organizationID string) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		// Delete reviewers on the target branch who have never approved on either pass.
		q1, args1 := a.builder.Delete("branch_reviewers").
			Where(sq.Eq{
				"fk_branch_id":        toBranchID,
				"fk_organization_id":  organizationID,
				"previously_approved": false,
				"currently_approved":  false,
			}).MustSql()

		if _, err := tx.ExecContext(ctx, q1, args1...); err != nil {
			return err
		}

		// Fold currently_approved into previously_approved on the target
		// branch. Assigning instead of folding would clear the history q1
		// just kept these rows alive for, and the next promotion's q1 would
		// then delete them.
		q2, args2 := a.builder.Update("branch_reviewers").
			SetMap(map[string]any{
				"previously_approved": sq.Expr("previously_approved OR currently_approved"),
				"currently_approved":  false,
			}).
			Where(sq.Eq{
				"fk_branch_id":       toBranchID,
				"fk_organization_id": organizationID,
			}).MustSql()

		if _, err := tx.ExecContext(ctx, q2, args2...); err != nil {
			return err
		}

		// Upsert source-branch reviewers into the target branch.
		const q3 = `
			INSERT INTO branch_reviewers (fk_branch_id, fk_user_id, fk_organization_id, previously_approved, currently_approved)
			SELECT $1, fk_user_id, fk_organization_id, false, currently_approved
			FROM branch_reviewers
			WHERE fk_branch_id = $2 AND fk_organization_id = $3
			ON CONFLICT (fk_branch_id, fk_user_id) DO UPDATE SET currently_approved = EXCLUDED.currently_approved
		`

		if _, err := tx.ExecContext(ctx, q3, toBranchID, fromBranchID, organizationID); err != nil {
			return err
		}

		// Clear all reviewers from the source branch.
		q4, args4 := a.builder.Delete("branch_reviewers").
			Where(sq.Eq{
				"fk_branch_id":       fromBranchID,
				"fk_organization_id": organizationID,
			}).MustSql()

		_, err := tx.ExecContext(ctx, q4, args4...)

		return err
	})
}

// selectBranchReviewer prepares a select statement for branch reviewers,
// carrying the organization scope so that no query can forget it.
func (a *agent) selectBranchReviewer(b sq.SelectBuilder, organizationID string) sq.SelectBuilder {
	return b.Columns(
		"fk_branch_id",
		"fk_user_id",
		"fk_organization_id",
		"currently_approved",
		"previously_approved",
	).From("branch_reviewers").
		Where(sq.Eq{"fk_organization_id": organizationID})
}
