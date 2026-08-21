import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import PasswordChangeAction from "./PasswordChangeAction.vue"
import {
	at,
	findButtonByText,
	mockAuthEndpoint,
	mountWithFrozenClock,
	seedAuthAccounts,
	seedAuthSession,
	settleActionSubmit,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const VALID_PASSWORD = "correct-horse-1!"

function mountAction() {
	return mountWithFrozenClock(PasswordChangeAction)
}

async function submitPasswords(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
	values: { current: string; next: string; confirm?: string },
) {
	const inputs = wrapper.findAll("input")
	await at(inputs, 0).setValue(values.current)
	await at(inputs, 1).setValue(values.next)
	await at(inputs, 2).setValue(values.confirm ?? values.next)
	await wrapper.get("form").trigger("submit")
	await settleActionSubmit()
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the submit flow is driven by the
// global fake timers
describe("<PasswordChangeAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "Ada" })
	})

	afterEach(disposeMockEndpoints)

	describe("when the account has a password", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthAccounts(["credential"])
		})

		it("offers the change-password form", async ({ expect }) => {
			const wrapper = await mountAction()

			expect(wrapper.findAll("input")).toHaveLength(3)
			expect(wrapper.text()).toContain("Choose a new password")
		})

		it("changes the password and revokes the other sessions", async ({
			expect,
		}) => {
			const calls = mockAuthEndpoint("change-password", () => ({
				token: "t",
			}))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: VALID_PASSWORD,
			})

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({
				currentPassword: "old-password",
				newPassword: VALID_PASSWORD,
				revokeOtherSessions: true,
			})
		})

		it("confirms and closes once the password is changed", async ({
			expect,
		}) => {
			mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: VALID_PASSWORD,
			})

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("rejects a new password that repeats no digit", async ({ expect }) => {
			const calls = mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: "correct-horse-!!",
			})

			expect(calls).toHaveLength(0)
			expect(wrapper.text()).toContain("at least one number")
		})

		it("rejects a new password with no special symbol", async ({ expect }) => {
			const calls = mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: "correcthorse1234",
			})

			expect(calls).toHaveLength(0)
			expect(wrapper.text()).toContain("at least one special symbol")
		})

		it("rejects a confirmation that does not match", async ({ expect }) => {
			const calls = mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: VALID_PASSWORD,
				confirm: "something-else-2!",
			})

			expect(calls).toHaveLength(0)
			expect(wrapper.text()).toContain("Passwords do not match")
		})

		it("rejects an empty current password", async ({ expect }) => {
			const calls = mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await submitPasswords(wrapper, { current: "", next: VALID_PASSWORD })

			expect(calls).toHaveLength(0)
			expect(wrapper.text()).toContain("enter your current password")
		})

		it("points at the current password field when the server rejects it", async ({
			expect,
		}) => {
			mockAuthEndpoint("change-password", (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "INVALID_PASSWORD", message: "nope" }
			})
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "wrong-password",
				next: VALID_PASSWORD,
			})

			expect(wrapper.text()).toContain("Incorrect password")
			expect(toast.custom).toHaveBeenCalledTimes(0)
		})

		it("warns when the change fails for any other reason", async ({
			expect,
		}) => {
			mockAuthEndpoint("change-password", (_call, event) => {
				setResponseStatus(event, 500)

				return { code: "BOOM", message: "boom" }
			})
			const wrapper = await mountAction()

			await submitPasswords(wrapper, {
				current: "old-password",
				next: VALID_PASSWORD,
			})

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("closes without changing anything when cancelled", async ({
			expect,
		}) => {
			const calls = mockAuthEndpoint("change-password", () => ({ token: "t" }))
			const wrapper = await mountAction()

			await findButtonByText(wrapper, "Cancel").trigger("click")

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})
	})

	describe("when the account has no password", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthAccounts(["github"])
		})

		it("offers to email a setup link instead of a form", async ({ expect }) => {
			const wrapper = await mountAction()

			expect(wrapper.findAll("input")).toHaveLength(0)
			expect(wrapper.text()).toContain("doesn't have a password yet")
		})

		it("names the address the link goes to", async ({ expect }) => {
			const wrapper = await mountAction()

			expect(wrapper.text()).toContain("ada@oxynote.test")
		})

		it("asks the server for a password reset pointing at the reset page", async ({
			expect,
		}) => {
			const calls = mockAuthEndpoint("request-password-reset", () => ({
				status: true,
			}))
			const wrapper = await mountAction()

			await findButtonByText(wrapper, "Send Link").trigger("click")
			await settleActionSubmit()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({
				email: "ada@oxynote.test",
				redirectTo: "http://test.local/reset-password",
			})
		})

		it("confirms and closes once the link is on its way", async ({
			expect,
		}) => {
			mockAuthEndpoint("request-password-reset", () => ({ status: true }))
			const wrapper = await mountAction()

			await findButtonByText(wrapper, "Send Link").trigger("click")
			await settleActionSubmit()

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("warns and stays open when the link cannot be sent", async ({
			expect,
		}) => {
			mockAuthEndpoint("request-password-reset", () => {
				throw new Error("boom")
			})
			const wrapper = await mountAction()

			await findButtonByText(wrapper, "Send Link").trigger("click")
			await settleActionSubmit()

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("does nothing while the signed-in address is unknown", async ({
			expect,
		}) => {
			seedAuthSession({ id: "u1", name: "Ada" })
			const calls = mockAuthEndpoint("request-password-reset", () => ({
				status: true,
			}))
			const wrapper = await mountAction()

			await findButtonByText(wrapper, "Send Link").trigger("click")
			await settleActionSubmit()

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})
})
