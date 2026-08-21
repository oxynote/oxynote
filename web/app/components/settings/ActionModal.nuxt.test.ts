import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import ActionModal from "./ActionModal.vue"
import WorkspaceURLChangeAction from "./WorkspaceURLChangeAction.vue"
import type { OrganizationMember } from "./workspace"
import {
	clearTeleportedOverlays,
	emitFrom,
	mockAuthOrganization,
	seedAuthAccounts,
	seedAuthSession,
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

const MEMBER: OrganizationMember = {
	id: "member-1",
	organizationId: "org-1",
	userId: "u2",
	role: "member",
	user: { name: "Linus", email: "linus@oxynote.test" },
}

function mountModal(props: Record<string, unknown>) {
	return mountSuspended(ActionModal, { props: props })
}

function dialogText() {
	return (
		document.body.querySelector("[data-slot='dialog-content']")?.textContent ??
		""
	)
}

// the dialog body is teleported into the shared <body> and the query cache
// is app-wide
describe("<ActionModal>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		seedAuthSession({ id: "u1", email: "ada@oxynote.test", name: "ada" })
		seedAuthAccounts(["credential"])
		mockAuthOrganization({
			id: "org-1",
			name: "Acme",
			slug: "acme-corp",
			members: [],
			invitations: [],
		})
		mockEndpoint("GET", "/api/data-sources", () => [])
	})

	afterEach(disposeMockEndpoints)

	it("stays closed while no action is picked", async ({ expect }) => {
		await mountModal({ modelValue: null })

		expect(
			document.body.querySelector("[data-slot='dialog-content']"),
		).toBeNull()
	})

	it.for([
		{ action: "email-change", expected: "Change Email" },
		{ action: "account-deletion", expected: "Delete Account" },
		{ action: "url-change", expected: "Change Workspace URL" },
		{ action: "workspace-invitation", expected: "Invite to Your Workspace" },
	])("titles the $action modal", async ({ action, expected }, { expect }) => {
		await mountModal({ modelValue: action })

		expect(dialogText()).toContain(expected)
	})

	it("titles the password modal as a change when a password is set", async ({
		expect,
	}) => {
		await mountModal({ modelValue: "password-change" })

		expect(dialogText()).toContain("Change Password")
	})

	it("titles the password modal as a first-time setup when none is set", async ({
		expect,
	}) => {
		seedAuthAccounts(["github"])

		await mountModal({ modelValue: "password-change" })

		expect(dialogText()).toContain("Set Password")
	})

	it("titles the invitation modal differently once the workspace is full", async ({
		expect,
	}) => {
		mockAuthOrganization({
			id: "org-1",
			members: Array.from({ length: 5 }, (_, i) => ({ id: `m${i}` })),
			invitations: [],
		})

		await mountModal({ modelValue: "workspace-invitation" })

		expect(dialogText()).toContain("Max Members Reached")
	})

	it("names the member in the removal modal", async ({ expect }) => {
		await mountModal({ modelValue: "workspace-member-removal", opts: MEMBER })

		expect(dialogText()).toContain("Remove from Your Workspace")
		expect(dialogText()).toContain("Linus")
	})

	it("renders nothing for a member removal without a member", async ({
		expect,
	}) => {
		await mountModal({ modelValue: "workspace-member-removal", opts: null })

		expect(dialogText()).toBe("")
	})

	it("renders nothing for a member removal whose target is not a member", async ({
		expect,
	}) => {
		await mountModal({
			modelValue: "workspace-member-removal",
			opts: DATA_SOURCE,
		})

		expect(dialogText()).toBe("")
	})

	it("titles the data source creation modal after its type", async ({
		expect,
	}) => {
		await mountModal({
			modelValue: "data-source-creation",
			opts: DataSourceType.PostgreSQL,
		})

		expect(dialogText()).toContain("Connect PostgreSQL")
	})

	it("titles the data source update modal after the target's type", async ({
		expect,
	}) => {
		await mountModal({ modelValue: "data-source-update", opts: DATA_SOURCE })

		expect(dialogText()).toContain("Update Prometheus")
	})

	it("titles the data source removal modal after the target's type", async ({
		expect,
	}) => {
		await mountModal({ modelValue: "data-source-removal", opts: DATA_SOURCE })

		expect(dialogText()).toContain("Remove Prometheus Data Source")
		expect(dialogText()).toContain("Prod metrics")
	})

	it.for(["data-source-creation", "data-source-update", "data-source-removal"])(
		"renders nothing for %s without a target",
		async (action, { expect }) => {
			await mountModal({ modelValue: action, opts: null })

			expect(dialogText()).toBe("")
		},
	)

	it("renders nothing for an action it does not know", async ({ expect }) => {
		await mountModal({ modelValue: "teleport-the-workspace" })

		expect(dialogText()).toBe("")
	})

	it("clears the picked action when the close button is pressed", async ({
		expect,
	}) => {
		const wrapper = await mountModal({ modelValue: "email-change" })

		teleportedButton("Close").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("clears the picked action when the child action closes", async ({
		expect,
	}) => {
		const wrapper = await mountModal({ modelValue: "email-change" })

		teleportedButton("Cancel").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("passes a slug refresh from the child on to its parent", async ({
		expect,
	}) => {
		const wrapper = await mountModal({ modelValue: "url-change" })

		emitFrom(wrapper, WorkspaceURLChangeAction, "refresh-organization-slug")
		await nextTick()

		expect(wrapper.emitted("refresh-organization-slug")).toHaveLength(1)
	})
})
