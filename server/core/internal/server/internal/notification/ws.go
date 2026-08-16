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
	var (
		mu    sync.Mutex
		unsub func()
	)

	tpc.OnFirstSub(func(_ context.Context) {
		mu.Lock()
		defer mu.Unlock()

		// a re-subscribe that overtakes the previous unsubscribe would
		// otherwise strand the earlier registration forever.
		if unsub != nil {
			unsub()
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

		// the callbacks are dispatched as independent goroutines, so an
		// unsubscribe can arrive before any subscribe registered anything.
		if unsub == nil {
			return
		}

		unsub()

		unsub = nil
	})
}
