import { describe, it } from "vitest"
import MainBlock from "./MainBlock.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"

function contentClasses(wrapper: {
	find: (s: string) => { classes: () => string[] }
}) {
	return wrapper.find("[data-node-view-content]").classes()
}

describe("<SplitDocumentationMainBlock>", () => {
	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountNodeView(MainBlock, {
			node: makeNode({ uid: "block-1" }),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("block-1")
		expect(root.attributes("data-uid")).toBe("block-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(MainBlock, {
			node: makeNode({
				uid: "block-1",
				nodeCommentId: "comment-1",
				diffStatus: "added",
			}),
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("added")
	})

	it("lays the two sides out in document order by default", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(MainBlock, {
			node: makeNode({ uid: "block-1" }),
		})

		expect(contentClasses(wrapper)).toContain("flex-col")
		expect(contentClasses(wrapper)).not.toContain("flex-col-reverse")
	})

	it("reverses the two sides when the node is inversed", async ({ expect }) => {
		const wrapper = await mountNodeView(MainBlock, {
			node: makeNode({ uid: "block-1", inversed: true }),
		})

		expect(contentClasses(wrapper)).toContain("flex-col-reverse")
		expect(contentClasses(wrapper)).not.toContain("flex-col")
	})

	it("treats a missing inversed attribute as not inversed", async ({
		expect,
	}) => {
		const wrapper = await mountNodeView(MainBlock, {
			node: makeNode({ uid: "block-1", inversed: null }),
		})

		expect(contentClasses(wrapper)).toContain("flex-col")
	})
})
