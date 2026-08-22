import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Editor } from "@tiptap/core"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import * as Y from "yjs"
import DiffEditor from "./DiffEditor.vue"
import CommentIndicatorContainer from "../comments/CommentIndicatorContainer.vue"
import TextBubbleMenu from "../TextBubbleMenu.vue"
import BlockHandle from "../drag-handle/BlockHandle.vue"
import { stubThemeColorContext } from "../test-helpers/theme"
import { clearQueryCache, makeXid } from "~/composables/api/test-helpers"
import {
	emitFrom,
	stubViewportMatches,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")

// the comment renderer drives its own tiptap instances and comment
// requests; the diff editor's contract with it is which of its actions
// it forwards
const childStubs = {
	CommentRenderer: true,
}

function paragraph(text: string) {
	const elem = new Y.XmlElement("paragraph")
	elem.insert(0, [new Y.XmlText(text)])

	return elem
}

function branchDoc(...texts: string[]): Y.Doc {
	const ydoc = new Y.Doc()
	ydoc.getXmlFragment("content").insert(0, texts.map(paragraph))

	return ydoc
}

function mountDiff(target: Y.Doc, active: Y.Doc) {
	return mountSuspended(DiffEditor, {
		props: {
			targetBranchYdoc: target,
			activeBranchYdoc: active,
			contentEditor: {} as unknown as Editor,
		},
		global: { stubs: childStubs },
	})
}

function diffEditorOf(wrapper: VueWrapper) {
	return wrapper.findComponent(TextBubbleMenu).props("editor") as {
		commands: Record<string, (...args: unknown[]) => boolean>
	}
}

// tiptap rebuilds editor.commands on every access, so a spy on one of
// them would never be the object the component calls
function watchCommands(editor: {
	commands: Record<string, (...args: unknown[]) => boolean>
}): [string, unknown[]][] {
	const called: [string, unknown[]][] = []
	const real = editor.commands
	const proxy = new Proxy(real, {
		get(target, name: string | symbol) {
			return (...args: unknown[]) => {
				called.push([String(name), args])

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

// the editor store, the editable flag and the viewport stub are shared
// app-wide, so these tests cannot interleave
describe("<DiffEditor>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		stubThemeColorContext()
		stubViewportMatches(true)
		clearQueryCache()
		useEditorMeta().setEditable(true)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(true)
	})

	it("shows the content of both branches merged", async ({ expect }) => {
		const wrapper = await mountDiff(
			branchDoc("kept", "removed"),
			branchDoc("kept", "added"),
		)

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("kept")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain("removed")
		expect(wrapper.text()).toContain("added")
	})

	it("shows nothing for two empty branches", async ({ expect }) => {
		const wrapper = await mountDiff(new Y.Doc(), new Y.Doc())

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get(".ProseMirror").text()).toBe("")
	})

	it("renders the diff read-only", async ({ expect }) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.classes()).toContain("editor-not-editable")
	})

	it("offers a bubble menu limited to commenting", async ({ expect }) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))

		await vi.waitFor(() => {
			expect(wrapper.findComponent(TextBubbleMenu).exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(
			wrapper.findComponent(TextBubbleMenu).props("diffContext"),
		).toBeDefined()
	})

	it("offers the block handle over the merged content", async ({ expect }) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))

		await vi.waitFor(() => {
			expect(wrapper.findComponent(BlockHandle).exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
	})

	it("highlights the node a comment indicator points at", async ({
		expect,
	}) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))
		await vi.waitFor(() => {
			expect(wrapper.findComponent(TextBubbleMenu).exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		const called = watchCommands(diffEditorOf(wrapper))

		emitFrom(
			wrapper,
			CommentIndicatorContainer,
			"comment-hover-change",
			"node",
			"comment-1",
			true,
		)
		await nextTick()

		expect(called).toEqual([
			["setNodeCommentForcedHighlight", ["comment-1", true]],
		])
	})

	it("highlights the text a comment indicator points at", async ({
		expect,
	}) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))
		await vi.waitFor(() => {
			expect(wrapper.findComponent(TextBubbleMenu).exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		const called = watchCommands(diffEditorOf(wrapper))

		emitFrom(
			wrapper,
			CommentIndicatorContainer,
			"comment-hover-change",
			"text",
			"comment-1",
			false,
		)
		await nextTick()

		expect(called).toEqual([
			["setCommentMarkForcedHighlight", ["comment-1", false]],
		])
	})

	it("stops the diff once it is taken down", async ({ expect }) => {
		const wrapper = await mountDiff(branchDoc("kept"), branchDoc("kept"))
		await vi.waitFor(() => {
			expect(wrapper.findComponent(TextBubbleMenu).exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)

		wrapper.unmount()

		expect(wrapper.findComponent(TextBubbleMenu).exists()).toBe(false)
	})
})
