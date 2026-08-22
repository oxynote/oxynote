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

const NEW_HOOK_LABEL = "editor.hooks.url-watcher.title"

function urlHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.URLWatcher,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: { url: "https://oxynote.test/docs" },
		...overrides,
	})
}

function mountMenu(props: Record<string, unknown> = {}) {
	return mountHookMenu(ConfigMenu, { nodeId: "block-1", ...props })
}

// the editor store, the query cache, the mocked toast module and the
// teleported menu bodies are all shared, so these tests cannot interleave
describe("<URLWatcherConfigMenu>", { concurrent: false }, () => {
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

	it("names the site an active hook watches", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		expect(menuText()).toContain("Watching oxynote.test")
	})

	it("reports a hook that has already fired", async ({ expect }) => {
		await mountMenu({ hook: urlHook({ score: "0" }) })

		expect(menuText()).toContain("Changes in oxynote.test")
	})

	it("prefills the address of the hook it edits", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu("Watching oxynote.test")

		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"https://oxynote.test/docs",
		)
	})

	it("explains what an active block hook will do", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu("Watching oxynote.test")

		expect(menuText()).toContain("the block will be highlighted")
	})

	it("explains what an active document-wide hook will do", async ({
		expect,
	}) => {
		await mountMenu({ hook: urlHook(), nodeId: null })

		await openHookSubMenu("Watching oxynote.test")

		expect(menuText()).toContain("the relevant sections will be highlighted")
	})

	it("warns about a site it cannot reach", async ({ expect }) => {
		await mountMenu({
			hook: urlHook({ state: { status: "unreachable_url" } }),
		})

		await openHookSubMenu("Watching oxynote.test")

		expect(menuText()).toContain("cannot be reached")
	})

	it("keeps the create button out of reach until an address is typed", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a hook for the address the reader typed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await typeInMenu("oxynote.test/docs")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.URLWatcher,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: { url: "https://oxynote.test/docs" },
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
		await typeInMenu("oxynote.test/docs")

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("updates the address an existing hook watches", async ({ expect }) => {
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => ({ id: HOOK_ID }),
		)
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu("Watching oxynote.test")
		await typeInMenu("https://other.test")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: { url: "https://other.test" },
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
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu("Watching oxynote.test")
		await typeInMenu("https://other.test")

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
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu("Watching oxynote.test")

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
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu("Watching oxynote.test")

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
		await mountMenu({ hook: urlHook({ score: "0" }) })
		await openHookSubMenu("Changes in oxynote.test")

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
		await mountMenu({ hook: urlHook({ score: "0" }) })
		await openHookSubMenu("Changes in oxynote.test")

		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers no approval for a hook that has not fired", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu("Watching oxynote.test")

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
		await typeInMenu("oxynote.test")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		expect(calls).toHaveLength(0)
	})
})
