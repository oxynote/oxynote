import { flushPromises } from "@vue/test-utils"
import { setResponseHeader, setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import WorkspaceSection from "./WorkspaceSection.vue"
import {
	at,
	clearTeleportedOverlays,
	findButtonByText,
	menuItem,
	mockAuthEndpoint,
	mockAuthOrganization,
	mountUnderTooltipProvider,
	openSubMenu,
	raisedToasts,
	refuseForRole,
	seedAuthConfig,
	seedAuthOrganization,
	seedAuthSession,
	settleMutations,
	stubViewportMatches,
	t,
	WAIT_FOR_OPTIONS,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

function member(id: string, name: string, role = "editor") {
	return {
		id: `member-${id}`,
		organizationId: "org-1",
		userId: id,
		role: role,
		createdAt: "2026-01-01T00:00:00Z",
		user: { name: name, email: `${name}@oxynote.test`, image: "" },
	}
}

function invitation(inviterId: string) {
	return {
		id: "inv-1",
		organizationId: "org-1",
		status: "pending",
		email: "grace@oxynote.test",
		role: "viewer",
		inviterId: inviterId,
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
		members: overrides.members ?? [member("u1", "ada", "admin")],
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

	it("fills the name field once a late organization arrives", async ({
		expect,
	}) => {
		seedAuthOrganization(null)

		const { wrapper } = await mountSection()
		expect(nameInput(wrapper).element.value).toBe("")

		mockAuthOrganization({
			id: "org-1",
			name: "Acme",
			slug: "acme-corp",
			logo: "",
			members: [member("u1", "ada", "admin")],
			invitations: [],
		})
		await nextTick()

		expect(nameInput(wrapper).element.value).toBe("Acme")
	})

	it("follows a name that changed since the field was filled", async ({
		expect,
	}) => {
		seedWorkspace()

		const { wrapper } = await mountSection()
		expect(nameInput(wrapper).element.value).toBe("Acme")

		mockAuthOrganization({
			id: "org-1",
			name: "Acme-Renamed",
			slug: "acme-corp",
			logo: "",
			members: [member("u1", "ada", "admin")],
			invitations: [],
		})
		await nextTick()

		expect(nameInput(wrapper).element.value).toBe("Acme-Renamed")
	})

	it("leaves a half-typed name alone when the organization updates", async ({
		expect,
	}) => {
		seedWorkspace()

		const { wrapper } = await mountSection()
		await nameInput(wrapper).setValue("half-typed")

		mockAuthOrganization({
			id: "org-1",
			name: "Acme-Renamed",
			slug: "acme-corp",
			logo: "",
			members: [member("u1", "ada", "admin")],
			invitations: [],
		})
		await nextTick()

		expect(nameInput(wrapper).element.value).toBe("half-typed")
	})

	it("shows the workspace url", async ({ expect }) => {
		seedWorkspace()

		const { wrapper } = await mountSection()

		expect(wrapper.text()).toContain(t("settings.workspace.url-prefix"))
		expect(wrapper.text()).toContain("acme-corp")
	})

	it("asks to change the url when its pencil is pressed", async ({
		expect,
	}) => {
		seedWorkspace()
		const { wrapper, section } = await mountSection()

		await findButtonByText(
			wrapper,
			t("settings.workspace.url-change-button-screen-reader-hint"),
		).trigger("click")

		expect(section.emitted("url-change")).toHaveLength(1)
	})

	it("asks to invite someone when the invite button is pressed", async ({
		expect,
	}) => {
		seedWorkspace()
		const { wrapper, section } = await mountSection()

		await findButtonByText(
			wrapper,
			t("settings.workspace.invitation-button"),
		).trigger("click")

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

	it("retains the saved name when blurred after form submission", async ({
		expect,
	}) => {
		const org = {
			id: "org-1",
			name: "Acme",
			slug: "acme-corp",
			logo: "",
			members: [member("u1", "ada", "admin")],
			invitations: [],
		}
		mockAuthOrganization(org)
		const calls = mockAuthEndpoint("organization/update", (args) => {
			const body = args.body as { data: { name?: string } }
			if (body.data.name) {
				org.name = body.data.name
			}
			return { id: "org-1" }
		})
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Acme-EU")
		await wrapper.find("form").trigger("submit")
		await settleMutations()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			data: { name: "Acme-EU" },
			organizationId: "org-1",
		})

		await nameInput(wrapper).trigger("blur")
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(nameInput(wrapper).element.value).toBe("Acme-EU")
	})

	it("preserves edits made while a save is pending", async ({ expect }) => {
		const org = {
			id: "org-1",
			name: "Acme",
			slug: "acme-corp",
			logo: "",
			members: [member("u1", "ada", "admin")],
			invitations: [],
		}
		mockAuthOrganization(org)
		let resolveUpdate: (value: { id: string }) => void = () => undefined
		let requestDispatched: () => void = () => undefined
		const dispatched = new Promise<void>((resolve) => {
			requestDispatched = resolve
		})
		const calls = mockAuthEndpoint("organization/update", (args) => {
			requestDispatched()
			const body = args.body as { data: { name?: string } }
			if (body.data.name) {
				org.name = body.data.name
			}
			return new Promise<{ id: string }>((resolve) => {
				resolveUpdate = resolve
			})
		})
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Acme-EU")
		await wrapper.find("form").trigger("submit")
		await dispatched

		await nameInput(wrapper).setValue("Acme-US")

		resolveUpdate({ id: "org-1" })
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({
			data: { name: "Acme-EU" },
			organizationId: "org-1",
		})
		expect(nameInput(wrapper).element.value).toBe("Acme-US")
	})

	it("saves a name made of several words", async ({ expect }) => {
		seedWorkspace()
		const calls = mockAuthEndpoint("organization/update", () => ({
			id: "org-1",
		}))
		const { wrapper } = await mountSection()

		await nameInput(wrapper).setValue("Sea of Clouds")
		await nameInput(wrapper).trigger("blur")

		// the save chains vee-validate's validation, the update request and
		// a refetch, none of which the component signals the end of
		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			data: { name: "Sea of Clouds" },
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
			expect(wrapper.text()).toContain(
				t("settings.workspace.errors.name-regex"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(calls).toHaveLength(0)
	})

	it("warns when the name cannot be saved", async ({ expect }) => {
		seedWorkspace()
		mockAuthEndpoint("organization/update", () => {
			throw createError({ statusCode: 500 })
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
			throw createError({ statusCode: 500 })
		})
		const { wrapper } = await mountSection()

		await pickLogo(wrapper)

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("shows only the access denied toast for a refused logo upload", async ({
		expect,
	}) => {
		seedWorkspace()
		mockEndpoint("PUT", "/api/organizations/logo", (_call, event) =>
			refuseForRole(event),
		)
		const { wrapper } = await mountSection()

		await pickLogo(wrapper)

		await settleMutations()

		expect(raisedToasts()).toMatchObject([
			{ type: "error", title: t("general.errors.role-not-allowed") },
		])
	})

	it("opens the file dialog when an admin presses the logo", async ({
		expect,
	}) => {
		seedWorkspace()
		const { wrapper } = await mountSection()
		const input = wrapper.get<HTMLInputElement>("input[type='file']")
		const click = vi.spyOn(input.element, "click")

		await logoAvatar(wrapper).trigger("click")

		expect(click).toHaveBeenCalledTimes(1)
	})

	it("does nothing when the file dialog is dismissed", async ({ expect }) => {
		seedWorkspace()
		const calls = mockEndpoint("PUT", "/api/organizations/logo", () => null)
		const { wrapper } = await mountSection()

		await wrapper.get("input[type='file']").trigger("change")
		await settleMutations()

		expect(calls).toHaveLength(0)
	})

	describe("without email", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthConfig({ emailEnabled: false })
		})

		it("says that invitation links go out by hand", async ({ expect }) => {
			seedWorkspace()

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain(
				t("settings.workspace.email-disabled-notice"),
			)
		})
	})

	// settings can be opened without the login page having filled the auth
	// config cache first, so the section has to fetch it itself
	it("shows the notice once the server's config arrives", async ({
		expect,
	}) => {
		mockEndpoint(
			"GET",
			"http://test.local/auth-realtime/api/auth-config",
			() => ({ methods: ["email-password"], emailEnabled: false }),
		)
		seedWorkspace()

		const { wrapper } = await mountSection()
		await settleMutations()

		expect(wrapper.text()).toContain(
			t("settings.workspace.email-disabled-notice"),
		)
	})

	it("shows no email notice where email is delivered", async ({ expect }) => {
		seedAuthConfig()
		seedWorkspace()

		const { wrapper } = await mountSection()

		expect(wrapper.text()).not.toContain(
			t("settings.workspace.email-disabled-notice"),
		)
	})

	describe("member list", { concurrent: false }, () => {
		beforeEach(() => {
			clearTeleportedOverlays()
			seedAuthConfig()
		})

		it("lists the joined members", async ({ expect }) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain("ada")
			expect(wrapper.text()).toContain("linus@oxynote.test")
			expect(wrapper.text()).toContain("Members (2/5)")
		})

		it("counts the members without a maximum when there is no limit", async ({
			expect,
		}) => {
			seedAuthConfig({ maxOrganizationMembers: null })
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain(
				t("settings.workspace.members-label-unlimited", { current: 2 }),
			)
		})

		it("shows when a member joined", async ({ expect }) => {
			seedWorkspace()

			const { wrapper } = await mountSection()

			expect(wrapper.text()).toContain(t("settings.workspace.joined-label"))
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
			expect(wrapper.text()).toContain(t("settings.workspace.invited-label"))
			expect(wrapper.text()).toContain("Members (2/5)")
		})

		it("gives a pending invitation the shared invited-user avatar", async ({
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
			await flushPromises()

			const row = wrapper
				.findAll("tr")
				.find((r) => r.text().includes("grace@oxynote.test"))
			expect(
				row?.get("[data-slot='avatar-fallback'] img").attributes("src"),
			).toBe(defaultAvatar("invited-user"))
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

		it("offers the signed-in user only an email copy on their own row", async ({
			expect,
		}) => {
			seedWorkspace()
			const { wrapper } = await mountSection()

			await openMemberOptions(wrapper, "ada@oxynote.test")

			expect(
				menuItem(t("settings.workspace.member-options.copy-email.title")),
			).toBeTruthy()
			expect(() =>
				menuItem(t("settings.workspace.member-options.delete.title")),
			).toThrow()
		})

		it("copies a member's email from their options menu", async ({
			expect,
		}) => {
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue(undefined)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "linus@oxynote.test")

			menuItem(t("settings.workspace.member-options.copy-email.title")).click()
			await settleMutations()

			expect(writeText).toHaveBeenCalledExactlyOnceWith("linus@oxynote.test")
			expect(raisedToasts()).toMatchObject([
				{ type: "success", title: t("settings.workspace.email-copied") },
			])
		})

		it("labels each member and pending invitation with their role", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
				invitations: [invitation("u2")],
			})

			const { wrapper } = await mountSection()

			const rows = wrapper.findAll("tr").map((row) => row.text())
			expect(rows).toHaveLength(3)
			expect(rows[0]).toContain(t("general.member-roles.admin"))
			expect(rows[1]).toContain(t("general.member-roles.editor"))
			expect(rows[2]).toContain(t("general.member-roles.viewer"))
		})

		it("offers a role picker only on the other joined members' rows", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
				invitations: [invitation("u2")],
			})

			const { wrapper } = await mountSection()

			expect(
				wrapper
					.findAll("tr")
					.map((row) => row.find("[role='combobox']").exists()),
			).toEqual([false, true, false])
		})

		it("explains each role in the role picker", async ({ expect }) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()

			await openRolePicker(wrapper)

			expect(
				Array.from(document.body.querySelectorAll("[role='option']")).map((o) =>
					o.textContent.replace(/\s+/g, " ").trim(),
				),
			).toEqual(
				(["admin", "editor", "viewer"] as const).map(
					(role) =>
						`${t(`general.member-roles.${role}`)}${t(`general.member-role-descriptions.${role}`)}`,
				),
			)
		})

		it("changes a role from the options menu on a small screen", async ({
			expect,
		}) => {
			stubViewportMatches(false)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const calls = mockAuthEndpoint("organization/update-member-role", () => ({
				id: "member-u2",
			}))
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "linus@oxynote.test")
			await openSubMenu(t("settings.workspace.member-options.role.title"))

			menuItem(t("general.member-role-descriptions.viewer")).click()
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ memberId: "member-u2", role: "viewer" })
		})

		it("marks the member's role in the options menu on a small screen", async ({
			expect,
		}) => {
			stubViewportMatches(false)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "linus@oxynote.test")

			await openSubMenu(t("settings.workspace.member-options.role.title"))

			expect(
				(["admin", "editor", "viewer"] as const).map(
					(role) =>
						menuItem(
							t(`general.member-role-descriptions.${role}`),
						).querySelector("[class*='i-lucide:check']") !== null,
				),
			).toEqual([false, true, false])
		})

		it("keeps the role out of the options menu on a wide screen", async ({
			expect,
		}) => {
			stubViewportMatches(true)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()

			await openMemberOptions(wrapper, "linus@oxynote.test")

			expect(() =>
				menuItem(t("settings.workspace.member-options.role.title")),
			).toThrow()
		})

		it("changes the role picked from a member's role picker", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const calls = mockAuthEndpoint("organization/update-member-role", () => ({
				id: "member-u2",
			}))
			// the cache keeps the editor, so only a refetch shows the viewer.
			mockAuthEndpoint(
				"organization/get-full-organization",
				() => ({
					id: "org-1",
					name: "Acme",
					slug: "acme-corp",
					logo: "",
					members: [
						member("u1", "ada", "admin"),
						member("u2", "linus", "viewer"),
					],
					invitations: [],
				}),
				"GET",
			)
			const { wrapper } = await mountSection()
			await openRolePicker(wrapper)

			pickRole(t("general.member-roles.viewer"))
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ memberId: "member-u2", role: "viewer" })
			expect(at(wrapper.findAll("tr"), 1).text()).toContain(
				t("general.member-roles.viewer"),
			)
			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					title: t("settings.workspace.success-messages.role.title"),
					description: {
						keypath:
							"settings.workspace.success-messages.role.description.viewer",
						params: { name: "linus", role: t("general.member-roles.viewer") },
					},
				},
			])
		})

		it("leaves the role alone when the member's own role is picked", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const calls = mockAuthEndpoint("organization/update-member-role", () => ({
				id: "member-u2",
			}))
			const { wrapper } = await mountSection()
			await openRolePicker(wrapper)

			pickRole(t("general.member-roles.editor"))
			await settleMutations()

			expect(calls).toHaveLength(0)
		})

		it("tells the admin why a role change failed", async ({ expect }) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			mockAuthEndpoint("organization/update-member-role", (_, event) => {
				setResponseStatus(event, 400)

				return {
					code: "ROLE_NOT_FOUND",
					message: "ROLE_NOT_FOUND: viewer",
				}
			})
			const { wrapper } = await mountSection()
			await openRolePicker(wrapper)

			pickRole(t("general.member-roles.viewer"))
			await settleMutations()

			expect(raisedToasts()).toMatchObject([
				{
					type: "error",
					title: t("settings.workspace.errors.toasts.role.title"),
					description: t("settings.workspace.errors.toasts.role.description"),
				},
			])
		})

		it("asks to remove the member picked from their options menu", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper, section } = await mountSection()
			await openMemberOptions(wrapper, "linus@oxynote.test")

			menuItem(t("settings.workspace.member-options.delete.title")).click()
			await nextTick()

			expect(section.emitted("member-removal")?.[0]?.[0]).toMatchObject({
				id: "member-u2",
			})
		})

		it("copies a pending invitation's link to hand over", async ({
			expect,
		}) => {
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue(undefined)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
				invitations: [invitation("u2")],
			})
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "grace@oxynote.test")

			menuItem(t("settings.workspace.member-options.copy-link.title")).click()
			await settleMutations()

			expect(writeText).toHaveBeenCalledTimes(1)
			expect(writeText).toHaveBeenCalledWith(
				"http://test.local/accept-invite?id=inv-1&email=grace%40oxynote.test&inviter=linus&orgName=Acme&orgId=org-1",
			)
			expect(raisedToasts()).toMatchObject([
				{
					type: "success",
					title: t("settings.workspace.invitation-link-copied"),
				},
			])
		})

		it("names the signed-in user as inviter once the inviter has left", async ({
			expect,
		}) => {
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue(undefined)
			seedWorkspace({ invitations: [invitation("u9")] })
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "grace@oxynote.test")

			menuItem(t("settings.workspace.member-options.copy-link.title")).click()
			await settleMutations()

			expect(writeText).toHaveBeenCalledWith(
				expect.stringContaining("&inviter=ada&"),
			)
		})

		it("offers no link to copy on a joined member's row", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()

			await openMemberOptions(wrapper, "linus@oxynote.test")

			expect(() =>
				menuItem(t("settings.workspace.member-options.copy-link.title")),
			).toThrow()
		})
	})

	describe("for a member who is not an admin", { concurrent: false }, () => {
		beforeEach(() => {
			clearTeleportedOverlays()
			seedAuthConfig()
		})

		it("shows the workspace without the controls that change it", async ({
			expect,
		}) => {
			seedWorkspace({
				members: [member("u1", "ada"), member("u2", "linus", "admin")],
				invitations: [invitation("u2")],
			})

			const { wrapper } = await mountSection()

			expect.soft(wrapper.find("input[type='text']").exists()).toBe(false)
			expect.soft(wrapper.text()).toContain("Acme")
			expect
				.soft(() =>
					findButtonByText(
						wrapper,
						t("settings.workspace.url-change-button-screen-reader-hint"),
					),
				)
				.toThrow()
			expect
				.soft(() =>
					findButtonByText(wrapper, t("settings.workspace.invitation-button")),
				)
				.toThrow()
			expect.soft(wrapper.find("[role='combobox']").exists()).toBe(false)
		})

		it("offers only the role list and an email copy in a member's options menu", async ({
			expect,
		}) => {
			stubViewportMatches(false)
			seedWorkspace({
				members: [member("u1", "ada"), member("u2", "linus", "admin")],
				invitations: [invitation("u2")],
			})
			const { wrapper } = await mountSection()

			await openMemberOptions(wrapper, "grace@oxynote.test")

			expect(
				Array.from(
					document.body.querySelectorAll("[role='menu'] [role^='menuitem']"),
				).map((el) => el.textContent.trim()),
			).toEqual([
				t("settings.workspace.member-options.role.title"),
				t("settings.workspace.member-options.copy-email.title"),
			])
		})

		it("disables the small screen role list on a row the user cannot change", async ({
			expect,
		}) => {
			stubViewportMatches(false)
			seedWorkspace({
				members: [member("u1", "ada", "admin"), member("u2", "linus")],
			})
			const { wrapper } = await mountSection()
			await openMemberOptions(wrapper, "ada@oxynote.test")

			await openSubMenu(t("settings.workspace.member-options.role.title"))

			expect(
				(["admin", "editor", "viewer"] as const).map((role) =>
					menuItem(t(`general.member-role-descriptions.${role}`)).hasAttribute(
						"data-disabled",
					),
				),
			).toEqual([true, true, true])
		})

		it("opens no file dialog when the logo is pressed", async ({ expect }) => {
			seedWorkspace({
				members: [member("u1", "ada"), member("u2", "linus", "admin")],
			})
			const { wrapper } = await mountSection()
			const input = wrapper.get<HTMLInputElement>("input[type='file']")
			const click = vi.spyOn(input.element, "click")

			await logoAvatar(wrapper).trigger("click")

			expect(click).toHaveBeenCalledTimes(0)
		})
	})
})

// the logo comes before the member list, so it is the first avatar
function logoAvatar(
	wrapper: Awaited<ReturnType<typeof mountSection>>["wrapper"],
) {
	return wrapper.get("[data-slot='avatar']")
}

// the tests that pick a role render one picker, on the other member's row
async function openRolePicker(
	wrapper: Awaited<ReturnType<typeof mountSection>>["wrapper"],
) {
	await wrapper.get("[role='combobox']").trigger("keydown", { key: "Enter" })
	await nextTick()
}

// the row's options menu, found by the email the row shows
async function openMemberOptions(
	wrapper: Awaited<ReturnType<typeof mountSection>>["wrapper"],
	email: string,
) {
	const row = wrapper.findAll("tr").find((r) => r.text().includes(email))
	if (!row) {
		throw new Error(`no member row for "${email}"`)
	}

	await row.get("[data-slot='dropdown-menu-trigger']").trigger("click")
}

// the open list is teleported into <body>. An option shows the role's name
// above its description, so it is found by the name's line.
function pickRole(label: string) {
	const option = Array.from(
		document.body.querySelectorAll("[role='option']"),
	).find(
		(o) =>
			o.querySelector(".text-popover-foreground")?.textContent.trim() === label,
	)
	if (!option) {
		throw new Error(`no role option "${label}"`)
	}

	option.dispatchEvent(
		new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
	)
}

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
