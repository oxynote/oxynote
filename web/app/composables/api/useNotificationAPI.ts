import type { EntryKey, UseInfiniteQueryData } from "@pinia/colada"
import isDeepEqual from "fast-deep-equal"

type NotificationPages = UseInfiniteQueryData<NotificationsResponse, number>

const NOTIFICATION_QUERY_KEYS = {
	list: (limit: number, read?: boolean) =>
		["notifications", "list", limit, read ?? "all"] as const,
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
		return useInfiniteQuery<NotificationsResponse, Error, number>(() => {
			const { limit, read } = toValue(paramsRef)

			return {
				key: NOTIFICATION_QUERY_KEYS.list(limit, read),
				query: async ({ pageParam }) => {
					const searchParams = new URLSearchParams({
						limit: String(limit),
						page: String(pageParam),
					})

					if (read !== undefined) {
						searchParams.set("filter-read_eq", String(read))
					}

					return await $coreAPIClient<NotificationsResponse>(
						`/api/notifications?${searchParams.toString()}`,
						{ method: "GET" },
					)
				},
				initialPageParam: 1,
				getNextPageParam: (lastPage, _pages, lastPageParam) =>
					lastPageParam < lastPage.pageCount ? lastPageParam + 1 : null,
				refetchOnMount: false,
				refetchOnWindowFocus: false,
				refetchOnReconnect: false,
				staleTime: 60 * 1000, // 1 min
				autoRefetch: true,
			}
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
			// the key travels next to the data rather than inside it, so the
			// bookkeeping never ends up in the cached data
			const oldNotifs: { key: EntryKey; data: NotificationPages }[] = []

			entries.forEach((entry) => {
				const oldNotifData = clone(
					queryCache.getQueryData<NotificationPages>(entry.key),
				)

				// entries can exist without data (a query that has not
				// resolved yet); including them would make the update loop
				// below throw on the missing pages array and abort the whole
				// mutation
				if (!oldNotifData) {
					return
				}

				oldNotifs.push({ key: clone(entry.key), data: oldNotifData })
			})

			const newNotifs = clone(oldNotifs)
			newNotifs.forEach(({ key, data }) => {
				data.pages.forEach((page) => {
					page.notifications.forEach((notif) => {
						if (req.ids.length === 0 || req.ids.includes(notif.id)) {
							notif.read = true
						}
					})

					if (key[3] === false) {
						page.notifications = page.notifications.filter(
							(notif) => !notif.read,
						)
					}
				})

				queryCache.setQueryData(key, data)
				queryCache.cancelQueries({ key })
			})

			return { newNotifs, oldNotifs }
		},
		mutation: async (req: MarkNotificationsReadRequest) => {
			await $coreAPIClient(`/api/notifications/read-status`, {
				method: "PUT",
				body: req,
			})
		},
		onSuccess: invalidateNotifications,
		onError(_err, _data, { oldNotifs, newNotifs }) {
			if (!newNotifs) {
				return
			}

			const entries = queryCache.getEntries({
				key: NOTIFICATION_QUERY_KEYS.listRoot,
				// switching filters can populate another cache while the
				// mutation is pending; it was not optimistically changed
				predicate: (entry) =>
					newNotifs.some(({ key }) => isDeepEqual(key, entry.key)),
			})
			const cachedNotifs: { key: EntryKey; data: NotificationPages }[] = []

			entries.forEach((entry) => {
				const cachedNotifData = clone(
					queryCache.getQueryData<NotificationPages>(entry.key),
				)

				// mirror the onMutate filter so the rollback comparison sees
				// the same set of entries the optimistic update touched
				if (!cachedNotifData) {
					return
				}

				cachedNotifs.push({ key: clone(entry.key), data: cachedNotifData })
			})

			if (!isDeepEqual(newNotifs, cachedNotifs)) {
				return
			}

			// rollback
			oldNotifs.forEach(({ key, data }) => {
				queryCache.setQueryData(key, data)
			})
		},
	})

	async function invalidateNotifications() {
		await Promise.all([
			queryCache.invalidateQueries({ key: NOTIFICATION_QUERY_KEYS.listRoot }),
			queryCache.invalidateQueries({ key: NOTIFICATION_QUERY_KEYS.countRoot }),
		])
	}

	return {
		invalidateNotifications,
		useFetchManyNotifications,
		useFetchNotificationCount,
		markNotificationsRead,
	}
}
