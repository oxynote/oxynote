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
			Code: "test.notification",
			Metadata: notification.Metadata{
				"index": strconv.Itoa(i),
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
			Code: "test.notification",
			Metadata: notification.Metadata{
				"key": "value",
			},
			ID:             xid.New(),
			UserID:         userID,
			OrganizationID: organizationID,
			CreatedAt:      timeutil.Now().Truncate(time.Second),
		}
	}

	type tcase struct {
		Agent            *agent
		CancelledContext bool
		Notification     *notification.Notification
		UnreadCount      uint64
		Result           []*notification.Notification
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				Notification:     stubNotification("org-id", "user-id"),
				Err:              assert.AnError,
			}
		},
		"Error returned by the insert": func(t *testing.T, db *DB) tcase {
			return tcase{
				Notification: prepNotifications(t, db, 1, nil)[0],
				Err:          assert.AnError,
			}
		},
		"Error returned by the retention delete": func(t *testing.T, _ *DB) tcase {
			a, mock := prepMockDB(t)
			a.opts.MaxNotifications = 1

			mock.ExpectBegin()
			mock.ExpectExec("INSERT INTO notifications").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM notifications").WillReturnError(assert.AnError)
			mock.ExpectRollback()

			return tcase{
				Agent:        a,
				Notification: stubNotification("org-id", "user-id"),
				Err:          assert.AnError,
			}
		},
		"Successful creation without a retention limit": func(t *testing.T, db *DB) tcase {
			existing := prepNotifications(t, db, 2, nil)
			nt := stubNotification(existing[0].OrganizationID, existing[0].UserID)
			nt.CreatedAt = existing[1].CreatedAt.Add(time.Hour)

			return tcase{
				Notification: nt,
				UnreadCount:  3,
				Result:       []*notification.Notification{nt, existing[1], existing[0]},
			}
		},
		"Successful creation with retention trimming": func(t *testing.T, db *DB) tcase {
			db.opts.MaxNotifications = 2

			existing := prepNotifications(t, db, 2, nil)
			nt := stubNotification(existing[0].OrganizationID, existing[0].UserID)
			nt.CreatedAt = existing[1].CreatedAt.Add(time.Hour)

			return tcase{
				Notification: nt,
				UnreadCount:  2,
				// the oldest notification is trimmed; the two newest remain.
				Result: []*notification.Notification{nt, existing[1]},
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

			ag := &db.agent

			if c.Agent != nil {
				ag = c.Agent
			}

			err := ag.CreateNotification(ctx, c.Notification)
			testutil.RequireEqualError(t, c.Err, err)

			if err != nil {
				return
			}

			count, err := db.FetchNotificationCount(context.Background(), c.Notification.OrganizationID, c.Notification.UserID, false)
			require.NoError(t, err)
			assert.Equal(t, c.UnreadCount, count)

			res, _, err := db.FetchManyNotifications(
				context.Background(),
				c.Notification.OrganizationID,
				c.Notification.UserID,
				httpserver.Query{Limit: 10, Page: 1},
			)
			require.NoError(t, err)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchManyNotifications(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		Query            httpserver.Query
		Result           []*notification.Notification
		Pages            uint64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Invalid query": func(*testing.T, *DB) tcase {
			return tcase{
				OrganizationID: "org-id",
				UserID:         "user-id",
				Err:            assert.AnError,
			}
		},
		"Invalid filter key": func(*testing.T, *DB) tcase {
			return tcase{
				OrganizationID: "org-id",
				UserID:         "user-id",
				Query: httpserver.Query{
					Limit: 10,
					Page:  1,
					Filters: map[string]string{
						"unknown": "true",
					},
				},
				Err: httpserver.ErrInvalidFilterKey,
			}
		},
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   "org-id",
				UserID:           "user-id",
				Query:            httpserver.Query{Limit: 10, Page: 1},
				Err:              assert.AnError,
			}
		},
		"No notifications": func(*testing.T, *DB) tcase {
			return tcase{
				OrganizationID: "org-id",
				UserID:         "user-id",
				Query:          httpserver.Query{Limit: 10, Page: 1},
				Result:         []*notification.Notification{},
			}
		},
		"First page newest first": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Query: httpserver.Query{
					Limit: 2,
					Page:  1,
				},
				Result: []*notification.Notification{notifications[2], notifications[1]},
				Pages:  2,
			}
		},
		"Second page": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Query: httpserver.Query{
					Limit: 2,
					Page:  2,
				},
				Result: []*notification.Notification{notifications[0]},
				Pages:  2,
			}
		},
		"Filtered by read state": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Query: httpserver.Query{
					Limit: 10,
					Page:  1,
					Filters: map[string]string{
						notification.FilterReadEq: "true",
					},
				},
				Result: []*notification.Notification{notifications[0]},
				Pages:  1,
			}
		},
		"Sorted by creation time ascending": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Query: httpserver.Query{
					Limit: 10,
					Page:  1,
					SortKeys: []httpserver.SortKey{
						{Key: notification.SortCreatedAt, Asc: true},
					},
				},
				Result: notifications,
				Pages:  1,
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

			res, pages, err := db.FetchManyNotifications(ctx, c.OrganizationID, c.UserID, c.Query)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Pages, pages)
			testutil.AssertFilterEqual(t, c.Result, res)
		})
	}
}

func Test_agent_FetchNotificationCount(t *testing.T) {
	type tcase struct {
		CancelledContext bool
		OrganizationID   string
		UserID           string
		Read             bool
		Result           uint64
		Err              error
	}

	cc := map[string]func(*testing.T, *DB) tcase{
		"Cancelled context": func(*testing.T, *DB) tcase {
			return tcase{
				CancelledContext: true,
				OrganizationID:   "org-id",
				UserID:           "user-id",
				Err:              assert.AnError,
			}
		},
		"Successful count of unread notifications": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Result:         2,
			}
		},
		"Successful count of read notifications": func(t *testing.T, db *DB) tcase {
			notifications := prepNotifications(t, db, 3, func(i int, nt *notification.Notification) {
				nt.Read = i == 0
			})

			return tcase{
				OrganizationID: notifications[0].OrganizationID,
				UserID:         notifications[0].UserID,
				Read:           true,
				Result:         1,
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

			count, err := db.FetchNotificationCount(ctx, c.OrganizationID, c.UserID, c.Read)
			testutil.AssertEqualError(t, c.Err, err)
			assert.Equal(t, c.Result, count)
		})
	}
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
