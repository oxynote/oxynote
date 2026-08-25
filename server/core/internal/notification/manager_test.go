package notification

import (
	"context"
	"log/slog"
	"testing"

	"github.com/cenkalti/backoff/v4"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/pkg/testutil"
	"github.com/rs/xid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func Test_NewManager(t *testing.T) {
	t.Parallel()

	db := &DBMock{}

	m := NewManager(slog.New(slog.DiscardHandler), db)
	defer m.Close() //nolint:errcheck // close cannot fail here

	require.NotNil(t, m)
	assert.Equal(t, db, m.db)
	assert.NotNil(t, m.log)
	assert.NotNil(t, m.supv)
	assert.NotNil(t, m.backoffStrategy)
	assert.NotNil(t, m.subs)
	assert.Empty(t, m.subs)
	assert.Zero(t, m.nextID)
}

func Test_Manager_Close(t *testing.T) {
	t.Parallel()

	m := NewManager(slog.New(slog.DiscardHandler), &DBMock{})

	assert.NoError(t, m.Close())
}

func Test_Manager_OnNotification(t *testing.T) {
	t.Parallel()

	m := &Manager{subs: make(map[uint64]func(context.Context, Notification))}

	unsub := m.OnNotification(func(_ context.Context, _ Notification) {})
	require.NotNil(t, unsub)
	assert.Len(t, m.subs, 1)

	// second subscription - a fresh ID is assigned
	unsub2 := m.OnNotification(func(_ context.Context, _ Notification) {})
	require.NotNil(t, unsub2)
	assert.Len(t, m.subs, 2)
	assert.EqualValues(t, 2, m.nextID)

	// unsubscribe - only the matching subscription is removed
	unsub()
	assert.Len(t, m.subs, 1)

	unsub2()
	assert.Empty(t, m.subs)
}

func Test_Manager_PublishNotifications(t *testing.T) {
	t.Parallel()

	cc := map[string]struct {
		DB          *DBMock
		Backoff     backoff.BackOff
		UserIDs     []string
		Delivered   bool
		LogContains string
		LogOmits    string
	}{
		"Error returned by db.CreateNotification": {
			DB: &DBMock{
				CreateNotificationFunc: func(_ context.Context, _ *Notification) error {
					return assert.AnError
				},
			},
			Backoff:     &backoff.StopBackOff{},
			UserIDs:     []string{"user1"},
			LogContains: "cannot create a new notification",
		},
		"Cancelled context error is swallowed": {
			DB: &DBMock{
				CreateNotificationFunc: func(_ context.Context, _ *Notification) error {
					return backoff.Permanent(context.Canceled)
				},
			},
			Backoff:  &backoff.StopBackOff{},
			UserIDs:  []string{"user1"},
			LogOmits: "cannot create a new notification",
		},
		"Successful publication to multiple users": {
			DB:        &DBMock{},
			UserIDs:   []string{"user1", "user2"},
			Delivered: true,
			LogOmits:  "cannot create a new notification",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			t.Parallel()

			strategy := c.Backoff
			if strategy == nil {
				strategy = &backoff.ZeroBackOff{}
			}

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewTextHandler(out, nil))

			m := &Manager{
				log:             log,
				db:              c.DB,
				supv:            xync.NewSupervisor(),
				backoffStrategy: func() backoff.BackOff { return strategy },
				subs:            make(map[uint64]func(context.Context, Notification)),
			}

			received := make(chan Notification, len(c.UserIDs))

			m.OnNotification(func(_ context.Context, n Notification) {
				received <- n
			})

			// waiting on the create call keeps the supervisor context from
			// cancelling a retry mid-flight; the fan-out cannot be used for
			// that, as a failed write is not supposed to produce one.
			created := make(chan struct{}, len(c.UserIDs))
			create := c.DB.CreateNotificationFunc

			c.DB.CreateNotificationFunc = func(ctx context.Context, nt *Notification) error {
				var err error

				if create != nil {
					err = create(ctx, nt)
				}

				created <- struct{}{}

				return err
			}

			nc := NewDocumentReviewRequestNotification("user1", xid.New(), xid.New())

			m.PublishNotifications("org1", nc, c.UserIDs...)

			for range c.UserIDs {
				<-created
			}

			// a notification the database never stored must not reach the
			// subscribers: it would show up as a toast and then vanish on
			// the next reload.
			if c.Delivered {
				assertDelivered(t, received, nc, c.UserIDs)
			}

			m.supv.CloseAndWait()

			if !c.Delivered {
				assert.Empty(t, received)
			}

			require.NoError(t, out.Flush())

			if c.LogContains != "" {
				assert.Contains(t, b.String(), c.LogContains)
			}

			if c.LogOmits != "" {
				assert.NotContains(t, b.String(), c.LogOmits)
			}

			ff := c.DB.CreateNotificationCalls()
			require.Len(t, ff, len(c.UserIDs))
		})
	}
}

// assertDelivered drains one fan-out notification per target user and checks
// it carries the published core.
func assertDelivered(t *testing.T, received <-chan Notification, nc Core, userIDs []string) {
	t.Helper()

	users := make(map[string]bool)

	for range userIDs {
		n := <-received

		users[n.UserID] = true

		assert.Equal(t, nc, n.Core)
		assert.Equal(t, "org1", n.OrganizationID)
		assert.False(t, n.ID.IsNil())
	}

	for _, userID := range userIDs {
		assert.True(t, users[userID])
	}
}
