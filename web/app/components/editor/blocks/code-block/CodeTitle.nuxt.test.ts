import { beforeEach, describe, it } from "vitest"
import CodeTitle from "./CodeTitle.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"
import { CODE_BLOCK_TITLE_NAME } from "../node-names"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { t } from "~/components/test-helpers"

function mountTitle(attrs: Record<string, unknown> = {}, textContent = "") {
	return mountNodeView(CodeTitle, {
		node: makeNode(
			{ uid: "title-1", ...attrs },
			{ typeName: CODE_BLOCK_TITLE_NAME, textContent: textContent },
		),
	})
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe("<CodeTitle>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountTitle()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("title-1")
		expect(root.attributes("data-uid")).toBe("title-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountTitle({
			nodeCommentId: "comment-1",
			diffStatus: DiffStatus.Modified,
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("prompts for a title while the node is empty and editable", async ({
		expect,
	}) => {
		const wrapper = await mountTitle()

		expect(
			wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
		).toBe(t("editor.placeholders.content.extended-code-block.header-title"))
	})

	it("marks an empty title as unfilled in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountTitle()

		expect(
			wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
		).toBe(
			t("editor.placeholders.content.extended-code-block.header-title-empty"),
		)
	})

	it("marks an empty title as unfilled while a reviewable diff is shown", async ({
		expect,
	}) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountTitle()

		expect(
			wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
		).toBe(
			t("editor.placeholders.content.extended-code-block.header-title-empty"),
		)
	})

	it("shows no placeholder once the title has text", async ({ expect }) => {
		const wrapper = await mountTitle({}, "setup.sh")

		expect(
			wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
		).toBeUndefined()
	})

	it("shows a caret while editing", async ({ expect }) => {
		const wrapper = await mountTitle()

		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"caret-foreground",
		)
	})

	it("hides the caret while a reviewable diff is shown", async ({ expect }) => {
		useEditorStore().setReviewableDiffActive(true)

		const wrapper = await mountTitle()

		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"caret-transparent",
		)
	})

	it("tints an unchanged title", async ({ expect }) => {
		const wrapper = await mountTitle({ diffStatus: DiffStatus.Unchanged })

		expect(wrapper.get("[data-node-view-wrapper]").classes()).toContain(
			"bg-muted-highlight",
		)
	})

	it.for([DiffStatus.Added, DiffStatus.Removed])(
		"leaves the diff colour of a %s title alone",
		async (diffStatus, { expect }) => {
			const wrapper = await mountTitle({ diffStatus: diffStatus })

			expect(wrapper.get("[data-node-view-wrapper]").classes()).not.toContain(
				"bg-muted-highlight",
			)
		},
	)
})
