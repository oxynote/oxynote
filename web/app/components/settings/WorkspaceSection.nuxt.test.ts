import { setResponseHeader } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import WorkspaceSection from "./WorkspaceSection.vue"
import {
	findButtonByText,
	menuItem,
	mockAuthEndpoint,
	mockAuthOrganization,
	mountUnderTooltipProvider,
	seedAuthSession,
	settleMutations,
	WAIT_FOR_OPTIONS,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function member(id: string, name: string) {
	return {
		id: `member-${id}`,
		organizationId: "org-1",
		userId: id,
		role: "member",
		createdAt: "2026-01-01T00:00:00Z",
		user: { name: name, email: `${name}@oxynote.test`, image: "" },
	}
}

function seedWorkspace(
	overrides: { members?: unknown[]; invitations?: unknown[] } = {},
) {
	mockAuthOrganization({
		id: "org-1",
		name: "Acme",
		slug: "acme-corp",
		logo: "",
		members: overrides.members ?? [member("u1", "ada")],
		invitations: overrides.invitations ?? [],
	})
}

async function mountSection() {
	const wrapper = await mountUnderTooltipProvider(WorkspaceSection, {})

	return { wrapper, section: wrapper.findComponent(WorkspaceSection) }
}

function nameInput(
	wrapper: Awaited<ReturnType<typeof mountSection>>["wrapper"],
) {
	return wrapper.get<HTMLInputElement>("input[type='text']")
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the member menus are teleported into
// the shared <body>
describe("<WorkspaceSection>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "ada" })
	})

	afterEach(disposeMockEndpoints)

	it("starts the name field from the workspace name", async ({ expect }) => {
		seedWorkspace()

		const { wrapper } = await mountSection()

		expect(nameInput(wrapper).element.value).toBe("Acme")
	})

	it("shows the workspace url", async ({ expect }) => {
		seedWorkspace()

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain("oxynote.io/")
		expect(wrapper.text()).toContain("acme-corp")
	})

	it("asks to change the url when its pencil is pressed", async ({
		expect,
	}) => {
		seedWorkspace()
		const { wrapper, section } = await mountSection()

		await findButtonByText(wrapper, "Change Workspace URL").trigger("click")

		expect(section.emitted("url-change")).toHaveLength(1)
	})

	it("asks to invite someone when the invite button is pressed", async ({
		expect,
	}) => {
		seedWorkspace()
		const { wrapper, section } = await mountSection()

		await findButtonByText(wrapper, "Invite").trigger("click")

		expect(section.emitted("invitation")).toHaveLength(1)
	})

	it("saves a changed workspace name on blur", async ({ expect }) => {
		seedWorkspace()
		const calls = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Acme-EU")
		await nameInput(wrapper).trigger("blur")

		// the save chains vee-validate's validation, the update request and
		// a refetch, none of which the component signals the end of
		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			data: { name: "Acme-EU" },
			organizationId: "org-1",
		})
	})

	it("saves nothing when the name is unchanged", async ({ expect }) => {
		seedWorkspace()
		const calls = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const { wrapper } = await mountSection()

		await nameInput(wrapper).trigger("blur")
		await settleMutations()

		expect(calls).toHaveLength(0)
	})

	it("rejects a name with characters outside the allowed set", async ({
		expect,
	}) => {
		seedWorkspace()
		const calls = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Acme Corp!")
		await nameInput(wrapper).trigger("blur")

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Only letters, numbers, hyphens")
		}, WAIT_FOR_OPTIONS)
		expect(calls).toHaveLength(0)
	})

	it("warns when the name cannot be saved", async ({ expect }) => {
		seedWorkspace()
		mockAuthEndpoint("organization/update", () => {
			throw new Error("boom")
		})
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Acme-EU")
		await nameInput(wrapper).trigger("blur")

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("uploads a picked logo and saves the returned url", async ({ expect }) => {
		seedWorkspace()
		mockEndpoint("PUT", "/api/organizations/logo", (_call, event) => {
			// the composable reads the stored url off the location header
			setResponseHeader(event, "location", "https://cdn.test/logo.png")

			return null
		})
		const updates = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const { wrapper } = await mountSection()

		await pickLogo(wrapper)

		await vi.waitFor(() => {
			expect(updates).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(updates[0]?.body).toEqual({
			data: { logo: "https://cdn.test/logo.png" },
			organizationId: "org-1",
		})
		// the confirmation lands after the refetch that follows the update;
		// leaving it in flight would spill into the next test
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the logo upload fails", async ({ expect }) => {
		seedWorkspace()
		mockEndpoint("PUT", "/api/organizations/logo", () => {
			throw new Error("boom")
		})
		const { wrapper } = await mountSection()

		await pickLogo(wrapper)

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("does nothing when the file dialog is dismissed", async ({ expect }) => {
		seedWorkspace()
		const calls = mockEndpoint("PUT", "/api/organizations/logo", () => null)
		const { wrapper } = await mountSection()

		await wrapper.get("input[type='file']").trigger("change")
		await settleMutations()

		expect(calls).toHaveLength(0)
	})

	describe("member list", { concurrent: false }, () => {
		it("lists the joined members", async ({ expect }) => {
			seedWorkspace({ members: [member("u1", "ada"), member("u2", "linus")] })

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain("ada")
			expect(wrapper.text()).toContain("linus@oxynote.test")
			expect(wrapper.text()).toContain("Members (2/5)")
		})

		it("shows when a member joined", async ({ expect }) => {
			seedWorkspace()

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain("Joined")
			expect(wrapper.text()).toContain("Jan 1, 2026")
		})

		it("lists a pending invitation as an invited member", async ({
			expect,
		}) => {
			seedWorkspace({
				invitations: [
					{
						id: "inv-1",
						organizationId: "org-1",
						status: "pending",
						email: "grace@oxynote.test",
						role: "member",
					},
				],
			})

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain("grace@oxynote.test")
			expect(wrapper.text()).toContain("Invited")
			expect(wrapper.text()).toContain("Members (2/5)")
		})

		it("leaves an accepted invitation out of the list", async ({ expect }) => {
			seedWorkspace({
				invitations: [
					{
						id: "inv-1",
						organizationId: "org-1",
						status: "accepted",
						email: "grace@oxynote.test",
						role: "member",
					},
				],
			})

			const { wrapper } = await mountSection()

			expect(wrapper.text()).not.toContain("grace@oxynote.test")
		})

		it("offers no options menu on the signed-in user's own row", async ({
			expect,
		}) => {
			seedWorkspace()

			const { wrapper } = await mountSection()

			expect(wrapper.find("[data-slot='dropdown-menu-trigger']").exists()).toBe(
				false,
			)
		})

		it("asks to remove the member picked from their options menu", async ({
			expect,
		}) => {
			seedWorkspace({ members: [member("u1", "ada"), member("u2", "linus")] })
			const { wrapper, section } = await mountSection()
			await wrapper.get("[data-slot='dropdown-menu-trigger']").trigger("click")

			menuItem("Remove").click()
			await nextTick()

			expect(section.emitted("member-removal")?.[0]?.[0]).toMatchObject({
				id: "member-u2",
			})
		})
	})
})

// happy-dom does not let a test click through a real file dialog, so the
// picked file is put on the input directly before its change handler runs
async function pickLogo(
	wrapper: Awaited<ReturnType<typeof mountSection>>["wrapper"],
) {
	const input = wrapper.get("input[type='file']")
	const file = new File(["logo"], "logo.png", { type: "image/png" })
	Object.defineProperty(input.element, "files", { value: [file] })

	await input.trigger("change")
	await settleMutations()
}
