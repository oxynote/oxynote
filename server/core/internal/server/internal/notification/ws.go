package notification

import (
	"context"
	"sync"

	notificationCore "github.com/oxynote/oxynote/server/core/internal/notification"
	"github.com/oxynote/oxynote/server/core/internal/server/internal/auth"
	"github.com/oxynote/oxynote/server/core/pkg/logutil"
	"github.com/oxynote/wetsocks/wsserver"
)

// BindNotifications publishes notifications.
func (h *Handler) BindNotifications(tpc wsserver.Topic) {
	// the callbacks are dispatched as independent goroutines, so a stale
	// unsubscribe can arrive after a newer subscribe and must not tear its
	// registration down. First-subs and last-unsubs alternate at the topic,
	// so the registration follows the balance of observed transitions
	// instead of the latest event.
	var (
		mu     sync.Mutex
		starts uint64
		stops  uint64
		unsub  func()
	)

	tpc.OnFirstSub(func(_ context.Context) {
		mu.Lock()
		defer mu.Unlock()

		starts++

		if starts <= stops || unsub != nil {
			return
		}

		unsub = h.notifier.OnNotification(func(ctx context.Context, nt notificationCore.Notification) {
			count, err := h.db.FetchNotificationCount(ctx, nt.OrganizationID, nt.UserID, false)
			if err != nil {
				logutil.Critical(h.log, err).
					Error("failed to fetch notifications count")
			}

			tpc.PublishMany(ctx, struct {
				Notification notificationCore.Notification `json:"notification"`
				UnreadCount  uint64                        `json:"unreadCount"`
			}{
				Notification: nt,
				UnreadCount:  count,
			}, func(ctx context.Context, rawTopic string) bool {
				return auth.FilterUser(nt.OrganizationID, nt.UserID)(ctx, rawTopic)
			})
		})
	})

	tpc.OnLastUnsub(func(_ context.Context) {
		mu.Lock()
		defer mu.Unlock()

		stops++

		if starts > stops || unsub == nil {
			return
		}

		unsub()

		unsub = nil
	})
}
