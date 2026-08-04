package db

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/oxynote/server/core/pkg/errutil"
)

// FetchOrganizationMembers retrieves all member user IDs of an organization.
func (a *agent) FetchOrganizationMembers(ctx context.Context, organizationID string) ([]string, error) {
	q, args := a.builder.Select("fk_user_id").
		From("organization_members").
		Where(sq.Eq{
			"fk_organization_id": organizationID,
		}).
		OrderBy("created_at ASC").
		MustSql()

	var userIDs []string

	if err := sqlx.SelectContext(ctx, a.sql, &userIDs, q, args...); err != nil {
		return nil, err
	}

	return userIDs, nil
}

// CheckOrganizationMember checks if a user is a member of an organization.
func (a *agent) CheckOrganizationMember(ctx context.Context, organizationID, userID string) (bool, error) {
	q, args := a.builder.Select("1").
		From("organization_members").
		Where(sq.Eq{
			"fk_organization_id": organizationID,
			"fk_user_id":         userID,
		}).
		Limit(1).
		MustSql()

	var exists int

	if err := sqlx.GetContext(ctx, a.sql, &exists, q, args...); err != nil {
		if errutil.IsNotFound(err) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// FetchOrganizationSlug retrieves the slug of an organization by its ID.
func (a *agent) FetchOrganizationSlug(ctx context.Context, organizationID string) (string, error) {
	q, args := a.builder.Select("slug").
		From("organizations").
		Where(sq.Eq{
			"id": organizationID,
		}).
		Limit(1).
		MustSql()

	var slug string

	if err := sqlx.GetContext(ctx, a.sql, &slug, q, args...); err != nil {
		return "", err
	}

	return slug, nil
}

// UpdateOrganizationLogo updates the logo of an organization.
func (a *agent) UpdateOrganizationLogo(
	ctx context.Context,
	organizationID string,
	logo string,
) error {
	q, args := a.builder.Update("organizations").
		SetMap(map[string]any{
			"logo": logo,
		}).
		Where(sq.Eq{
			"id": organizationID,
		}).MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}
