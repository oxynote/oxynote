import type { EntryKey } from "@pinia/colada"
import isDeepEqual from "fast-deep-equal"

const NOTIFICATION_QUERY_KEYS = {
	list: (limit: number, page: number) =>
		["notifications", "list", limit, page] as const,
	count: (read: boolean) => ["notifications", "count", read] as const,
	listRoot: ["notifications", "list"] as const,
	countRoot: ["notifications", "count"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	function useFetchManyNotifications(
		paramsRef: MaybeRefOrGetter<NotificationsParams>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)
				const { limit, page } = params

				return NOTIFICATION_QUERY_KEYS.list(limit, page)
			},
			query: async () => {
				const params = toValue(paramsRef)
				const { limit, page } = params
				const searchParams = new URLSearchParams({
					limit: String(limit),
					page: String(page),
				})

				return await $coreAPIClient<NotificationsResponse>(
					`/api/notifications?${searchParams.toString()}`,
					{ method: "GET" },
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 60 * 1000, // 1 min
			autoRefetch: true,
		})
	}

	function useFetchNotificationCount(
		paramsRef: MaybeRefOrGetter<NotificationsCountParams>,
	) {
		return useQuery({
			key: () => {
				const params = toValue(paramsRef)

				return NOTIFICATION_QUERY_KEYS.count(params.read)
			},
			query: async () => {
				const params = toValue(paramsRef)
				const searchParams = new URLSearchParams()

				searchParams.set("read", String(params.read))

				const query = searchParams.toString()
				const url = query
					? `/api/notifications/count?${query}`
					: `/api/notifications/count`

				return await $coreAPIClient<{ count: number }>(url, { method: "GET" })
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 60 * 1000, // 1 min
			autoRefetch: true,
		})
	}

	const markNotificationsRead = useMutation({
		// count is hard to optimistically update, so we skip it
		onMutate: (req) => {
			const entries = queryCache.getEntries({
				key: NOTIFICATION_QUERY_KEYS.listRoot,
			})
			const oldNotifs: (NotificationsResponse & { key: EntryKey })[] = []

			entries.forEach((entry) => {
				const oldNotifPage = clone(
					queryCache.getQueryData<NotificationsResponse>(entry.key),
				)

				// entries can exist without data (a query that has not
				// resolved yet); including them would make the update loop
				// below throw on the missing notifications array and abort
				// the whole mutation
				if (!oldNotifPage) {
					return
				}

				oldNotifs.push({ ...oldNotifPage, key: clone(entry.key) })
			})

			const newNotifs = clone(oldNotifs)
			newNotifs.forEach((notifPage) => {
				notifPage.notifications.forEach((notif) => {
					if (req.ids.length === 0 || req.ids.includes(notif.id)) {
						notif.read = true
					}
				})

				queryCache.setQueryData(notifPage.key, notifPage)
				queryCache.cancelQueries({
					key: notifPage.key,
				})
			})

			return { newNotifs, oldNotifs }
		},
		mutation: async (req: MarkNotificationsReadRequest) => {
			await $coreAPIClient(`/api/notifications/read-status`, {
				method: "PUT",
				body: req,
			})
		},
		async onSuccess() {
			await queryCache.invalidateQueries({
				key: NOTIFICATION_QUERY_KEYS.listRoot,
			})
			await queryCache.invalidateQueries({
				key: NOTIFICATION_QUERY_KEYS.countRoot,
			})
		},
		onError(_err, _data, { oldNotifs, newNotifs }) {
			const entries = queryCache.getEntries({
				key: NOTIFICATION_QUERY_KEYS.listRoot,
			})
			const cachedNotifs: (NotificationsResponse & { key: EntryKey })[] = []

			entries.forEach((entry) => {
				const cachedNotifPage = clone(
					queryCache.getQueryData<NotificationsResponse>(entry.key),
				)

				// mirror the onMutate filter so the rollback comparison sees
				// the same set of pages the optimistic update touched
				if (!cachedNotifPage) {
					return
				}

				cachedNotifs.push({ ...cachedNotifPage, key: clone(entry.key) })
			})

			if (!isDeepEqual(newNotifs, cachedNotifs)) {
				return
			}

			// rollback
			oldNotifs?.forEach((notifPage) => {
				queryCache.setQueryData(notifPage.key, notifPage)
			})
		},
	})

	return {
		useFetchManyNotifications,
		useFetchNotificationCount,
		markNotificationsRead,
	}
}
