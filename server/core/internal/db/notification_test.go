package db

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	"github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/sqlutil"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/oxynote/oxynote/server/core/pkg/timeutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func prepNotifications(t *testing.T, db *DB, count int, fn func(int, *notification.Notification)) []*notification.Notification {
	t.Helper()

	res := make([]*notification.Notification, count)

	now := timeutil.Now().Truncate(time.Second)

	// notifications missing an owner after fn ran share one lazily
	// created organization member.
	var (
		organizationID string
		userID         string
	)

	for i := range count {
		nt := &notification.Notification{
			Core: notification.Core{
				Code: "test.notification",
				Metadata: notification.Metadata{
					"index": strconv.Itoa(i),
				},
			},
			ID: xid.New(),
			// distinct ascending timestamps keep the fetch order
			// deterministic.
			CreatedAt: now.Add(time.Duration(i) * time.Second),
		}

		if fn != nil {
			fn(i, nt)
		}

		if nt.UserID == "" {
			if userID == "" {
				organizationID = prepOrganizations(t, db, 1)[0]
				userID = prepUsers(t, db, 1)[0]
			}

			nt.UserID = userID
			nt.OrganizationID = organizationID
		}

		res[i] = nt

		q, args := db.builder.Insert("notifications").
			SetMap(map[string]any{
				"id":                 nt.ID,
				"code":               nt.Code,
				"metadata":           nt.Metadata,
				"fk_user_id":         nt.UserID,
				"fk_organization_id": nt.OrganizationID,
				"read":               nt.Read,
				"created_at":         nt.CreatedAt,
			}).MustSql()

		_, err := db.sql.Exec(q, args...)
		require.NoError(t, err)
	}

	return res
}

func Test_agent_CreateNotification(t *testing.T) {
	stubNotification := func(organizationID, userID string) *notification.Notification {
		return &notification.Notification{
			Core: notification.Core{
				Code: "test.notification",
				Metadata: notification.Metadata{
					"key": "value",
				},
			},
			ID:             xid.New(),
			UserID:         userID,
			OrganizationID: organizationID,
			CreatedAt:      timeutil.Now().Truncate(time.Second),
		}
	}

	t.Run("Cancelled context", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := db.CreateNotification(ctx, stubNotification("org-id", "user-id"))
		require.Error(t, err)
	})

	t.Run("Error returned by the insert", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		nt := prepNotifications(t, db, 1, nil)[0]

		err := db.CreateNotification(context.Background(), nt)
		require.Error(t, err)
	})

	t.Run("Error returned by the retention delete", func(t *testing.T) {
		t.Parallel()

		a, mock := prepMockDB(t)
		a.opts.MaxNotifications = 1

		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO notifications").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("DELETE FROM notifications").WillReturnError(assert.AnError)
		mock.ExpectRollback()

		err := a.CreateNotification(context.Background(), stubNotification("org-id", "user-id"))
		assert.Equal(t, assert.AnError, err)
	})

	t.Run("Successful creation without a retention limit", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)

		existing := prepNotifications(t, db, 2, nil)
		nt := stubNotification(existing[0].OrganizationID, existing[0].UserID)

		err := db.CreateNotification(context.Background(), nt)
		require.NoError(t, err)

		count, err := db.FetchNotificationCount(context.Background(), nt.OrganizationID, nt.UserID, false)
		require.NoError(t, err)
		assert.Equal(t, uint64(3), count)
	})

	t.Run("Successful creation with retention trimming", func(t *testing.T) {
		t.Parallel()

		db := prepTempDB(t)
		db.opts.MaxNotifications = 2

		existing := prepNotifications(t, db, 2, nil)
		nt := stubNotification(existing[0].OrganizationID, existing[0].UserID)
		nt.CreatedAt = existing[1].CreatedAt.Add(time.Hour)

		err := db.CreateNotification(context.Background(), nt)
		require.NoError(t, err)

		// the oldest notification is trimmed; the two newest remain.
		res, _, err := db.FetchManyNotifications(
			context.Background(),
			nt.OrganizationID,
			nt.UserID,
			httpserver.Query{Limit: 10, Page: 1},
		)
		require.NoError(t, err)
		testutil.AssertFilterEqual(t, []*notification.Notification{nt, existing[1]}, res)
	})
}

func Test_agent_FetchManyNotifications(t *testing.T) {
	db := prepTempDB(t)

	// error - invalid query
	res, pages, err := db.FetchManyNotifications(context.Background(), "org-id", "user-id", httpserver.Query{})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Zero(t, pages)

	// error - invalid filter key
	res, pages, err = db.FetchManyNotifications(context.Background(), "org-id", "user-id", httpserver.Query{
		Limit: 10,
		Page:  1,
		Filters: map[string]string{
			"unknown": "true",
		},
	})
	assert.Equal(t, httpserver.ErrInvalidFilterKey, err)
	assert.Nil(t, res)
	assert.Zero(t, pages)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res, pages, err = db.FetchManyNotifications(ctx, "org-id", "user-id", httpserver.Query{Limit: 10, Page: 1})
	require.Error(t, err)
	assert.Nil(t, res)
	assert.Zero(t, pages)

	// success - no notifications
	res, pages, err = db.FetchManyNotifications(context.Background(), "org-id", "user-id", httpserver.Query{Limit: 10, Page: 1})
	require.NoError(t, err)
	assert.Empty(t, res)
	assert.Zero(t, pages)

	notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
		nt.Read = i == 0
	})

	organizationID := notifications[0].OrganizationID
	userID := notifications[0].UserID

	// success - first page, newest first
	res, pages, err = db.FetchManyNotifications(context.Background(), organizationID, userID, httpserver.Query{
		Limit: 2,
		Page:  1,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(2), pages)
	testutil.AssertFilterEqual(t, []*notification.Notification{notifications[2], notifications[1]}, res)

	// success - second page
	res, pages, err = db.FetchManyNotifications(context.Background(), organizationID, userID, httpserver.Query{
		Limit: 2,
		Page:  2,
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(2), pages)
	testutil.AssertFilterEqual(t, []*notification.Notification{notifications[0]}, res)

	// success - filtered by read state
	res, pages, err = db.FetchManyNotifications(context.Background(), organizationID, userID, httpserver.Query{
		Limit: 10,
		Page:  1,
		Filters: map[string]string{
			notification.FilterReadEq: "true",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), pages)
	testutil.AssertFilterEqual(t, []*notification.Notification{notifications[0]}, res)

	// success - sorted by creation time ascending
	res, pages, err = db.FetchManyNotifications(context.Background(), organizationID, userID, httpserver.Query{
		Limit: 10,
		Page:  1,
		SortKeys: []httpserver.SortKey{
			{Key: notification.SortCreatedAt, Asc: true},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), pages)
	testutil.AssertFilterEqual(t, notifications, res)
}

func Test_agent_FetchNotificationCount(t *testing.T) {
	db := prepTempDB(t)

	// error - cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	count, err := db.FetchNotificationCount(ctx, "org-id", "user-id", false)
	require.Error(t, err)
	assert.Zero(t, count)

	// success
	notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
		nt.Read = i == 0
	})

	count, err = db.FetchNotificationCount(context.Background(), notifications[0].OrganizationID, notifications[0].UserID, false)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), count)

	count, err = db.FetchNotificationCount(context.Background(), notifications[0].OrganizationID, notifications[0].UserID, true)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), count)
}

func Test_agent_MarkReadByNotificationsIDs(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		IDs              []xid.ID
		UnreadCount      uint64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(t *testing.T, db *DB) tcase {
			nt := prepNotifications(t, db, 1, nil)[0]

			return tcase{
				CancelledContext: true,
				OrganizationID:   nt.OrganizationID,
				UserID:           nt.UserID,
				Err:              assert.AnError,
			}
		},
		"Successful update of specific notifications": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, nil)

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				IDs:            []xid.ID{notifications[0].ID, notifications[1].ID},
				UnreadCount:    1,
			}
		},
		"Successful update of all notifications": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, nil)

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
			}
		},
	}

	for cn, cfn := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			db := prepTempDB(t)
			c := cfn(t, db)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if c.CancelledContext {
				cancel()
			}

			err := db.MarkReadByNotificationsIDs(ctx, c.OrganizationID, c.UserID, c.IDs)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			count, err := db.FetchNotificationCount(context.Background(), c.OrganizationID, c.UserID, false)
			require.NoError(t, err)
			assert.Equal(t, c.UnreadCount, count)
		})
	}
}

func Test_applyNotificationFilter(t *testing.T) {
	cc := map[string]struct {
		Key    string
		Value  string
		Result sq.SelectBuilder
		Err    error
	}{
		"Invalid key": {
			Key: "unknown",
			Err: httpserver.ErrInvalidFilterKey,
		},
		"Invalid read value": {
			Key:   notification.FilterReadEq,
			Value: "not-a-bool",
			Err:   httpserver.ErrInvalidFilterValue,
		},
		"Successful execution with empty key": {
			Result: sq.Select(),
		},
		"Successful execution with read key": {
			Key:   notification.FilterReadEq,
			Value: "true",
			Result: sq.Select().Where(sq.Eq{
				"read": true,
			}),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			b, err := applyNotificationFilter(sq.Select(), c.Key, c.Value)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b)
		})
	}
}

func Test_applyNotificationSort(t *testing.T) {
	cc := map[string]struct {
		Key    httpserver.SortKey
		Result sq.SelectBuilder
		Err    error
	}{
		"Invalid key": {
			Key: httpserver.SortKey{
				Key: "unknown",
				Asc: true,
			},
			Err: httpserver.ErrInvalidSortKey,
		},
		"Successful execution with empty key": {
			Key: httpserver.SortKey{
				Asc: true,
			},
			Result: sq.Select().OrderBy(sqlutil.SortString(
				httpserver.SortKey{
					Key: "notifications." + notification.SortCreatedAt,
					Asc: false,
				},
			)),
		},
		"Successful execution with created_at key": {
			Key: httpserver.SortKey{
				Key: notification.SortCreatedAt,
				Asc: true,
			},
			Result: sq.Select().OrderBy(sqlutil.SortString(
				httpserver.SortKey{
					Key: "notifications." + notification.SortCreatedAt,
					Asc: true,
				},
			)),
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			b, err := applyNotificationSort(sq.Select(), c.Key)
			testutil.AssertEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			assert.Equal(t, c.Result, b)
		})
	}
}
