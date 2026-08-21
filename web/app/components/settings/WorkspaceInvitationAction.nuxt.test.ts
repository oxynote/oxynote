import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import WorkspaceInvitationAction from "./WorkspaceInvitationAction.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	mockAuthOrganization,
	mountWithFrozenClock,
	seedAuthSession,
	settleActionSubmit,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function member(id: string) {
	return { id: id, userId: id, user: { name: id, email: `${id}@test` } }
}

function seedWorkspace(memberCount: number, pendingInvitations = 0) {
	mockAuthOrganization({
		id: "org-1",
		name: "Acme Corp",
		slug: "acme-corp",
		members: Array.from({ length: memberCount }, (_, i) => member(`m${i}`)),
		invitations: Array.from({ length: pendingInvitations }, (_, i) => ({
			id: `inv${i}`,
			status: "pending",
			email: `inv${i}@test`,
			role: "member",
			organizationId: "org-1",
		})),
	})
}

function mountAction() {
	return mountWithFrozenClock(WorkspaceInvitationAction)
}

async function submitEmail(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
	email: string,
) {
	await wrapper.get("input").setValue(email)
	await wrapper.get("form").trigger("submit")
	await settleActionSubmit()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the submit flow is driven by the
// global fake timers
describe("<WorkspaceInvitationAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "Ada" })
	})

	afterEach(disposeMockEndpoints)

	it("invites the user to send an invitation", async ({ expect }) => {
		seedWorkspace(1)

		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("Send an invitation")
	})

	it("sends the invitation as an owner", async ({ expect }) => {
		seedWorkspace(1)
		const calls = mockAuthEndpoint("organization/invite-member", () => ({
			id: "inv-1",
		}))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "new@oxynote.test")

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({
			email: "new@oxynote.test",
			role: "owner",
			resend: true,
		})
	})

	it("confirms and closes once the invitation is sent", async ({ expect }) => {
		seedWorkspace(1)
		mockAuthEndpoint("organization/invite-member", () => ({ id: "inv-1" }))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "new@oxynote.test")

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("closes without inviting when the address is the user's own", async ({
		expect,
	}) => {
		seedWorkspace(1)
		const calls = mockAuthEndpoint("organization/invite-member", () => ({
			id: "inv-1",
		}))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "ada@oxynote.test")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("rejects an address that is not an email", async ({ expect }) => {
		seedWorkspace(1)
		const calls = mockAuthEndpoint("organization/invite-member", () => ({
			id: "inv-1",
		}))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "not-an-email")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("shows the server's rejection next to the field", async ({ expect }) => {
		seedWorkspace(1)
		mockAuthEndpoint("organization/invite-member", (_call, event) => {
			setResponseStatus(event, 400)

			return { code: "ALREADY_MEMBER", message: "Already a member" }
		})
		const wrapper = await mountAction()

		await submitEmail(wrapper, "new@oxynote.test")

		expect(wrapper.text()).toContain("Already a member")
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("counts pending invitations towards the member limit", async ({
		expect,
	}) => {
		seedWorkspace(3, 2)

		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("maximum number of members")
	})

	it("hides the email field once the workspace is full", async ({ expect }) => {
		seedWorkspace(5)

		const wrapper = await mountAction()

		expect(wrapper.find("input").exists()).toBe(false)
		expect(findButtonByText(wrapper, "Close").exists()).toBe(true)
	})

	it("keeps inviting available while there is room left", async ({
		expect,
	}) => {
		seedWorkspace(4)

		const wrapper = await mountAction()

		expect(wrapper.find("input").exists()).toBe(true)
		expect(wrapper.text()).not.toContain("maximum number of members")
	})

	it("keeps inviting available while the organization is unknown", async ({
		expect,
	}) => {
		const wrapper = await mountAction()

		expect(wrapper.find("input").exists()).toBe(true)
	})

	it("closes without inviting anyone when cancelled", async ({ expect }) => {
		seedWorkspace(1)
		const calls = mockAuthEndpoint("organization/invite-member", () => ({
			id: "inv-1",
		}))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Cancel").trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
