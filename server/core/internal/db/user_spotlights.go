package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/internal/user"
)

// UpsertUserSpotlights stores the state of a user's spotlights. A dismissed
// spotlight stays dismissed.
func (a *agent) UpsertUserSpotlights(
	ctx context.Context,
	userID string,
	inp user.SpotlightsInput,
) error {
	q, args := a.builder.Insert("user_spotlights").
		SetMap(map[string]any{
			"fk_user_id": userID,
			"star":       inp.Star,
		}).
		Suffix(
			"ON CONFLICT (fk_user_id) DO UPDATE SET star = EXCLUDED.star WHERE user_spotlights.star IS DISTINCT FROM ?",
			user.SpotlightStateDismissed,
		).
		MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchUserSpotlights fetches the state of a user's spotlights.
func (a *agent) FetchUserSpotlights(ctx context.Context, userID string) (user.Spotlights, error) {
	q, args := a.selectUserSpotlights(a.builder.Select(), userID).MustSql()

	var spotlights user.Spotlights

	if err := sqlx.GetContext(ctx, a.sql, &spotlights, q, args...); err != nil {
		return user.Spotlights{}, err
	}

	return spotlights, nil
}

// FetchUserSpotlightActivity fetches what a user has done so far, across
// every organization they belong to.
func (a *agent) FetchUserSpotlightActivity(ctx context.Context, userID string) (user.SpotlightActivity, error) {
	q, args := a.selectUserSpotlightActivity(a.builder.Select(), userID).MustSql()

	var activity user.SpotlightActivity

	if err := sqlx.GetContext(ctx, a.sql, &activity, q, args...); err != nil {
		return user.SpotlightActivity{}, err
	}

	return activity, nil
}

// selectUserSpotlights prepares a SQL select statement for fetching the state
// of a user's spotlights. It starts from the user, so a user who has no row
// yet still yields one, with every state null.
func (a *agent) selectUserSpotlights(b sq.SelectBuilder, userID string) sq.SelectBuilder {
	return b.Columns(
		`user_spotlights.star AS "star"`,
	).From("users").
		LeftJoin("user_spotlights ON user_spotlights.fk_user_id = users.id").
		Where(sq.Eq{
			"users.id": userID,
		})
}

// selectUserSpotlightActivity prepares a SQL select statement for fetching
// what a user has done so far. Each count is a subquery of its own, so one
// table's rows never multiply another's.
func (a *agent) selectUserSpotlightActivity(b sq.SelectBuilder, userID string) sq.SelectBuilder {
	return b.Columns(
		`users.created_at AS "account_created_at"`,
	).
		Column(sq.Expr(
			`(SELECT MAX(viewed_at) FROM document_branch_views WHERE fk_user_id = ?) AS "last_viewed_at"`,
			userID,
		)).
		Column(sq.Expr(
			`(SELECT COUNT(*) FROM documents WHERE fk_created_by = ?) AS "created_documents"`,
			userID,
		)).
		Column(sq.Expr(
			`(
				SELECT COUNT(DISTINCT fk_document_id)
				FROM document_branch_history_entries
				WHERE fk_last_updated_by = ?
			) AS "edited_documents"`,
			userID,
		)).
		Column(sq.Expr(
			`(
				SELECT COUNT(*)
				FROM organization_invitations
				WHERE fk_inviter_id = ? AND status = 'accepted'
			) AS "accepted_invitations"`,
			userID,
		)).
		Column(sq.Expr(
			`(
				SELECT COUNT(*)
				FROM branch_reviewers
				JOIN document_branches ON document_branches.id = branch_reviewers.fk_branch_id
				WHERE (branch_reviewers.currently_approved OR branch_reviewers.previously_approved)
					AND (branch_reviewers.fk_user_id = ? OR document_branches.fk_created_by = ?)
			) AS "approvals"`,
			userID,
			userID,
		)).
		Column(sq.Expr(
			`(
				(SELECT COUNT(*) FROM document_comments WHERE fk_user_id = ?) +
				(SELECT COUNT(*) FROM document_comment_replies WHERE fk_user_id = ?)
			) AS "comments"`,
			userID,
			userID,
		)).
		Column(sq.Expr(
			`(SELECT COUNT(*) FROM oauth_consents WHERE fk_user_id = ?) AS "mcp_clients"`,
			userID,
		)).
		From("users").
		Where(sq.Eq{
			"users.id": userID,
		})
}
