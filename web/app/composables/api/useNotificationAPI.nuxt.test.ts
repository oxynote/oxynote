import type { EntryKey, UseInfiniteQueryData } from "@pinia/colada"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	mockDeferredEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useNotificationAPI from "./useNotificationAPI"

type NotificationPages = UseInfiniteQueryData<NotificationsResponse, number>

const LIST_KEY_A = ["notifications", "list", 10, "all"] as const
const LIST_KEY_B = ["notifications", "list", 5, "all"] as const
const UNREAD_KEY = ["notifications", "list", 10, false] as const
const READ_KEY = ["notifications", "list", 10, true] as const

function makeNotificationAPI() {
	return runInApp(() => useNotificationAPI())
}

// cache seeds pass through the composable's JSON-based clone, so the
// fixture uses a string date — exactly what a cache round-trip produces
function makeNotification(id: string, read: boolean) {
	return {
		id,
		userId: "u1",
		organizationId: "org1",
		code: NotificationCode.DocumentNewComment,
		metadata: {
			userId: "u2",
			documentId: "d1",
			branchId: "b1",
			commentId: "c1",
			anchorBlockId: null,
		},
		read,
		createdAt: "2024-01-01T00:00:00.000Z",
	}
}

function makePage(
	notifications: NotificationsResponse["notifications"],
	pageCount = 1,
): NotificationsResponse {
	return { notifications, pageCount }
}

// what the cache holds for the given pages, loaded from the first one on
function makePages(...pages: NotificationsResponse[]): NotificationPages {
	return { pages, pageParams: pages.map((_page, index) => index + 1) }
}

function seedPages(key: EntryKey, ...pages: NotificationsResponse[]) {
	seedQueryData(key, makePages(...pages))
}

function getPages(key: EntryKey) {
	return readQueryData(key) as NotificationPages | undefined
}

function readStates(key: EntryKey) {
	return getPages(key)?.pages.map((page) =>
		page.notifications.map((n) => n.read),
	)
}

// creating a factory query eagerly loads it once; refresh() joins that
// in-flight load (or reuses its fresh result) instead of forcing a second
// request, which keeps the call accounting deterministic
describe("useNotificationAPI", { concurrent: false }, () => {
	// the tests share the app-wide query cache and the test-time endpoint
	// registry, so they cannot interleave
	beforeEach(clearQueryCache)

	afterEach(disposeMockEndpoints)

	describe("useFetchManyNotifications", () => {
		it.for([
			{ read: false, expected: "false" },
			{ read: true, expected: "true" },
		])(
			"filters notifications on the server with read=$read",
			async ({ read, expected }, { expect }) => {
				const page = makePage([makeNotification("n1", read)])
				const calls = mockEndpoint("GET", "/api/notifications", () => page)
				const api = makeNotificationAPI()
				const params = { limit: 10, read }
				const list = runInApp(() => api.useFetchManyNotifications(params))

				await list.refresh()

				expect(list.data.value).toEqual(makePages(page))
				expect(calls.map((call) => call.query)).toEqual([
					{ limit: "10", page: "1", "filter-read_eq": expected },
				])
			},
		)

		it("fetches the first notification page", async ({ expect }) => {
			const page = makePage([makeNotification("n1", false)])
			const listCalls = mockEndpoint("GET", "/api/notifications", () => page)
			const api = makeNotificationAPI()
			const list = runInApp(() => api.useFetchManyNotifications({ limit: 10 }))

			const result = await list.refresh()

			expect(result.data).toEqual(makePages(page))
			expect(list.hasNextPage.value).toBe(false)
			expect(listCalls).toHaveLength(1)
			expect(listCalls[0]?.query).toEqual({ limit: "10", page: "1" })
		})

		it("loads the next page while the server reports more", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", (call) =>
				makePage([makeNotification(`n${String(call.query.page)}`, false)], 2),
			)
			const api = makeNotificationAPI()
			const list = runInApp(() => api.useFetchManyNotifications({ limit: 10 }))
			await list.refresh()
			expect(list.hasNextPage.value).toBe(true)

			await list.loadNextPage()

			expect(list.data.value).toEqual(
				makePages(
					makePage([makeNotification("n1", false)], 2),
					makePage([makeNotification("n2", false)], 2),
				),
			)
			expect(list.hasNextPage.value).toBe(false)
			expect(listCalls.map((call) => call.query.page)).toEqual(["1", "2"])
		})

		it("keeps all, unread and read pages in separate caches when switching filters", async ({
			expect,
		}) => {
			const calls = mockEndpoint("GET", "/api/notifications", (call) =>
				makePage(
					[
						makeNotification(
							`${String(call.query["filter-read_eq"] ?? "all")}-${String(call.query.page)}`,
							call.query["filter-read_eq"] === "true",
						),
					],
					2,
				),
			)
			const api = makeNotificationAPI()
			const params = ref<NotificationsParams>({ limit: 10 })
			const list = runInApp(() => api.useFetchManyNotifications(params))
			await list.refresh()
			await list.loadNextPage()
			params.value = { limit: 10, read: false }
			await nextTick()
			await list.refresh()
			await list.loadNextPage()
			params.value = { limit: 10, read: true }
			await nextTick()
			await list.refresh()

			params.value = { limit: 10 }
			await nextTick()
			await list.refresh()

			expect(list.data.value?.pageParams).toEqual([1, 2])
			expect(
				getPages(LIST_KEY_A)?.pages.flatMap((page) =>
					page.notifications.map((notification) => notification.id),
				),
			).toEqual(["all-1", "all-2"])
			expect(
				getPages(UNREAD_KEY)?.pages.flatMap((page) =>
					page.notifications.map((notification) => notification.id),
				),
			).toEqual(["false-1", "false-2"])
			expect(
				getPages(READ_KEY)?.pages.flatMap((page) =>
					page.notifications.map((notification) => notification.id),
				),
			).toEqual(["true-1"])
			expect(calls.map((call) => call.query)).toEqual([
				{ limit: "10", page: "1" },
				{ limit: "10", page: "2" },
				{ limit: "10", page: "1", "filter-read_eq": "false" },
				{ limit: "10", page: "2", "filter-read_eq": "false" },
				{ limit: "10", page: "1", "filter-read_eq": "true" },
			])
		})

		it("keeps an in-flight multipage refetch on its original filter", async ({
			expect,
		}) => {
			let releasePage: () => void = () => undefined
			let reachedPage: () => void = () => undefined
			const reached = new Promise<void>((resolve) => {
				reachedPage = resolve
			})
			const release = new Promise<void>((resolve) => {
				releasePage = resolve
			})
			let hold = false
			const calls = mockEndpoint("GET", "/api/notifications", async (call) => {
				if (
					hold &&
					call.query.page === "1" &&
					call.query["filter-read_eq"] === undefined
				) {
					reachedPage()
					await release
				}

				return makePage(
					[
						makeNotification(
							`${String(call.query["filter-read_eq"] ?? "all")}-${String(call.query.page)}`,
							false,
						),
					],
					2,
				)
			})
			const api = makeNotificationAPI()
			const params = ref<NotificationsParams>({ limit: 10 })
			const list = runInApp(() => api.useFetchManyNotifications(params))
			await list.refresh()
			await list.loadNextPage()
			hold = true
			const pending = list.refetch()
			await reached

			params.value = { limit: 10, read: false }
			await nextTick()
			await list.refresh()
			releasePage()
			await pending

			expect(
				getPages(LIST_KEY_A)?.pages.flatMap((page) =>
					page.notifications.map((notification) => notification.id),
				),
			).toEqual(["all-1", "all-2"])
			expect(
				list.data.value?.pages.flatMap((page) =>
					page.notifications.map((notification) => notification.id),
				),
			).toEqual(["false-1"])
			expect(calls.map((call) => call.query)).toEqual([
				{ limit: "10", page: "1" },
				{ limit: "10", page: "2" },
				{ limit: "10", page: "1" },
				{ limit: "10", page: "1", "filter-read_eq": "false" },
				{ limit: "10", page: "2" },
			])
		})

		it("refetches every loaded page in order", async ({ expect }) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", (call) =>
				makePage([makeNotification(`n${String(call.query.page)}`, false)], 2),
			)
			const api = makeNotificationAPI()
			const list = runInApp(() => api.useFetchManyNotifications({ limit: 10 }))
			await list.refresh()
			await list.loadNextPage()

			await list.refetch()

			expect(list.data.value?.pages).toHaveLength(2)
			expect(listCalls.map((call) => call.query.page)).toEqual([
				"1",
				"2",
				"1",
				"2",
			])
		})
	})

	describe("useFetchNotificationCount", () => {
		it.for([
			{ read: false, expected: "false" },
			{ read: true, expected: "true" },
		])(
			"fetches the count of notifications with read=$read",
			async ({ read, expected }, { expect }) => {
				const countCalls = mockEndpoint(
					"GET",
					"/api/notifications/count",
					() => ({ count: 3 }),
				)
				const api = makeNotificationAPI()
				const count = runInApp(() => api.useFetchNotificationCount({ read }))

				const result = await count.refresh()

				expect(result.data).toEqual({ count: 3 })
				expect(countCalls).toHaveLength(1)
				expect(countCalls[0]?.query).toEqual({ read: expected })
			},
		)
	})

	describe("markNotificationsRead", () => {
		it("preserves the error when preparing the optimistic update fails", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", () =>
				makePage([]),
			)
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({ count: 0 }),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			const page = makePage([makeNotification("n1", false)])
			seedPages(LIST_KEY_A, page)
			const api = makeNotificationAPI()
			const queryCache = runInApp(() => useQueryCache())
			const error = new Error("Cache unavailable")
			const getQueryData = vi
				.spyOn(queryCache, "getQueryData")
				.mockImplementationOnce(() => {
					throw error
				})

			try {
				await expect(
					api.markNotificationsRead.mutateAsync({ ids: ["n1"] }),
				).rejects.toBe(error)
				expect(getQueryData).toHaveBeenCalledExactlyOnceWith(LIST_KEY_A)
				expect(putCalls).toHaveLength(0)
				expect(listCalls).toHaveLength(0)
				expect(countCalls).toHaveLength(0)
			} finally {
				getQueryData.mockRestore()
			}

			expect(getPages(LIST_KEY_A)).toEqual(makePages(page))
		})

		it.for([
			{ name: "one notification", ids: ["n1"], remaining: ["n2"] },
			{ name: "all notifications", ids: [], remaining: [] },
		])(
			"removes $name from unread pages while updating all pages",
			async ({ ids, remaining }, { expect }) => {
				const put = mockDeferredEndpoint(
					"PUT",
					"/api/notifications/read-status",
				)
				seedPages(
					LIST_KEY_A,
					makePage([
						makeNotification("n1", false),
						makeNotification("n2", false),
					]),
				)
				seedPages(
					UNREAD_KEY,
					makePage([makeNotification("n1", false)]),
					makePage([makeNotification("n2", false)]),
				)
				seedPages(READ_KEY, makePage([makeNotification("n3", true)]))
				const api = makeNotificationAPI()

				const pending = api.markNotificationsRead.mutateAsync({ ids })
				await put.reached

				expect(
					getPages(UNREAD_KEY)?.pages.flatMap((page) =>
						page.notifications.map((notification) => notification.id),
					),
				).toEqual(remaining)
				expect(readStates(LIST_KEY_A)).toEqual([[true, ids.length === 0]])
				expect(getPages(READ_KEY)).toEqual(
					makePages(makePage([makeNotification("n3", true)])),
				)
				expect(put.calls.map((call) => call.body)).toEqual([{ ids }])
				put.resolve({})
				await pending
			},
		)

		it("restores unread page membership when marking read fails", async ({
			expect,
		}) => {
			const put = mockDeferredEndpoint("PUT", "/api/notifications/read-status")
			const unreadPage = makePage([makeNotification("n1", false)])
			seedPages(LIST_KEY_A, unreadPage)
			seedPages(UNREAD_KEY, unreadPage)
			seedPages(READ_KEY, makePage([makeNotification("n2", true)]))
			const api = makeNotificationAPI()
			const pending = api.markNotificationsRead.mutateAsync({ ids: ["n1"] })
			await put.reached
			expect(getPages(UNREAD_KEY)?.pages[0]?.notifications).toEqual([])

			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getPages(UNREAD_KEY)).toEqual(makePages(unreadPage))
			expect(getPages(LIST_KEY_A)).toEqual(makePages(unreadPage))
			expect(readStates(READ_KEY)).toEqual([[true]])
			expect(put.calls).toHaveLength(1)
		})

		it("rolls back touched pages when a different filter loads during a failed mutation", async ({
			expect,
		}) => {
			const unreadPage = makePage([makeNotification("n1", false)])
			const calls = mockEndpoint("GET", "/api/notifications", (call) =>
				call.query["filter-read_eq"] === "false" ? unreadPage : makePage([]),
			)
			const put = mockDeferredEndpoint("PUT", "/api/notifications/read-status")
			seedPages(LIST_KEY_A, unreadPage)
			const api = makeNotificationAPI()
			const params = ref<NotificationsParams>({ limit: 10, read: false })
			const list = runInApp(() => api.useFetchManyNotifications(params))
			await list.refresh()
			const pending = api.markNotificationsRead.mutateAsync({ ids: ["n1"] })
			await put.reached
			expect(getPages(UNREAD_KEY)?.pages[0]?.notifications).toEqual([])
			params.value = { limit: 10, read: true }
			await nextTick()
			await list.refresh()

			put.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getPages(UNREAD_KEY)).toEqual(makePages(unreadPage))
			expect(getPages(LIST_KEY_A)).toEqual(makePages(unreadPage))
			expect(getPages(READ_KEY)).toEqual(makePages(makePage([])))
			expect(calls.map((call) => call.query["filter-read_eq"])).toEqual([
				"false",
				"true",
			])
			expect(put.calls).toHaveLength(1)
		})

		it("marks every cached notification as read when no ids are given", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", () =>
				makePage([]),
			)
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({
					count: 0,
				}),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			seedPages(
				LIST_KEY_A,
				makePage([makeNotification("n1", false), makeNotification("n2", true)]),
				makePage([makeNotification("n4", false)]),
			)
			seedPages(LIST_KEY_B, makePage([makeNotification("n3", false)]))
			const api = makeNotificationAPI()

			await api.markNotificationsRead.mutateAsync({ ids: [] })

			expect(readStates(LIST_KEY_A)).toEqual([[true, true], [true]])
			expect(readStates(LIST_KEY_B)).toEqual([[true]])
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ ids: [] })
			// the success invalidation only refetches active queries — the
			// seeded entries have no query attached, so nothing is fetched
			expect(listCalls).toHaveLength(0)
			expect(countCalls).toHaveLength(0)
		})

		it("marks only the notifications with the given ids as read", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", () =>
				makePage([]),
			)
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({
					count: 0,
				}),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			seedPages(
				LIST_KEY_A,
				makePage([
					makeNotification("n1", false),
					makeNotification("n2", false),
				]),
				makePage([
					makeNotification("n4", false),
					makeNotification("n5", false),
				]),
			)
			seedPages(LIST_KEY_B, makePage([makeNotification("n3", false)]))
			const api = makeNotificationAPI()

			await api.markNotificationsRead.mutateAsync({ ids: ["n1", "n3", "n5"] })

			expect(readStates(LIST_KEY_A)).toEqual([
				[true, false],
				[false, true],
			])
			expect(readStates(LIST_KEY_B)).toEqual([[true]])
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ ids: ["n1", "n3", "n5"] })
			expect(listCalls).toHaveLength(0)
			expect(countCalls).toHaveLength(0)
		})

		it("rolls back the cached pages when the request fails", async ({
			expect,
		}) => {
			const listCalls = mockEndpoint("GET", "/api/notifications", () =>
				makePage([]),
			)
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({
					count: 0,
				}),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => {
					throw createError({ statusCode: 500 })
				},
			)
			seedPages(
				LIST_KEY_A,
				makePage([makeNotification("n1", false)]),
				makePage([makeNotification("n3", false)]),
			)
			seedPages(LIST_KEY_B, makePage([makeNotification("n2", false)]))
			const api = makeNotificationAPI()

			await expect(
				api.markNotificationsRead.mutateAsync({ ids: [] }),
			).rejects.toThrow()

			expect(getPages(LIST_KEY_A)).toEqual(
				makePages(
					makePage([makeNotification("n1", false)]),
					makePage([makeNotification("n3", false)]),
				),
			)
			expect(getPages(LIST_KEY_B)).toEqual(
				makePages(makePage([makeNotification("n2", false)])),
			)
			expect(putCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(countCalls).toHaveLength(0)
		})

		it("skips the rollback when the cache changed after the optimistic update", async ({
			expect,
		}) => {
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			const listCalls = mockEndpoint("GET", "/api/notifications", () =>
				makePage([]),
			)
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({
					count: 0,
				}),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => {
					putReached()

					return new Promise((_resolve, reject) => {
						rejectPut = reject
					})
				},
			)
			seedPages(LIST_KEY_A, makePage([makeNotification("n1", false)]))
			seedPages(LIST_KEY_B, makePage([makeNotification("n2", false)]))
			const api = makeNotificationAPI()

			const pending = api.markNotificationsRead.mutateAsync({ ids: [] })
			await putReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(readStates(LIST_KEY_A)).toEqual([[true]])
			const divergent = makePage([makeNotification("n9", false)])
			seedPages(LIST_KEY_A, divergent)
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getPages(LIST_KEY_A)).toEqual(makePages(divergent))
			// the rollback is all-or-nothing, so the untouched entry keeps its
			// optimistic state too
			expect(readStates(LIST_KEY_B)).toEqual([[true]])
			expect(putCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(0)
			expect(countCalls).toHaveLength(0)
		})

		it("skips cached entries that have no data yet", async ({ expect }) => {
			let resolveList: (page: NotificationsResponse) => void = () => undefined
			let rejectPut: (err: unknown) => void = () => undefined
			let putReached: () => void = () => undefined
			const putReachedSignal = new Promise<void>((resolve) => {
				putReached = resolve
			})

			// the list load stays pending for the whole mutation, so its cache
			// entry exists without data while the optimistic update runs
			const listCalls = mockEndpoint("GET", "/api/notifications", () => {
				return new Promise((resolve) => {
					resolveList = resolve
				})
			})
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({
					count: 0,
				}),
			)
			const putCalls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => {
					putReached()

					return new Promise((_resolve, reject) => {
						rejectPut = reject
					})
				},
			)
			seedPages(LIST_KEY_B, makePage([makeNotification("n1", false)]))
			const api = makeNotificationAPI()
			const list = runInApp(() => api.useFetchManyNotifications({ limit: 10 }))

			const pending = api.markNotificationsRead.mutateAsync({ ids: [] })
			await putReachedSignal

			// reaching the request proves the data-less entry did not abort
			// the optimistic update; only the seeded entry was touched
			expect(getPages(LIST_KEY_A)).toBeUndefined()
			expect(readStates(LIST_KEY_B)).toEqual([[true]])
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			// the data-less entry is filtered from the rollback comparison
			// too, so the rollback still restores the seeded entry
			expect(getPages(LIST_KEY_B)).toEqual(
				makePages(makePage([makeNotification("n1", false)])),
			)
			expect(putCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(1)
			expect(countCalls).toHaveLength(0)

			// settle the deferred load so nothing stays in flight
			const resolved = makePage([makeNotification("n2", true)])
			resolveList(resolved)
			const result = await list.refresh()
			expect(result.data).toEqual(makePages(resolved))
		})
	})
})
