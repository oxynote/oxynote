import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import DesktopAuth from "./desktop-auth.vue"
import { t } from "../components/test-helpers"

// the server leaves the authorization code in this cookie once the
// browser half of the sign-in is done
const REDIRECT_COOKIE = "better-auth.electron"

// fake timers and document.cookie are shared by every mount in the file
describe("<desktop-auth>", { concurrent: false }, () => {
	// the page stops the poll with clearTimeout, which a browser accepts
	// for an interval too. Only timers are faked, as a frozen Date would
	// stall the nuxt app's own setup during the mount.
	beforeEach(() => {
		vi.useFakeTimers({
			toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"],
		})
	})

	afterEach(() => {
		vi.useRealTimers()
		document.cookie = `${REDIRECT_COOKIE}=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/`
	})

	it("tells the user to return to the desktop app", async ({ expect }) => {
		const wrapper = await mountSuspended(DesktopAuth)

		expect(wrapper.text()).toContain(t("onboarding.desktop-auth.message"))
	})

	it("hands the authorization code to the desktop app once it arrives", async ({
		expect,
	}) => {
		const replace = vi
			.spyOn(window.location, "replace")
			.mockImplementation(() => undefined)
		await mountSuspended(DesktopAuth)
		document.cookie = `${REDIRECT_COOKIE}=code1; path=/`

		vi.advanceTimersByTime(100)

		expect.soft(replace).toHaveBeenCalledTimes(1)
		expect
			.soft(replace)
			.toHaveBeenCalledWith("oxynote://auth/callback#token=code1")
		expect.soft(document.cookie).not.toContain(REDIRECT_COOKIE)
	})

	it("stops waiting for the code when the page goes away", async ({
		expect,
	}) => {
		const replace = vi
			.spyOn(window.location, "replace")
			.mockImplementation(() => undefined)
		const wrapper = await mountSuspended(DesktopAuth)
		document.cookie = `${REDIRECT_COOKIE}=code1; path=/`

		wrapper.unmount()

		vi.advanceTimersByTime(100)
		expect(replace).not.toHaveBeenCalled()
	})
})
