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
	menuButtonLabels,
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

const TITLE = "editor.hooks.container-image-watcher.title"
const IMAGE = "postgres:16.4-alpine"

function imageHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.ContainerImageWatcher,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: { image: IMAGE },
		state: { digest: "sha256:old" },
		status: "active",
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
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().setActiveBranchProtected(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("offers to start watching an image", async ({ expect }) => {
		await mountMenu()

		expect(menuText()).toContain(t(TITLE))
		expect(menuText()).toContain(
			t("editor.hooks.container-image-watcher.description"),
		)
	})

	it("names the image an active hook watches", async ({ expect }) => {
		await mountMenu({ hook: imageHook() })

		expect(menuText()).toContain(IMAGE)
	})

	it.for([
		{
			name: "an unreachable image",
			input: "unauthorized",
			expected: "editor.hooks.container-image-watcher.detail-unreachable",
		},
		{
			name: "an image gone from its registry",
			input: "image_not_found",
			expected: "editor.hooks.container-image-watcher.detail-missing",
		},
	] as const)(
		"names $name on the hook's row",
		async ({ input, expected }, { expect }) => {
			await mountMenu({ hook: imageHook({ status: input }) })

			expect(menuText()).toContain(
				t("editor.hooks.subtext-detail", {
					subtext: IMAGE,
					detail: t(expected),
				}),
			)
		},
	)

	it("prefills the image reference of the hook it edits", async ({
		expect,
	}) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu(t(TITLE))

		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			IMAGE,
		)
	})

	it.for([
		{
			name: "block",
			nodeId: "block-1",
			key: "editor.hooks.container-image-watcher.fresh-notice",
		},
		{
			name: "page",
			nodeId: null,
			key: "editor.hooks.container-image-watcher.fresh-notice",
		},
	])(
		"explains what a $name hook will do",
		async ({ nodeId, key }, { expect }) => {
			await mountMenu({ hook: imageHook(), nodeId: nodeId })

			await openHookSubMenu(t(TITLE))

			expect(hookNotice().dataset.hookStatus).toBe("fresh")
			expect(hookNotice().textContent.trim()).toBe(t(key, { image: IMAGE }))
		},
	)

	it("names the image in its notice as the reader types it", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(TITLE))

		await typeInMenu("redis:7")

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.container-image-watcher.new-notice", {
				image: "redis:7",
			}),
		)
	})

	it("names no image in its notice before one is typed", async ({ expect }) => {
		await mountMenu()

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().textContent.trim()).toBe(
			t("editor.hooks.container-image-watcher.new-notice", {
				image: t("editor.hooks.container-image-watcher.image-fallback"),
			}),
		)
		expect(hookNotice().querySelector(".font-semibold")).toBeNull()
	})

	it.for([
		{
			name: "an image it cannot reach",
			input: "unauthorized",
			expected: "editor.hooks.container-image-watcher.unreachable-image",
		},
		{
			name: "an image gone from its registry",
			input: "image_not_found",
			expected: "editor.hooks.container-image-watcher.missing-image",
		},
	] as const)("warns about $name", async ({ input, expected }, { expect }) => {
		await mountMenu({ hook: imageHook({ status: input }) })

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("needs-attention")
		expect(hookNotice().textContent.trim()).toBe(t(expected))
	})

	it("keeps the create button out of reach until an image is typed", async ({
		expect,
	}) => {
		await mountMenu()
		await openHookSubMenu(t(TITLE))

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a hook for the image the reader typed", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
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

	it("starts empty again once a hook is created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, () => ({
			id: HOOK_ID,
		}))
		await mountMenu()
		await openHookSubMenu(t(TITLE))
		await typeInMenu("redis:7")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await openHookSubMenu(t(TITLE))
		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"",
		)
	})

	it("keeps the typed image when the hook cannot be created", async ({
		expect,
	}) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		await mountMenu()
		await openHookSubMenu(t(TITLE))
		await typeInMenu("redis:7")

		menuButton(t("editor.hooks.create")).click()
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)

		await openHookSubMenu(t(TITLE))
		expect(document.body.querySelector<HTMLInputElement>("input")?.value).toBe(
			"redis:7",
		)
	})

	it("keeps the update button out of reach until the image changes", async ({
		expect,
	}) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu(t(TITLE))

		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("sends nothing while no image is typed", async ({ expect }) => {
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

	it("updates the image an existing hook watches", async ({ expect }) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		await mountMenu({ hook: imageHook() })
		await openHookSubMenu(t(TITLE))
		await typeInMenu("postgres:17")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({ settings: { image: "postgres:17" } })
	})

	it("deletes the hook", async ({ expect }) => {
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		await mountMenu({ hook: imageHook() })
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
		await mountMenu({ hook: imageHook({ score: "0" }) })
		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("triggered")
		expect(hookNotice().textContent).toContain(
			t("editor.hooks.container-image-watcher.triggered-notice", {
				image: IMAGE,
			}),
		)
		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers no dismissal for a hook that has not triggered", async ({
		expect,
	}) => {
		await mountMenu({ hook: imageHook() })

		await openHookSubMenu(t(TITLE))

		expect(menuText()).not.toContain(t("editor.hooks.reset"))
	})

	describe("when the page is read only", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorMeta().setEditable(false)
		})

		it("shows the image without a way to change it", async ({ expect }) => {
			await mountMenu({ hook: imageHook() })

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([[IMAGE, "unchanged"]])
			expect(menuText()).toContain(t("editor.hooks.read-only-mode"))
			expect(menuButtonLabels()).not.toContain(t("editor.hooks.update"))
			expect(menuButtonLabels()).not.toContain(t("editor.hooks.delete"))
		})

		it("still dismisses a triggered hook", async ({ expect }) => {
			const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
				id: HOOK_ID,
			}))
			await mountMenu({ hook: imageHook({ score: "0" }) })
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
				key: "editor.hooks.container-image-watcher.triggered-notice",
			},
			{
				name: "the page",
				nodeId: null,
				key: "editor.hooks.container-image-watcher.triggered-notice",
			},
		])(
			"says what a triggered hook on $name did",
			async ({ nodeId, key }, { expect }) => {
				await mountMenu({ hook: imageHook({ score: "0" }), nodeId: nodeId })

				await openHookSubMenu(t(TITLE))

				expect(hookNotice().textContent).toContain(t(key, { image: IMAGE }))
			},
		)
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorStore().setReviewableDiffActive(true)
		})

		it("shows the new image over the old one", async ({ expect }) => {
			await mountMenu({
				hook: imageHook(),
				diff: {
					status: DiffStatus.Modified,
					targetHook: imageHook({ settings: { image: "postgres:15" } }),
					signColumn: false,
				},
			})

			expect(menuText()).toContain(
				t("editor.hooks.value-change", { old: "postgres:15", new: IMAGE }),
			)
			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[IMAGE, "added"],
				["postgres:15", "removed"],
			])
			expect(menuButtonLabels()).not.toContain(t("editor.hooks.update"))
		})

		it.for([
			{ name: "added", status: DiffStatus.Added },
			{ name: "removed", status: DiffStatus.Removed },
		])(
			"marks every field of an $name hook",
			async ({ status, name }, { expect }) => {
				await mountMenu({
					hook: imageHook(),
					diff: { status: status, targetHook: null, signColumn: true },
				})

				await openHookSubMenu(t(TITLE))

				expect(readonlyFields()).toEqual([[IMAGE, name]])
			},
		)

		it("keeps a triggered hook from being dismissed", async ({ expect }) => {
			await mountMenu({
				hook: imageHook({ score: "0" }),
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
