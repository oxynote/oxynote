import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { setResponseStatus } from "h3"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import EmailChangeAction from "./EmailChangeAction.vue"
import {
	at,
	findButtonByText,
	mockAuthEndpoint,
	mountWithFrozenClock,
	raisedToasts,
	seedAuthAccounts,
	seedAuthConfig,
	seedAuthSession,
	settleActionSubmit,
	t,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function mountAction() {
	return mountWithFrozenClock(EmailChangeAction)
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
describe("<EmailChangeAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "Ada" })
	})

	afterEach(disposeMockEndpoints)

	it("explains that the current address approves the change first", async ({
		expect,
	}) => {
		const wrapper = await mountAction()

		expect(wrapper.text()).toContain(
			t("settings.action-modals.email-change.description"),
		)
	})

	it("sends the new address to the server with a verification callback", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("change-email", () => ({ status: true }))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "new@oxynote.test")

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toMatchObject({ newEmail: "new@oxynote.test" })
		expect(
			(calls[0]?.body as { callbackURL: string } | undefined)?.callbackURL,
		).toContain("new%40oxynote.test")
	})

	it("confirms and closes once the verification email is on its way", async ({
		expect,
	}) => {
		mockAuthEndpoint("change-email", () => ({ status: true }))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "new@oxynote.test")

		expect(raisedToasts()).toMatchObject([
			{
				type: "success",
				title: t("settings.action-modals.email-change.success-message.title"),
				description: t(
					"settings.action-modals.email-change.success-message.description",
					{ current: "ada@oxynote.test", email: "new@oxynote.test" },
				),
			},
		])
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("closes without asking the server when the address is unchanged", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("change-email", () => ({ status: true }))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "ada@oxynote.test")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("rejects an address that is not an email", async ({ expect }) => {
		const calls = mockAuthEndpoint("change-email", () => ({ status: true }))
		const wrapper = await mountAction()

		await submitEmail(wrapper, "not-an-email")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("shows the server's rejection next to the field", async ({ expect }) => {
		mockAuthEndpoint("change-email", (_call, event) => {
			setResponseStatus(event, 400)

			return { code: "EMAIL_TAKEN", message: "Email already taken" }
		})
		const wrapper = await mountAction()

		await submitEmail(wrapper, "taken@oxynote.test")

		expect(wrapper.text()).toContain("Email already taken")
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("closes without sending anything when cancelled", async ({ expect }) => {
		const calls = mockAuthEndpoint("change-email", () => ({ status: true }))
		const wrapper = await mountAction()

		await findButtonByText(
			wrapper,
			t("settings.action-modals.email-change.cancel-button"),
		).trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	describe("for the default admin", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthConfig({
				singleOrganization: true,
				defaultAdmin: { email: "admin@example.com", password: null },
			})
		})

		it("explains that only the new address gets a link", async ({ expect }) => {
			seedAuthSession({ id: "u1", email: "admin@example.com", name: "Admin" })

			const wrapper = await mountAction()

			expect(wrapper.text()).toContain(
				t("settings.action-modals.email-change.description-default-admin"),
			)
		})

		it("confirms that the verification link went to the new address", async ({
			expect,
		}) => {
			seedAuthSession({ id: "u1", email: "admin@example.com", name: "Admin" })
			mockAuthEndpoint("change-email", () => ({ status: true }))
			const wrapper = await mountAction()

			await submitEmail(wrapper, "boss@oxynote.test")

			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					description: t(
						"settings.action-modals.email-change.success-message-default-admin.description",
						{ email: "boss@oxynote.test" },
					),
				},
			])
		})

		it("keeps the approval step for every other member", async ({ expect }) => {
			const wrapper = await mountAction()

			expect(wrapper.text()).toContain(
				t("settings.action-modals.email-change.description"),
			)
		})
	})

	describe("when the server sends no email", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthConfig({ emailEnabled: false })
			seedAuthAccounts(["credential"])
		})

		async function submitWithPassword(
			wrapper: Awaited<ReturnType<typeof mountAction>>,
			password: string,
		) {
			const inputs = wrapper.findAll("input")
			await at(inputs, 0).setValue("new@oxynote.test")
			await at(inputs, 1).setValue(password)
			await wrapper.get("form").trigger("submit")
			await settleActionSubmit()
		}

		it("asks for the current password instead of promising an email", async ({
			expect,
		}) => {
			const wrapper = await mountAction()

			expect(wrapper.find("input[type='password']").exists()).toBe(true)
			expect(wrapper.text()).toContain(
				t("settings.action-modals.email-change.description-without-email"),
			)
		})

		it("sends the password along with the new address", async ({ expect }) => {
			const calls = mockAuthEndpoint("change-email", () => ({ status: true }))
			const wrapper = await mountAction()

			await submitWithPassword(wrapper, "correct-horse-1!")

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toMatchObject({
				newEmail: "new@oxynote.test",
				password: "correct-horse-1!",
			})
		})

		it("reports the address as changed and closes", async ({ expect }) => {
			mockAuthEndpoint("change-email", () => ({ status: true }))
			const wrapper = await mountAction()

			await submitWithPassword(wrapper, "correct-horse-1!")

			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					title: t(
						"settings.action-modals.email-change.success-message-without-email.title",
					),
				},
			])
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("points at the password field when the server rejects it", async ({
			expect,
		}) => {
			mockAuthEndpoint("change-email", (_call, event) => {
				setResponseStatus(event, 400)

				return { code: "INVALID_PASSWORD", message: "Invalid password" }
			})
			const wrapper = await mountAction()

			await submitWithPassword(wrapper, "wrong-password")

			expect(wrapper.text()).toContain(
				t("settings.action-modals.email-change.errors.invalid-password"),
			)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("explains why an account without a password cannot change it", async ({
			expect,
		}) => {
			seedAuthAccounts(["github"])
			mockAuthEndpoint("change-email", (_call, event) => {
				setResponseStatus(event, 403)

				return {
					code: "CREDENTIAL_ACCOUNT_NOT_FOUND",
					message: "Credential account not found",
				}
			})
			const wrapper = await mountAction()

			await submitEmail(wrapper, "new@oxynote.test")

			expect(wrapper.find("input[type='password']").exists()).toBe(false)
			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					description: t(
						"settings.action-modals.email-change.errors.no-password.description",
					),
				},
			])
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})
})
