import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import Code from "./Code.vue"
import {
	makeEditor,
	makeEditorState,
	makeNode,
	mountNodeView,
} from "../../test-helpers/node-view"
import { CODE_BLOCK_NAME, TITLED_CODE_BLOCK_NAME } from "../node-names"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { t } from "~/components/test-helpers"

function mountCode(
	options: {
		attrs?: Record<string, unknown>
		textContent?: string
		type?: string
		state?: Record<string, unknown>
		getPos?: () => number | undefined
	} = {},
) {
	return mountNodeView(Code, {
		node: makeNode(
			{ uid: "code-1", ...options.attrs },
			{
				typeName: CODE_BLOCK_NAME,
				textContent: options.textContent ?? "",
				nodeSize: 10,
			},
		),
		extension: { options: { type: options.type ?? "document" } },
		editor: makeEditor({ state: options.state ?? makeEditorState() }).editor,
		...(options.getPos ? { getPos: options.getPos } : {}),
	})
}

// the placeholder is an overlay div, separate from the code content
function placeholderText(wrapper: VueWrapper): string {
	const overlay = wrapper.find(".pointer-events-none")

	return overlay.exists() ? overlay.text() : ""
}

// the button row fades in while the caret is inside the block
function buttonsVisible(wrapper: VueWrapper): boolean {
	return wrapper
		.get("[data-node-view-wrapper] > div:last-child")
		.classes()
		.includes("opacity-100")
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<Code>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountCode()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("code-1")
		expect(root.attributes("data-uid")).toBe("code-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			attrs: { nodeCommentId: "comment-1", diffStatus: DiffStatus.Modified },
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("renders the code inside a pre element", async ({ expect }) => {
		const wrapper = await mountCode()

		expect(wrapper.get("[data-node-view-wrapper]").element.tagName).toBe("PRE")
		expect(wrapper.get("[data-node-view-content]").element.tagName).toBe("CODE")
	})

	it("prompts for code while the block is empty and editable", async ({
		expect,
	}) => {
		const wrapper = await mountCode()

		expect(placeholderText(wrapper)).toBe(
			t("editor.placeholders.content.extended-code-block.content"),
		)
	})

	it("marks an empty block as unfilled in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountCode()

		expect(placeholderText(wrapper)).toBe(
			t("editor.placeholders.content.extended-code-block.content-empty"),
		)
	})

	it("marks an empty block as unfilled while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountCode()

		expect(placeholderText(wrapper)).toBe(
			t("editor.placeholders.content.extended-code-block.content-empty"),
		)
	})

	it("shows no placeholder once the block has code", async ({ expect }) => {
		const wrapper = await mountCode({ textContent: "echo hi" })

		expect(placeholderText(wrapper)).toBe("")
	})

	it("tints an unchanged block", async ({ expect }) => {
		const wrapper = await mountCode({
			attrs: { diffStatus: DiffStatus.Unchanged },
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"bg-muted!",
		)
	})

	it.for([DiffStatus.Added, DiffStatus.Removed])(
		"leaves the diff colour of a %s block alone",
		async (diffStatus, { expect }) => {
			const wrapper = await mountCode({ attrs: { diffStatus: diffStatus } })

			expect(wrapper.get("[data-node-view-wrapper]").classes()).not.toContain(
				"bg-muted!",
			)
		},
	)

	it("sizes a document code block for body text", async ({ expect }) => {
		const wrapper = await mountCode()

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"text-sm",
		)
		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"min-h-11",
		)
	})

	it("sizes a comment code block smaller", async ({ expect }) => {
		const wrapper = await mountCode({ type: "comment" })

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"text-2sm",
		)
		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"min-h-9",
		)
	})

	it("joins its border with the title above it inside a titled block", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			state: makeEditorState({ parentTypeName: TITLED_CODE_BLOCK_NAME }),
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"rounded-t-none",
		)
	})

	it("renders borderless outside a titled block", async ({ expect }) => {
		const wrapper = await mountCode()

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"border-0",
		)
	})

	it("renders borderless when the block has no resolvable position", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			state: makeEditorState({ parentTypeName: TITLED_CODE_BLOCK_NAME }),
			getPos: () => undefined,
		})

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"border-0",
		)
	})

	it("shows the block buttons while the caret is inside the block", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			state: makeEditorState({ from: 2, to: 4 }),
		})

		expect(buttonsVisible(wrapper)).toBe(true)
	})

	it("hides the block buttons while the caret is elsewhere", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			state: makeEditorState({ from: 40, to: 40 }),
		})

		expect(buttonsVisible(wrapper)).toBe(false)
	})

	it("hides the block buttons when the block has no resolvable position", async ({
		expect,
	}) => {
		const wrapper = await mountCode({
			state: makeEditorState({ from: 2, to: 4 }),
			getPos: () => undefined,
		})

		expect(buttonsVisible(wrapper)).toBe(false)
	})
})
