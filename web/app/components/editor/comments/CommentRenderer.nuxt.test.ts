import { mountSuspended } from "@nuxt/test-utils/runtime"
import { Editor as TiptapEditor, type Editor } from "@tiptap/core"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import CommentRenderer from "./CommentRenderer.vue"
import { CommentMark } from "./comment-mark"
import { NodeComment } from "./node-comment-extension"
import { CommentExtensions } from "./utils"
import UniqueID from "../tiptap-utils/unique-id"
import { stubThemeColorContext } from "../test-helpers/theme"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import {
	seedAuthOrganization,
	seedAuthSession,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import type WsState from "~/utils/websocket"

// the thread list is virtualized off the rendered row heights, which
// happy-dom reports as zero — no comment would ever be drawn. The
// stand-ins render every row the thread hands them.
vi.mock("vue-virtual-scroller", async () => {
	const { defineComponent, h } = await import("vue")

	return {
		DynamicScroller: defineComponent({
			name: "DynamicScroller",
			props: { items: { type: Array, required: true } },
			setup:
				(props, { slots }) =>
				() =>
					h(
						"div",
						(props.items as Record<string, unknown>[]).map((item, index) =>
							slots.default?.({ item: item, index: index, active: true }),
						),
					),
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

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const ME = makeXid("usme")
const COMMENT_ID = makeXid("cmt")

let contentEditor: Editor | null = null

// the renderer anchors comments in a live document, so the suite drives a
// real editor holding the same mark and node comment extensions the app
// gives its content editor
function textDoc(text: string): Editor {
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
		content: `<p>${text}</p>`,
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
		stubThemeColorContext()
		clearQueryCache()
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
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/resolve`,
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
		expect(popoverOpen(wrapper)).toBe(false)
	})

	it("warns when the thread cannot be resolved", async ({ expect }) => {
		const editor = textDoc("hello world")
		editor.commands.addNodeComment(0, { nodeCommentId: COMMENT_ID })
		seedComments([serverComment()])
		mockEndpoint(
			"PUT",
			`/api/documents/${DOCUMENT_ID}/comments/${COMMENT_ID}/resolve`,
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

		await vi.waitFor(() => {
			expect(popoverOpen(wrapper)).toBe(false)
		}, WAIT_FOR_OPTIONS)
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
})
