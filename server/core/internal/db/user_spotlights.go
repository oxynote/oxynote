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
