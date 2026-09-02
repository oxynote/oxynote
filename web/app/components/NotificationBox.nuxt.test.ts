import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import NotificationBox from "./NotificationBox.vue"
import {
	at,
	findButtonByText,
	mockAuthOrganization,
	mountUnderTooltipProvider,
	openTooltipText,
	renderedIconNames,
	seedAuthOrganization,
	t,
} from "./test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// opening a notification navigates; the destination it picks is the whole
// behaviour, and the test app has no page to land on
const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

const TOOLTIP_DELAY_MS = 600
const DOC_ID = "doc1".padEnd(20, "0")
const USER_ID = "user".padEnd(20, "0")

function makeNotification(overrides: Record<string, unknown> = {}) {
	return {
		id: "notif-1",
		userId: USER_ID,
		organizationId: "org-1",
		code: NotificationCode.DocumentReviewRequest,
		metadata: { documentId: DOC_ID, branchId: "branch-1" },
		read: false,
		createdAt: "2026-03-14T11:00:00Z",
		...overrides,
	}
}

function seedNotifications(notifications: unknown[]) {
	seedQueryData(["notifications", "list", 100, 1], {
		notifications: notifications,
		pageCount: 1,
	})
	seedQueryData(["notifications", "count", false], { count: 0 })
}

function seedTree() {
	seedQueryData(
		["documents", "tree"],
		[
			{
				id: DOC_ID,
				documentName: "Runbook",
				icon: "lucide:file",
				protected: false,
				children: null,
			},
		],
	)
}

function mountBox(props: Record<string, unknown> = {}) {
	return mountUnderTooltipProvider(NotificationBox, { props: props })
}

function rows(wrapper: Awaited<ReturnType<typeof mountBox>>) {
	return wrapper.findAll("div.cursor-pointer")
}

// the query cache, the pinia websocket store and the vue-sonner module
// mock are all app-wide singletons every mount in the file shares
describe("<NotificationBox>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
		vi.setSystemTime(new Date("2026-03-14T12:00:00Z"))
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

	it("names the document the notification is about", async ({ expect }) => {
		seedTree()
		seedNotifications([makeNotification()])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain("Runbook")
	})

	it("spells out the document name in a tooltip once hovered", async ({
		expect,
	}) => {
		vi.useFakeTimers()
		seedTree()
		seedNotifications([makeNotification()])

		const wrapper = await mountBox()
		await at(wrapper.findAll("[data-slot='tooltip-trigger']"), 0).trigger(
			"pointermove",
		)
		await vi.advanceTimersByTimeAsync(TOOLTIP_DELAY_MS)

		expect(openTooltipText(wrapper)).toBe("Runbook")
	})

	it("falls back to a placeholder name for a deleted document", async ({
		expect,
	}) => {
		seedNotifications([makeNotification()])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(t("notification.document-fallback"))
	})

	it("labels a notification from the last minute as new", async ({
		expect,
	}) => {
		seedNotifications([makeNotification({ createdAt: "2026-03-14T11:59:30Z" })])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain(t("notification.now-time-label"))
	})

	it("shows how long ago an older notification arrived", async ({ expect }) => {
		seedNotifications([makeNotification({ createdAt: "2026-03-14T11:00:00Z" })])

		const wrapper = await mountBox()

		expect(wrapper.text()).toContain("1 hour ago")
	})

	it("dims a notification that has been read", async ({ expect }) => {
		seedNotifications([makeNotification({ read: true })])

		const wrapper = await mountBox()

		expect(rows(wrapper)[0]?.classes()).toContain("opacity-60")
	})

	it("offers a mark-read button only on unread notifications", async ({
		expect,
	}) => {
		seedNotifications([
			makeNotification({ id: "notif-1", read: false }),
			makeNotification({ id: "notif-2", read: true }),
		])

		const wrapper = await mountBox()

		expect(wrapper.text().match(/Mark as read/g)).toHaveLength(1)
	})

	describe("descriptions", { concurrent: false }, () => {
		it("describes a review request", async ({ expect }) => {
			seedNotifications([makeNotification()])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain(
				t("notification.messages.document-review-request-description"),
			)
		})

		it("names the hook that fired", async ({ expect }) => {
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentHookTrigerred,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: null,
						type: DocumentHookType.URLWatcher,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain("hook was triggered")
		})

		it("names the commenter on a new comment", async ({ expect }) => {
			seedAuthOrganization({
				members: [{ userId: USER_ID, user: { name: "Ada" } }],
			})
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentNewComment,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						anchorBlockId: null,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain("Ada has posted a new comment")
		})

		it("names the commenter on a reply", async ({ expect }) => {
			seedAuthOrganization({
				members: [{ userId: USER_ID, user: { name: "Ada" } }],
			})
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentNewCommentReply,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						commentReplyId: "r",
						anchorBlockId: null,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain("Ada has posted a reply to a comment")
		})

		it("calls an unknown commenter deleted", async ({ expect }) => {
			seedAuthOrganization({ members: [] })
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentNewComment,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						anchorBlockId: null,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain("deleted has posted a new comment")
		})

		it("falls back to a generic description for an unknown code", async ({
			expect,
		}) => {
			seedNotifications([makeNotification({ code: "notification.unknown" })])

			const wrapper = await mountBox()

			expect(wrapper.text()).toContain(
				t("notification.messages.default-description"),
			)
		})
	})

	describe("icons", { concurrent: false }, () => {
		it("marks a review request with a reviewer icon", async ({ expect }) => {
			seedNotifications([makeNotification()])

			const wrapper = await mountBox()

			expect(renderedIconNames(wrapper)).toContain("lucide:file-user")
		})

		it.for([
			{ type: DocumentHookType.URLWatcher, expected: "mingcute:earth-2-line" },
			{
				type: DocumentHookType.GitHubTracking,
				expected: "simple-icons:github",
			},
			{ type: DocumentHookType.ScheduledReminder, expected: "lucide:timer" },
			{
				type: DocumentHookType.ContainerImageWatcher,
				expected: "lucide:container",
			},
		])(
			"marks a $type hook trigger with its own icon",
			async ({ type, expected }, { expect }) => {
				seedNotifications([
					makeNotification({
						code: NotificationCode.DocumentHookTrigerred,
						metadata: {
							documentId: DOC_ID,
							branchId: "b",
							blockId: null,
							type: type,
						},
					}),
				])

				const wrapper = await mountBox()

				expect(renderedIconNames(wrapper)).toContain(expected)
			},
		)

		it("marks a hook type this build does not know with a warning icon", async ({
			expect,
		}) => {
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentHookTrigerred,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: null,
						type: "future-hook" as DocumentHookType,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(renderedIconNames(wrapper)).toContain(
				"lucide:file-exclamation-point",
			)
		})

		it("marks a comment notification with a message icon", async ({
			expect,
		}) => {
			seedNotifications([
				makeNotification({
					code: NotificationCode.DocumentNewComment,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						anchorBlockId: null,
					},
				}),
			])

			const wrapper = await mountBox()

			expect(renderedIconNames(wrapper)).toContain("mingcute:message-4-fill")
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
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await findButtonByText(
				wrapper,
				t("notification.read-all-button"),
			).trigger("click")
			await flushPromises()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ ids: [] })
		})

		it("hides the button when every notification is already read", async ({
			expect,
		}) => {
			seedNotifications([makeNotification({ read: true })])

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
			seedNotifications([makeNotification()])
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
		it("announces the navigation when a notification is opened", async ({
			expect,
		}) => {
			mockEndpoint("PUT", "/api/notifications/read-status", () => ({}))
			seedAuthOrganization({ slug: "acme" })
			seedTree()
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(
				wrapper
					.findComponent(NotificationBox)
					.emitted("notification-navigation"),
			).toHaveLength(1)
		})

		it.for([
			{
				name: "opens a review request at the document",
				notification: {
					code: NotificationCode.DocumentReviewRequest,
					metadata: { documentId: DOC_ID, branchId: "b" },
				},
				expected: `/acme/Runbook-${DOC_ID}`,
			},
			{
				name: "opens a hook trigger at the block that fired",
				notification: {
					code: NotificationCode.DocumentHookTrigerred,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: "block-7",
						type: DocumentHookType.URLWatcher,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}#block-7`,
			},
			{
				name: "opens a document-wide hook trigger at the document",
				notification: {
					code: NotificationCode.DocumentHookTrigerred,
					metadata: {
						documentId: DOC_ID,
						branchId: "b",
						blockId: null,
						type: DocumentHookType.URLWatcher,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}`,
			},
			{
				name: "opens a new comment at the block it is anchored to",
				notification: {
					code: NotificationCode.DocumentNewComment,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						anchorBlockId: "block-3",
					},
				},
				expected: `/acme/Runbook-${DOC_ID}#block-3`,
			},
			{
				name: "opens an unanchored comment at the document",
				notification: {
					code: NotificationCode.DocumentNewComment,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						anchorBlockId: null,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}`,
			},
			{
				name: "opens a comment reply at the block it is anchored to",
				notification: {
					code: NotificationCode.DocumentNewCommentReply,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						commentReplyId: "r",
						anchorBlockId: "block-4",
					},
				},
				expected: `/acme/Runbook-${DOC_ID}#block-4`,
			},
			{
				name: "opens an unanchored comment reply at the document",
				notification: {
					code: NotificationCode.DocumentNewCommentReply,
					metadata: {
						userId: USER_ID,
						documentId: DOC_ID,
						branchId: "b",
						commentId: "c",
						commentReplyId: "r",
						anchorBlockId: null,
					},
				},
				expected: `/acme/Runbook-${DOC_ID}`,
			},
		])("$name", async ({ notification, expected }, { expect }) => {
			mockEndpoint("PUT", "/api/notifications/read-status", () => ({}))
			mockAuthOrganization({ id: "org-1", slug: "acme", members: [] })
			seedTree()
			seedNotifications([makeNotification(notification)])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(navigateToMock).toHaveBeenCalledExactlyOnceWith(expected)
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
			seedAuthOrganization({ slug: "acme" })
			seedNotifications([makeNotification()])
			const wrapper = await mountBox()

			await at(rows(wrapper), 0).trigger("click")
			await flushPromises()

			expect(
				wrapper
					.findComponent(NotificationBox)
					.emitted("notification-navigation"),
			).toBeUndefined()
		})
	})

	describe("mobile", { concurrent: false }, () => {
		it("hides the close button on wide viewports", async ({ expect }) => {
			seedNotifications([])

			const wrapper = await mountBox()

			expect(wrapper.text()).not.toContain(
				t("notification.actions.close-notification-box"),
			)
		})

		it("closes the box when its close button is pressed", async ({
			expect,
		}) => {
			seedNotifications([])
			const wrapper = await mountBox({ mobile: true })

			await findButtonByText(
				wrapper,
				t("notification.actions.close-notification-box"),
			).trigger("click")

			expect(
				wrapper
					.findComponent(NotificationBox)
					.emitted("close-notification-box"),
			).toHaveLength(1)
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
