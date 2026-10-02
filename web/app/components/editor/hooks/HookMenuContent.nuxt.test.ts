import { afterEach, beforeEach, describe, it, vi } from "vitest"
import HookMenuContent from "./HookMenuContent.vue"
import {
	makeHook,
	menuText,
	mountHookMenu,
	openHookSubMenu,
} from "./test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	seedCapabilities,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

const ADD_NEW = "editor.hooks.add-new"

function reminder(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: `reminder-${overrides.score ?? "100"}`,
		type: DocumentHookType.ScheduledReminder,
		settings: {
			scale: "linear",
			duration: "24h",
			schedule: new Date("2026-09-01T10:00:00Z"),
		},
		state: { status: "active" },
		...overrides,
	})
}

function urlWatcher(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: "url-1",
		type: DocumentHookType.URLWatcher,
		settings: { url: "https://oxynote.test" },
		state: { status: "active" },
		...overrides,
	})
}

function mockGitHub(configured: boolean) {
	seedCapabilities({ github: configured })
	mockEndpoint("GET", "/api/github", () => ({
		connected: false,
		configured: configured,
	}))
	mockEndpoint("GET", "/api/github/repositories", () => [])
}

function mountContent(props: Record<string, unknown> = {}) {
	return mountHookMenu(HookMenuContent, {
		activeBranchHooks: [],
		nodeId: "block-1",
		...props,
	})
}

// the editor store, the query cache and the teleported menu bodies are
// all shared, so these tests cannot interleave
describe("<HookMenuContent>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("offers to add a hook", async ({ expect }) => {
		mockGitHub(true)

		await mountContent()

		expect(menuText()).toContain(t(ADD_NEW))
	})

	it("offers every hook type once github is available", async ({ expect }) => {
		mockGitHub(true)
		await mountContent()

		await openHookSubMenu(t(ADD_NEW))

		await vi.waitFor(() => {
			expect(menuText()).toContain(t("editor.hooks.github-tracking.title"))
		}, WAIT_FOR_OPTIONS)
		expect(menuText()).toContain(t("editor.hooks.scheduled-reminder.title"))
		expect(menuText()).toContain(t("editor.hooks.url-watcher.title"))
		expect(menuText()).toContain(
			t("editor.hooks.container-image-watcher.title"),
		)
	})

	it("leaves github out when the server has no integration", async ({
		expect,
	}) => {
		mockGitHub(false)
		await mountContent()

		await openHookSubMenu(t(ADD_NEW))

		await vi.waitFor(() => {
			expect(menuText()).not.toContain(t("editor.hooks.github-tracking.title"))
		}, WAIT_FOR_OPTIONS)
		expect(menuText()).toContain(t("editor.hooks.scheduled-reminder.title"))
	})

	it("leaves website changes out when the server has no changedetection", async ({
		expect,
	}) => {
		mockGitHub(true)
		seedCapabilities({ changeDetection: false })
		await mountContent()

		await openHookSubMenu(t(ADD_NEW))

		await vi.waitFor(() => {
			expect(menuText()).toContain(t("editor.hooks.github-tracking.title"))
		}, WAIT_FOR_OPTIONS)
		expect(menuText()).not.toContain(t("editor.hooks.url-watcher.title"))
		expect(menuText()).toContain(t("editor.hooks.scheduled-reminder.title"))
	})

	it("lists the hooks already on the block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({ activeBranchHooks: [reminder(), urlWatcher()] })

		expect(menuText()).toContain("Remind on")
		expect(menuText()).toContain("Watching oxynote.test")
	})

	it("leaves out hooks belonging to another block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [urlWatcher({ blockId: "block-2" })],
		})

		expect(menuText()).not.toContain("Watching oxynote.test")
	})

	it("puts the hooks that have fired above the active ones", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [reminder(), urlWatcher({ score: "0" })],
		})

		const labels = Array.from(
			document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
		).map((item) => item.textContent)

		expect(labels[0]).toContain("Changes in oxynote.test")
		expect(labels[1]).toContain("Remind on")
	})

	it("orders hooks of the same kind by when they last ran", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [
				urlWatcher({
					id: "url-newer",
					settings: { url: "https://newer.test" },
					updatedAt: new Date("2026-02-01T00:00:00Z"),
				}),
				urlWatcher({
					id: "url-older",
					settings: { url: "https://older.test" },
					updatedAt: new Date("2026-01-01T00:00:00Z"),
				}),
			],
		})

		const labels = Array.from(
			document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
		).map((item) => item.textContent)

		expect(labels[0]).toContain("older.test")
		expect(labels[1]).toContain("newer.test")
	})

	it("shows only the add action for a block with no hooks", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent()

		expect(document.body.querySelectorAll("[role^='menuitem']")).toHaveLength(1)
	})

	it("offers no add action on a read only page", async ({ expect }) => {
		mockGitHub(true)
		useEditorMeta().setEditable(false)

		await mountContent({ activeBranchHooks: [urlWatcher()] })

		expect(menuText()).toContain("Watching oxynote.test")
		expect(menuText()).not.toContain(t(ADD_NEW))
	})

	it.for([
		{ name: "block", nodeId: "block-1", key: "editor.hooks.empty-block" },
		{ name: "page", nodeId: null, key: "editor.hooks.empty-document" },
	])(
		"says a $name has no hooks on a read only page",
		async ({ nodeId, key }, { expect }) => {
			mockGitHub(true)
			useEditorMeta().setEditable(false)

			await mountContent({ nodeId: nodeId })

			expect(menuText()).toContain(t(key))
			expect(document.body.querySelectorAll("[role^='menuitem']")).toHaveLength(
				0,
			)
		},
	)

	it("lists the active hooks alone while the diff is off", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent({ targetBranchHooks: [urlWatcher()] })

		expect(menuText()).not.toContain("Watching oxynote.test")
		expect(menuText()).toContain(t(ADD_NEW))
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			mockGitHub(true)
			useEditorStore().setReviewableDiffActive(true)
		})

		it("offers no add action", async ({ expect }) => {
			await mountContent({ activeBranchHooks: [urlWatcher()] })

			expect(menuText()).not.toContain(t(ADD_NEW))
		})

		it("lists a hook the active branch added", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [],
			})

			const row = hookRows()[0]
			expect(row?.textContent).toContain("Watching oxynote.test")
			expect(row?.textContent).toContain(t("editor.hooks.diff.added"))
			expect(row?.classList).toContain("bg-diff-added/30")
		})

		it("lists a hook the active branch removed", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [],
				targetBranchHooks: [urlWatcher()],
			})

			const row = hookRows()[0]
			expect(row?.textContent).toContain("Watching oxynote.test")
			expect(row?.textContent).toContain(t("editor.hooks.diff.removed"))
			expect(row?.classList).toContain("bg-diff-removed/30")
		})

		it("counts the changed settings of a modified hook", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [
					urlWatcher({ id: "url-old", settings: { url: "https://old.test" } }),
				],
			})

			const row = hookRows()[0]
			expect(row?.textContent).toContain(
				t("editor.diff-change-marker.removed", { count: 1 }),
			)
			expect(row?.textContent).toContain(
				t("editor.diff-change-marker.added", { count: 1 }),
			)
		})

		it("keeps a sign column on every row once a hook was added", async ({
			expect,
		}) => {
			await mountContent({
				activeBranchHooks: [urlWatcher(), reminder()],
				targetBranchHooks: [reminder({ id: "reminder-target" })],
			})

			expect(hookRows().map((row) => !!row.querySelector(".w-2"))).toEqual([
				true,
				true,
			])
		})

		it("keeps no sign column when no hook was added or removed", async ({
			expect,
		}) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [
					urlWatcher({ id: "url-old", settings: { url: "https://old.test" } }),
				],
			})

			expect(hookRows().map((row) => !!row.querySelector(".w-2"))).toEqual([
				false,
			])
		})

		it("leaves an unchanged hook unmarked", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [urlWatcher({ id: "url-target" })],
			})

			const row = hookRows()[0]
			expect(row?.textContent).not.toContain(t("editor.hooks.diff.added"))
			expect(row?.textContent).not.toContain(t("editor.hooks.diff.removed"))
			expect(row?.classList).not.toContain("bg-diff-added/30")
			expect(row?.classList).not.toContain("bg-diff-removed/30")
		})
	})
})

// the rows are teleported with the menu, out of the wrapper's reach
function hookRows(): HTMLElement[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	)
}
