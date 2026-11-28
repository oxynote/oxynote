import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { Editor as TiptapEditor, type Editor } from "@tiptap/core"
import { ySyncPluginKey } from "@tiptap/y-tiptap"
import {
	enableAutoUnmount,
	flushPromises,
	type VueWrapper,
} from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import CommentRenderer from "./CommentRenderer.vue"
import { CommentMark, findCommentMarkById } from "./comment-mark"
import { NodeComment } from "./node-comment-extension"
import { CommentExtensions } from "./utils"
import UniqueID from "../tiptap-utils/unique-id"
import { stubThemeColorContext } from "../test-helpers/theme"
import { DiffStatus, type PositionMap } from "../diff/position-map"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import {
	at,
	clearTeleportedOverlays,
	menuItem,
	raisedToasts,
	refuseForRole,
	seedAuthOrganization,
	seedAuthSession,
	settleMutations,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import type WsState from "~/utils/websocket"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

// the thread list is virtualized off the rendered row heights, which
// happy-dom reports as zero — no comment would ever be drawn. The
// stand-ins render every row the thread hands them.
vi.mock("vue-virtual-scroller", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		DynamicScroller: defineComponent({
			name: "DynamicScroller",
			props: { items: { type: Array, required: true } },
			setup(props, { slots, expose }) {
				// a new reply scrolls the thread to its end
				expose({ scrollToBottom: () => undefined })

				return () =>
					h(
						"div",
						(props.items as Record<string, unknown>[]).map((item, index) =>
							slots.default?.({ item: item, index: index, active: true }),
						),
					)
			},
		}),
		DynamicScrollerItem: defineComponent({
			name: "DynamicScrollerItem",
			setup:
				(_props, { slots }) =>
				() =>
					h("div", slots.default?.()),
		}),
	}
})

// happy-dom lays nothing out, so the popover never learns its height.
// The tests set the height by hand.
const popover = vi.hoisted(() => ({
	height: null as unknown as { value: number },
}))
mockNuxtImport("useElementSize", () => () => ({ height: popover.height }))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const ME = makeXid("usme")
const COMMENT_ID = makeXid("cmt")
const REPLY_ID = makeXid("rpl")
const BLOCK_UID = "block-1"

let contentEditor: Editor | null = null
let diffEditor: Editor | null = null

// the renderer anchors comments in a live document, so the suite drives a
// real editor holding the same mark and node comment extensions the app
// gives its content editor
function textDoc(...paragraphs: string[]): Editor {
	const element = document.createElement("div")
	document.body.appendChild(element)

	contentEditor = new TiptapEditor({
		element: element,
		extensions: [
			...CommentExtensions,
			UniqueID.configure({ types: ["paragraph"], attributeName: "uid" }),
			CommentMark,
			NodeComment.configure({ types: ["paragraph"] }),
		],
		content: paragraphs.map((text) => `<p>${text}</p>`).join(""),
	})

	return contentEditor
}

function commentBody(text: string) {
	return {
		type: "doc",
		content: [{ type: "paragraph", content: [{ type: "text", text: text }] }],
	}
}

function seedComments(comments: Record<string, unknown>[]) {
	seedQueryData(["documents", DOCUMENT_ID, "comments", BRANCH_ID], comments)
}

function serverComment(overrides: Record<string, unknown> = {}) {
	return {
		id: COMMENT_ID,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
		userId: ME,
		content: commentBody("looks good"),
		blockId: "block-1",
		resolved: false,
		replies: [],
		createdAt: new Date("2026-01-01T00:00:00Z"),
		...overrides,
	}
}

// a diff editor over the same content. Both editors carry the block under
// one uid, the way the diff view pairs merged blocks with their source.
function diffDocOf(source: Editor): Editor {
	const element = document.createElement("div")
	document.body.appendChild(element)

	diffEditor = new TiptapEditor({
		element: element,
		extensions: [
			...CommentExtensions,
			UniqueID.configure({ types: ["paragraph"], attributeName: "uid" }),
			CommentMark,
			NodeComment.configure({ types: ["paragraph"] }),
		],
		content: source.getJSON(),
	})

	for (const editor of [source, diffEditor]) {
		const block = editor.state.doc.firstChild
		editor.view.dispatch(
			editor.state.tr.setNodeMarkup(0, undefined, {
				...block?.attrs,
				uid: BLOCK_UID,
			}),
		)
	}

	return diffEditor
}

// the first block, reported as added on the branch
function addedBlockMap(editor: Editor): PositionMap {
	return [
		{
			source: "modified",
			blockIndex: 0,
			startPos: 0,
			nodeSize: editor.state.doc.firstChild?.nodeSize ?? 0,
			diffStatus: DiffStatus.Added,
			uid: BLOCK_UID,
		},
	]
}

function mountDiffRenderer(content: Editor, diff: Editor) {
	return mountSuspended(CommentRenderer, {
		props: {
			contentEditor: content,
			container: null,
			diffContext: {
				diffEditor: diff,
				positionMap: addedBlockMap(diff),
				suppressNextRecompute: () => undefined,
			},
		},
	})
}

// types a draft into the open thread and saves it against the stubbed
// server, which answers with COMMENT_ID
async function saveDraft(wrapper: VueWrapper) {
	mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/comments`, () =>
		serverComment(),
	)
	// saving refetches the thread list
	mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/comments`, () => [
		serverComment(),
	])
	commentEditor(wrapper).commands.setContent(commentBody("looks good"))
	await flushPromises()

	await popoverButton(
		wrapper,
		t("editor.comment-thread.comment-button"),
	).trigger("click")
}

function mountRenderer(editor: Editor) {
	return mountSuspended(CommentRenderer, {
		props: { contentEditor: editor, container: null },
	})
}

interface CommentRendererApi {
	isCommentPopoverOpen: (targetId?: string) => boolean
	selectComment: (target: {
		textComment: boolean
		id?: string
		pos?: number
	}) => Promise<void>
	addNewComment: (pos: number | "text-selection") => Promise<void>
}

// eslint's ts program cannot type a component exposed through a wrapper,
// so the exposed surface is named here
function api(wrapper: VueWrapper): CommentRendererApi {
	return wrapper.vm as unknown as CommentRendererApi
}

function press(target: Element) {
	target.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }))
}

// tiptap hangs its editor instance off the editable element. The thread
// draws each comment in an editor of its own, and the draft's editor comes
// last, below them.
function commentEditor(wrapper: VueWrapper): Editor {
	const editors = wrapper.findAll<Element & { editor?: Editor }>(
		".z-popover .ProseMirror",
	)
	const element = editors[editors.length - 1]?.element

	if (!element?.editor) {
		throw new Error("no comment editor in the popover")
	}

	return element.editor
}

function popoverOpen(wrapper: VueWrapper): boolean {
	return wrapper.find(".z-popover").exists()
}

function popoverButton(wrapper: VueWrapper, text: string) {
	const button = wrapper
		.findAll("button")
		.find((candidate) => candidate.text() === text)
	if (!button) {
		throw new Error(`no popover button rendering "${text}"`)
	}

	return button
}

// the editor store, the auth and query caches and the websocket store are
// shared app-wide, so these tests cannot interleave
describe("<CommentRenderer>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		clearTeleportedOverlays()
		stubThemeColorContext()
		popover.height = ref(0)
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorMeta().setEditable(true)
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
		useEditorStore().setReviewableDiffActive(false)
		useWebSocketStateStore().state = null
		seedAuthSession({ id: ME, name: "Me" })
		seedAuthOrganization({
			members: [{ userId: ME, user: { name: "Me", image: undefined } }],
		})
		seedComments([])
	})

	afterEach(() => {
		disposeMockEndpoints()

		if (contentEditor) {
			const element = contentEditor.view.dom.parentElement

			contentEditor.destroy()
			element?.remove()
			contentEditor = null
		}

		if (diffEditor) {
			const element = diffEditor.view.dom.parentElement

			diffEditor.destroy()
			element?.remove()
			diffEditor = null
		}
	})

	it("stays out of the way until a comment is opened", async ({ expect }) => {
		const editor = textDoc("hello world")

		const wrapper = await mountRenderer(editor)

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("opens an empty thread for the selected text", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment("text-selection")
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
		expect(wrapper.text()).toContain(
			t("editor.comment-thread.title-new-thread"),
		)
	})

	it("marks the selected text as commented", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment("text-selection")

		expect(editor.getHTML()).toContain("data-comment-id")
	})

	it("opens an empty thread for a whole block", async ({ expect }) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment(0)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
		expect(wrapper.text()).toContain(
			t("editor.comment-thread.title-new-thread"),
		)
	})

	it("scrolls a thread into view once it is drawn", async ({ expect }) => {
		const scrollIntoView = vi.spyOn(Element.prototype, "scrollIntoView")
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await flushPromises()

		popover.height.value = 120
		await flushPromises()

		expect(scrollIntoView).toHaveBeenCalledExactlyOnceWith({
			behavior: "smooth",
			block: "nearest",
		})
		expect(scrollIntoView.mock.contexts[0]).toBe(
			wrapper.find(".z-popover").element,
		)
	})

	it("scrolls a thread into view again when its height changes", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		popover.height.value = 120
		await flushPromises()
		const scrollIntoView = vi.spyOn(Element.prototype, "scrollIntoView")

		popover.height.value = 200
		await flushPromises()

		expect(scrollIntoView).toHaveBeenCalledExactlyOnceWith({
			behavior: "smooth",
			block: "nearest",
		})
		expect(scrollIntoView.mock.contexts[0]).toBe(
			wrapper.find(".z-popover").element,
		)
	})

	it("leaves the page where it is while a thread is not drawn", async ({
		expect,
	}) => {
		const scrollIntoView = vi.spyOn(Element.prototype, "scrollIntoView")
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment(0)
		await flushPromises()

		expect(scrollIntoView).not.toHaveBeenCalled()
	})

	it("opens nothing while the reader is not signed in", async ({ expect }) => {
		seedAuthSession(null)
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment(0)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("reports which thread is open", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)

		expect(api(wrapper).isCommentPopoverOpen(COMMENT_ID)).toBe(false)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		expect(api(wrapper).isCommentPopoverOpen(COMMENT_ID)).toBe(true)
		expect(api(wrapper).isCommentPopoverOpen("another-comment")).toBe(false)
	})

	it("shows the thread of an existing block comment", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })

		// the thread list resolves asynchronously and each comment body is
		// a tiptap editor that renders once it is mounted
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("looks good")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain(
			t("editor.comment-thread.title-reply-existing-thread"),
		)
	})

	it("shows a comment from someone who is no longer a member as a deleted user", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment({ userId: null })])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })

		// the thread list resolves asynchronously, and the avatar component
		// loads on demand
		await vi.waitFor(() => {
			expect(wrapper.find("[data-slot='avatar-fallback'] img").exists()).toBe(
				true,
			)
		}, WAIT_FOR_OPTIONS)
		expect(
			wrapper.get("[data-slot='avatar-fallback'] img").attributes("src"),
		).toBe(defaultAvatar("deleted-user"))
		expect(wrapper.text()).toContain(t("general.deleted-user"))
	})

	it("shows the thread of an existing text comment", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		editor.commands.addCommentMark({ commentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: true, id: COMMENT_ID })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("looks good")
		}, WAIT_FOR_OPTIONS)
	})

	it("opens nothing for a comment that is not in the document", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: true, id: COMMENT_ID })
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("shows the replies a thread already has", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([
			serverComment({
				replies: [
					{
						id: makeXid("rpl"),
						commentId: COMMENT_ID,
						userId: ME,
						content: commentBody("agreed"),
						createdAt: new Date("2026-01-02T00:00:00Z"),
					},
				],
			}),
		])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("agreed")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain("looks good")
	})

	it("shows a deleted comment as a placeholder above its replies, with no actions", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([
			serverComment({
				content: null,
				deletedAt: new Date("2026-01-03T00:00:00Z"),
				replies: [
					{
						id: makeXid("rpl"),
						commentId: COMMENT_ID,
						userId: ME,
						content: commentBody("agreed"),
						createdAt: new Date("2026-01-02T00:00:00Z"),
					},
				],
			}),
		])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("agreed")
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toContain(t("editor.comment-thread.deleted-comment"))
		// only the reply keeps an actions menu
		expect(wrapper.findAll("[data-slot='dropdown-menu-trigger']")).toHaveLength(
			1,
		)
	})

	it("closes a deleted comment's thread and removes its mark when its last reply is deleted", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([
			serverComment({
				content: null,
				deletedAt: new Date("2026-01-03T00:00:00Z"),
				replies: [myReply()],
			}),
		])
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/replies/${REPLY_ID}`,
			() => ({}),
		)
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/comments`, () => [])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("agreed")
		}, WAIT_FOR_OPTIONS)
		await openItemMenu(wrapper, 0)

		menuItem(t("editor.comment-thread.delete-button")).click()
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(popoverOpen(wrapper)).toBe(false)
		expect(editor.state.doc.firstChild?.attrs.nodeCommentId).toBeNull()
	})

	it("removes the mark of a comment deleted without replies", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment({ replies: [] })])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("looks good")
		}, WAIT_FOR_OPTIONS)
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}`,
			() => ({}),
		)
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/comments`, () => [])
		await openItemMenu(wrapper, 0)

		menuItem(t("editor.comment-thread.delete-button")).click()
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(popoverOpen(wrapper)).toBe(false)
		expect(editor.state.doc.firstChild?.attrs.nodeCommentId).toBeNull()
	})

	it("keeps the mark of a comment deleted above its replies", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment({ replies: [myReply()] })])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("looks good")
		}, WAIT_FOR_OPTIONS)
		const calls = mockEndpoint(
			"DELETE",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}`,
			() => ({}),
		)
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/comments`, () => [
			serverComment({
				content: null,
				deletedAt: new Date("2026-01-03T00:00:00Z"),
				replies: [myReply()],
			}),
		])
		await openItemMenu(wrapper, 0)

		menuItem(t("editor.comment-thread.delete-button")).click()
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(editor.state.doc.firstChild?.attrs.nodeCommentId).toBe(COMMENT_ID)
	})

	it("closes the thread on request", async ({ expect }) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await nextTick()

		await popoverButton(
			wrapper,
			t("editor.comment-thread.close-button"),
		).trigger("click")

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("closes a text thread when a press lands outside the editor", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		editor.commands.addCommentMark({ commentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: true, id: COMMENT_ID })
		await nextTick()

		press(document.body)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("keeps a text thread open on a press in the editor until the click lands", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		editor.commands.addCommentMark({ commentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: true, id: COMMENT_ID })
		await nextTick()

		press(editor.view.dom)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("closes a block thread when a press lands on another block", async ({
		expect,
	}) => {
		const editor = textDoc("first", "second")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		press(editor.view.nodeDOM(7) as Element)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("keeps a block thread open while presses land inside its block", async ({
		expect,
	}) => {
		const editor = textDoc("first", "second")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		press(editor.view.nodeDOM(0) as Element)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("drops a draft block comment when a press lands beside the editor", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const handle = document.createElement("div")
		editor.view.dom.parentElement?.appendChild(handle)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await nextTick()

		press(handle)
		await nextTick()

		expect.soft(popoverOpen(wrapper)).toBe(false)
		expect.soft(editor.state.doc.firstChild?.attrs.nodeCommentId).toBeNull()
	})

	it("keeps the thread open while presses land inside it", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await nextTick()

		press(wrapper.find(".z-popover").element)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("keeps the thread open while presses land on a comment indicator", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const indicator = document.createElement("button")
		editor.view.dom.parentElement
			?.querySelector(".node-comment-overlay-container")
			?.appendChild(indicator)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await nextTick()

		press(indicator)
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("keeps the thread open while presses land on its actions menu", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await vi.waitFor(() => {
			expect(wrapper.find("[data-slot='dropdown-menu-trigger']").exists()).toBe(
				true,
			)
		}, WAIT_FOR_OPTIONS)
		const trigger = wrapper.find("[data-slot='dropdown-menu-trigger']")
		await trigger.trigger("pointerdown", { button: 0 })
		await trigger.trigger("click")
		await nextTick()

		press(menuItem(t("editor.comment-thread.edit-start-button")))
		await nextTick()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("closes a block thread when its block is deleted", async ({ expect }) => {
		const editor = textDoc("first", "second")
		editor.commands.addNodeComment(7, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		editor.commands.setTextSelection(3)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		editor.chain().setTextSelection(6).deleteRange({ from: 7, to: 15 }).run()
		await flushPromises()

		expect.soft(popoverOpen(wrapper)).toBe(false)
		// the cursor stays where the edit left it
		expect.soft(editor.state.selection.from).toBe(6)
	})

	it("closes a text thread when its text is deleted", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.setTextSelection({ from: 1, to: 6 })
		editor.commands.addCommentMark({ commentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: true, id: COMMENT_ID })
		await nextTick()

		editor.commands.deleteRange({ from: 1, to: 6 })
		await flushPromises()

		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("closes a block thread and restores the cursor when a collaborator deletes its block", async ({
		expect,
	}) => {
		const editor = textDoc("first", "second")
		editor.commands.addNodeComment(7, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		editor.commands.setTextSelection(3)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		editor.view.dispatch(
			editor.state.tr.delete(7, 15).setMeta(ySyncPluginKey, {}),
		)
		await flushPromises()

		expect.soft(popoverOpen(wrapper)).toBe(false)
		expect.soft(editor.state.selection.from).toBe(3)
	})

	it("keeps the thread open while the rest of the document changes", async ({
		expect,
	}) => {
		const editor = textDoc("first", "second")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		editor.commands.deleteRange({ from: 7, to: 15 })
		await flushPromises()

		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("keeps a new block thread open once the server gives it an id", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/comments`, () =>
			serverComment(),
		)
		// saving refetches the thread list
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/comments`, () => [
			serverComment(),
		])
		const wrapper = await mountRenderer(editor)
		await api(wrapper).addNewComment(0)
		await nextTick()
		commentEditor(wrapper).commands.setContent(commentBody("looks good"))
		await flushPromises()

		await popoverButton(
			wrapper,
			t("editor.comment-thread.comment-button"),
		).trigger("click")

		await vi.waitFor(() => {
			expect(editor.state.doc.firstChild?.attrs.nodeCommentId).toBe(COMMENT_ID)
		}, WAIT_FOR_OPTIONS)
		await nextTick()
		expect(popoverOpen(wrapper)).toBe(true)
	})

	it("keeps the comment button out of reach while the draft is empty", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const wrapper = await mountRenderer(editor)

		await api(wrapper).addNewComment(0)
		await nextTick()

		expect(
			popoverButton(
				wrapper,
				t("editor.comment-thread.comment-button"),
			).attributes("disabled"),
		).toBe("")
	})

	it("offers to resolve a thread that already exists", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const wrapper = await mountRenderer(editor)

		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		expect(wrapper.text()).toContain(t("editor.comment-thread.resolve-button"))
	})

	it("resolves the thread", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		const calls = mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/status`,
			() => null,
		)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		await popoverButton(
			wrapper,
			t("editor.comment-thread.resolve-button"),
		).trigger("click")

		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
		expect(calls[0]?.body).toEqual({ resolved: true })
		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("warns when the thread cannot be resolved", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/status`,
			(_c, event) => {
				setResponseStatus(event, 500)

				return { message: "boom" }
			},
		)
		const wrapper = await mountRenderer(editor)
		await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })
		await nextTick()

		await popoverButton(
			wrapper,
			t("editor.comment-thread.resolve-button"),
		).trigger("click")

		// the thread closes before the request answers, and the warning
		// follows once it has
		await vi.waitFor(() => {
			expect(toast.custom).toHaveBeenCalledTimes(1)
		}, WAIT_FOR_OPTIONS)
		expect(popoverOpen(wrapper)).toBe(false)
	})

	describe("when the server refuses a write for the role", () => {
		it("shows only the access denied toast for a refused comment", async ({
			expect,
		}) => {
			const editor = textDoc("hello world")
			mockEndpoint(
				"POST",
				`/api/documents/${DOCUMENT_ID}/comments`,
				(_c, event) => refuseForRole(event),
			)
			const wrapper = await mountRenderer(editor)
			await api(wrapper).addNewComment(0)
			await nextTick()
			await writeDraft(wrapper, t("editor.comment-thread.comment-button"))

			await popoverButton(
				wrapper,
				t("editor.comment-thread.comment-button"),
			).trigger("click")
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused resolve", async ({
			expect,
		}) => {
			const wrapper = await openThread()
			mockEndpoint(
				"PUT",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/status`,
				(_c, event) => refuseForRole(event),
			)

			await popoverButton(
				wrapper,
				t("editor.comment-thread.resolve-button"),
			).trigger("click")
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused reply", async ({
			expect,
		}) => {
			const wrapper = await openThread()
			mockEndpoint(
				"POST",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/replies`,
				(_c, event) => refuseForRole(event),
			)
			await writeDraft(wrapper, t("editor.comment-thread.reply-button"))

			await popoverButton(
				wrapper,
				t("editor.comment-thread.reply-button"),
			).trigger("click")
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused comment edit", async ({
			expect,
		}) => {
			const wrapper = await openThread()
			mockEndpoint(
				"PUT",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}`,
				(_c, event) => refuseForRole(event),
			)
			await openItemMenu(wrapper, 0)
			menuItem(t("editor.comment-thread.edit-start-button")).click()
			await nextTick()

			await popoverButton(
				wrapper,
				t("editor.comment-thread.edit-button"),
			).trigger("click")
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused comment deletion", async ({
			expect,
		}) => {
			const wrapper = await openThread()
			mockEndpoint(
				"DELETE",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}`,
				(_c, event) => refuseForRole(event),
			)
			await openItemMenu(wrapper, 0)

			menuItem(t("editor.comment-thread.delete-button")).click()
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused reply edit", async ({
			expect,
		}) => {
			const wrapper = await openThread([myReply()])
			mockEndpoint(
				"PUT",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/replies/${REPLY_ID}`,
				(_c, event) => refuseForRole(event),
			)
			await openItemMenu(wrapper, 1)
			menuItem(t("editor.comment-thread.edit-start-button")).click()
			await nextTick()

			await popoverButton(
				wrapper,
				t("editor.comment-thread.edit-button"),
			).trigger("click")
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})

		it("shows only the access denied toast for a refused reply deletion", async ({
			expect,
		}) => {
			const wrapper = await openThread([myReply()])
			mockEndpoint(
				"DELETE",
				`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/replies/${REPLY_ID}`,
				(_c, event) => refuseForRole(event),
			)
			await openItemMenu(wrapper, 1)

			menuItem(t("editor.comment-thread.delete-button")).click()
			await settleMutations()

			expect(raisedToasts()).toMatchObject([roleToast()])
		})
	})

	describe("for a viewer", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthOrganization({
				members: [
					{
						userId: ME,
						role: "viewer",
						user: { name: "Me", image: undefined },
					},
				],
			})
		})

		it("shows the thread, their own comment included, with nothing to write or change", async ({
			expect,
		}) => {
			const wrapper = await openThread()

			expect.soft(wrapper.text()).toContain("looks good")
			expect
				.soft(wrapper.find(".z-popover [contenteditable='true']").exists())
				.toBe(false)
			expect.soft(wrapper.find(".z-popover button").exists()).toBe(false)
		})
	})

	it("refetches the comments when the server says they changed", async ({
		expect,
	}) => {
		const editor = textDoc("hello world")
		const calls = mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/comments`,
			() => [],
		)
		const handlers: (() => void)[] = []
		const subscribe = vi.fn((_topic: string, handler: () => void) => {
			handlers.push(handler)

			return () => undefined
		})
		useWebSocketStateStore().state = {
			subscribe: subscribe,
		} as unknown as WsState
		await mountRenderer(editor)

		handlers.forEach((handler) => {
			handler()
		})

		await vi.waitFor(() => {
			expect(calls.length).toBeGreaterThan(0)
		}, WAIT_FOR_OPTIONS)
	})
	// the content editor keeps its own renderer mounted, hidden, next to
	// the diff one
	describe("when the diff view is on", () => {
		beforeEach(() => {
			useEditorStore().setReviewableDiffActive(true)
		})

		it("keeps a block comment on added content in the branch", async ({
			expect,
		}) => {
			const content = textDoc("hello world")
			const diff = diffDocOf(content)
			await mountRenderer(content)
			const wrapper = await mountDiffRenderer(content, diff)
			await api(wrapper).addNewComment(0)
			await nextTick()

			await saveDraft(wrapper)

			await vi.waitFor(() => {
				expect(diff.state.doc.firstChild?.attrs.nodeCommentId).toBe(COMMENT_ID)
			}, WAIT_FOR_OPTIONS)
			expect(content.state.doc.firstChild?.attrs.nodeCommentId).toBe(COMMENT_ID)
		})

		it("keeps a text comment on added content in the branch", async ({
			expect,
		}) => {
			const content = textDoc("hello world")
			const diff = diffDocOf(content)
			diff.commands.setTextSelection({ from: 1, to: 6 })
			await mountRenderer(content)
			const wrapper = await mountDiffRenderer(content, diff)
			await api(wrapper).addNewComment("text-selection")
			await nextTick()

			await saveDraft(wrapper)

			await vi.waitFor(() => {
				expect(findCommentMarkById(diff.state, COMMENT_ID)).not.toBeNull()
			}, WAIT_FOR_OPTIONS)
			expect(findCommentMarkById(content.state, COMMENT_ID)).toMatchObject({
				from: 1,
				to: 6,
			})
		})
	})
})

// the access denied toast the api client raises for a refused request
function roleToast() {
	return { type: "error", title: t("general.errors.role-not-allowed") }
}

function myReply() {
	return {
		id: REPLY_ID,
		commentId: COMMENT_ID,
		userId: ME,
		content: commentBody("agreed"),
		createdAt: new Date("2026-01-02T00:00:00Z"),
	}
}

// opens the reader's own block comment, with the given replies
async function openThread(replies: unknown[] = []) {
	const editor = textDoc("hello world")
	editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
	seedComments([serverComment({ replies: replies })])
	const wrapper = await mountRenderer(editor)
	await api(wrapper).selectComment({ textComment: false, id: COMMENT_ID })

	// the thread list loads as an async component, so its rows show a few
	// ticks after the popover opens
	await vi.waitFor(() => {
		if (!wrapper.text().includes("looks good")) {
			throw new Error("the thread has not been drawn yet")
		}
	}, WAIT_FOR_OPTIONS)

	return wrapper
}

// tiptap hands vue a new draft two animation frames after it changes, so
// the button that saves the draft stays disabled until then
async function writeDraft(wrapper: VueWrapper, saveButton: string) {
	commentEditor(wrapper).commands.setContent(commentBody("agreed"))

	await vi.waitFor(() => {
		if (
			popoverButton(wrapper, saveButton).attributes("disabled") !== undefined
		) {
			throw new Error(`"${saveButton}" is still disabled`)
		}
	}, WAIT_FOR_OPTIONS)
}

// the thread lists the comment first and its replies below, each with an
// actions menu of its own
async function openItemMenu(wrapper: VueWrapper, index: number) {
	const trigger = at(
		wrapper.findAll("[data-slot='dropdown-menu-trigger']"),
		index,
	)
	await trigger.trigger("pointerdown", { button: 0 })
	await trigger.trigger("click")
	await nextTick()
}
