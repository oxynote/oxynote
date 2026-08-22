import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import BaseModal from "./BaseModal.vue"
import DataSourceSection from "./DataSourceSection.vue"
import ProfileSection from "./ProfileSection.vue"
import WorkspaceSection from "./WorkspaceSection.vue"
import {
	clearTeleportedOverlays,
	emitFrom,
	mockAuthOrganization,
	seedAuthAccounts,
	seedAuthSession,
	t,
	teleportedButton,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DATA_SOURCE: DataSource = {
	id: "ds1".padEnd(20, "0"),
	name: "Prod metrics",
	type: DataSourceType.Prometheus,
	url: "http://prometheus.test",
	status: DataSourceStatus.Success,
	createdAt: "2026-01-01T00:00:00Z",
	updatedAt: null,
}

type SettingsTarget = "org-members" | "github" | "metric-data-sources"

function mountModal(open: boolean | SettingsTarget = true) {
	return mountSuspended(BaseModal, { props: { modelValue: open } })
}

// the settings modal and the action modal it hosts are two separate
// dialogs, each teleported into <body> in whichever order they mounted
function dialogText() {
	return Array.from(
		document.body.querySelectorAll("[data-slot='dialog-content']"),
	)
		.map((dialog) => dialog.textContent)
		.join("")
}

// the dialog body is teleported into the shared <body> and the query cache
// is app-wide
describe("<BaseModal>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "ada" })
		seedAuthAccounts(["credential"])
		mockAuthOrganization({
			id: "org-1",
			name: "Acme",
			slug: "acme-corp",
			logo: "",
			members: [],
			invitations: [],
		})
		seedQueryData(["github", "connected"], {
			connected: false,
			configured: true,
		})
		seedQueryData(["slack", "connected"], {
			connected: false,
			configured: true,
		})
		seedQueryData(["data-sources", "list"], [])
		mockEndpoint("GET", "/api/github", () => ({
			connected: false,
			configured: true,
		}))
		mockEndpoint("GET", "/api/slack", () => ({
			connected: false,
			configured: true,
		}))
		mockEndpoint("GET", "/api/data-sources", () => [])
	})

	afterEach(disposeMockEndpoints)

	it("stays closed while the model is false", async ({ expect }) => {
		await mountModal(false)

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).toBeNull()
	})

	it("lists every settings section in order", async ({ expect }) => {
		await mountModal()

		expect(
			Array.from(document.body.querySelectorAll<HTMLElement>("h3")).map(
				(h) => h.textContent,
			),
		).toEqual([
			t("settings.profile.title"),
			t("settings.workspace.title"),
			t("settings.apps.title"),
			t("settings.data-sources.title"),
		])
	})

	it("gives the deep-linkable sections their anchors", async ({ expect }) => {
		await mountModal()

		expect(document.getElementById("settings-org")).not.toBeNull()
		expect(document.getElementById("settings-apps")).not.toBeNull()
		expect(document.getElementById("settings-data-sources")).not.toBeNull()
	})

	it.for([
		{ target: "org-members", anchor: "settings-org" },
		{ target: "github", anchor: "settings-apps" },
		{ target: "metric-data-sources", anchor: "settings-data-sources" },
	] as { target: SettingsTarget; anchor: string }[])(
		"scrolls to $anchor when opened at $target",
		async ({ target, anchor }, { expect }) => {
			const scrolled: string[] = []
			// happy-dom has no layout, so scrollIntoView is a no-op stub — the
			// element it was called on is the observable part
			vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
				function (this: HTMLElement) {
					scrolled.push(this.id)
				},
			)

			// whenever() watches for a change, so the modal has to actually
			// open on the target rather than start out on it
			const wrapper = await mountModal(false)
			await wrapper.setProps({ modelValue: target })
			await nextTick()
			await nextTick()

			expect(scrolled).toContain(anchor)
		},
	)

	it("scrolls nowhere when opened without a target", async ({ expect }) => {
		const scrollIntoView = vi
			.spyOn(HTMLElement.prototype, "scrollIntoView")
			.mockImplementation(() => undefined)

		const wrapper = await mountModal(false)
		await wrapper.setProps({ modelValue: true })
		await nextTick()
		await nextTick()

		expect(scrollIntoView).toHaveBeenCalledTimes(0)
	})

	it("closes when the close button is pressed", async ({ expect }) => {
		const wrapper = await mountModal()

		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([false])
	})

	it.for([
		{
			event: "email-change",
			expectedKey: "settings.action-modals.email-change.title",
		},
		{
			event: "password-change",
			expectedKey: "settings.action-modals.password-change.title",
		},
		{
			event: "account-deletion",
			expectedKey: "settings.action-modals.account-deletion.title",
		},
	])(
		"opens the $event action from the profile section",
		async ({ event, expectedKey }, { expect }) => {
			const wrapper = await mountModal()

			emitFrom(wrapper, ProfileSection, event)
			await nextTick()

			expect(dialogText()).toContain(t(expectedKey))
		},
	)

	it.for([
		{
			event: "url-change",
			expectedKey: "settings.action-modals.workspace-url-change.title",
		},
		{
			event: "invitation",
			expectedKey: "settings.action-modals.workspace-invitation.title",
		},
	])(
		"opens the $event action from the workspace section",
		async ({ event, expectedKey }, { expect }) => {
			const wrapper = await mountModal()

			emitFrom(wrapper, WorkspaceSection, event)
			await nextTick()

			expect(dialogText()).toContain(t(expectedKey))
		},
	)

	it("opens the member removal action with the picked member", async ({
		expect,
	}) => {
		const wrapper = await mountModal()

		emitFrom(wrapper, WorkspaceSection, "member-removal", {
			id: "member-1",
			organizationId: "org-1",
			userId: "u2",
			role: "member",
			user: { name: "Linus", email: "linus@oxynote.test" },
		})
		await nextTick()

		expect(dialogText()).toContain(
			t("settings.action-modals.workspace-member-removal.title"),
		)
		expect(dialogText()).toContain("Linus")
	})

	it("opens the data source creation action for the picked type", async ({
		expect,
	}) => {
		const wrapper = await mountModal()

		emitFrom(
			wrapper,
			DataSourceSection,
			"data-source-creation",
			DataSourceType.MySQL,
		)
		await nextTick()

		expect(dialogText()).toContain(
			t("settings.action-modals.data-source-upsert.title.creation.mysql"),
		)
	})

	it("opens the data source update action for the picked data source", async ({
		expect,
	}) => {
		const wrapper = await mountModal()

		emitFrom(wrapper, DataSourceSection, "data-source-update", DATA_SOURCE)
		await nextTick()

		expect(dialogText()).toContain(
			t("settings.action-modals.data-source-upsert.title.update.prometheus"),
		)
	})

	it("opens the data source removal action for the picked data source", async ({
		expect,
	}) => {
		const wrapper = await mountModal()

		emitFrom(wrapper, DataSourceSection, "data-source-removal", DATA_SOURCE)
		await nextTick()

		expect(dialogText()).toContain(
			t("settings.action-modals.data-source-removal.title.prometheus"),
		)
	})

	it("passes a slug refresh from an action on to its parent", async ({
		expect,
	}) => {
		const wrapper = await mountModal()

		emitFrom(wrapper, "SettingsActionModal", "refresh-organization-slug")
		await nextTick()

		expect(wrapper.emitted("refresh-organization-slug")).toHaveLength(1)
	})
})
