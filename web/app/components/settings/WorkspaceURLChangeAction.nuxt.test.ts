import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import WorkspaceURLChangeAction from "./WorkspaceURLChangeAction.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	mockAuthOrganization,
	mountWithFrozenClock,
	settleActionSubmit,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function mountAction() {
	return mountWithFrozenClock(WorkspaceURLChangeAction)
}

async function submitUrl(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
	url: string,
) {
	await wrapper.get("input").setValue(url)
	await wrapper.get("form").trigger("submit")
	await settleActionSubmit()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the submit flow is driven by the
// global fake timers
describe("<WorkspaceURLChangeAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		mockAuthOrganization({
			id: "org-1",
			name: "Acme Corp",
			slug: "acme-corp",
			members: [],
			invitations: [],
		})
	})

	afterEach(disposeMockEndpoints)

	it("explains what changing the url does", async ({ expect }) => {
		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("replace all your workspace URLs")
	})

	it("suggests the current slug as the placeholder", async ({ expect }) => {
		const wrapper = await mountAction()

		expect(wrapper.get("input").attributes("placeholder")).toBe("acme-corp")
	})

	it("checks the new slug before claiming it", async ({ expect }) => {
		const checks = mockAuthEndpoint("organization/check-slug", () => ({
			status: true,
		}))
		mockAuthEndpoint("organization/update", () => ({ id: "org-1" }))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(checks).toHaveLength(1)
		expect(checks[0]?.body).toEqual({ slug: "acme-eu" })
	})

	it("claims the new slug for the workspace", async ({ expect }) => {
		mockAuthEndpoint("organization/check-slug", () => ({ status: true }))
		const updates = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(updates).toHaveLength(1)
		expect(updates[0]?.body).toEqual({
			data: { slug: "acme-eu" },
			organizationId: "org-1",
		})
	})

	it("confirms the change and asks the page to refresh its slug", async ({
		expect,
	}) => {
		mockAuthEndpoint("organization/check-slug", () => ({ status: true }))
		mockAuthEndpoint("organization/update", () => ({ id: "org-1" }))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
		expect(wrapper.emitted("refresh-organization-slug")).toHaveLength(1)
	})

	it("closes without asking the server when the url is unchanged", async ({
		expect,
	}) => {
		const checks = mockAuthEndpoint("organization/check-slug", () => ({
			status: true,
		}))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-corp")

		expect(checks).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("rejects a url with characters outside the allowed set", async ({
		expect,
	}) => {
		const checks = mockAuthEndpoint("organization/check-slug", () => ({
			status: true,
		}))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme corp!")

		expect(checks).toHaveLength(0)
		expect(wrapper.text()).toContain("Only letters, numbers, hyphens")
	})

	it("rejects a url shorter than two characters", async ({ expect }) => {
		const checks = mockAuthEndpoint("organization/check-slug", () => ({
			status: true,
		}))
		const wrapper = await mountAction()

		await submitUrl(wrapper, "a")

		expect(checks).toHaveLength(0)
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("reports a slug that someone else already holds", async ({ expect }) => {
		mockAuthEndpoint("organization/check-slug", (_call, event) => {
			setResponseStatus(event, 400)

			return { code: "SLUG_TAKEN", message: "taken" }
		})
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(wrapper.text()).toContain("already taken")
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("reports a slug the server refuses for any other reason", async ({
		expect,
	}) => {
		mockAuthEndpoint("organization/check-slug", (_call, event) => {
			setResponseStatus(event, 500)

			return { code: "BOOM", message: "boom" }
		})
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(wrapper.text()).toContain("is invalid")
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("shows the server's rejection of the update next to the field", async ({
		expect,
	}) => {
		mockAuthEndpoint("organization/check-slug", () => ({ status: true }))
		mockAuthEndpoint("organization/update", (_call, event) => {
			setResponseStatus(event, 403)

			return { code: "FORBIDDEN", message: "Not allowed" }
		})
		const wrapper = await mountAction()

		await submitUrl(wrapper, "acme-eu")

		expect(wrapper.text()).toContain("Not allowed")
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("closes without changing anything when cancelled", async ({ expect }) => {
		const checks = mockAuthEndpoint("organization/check-slug", () => ({
			status: true,
		}))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Cancel").trigger("click")

		expect(checks).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
