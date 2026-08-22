import type { VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ReviewerList from "./ReviewerList.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	menuItem,
	mountUnderTooltipProvider,
	seedAuthOrganization,
	seedAuthSession,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import type WsState from "~/utils/websocket"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("brancha")
const TARGET_BRANCH_ID = makeXid("branchb")

const ME = makeXid("usme")
const ADA = makeXid("usada")
const GRACE = makeXid("usgrc")
const ALAN = makeXid("usaln")

function member(id: string, name: string) {
	return {
		userId: id,
		user: { name: name, email: `${name.toLowerCase()}@test`, image: undefined },
	}
}

function seedMembers(...members: ReturnType<typeof member>[]) {
	seedAuthOrganization({ members: members })
}

function seedReviewers(
	branchId: string,
	reviewers: { userId: string; currentlyApproved: boolean }[],
) {
	seedQueryData(
		["documents", DOCUMENT_ID, "branches", branchId, "reviewers"],
		reviewers,
	)
}

function mountList() {
	return mountUnderTooltipProvider(ReviewerList, {})
}

async function openPopover(wrapper: VueWrapper) {
	await wrapper.get("[data-slot='popover-trigger']").trigger("click")
	await nextTick()
}

function popoverText(): string {
	return (
		document.body.querySelector("[data-slot='popover-content']")?.textContent ??
		""
	)
}

function popoverButton(hint: string): HTMLButtonElement {
	const button = Array.from(
		document.body.querySelectorAll<HTMLButtonElement>(
			"[data-slot='popover-content'] button",
		),
	).find((candidate) => candidate.textContent.includes(hint))
	if (!button) {
		throw new Error(`no popover button rendering "${hint}"`)
	}

	return button
}

// the editor store, the auth cache, the query cache, the mocked toast
// module and the teleported overlays are all shared, so these tests
// cannot interleave
describe("<ReviewerList>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().updateTargetBranchId(TARGET_BRANCH_ID)
		useEditorStore().setBranchReviewableActionsActive(false)
		useWebSocketStateStore().state = null
		seedAuthSession({ id: ME, name: "Me" })
	})

	afterEach(disposeMockEndpoints)

	it("shows nothing while the branch has no reviewers and no actions", async ({
		expect,
	}) => {
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [])
		seedReviewers(TARGET_BRANCH_ID, [])

		const wrapper = await mountList()

		expect(wrapper.text()).toBe("")
	})

	it("appears once the branch can be reviewed", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [])
		seedReviewers(TARGET_BRANCH_ID, [])

		const wrapper = await mountList()

		expect(wrapper.text()).toContain(t("editor.name-editor.reviewers"))
	})

	it("shows the reviewers taking part", async ({ expect }) => {
		seedMembers(member(ADA, "Ada"), member(GRACE, "Grace"))
		seedReviewers(BRANCH_ID, [
			{ userId: ADA, currentlyApproved: true },
			{ userId: GRACE, currentlyApproved: false },
		])
		seedReviewers(TARGET_BRANCH_ID, [])

		const wrapper = await mountList()

		expect(wrapper.findAll("li")).toHaveLength(2)
	})

	it("leaves out reviewers who are no longer members", async ({ expect }) => {
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [
			{ userId: ADA, currentlyApproved: true },
			{ userId: GRACE, currentlyApproved: false },
		])
		seedReviewers(TARGET_BRANCH_ID, [])

		const wrapper = await mountList()

		expect(wrapper.findAll("li")).toHaveLength(1)
	})

	it("separates the reviewers who approved from those still invited", async ({
		expect,
	}) => {
		seedMembers(member(ADA, "Ada"), member(GRACE, "Grace"))
		seedReviewers(BRANCH_ID, [
			{ userId: ADA, currentlyApproved: true },
			{ userId: GRACE, currentlyApproved: false },
		])
		seedReviewers(TARGET_BRANCH_ID, [])
		const wrapper = await mountList()

		await openPopover(wrapper)

		expect(popoverText()).toContain(
			t("editor.name-editor.reviewer-popover.approved-by-label"),
		)
		expect(popoverText()).toContain("Ada")
		expect(popoverText()).toContain(
			t("editor.name-editor.reviewer-popover.invited-label"),
		)
		expect(popoverText()).toContain("Grace")
	})

	it("offers no per-reviewer actions while the branch cannot be reviewed", async ({
		expect,
	}) => {
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: true }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const wrapper = await mountList()

		await openPopover(wrapper)

		expect(
			document.body.querySelectorAll("[data-slot='popover-content'] button"),
		).toHaveLength(0)
	})

	it("asks an approver to review again", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: true }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			() => null,
		)
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t(
				"editor.name-editor.reviewer-popover.request-review-screen-reader-hint",
			),
		).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({ userId: ADA })
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("warns when the review request fails", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: true }])
		seedReviewers(TARGET_BRANCH_ID, [])
		mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t(
				"editor.name-editor.reviewer-popover.request-review-screen-reader-hint",
			),
		).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("withdraws an invitation", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(GRACE, "Grace"))
		seedReviewers(BRANCH_ID, [{ userId: GRACE, currentlyApproved: false }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			() => null,
		)
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t("editor.name-editor.reviewer-popover.invite-remove-screen-reader-hint"),
		).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.query).toEqual({ userId: GRACE })
	})

	it("warns when the invitation cannot be withdrawn", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(GRACE, "Grace"))
		seedReviewers(BRANCH_ID, [{ userId: GRACE, currentlyApproved: false }])
		seedReviewers(TARGET_BRANCH_ID, [])
		mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t("editor.name-editor.reviewer-popover.invite-remove-screen-reader-hint"),
		).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("suggests the reviewers of the branch being merged into", async ({
		expect,
	}) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"), member(GRACE, "Grace"), member(ME, "Me"))
		seedReviewers(BRANCH_ID, [])
		seedReviewers(TARGET_BRANCH_ID, [
			{ userId: ADA, currentlyApproved: true },
			{ userId: ME, currentlyApproved: true },
		])
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t("editor.name-editor.reviewer-popover.invite-button"),
		).click()
		await nextTick()

		expect(menuItem("Ada")).toBeDefined()
		expect(document.body.textContent).toContain(
			t("editor.name-editor.reviewer-popover.suggestions-label"),
		)
	})

	it("offers every other member as an invitee", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"), member(ALAN, "Alan"), member(ME, "Me"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: false }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const wrapper = await mountList()
		await openPopover(wrapper)

		popoverButton(
			t("editor.name-editor.reviewer-popover.invite-button"),
		).click()
		await nextTick()

		expect(menuItem("Alan")).toBeDefined()
		expect(document.body.textContent).not.toContain(
			t("editor.name-editor.reviewer-popover.suggestions-label"),
		)
	})

	it("invites the member the reader picked", async ({ expect }) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ALAN, "Alan"))
		seedReviewers(BRANCH_ID, [])
		seedReviewers(TARGET_BRANCH_ID, [])
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			() => null,
		)
		const wrapper = await mountList()
		await openPopover(wrapper)
		popoverButton(
			t("editor.name-editor.reviewer-popover.invite-button"),
		).click()
		await nextTick()

		menuItem("Alan").click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({ userId: ALAN })
	})

	it("offers nobody to invite when everyone is already a reviewer", async ({
		expect,
	}) => {
		useEditorStore().setBranchReviewableActionsActive(true)
		seedMembers(member(ADA, "Ada"), member(ME, "Me"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: false }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const wrapper = await mountList()

		await openPopover(wrapper)

		expect(
			popoverButton(t("editor.name-editor.reviewer-popover.invite-button"))
				.disabled,
		).toBe(true)
	})

	it("refetches the reviewers when the server says they changed", async ({
		expect,
	}) => {
		seedMembers(member(ADA, "Ada"))
		seedReviewers(BRANCH_ID, [{ userId: ADA, currentlyApproved: true }])
		seedReviewers(TARGET_BRANCH_ID, [])
		const calls = mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/reviewers`,
			() => [{ userId: ADA, currentlyApproved: true }],
		)
		const handlers: (() => void)[] = []
		const subscribe = vi.fn((_topic: string, handler: () => void) => {
			handlers.push(handler)

			return () => undefined
		})
		useWebSocketStateStore().state = {
			subscribe: subscribe,
		} as unknown as WsState
		await mountList()

		handlers.forEach((handler) => {
			handler()
		})

		await vi.waitFor(() => {
			expect(calls.length).toBeGreaterThan(0)
		}, WAIT_FOR_OPTIONS)
		expect(subscribe.mock.calls[0]?.[0]).toBe(
			makeWsDocumentReviewersChangeTopic(DOCUMENT_ID),
		)
	})

	it("subscribes to nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)
		seedMembers(member(ADA, "Ada"))
		const subscribe = vi.fn()
		useWebSocketStateStore().state = {
			subscribe: subscribe,
		} as unknown as WsState

		await mountList()

		expect(subscribe).toHaveBeenCalledTimes(0)
	})
})
