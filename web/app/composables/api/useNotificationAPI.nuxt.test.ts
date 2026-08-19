import type { EntryKey } from "@pinia/colada"
import { afterEach, beforeEach, describe, it } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedQueryData,
} from "./test-helpers"
import useNotificationAPI from "./useNotificationAPI"

const LIST_KEY_A = ["notifications", "list", 10, 1] as const
const LIST_KEY_B = ["notifications", "list", 5, 2] as const

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
): NotificationsResponse {
	return { notifications, pageCount: 1 }
}

function seedPage(key: EntryKey, page: NotificationsResponse) {
	seedQueryData(key, page)
}

function getPage(key: EntryKey) {
	return readQueryData(key) as NotificationsResponse | undefined
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
		it("fetches the requested notification page", async ({ expect }) => {
			const page = makePage([makeNotification("n1", false)])
			const listCalls = mockEndpoint("GET", "/api/notifications", () => page)
			const api = makeNotificationAPI()
			const list = runInApp(() =>
				api.useFetchManyNotifications({ limit: 10, page: 2 }),
			)

			const result = await list.refresh()

			expect(result.data).toEqual(page)
			expect(listCalls).toHaveLength(1)
			expect(listCalls[0]?.query).toEqual({ limit: "10", page: "2" })
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
			seedPage(
				LIST_KEY_A,
				makePage([makeNotification("n1", false), makeNotification("n2", true)]),
			)
			seedPage(LIST_KEY_B, makePage([makeNotification("n3", false)]))
			const api = makeNotificationAPI()

			await api.markNotificationsRead.mutateAsync({ ids: [] })

			expect(getPage(LIST_KEY_A)?.notifications.map((n) => n.read)).toEqual([
				true,
				true,
			])
			expect(getPage(LIST_KEY_B)?.notifications.map((n) => n.read)).toEqual([
				true,
			])
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
			seedPage(
				LIST_KEY_A,
				makePage([
					makeNotification("n1", false),
					makeNotification("n2", false),
				]),
			)
			seedPage(LIST_KEY_B, makePage([makeNotification("n3", false)]))
			const api = makeNotificationAPI()

			await api.markNotificationsRead.mutateAsync({ ids: ["n1", "n3"] })

			expect(getPage(LIST_KEY_A)?.notifications.map((n) => n.read)).toEqual([
				true,
				false,
			])
			expect(getPage(LIST_KEY_B)?.notifications.map((n) => n.read)).toEqual([
				true,
			])
			expect(putCalls).toHaveLength(1)
			expect(putCalls[0]?.body).toEqual({ ids: ["n1", "n3"] })
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
			seedPage(LIST_KEY_A, makePage([makeNotification("n1", false)]))
			seedPage(LIST_KEY_B, makePage([makeNotification("n2", false)]))
			const api = makeNotificationAPI()

			await expect(
				api.markNotificationsRead.mutateAsync({ ids: [] }),
			).rejects.toThrow()

			// the rollback writes back the snapshot taken in onMutate, which
			// carries the entry key the composable injects for bookkeeping
			expect(getPage(LIST_KEY_A)).toEqual({
				...makePage([makeNotification("n1", false)]),
				key: [...LIST_KEY_A],
			})
			expect(getPage(LIST_KEY_B)).toEqual({
				...makePage([makeNotification("n2", false)]),
				key: [...LIST_KEY_B],
			})
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
			seedPage(LIST_KEY_A, makePage([makeNotification("n1", false)]))
			seedPage(LIST_KEY_B, makePage([makeNotification("n2", false)]))
			const api = makeNotificationAPI()

			const pending = api.markNotificationsRead.mutateAsync({ ids: [] })
			await putReachedSignal

			// the optimistic update landed; divergent data written afterwards
			// must survive the failure
			expect(getPage(LIST_KEY_A)?.notifications.map((n) => n.read)).toEqual([
				true,
			])
			const divergent = makePage([makeNotification("n9", false)])
			seedPage(LIST_KEY_A, divergent)
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			expect(getPage(LIST_KEY_A)).toEqual(divergent)
			// the rollback is all-or-nothing, so the untouched page keeps its
			// optimistic state too
			expect(getPage(LIST_KEY_B)?.notifications.map((n) => n.read)).toEqual([
				true,
			])
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
			seedPage(LIST_KEY_B, makePage([makeNotification("n1", false)]))
			const api = makeNotificationAPI()
			const list = runInApp(() =>
				api.useFetchManyNotifications({ limit: 10, page: 1 }),
			)

			const pending = api.markNotificationsRead.mutateAsync({ ids: [] })
			await putReachedSignal

			// reaching the request proves the data-less entry did not abort
			// the optimistic update; only the seeded page was touched
			expect(getPage(LIST_KEY_A)).toBeUndefined()
			expect(getPage(LIST_KEY_B)?.notifications.map((n) => n.read)).toEqual([
				true,
			])
			rejectPut(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			// the data-less entry is filtered from the rollback comparison
			// too, so the rollback still restores the seeded page — with the
			// bookkeeping key the composable injects into its snapshot
			expect(getPage(LIST_KEY_B)).toEqual({
				...makePage([makeNotification("n1", false)]),
				key: [...LIST_KEY_B],
			})
			expect(putCalls).toHaveLength(1)
			expect(listCalls).toHaveLength(1)
			expect(countCalls).toHaveLength(0)

			// settle the deferred load so nothing stays in flight
			const resolved = makePage([makeNotification("n2", true)])
			resolveList(resolved)
			const result = await list.refresh()
			expect(result.data).toEqual(resolved)
		})
	})
})
