import { beforeEach, describe, it } from "vitest"
import ParameterListHeader from "./ParameterListHeader.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"
import { SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME } from "../node-names"
import { t } from "~/components/test-helpers"

function mountHeader(overrides: Record<string, unknown> = {}) {
	return mountNodeView(ParameterListHeader, {
		node: makeNode(
			{ uid: "header-1" },
			{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME },
		),
		...overrides,
	})
}

// the editable flag is a shared cookie state and the editor store is
// app-wide, so these tests cannot interleave
describe(
	"<SplitDocumentationParameterListHeader>",
	{ concurrent: false },
	() => {
		beforeEach(() => {
			useEditorMeta().setEditable(true)
			useEditorStore().setReviewableDiffActive(false)
		})

		it("identifies the wrapper by the node's uid", async ({ expect }) => {
			const wrapper = await mountHeader()

			const root = wrapper.get("[data-node-view-wrapper]")

			expect(root.attributes("id")).toBe("header-1")
			expect(root.attributes("data-uid")).toBe("header-1")
		})

		it("exposes the node's comment id and diff status on the wrapper", async ({
			expect,
		}) => {
			const wrapper = await mountHeader({
				node: makeNode(
					{
						uid: "header-1",
						nodeCommentId: "comment-1",
						diffStatus: "added",
					},
					{ typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME },
				),
			})

			const root = wrapper.get("[data-node-view-wrapper]")

			expect(root.attributes("data-node-comment-id")).toBe("comment-1")
			expect(root.attributes("data-diff-status")).toBe("added")
		})

		it("prompts for content while the node is empty and editable", async ({
			expect,
		}) => {
			const wrapper = await mountHeader()

			expect(
				wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
			).toBe(
				t(
					"editor.placeholders.content.split-documentation.parameter-list.header-name",
				),
			)
		})

		it("marks an empty node as unfilled in read mode", async ({ expect }) => {
			useEditorMeta().setEditable(false)

			const wrapper = await mountHeader()

			expect(
				wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
			).toBe(
				t(
					"editor.placeholders.content.split-documentation.parameter-list.header-name-empty",
				),
			)
		})

		it("marks an empty node as unfilled while a reviewable diff is shown", async ({
			expect,
		}) => {
			useEditorStore().setReviewableDiffActive(true)

			const wrapper = await mountHeader()

			expect(
				wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
			).toBe(
				t(
					"editor.placeholders.content.split-documentation.parameter-list.header-name-empty",
				),
			)
		})

		it("shows no placeholder once the node has text", async ({ expect }) => {
			const wrapper = await mountHeader({
				node: makeNode(
					{ uid: "header-1" },
					{
						typeName: SPLIT_DOCUMENTATION_PARAMETER_LIST_HEADER_NAME,
						textContent: "Timeout",
					},
				),
			})

			expect(
				wrapper.get("[data-node-view-content]").attributes("data-placeholder"),
			).toBeUndefined()
		})
	},
)
