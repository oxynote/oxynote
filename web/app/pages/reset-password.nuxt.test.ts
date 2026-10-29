import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { RouteMiddleware } from "#app"
import type { RouteLocationNormalized } from "vue-router"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
} from "~/composables/api/test-helpers"
import ResetPassword from "./reset-password.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	raisedToasts,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "../components/test-helpers"

const VALID_PASSWORD = "correct-horse-battery-1"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// route.query is a readonly proxy, so the route itself is what a test
// arranges
const { useRouteMock, navigateToMock } = vi.hoisted(() => ({
	useRouteMock: vi.fn(),
	navigateToMock: vi.fn(),
}))

mockNuxtImport("useRoute", () => useRouteMock)
mockNuxtImport("navigateTo", () => navigateToMock)

// the vue-sonner, useRoute and navigateTo module mocks are app-wide
// singletons every mount in the file shares
describe("<reset-password>", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useRouteMock.mockReset()
		navigateToMock.mockReset()
	})

	afterEach(disposeMockEndpoints)

	it("sends a visitor without a reset token home", async ({ expect }) => {
		const to = { query: {} } as unknown as RouteLocationNormalized

		await pageMiddleware()(to, to)

		expect(navigateToMock).toHaveBeenCalledTimes(1)
		expect(navigateToMock).toHaveBeenCalledWith("/")
	})

	it("lets a visitor with a reset token through", async ({ expect }) => {
		const to = { query: { token: "t1" } } as unknown as RouteLocationNormalized

		const result = await pageMiddleware()(to, to)

		expect(result).toBeUndefined()
		expect(navigateToMock).not.toHaveBeenCalled()
	})

	it.for([
		{
			name: "asks for a number in the new password",
			input: { password: "correct-horse-battery!", confirm: "" },
			expected: "onboarding.reset-password.errors.password-number",
		},
		{
			name: "asks for a symbol in the new password",
			input: { password: "correcthorsebattery1", confirm: "" },
			expected: "onboarding.reset-password.errors.password-symbol",
		},
		{
			name: "asks for a confirmation that matches the new password",
			input: { password: VALID_PASSWORD, confirm: "correct-horse-battery-2" },
			expected: "onboarding.reset-password.errors.password-mismatch",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const calls = mockAuthEndpoint("reset-password", () => ({ status: true }))
		const wrapper = await mountResetPassword()

		await submit(wrapper, input.password, input.confirm || input.password)
		await waitForSubmit(() => {
			expect(wrapper.text()).toContain(t(expected))
		})

		expect.soft(calls).toHaveLength(0)
		expect.soft(navigateToMock).not.toHaveBeenCalled()
	})

	it("sets the new password and sends the user to log in", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("reset-password", () => ({ status: true }))
		const wrapper = await mountResetPassword()

		await submit(wrapper, VALID_PASSWORD, VALID_PASSWORD)
		await waitForSubmit(() => {
			expect(navigateToMock).toHaveBeenCalled()
		})

		expect.soft(calls).toHaveLength(1)
		expect.soft(calls[0]?.body).toEqual({
			newPassword: VALID_PASSWORD,
			token: "t1",
		})
		expect.soft(navigateToMock).toHaveBeenCalledTimes(1)
		expect.soft(navigateToMock).toHaveBeenCalledWith({
			path: "/login",
			query: { reset: "true" },
		})
		expect.soft(toast.custom).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.reset-password.continue"),
				).attributes("disabled"),
			)
			.toBeDefined()
	})

	it("reports a refused reset and lets the user try again", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("reset-password", () => {
			throw createError({ statusCode: 400 })
		})
		const wrapper = await mountResetPassword()

		await submit(wrapper, VALID_PASSWORD, VALID_PASSWORD)
		await waitForSubmit(() => {
			expect(toast.custom).toHaveBeenCalled()
		})
		await settleMutations()

		expect.soft(calls).toHaveLength(1)
		expect.soft(raisedToasts()).toMatchObject([
			{
				type: "error",
				title: t("onboarding.reset-password.errors.reset-failed"),
			},
		])
		expect.soft(navigateToMock).not.toHaveBeenCalled()
		expect
			.soft(
				findButtonByText(
					wrapper,
					t("onboarding.reset-password.continue"),
				).attributes("disabled"),
			)
			.toBeUndefined()
	})
})

// pageMiddleware is the guard definePageMeta attached to the page's
// route, the way the router would run it before rendering
function pageMiddleware() {
	const route = useRouter()
		.getRoutes()
		.find((r) => r.name === "reset-password")
	const middleware = route?.meta.middleware

	if (typeof middleware !== "function") {
		throw new Error("the reset-password page registers no inline middleware")
	}

	return middleware as RouteMiddleware
}

function mountResetPassword() {
	useRouteMock.mockReturnValue({ query: { token: "t1" } })

	return mountSuspended(ResetPassword)
}

async function submit(
	wrapper: Awaited<ReturnType<typeof mountResetPassword>>,
	password: string,
	confirm: string,
) {
	const [passwordInput, confirmInput] = wrapper.findAll(
		"input[type='password']",
	)
	await passwordInput?.setValue(password)
	await confirmInput?.setValue(confirm)

	// happy-dom does not raise a submit event from a click on a submit
	// button, so the form is submitted here
	await wrapper.get("form").trigger("submit")
}

// a submit runs vee-validate's validation first, on its own scheduler,
// and nothing signals when that or the request after it is done
function waitForSubmit(check: () => void) {
	return vi.waitFor(check, WAIT_FOR_OPTIONS)
}
