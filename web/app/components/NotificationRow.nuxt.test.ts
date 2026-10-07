import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import NotificationRow from "./NotificationRow.vue"
import { findButtonByText, renderedIconNames, t } from "./test-helpers"

const NOW = new Date("2026-03-14T12:00:00Z")
const DOCUMENT = {
	id: "doc-1",
	documentName: "Runbook",
	icon: "mingcute:book-2-line",
	protected: false,
	children: null,
}
const ACTOR = { id: "user-1", name: "Ada", image: null }
const COMMENT_METADATA = {
	userId: "user-1",
	documentId: "doc-1",
	branchId: "branch-1",
	commentId: "comment-1",
	anchorBlockId: null,
}
const HOOK_METADATA = {
	documentId: "doc-1",
	branchId: "branch-1",
	blockId: null,
	type: DocumentHookType.URLWatcher,
}

describe("<NotificationRow>", () => {
	it("names the page the notification is about", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(wrapper.text()).toContain("Runbook")
	})

	it("shows the page's own icon", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(renderedIconNames(wrapper)).toContain("mingcute:book-2-line")
	})

	it("falls back to a placeholder name for a deleted page", async ({
		expect,
	}) => {
		const wrapper = await mountRow({}, { document: null })

		expect(wrapper.text()).toContain(t("notification.document-fallback"))
	})

	it("shows how long ago the notification arrived", async ({ expect }) => {
		const wrapper = await mountRow({ createdAt: "2026-03-14T10:00:00Z" })

		expect(wrapper.text()).toContain(
			t("general.relative-time.hours", { count: 2 }),
		)
	})

	it("marks an unread notification with a dot and a bold title", async ({
		expect,
	}) => {
		const wrapper = await mountRow({ read: false })

		expect(wrapper.find(".size-1\\.5").exists()).toBe(true)
		expect(wrapper.find(".font-semibold").text()).toBe("Runbook")
	})

	it("mutes a notification that has been read", async ({ expect }) => {
		const wrapper = await mountRow({ read: true })

		expect(wrapper.find(".size-1\\.5").exists()).toBe(false)
		expect(wrapper.find(".font-semibold").exists()).toBe(false)
		expect(wrapper.findAll("button")).toHaveLength(0)
	})

	it("shows the actor's picture when they have one", async ({ expect }) => {
		const wrapper = await mountRow(
			{},
			{ actor: { ...ACTOR, image: "https://example.com/ada.png" } },
		)

		expect(wrapper.find("img").attributes("src")).toBe(
			"https://example.com/ada.png",
		)
	})

	it.for([
		{
			input: DocumentHookType.URLWatcher,
			expected: "mingcute:earth-2-line",
		},
		{
			input: DocumentHookType.GitHubTracking,
			expected: "simple-icons:github",
		},
		{
			input: DocumentHookType.ScheduledReminder,
			expected: "mingcute:stopwatch-line",
		},
		{
			input: DocumentHookType.ContainerImageWatcher,
			expected: "simple-icons:docker",
		},
		{
			input: "future-hook",
			expected: "mingcute:notification-line",
		},
	])(
		"shows the $input hook's icon in the circle",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow(
				{
					code: NotificationCode.DocumentHookTriggered,
					metadata: { ...HOOK_METADATA, type: input },
				},
				{ actor: null },
			)

			expect(renderedIconNames(wrapper)).toContain(expected)
		},
	)

	it.for([
		{
			input: NotificationCode.DocumentNewComment,
			expected: "mingcute:message-4-fill",
		},
		{
			input: NotificationCode.DocumentNewCommentReply,
			expected: "mingcute:back-2-fill",
		},
		{
			input: NotificationCode.DocumentCommentResolved,
			expected: "mingcute:check-fill",
		},
		{
			input: NotificationCode.DocumentReviewRequest,
			expected: "mingcute:eye-2-fill",
		},
		{
			input: NotificationCode.DocumentHookTriggered,
			expected: "mingcute:flash-fill",
		},
		{
			input: NotificationCode.DocumentHookNeedsAttention,
			expected: "ph:exclamation-mark-bold",
		},
	])(
		"badges $input with $expected",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow({
				code: input,
				metadata: { ...COMMENT_METADATA, ...HOOK_METADATA },
			})

			expect(renderedIconNames(wrapper)).toContain(expected)
		},
	)

	it.for([
		{
			input: NotificationCode.DocumentNewComment,
			expected: "notification.messages.document-new-comment-description",
		},
		{
			input: NotificationCode.DocumentNewCommentReply,
			expected: "notification.messages.document-new-comment-reply-description",
		},
		{
			input: NotificationCode.DocumentCommentResolved,
			expected: "notification.messages.document-comment-resolved-description",
		},
		{
			input: NotificationCode.DocumentReviewRequest,
			expected: "notification.messages.document-review-request-description",
		},
	])(
		"says what the actor did for $input",
		async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow({ code: input })

			expect(wrapper.text()).toContain(t(expected, { user: "Ada" }))
		},
	)

	it("follows a comment with its excerpt", async ({ expect }) => {
		const wrapper = await mountRow({
			metadata: { ...COMMENT_METADATA, commentExcerpt: "Worth a second look" },
		})

		expect(wrapper.text()).toContain(t("notification.messages.separator"))
		expect(wrapper.text()).toContain("Worth a second look")
	})

	it("leaves the separator out without an excerpt", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(wrapper.text()).not.toContain(t("notification.messages.separator"))
	})

	it("says what a hook found and what it watches", async ({ expect }) => {
		const wrapper = await mountRow(
			{
				code: NotificationCode.DocumentHookTriggered,
				metadata: {
					...HOOK_METADATA,
					hookSettings: { url: "https://nordpost.io/status" },
				},
			},
			{ actor: null },
		)

		expect(wrapper.text()).toContain(
			t("notification.messages.hook-triggered.url-watcher"),
		)
		expect(wrapper.text()).toContain("nordpost.io/status")
	})

	it("falls back to a generic line for a code it does not know", async ({
		expect,
	}) => {
		const wrapper = await mountRow(
			{ code: "notification.unknown" },
			{ actor: null },
		)

		expect(wrapper.text()).toContain(
			t("notification.messages.default-description"),
		)
		expect(renderedIconNames(wrapper)).toContain("mingcute:notification-line")
	})

	it("asks to be opened when clicked", async ({ expect }) => {
		const wrapper = await mountRow()

		await wrapper.find("[role='link']").trigger("click")

		expect(wrapper.emitted("open")).toHaveLength(1)
		expect(wrapper.emitted("mark-read")).toBeUndefined()
	})

	it("asks to be opened on enter", async ({ expect }) => {
		const wrapper = await mountRow()

		await wrapper.find("[role='link']").trigger("keydown.enter")

		expect(wrapper.emitted("open")).toHaveLength(1)
	})

	it("asks to be marked read without opening", async ({ expect }) => {
		const wrapper = await mountRow()

		await findButtonByText(
			wrapper,
			t("notification.actions.mark-read"),
		).trigger("click")

		expect(wrapper.emitted("mark-read")).toHaveLength(1)
		expect(wrapper.emitted("open")).toBeUndefined()
	})
})

function mountRow(
	notification: Record<string, unknown> = {},
	props: Record<string, unknown> = {},
) {
	return mountSuspended(NotificationRow, {
		props: {
			notification: {
				id: "notif-1",
				userId: "user-9",
				organizationId: "org-1",
				code: NotificationCode.DocumentNewComment,
				metadata: COMMENT_METADATA,
				read: false,
				createdAt: "2026-03-14T11:00:00Z",
				...notification,
			},
			document: DOCUMENT,
			actor: ACTOR,
			now: NOW,
			...props,
		},
	})
}
