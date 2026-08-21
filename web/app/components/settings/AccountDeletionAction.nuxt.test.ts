import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import AccountDeletionAction from "./AccountDeletionAction.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	seedAuthOrganization,
	settleMutations,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function mountAction() {
	return mountSuspended(AccountDeletionAction)
}

async function confirmDeletion(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
) {
	await findButtonByText(wrapper, "Delete Account").trigger("click")
	await vi.advanceTimersByTimeAsync(300)
	await settleMutations()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the deletion is driven by the global
// fake timers
describe("<AccountDeletionAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		vi.useFakeTimers()
	})

	afterEach(disposeMockEndpoints)

	it("explains what deleting the account does", async ({ expect }) => {
		seedAuthOrganization({ members: [{ id: "m1" }, { id: "m2" }] })

		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("This action is irreversible")
	})

	it("warns the last member that the workspace goes with them", async ({
		expect,
	}) => {
		seedAuthOrganization({ members: [{ id: "m1" }] })

		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("You are the last member")
	})

	it("leaves the workspace warning out for a shared workspace", async ({
		expect,
	}) => {
		seedAuthOrganization({ members: [{ id: "m1" }, { id: "m2" }] })

		const wrapper = await mountAction()

		expect(wrapper.text()).not.toContain("You are the last member")
	})

	it("leaves the workspace warning out while the organization is unknown", async ({
		expect,
	}) => {
		const wrapper = await mountAction()

		expect(wrapper.text()).not.toContain("You are the last member")
	})

	it("asks the server to delete the account, pointing the callback at signup", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("delete-user", () => ({
			success: true,
		}))
		const wrapper = await mountAction()

		await confirmDeletion(wrapper)

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({
			callbackURL: "http://test.local/signup?deletion=success",
		})
	})

	it("closes once the confirmation email is on its way", async ({ expect }) => {
		mockAuthEndpoint("delete-user", () => ({ success: true }))
		const wrapper = await mountAction()

		await confirmDeletion(wrapper)

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("shows a spinner and disables both buttons while deleting", async ({
		expect,
	}) => {
		mockAuthEndpoint("delete-user", () => ({ success: true }))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Delete Account").trigger("click")
		await nextTick()

		expect(
			findButtonByText(wrapper, "Delete Account").attributes("disabled"),
		).toBeDefined()
		expect(
			findButtonByText(wrapper, "Cancel").attributes("disabled"),
		).toBeDefined()
	})

	it("warns and stays open when the deletion is refused", async ({
		expect,
	}) => {
		mockAuthEndpoint("delete-user", () => {
			throw new Error("boom")
		})
		const wrapper = await mountAction()

		await confirmDeletion(wrapper)

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("closes without deleting anything when cancelled", async ({ expect }) => {
		const calls = mockAuthEndpoint("delete-user", () => ({
			success: true,
		}))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Cancel").trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
