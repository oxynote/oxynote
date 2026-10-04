import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import HookConfigPanel from "../HookConfigPanel.vue"
import { DiffStatus } from "../../diff/position-map"
import {
	hookNotice,
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	readonlyFields,
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
	emitFrom,
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
const HOOK_PATH = `/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`

const TITLE = "editor.hooks.url-watcher.title"
const SHOWN_URL = "oxynote.test/docs"

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
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().setActiveBranchProtected(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("offers to start watching a website", async ({ expect }) => {
		await mountMenu()

		expect(menuText()).toContain(t(TITLE))
		expect(menuText()).toContain(t("editor.hooks.url-watcher.description"))
	})

	it("names the page an active hook watches", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		expect(menuText()).toContain(SHOWN_URL)
		expect(menuText()).not.toContain("https://")
	})

	it("says the page of a triggered hook changed", async ({ expect }) => {
		await mountMenu({ hook: urlHook({ score: "0" }) })

		const detail = t("editor.hooks.url-watcher.detail-triggered")
		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", { subtext: SHOWN_URL, detail: detail }),
		)
	})

	it("says the page of a failing hook is unreachable", async ({ expect }) => {
		await mountMenu({
			hook: urlHook({ state: {}, status: "unreachable_url" }),
		})

		const detail = t("editor.hooks.url-watcher.detail-unreachable")
		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", { subtext: SHOWN_URL, detail: detail }),
		)
	})

	it("says change detection is not set up for a hook that needs it", async ({
		expect,
	}) => {
		await mountMenu({ hook: urlHook({ status: "unconfigured" }) })

		const detail = t("editor.hooks.url-watcher.detail-unconfigured")
		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", { subtext: SHOWN_URL, detail: detail }),
		)
	})

	it("prefills the address of the hook it edits", async ({ expect }) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu(t(TITLE))

		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"https://oxynote.test/docs",
		)
	})

	it.for([
		{
			name: "block",
			nodeId: "block-1",
			key: "editor.hooks.url-watcher.fresh-notice",
		},
		{
			name: "page",
			nodeId: null,
			key: "editor.hooks.url-watcher.fresh-notice",
		},
	])(
		"explains what a $name hook will do",
		async ({ nodeId, key }, { expect }) => {
			await mountMenu({ hook: urlHook(), nodeId: nodeId })

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().dataset.hookStatus).toBe("fresh")
			expect(hookNotice().textContent.trim()).toBe(t(key, { url: SHOWN_URL }))
		},
	)

	it("names the page in its notice as the reader types it", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(TITLE))

		await typeInMenu("https://oxynote.test/guide/")

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.url-watcher.new-notice", {
				url: "oxynote.test/guide",
			}),
		)
		expect(hookNotice().querySelector(".font-semibold")?.textContent).toBe(
			"oxynote.test/guide",
		)
	})

	it("names no page in its notice before one is typed", async ({ expect }) => {
		await mountMenu()

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.url-watcher.new-notice", {
				url: t("editor.hooks.url-watcher.url-fallback"),
			}),
		)
		expect(hookNotice().querySelector(".font-semibold")).toBeNull()
	})

	it("warns about a site it cannot reach", async ({ expect }) => {
		await mountMenu({
			hook: urlHook({ state: {}, status: "unreachable_url" }),
		})

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("needs-attention")
		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.url-watcher.unreachable-url"),
		)
	})

	it("keeps the create button out of reach until an address is typed", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(TITLE))

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a hook for the address the reader typed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
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

	it("starts empty again once a hook is created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, () => ({
			id: HOOK_ID,
		}))
		await mountMenu()
		await openHookSubMenu(t(TITLE))
		await typeInMenu("oxynote.test/docs")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await openHookSubMenu(t(TITLE))
		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"",
		)
	})

	it("keeps the typed address when the hook cannot be created", async ({
		expect,
	}) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		await mountMenu()
		await openHookSubMenu(t(TITLE))
		await typeInMenu("oxynote.test/docs")

		menuButton(t("editor.hooks.create")).click()
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)

		await openHookSubMenu(t(TITLE))
		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"oxynote.test/docs",
		)
	})

	it("keeps the update button out of reach until the address changes", async ({
		expect,
	}) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu(t(TITLE))

		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("sends nothing while no address is typed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		emitFrom(wrapper, HookConfigPanel, "submit")
		await settleMutations()

		expect(calls).toHaveLength(0)
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toBeUndefined()
	})

	it("updates the address an existing hook watches", async ({ expect }) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu(t(TITLE))
		await typeInMenu("other.test")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: { url: "https://other.test" },
		})
	})

	it("deletes the hook", async ({ expect }) => {
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		await mountMenu({ hook: urlHook() })
		await openHookSubMenu(t(TITLE))

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("dismisses a triggered hook from its notice", async ({ expect }) => {
		const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
			id: HOOK_ID,
		}))
		await mountMenu({ hook: urlHook({ score: "0" }) })
		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("triggered")
		expect(hookNotice().textContent).toContain(
			t("editor.hooks.url-watcher.triggered-notice", { url: SHOWN_URL }),
		)
		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers no dismissal for a hook that has not triggered", async ({
		expect,
	}) => {
		await mountMenu({ hook: urlHook() })

		await openHookSubMenu(t(TITLE))

		expect(menuText()).not.toContain(t("editor.hooks.reset"))
	})

	describe("when the page is read only", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorMeta().setEditable(false)
		})

		it("shows the address without a way to change it", async ({ expect }) => {
			await mountMenu({ hook: urlHook() })

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				["https://oxynote.test/docs", "unchanged"],
			])
			expect(menuText()).toContain(t("editor.hooks.read-only-mode"))
			expect(menuText()).not.toContain(t("editor.hooks.update"))
			expect(menuText()).not.toContain(t("editor.hooks.delete"))
		})

		it("still dismisses a triggered hook", async ({ expect }) => {
			const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
				id: HOOK_ID,
			}))
			await mountMenu({ hook: urlHook({ score: "0" }) })
			await openHookSubMenu(t(TITLE))

			menuButton(t("editor.hooks.reset")).click()

			await vi.waitFor(() => {
				expect(calls).toHaveLength(1)
			}, WAIT_FOR_OPTIONS)
		})

		it.for([
			{
				name: "a block",
				nodeId: "block-1",
				key: "editor.hooks.url-watcher.triggered-notice",
			},
			{
				name: "the page",
				nodeId: null,
				key: "editor.hooks.url-watcher.triggered-notice",
			},
		])(
			"says what a triggered hook on $name did",
			async ({ nodeId, key }, { expect }) => {
				await mountMenu({ hook: urlHook({ score: "0" }), nodeId: nodeId })

				await openHookSubMenu(t(TITLE))

				expect(hookNotice().textContent).toContain(t(key, { url: SHOWN_URL }))
			},
		)
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorStore().setReviewableDiffActive(true)
		})

		it("shows the new url over the old one", async ({ expect }) => {
			await mountMenu({
				hook: urlHook(),
				diff: {
					status: DiffStatus.Modified,
					targetHook: urlHook({ settings: { url: "https://old.test" } }),
					signColumn: false,
				},
			})

			expect(menuText()).toContain(
				t("editor.hooks.value-change", { old: "old.test", new: SHOWN_URL }),
			)
			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				["https://oxynote.test/docs", "added"],
				["https://old.test", "removed"],
			])
			expect(menuText()).not.toContain(t("editor.hooks.update"))
		})

		it.for([
			{ name: "added", status: DiffStatus.Added },
			{ name: "removed", status: DiffStatus.Removed },
		])(
			"marks every field of an $name hook",
			async ({ status, name }, { expect }) => {
				await mountMenu({
					hook: urlHook(),
					diff: { status: status, targetHook: null, signColumn: true },
				})

				await openHookSubMenu(t(TITLE))

				expect(readonlyFields()).toEqual([["https://oxynote.test/docs", name]])
			},
		)

		it("says a removed hook was removed", async ({ expect }) => {
			await mountMenu({
				hook: urlHook(),
				diff: {
					status: DiffStatus.Removed,
					targetHook: null,
					signColumn: true,
				},
			})

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().textContent.trim()).toBe(t("editor.hooks.removed"))
		})

		it("keeps a triggered hook from being dismissed", async ({ expect }) => {
			await mountMenu({
				hook: urlHook({ score: "0" }),
				diff: {
					status: DiffStatus.Unchanged,
					targetHook: null,
					signColumn: false,
				},
			})

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().dataset.hookStatus).toBe("triggered")
			expect(menuText()).not.toContain(t("editor.hooks.reset"))
		})
	})
})
