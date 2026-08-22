import { describe, it } from "vitest"
import GridView from "./GridView.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"

describe("<MetricGridView>", () => {
	it("marks itself as the metric grid", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, { node: makeNode({}) })

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-type")).toBe("metric-grid")
		expect(root.classes()).toContain("metric-grid")
	})

	it("exposes the node's diff status on the wrapper", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, {
			node: makeNode({ diffStatus: "added" }),
		})

		expect(
			wrapper.get("[data-node-view-wrapper]").attributes("data-diff-status"),
		).toBe("added")
	})

	it("lays the blocks out in a grid", async ({ expect }) => {
		const wrapper = await mountNodeView(GridView, { node: makeNode({}) })

		expect(wrapper.get("[data-node-view-content]").classes()).toContain(
			"metric-grid-content",
		)
		expect(wrapper.get("[data-node-view-content]").classes()).toContain("grid")
	})
})
