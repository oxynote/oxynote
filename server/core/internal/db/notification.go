package db

import (
	"context"
	"strconv"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/oxynote/heimdall/internal/notification"
	"github.com/oxynote/purse/http/httpserver"
	"github.com/oxynote/purse/util/sqlutil"
	"github.com/rs/xid"
)

// CreateNotification inserts a new notification into the database.
func (a *agent) CreateNotification(ctx context.Context, nt *notification.Notification) error {
	return sqlutil.WrapTx(ctx, a.sql, func(tx *sqlx.Tx) error {
		q, args := a.builder.Insert("notifications").
			SetMap(map[string]any{
				"id":                 nt.ID,
				"code":               nt.Code,
				"metadata":           nt.Metadata,
				"fk_user_id":         nt.UserID,
				"fk_organization_id": nt.OrganizationID,
				"read":               nt.Read,
				"created_at":         nt.CreatedAt,
			}).MustSql()

		_, err := tx.ExecContext(ctx, q, args...)

		b := a.builder.Select("id").
			From("notifications").
			Where(sq.Eq{
				"fk_user_id":         nt.UserID,
				"fk_organization_id": nt.OrganizationID,
			}).
			OrderBy("created_at DESC").
			Limit(a.opts.MaxNotifications).
			Prefix("id NOT IN (").
			Suffix(")")

		q, args = a.builder.Delete("notifications").Where(sq.And{
			b,
			sq.Eq{
				"fk_user_id":         nt.UserID,
				"fk_organization_id": nt.OrganizationID,
			},
		}).MustSql()

		_, err = tx.ExecContext(ctx, q, args...)

		return err
	})
}

// FetchManyNotifications retrieves all notifications for a user in an organization.
// Returns notifications and total count.
func (a *agent) FetchManyNotifications(ctx context.Context, organizationID, userID string, qr httpserver.Query) ([]*notification.Notification, uint64, error) {
	b, err := sqlutil.Select(
		qr,
		applyNotificationFilter,
		applyNotificationSort,
		sqlutil.PageCount,
	)
	if err != nil {
		return nil, 0, err
	}

	q, args := a.selectNotification(b).
		Where(sq.Eq{
			"notifications.fk_organization_id": organizationID,
			"notifications.fk_user_id":         userID,
		}).
		OrderBy("notifications.id ASC").
		MustSql()

	var data []struct {
		*notification.Notification

		PageCount uint64 `db:"page_count"`
	}

	if err := sqlx.SelectContext(ctx, a.sql, &data, q, args...); err != nil {
		return nil, 0, err
	}

	if len(data) == 0 {
		return []*notification.Notification{}, 0, nil
	}

	notifications := make([]*notification.Notification, len(data))
	for i := range data {
		notifications[i] = data[i].Notification
	}

	return notifications, data[0].PageCount, nil
}

// FetchNotificationCount retrieves the count of notifications for a user.
func (a *agent) FetchNotificationCount(ctx context.Context, organizationID, userID string, read bool) (uint64, error) {
	q, args := a.builder.Select("COUNT(*)").
		From("notifications").
		Where(sq.Eq{
			"notifications.fk_organization_id": organizationID,
			"notifications.fk_user_id":         userID,
			"notifications.read":               read,
		}).
		MustSql()

	var count uint64
	if err := sqlx.GetContext(ctx, a.sql, &count, q, args...); err != nil {
		return 0, err
	}

	return count, nil
}

// MarkReadByNotificationsIDs marks notifications as read by their IDs.
// If no IDs are provided, all notifications for the user are marked as read.
func (a *agent) MarkReadByNotificationsIDs(ctx context.Context, organizationID, userID string, ids []xid.ID) error {
	b := a.builder.Update("notifications").
		Set("read", true).
		Where(sq.Eq{
			"fk_organization_id": organizationID,
			"fk_user_id":         userID,
		})

	if len(ids) > 0 {
		b = b.Where(sq.Eq{"id": ids})
	}

	q, args := b.MustSql()

	_, err := a.sql.ExecContext(ctx, q, args...)

	return err
}

// selectNotification prepares a sql select statement for fetching notifications.
func (a *agent) selectNotification(b sq.SelectBuilder) sq.SelectBuilder {
	return b.Columns(
		`notifications.id AS "id"`,
		`notifications.code AS "code"`,
		`notifications.metadata AS "metadata"`,
		`notifications.fk_user_id AS "fk_user_id"`,
		`notifications.fk_organization_id AS "fk_organization_id"`,
		`notifications.read AS "read"`,
		`notifications.created_at AS "created_at"`,
	).From("notifications")
}

// applyNotificationFilter checks if filter key is valid and adds it to
// the query.
func applyNotificationFilter(b sq.SelectBuilder, k, v string) (sq.SelectBuilder, error) {
	switch k {
	case "":
		return b, nil
	case notification.FilterReadEq:
		read, err := strconv.ParseBool(v)
		if err != nil {
			return sq.SelectBuilder{}, httpserver.ErrInvalidFilterValue
		}

		return b.Where(sq.Eq{
			"read": read,
		}), nil
	}

	return sq.SelectBuilder{}, httpserver.ErrInvalidFilterKey
}

// applyNotificationSort checks if sort key is valid and adds it to the
// query.
func applyNotificationSort(b sq.SelectBuilder, k httpserver.SortKey) (sq.SelectBuilder, error) {
	switch k.Key {
	case "":
		k.Key = notification.SortCreatedAt
		k.Asc = false

		fallthrough
	case notification.SortCreatedAt:
		return b.OrderBy(sqlutil.SortString(httpserver.SortKey{
			Key: "notifications." + k.Key,
			Asc: k.Asc,
		})), nil
	}

	return sq.SelectBuilder{}, httpserver.ErrInvalidSortKey
}
