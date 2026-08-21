import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import HeaderBreadcrumbs from "./HeaderBreadcrumbs.vue"
import type { DocumentBreadcrumb } from "./sidebar"

function crumb(name: string): DocumentBreadcrumb {
	return {
		id: `id-${name}`,
		name: name,
		href: `/${name}`,
		icon: `lucide:${name}`,
	}
}

function crumbs(...names: string[]): DocumentBreadcrumb[] {
	return names.map(crumb)
}

// the crumbs the component actually rendered, in document order. The
// current page is a <span>, the ancestors are links
function renderedNames(wrapper: VueWrapper) {
	return wrapper.findAll("span.truncate").map((span) => span.text())
}

describe("<HeaderBreadcrumbs>", () => {
	it("renders nothing when there are no breadcrumbs", async ({ expect }) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: [] },
		})

		expect(wrapper.find("nav").exists()).toBe(false)
	})

	it("shows only the current page when it is the whole path", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Runbook") },
		})

		expect(renderedNames(wrapper)).toEqual(["Runbook"])
		expect(wrapper.findAll("a")).toHaveLength(0)
	})

	it("links the ancestor and shows the current page for a two-level path", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "Leaf") },
		})

		expect(renderedNames(wrapper)).toEqual(["Root", "Leaf"])
		expect(wrapper.findAll("a").map((a) => a.attributes("href"))).toEqual([
			"/Root",
		])
	})

	it("shows all three crumbs of a three-level path without an ellipsis", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "Middle", "Leaf") },
		})

		expect(renderedNames(wrapper)).toEqual(["Root", "Middle", "Leaf"])
		expect(wrapper.find("[data-collapsed-exist]").exists()).toBe(false)
	})

	it("collapses the middle of a deeper path into an ellipsis", async ({
		expect,
	}) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "A", "B", "Parent", "Leaf") },
		})

		expect(renderedNames(wrapper)).toEqual(["Root", "Parent", "Leaf"])
		expect(wrapper.find("[data-collapsed-exist]").exists()).toBe(true)
	})

	it("links every ancestor it renders at its own href", async ({ expect }) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "A", "Parent", "Leaf") },
		})

		expect(wrapper.findAll("a").map((a) => a.attributes("href"))).toEqual([
			"/Root",
			"/Parent",
		])
	})

	it("shows each crumb's own icon", async ({ expect }) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "Leaf") },
		})

		expect(
			wrapper
				.findAll('[data-slot="breadcrumb-item"] .iconify')
				.map((i) => i.classes().find((c) => c.startsWith("i-"))),
		).toEqual(["i-lucide:Root", "i-lucide:Leaf"])
	})

	it("re-renders when the breadcrumb path changes", async ({ expect }) => {
		const wrapper = await mountSuspended(HeaderBreadcrumbs, {
			props: { breadcrumbs: crumbs("Root", "Leaf") },
		})

		await wrapper.setProps({ breadcrumbs: crumbs("Other") })

		expect(renderedNames(wrapper)).toEqual(["Other"])
	})
})
