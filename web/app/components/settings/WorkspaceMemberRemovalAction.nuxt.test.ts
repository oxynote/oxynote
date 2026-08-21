import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import WorkspaceMemberRemovalAction from "./WorkspaceMemberRemovalAction.vue"
import type { OrganizationMember } from "./workspace"
import {
	findButtonByText,
	mockAuthEndpoint,
	mountWithFrozenClock,
	mockAuthOrganization,
	settleActionSubmit,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function makeMember(overrides: Partial<OrganizationMember> = {}) {
	return {
		id: "member-1",
		organizationId: "org-1",
		userId: "user-1",
		role: "member",
		user: { name: "Ada", email: "ada@oxynote.test" },
		...overrides,
	}
}

function mountAction(member: OrganizationMember) {
	return mountWithFrozenClock(WorkspaceMemberRemovalAction, {
		props: { member: member },
	})
}

async function confirmRemoval(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
) {
	await findButtonByText(wrapper, "Remove Member").trigger("click")
	await settleActionSubmit()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the removal is driven by the global
// fake timers
describe("<WorkspaceMemberRemovalAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		mockAuthOrganization({ id: "org-1", members: [], invitations: [] })
	})

	afterEach(disposeMockEndpoints)

	it("names the member it is about to remove", async ({ expect }) => {
		const wrapper = await mountAction(makeMember())

		expect(wrapper.text()).toContain("Ada")
	})

	it("removes a joined member", async ({ expect }) => {
		const calls = mockAuthEndpoint("organization/remove-member", () => ({
			member: { id: "member-1" },
		}))
		const wrapper = await mountAction(makeMember())

		await confirmRemoval(wrapper)

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({ memberIdOrEmail: "member-1" })
	})

	it("cancels a pending invitation instead of removing a member", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("organization/cancel-invitation", () => ({
			id: "member-1",
		}))
		const wrapper = await mountAction(makeMember({ invitationPending: true }))

		await confirmRemoval(wrapper)

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({ invitationId: "member-1" })
	})

	it("confirms and closes once the member is gone", async ({ expect }) => {
		mockAuthEndpoint("organization/remove-member", () => ({
			member: { id: "member-1" },
		}))
		const wrapper = await mountAction(makeMember())

		await confirmRemoval(wrapper)

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("warns and closes when removing the member fails", async ({ expect }) => {
		mockAuthEndpoint("organization/remove-member", () => {
			throw new Error("boom")
		})
		const wrapper = await mountAction(makeMember())

		await confirmRemoval(wrapper)

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("warns and closes when cancelling the invitation fails", async ({
		expect,
	}) => {
		mockAuthEndpoint("organization/cancel-invitation", () => {
			throw new Error("boom")
		})
		const wrapper = await mountAction(makeMember({ invitationPending: true }))

		await confirmRemoval(wrapper)

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("shows a spinner and disables both buttons while removing", async ({
		expect,
	}) => {
		mockAuthEndpoint("organization/remove-member", () => ({
			member: { id: "member-1" },
		}))
		const wrapper = await mountAction(makeMember())

		await findButtonByText(wrapper, "Remove Member").trigger("click")
		await nextTick()

		expect(
			findButtonByText(wrapper, "Remove Member").attributes("disabled"),
		).toBeDefined()
		expect(
			findButtonByText(wrapper, "Cancel").attributes("disabled"),
		).toBeDefined()
	})

	it("closes without removing anyone when cancelled", async ({ expect }) => {
		const calls = mockAuthEndpoint("organization/remove-member", () => ({
			member: { id: "member-1" },
		}))
		const wrapper = await mountAction(makeMember())

		await findButtonByText(wrapper, "Cancel").trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
