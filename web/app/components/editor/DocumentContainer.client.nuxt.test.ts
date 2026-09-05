import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import DocumentContainer from "./DocumentContainer.client.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { at, emitFrom, seedAuthSession } from "~/components/test-helpers"
import { stubThemeColorContext } from "./test-helpers/theme"

// the real provider opens a websocket to the realtime service; the
// stand-in records what it was asked to sync and lets the suite fire the
// callbacks the container passes it
interface ProviderOptions {
	name: string
	onSynced: () => void
	onConnect: () => void
	onAuthenticationFailed: () => void
	onStateless: (message: { payload: string }) => void
}

interface ProviderEntry {
	name: string
	options: ProviderOptions
	destroy: () => void
	destroyed: boolean
}

const providers = vi.hoisted(() => {
	return {
		created: [] as ProviderEntry[],
	}
})

const { showToastMessageMock } = vi.hoisted(() => {
	return { showToastMessageMock: vi.fn() }
})

vi.mock("~/components/toast", () => {
	return { showToastMessage: showToastMessageMock }
})

vi.mock("@hocuspocus/provider", () => {
	function FakeProvider(options: ProviderOptions): ProviderEntry {
		const entry: ProviderEntry = {
			name: options.name,
			options: options,
			destroy: () => {
				entry.destroyed = true
			},
			destroyed: false,
		}

		providers.created.push(entry)

		return entry
	}

	return { HocuspocusProvider: FakeProvider }
})

const redirectToLogin = vi.hoisted(() => vi.fn())

vi.mock("~/plugins/03.api-fetch", async (importOriginal) => ({
	...(await importOriginal<Record<string, unknown>>()),
	redirectToLogin: redirectToLogin,
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("brancha")
const TARGET_BRANCH_ID = makeXid("branchb")

// the three editors each drive a live tiptap instance of their own; the
// container's contract with them is which of them it shows and what it
// does with the editors they hand back
const editorStubs = {
	NameEditor: true,
	ContentEditor: true,
	DiffEditor: true,
}

function seedBranches(...branches: { branchId: string; protected: boolean }[]) {
	seedQueryData(["documents", DOCUMENT_ID, "branches"], branches)
}

function mountContainer(props: Record<string, unknown> = {}) {
	return mountSuspended(DocumentContainer, {
		props: { allInitialSectionsLoaded: true, timestamps: {}, ...props },
		global: { stubs: editorStubs },
	})
}

function providerFor(branchId: string) {
	const entry = providers.created.find(
		(candidate) => candidate.name === `${DOCUMENT_ID}-${branchId}`,
	)
	if (!entry) {
		throw new Error(`no provider was created for branch ${branchId}`)
	}

	return entry
}

async function sync(branchId: string) {
	providerFor(branchId).options.onSynced()
	await nextTick()
}

function bodyShown(wrapper: VueWrapper): boolean {
	return wrapper.findComponent({ name: "NameEditor" }).exists()
}

// the test environment builds a PageTransitionEvent without honouring the
// persisted flag, so it is defined on the event directly.
function pagehide(persisted: boolean): Event {
	const event = new Event("pagehide")

	Object.defineProperty(event, "persisted", { value: persisted })

	return event
}

// the editor store, the auth and query caches and the recorded providers
// are all shared, so these tests cannot interleave
describe("<DocumentContainer>", { concurrent: false }, () => {
	// a container keeps syncing its branches for as long as it is mounted,
	// so a leftover one would answer for the next test too
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		stubThemeColorContext()
		providers.created.splice(0)
		redirectToLogin.mockReset()
		clearQueryCache()
		useEditorMeta().setEditable(true)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().updateTargetBranchId(null)
		useEditorStore().updatePreloadedBranchIds([])
		useEditorStore().setReviewableDiffActive(false)
		seedAuthSession({ id: makeXid("usme"), name: "Me" })
		seedBranches({ branchId: BRANCH_ID, protected: false })
		seedQueryData(["documents", "hooks", DOCUMENT_ID, BRANCH_ID], [])
	})

	afterEach(disposeMockEndpoints)

	it("syncs the active branch under the document and branch id", async ({
		expect,
	}) => {
		await mountContainer()

		expect(providers.created.map((entry) => entry.name)).toEqual([
			`${DOCUMENT_ID}-${BRANCH_ID}`,
		])
	})

	it("syncs the preloaded branches too", async ({ expect }) => {
		useEditorStore().updatePreloadedBranchIds([TARGET_BRANCH_ID])

		await mountContainer()

		expect(providers.created.map((entry) => entry.name)).toContain(
			`${DOCUMENT_ID}-${TARGET_BRANCH_ID}`,
		)
	})

	it("syncs the branch a diff compares against", async ({ expect }) => {
		useEditorStore().updateTargetBranchId(TARGET_BRANCH_ID)

		await mountContainer()

		expect(providers.created.map((entry) => entry.name)).toContain(
			`${DOCUMENT_ID}-${TARGET_BRANCH_ID}`,
		)
	})

	it("syncs each branch only once", async ({ expect }) => {
		useEditorStore().updatePreloadedBranchIds([BRANCH_ID])
		useEditorStore().updateTargetBranchId(BRANCH_ID)

		await mountContainer()

		expect(providers.created).toHaveLength(1)
	})

	it("syncs nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)

		await mountContainer()

		expect(providers.created).toHaveLength(0)
	})

	it("stays empty until the active branch has synced", async ({ expect }) => {
		const wrapper = await mountContainer()

		expect(bodyShown(wrapper)).toBe(false)
	})

	it("shows the editors once the active branch has synced", async ({
		expect,
	}) => {
		const wrapper = await mountContainer()

		await sync(BRANCH_ID)

		expect(bodyShown(wrapper)).toBe(true)
		expect(wrapper.findComponent({ name: "ContentEditor" }).exists()).toBe(true)
	})

	it("stays empty until the sidebar has finished loading", async ({
		expect,
	}) => {
		const wrapper = await mountContainer({ allInitialSectionsLoaded: false })

		await sync(BRANCH_ID)

		expect(bodyShown(wrapper)).toBe(false)
	})

	it("reports the first sync as the initial load", async ({ expect }) => {
		const wrapper = await mountContainer()

		await sync(BRANCH_ID)

		expect(wrapper.emitted("initial-load-complete")).toHaveLength(1)
	})

	it("treats a preloaded branch's sync as no initial load", async ({
		expect,
	}) => {
		useEditorStore().updateTargetBranchId(TARGET_BRANCH_ID)
		const wrapper = await mountContainer()

		await sync(TARGET_BRANCH_ID)

		expect(wrapper.emitted("initial-load-complete")).toBeUndefined()
	})

	it("sends the reader to the login page when the sync is rejected", async ({
		expect,
	}) => {
		await mountContainer()

		providerFor(BRANCH_ID).options.onAuthenticationFailed()

		expect(redirectToLogin).toHaveBeenCalledTimes(1)
	})

	it("tells the reader about an error the realtime service sends", async ({
		expect,
	}) => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {
			return undefined
		})
		showToastMessageMock.mockClear()
		await mountContainer()

		providerFor(BRANCH_ID).options.onStateless({
			payload: JSON.stringify({ type: "error" }),
		})

		expect(consoleError).toHaveBeenCalledTimes(1)
		expect(showToastMessageMock).toHaveBeenCalledTimes(1)
		expect(showToastMessageMock.mock.calls[0]?.[0]).toBe("error")
	})

	it("stays quiet for any other realtime message", async ({ expect }) => {
		const consoleError = vi.spyOn(console, "error").mockImplementation(() => {
			return undefined
		})
		await mountContainer()

		showToastMessageMock.mockClear()

		providerFor(BRANCH_ID).options.onStateless({
			payload: JSON.stringify({ type: "info" }),
		})

		expect(consoleError).toHaveBeenCalledTimes(0)
		expect(showToastMessageMock).toHaveBeenCalledTimes(0)
	})

	it("keeps a protected branch read-only", async ({ expect }) => {
		seedBranches({ branchId: BRANCH_ID, protected: true })

		await mountContainer()

		expect(useEditorMeta().isEditable.value).toBe(false)
	})

	it("lets an unprotected branch be edited", async ({ expect }) => {
		await mountContainer()

		expect(useEditorMeta().isEditable.value).toBe(true)
	})

	it("keeps an unknown branch read-only", async ({ expect }) => {
		seedBranches({ branchId: TARGET_BRANCH_ID, protected: false })

		await mountContainer()

		expect(useEditorMeta().isEditable.value).toBe(false)
	})

	it("passes the content editor on once it is ready", async ({ expect }) => {
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)
		const editor = {
			chain: () => ({ run: () => true }),
			commands: { refreshGapDecorations: vi.fn() },
			setEditable: vi.fn(),
		}

		emitFrom(wrapper, "ContentEditor", "editor-ready", editor)
		await nextTick()

		expect(wrapper.emitted("editor-ready")).toEqual([[editor]])
	})

	it("passes the name editor on once it is ready", async ({ expect }) => {
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)
		const editor = { setEditable: vi.fn() }

		emitFrom(wrapper, "NameEditor", "editor-ready", editor)
		await nextTick()

		expect(wrapper.emitted("name-editor-ready")).toEqual([[editor]])
	})

	it("makes the editors read-only on a protected branch", async ({
		expect,
	}) => {
		seedBranches({ branchId: BRANCH_ID, protected: true })
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)
		const editor = { setEditable: vi.fn() }

		emitFrom(wrapper, "NameEditor", "editor-ready", editor)
		await nextTick()

		expect(editor.setEditable).toHaveBeenCalledWith(false)
	})

	it("hides the content editor and shows the diff while a diff is on", async ({
		expect,
	}) => {
		useEditorStore().updateTargetBranchId(TARGET_BRANCH_ID)
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)
		// the diff renders against the content editor, so it only appears
		// once ContentEditor has handed one over
		emitFrom(wrapper, "ContentEditor", "editor-ready", {
			chain: () => ({ run: () => true }),
			commands: { refreshGapDecorations: vi.fn() },
			setEditable: vi.fn(),
		})

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.findComponent({ name: "DiffEditor" }).exists()).toBe(true)
	})

	it("shows no diff without a branch to compare against", async ({
		expect,
	}) => {
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.findComponent({ name: "DiffEditor" }).exists()).toBe(false)
	})

	it("shows no diff of a branch against itself", async ({ expect }) => {
		useEditorStore().updateTargetBranchId(BRANCH_ID)
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()

		expect(wrapper.findComponent({ name: "DiffEditor" }).exists()).toBe(false)
	})

	it("passes a live icon change on", async ({ expect }) => {
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)

		emitFrom(wrapper, "NameEditor", "updated-live-icon", "mingcute:at-fill")
		await nextTick()

		expect(wrapper.emitted("updated-live-icon")).toEqual([["mingcute:at-fill"]])
	})

	it("passes a settings request on", async ({ expect }) => {
		const wrapper = await mountContainer()
		await sync(BRANCH_ID)

		emitFrom(wrapper, "ContentEditor", "open-settings", "github")
		await nextTick()

		expect(wrapper.emitted("open-settings")).toEqual([["github"]])
	})

	it("closes every branch sync when it is taken down", async ({ expect }) => {
		useEditorStore().updatePreloadedBranchIds([TARGET_BRANCH_ID])
		const wrapper = await mountContainer()

		wrapper.unmount()

		expect(providers.created.map((entry) => entry.destroyed)).toEqual([
			true,
			true,
		])
		expect(at(providers.created, 0).destroyed).toBe(true)
	})

	// closing a tab unmounts nothing, so without this everyone else keeps
	// seeing this user's caret until the socket is noticed as gone
	it("closes every branch sync when the tab goes away", async ({ expect }) => {
		useEditorStore().updatePreloadedBranchIds([TARGET_BRANCH_ID])
		await mountContainer()

		window.dispatchEvent(pagehide(false))

		expect(providers.created.map((entry) => entry.destroyed)).toEqual([
			true,
			true,
		])
	})

	it("keeps the branch syncs when the page is only cached", async ({
		expect,
	}) => {
		useEditorStore().updatePreloadedBranchIds([TARGET_BRANCH_ID])
		await mountContainer()

		window.dispatchEvent(pagehide(true))

		expect(providers.created.map((entry) => entry.destroyed)).toEqual([
			false,
			false,
		])
	})
})
