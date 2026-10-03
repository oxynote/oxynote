import { afterEach, beforeEach, describe, it, vi } from "vitest"
import HookMenuContent from "./HookMenuContent.vue"
import {
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	typeInMenu,
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
	settleMutations,
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

function githubHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: "github-1",
		type: DocumentHookType.GitHubTracking,
		settings: { repository: "runbooks", branch: "main", paths: ["a.md"] },
		state: { pathsChecksums: {}, status: "active" },
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
		useEditorStore().setActiveBranchProtected(false)
		useEditorMeta().setEditable(true)
	})

	afterEach(disposeMockEndpoints)

	it("titles the menu and offers to add a hook", async ({ expect }) => {
		mockGitHub(true)

		await mountContent()

		expect(headerText()).toContain(t("editor.hooks.title"))
		expect(menuText()).toContain(t(ADD_NEW))
	})

	it("offers every hook type and what it watches once github is available", async ({
		expect,
	}) => {
		mockGitHub(true)
		await mountContent()

		await openHookSubMenu(t(ADD_NEW))

		await vi.waitFor(() => {
			expect(menuText()).toContain(t("editor.hooks.github-tracking.title"))
		}, WAIT_FOR_OPTIONS)
		for (const type of [
			"scheduled-reminder",
			"github-tracking",
			"url-watcher",
			"container-image-watcher",
		]) {
			expect(menuText()).toContain(t(`editor.hooks.${type}.title`))
			expect(menuText()).toContain(t(`editor.hooks.${type}.description`))
		}
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

	it("closes the type picker once a hook is created", async ({ expect }) => {
		mockGitHub(true)
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: makeXid("hook") }),
		)
		await mountContent()
		await openHookSubMenu(t(ADD_NEW))
		await openHookSubMenu(t("editor.hooks.url-watcher.title"))
		await typeInMenu("oxynote.test")

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(menuText()).not.toContain(t("editor.hooks.url-watcher.description"))
	})

	it("passes on the type picker's call for the github settings", async ({
		expect,
	}) => {
		mockGitHub(true)
		const wrapper = await mountContent()
		await openHookSubMenu(t(ADD_NEW))
		await openHookSubMenu(t("editor.hooks.github-tracking.title"))
		await vi.waitFor(() => {
			expect(menuText()).toContain(
				t("editor.hooks.github-tracking.not-connected.placeholder"),
			)
		}, WAIT_FOR_OPTIONS)

		menuButton(
			t("editor.hooks.github-tracking.not-connected.placeholder"),
		).click()
		await nextTick()

		expect(
			wrapper.findComponent(HookMenuContent).emitted("open-settings"),
		).toEqual([["github"]])
	})

	it("passes on a github hook's call for the settings", async ({ expect }) => {
		mockGitHub(true)
		const wrapper = await mountContent({ activeBranchHooks: [githubHook()] })
		await openHookSubMenu(t("editor.hooks.github-tracking.title"))
		await vi.waitFor(() => {
			expect(menuText()).toContain(
				t("editor.hooks.github-tracking.not-connected.placeholder"),
			)
		}, WAIT_FOR_OPTIONS)

		menuButton(
			t("editor.hooks.github-tracking.not-connected.placeholder"),
		).click()
		await nextTick()

		expect(
			wrapper.findComponent(HookMenuContent).emitted("open-settings"),
		).toEqual([["github"]])
	})

	it("lists the hooks already on the block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({ activeBranchHooks: [reminder(), urlWatcher()] })

		const rows = hookRows().map((row) => row.textContent)
		expect(rows[0]).toContain(t("editor.hooks.scheduled-reminder.title"))
		expect(rows[1]).toContain(t("editor.hooks.url-watcher.title"))
		expect(rows[1]).toContain("oxynote.test")
	})

	it("leaves out hooks belonging to another block", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [urlWatcher({ blockId: "block-2" })],
		})

		expect(menuText()).not.toContain(t("editor.hooks.url-watcher.title"))
	})

	it("lists the hooks in the order they were made", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [
				urlWatcher({
					id: "url-second",
					settings: { url: "https://second.test" },
					createdAt: new Date("2026-02-01T00:00:00Z"),
				}),
				urlWatcher({
					id: "url-first",
					settings: { url: "https://first.test" },
					createdAt: new Date("2026-01-01T00:00:00Z"),
				}),
			],
		})

		const rows = hookRows().map((row) => row.textContent)
		expect(rows[0]).toContain("first.test")
		expect(rows[1]).toContain("second.test")
	})

	it("lists hooks made at the same moment by their id", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [
				urlWatcher({ id: "url-b", settings: { url: "https://b.test" } }),
				urlWatcher({ id: "url-a", settings: { url: "https://a.test" } }),
			],
		})

		const rows = hookRows().map((row) => row.textContent)
		expect(rows[0]).toContain("a.test")
		expect(rows[1]).toContain("b.test")
	})

	it.for([
		{
			name: "a hook that was updated last",
			input: { updatedAt: new Date("2026-03-01T00:00:00Z") },
		},
		{ name: "a triggered hook", input: { score: "0" } },
		{
			name: "a hook that needs attention",
			input: { score: "0", state: { status: "unreachable_url" as const } },
		},
	])("keeps $name in its place", async ({ input }, { expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [
				urlWatcher({
					id: "url-first",
					settings: { url: "https://first.test" },
					createdAt: new Date("2026-01-01T00:00:00Z"),
					updatedAt: new Date("2026-02-01T00:00:00Z"),
					...input,
				}),
				urlWatcher({
					id: "url-second",
					settings: { url: "https://second.test" },
					createdAt: new Date("2026-01-02T00:00:00Z"),
					updatedAt: new Date("2026-01-02T00:00:00Z"),
				}),
			],
		})

		const rows = hookRows().map((row) => row.textContent)
		expect(rows[0]).toContain("first.test")
		expect(rows[1]).toContain("second.test")
	})

	it("says all is fresh while no hook needs a look", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({ activeBranchHooks: [reminder(), urlWatcher()] })

		expect(headerText()).toContain(t("editor.hooks.status.fresh"))
	})

	it("counts the triggered hooks in its header", async ({ expect }) => {
		mockGitHub(true)

		await mountContent({
			activeBranchHooks: [reminder(), urlWatcher({ score: "0" })],
		})

		expect(headerText()).toContain(
			t("editor.hooks.status.triggered", { count: 1 }),
		)
	})

	it("explains an empty block and offers to add a hook", async ({ expect }) => {
		mockGitHub(true)

		await mountContent()

		expect(menuText()).toContain(t("editor.hooks.empty-block"))
		expect(menuText()).toContain(t("editor.hooks.empty-block-description"))
		expect(hookRows()).toHaveLength(1)
		expect(hookRows()[0]?.textContent).toContain(t(ADD_NEW))
	})

	it.for([
		{
			name: "block",
			nodeId: "block-1",
			title: "editor.hooks.empty-block",
			description: "editor.hooks.empty-block-description",
		},
		{
			name: "page",
			nodeId: null,
			title: "editor.hooks.empty-document",
			description: "editor.hooks.empty-document-description",
		},
	])(
		"explains a $name without hooks on a read only page",
		async ({ nodeId, title, description }, { expect }) => {
			mockGitHub(true)
			useEditorMeta().setEditable(false)

			await mountContent({ nodeId: nodeId })

			expect(menuText()).toContain(t(title))
			expect(menuText()).toContain(t(description))
			expect(hookRows()).toHaveLength(0)
		},
	)

	it("offers no add action on a read only page", async ({ expect }) => {
		mockGitHub(true)
		useEditorMeta().setEditable(false)

		await mountContent({ activeBranchHooks: [urlWatcher()] })

		expect(menuText()).toContain(t("editor.hooks.url-watcher.title"))
		expect(menuText()).not.toContain(t(ADD_NEW))
		expect(menuText()).not.toContain(t("editor.hooks.empty-block"))
	})

	it("lists the active hooks alone while the diff is off", async ({
		expect,
	}) => {
		mockGitHub(true)

		await mountContent({ targetBranchHooks: [urlWatcher()] })

		expect(menuText()).not.toContain(t("editor.hooks.url-watcher.title"))
		expect(menuText()).toContain(t(ADD_NEW))
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			mockGitHub(true)
			useEditorStore().setReviewableDiffActive(true)
		})

		it("offers no add action and says how to edit", async ({ expect }) => {
			await mountContent({ activeBranchHooks: [urlWatcher()] })

			expect(menuText()).not.toContain(t(ADD_NEW))
			expect(menuText().replace(/\s+/g, " ")).toContain(
				t("editor.hooks.showing-changes", {
					toggle: t("editor.name-editor.review-workflow.show-diff"),
				}),
			)
		})

		it("lists a hook the active branch added", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [],
			})

			const row = hookRows()[0]
			expect(row?.textContent).toContain(t("editor.hooks.url-watcher.title"))
			expect(row?.textContent).toContain(t("editor.hooks.diff.added"))
			expect(row?.classList).toContain("bg-diff-added/30")
		})

		it("lists a hook the active branch removed", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [],
				targetBranchHooks: [urlWatcher()],
			})

			const row = hookRows()[0]
			expect(row?.textContent).toContain(t("editor.hooks.url-watcher.title"))
			expect(row?.textContent).toContain(t("editor.hooks.diff.removed"))
			expect(row?.classList).toContain("bg-diff-removed/30")
		})

		it("leaves a removed hook out of the header's count", async ({
			expect,
		}) => {
			await mountContent({
				activeBranchHooks: [reminder()],
				targetBranchHooks: [reminder(), urlWatcher({ score: "0" })],
			})

			expect(headerText()).toContain(t("editor.hooks.status.fresh"))
			expect(headerText()).not.toContain(
				t("editor.hooks.status.triggered", { count: 1 }),
			)
		})

		it("counts the changed settings of a modified hook", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [
					urlWatcher({
						id: "url-old",
						crossBranchId: "url-1",
						settings: { url: "https://old.test" },
					}),
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

		it("lists a hook that replaced a removed one of its type as one removed and one added", async ({
			expect,
		}) => {
			await mountContent({
				activeBranchHooks: [
					urlWatcher({
						id: "url-new",
						createdAt: new Date("2026-02-01T00:00:00Z"),
					}),
				],
				targetBranchHooks: [
					urlWatcher({ id: "url-old", settings: { url: "https://old.test" } }),
				],
			})

			const rows = hookRows()
			expect(rows).toHaveLength(2)
			expect(rows[0]?.textContent).toContain("old.test")
			expect(rows[0]?.textContent).toContain(t("editor.hooks.diff.removed"))
			expect(rows[1]?.textContent).toContain("oxynote.test")
			expect(rows[1]?.textContent).toContain(t("editor.hooks.diff.added"))
		})

		it("keeps a sign column on every row once a hook was added", async ({
			expect,
		}) => {
			await mountContent({
				activeBranchHooks: [urlWatcher(), reminder()],
				targetBranchHooks: [
					reminder({ id: "reminder-target", crossBranchId: "reminder-100" }),
				],
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
					urlWatcher({
						id: "url-old",
						crossBranchId: "url-1",
						settings: { url: "https://old.test" },
					}),
				],
			})

			expect(hookRows().map((row) => !!row.querySelector(".w-2"))).toEqual([
				false,
			])
		})

		it("leaves an unchanged hook unmarked", async ({ expect }) => {
			await mountContent({
				activeBranchHooks: [urlWatcher()],
				targetBranchHooks: [
					urlWatcher({ id: "url-target", crossBranchId: "url-1" }),
				],
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

function headerText(): string {
	return (
		document.body.querySelector("[data-slot='dropdown-menu-label']")
			?.textContent ?? ""
	)
}
