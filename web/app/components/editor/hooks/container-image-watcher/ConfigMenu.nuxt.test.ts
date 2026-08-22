import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import {
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	typeInMenu,
} from "../test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const HOOK_ID = makeXid("hook")

const NEW_HOOK_LABEL = "editor.hooks.container-image-watcher.title"

function imageHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.ContainerImageWatcher,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: {
			image: t("editor.hooks.container-image-watcher.image-input-placeholder"),
		},
		...overrides,
	})
}

function mountMenu(props: Record<string, unknown> = {}) {
	return mountHookMenu(ConfigMenu, { nodeId: "block-1", ...props })
}

// the editor store, the query cache, the mocked toast module and the
// teleported menu bodies are all shared, so these tests cannot interleave
describe("<ContainerImageWatcherConfigMenu>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
	})

	afterEach(disposeMockEndpoints)

	it("offers to start watching a website", async ({ expect }) => {
		await mountMenu()

		expect(menuText()).toContain(t(NEW_HOOK_LABEL))
	})

	it("names the image an active hook watches", async ({ expect }) => {
		await mountMenu({ hook: imageHook() })

		expect(menuText()).toContain("Watching postgres:16.4-alpine")
	})

	it("reports a hook that has already fired", async ({ expect }) => {
		await mountMenu({ hook: imageHook({ score: "0" }) })

		expect(menuText()).toContain("Updates in postgres:16.4-alpine")
	})

	it("prefills the image reference of the hook it edits", async ({
		expect,
	}) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu("Watching postgres:16.4-alpine")

		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			t("editor.hooks.container-image-watcher.image-input-placeholder"),
		)
	})

	it("explains what an active block hook will do", async ({ expect }) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu("Watching postgres:16.4-alpine")

		expect(menuText()).toContain("the block will be highlighted")
	})

	it("explains what an active document-wide hook will do", async ({
		expect,
	}) => {
		await mountMenu({ hook: imageHook(), nodeId: null })

		await openHookSubMenu("Watching postgres:16.4-alpine")

		expect(menuText()).toContain("the relevant sections will be highlighted")
	})

	it("warns about an image it cannot reach", async ({ expect }) => {
		await mountMenu({
			hook: imageHook({
				state: { digest: "sha256:old", status: "unauthorized" },
			}),
		})

		await openHookSubMenu("Watching postgres:16.4-alpine")

		expect(menuText()).toContain("cannot be reached")
	})

	it("keeps the create button out of reach until an image is typed", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a hook for the image the reader typed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await typeInMenu("redis:7")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.ContainerImageWatcher,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: { image: "redis:7" },
		})
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toHaveLength(1)
	})

	it("warns when the hook cannot be created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await typeInMenu("redis:7")

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("updates the image an existing hook watches", async ({ expect }) => {
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => ({ id: HOOK_ID }),
		)
		await mountMenu({ hook: imageHook() })
		await openHookSubMenu("Watching postgres:16.4-alpine")
		await typeInMenu("redis:8")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: { image: "redis:8" },
		})
	})

	it("warns when the hook cannot be updated", async ({ expect }) => {
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: imageHook() })
		await openHookSubMenu("Watching postgres:16.4-alpine")
		await typeInMenu("redis:8")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("deletes the hook", async ({ expect }) => {
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => null,
		)
		await mountMenu({ hook: imageHook() })
		await openHookSubMenu("Watching postgres:16.4-alpine")

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the hook cannot be deleted", async ({ expect }) => {
		mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: imageHook() })
		await openHookSubMenu("Watching postgres:16.4-alpine")

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers to approve a hook that has fired", async ({ expect }) => {
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}/reset`,
			() => ({ id: HOOK_ID }),
		)
		await mountMenu({ hook: imageHook({ score: "0" }) })
		await openHookSubMenu("Updates in postgres:16.4-alpine")

		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the hook cannot be approved", async ({ expect }) => {
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}/reset`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: imageHook({ score: "0" }) })
		await openHookSubMenu("Updates in postgres:16.4-alpine")

		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers no approval for a hook that has not fired", async ({ expect }) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu("Watching postgres:16.4-alpine")

		expect(menuText()).not.toContain(t("editor.hooks.reset"))
	})

	it("creates nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await typeInMenu("redis:7")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		expect(calls).toHaveLength(0)
	})
})
