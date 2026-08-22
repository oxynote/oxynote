import { setResponseStatus } from "h3"
import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ConfigMenu from "./ConfigMenu.vue"
import FileSelectInput from "./FileSelectInput.vue"
import {
	makeHook,
	menuButton,
	menuText,
	mountHookMenu,
	openHookSubMenu,
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

const NEW_HOOK_LABEL = "editor.hooks.github-tracking.title"
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
		state: { pathsChecksums: {}, status: "active" },
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
	})

	afterEach(disposeMockEndpoints)

	it("offers to start watching a repository", async ({ expect }) => {
		mockGitHub()

		await mountMenu()

		expect(menuText()).toContain(t(NEW_HOOK_LABEL))
	})

	it("names the repository an active hook watches", async ({ expect }) => {
		mockGitHub()

		await mountMenu({ hook: githubHook() })

		expect(menuText()).toContain(`Watching ${REPOSITORY}`)
	})

	it("reports a hook that has already fired", async ({ expect }) => {
		mockGitHub()

		await mountMenu({ hook: githubHook({ score: "0" }) })

		expect(menuText()).toContain(`Changes in ${REPOSITORY}`)
	})

	it("asks the reader to connect github first", async ({ expect }) => {
		mockGitHub({ connected: false })
		const wrapper = await mountMenu()

		await openHookSubMenu(t(NEW_HOOK_LABEL))

		await vi.waitFor(() => {
			expect(menuText()).toContain("connect your GitHub organization")
		}, WAIT_FOR_OPTIONS)
		menuButton(
			t("editor.hooks.github-tracking.not-connected.placeholder"),
		).click()
		await nextTick()

		expect(wrapper.findComponent(ConfigMenu).emitted("open-settings")).toEqual([
			["github"],
		])
	})

	it("says so when the connected account has no repositories", async ({
		expect,
	}) => {
		mockGitHub({ repositories: [] })
		await mountMenu()

		await openHookSubMenu(t(NEW_HOOK_LABEL))

		await vi.waitFor(() => {
			expect(menuText()).toContain("no accessible repositories")
		}, WAIT_FOR_OPTIONS)
	})

	it("warns about a repository it can no longer reach", async ({ expect }) => {
		mockGitHub()
		await mountMenu({
			hook: githubHook({
				state: { pathsChecksums: {}, status: "missing_repository" },
			}),
		})

		await openHookSubMenu(`Watching ${REPOSITORY}`)

		await vi.waitFor(() => {
			expect(menuText()).toContain(
				"The selected GitHub repository cannot be accessed",
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns about a branch it can no longer reach", async ({ expect }) => {
		mockGitHub()
		await mountMenu({
			hook: githubHook({
				state: { pathsChecksums: {}, status: "missing_branch" },
			}),
		})

		await openHookSubMenu(`Watching ${REPOSITORY}`)

		await vi.waitFor(() => {
			expect(menuText()).toContain(
				"The selected GitHub branch cannot be accessed",
			)
		}, WAIT_FOR_OPTIONS)
	})

	it("explains what an active block hook will do", async ({ expect }) => {
		mockGitHub()
		await mountMenu({ hook: githubHook() })

		await openHookSubMenu(`Watching ${REPOSITORY}`)

		await vi.waitFor(() => {
			expect(menuText()).toContain("the block will be highlighted")
		}, WAIT_FOR_OPTIONS)
	})

	it("explains what an active document-wide hook will do", async ({
		expect,
	}) => {
		mockGitHub()
		await mountMenu({ hook: githubHook(), nodeId: null })

		await openHookSubMenu(`Watching ${REPOSITORY}`)

		await vi.waitFor(() => {
			expect(menuText()).toContain("the relevant sections will be highlighted")
		}, WAIT_FOR_OPTIONS)
	})

	it("keeps the create button out of reach until everything is picked", async ({
		expect,
	}) => {
		mockGitHub()
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))

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
		await openHookSubMenu(t(NEW_HOOK_LABEL))
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

	it("warns when the hook cannot be created", async ({ expect }) => {
		mockGitHub()
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, (_c, event) => {
			setResponseStatus(event, 500)

			return { message: "boom" }
		})
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")
		await pickPaths(wrapper, ["docs/readme.md"])

		menuButton(t("editor.hooks.create")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("updates what an existing hook watches", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu({ hook: githubHook() })
		await openHookSubMenu(`Watching ${REPOSITORY}`)
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

	it("warns when the hook cannot be updated", async ({ expect }) => {
		mockGitHub()
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountMenu({ hook: githubHook() })
		await openHookSubMenu(`Watching ${REPOSITORY}`)
		await pickBranch(wrapper, "next")

		menuButton(t("editor.hooks.update")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("deletes the hook", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			() => null,
		)
		await mountMenu({ hook: githubHook() })
		await openHookSubMenu(`Watching ${REPOSITORY}`)

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the hook cannot be deleted", async ({ expect }) => {
		mockGitHub()
		mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: githubHook() })
		await openHookSubMenu(`Watching ${REPOSITORY}`)

		menuButton(t("editor.hooks.delete")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("offers to approve a hook that has fired", async ({ expect }) => {
		mockGitHub()
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}/reset`,
			() => ({ id: HOOK_ID }),
		)
		await mountMenu({ hook: githubHook({ score: "0" }) })
		await openHookSubMenu(`Changes in ${REPOSITORY}`)

		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("warns when the hook cannot be approved", async ({ expect }) => {
		mockGitHub()
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}/reset`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		await mountMenu({ hook: githubHook({ score: "0" }) })
		await openHookSubMenu(`Changes in ${REPOSITORY}`)

		menuButton(t("editor.hooks.reset")).click()

		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("creates nothing while no page is open", async ({ expect }) => {
		mockGitHub()
		useEditorStore().updateActiveDocumentId(null)
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const wrapper = await mountMenu()
		await openHookSubMenu(t(NEW_HOOK_LABEL))
		await pickRepository(wrapper, REPOSITORY)
		await pickBranch(wrapper, "main")
		await pickPaths(wrapper, ["docs/readme.md"])

		menuButton(t("editor.hooks.create")).click()
		await nextTick()

		expect(calls).toHaveLength(0)
	})
})
