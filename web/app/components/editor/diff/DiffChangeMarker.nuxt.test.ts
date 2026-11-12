import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import { flushPromises } from "@vue/test-utils"
import { defineComponent, h } from "vue"
import DiffChangeMarker from "./DiffChangeMarker.vue"
import { DiffStatus } from "./position-map"
import { makeNode } from "../test-helpers/node-view"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

const MODIFIED_IMAGE = {
	src: "b.png",
	diffStatus: DiffStatus.Modified,
	oldNode: { attrs: { src: "a.png" } },
}

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

	it.for([DiffStatus.Added, DiffStatus.Unchanged])(
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

	it("renders nothing for a removed node with no old places", async ({
		expect,
	}) => {
		const wrapper = await mountMarker({
			src: "a.png",
			diffStatus: DiffStatus.Removed,
		})

		expect(wrapper.html()).toBe("<!--v-if-->")
	})

	// a removed grid block shows after its grid. Its old node keeps the
	// places where it was.
	it("counts the old places of a removed grid block as removed", async ({
		expect,
	}) => {
		const wrapper = await mountMarker({
			diffStatus: DiffStatus.Removed,
			oldNode: {
				attrs: { gridPlace: { row: 1, column: 12 }, stackPlace: 2 },
			},
		})

		expect(
			wrapper.findAll("[aria-hidden='true']").map((s) => s.text()),
		).toEqual([t("editor.diff-change-marker.removed", { count: 2 })])
	})

	// the card lands in <body>, which every test in the file shares
	describe("when clicked", { concurrent: false }, () => {
		beforeEach(() => {
			clearTeleportedOverlays()
		})

		it("opens the details from its slot", async ({ expect }) => {
			const wrapper = await mountMarker(MODIFIED_IMAGE, () => "image details")

			await wrapper.get("span").trigger("click")

			expect(document.body.textContent).toContain("image details")
		})

		it("closes the details on a click elsewhere", async ({ expect }) => {
			const wrapper = await mountMarker(MODIFIED_IMAGE, () => "image details")
			await wrapper.get("span").trigger("click")
			// the popover's outside-click listener is attached a task after it
			// opens
			await new Promise((resolve) => setTimeout(resolve, 0))

			document.body.dispatchEvent(
				new PointerEvent("pointerdown", { bubbles: true, button: 0 }),
			)
			await flushPromises()

			expect(document.body.textContent).not.toContain("image details")
		})

		it("looks clickable only when it has details", async ({ expect }) => {
			const withDetails = await mountMarker(MODIFIED_IMAGE, () => "details")
			const withoutDetails = await mountMarker(MODIFIED_IMAGE)

			expect(withDetails.get("span").classes()).toEqual(
				expect.arrayContaining(["cursor-pointer", "active:opacity-90"]),
			)
			expect(withoutDetails.get("span").classes()).not.toContain(
				"cursor-pointer",
			)
			expect(withoutDetails.get("span").classes()).not.toContain(
				"active:opacity-90",
			)
		})

		it("opens no card when it has no details", async ({ expect }) => {
			const wrapper = await mountMarker(MODIFIED_IMAGE)

			await wrapper.get("span").trigger("click")

			expect(
				document.body.querySelector("[data-slot='popover-content']"),
			).toBeNull()
		})
	})

	describe("inside a link", () => {
		it("keeps a click on the pill from reaching the element around it", async ({
			expect,
		}) => {
			const onClick = vi.fn()
			const wrapper = await mountSuspended(
				defineComponent({
					render: () =>
						h("a", { href: "#file", onClick: onClick }, [
							h(DiffChangeMarker, { node: makeNode(MODIFIED_IMAGE) }),
						]),
				}),
			)

			const event = new MouseEvent("click", { bubbles: true, cancelable: true })
			wrapper.get("a > span").element.dispatchEvent(event)

			expect(event.defaultPrevented).toBe(true)
			expect(onClick).not.toHaveBeenCalled()
		})
	})
})

function mountMarker(attrs: Record<string, unknown>, details?: () => string) {
	return mountSuspended(DiffChangeMarker, {
		props: { node: makeNode(attrs) },
		slots: details ? { default: details } : {},
	})
}
