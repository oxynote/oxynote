package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

// UpdateUserImage updates the image of a user.
func (a *agent) UpdateUserImage(
	ctx context.Context,
	userID string,
	image string,
) error {
	q, args := a.builder.Update("users").
		SetMap(map[string]any{
			"image": image,
		}).
		Where(sq.Eq{
			"id": userID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// FetchUserName fetches the name of a user by their ID.
func (a *agent) FetchUserName(
	ctx context.Context,
	userID string,
) (string, error) {
	var name string

	q, args := a.builder.Select("name").
		From("users").
		Where(sq.Eq{
			"id": userID,
		}).MustSql()

	err := sqlx.GetContext(ctx, a.sql, &name, q, args...)
	if err != nil {
		return "", err
	}

	return name, nil
}
