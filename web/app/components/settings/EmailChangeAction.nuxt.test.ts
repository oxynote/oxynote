import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { setResponseStatus } from "h3"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import EmailChangeAction from "./EmailChangeAction.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	mountWithFrozenClock,
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

	it("explains what changing the email does", async ({ expect }) => {
		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("Changing your email address")
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

		expect(toast.custom).toHaveBeenCalledTimes(1)
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
})
