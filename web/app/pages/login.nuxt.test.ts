import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it } from "vitest"
import { clearQueryCache } from "~/composables/api/test-helpers"
import Login from "./login.vue"
import { findButtonByText, seedAuthConfig, t } from "../components/test-helpers"

// the query cache is an app-wide singleton every mount in the file shares
describe("<login>", { concurrent: false }, () => {
	beforeEach(clearQueryCache)

	it("shows the default admin credentials above the email field while they still sign in", async ({
		expect,
	}) => {
		seedAuthConfig({
			singleOrganization: true,
			defaultAdmin: {
				email: "admin@example.com",
				password: "oxynote-admin-1234",
			},
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.findAll("li").map((li) => li.text())).toEqual([
			"admin@example.com",
			"oxynote-admin-1234",
		])
		expect(wrapper.findAll("code").map((c) => c.element.textContent)).toEqual([
			"admin@example.com",
			"oxynote-admin-1234",
		])

		const form = wrapper.get("form").element
		const box = form.firstElementChild
		expect(box?.textContent).toContain("oxynote-admin-1234")
		expect(
			box?.nextElementSibling?.querySelector("input[type='email']"),
		).not.toBeNull()
	})

	it("shows no credentials once the default admin has changed the password", async ({
		expect,
	}) => {
		seedAuthConfig({
			singleOrganization: true,
			defaultAdmin: { email: "admin@example.com", password: null },
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.text()).not.toContain("admin@example.com")
	})

	it("shows no credentials outside single-workspace mode", async ({
		expect,
	}) => {
		seedAuthConfig({
			singleOrganization: false,
			defaultAdmin: {
				email: "admin@example.com",
				password: "oxynote-admin-1234",
			},
		})

		const wrapper = await openEmailPasswordForm()

		expect(wrapper.text()).not.toContain("admin@example.com")
	})
})

async function openEmailPasswordForm() {
	const wrapper = await mountSuspended(Login)

	await findButtonByText(
		wrapper,
		t("onboarding.login.login-email-password"),
	).trigger("click")

	return wrapper
}
