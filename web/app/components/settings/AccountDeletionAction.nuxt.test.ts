import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { setResponseStatus } from "h3"
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
	mountUnderDialogRoot,
	raisedToasts,
	seedAuthAccounts,
	seedAuthConfig,
	seedAuthOrganization,
	settleActionSubmit,
	settleMutations,
	t,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const navigateToMock = vi.hoisted(() => vi.fn())
mockNuxtImport("navigateTo", () => navigateToMock)

function mountAction() {
	return mountUnderDialogRoot(AccountDeletionAction)
}

async function confirmDeletion(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
) {
	// the confirm button submits the form, which is what lets Enter in the
	// password field delete the account. happy-dom does not raise a submit
	// event from a click on a submit button, so the form is submitted here
	await wrapper.get("form").trigger("submit")
	await settleActionSubmit()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the deletion is driven by the global
// fake timers
describe("<AccountDeletionAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		navigateToMock.mockReset()
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

		await wrapper.get("form").trigger("submit")
		// vee-validate's scheduler needs a turn of the clock before the
		// submit handler runs; 100ms leaves the spinner's own delay(300)
		// pending, which is the state under test
		await vi.advanceTimersByTimeAsync(100)
		await settleMutations()

		expect(
			findButtonByText(
				wrapper,
				t("settings.action-modals.account-deletion.confirm-button"),
			).attributes("disabled"),
		).toBeDefined()
		expect(
			findButtonByText(
				wrapper,
				t("settings.action-modals.account-deletion.cancel-button"),
			).attributes("disabled"),
		).toBeDefined()
	})

	// a submit button is what makes Enter in the password field confirm the
	// deletion, the way every other dialog in settings behaves
	it("confirms through a submit button rather than a click handler", async ({
		expect,
	}) => {
		const wrapper = await mountAction()

		expect(
			findButtonByText(
				wrapper,
				t("settings.action-modals.account-deletion.confirm-button"),
			).attributes("type"),
		).toBe("submit")
	})

	it("warns and stays open when the deletion is refused", async ({
		expect,
	}) => {
		mockAuthEndpoint("delete-user", () => {
			throw createError({ statusCode: 500 })
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

		await findButtonByText(
			wrapper,
			t("settings.action-modals.account-deletion.cancel-button"),
		).trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	describe("when the server sends no email", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthConfig({ emailEnabled: false })
			seedAuthAccounts(["credential"])
		})

		it("asks for the current password instead of promising an email", async ({
			expect,
		}) => {
			const wrapper = await mountAction()

			expect(wrapper.find("input[type='password']").exists()).toBe(true)
			expect(wrapper.text()).toContain(
				t("settings.action-modals.account-deletion.description-without-email"),
			)
		})

		it("deletes the account with the password and lands on signup", async ({
			expect,
		}) => {
			const calls = mockAuthEndpoint("delete-user", () => ({
				success: true,
				message: "User deleted",
			}))
			const wrapper = await mountAction()
			await wrapper.get("input[type='password']").setValue("correct-horse-1!")

			await confirmDeletion(wrapper)

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toMatchObject({ password: "correct-horse-1!" })
			expect(navigateToMock).toHaveBeenCalledTimes(1)
			expect(navigateToMock).toHaveBeenCalledWith({
				path: "/signup",
				query: { deletion: "success" },
			})
			expect(toast.custom).not.toHaveBeenCalled()
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("asks for the password before deleting anything", async ({ expect }) => {
			const calls = mockAuthEndpoint("delete-user", () => ({
				success: true,
			}))
			const wrapper = await mountAction()

			await confirmDeletion(wrapper)

			expect(wrapper.text()).toContain(
				t("settings.action-modals.account-deletion.errors.password-required"),
			)
			expect(calls).toHaveLength(0)
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("shows a wrong password under the field and stays open", async ({
			expect,
		}) => {
			mockAuthEndpoint("delete-user", (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "INVALID_PASSWORD", message: "Invalid password" }
			})
			const wrapper = await mountAction()
			await wrapper.get("input[type='password']").setValue("wrong-password")

			await confirmDeletion(wrapper)

			expect(wrapper.text()).toContain(
				t("settings.action-modals.account-deletion.errors.invalid-password"),
			)
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("explains why an account without a password cannot be deleted", async ({
			expect,
		}) => {
			seedAuthAccounts(["github"])
			mockAuthEndpoint("delete-user", (_call, event) => {
				setResponseStatus(event, 403)

				return {
					code: "CREDENTIAL_ACCOUNT_NOT_FOUND",
					message: "Credential account not found",
				}
			})
			const wrapper = await mountAction()

			await confirmDeletion(wrapper)

			expect(wrapper.find("input[type='password']").exists()).toBe(false)
			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					description: t(
						"settings.action-modals.account-deletion.errors.no-password.description",
					),
				},
			])
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})
	describe("in a single-workspace instance", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthConfig({ singleOrganization: true })
		})

		it("offers the last member only a way to close the modal", async ({
			expect,
		}) => {
			seedAuthOrganization({ members: [{ id: "m1" }] })

			const wrapper = await mountAction()

			expect(wrapper.text()).toContain(
				t(
					"settings.action-modals.account-deletion.description-last-member-kept",
				),
			)
			expect(wrapper.text()).not.toContain(
				t(
					"settings.action-modals.account-deletion.description-last-org-member",
				),
			)
			expect(wrapper.findAll("button").map((b) => b.text())).toEqual([
				t("settings.action-modals.account-deletion.close-button"),
			])
		})

		it("deletes nothing when the last member's form is submitted anyway", async ({
			expect,
		}) => {
			seedAuthOrganization({ members: [{ id: "m1" }] })
			const calls = mockAuthEndpoint("delete-user", () => ({
				success: true,
			}))
			const wrapper = await mountAction()

			await confirmDeletion(wrapper)

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("asks the last member for no password when the server sends no email", async ({
			expect,
		}) => {
			seedAuthConfig({ singleOrganization: true, emailEnabled: false })
			seedAuthAccounts(["credential"])
			seedAuthOrganization({ members: [{ id: "m1" }] })

			const wrapper = await mountAction()

			expect(wrapper.find("input[type='password']").exists()).toBe(false)
			expect(wrapper.findAll("button").map((b) => b.text())).toEqual([
				t("settings.action-modals.account-deletion.close-button"),
			])
		})

		it("lets a member of a shared workspace delete their account", async ({
			expect,
		}) => {
			seedAuthOrganization({ members: [{ id: "m1" }, { id: "m2" }] })
			const calls = mockAuthEndpoint("delete-user", () => ({
				success: true,
			}))
			const wrapper = await mountAction()

			await confirmDeletion(wrapper)

			expect(wrapper.text()).not.toContain(
				t(
					"settings.action-modals.account-deletion.description-last-member-kept",
				),
			)
			expect(calls).toHaveLength(1)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("explains a refusal for the last member and stays open", async ({
			expect,
		}) => {
			seedAuthOrganization({ members: [{ id: "m1" }, { id: "m2" }] })
			mockAuthEndpoint("delete-user", (_call, event) => {
				setResponseStatus(event, 403)

				return {
					code: "LAST_ORGANIZATION_MEMBER",
					message: "The last member of the organization cannot be deleted",
				}
			})
			const wrapper = await mountAction()

			await confirmDeletion(wrapper)

			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					description: t(
						"settings.action-modals.account-deletion.errors.last-member.description",
					),
				},
			])
			expect(navigateToMock).not.toHaveBeenCalled()
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})
})
