import { setResponseStatus } from "h3"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import HookConfigPanel from "../HookConfigPanel.vue"
import { DiffStatus } from "../../diff/position-map"
import FileSelectInput from "./FileSelectInput.vue"
import {
	hookNotice,
	iconNames,
	makeHook,
	menuButton,
	menuButtonLabels,
	menuText,
	mountHookMenu,
	openHookSubMenu,
	readonlyFields,
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
	emitFromNth,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import { Select } from "~/components/shadcn/ui/select"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const HOOK_ID = makeXid("hook")
const HOOK_PATH = `/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`

const TITLE = "editor.hooks.github-tracking.title"
const REPOSITORY = "runbooks"

function githubHook(overrides: Partial<DocumentHook> = {}) {
	return makeHook({
		id: HOOK_ID,
		type: DocumentHookType.GitHubTracking,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		settings: {
			repository: REPOSITORY,
			branch: "main",
			paths: ["docs/readme.md"],
		},
		state: { pathsChecksums: {} },
		status: "active",
		...overrides,
	})
}

function mockGitHub(
	options: { connected?: boolean; repositories?: string[] } = {},
) {
	mockEndpoint("GET", "/api/github", () => ({
		connected: options.connected ?? true,
	}))
	mockEndpoint("GET", "/api/github/repositories", () =>
		(options.repositories ?? [REPOSITORY]).map((name) => ({ name: name })),
	)
	mockEndpoint("GET", `/api/github/repositories/${REPOSITORY}/branches`, () => [
		"main",
		"next",
	])
	mockEndpoint("GET", `/api/github/repositories/${REPOSITORY}/tree`, () => [
		{
			type: "file",
			name: "docs/readme.md",
			items: null,
			checksum: "sum-1",
		},
	])
}

function mountMenu(props: Record<string, unknown> = {}) {
	return mountHookMenu(ConfigMenu, { nodeId: "block-1", ...props })
}

async function pickRepository(wrapper: VueWrapper, name: string) {
	emitFrom(wrapper, Select, "update:modelValue", name)
	await nextTick()
}

async function pickBranch(wrapper: VueWrapper, name: string) {
	emitFromNth(wrapper, Select, 1, "update:modelValue", name)
	await nextTick()
}

async function pickPaths(wrapper: VueWrapper, paths: string[]) {
	emitFrom(wrapper, FileSelectInput, "update:modelValue", paths)
	await nextTick()
}

// the editor store, the query cache, the mocked toast module and the
// teleported menu bodies are all shared, so these tests cannot interleave
describe("<GitHubTrackingConfigMenu>", { concurrent: false }, () => {
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

	it("offers to start watching a repository", async ({ expect }) => {
		mockGitHub()

		await mountMenu()

		expect(menuText()).toContain(t(TITLE))
		expect(menuText()).toContain(t("editor.hooks.github-tracking.description"))
	})

	it("names the repository, branch and file count an active hook watches", async ({
		expect,
	}) => {
		mockGitHub()

		await mountMenu({ hook: githubHook() })

		const detail = t("editor.hooks.github-tracking.file-count.one")
		expect(menuText()).toContain(
			t("editor.hooks.subtext-detail", {
				subtext: t("editor.hooks.github-tracking.subtext", {
					repository: REPOSITORY,
					branch: "main",
				}),
				detail: detail,
			}),
		)
	})

	it.for([
		{
			name: "a missing installation",
			status: "missing_installation" as const,
			problem: "editor.hooks.github-tracking.problems.missing-installation",
		},
		{
			name: "a missing repository",
			status: "missing_repository" as const,
			problem: "editor.hooks.github-tracking.problems.missing-repository",
		},
		{
			name: "a missing branch",
			status: "missing_branch" as const,
			problem: "editor.hooks.github-tracking.problems.missing-branch",
		},
		{
			name: "a repository too large to compare",
			status: "tree_truncated" as const,
			problem: "editor.hooks.github-tracking.problems.tree-truncated",
		},
		{
			name: "github missing on the server",
			status: "unconfigured" as const,
			problem: "editor.hooks.github-tracking.problems.unconfigured",
		},
	])(
		"names $name on the hook's row",
		async ({ status, problem }, { expect }) => {
			mockGitHub()

			await mountMenu({ hook: githubHook({ status: status }) })

			expect(menuText()).toContain(
				t("editor.hooks.subtext-detail", {
					subtext: t("editor.hooks.github-tracking.subtext", {
						repository: REPOSITORY,
						branch: "main",
					}),
					detail: t(problem),
				}),
			)
		},
	)

	it("asks the reader to connect github first", async ({ expect }) => {
		mockGitHub({ connected: false })
		const wrapper = await mountMenu()

		await openHookSubMenu(t(TITLE))

		await vi.waitFor(() => {
			expect(hookNotice().textContent).toContain(
				"connect your GitHub organization",
			)
		}, WAIT_FOR_OPTIONS)
		expect(hookNotice().dataset.hookStatus).toBe("fresh")
		expect(iconNames(hookNotice())).toEqual(["mingcute:information-fill"])
		menuButton(
			t("editor.hooks.github-tracking.not-connected.placeholder"),
		).click()
		await nextTick()

		expect(wrapper.findComponent(ConfigMenu).emitted("open-settings")).toEqual([
			["github"],
		])
	})

	it("warns that a hook lost its github connection", async ({ expect }) => {
		mockGitHub()
		await mountMenu({ hook: githubHook({ status: "missing_installation" }) })

		await openHookSubMenu(t(TITLE))

		expect(hookNotice().dataset.hookStatus).toBe("needs-attention")
		expect(hookNotice().textContent).toContain(
			"connect your GitHub organization",
		)
		expect(iconNames(hookNotice())).toEqual(["mingcute:alert-fill"])
	})

	it("says so when the connected account has no repositories", async ({
		expect,
	}) => {
		mockGitHub({ repositories: [] })
		await mountMenu()

		await openHookSubMenu(t(TITLE))

		await vi.waitFor(() => {
			expect(hookNotice().textContent.trim()).toBe(
				t("editor.hooks.github-tracking.no-repositories"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(hookNotice().dataset.hookStatus).toBe("fresh")
	})

	it.for([
		{
			name: "a repository",
			status: "missing_repository" as const,
			key: "editor.hooks.github-tracking.missing-target-repository",
		},
		{
			name: "a branch",
			status: "missing_branch" as const,
			key: "editor.hooks.github-tracking.missing-target-branch",
		},
		{
			name: "a repository too large to compare",
			status: "tree_truncated" as const,
			key: "editor.hooks.github-tracking.tree-truncated",
		},
	])(
		"warns about $name it can no longer reach",
		async ({ status, key }, { expect }) => {
			mockGitHub()
			await mountMenu({ hook: githubHook({ status: status }) })

			await openHookSubMenu(t(TITLE))

			await vi.waitFor(() => {
				expect(hookNotice().textContent.trim()).toBe(t(key))
			}, WAIT_FOR_OPTIONS)
			expect(hookNotice().dataset.hookStatus).toBe("needs-attention")
		},
	)

	// a server without github cannot be fixed from the settings, so the
	// notice does not send the reader there
	it("warns that github is not set up on this server", async ({ expect }) => {
		mockGitHub({ connected: false })
		await mountMenu({ hook: githubHook({ status: "unconfigured" }) })

		await openHookSubMenu(t(TITLE))

		await vi.waitFor(() => {
			expect(hookNotice().textContent.trim()).toBe(
				t("editor.hooks.github-tracking.unconfigured"),
			)
		}, WAIT_FOR_OPTIONS)
		expect(hookNotice().dataset.hookStatus).toBe("needs-attention")
	})

	it.for([
		{
			name: "block",
			nodeId: "block-1",
			key: "editor.hooks.github-tracking.fresh-notice",
		},
		{
			name: "page",
			nodeId: null,
			key: "editor.hooks.github-tracking.fresh-notice",
		},
	])(
		"explains what a $name hook will do",
		async ({ nodeId, key }, { expect }) => {
			mockGitHub()
			await mountMenu({ hook: githubHook(), nodeId: nodeId })

			await openHookSubMenu(t(TITLE))

			await vi.waitFor(() => {
				expect(hookNotice().textContent.trim()).toBe(
					t(key, { files: "docs/readme.md", branch: "main" }),
				)
			}, WAIT_FOR_OPTIONS)
			expect(hookNotice().dataset.hookStatus).toBe("fresh")
		},
	)

	it("names nothing in its notice before anything is picked", async ({
		expect,
	}) => {
		mockGitHub()
		await mountMenu()

		await openHookSubMenu(t(TITLE))

		await vi.waitFor(() => {
			expect(hookNotice().textContent.trim()).toBe(
				t("editor.hooks.github-tracking.new-notice", {
					files: t("editor.hooks.github-tracking.files-fallback"),
					branch: t("editor.hooks.github-tracking.branch-fallback"),
				}),
			)
		}, WAIT_FOR_OPTIONS)
		expect(hookNotice().querySelector(".font-semibold")).toBeNull()
	})

	it("counts the picked files in its notice once there are many", async ({
		expect,
	}) => {
		mockGitHub()
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "next")

		await pickPaths(wrapper, ["a.md", "b.md", "c.md"])

		await vi.waitFor(() => {
			expect(hookNotice().textContent.trim()).toBe(
				t("editor.hooks.github-tracking.new-notice", {
					files: t("editor.hooks.github-tracking.file-count.other", {
						count: 3,
					}),
					branch: "next",
				}),
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("keeps the create button out of reach until everything is picked", async ({
		expect,
	}) => {
		mockGitHub()
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))

		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")

		expect(menuButton(t("editor.hooks.create")).disabled).toBe(true)
	})

	it("creates a hook for the repository, branch and files picked", async ({
		expect,
	}) => {
		mockGitHub()
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")
		await pickPaths(wrapper, ["docs/readme.md"])

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.GitHubTracking,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: {
				repository: REPOSITORY,
				branch: "main",
				paths: ["docs/readme.md"],
			},
		})
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toHaveLength(1)
	})

	it("starts empty again once a hook is created", async ({ expect }) => {
		mockGitHub()
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, () => ({
			id: HOOK_ID,
		}))
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")
		await pickPaths(wrapper, ["docs/readme.md"])

		menuButton(t("editor.hooks.create")).click()
		await settleMutations()

		await openHookSubMenu(t(TITLE))
		const [repositorySelect, branchSelect] = wrapper.findAllComponents(Select)
		expect(repositorySelect?.props("modelValue")).toBeUndefined()
		expect(branchSelect?.props("modelValue")).toBeUndefined()
		expect(
			wrapper.findComponent(FileSelectInput).props("modelValue"),
		).toBeUndefined()
	})

	it("keeps the picks when the hook cannot be created", async ({ expect }) => {
		mockGitHub()
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")
		await pickPaths(wrapper, ["docs/readme.md"])

		menuButton(t("editor.hooks.create")).click()
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)

		await openHookSubMenu(t(TITLE))
		const [repositorySelect, branchSelect] = wrapper.findAllComponents(Select)
		expect(repositorySelect?.props("modelValue")).toBe(REPOSITORY)
		expect(branchSelect?.props("modelValue")).toBe("main")
	})

	it("keeps the update button out of reach until a pick changes", async ({
		expect,
	}) => {
		mockGitHub()
		await mountMenu({ hook: githubHook() })

		await openHookSubMenu(t(TITLE))

		expect(menuButton(t("editor.hooks.update")).disabled).toBe(true)
	})

	it("sends nothing while no files are picked", async ({ expect }) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		mockGitHub()
		const wrapper = await mountMenu()
		await openHookSubMenu(t(TITLE))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")

		emitFrom(wrapper, HookConfigPanel, "submit")
		await settleMutations()

		expect(calls).toHaveLength(0)
		expect(
			wrapper.findComponent(ConfigMenu).emitted("force-close"),
		).toBeUndefined()
	})

	it("updates what an existing hook watches", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		const wrapper = await mountMenu({ hook: githubHook() })
		await openHookSubMenu(t(TITLE))
		await pickBranch(wrapper, "next")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({
			settings: {
				repository: REPOSITORY,
				branch: "next",
				paths: ["docs/readme.md"],
			},
		})
	})

	it("deletes the hook", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		await mountMenu({ hook: githubHook() })
		await openHookSubMenu(t(TITLE))

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("dismisses a triggered hook from its notice", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
			id: HOOK_ID,
		}))
		await mountMenu({ hook: githubHook({ score: "0" }) })
		await openHookSubMenu(t(TITLE))

		await vi.waitFor(() => {
			expect(hookNotice().textContent).toContain(
				t("editor.hooks.github-tracking.triggered-notice", {
					branch: "main",
				}),
			)
		}, WAIT_FOR_OPTIONS)
		expect(hookNotice().dataset.hookStatus).toBe("triggered")
		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers no dismissal while the hook cannot check its files", async ({
		expect,
	}) => {
		mockGitHub()
		await mountMenu({
			hook: githubHook({
				score: "0",
				status: "missing_branch",
			}),
		})

		await openHookSubMenu(t(TITLE))

		expect(menuButtonLabels()).not.toContain(t("editor.hooks.reset"))
	})

	describe("when the page is read only", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorMeta().setEditable(false)
		})

		it("shows the settings without a way to change them", async ({
			expect,
		}) => {
			mockGitHub({ connected: false })
			await mountMenu({ hook: githubHook() })

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[REPOSITORY, "unchanged"],
				["main", "unchanged"],
				["docs/readme.md", "unchanged"],
			])
			expect(menuText()).toContain(t("editor.hooks.read-only-mode"))
			expect(menuButtonLabels()).toEqual([])
			// connecting github is setup for an editor, so a working hook
			// says what it will do instead
			expect(hookNotice().textContent.trim()).toBe(
				t("editor.hooks.github-tracking.fresh-notice", {
					files: "docs/readme.md",
					branch: "main",
				}),
			)
		})

		it("still dismisses a triggered hook", async ({ expect }) => {
			mockGitHub()
			const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
				id: HOOK_ID,
			}))
			await mountMenu({ hook: githubHook({ score: "0" }) })
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
				key: "editor.hooks.github-tracking.triggered-notice",
			},
			{
				name: "the page",
				nodeId: null,
				key: "editor.hooks.github-tracking.triggered-notice",
			},
		])(
			"says what a triggered hook on $name did",
			async ({ nodeId, key }, { expect }) => {
				mockGitHub()
				await mountMenu({ hook: githubHook({ score: "0" }), nodeId: nodeId })

				await openHookSubMenu(t(TITLE))

				expect(hookNotice().textContent).toContain(t(key, { branch: "main" }))
			},
		)
	})

	describe("when the diff is shown", { concurrent: false }, () => {
		beforeEach(() => {
			useEditorStore().setReviewableDiffActive(true)
		})

		it("shows what changed in each setting", async ({ expect }) => {
			mockGitHub()
			await mountMenu({
				hook: githubHook(),
				diff: {
					status: DiffStatus.Modified,
					targetHook: githubHook({
						settings: {
							repository: REPOSITORY,
							branch: "next",
							paths: ["docs/readme.md", "docs/old.md"],
						},
					}),
					signColumn: false,
				},
			})

			await openHookSubMenu(t(TITLE))

			expect(readonlyFields()).toEqual([
				[REPOSITORY, "unchanged"],
				["main", "added"],
				["next", "removed"],
				["docs/readme.md", "unchanged"],
				["docs/old.md", "removed"],
			])
			expect(menuButtonLabels()).toEqual([])
		})

		it.for([
			{ name: "added", status: DiffStatus.Added },
			{ name: "removed", status: DiffStatus.Removed },
		])(
			"marks every field of an $name hook",
			async ({ status, name }, { expect }) => {
				mockGitHub()
				await mountMenu({
					hook: githubHook(),
					diff: { status: status, targetHook: null, signColumn: true },
				})

				await openHookSubMenu(t(TITLE))

				expect(readonlyFields()).toEqual([
					[REPOSITORY, name],
					["main", name],
					["docs/readme.md", name],
				])
			},
		)
	})
})
