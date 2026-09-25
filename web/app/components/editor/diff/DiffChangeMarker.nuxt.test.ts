import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import DiffChangeMarker from "./DiffChangeMarker.vue"
import { DiffStatus } from "./position-map"
import { makeNode } from "../test-helpers/node-view"
import { t } from "~/components/test-helpers"

describe("DiffChangeMarker", () => {
	it("shows the removals and additions of a modified node", async ({
		expect,
	}) => {
		const wrapper = await mountMarker({
			src: "b.png",
			alt: "a cat",
			diffStatus: DiffStatus.Modified,
			oldNode: { attrs: { src: "a.png" } },
		})

		expect(
			wrapper.findAll("[aria-hidden='true']").map((s) => s.text()),
		).toEqual([
			t("editor.diff-change-marker.removed", { count: 1 }),
			t("editor.diff-change-marker.added", { count: 2 }),
		])
		expect(wrapper.get(".sr-only").text()).toBe(
			t("editor.diff-change-marker.label", { removed: 1, added: 2 }),
		)
	})

	it("leaves out the side without changes", async ({ expect }) => {
		const wrapper = await mountMarker({
			alt: "a cat",
			diffStatus: DiffStatus.Modified,
			oldNode: { attrs: {} },
		})

		expect(
			wrapper.findAll("[aria-hidden='true']").map((s) => s.text()),
		).toEqual([t("editor.diff-change-marker.added", { count: 1 })])
	})

	it("renders nothing for a modified node whose counted attributes match", async ({
		expect,
	}) => {
		const wrapper = await mountMarker({
			src: "a.png",
			diffStatus: DiffStatus.Modified,
			oldNode: { attrs: { src: "a.png" } },
		})

		expect(wrapper.html()).toBe("<!--v-if-->")
	})

	it.for([DiffStatus.Added, DiffStatus.Removed, DiffStatus.Unchanged])(
		"renders nothing for a %s node",
		async (status, { expect }) => {
			const wrapper = await mountMarker({
				src: "b.png",
				diffStatus: status,
				oldNode: { attrs: { src: "a.png" } },
			})

			expect(wrapper.html()).toBe("<!--v-if-->")
		},
	)
})

function mountMarker(attrs: Record<string, unknown>) {
	return mountSuspended(DiffChangeMarker, {
		props: { node: makeNode(attrs) },
	})
}
