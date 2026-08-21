import { mountSuspended } from "@nuxt/test-utils/runtime"
import { setResponseHeader } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import ProfileSection from "./ProfileSection.vue"
import {
	findButtonByText,
	mockAuthEndpoint,
	mockAuthOrganization,
	seedAuthAccounts,
	seedAuthSession,
	settleMutations,
	WAIT_FOR_OPTIONS,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function seedSlack(options: {
	connected?: boolean
	linkSettings?: { notifications: boolean } | null
}) {
	seedQueryData(["slack", "connected"], {
		connected: options.connected ?? false,
		configured: true,
	})
	seedQueryData(
		["slack", "user-link-settings"],
		options.linkSettings === undefined ? null : options.linkSettings,
	)
}

function mountSection() {
	return mountSuspended(ProfileSection)
}

function usernameInput(wrapper: Awaited<ReturnType<typeof mountSection>>) {
	return wrapper.get<HTMLInputElement>("input[type='text']")
}

// the field saves on blur, and the chain behind it runs vee-validate's
// async validation, the update request, and two refetches
async function saveUsername(
	wrapper: Awaited<ReturnType<typeof mountSection>>,
	value?: string,
) {
	if (value !== undefined) {
		await usernameInput(wrapper).setValue(value)
	}

	await usernameInput(wrapper).trigger("blur")

	for (let round = 0; round < 6; round++) {
		await settleMutations()
	}
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares
describe("<ProfileSection>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		seedAuthSession({
			id: "u1",
			email: "ada@oxynote.test",
			name: "ada",
			image: "https://cdn.test/ada.png",
		})
		seedAuthAccounts(["credential"])
		mockAuthOrganization({ id: "org-1", members: [], invitations: [] })
		seedSlack({})
	})

	afterEach(disposeMockEndpoints)

	it("shows the signed-in email address", async ({ expect }) => {
		const wrapper = await mountSection()

		expect(wrapper.text()).toContain("ada@oxynote.test")
	})

	it("starts the username field from the current name", async ({ expect }) => {
		const wrapper = await mountSection()

		expect(usernameInput(wrapper).element.value).toBe("ada")
	})

	it("asks to change the email when its pencil is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountSection()

		await findButtonByText(wrapper, "Change Email").trigger("click")

		expect(wrapper.emitted("email-change")).toHaveLength(1)
	})

	it("asks to change the password when its button is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountSection()

		await findButtonByText(wrapper, "Change password").trigger("click")

		expect(wrapper.emitted("password-change")).toHaveLength(1)
	})

	it("offers to set a password when the account has none", async ({
		expect,
	}) => {
		seedAuthAccounts(["github"])

		const wrapper = await mountSection()

		expect(wrapper.text()).toContain("Set password")
		expect(wrapper.text()).toContain("No password set")
	})

	it("asks to delete the account when its button is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountSection()

		await findButtonByText(wrapper, "Delete Account").trigger("click")

		expect(wrapper.emitted("account-deletion")).toHaveLength(1)
	})

	it("saves a changed username on blur", async ({ expect }) => {
		const calls = mockAuthEndpoint("update-user", () => ({ status: true }))
		const wrapper = await mountSection()

		await saveUsername(wrapper, "ada-lovelace")

		// the save chains vee-validate's validation, the update request and
		// two refetches, none of which the component signals the end of
		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({ name: "ada-lovelace" })
	})

	it("confirms a saved username", async ({ expect }) => {
		mockAuthEndpoint("update-user", () => ({ status: true }))
		const wrapper = await mountSection()

		await saveUsername(wrapper, "ada-lovelace")

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("saves nothing when the username is unchanged", async ({ expect }) => {
		const calls = mockAuthEndpoint("update-user", () => ({ status: true }))
		const wrapper = await mountSection()

		await saveUsername(wrapper)

		expect(calls).toHaveLength(0)
	})

	it("rejects a username with characters outside the allowed set", async ({
		expect,
	}) => {
		const calls = mockAuthEndpoint("update-user", () => ({ status: true }))
		const wrapper = await mountSection()

		await saveUsername(wrapper, "ada lovelace!")

		expect(calls).toHaveLength(0)
		// vee-validate renders the message on its own scheduler, with no
		// signal the component surfaces
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Only letters, numbers, hyphens")
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the username cannot be saved", async ({ expect }) => {
		mockAuthEndpoint("update-user", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await saveUsername(wrapper, "ada-lovelace")

		// better-auth backs off internally before surfacing a failed
		// request, and exposes no signal for when it is done
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("uploads a picked avatar and saves the returned url", async ({
		expect,
	}) => {
		mockEndpoint("PUT", "/api/users/image", (_call, event) => {
			// the composable reads the stored url off the location header
			setResponseHeader(event, "location", "https://cdn.test/new.png")

			return null
		})
		const updates = mockAuthEndpoint("update-user", () => ({ status: true }))
		const wrapper = await mountSection()

		await pickAvatar(wrapper)

		// the upload and the profile update are chained requests, each with
		// its own internal scheduling
		await vi.waitFor(() => {
			expect(updates).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(updates[0]?.body).toEqual({ image: "https://cdn.test/new.png" })
		// the confirmation lands after the refetches that follow the update;
		// leaving it in flight would spill into the next test
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the avatar upload fails", async ({ expect }) => {
		mockEndpoint("PUT", "/api/users/image", () => {
			throw new Error("boom")
		})
		const wrapper = await mountSection()

		await pickAvatar(wrapper)

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("does nothing when the file dialog is dismissed", async ({ expect }) => {
		const calls = mockEndpoint("PUT", "/api/users/image", () => ({
			url: "https://cdn.test/new.png",
		}))
		const wrapper = await mountSection()

		await wrapper.get("input[type='file']").trigger("change")
		await settleMutations()

		expect(calls).toHaveLength(0)
	})

	describe("slack notifications", { concurrent: false }, () => {
		it("stays hidden while slack is not connected", async ({ expect }) => {
			seedSlack({ connected: false })

			const wrapper = await mountSection()

			expect(wrapper.text()).not.toContain("Slack Notifications")
		})

		it("appears once slack is connected", async ({ expect }) => {
			seedSlack({ connected: true, linkSettings: { notifications: true } })

			const wrapper = await mountSection()

			expect(wrapper.text()).toContain("Slack Notifications")
		})

		it("explains how to link an unlinked slack account", async ({ expect }) => {
			seedSlack({ connected: true, linkSettings: null })

			const wrapper = await mountSection()

			expect(wrapper.text()).toContain("link your Slack account")
			expect(wrapper.get("button[role='switch']").attributes("disabled")).toBe(
				"",
			)
		})

		it("saves the new notification preference", async ({ expect }) => {
			seedSlack({ connected: true, linkSettings: { notifications: false } })
			const calls = mockEndpoint("PUT", "/api/slack/users/settings", () => ({
				notifications: true,
			}))
			const wrapper = await mountSection()

			await wrapper.get("button[role='switch']").trigger("click")
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ notifications: true })
			expect(toast.custom).toHaveBeenCalledTimes(1)
		})

		it("puts the switch back and warns when saving fails", async ({
			expect,
		}) => {
			seedSlack({ connected: true, linkSettings: { notifications: false } })
			mockEndpoint("PUT", "/api/slack/users/settings", () => {
				throw new Error("boom")
			})
			const wrapper = await mountSection()

			await wrapper.get("button[role='switch']").trigger("click")
			await settleMutations()

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(
				wrapper.get("button[role='switch']").attributes("aria-checked"),
			).toBe("false")
		})
	})
})

// happy-dom does not let a test click through a real file dialog, so the
// picked file is put on the input directly before its change handler runs
async function pickAvatar(wrapper: Awaited<ReturnType<typeof mountSection>>) {
	const input = wrapper.get("input[type='file']")
	const file = new File(["avatar"], "avatar.png", { type: "image/png" })
	Object.defineProperty(input.element, "files", { value: [file] })

	await input.trigger("change")
	await settleMutations()
}
