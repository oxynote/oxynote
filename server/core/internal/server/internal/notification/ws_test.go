package notification

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	notificationCore "github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	wsMock "github.com/oxynote/wetsocks/wsserver/_mock"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReceiver captures the notification callback and counts
// unsubscriptions.
type fakeReceiver struct {
	fn      func(context.Context, notificationCore.Notification)
	unsubbd int
}

func (r *fakeReceiver) OnNotification(fn func(context.Context, notificationCore.Notification)) notificationCore.Unsubscribe {
	r.fn = fn

	return func() { r.unsubbd++ }
}

func Test_Handler_BindNotifications(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB    *DBMock
		Count uint64
	}{
		"Count fetch error publishes zero count": {
			DB: &DBMock{
				FetchNotificationCountFunc: func(context.Context, string, string, bool) (uint64, error) {
					return 0, errors.New("boom")
				},
			},
			Count: 0,
		},
		"Successful publish with unread count": {
			DB: &DBMock{
				FetchNotificationCountFunc: func(context.Context, string, string, bool) (uint64, error) {
					return 7, nil
				},
			},
			Count: 7,
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			rcv := &fakeReceiver{}

			hdl := Handler{
				log:      slog.New(slog.DiscardHandler),
				db:       c.DB,
				notifier: rcv,
			}

			// invoke lifecycle callbacks immediately so the domain
			// subscription is active by the time the test fires it.
			tpc := &wsMock.Topic{
				OnFirstSubFunc:  func(fn func(context.Context)) { fn(context.Background()) },
				OnLastUnsubFunc: func(fn func(context.Context)) { fn(context.Background()) },
			}

			hdl.BindNotifications(tpc)

			require.NotNil(t, rcv.fn)
			assert.Equal(t, 1, rcv.unsubbd)

			nt := notificationCore.Notification{
				ID:             xid.New(),
				UserID:         "u1",
				OrganizationID: "org1",
			}

			rcv.fn(context.Background(), nt)

			// the unread count must be fetched for the notification's
			// organization/user pair, in that argument order.
			countCalls := c.DB.FetchNotificationCountCalls()
			require.Len(t, countCalls, 1)
			assert.Equal(t, "org1", countCalls[0].OrganizationID)
			assert.Equal(t, "u1", countCalls[0].UserID)
			assert.False(t, countCalls[0].Read)

			pubs := tpc.PublishManyCalls()
			require.Len(t, pubs, 1)

			payload, ok := pubs[0].Payload.(struct {
				Notification notificationCore.Notification `json:"notification"`
				UnreadCount  uint64                        `json:"unreadCount"`
			})
			require.True(t, ok)
			assert.Equal(t, nt, payload.Notification)
			assert.Equal(t, c.Count, payload.UnreadCount)

			// the publish filter must only pass for the notification's
			// own user within its organization.
			owner := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u1",
				ActiveOrganizationID: "org1",
			})
			stranger := auth.AddSessionToContext(context.Background(), auth.Session{
				UserID:               "u2",
				ActiveOrganizationID: "org1",
			})

			assert.True(t, pubs[0].Filter(owner, "topic"))
			assert.False(t, pubs[0].Filter(stranger, "topic"))
		})
	}

	t.Run("Lifecycle callbacks arrive out of order", func(t *testing.T) {
		t.Parallel()

		rcv := &fakeReceiver{}

		hdl := Handler{
			log:      slog.New(slog.DiscardHandler),
			db:       &DBMock{},
			notifier: rcv,
		}

		// the lifecycle callbacks are dispatched as independent goroutines, so
		// an unsubscribe can land before any subscribe and a re-subscribe can
		// overtake a pending unsubscribe.
		var first, last func(context.Context)

		tpc := &wsMock.Topic{
			OnFirstSubFunc:  func(fn func(context.Context)) { first = fn },
			OnLastUnsubFunc: func(fn func(context.Context)) { last = fn },
		}

		hdl.BindNotifications(tpc)
		require.NotNil(t, first)
		require.NotNil(t, last)

		assert.NotPanics(t, func() {
			last(context.Background())
		})

		first(context.Background())
		first(context.Background())

		// the first registration is released rather than stranded.
		assert.Equal(t, 1, rcv.unsubbd)

		assert.NotPanics(t, func() {
			last(context.Background())
			last(context.Background())
		})

		assert.Equal(t, 2, rcv.unsubbd)
	})
}

// ensure the domain DB interface keeps satisfying the handler's narrow
// DB contract mirrored from it.
var _ DB = notificationCore.DB(nil)
