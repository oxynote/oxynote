package notification

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/cenkalti/backoff/v4"
	"github.com/jellydator/xync"
	"github.com/oxynote/oxynote/server/core/pkg/httpserver"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/rs/xid"
)

// _maxBackoffRetries is the maximum number of retries.
const _maxBackoffRetries = 5

// _newBackoff creates the retry backoff strategy. Variable so tests
// can substitute a faster strategy.
var _newBackoff = func() backoff.BackOff {
	return backoff.NewExponentialBackOff()
}

// Unsubscribe is a function used to unsubscribe from notifications.
type Unsubscribe func()

// Manager is a notification logger.
type Manager struct {
	log  *slog.Logger
	supv *xync.Supervisor
	db   DB

	mu     sync.RWMutex
	nextID uint64
	subs   map[uint64]func(context.Context, Notification)
}

// NewManager creates a new notification logger.
func NewManager(
	log *slog.Logger,
	db DB,
) *Manager {
	return &Manager{
		db:   db,
		log:  log.With("component", "notifications-manager"),
		supv: xync.NewSupervisor(),
		subs: make(map[uint64]func(context.Context, Notification)),
	}
}

// Close stops and closes all processes.
func (m *Manager) Close() error {
	m.supv.CloseAndWait()
	return nil
}

// OnNotification subscribes to the notifications.
func (m *Manager) OnNotification(
	fn func(
		context.Context,
		Notification,
	),
) Unsubscribe {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subs == nil {
		m.subs = make(map[uint64]func(context.Context, Notification))
	}

	id := m.nextID
	m.subs[id] = fn
	m.nextID++

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		delete(m.subs, id)
	}
}

// PublishNotifications publishes new notifications to users.
func (m *Manager) PublishNotifications(organizationID string, nc Core, userIDs ...string) {
	for _, userID := range userIDs {
		nt := newNotification(organizationID, userID, nc)

		m.supv.Go(func(ctx context.Context) {
			rerr := backoff.Retry(
				func() error {
					return m.db.CreateNotification(ctx, nt)
				},
				backoff.WithMaxRetries(
					backoff.WithContext(
						_newBackoff(),
						ctx,
					),
					_maxBackoffRetries,
				),
			)
			// a notification the subscribers deliver but the database never
			// stored disappears on the next reload, so a failed persist has
			// to stop the fan-out rather than fall through to it.
			if rerr != nil {
				if !errors.Is(rerr, context.Canceled) {
					logutil.Critical(m.log, rerr).
						Error("cannot create a new notification")
				}

				return
			}

			m.mu.RLock()
			defer m.mu.RUnlock()

			for _, fn := range m.subs {
				m.supv.Go(func(ctx context.Context) {
					fn(ctx, *nt)
				})
			}
		})
	}
}

// Publisher is an interface used to publish notifications.
type Publisher interface {
	// PublishNotification should publish a new notification.
	PublishNotifications(organizationID string, be Core, userIDs ...string)
}

// Receiver is an interface used to subscribe to notifications.
type Receiver interface {
	// OnNotification should subscribe to notifications.
	OnNotification(fn func(
		context.Context,
		Notification,
	)) Unsubscribe
}

// DB is an interface that handles communication with the notifications database.
//
//go:generate ../../scripts/codegen/mock -t internal DB db
type DB interface {
	// CreateNotification should create a new notification.
	CreateNotification(ctx context.Context, nt *Notification) error

	// FetchManyNotifications should fetch all notifications.
	FetchManyNotifications(ctx context.Context, organizationID, userID string, qr httpserver.Query) ([]*Notification, uint64, error)

	// FetchNotificationCount should fetch the total number of notifications.
	FetchNotificationCount(ctx context.Context, organizationID, userID string, read bool) (uint64, error)

	// MarkReadNotificationsByIDs should mark notifications as read by their IDs.
	// If no ids are provided, all notifications should be marked as read.
	MarkReadByNotificationsIDs(ctx context.Context, organizationID, userID string, ids []xid.ID) error
}
