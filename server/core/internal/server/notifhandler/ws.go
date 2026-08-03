package notifhandler

import (
	"context"
	"sync"

	"github.com/oxynote/heimdall/internal/notification"
	"github.com/oxynote/heimdall/internal/server/auth"
	"github.com/oxynote/purse/util/logutil"
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

		unsub = h.notifier.OnNotification(func(ctx context.Context, nt notification.Notification) {
			count, err := h.db.FetchNotificationCount(ctx, nt.UserID, nt.OrganizationID, false)
			if err != nil {
				logutil.Critical(h.log, err).
					Error("failed to fetch notifications count")
			}

			tpc.PublishMany(ctx, struct {
				Notification notification.Notification `json:"notification"`
				UnreadCount  uint64                    `json:"unreadCount"`
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

		unsub()
	})
}
