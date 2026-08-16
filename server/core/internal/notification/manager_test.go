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

	// nil subscriber map - the map is initialized lazily
	m := &Manager{}

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
	cc := map[string]struct {
		DB          *DBMock
		Backoff     backoff.BackOff
		UserIDs     []string
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
			DB:       &DBMock{},
			UserIDs:  []string{"user1", "user2"},
			LogOmits: "cannot create a new notification",
		},
	}

	for cn, c := range cc {
		t.Run(cn, func(t *testing.T) {
			// no t.Parallel: cases mutate the shared _newBackoff variable.
			if c.Backoff != nil {
				orig := _newBackoff
				_newBackoff = func() backoff.BackOff { return c.Backoff }

				defer func() { _newBackoff = orig }()
			}

			out, b := testutil.NewBuffer()
			log := slog.New(slog.NewTextHandler(out, nil))

			m := &Manager{
				log:  log,
				db:   c.DB,
				supv: xync.NewSupervisor(),
				subs: make(map[uint64]func(context.Context, Notification)),
			}

			received := make(chan Notification, len(c.UserIDs))

			m.OnNotification(func(_ context.Context, n Notification) {
				received <- n
			})

			nc := NewDocumentReviewRequestNotification("user1", xid.New(), xid.New())

			m.PublishNotifications("org1", nc, c.UserIDs...)

			// subscribers are notified once per target user even when
			// the database write failed. Receiving the fan-out first
			// also guarantees the create-and-log step has finished, so
			// the supervisor context cannot cancel a retry mid-flight.
			users := make(map[string]bool)

			for range c.UserIDs {
				n := <-received

				users[n.UserID] = true

				assert.Equal(t, nc, n.Core)
				assert.Equal(t, "org1", n.OrganizationID)
				assert.False(t, n.ID.IsNil())
			}

			for _, userID := range c.UserIDs {
				assert.True(t, users[userID])
			}

			m.supv.CloseAndWait()

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
