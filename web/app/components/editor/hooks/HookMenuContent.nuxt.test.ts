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
		documentHooks: [],
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
		expect(menuText()).toContain(t("editor.hooks.time-expiration.title"))
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
		expect(menuText()).toContain(t("editor.hooks.time-expiration.title"))
	})

	it("lists the hooks already on the block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({ documentHooks: [reminder(), urlWatcher()] })

		expect(menuText()).toContain("Remind on")
		expect(menuText()).toContain("Watching oxynote.test")
	})

	it("leaves out hooks belonging to another block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			documentHooks: [urlWatcher({ blockId: "block-2" })],
		})

		expect(menuText()).not.toContain("Watching oxynote.test")
	})

	it("puts the hooks that have fired above the active ones", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent({
			documentHooks: [reminder(), urlWatcher({ score: "0" })],
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
			documentHooks: [
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
})
