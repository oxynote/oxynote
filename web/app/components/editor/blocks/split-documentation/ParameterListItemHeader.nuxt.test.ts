import { describe, it } from "vitest"
import ParameterListItemHeader from "./ParameterListItemHeader.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"

describe("<SplitDocumentationParameterListItemHeader>", () => {
	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountNodeView(ParameterListItemHeader, {
			node: makeNode({ uid: "item-header-1" }),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("item-header-1")
		expect(root.attributes("data-uid")).toBe("item-header-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(ParameterListItemHeader, {
			node: makeNode({
				uid: "item-header-1",
				nodeCommentId: "comment-1",
				diffStatus: "modified",
			}),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("modified")
	})

	it("lays the title and type out side by side", async ({ expect }) => {
		const wrapper = await mountNodeView(ParameterListItemHeader, {
			node: makeNode({ uid: "item-header-1" }),
		})

		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"items-center",
		)
	})
})
