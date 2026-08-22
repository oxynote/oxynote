import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { HocuspocusProvider } from "@hocuspocus/provider"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { Awareness } from "y-protocols/awareness"
import * as Y from "yjs"
import ContentEditor from "./ContentEditor.vue"
import BlockHandle from "./drag-handle/BlockHandle.vue"
import { stubThemeColorContext } from "./test-helpers/theme"
import { METRIC_BLOCK_NAME } from "./blocks/node-names"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
} from "~/composables/api/test-helpers"
import {
	emitFrom,
	stubViewportMatches,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

// the comment renderer drives its own tiptap instances and comment
// requests; the content editor's contract with it is which of its actions
// it forwards, which the stub keeps observable
const childStubs = {
	CommentRenderer: true,
}

interface Branch {
	ydoc: Y.Doc
	awareness: Awareness
	provider: HocuspocusProvider
	sync: () => void
}

function makeBranch(): Branch {
	const ydoc = new Y.Doc()
	const awareness = new Awareness(ydoc)
	const handlers: Record<string, (() => void)[]> = {}

	return {
		ydoc: ydoc,
		awareness: awareness,
		provider: {
			document: ydoc,
			awareness: awareness,
			on: (event: string, handler: () => void) => {
				handlers[event] ??= []
				handlers[event].push(handler)
			},
			off: () => undefined,
		} as unknown as HocuspocusProvider,
		sync: () => {
			handlers.synced?.forEach((handler) => {
				handler()
			})
		},
	}
}

function paragraph(text: string, uid: string) {
	const elem = new Y.XmlElement("paragraph")
	elem.setAttribute("uid", uid)
	elem.insert(0, [new Y.XmlText(text)])

	return elem
}

function mountEditor(
	options: { branch?: Branch; documentHooks?: DocumentHook[] } = {},
) {
	const branch = options.branch ?? makeBranch()

	return mountSuspended(ContentEditor, {
		props: {
			activeBranchProvider: branch.provider,
			activeBranchYdoc: branch.ydoc,
			documentHooks: options.documentHooks ?? [],
			nameEditor: null,
			userCaretDetails: { name: "Me", color: "#ff0000" },
		},
		global: { stubs: childStubs },
	})
}

function editorOf(wrapper: VueWrapper) {
	const emitted = wrapper.emitted("editor-ready")
	const editor = emitted?.[0]?.[0]
	if (!editor) {
		throw new Error("the content editor never reported itself as ready")
	}

	return editor as {
		commands: {
			setTextSelection: (pos: number) => boolean
		} & Record<string, (...args: unknown[]) => boolean>
	}
}

// tiptap rebuilds editor.commands on every access, so a spy on one of
// them would never be the object the component calls. Shadowing the
// getter with a recording proxy is what makes those calls observable.
function watchCommands(editor: {
	commands: Record<string, (...args: unknown[]) => boolean>
}): string[] {
	const called: string[] = []
	const real = editor.commands
	const proxy = new Proxy(real, {
		get(target, name: string | symbol) {
			return (...args: unknown[]) => {
				called.push(String(name))

				const command = target[String(name)]

				return command ? command(...args) : false
			}
		},
	})

	Object.defineProperty(editor, "commands", {
		get: () => proxy,
		configurable: true,
	})

	return called
}

function editingNodeUid(branch: Branch): unknown {
	return branch.awareness.getLocalState()?.editingNodeUid
}

// the editable flag is a shared cookie state, the editor store and the
// viewport stub are app-wide, so these tests cannot interleave
describe("<ContentEditor>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		stubThemeColorContext()
		stubViewportMatches(true)
		clearQueryCache()
		useEditorMeta().setEditable(true)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(false)
		useEditorStore().activateMetricBlockConfig(null)
	})

	afterEach(disposeMockEndpoints)

	it("hands its editor to the host once it is ready", async ({ expect }) => {
		const wrapper = await mountEditor()

		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("renders the document without spellchecking", async ({ expect }) => {
		const wrapper = await mountEditor()

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get(".ProseMirror").attributes("spellcheck")).toBe("false")
	})

	it("shows the content the branch carries", async ({ expect }) => {
		const branch = makeBranch()
		branch.ydoc
			.getXmlFragment("content")
			.insert(0, [paragraph("hello world", "block-1")])

		const wrapper = await mountEditor({ branch: branch })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("hello world")
		}, WAIT_FOR_OPTIONS)
	})

	it("stays editable while the branch is", async ({ expect }) => {
		const wrapper = await mountEditor()

		expect(wrapper.classes()).not.toContain("editor-not-editable")
	})

	it("marks itself read-only in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountEditor()

		expect(wrapper.classes()).toContain("editor-not-editable")
	})

	it("marks itself read-only while a diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountEditor()

		expect(wrapper.classes()).toContain("editor-not-editable")
	})

	it("tells collaborators which block the reader is in", async ({ expect }) => {
		const branch = makeBranch()
		branch.ydoc
			.getXmlFragment("content")
			.insert(0, [paragraph("hello world", "block-1")])
		const wrapper = await mountEditor({ branch: branch })
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)

		editorOf(wrapper).commands.setTextSelection(3)
		await nextTick()

		expect(editingNodeUid(branch)).toEqual({
			uid: "block-1",
			type: "paragraph",
		})
	})

	it("tells collaborators nothing for a block without an id", async ({
		expect,
	}) => {
		const branch = makeBranch()
		const elem = new Y.XmlElement("paragraph")
		elem.insert(0, [new Y.XmlText("hello")])
		branch.ydoc.getXmlFragment("content").insert(0, [elem])
		const wrapper = await mountEditor({ branch: branch })
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)

		editorOf(wrapper).commands.setTextSelection(2)
		await nextTick()

		expect(editingNodeUid(branch)).toBeNull()
	})

	it("tells collaborators which metric block is being configured", async ({
		expect,
	}) => {
		const branch = makeBranch()
		const wrapper = await mountEditor({ branch: branch })
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)

		useEditorStore().activateMetricBlockConfig("metric-1")
		await nextTick()

		expect(editingNodeUid(branch)).toEqual({
			uid: "metric-1",
			type: METRIC_BLOCK_NAME,
		})
	})

	it("tells collaborators the metric block was closed again", async ({
		expect,
	}) => {
		const branch = makeBranch()
		const wrapper = await mountEditor({ branch: branch })
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		useEditorStore().activateMetricBlockConfig("metric-1")
		await nextTick()

		useEditorStore().activateMetricBlockConfig(null)
		await nextTick()

		expect(editingNodeUid(branch)).toBeNull()
	})

	it("tells collaborators nothing once it is gone", async ({ expect }) => {
		const branch = makeBranch()
		branch.ydoc
			.getXmlFragment("content")
			.insert(0, [paragraph("hello world", "block-1")])
		const wrapper = await mountEditor({ branch: branch })
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		editorOf(wrapper).commands.setTextSelection(3)

		wrapper.unmount()

		expect(editingNodeUid(branch)).toBeNull()
	})

	it("refreshes the hook highlights when the hooks change", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		const called = watchCommands(editorOf(wrapper))

		await wrapper.setProps({
			documentHooks: [
				{
					id: "hook-1",
					blockId: "block-1",
					score: "0",
				} as unknown as DocumentHook,
			],
		})

		expect(called).toEqual(["refreshHookDecorations"])
	})

	it("redraws the comment indicators when the diff is turned off", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)
		const wrapper = await mountEditor()
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		const called = watchCommands(editorOf(wrapper))

		useEditorStore().setReviewableDiffActive(false)
		await nextTick()
		await nextTick()

		expect(called).toEqual([
			"refreshNodeCommentOverlays",
			"refreshTextCommentIndicators",
		])
	})

	it("leaves the indicators alone when the diff is turned on", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		const called = watchCommands(editorOf(wrapper))

		useEditorStore().setReviewableDiffActive(true)
		await nextTick()
		await nextTick()

		expect(called).toEqual([])
	})

	it("passes a settings request from the block handle on", async ({
		expect,
	}) => {
		const wrapper = await mountEditor()
		await vi.waitFor(() => {
			expect(wrapper.emitted("editor-ready")).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)

		emitFrom(wrapper, BlockHandle, "open-settings", "github")
		await nextTick()

		expect(wrapper.emitted("open-settings")).toEqual([["github"]])
	})
})
