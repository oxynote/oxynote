import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { setInfiniteQueryData } from "@pinia/colada"
import { enableAutoUnmount, flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockDeferredEndpoint,
	mockEndpoint,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import NotificationBox from "./NotificationBox.vue"
import NotificationRow from "./NotificationRow.vue"
import {
	at,
	findButtonByText,
	mockAuthOrganization,
	renderedIconNames,
	seedAuthOrganization,
	settleMutations,
	t,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// opening a notification navigates; the destination it picks is the whole
// behaviour, and the test app has no page to land on
const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

// happy-dom lays nothing out, so the real scroll watcher would see the list
// as always at its end. The box's callbacks are driven by hand instead.
const infiniteScroll = vi.hoisted(() => ({
	load: (): unknown => undefined,
	canLoadMore: (): boolean => false,
}))
mockNuxtImport(
	"useInfiniteScroll",
	() =>
		(
			_element: unknown,
			onLoadMore: () => unknown,
			options: { canLoadMore: () => boolean },
		) => {
			infiniteScroll.load = onLoadMore
			infiniteScroll.canLoadMore = options.canLoadMore
		},
)

const DOC_ID = "doc1".padEnd(20, "0")
const USER_ID = "user".padEnd(20, "0")
const COMMENT_METADATA = {
	userId: USER_ID,
	documentId: DOC_ID,
	branchId: "b",
	commentId: "c",
	anchorBlockId: null,
}

// the query cache, the pinia websocket store and the vue-sonner module
// mock are all app-wide singletons every mount in the file shares
describe("<NotificationBox>", { concurrent: false }, () => {
	// the box keeps a clock and a polling query running while mounted
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		vi.setSystemTime(new Date(2026, 2, 14, 12, 0, 0))
		// a successful mark-read invalidates both notification queries; without
		// these the refetch rejects and drags the mutation down with it, so the
		// failure toast lands in whichever test is running by then
		mockEndpoint("GET", "/api/notifications", () => ({
			notifications: [],
			pageCount: 1,
		}))
		mockEndpoint("GET", "/api/notifications/count", () => ({ count: 0 }))
	})

	afterEach(() => {
		disposeMockEndpoints()
		useWebSocketStateStore().state = null
		vi.useRealTimers()
	})

	it("says the inbox is empty when there are no notifications", async ({
		expect,
	}) => {
		seedNotifications([])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(t("notification.empty-title"))
	})

	it("lists one row per notification", async ({ expect }) => {
		seedNotifications([makeNotification(), makeNotification({ id: "notif-2" })])

		const wrapper = await mountBox()

		expect(rows(wrapper)).toHaveLength(2)
	})

	it("groups the notifications under the day they arrived", async ({
		expect,
	}) => {
		seedNotifications([
			makeNotification({ id: "n1", createdAt: new Date(2026, 2, 14, 9) }),
			makeNotification({ id: "n2", createdAt: new Date(2026, 2, 13, 9) }),
			makeNotification({ id: "n3", createdAt: new Date(2026, 2, 13, 8) }),
			makeNotification({ id: "n4", createdAt: new Date(2026, 2, 2, 9) }),
		])

		const wrapper = await mountBox()

		expect(
			wrapper.findAll("section").map((section) => ({
				day: section.get("h3").text(),
				rows: section.findAll("[role='link']").length,
			})),
		).toEqual([
			{ day: t("notification.days.today"), rows: 1 },
			{ day: t("notification.days.yesterday"), rows: 2 },
			{ day: t("notification.days.earlier"), rows: 1 },
		])
	})

	it("leaves out a day without notifications", async ({ expect }) => {
		seedNotifications([
			makeNotification({ createdAt: new Date(2026, 2, 2, 9) }),
		])

		const wrapper = await mountBox()

		expect(wrapper.findAll("h3").map((heading) => heading.text())).toEqual([
			t("notification.days.earlier"),
		])
	})

	it("shows a notification the next page repeats only once", async ({
		expect,
	}) => {
		seedPages([
			[makeNotification({ id: "n1" }), makeNotification({ id: "n2" })],
			[makeNotification({ id: "n2" }), makeNotification({ id: "n3" })],
		])

		const wrapper = await mountBox()

		expect(rows(wrapper)).toHaveLength(3)
	})

	it("names the page the notification is about", async ({ expect }) => {
		seedTree()
		seedNotifications([makeNotification()])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain("Runbook")
	})

	it("names the member who commented", async ({ expect }) => {
		seedAuthOrganization({
			members: [{ userId: USER_ID, user: { name: "Ada" } }],
		})
		seedNotifications([
			makeNotification({
				code: NotificationCode.DocumentNewComment,
				metadata: COMMENT_METADATA,
			}),
		])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(
			t("notification.messages.document-new-comment-description", {
				user: "Ada",
			}),
		)
	})

	it("calls a commenter who left the organization a deleted user", async ({
		expect,
	}) => {
		seedAuthOrganization({ members: [] })
		seedNotifications([
			makeNotification({
				code: NotificationCode.DocumentNewComment,
				metadata: COMMENT_METADATA,
			}),
		])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(
			t("notification.messages.document-new-comment-description", {
				user: t("general.deleted-user"),
			}),
		)
	})

	it("closes the box when its collapse button is pressed", async ({
		expect,
	}) => {
		seedNotifications([])
		const wrapper = await mountBox()

		await findButtonByText(
			wrapper,
			t("notification.actions.close-notification-box"),
		).trigger("click")

		expect(wrapper.emitted("close-notification-box")).toHaveLength(1)
	})

	// filter changes share the mounted query and the endpoint registry.
	describe("filtering", { concurrent: false }, () => {
		it("selects all notifications by default", async ({ expect }) => {
			seedNotifications([
				makeNotification(),
				makeNotification({ id: "read", read: true }),
			])

			const wrapper = await mountBox()

			expect(wrapper.get("[role='group']").attributes("aria-label")).toBe(
				t("notification.filters.label"),
			)
			expect(wrapper.get("button[aria-pressed='true']").text()).toBe(
				t("notification.filters.all"),
			)
			expect(rows(wrapper)).toHaveLength(2)
		})

		it.for([
			{ filter: "unread", read: false },
			{ filter: "read", read: true },
		])(
			"fetches $filter notifications beyond the first unfiltered page",
			async ({ filter, read }, { expect }) => {
				disposeMockEndpoints()
				const notifications = Array.from({ length: 52 }, (_unused, index) =>
					makeNotification({
						id: `n${index}`,
						read: index < 50 ? !read : read,
					}),
				)
				const listCalls = mockEndpoint("GET", "/api/notifications", (call) => {
					const filtered =
						call.query["filter-read_eq"] === undefined
							? notifications
							: notifications.filter(
									(notification) =>
										String(notification.read) === call.query["filter-read_eq"],
								)

					return {
						notifications: filtered.slice(0, 50),
						pageCount: Math.ceil(filtered.length / 50),
					}
				})
				const countCalls = mockEndpoint(
					"GET",
					"/api/notifications/count",
					() => ({
						count: notifications.filter((notification) => !notification.read)
							.length,
					}),
				)
				const wrapper = await mountBox()
				await settleMutations()
				expect(rows(wrapper)).toHaveLength(50)

				await findButtonByText(
					wrapper,
					t(`notification.filters.${filter}`),
				).trigger("click")
				await settleMutations()

				expect(notificationIds(wrapper)).toEqual(["n50", "n51"])
				expect(wrapper.get("button[aria-pressed='true']").text()).toBe(
					t(`notification.filters.${filter}`),
				)
				expect(infiniteScroll.canLoadMore()).toBe(false)
				expect(listCalls.map((call) => call.query)).toEqual([
					{ limit: "50", page: "1" },
					{ limit: "50", page: "1", "filter-read_eq": String(read) },
				])
				expect(countCalls.map((call) => call.query)).toEqual([
					{ read: "false" },
				])
			},
		)

		it("starts a new filter at page one and resumes each filter's cached pages", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			const calls = mockEndpoint("GET", "/api/notifications", (call) => ({
				notifications: [
					makeNotification({
						id: `${String(call.query["filter-read_eq"] ?? "all")}-${String(call.query.page)}`,
					}),
				],
				pageCount: call.query["filter-read_eq"] === "false" ? 2 : 3,
			}))
			seedPages(
				[
					[makeNotification({ id: "all-1" })],
					[makeNotification({ id: "all-2" })],
				],
				3,
			)
			const wrapper = await mountBox()
			const scroll = wrapper.get(".overflow-y-auto").element
			scroll.scrollTop = 120

			await findButtonByText(wrapper, t("notification.filters.unread")).trigger(
				"click",
			)
			await settleMutations()
			expect(scroll.scrollTop).toBe(0)
			await infiniteScroll.load()
			await settleMutations()
			expect(notificationIds(wrapper)).toEqual(["false-1", "false-2"])
			expect(infiniteScroll.canLoadMore()).toBe(false)
			await findButtonByText(wrapper, t("notification.filters.all")).trigger(
				"click",
			)
			await settleMutations()
			expect(notificationIds(wrapper)).toEqual(["all-1", "all-2"])
			await infiniteScroll.load()
			await settleMutations()

			expect(notificationIds(wrapper)).toEqual(["all-1", "all-2", "all-3"])
			expect(calls.map((call) => call.query)).toEqual([
				{ limit: "50", page: "1", "filter-read_eq": "false" },
				{ limit: "50", page: "2", "filter-read_eq": "false" },
				{ limit: "50", page: "3" },
			])
		})

		it("describes an empty filtered list without saying the whole inbox is empty", async ({
			expect,
		}) => {
			seedNotifications([makeNotification()], 1)
			const wrapper = await mountBox()

			await findButtonByText(wrapper, t("notification.filters.read")).trigger(
				"click",
			)
			await settleMutations()

			expect(wrapper.text()).toContain(t("notification.filtered-empty-title"))
			expect(wrapper.text()).toContain(
				t("notification.filtered-empty-description"),
			)
			expect(wrapper.text()).not.toContain(t("notification.empty-title"))
			expect(rows(wrapper)).toHaveLength(0)
			expect(
				findButtonByText(wrapper, t("notification.read-all-button")).exists(),
			).toBe(true)
		})

		it.for([
			{ name: "one notification", all: false },
			{ name: "all notifications", all: true },
		])(
			"updates filtered results and the unread count after marking $name read",
			async ({ all }, { expect }) => {
				disposeMockEndpoints()
				let notifications = [
					makeNotification(),
					makeNotification({ id: "notif-2" }),
				]
				const listCalls = mockEndpoint("GET", "/api/notifications", (call) => ({
					notifications: notifications.filter(
						(notification) =>
							call.query["filter-read_eq"] === undefined ||
							String(notification.read) === call.query["filter-read_eq"],
					),
					pageCount: 1,
				}))
				const countCalls = mockEndpoint(
					"GET",
					"/api/notifications/count",
					() => ({
						count: notifications.filter((notification) => !notification.read)
							.length,
					}),
				)
				const putCalls = mockEndpoint(
					"PUT",
					"/api/notifications/read-status",
					(call) => {
						const { ids } = call.body as { ids: string[] }
						notifications = notifications.map((notification) => ({
							...notification,
							read:
								notification.read ||
								ids.length === 0 ||
								ids.includes(notification.id),
						}))

						return {}
					},
				)
				const wrapper = await mountBox()
				await settleMutations()
				await findButtonByText(
					wrapper,
					t("notification.filters.unread"),
				).trigger("click")
				await settleMutations()

				await findButtonByText(
					wrapper,
					t(
						all
							? "notification.read-all-button"
							: "notification.actions.mark-read",
					),
				).trigger("click")
				await settleMutations()

				expect(notificationIds(wrapper)).toEqual(all ? [] : ["notif-2"])
				expect(wrapper.get("button[aria-pressed='true']").text()).toBe(
					t("notification.filters.unread"),
				)
				expect(
					wrapper
						.findAll("button")
						.some(
							(button) => button.text() === t("notification.read-all-button"),
						),
				).toBe(!all)
				expect(putCalls.map((call) => call.body)).toEqual([
					{ ids: all ? [] : ["notif-1"] },
				])
				expect(countCalls).toHaveLength(2)
				expect(listCalls.map((call) => call.query["filter-read_eq"])).toEqual([
					undefined,
					"false",
					"false",
				])
				expect(toast.custom).not.toHaveBeenCalled()
			},
		)

		it("preserves the selected filter on live updates and invalidates inactive filters", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			let notifications = [makeNotification()]
			let onNotification: () => void = () => undefined
			const subscribe = vi.fn((_topic: string, callback: () => void) => {
				onNotification = callback

				return vi.fn()
			})
			useWebSocketStateStore().state = { subscribe } as never
			const listCalls = mockEndpoint("GET", "/api/notifications", () => ({
				notifications,
				pageCount: 1,
			}))
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({ count: notifications.length }),
			)
			const wrapper = await mountBox()
			await settleMutations()
			await findButtonByText(wrapper, t("notification.filters.unread")).trigger(
				"click",
			)
			await settleMutations()
			notifications = [...notifications, makeNotification({ id: "notif-2" })]

			onNotification()
			await settleMutations()

			expect(wrapper.get("button[aria-pressed='true']").text()).toBe(
				t("notification.filters.unread"),
			)
			expect(notificationIds(wrapper)).toEqual(["notif-1", "notif-2"])
			await findButtonByText(wrapper, t("notification.filters.all")).trigger(
				"click",
			)
			await settleMutations()
			expect(notificationIds(wrapper)).toEqual(["notif-1", "notif-2"])
			expect(listCalls.map((call) => call.query["filter-read_eq"])).toEqual([
				undefined,
				"false",
				"false",
				undefined,
			])
			expect(countCalls).toHaveLength(2)
			expect(subscribe).toHaveBeenCalledTimes(1)
		})
	})

	describe("loading more", { concurrent: false }, () => {
		it.for([
			{ name: "offers more while the server has pages", input: 2 },
			{ name: "offers no more on the last page", input: 1 },
		])("$name", async ({ input }, { expect }) => {
			seedPages([[makeNotification()]], input)

			await mountBox()

			expect(infiniteScroll.canLoadMore()).toBe(input > 1)
		})

		it("adds the next page when the list nears its end", async ({ expect }) => {
			disposeMockEndpoints()
			const listCalls = mockEndpoint("GET", "/api/notifications", () => ({
				notifications: [makeNotification({ id: "notif-2" })],
				pageCount: 2,
			}))
			seedPages([[makeNotification()]], 2)
			const wrapper = await mountBox()

			await infiniteScroll.load()
			await flushPromises()

			expect(rows(wrapper)).toHaveLength(2)
			expect(listCalls).toHaveLength(1)
			expect(listCalls[0]?.query).toEqual({ limit: "50", page: "2" })
		})

		it("stops asking for more once a page fails to load", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			// the client retries a GET once on a 5xx, which a 400 keeps out
			// of the count
			const listCalls = mockEndpoint("GET", "/api/notifications", () => {
				throw createError({ statusCode: 400 })
			})
			seedPages([[makeNotification()]], 2)
			const wrapper = await mountBox()

			await infiniteScroll.load()
			await flushPromises()

			expect(infiniteScroll.canLoadMore()).toBe(false)
			expect(rows(wrapper)).toHaveLength(1)
			expect(listCalls).toHaveLength(1)
		})

		it("shows a spinner while the next page loads", async ({ expect }) => {
			disposeMockEndpoints()
			const list = mockDeferredEndpoint("GET", "/api/notifications")
			seedPages([[makeNotification()]], 2)
			const wrapper = await mountBox()

			const loading = infiniteScroll.load()
			await list.reached
			await nextTick()

			expect(renderedIconNames(wrapper)).toContain(
				"svg-spinners:blocks-shuffle-3",
			)

			// settle the held request so nothing stays in flight
			list.resolve({ notifications: [], pageCount: 2 })
			await loading
			await flushPromises()
			expect(renderedIconNames(wrapper)).not.toContain(
				"svg-spinners:blocks-shuffle-3",
			)
		})
	})

	describe("marking as read", { concurrent: false }, () => {
		it("marks a single notification read", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await findButtonByText(
				wrapper,
				t("notification.actions.mark-read"),
			).trigger("click")
			await flushPromises()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ ids: ["notif-1"] })
		})

		it("warns when marking a single notification read fails", async ({
			expect,
		}) => {
			mockEndpoint("PUT", "/api/notifications/read-status", () => {
				throw createError({ statusCode: 500 })
			})
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await findButtonByText(
				wrapper,
				t("notification.actions.mark-read"),
			).trigger("click")
			await flushPromises()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})

		it("marks every notification read with an empty id list", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			seedNotifications([makeNotification()], 1)
			const wrapper = await mountBox()

			await findButtonByText(
				wrapper,
				t("notification.read-all-button"),
			).trigger("click")
			await flushPromises()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ ids: [] })
		})

		it("offers to mark everything read while a page not loaded yet holds unread ones", async ({
			expect,
		}) => {
			seedNotifications([makeNotification({ read: true })], 3)

			const wrapper = await mountBox()

			expect(
				findButtonByText(wrapper, t("notification.read-all-button")).exists(),
			).toBe(true)
		})

		it("hides the button when nothing is unread", async ({ expect }) => {
			seedNotifications([makeNotification({ read: true })], 0)

			const wrapper = await mountBox()

			expect(
				wrapper
					.findAll("button")
					.some((b) => b.text().includes(t("notification.read-all-button"))),
			).toBe(false)
		})

		it("warns when marking everything read fails", async ({ expect }) => {
			mockEndpoint("PUT", "/api/notifications/read-status", () => {
				throw createError({ statusCode: 500 })
			})
			seedNotifications([makeNotification()], 1)
			const wrapper = await mountBox()

			await findButtonByText(
				wrapper,
				t("notification.read-all-button"),
			).trigger("click")
			await flushPromises()

			expect(toast.custom).toHaveBeenCalledTimes(1)
		})
	})

	describe("navigation", { concurrent: false }, () => {
		it.for([
			{
				name: "opens a review request at the document",
				notification: {
					code: NotificationCode.DocumentReviewRequest,
					metadata: { userId: USER_ID, documentId: DOC_ID, branchId: "b" },
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b`,
			},
			{
				name: "opens a triggered hook at its block",
				notification: {
					code: NotificationCode.DocumentHookTriggered,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: "block-7",
						type: DocumentHookType.URLWatcher,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b#block-7`,
			},
			{
				name: "opens a triggered page-wide hook at the document",
				notification: {
					code: NotificationCode.DocumentHookTriggered,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: null,
						type: DocumentHookType.URLWatcher,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b`,
			},
			{
				name: "opens a hook that needs attention at the block it watches",
				notification: {
					code: NotificationCode.DocumentHookNeedsAttention,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: "block-7",
						type: DocumentHookType.URLWatcher,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b#block-7`,
			},
			{
				name: "opens a new comment at the block it is anchored to",
				notification: {
					code: NotificationCode.DocumentNewComment,
					metadata: { ...COMMENT_METADATA, anchorBlockId: "block-3" },
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b#block-3`,
			},
			{
				name: "opens an unanchored comment at the document",
				notification: {
					code: NotificationCode.DocumentNewComment,
					metadata: COMMENT_METADATA,
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b`,
			},
			{
				name: "opens a comment reply at the block it is anchored to",
				notification: {
					code: NotificationCode.DocumentNewCommentReply,
					metadata: {
						...COMMENT_METADATA,
						commentReplyId: "r",
						anchorBlockId: "block-4",
					},
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b#block-4`,
			},
			{
				name: "opens an unanchored comment reply at the document",
				notification: {
					code: NotificationCode.DocumentNewCommentReply,
					metadata: { ...COMMENT_METADATA, commentReplyId: "r" },
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b`,
			},
			{
				name: "opens a resolved comment at the block it is anchored to",
				notification: {
					code: NotificationCode.DocumentCommentResolved,
					metadata: { ...COMMENT_METADATA, anchorBlockId: "block-5" },
				},
				expected: `/acme/Runbook-${DOC_ID}?branch=b#block-5`,
			},
		])("$name", async ({ notification, expected }, { expect }) => {
			const calls = mockEndpoint(
				"PUT",
				"/api/notifications/read-status",
				() => ({}),
			)
			mockAuthOrganization({ id: "org-1", slug: "acme", members: [] })
			seedTree()
			seedNotifications([makeNotification(notification)])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(expected)
			expect(calls.map((call) => call.body)).toEqual([{ ids: ["notif-1"] }])
		})

		it("goes nowhere for a notification code it does not know", async ({
			expect,
		}) => {
			mockAuthOrganization({ id: "org-1", slug: "acme", members: [] })
			seedTree()
			seedNotifications([makeNotification({ code: "notification.unknown" })])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})

		it("stays put when the notification's document no longer exists", async ({
			expect,
		}) => {
			seedAuthOrganization({ slug: "acme", members: [] })
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(navigateToMock).toHaveBeenCalledTimes(0)
		})
	})

	describe("websocket updates", { concurrent: false }, () => {
		it("subscribes to notification creations while mounted", async ({
			expect,
		}) => {
			const subscribe = vi.fn().mockReturnValue(vi.fn())
			useWebSocketStateStore().state = { subscribe } as never
			seedNotifications([])

			await mountBox()

			expect(subscribe).toHaveBeenCalledTimes(1)
			expect(subscribe.mock.calls[0]?.[0]).toBe(WS_NOTIFICATION_CREATION_TOPIC)
			useWebSocketStateStore().state = null
		})

		it("still refreshes the unread count when a live list refresh fails", async ({
			expect,
		}) => {
			disposeMockEndpoints()
			let onNotification: () => void = () => undefined
			const subscribe = vi.fn((_topic: string, callback: () => void) => {
				onNotification = callback

				return vi.fn()
			})
			useWebSocketStateStore().state = { subscribe } as never
			const listCalls = mockEndpoint("GET", "/api/notifications", () => {
				throw createError({ statusCode: 400 })
			})
			const countCalls = mockEndpoint(
				"GET",
				"/api/notifications/count",
				() => ({ count: 2 }),
			)
			seedNotifications([])
			const wrapper = await mountBox()

			onNotification()
			await settleMutations()

			expect(
				findButtonByText(wrapper, t("notification.read-all-button")).exists(),
			).toBe(true)
			expect(wrapper.get("button[aria-pressed='true']").text()).toBe(
				t("notification.filters.all"),
			)
			expect(infiniteScroll.canLoadMore()).toBe(false)
			expect(listCalls).toHaveLength(1)
			expect(countCalls).toHaveLength(1)
			expect(subscribe).toHaveBeenCalledTimes(1)
		})

		it("unsubscribes when it goes away", async ({ expect }) => {
			const unsubscribe = vi.fn()
			useWebSocketStateStore().state = {
				subscribe: vi.fn().mockReturnValue(unsubscribe),
			} as never
			seedNotifications([])
			const wrapper = await mountBox()

			wrapper.unmount()

			expect(unsubscribe).toHaveBeenCalledTimes(1)
			useWebSocketStateStore().state = null
		})
	})
})

function makeNotification(overrides: Record<string, unknown> = {}) {
	return {
		id: "notif-1",
		userId: USER_ID,
		organizationId: "org-1",
		code: NotificationCode.DocumentReviewRequest,
		metadata: { userId: USER_ID, documentId: DOC_ID, branchId: "branch-1" },
		read: false,
		createdAt: new Date(2026, 2, 14, 11, 0, 0),
		...overrides,
	}
}

// seeds the pages the box has loaded so far, out of pageCount on the server
function seedPages(pages: unknown[][], pageCount = pages.length, unread = 0) {
	// a plain seed leaves the entry without its paging state, and the
	// box's query cannot mount on it
	runInApp(() => {
		setInfiniteQueryData(
			useQueryCache(),
			["notifications", "list", 50, "all"],
			{
				pages: pages.map((notifications) => ({
					notifications: notifications,
					pageCount: pageCount,
				})),
				pageParams: pages.map((_page, index) => index + 1),
			},
		)
	})
	seedQueryData(["notifications", "count", false], { count: unread })
}

function seedNotifications(notifications: unknown[], unread = 0) {
	seedPages([notifications], 1, unread)
}

function seedTree() {
	seedQueryData(
		["documents", "tree"],
		[
			{
				id: DOC_ID,
				documentName: "Runbook",
				icon: "mingcute:book-2-line",
				protected: false,
				children: null,
			},
		],
	)
}

function mountBox() {
	return mountSuspended(NotificationBox)
}

function rows(wrapper: Awaited<ReturnType<typeof mountBox>>) {
	return wrapper.findAll("[role='link']")
}

function notificationIds(wrapper: Awaited<ReturnType<typeof mountBox>>) {
	return wrapper
		.findAllComponents(NotificationRow)
		.map((row) => row.props("notification").id)
}
