import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { flushPromises } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import SearchResultRow from "./SearchResultRow.vue"
import {
	mountUnderTooltipProvider,
	openTooltipText,
	renderedIconNames,
	t,
} from "./test-helpers"

// happy-dom lays nothing out, so the breadcrumb line never learns its
// width. The tests set the width by hand and count a character as one
// unit of it.
const breadcrumbLine = vi.hoisted(() => ({
	width: null as unknown as { value: number },
	capacity: 0,
}))
mockNuxtImport("useElementSize", () => () => ({ width: breadcrumbLine.width }))

const HREF = "/Acme/Shipments-doc1"
const BREADCRUMBS = ["Acme", "Handbook", "Engineering", "Reference", "Objects"]

// the stand-in for the measured width is shared by every mount
describe("<SearchResultRow>", { concurrent: false }, () => {
	beforeEach(() => {
		breadcrumbLine.width = ref(0)
		breadcrumbLine.capacity = Number.POSITIVE_INFINITY
	})

	it("links to the page", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(wrapper.find("a").attributes("href")).toBe(HREF)
	})

	it("shows the page's own icon", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(renderedIconNames(wrapper)).toEqual(["mingcute:truck-line"])
	})

	it("names the page", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(wrapper.find(".font-semibold").text()).toBe("Shipments")
	})

	// what the sanitizer keeps of a highlight is covered by its own
	// browser tests, so the name here carries none
	it("prefers the name the search highlighted", async ({ expect }) => {
		const wrapper = await mountRow({ titleHtml: "Shipments, highlighted" })

		expect(wrapper.find(".font-semibold").text()).toBe("Shipments, highlighted")
	})

	it.for([
		{ name: "tags the page that is open", input: true },
		{ name: "leaves any other page untagged", input: false },
	])("$name", async ({ input }, { expect }) => {
		const wrapper = await mountRow({}, { current: input })

		expect(wrapper.text().includes(t("sidebar.search.current-page"))).toBe(
			input,
		)
	})

	it("shows the note it is given beside the name", async ({ expect }) => {
		const wrapper = await mountRow({}, { meta: "Lena · 2h" })

		expect(wrapper.find(".h-5").text()).toBe("ShipmentsLena · 2h")
	})

	it("explains the note in a tooltip once it is focused", async ({
		expect,
	}) => {
		const wrapper = await mountRow({}, { metaTooltip: "Last edited by Lena" })

		await wrapper.get("[data-slot='tooltip-trigger']").trigger("focus")
		await flushPromises()

		expect(openTooltipText(wrapper)).toBe("Last edited by Lena")
	})

	it("shows the branch label it is given", async ({ expect }) => {
		const wrapper = await mountRow({}, { branchLabel: "Draft copy" })

		expect(wrapper.text()).toContain("Draft copy")
		expect(renderedIconNames(wrapper)).toContain("mingcute:git-branch-line")
	})

	it("shows no branch label when it is given none", async ({ expect }) => {
		const wrapper = await mountRow()

		expect(renderedIconNames(wrapper)).not.toContain("mingcute:git-branch-line")
	})

	describe("breadcrumbs", { concurrent: false }, () => {
		beforeEach(() => {
			vi.spyOn(Element.prototype, "scrollWidth", "get").mockImplementation(
				function (this: Element) {
					return this.textContent.length
				},
			)
			vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(
				() => breadcrumbLine.capacity,
			)
		})

		it("lists the workspace and the parent pages", async ({ expect }) => {
			const wrapper = await mountRow()

			expect(breadcrumbs(wrapper)).toBe(written(BREADCRUMBS))
		})

		it.for([
			{
				name: "keeps the whole path while it fits",
				input: BREADCRUMBS,
				expected: BREADCRUMBS,
			},
			{
				name: "drops the middle steps first, the ones nearest the page last",
				input: ["Acme", "…", "Reference", "Objects"],
				expected: ["Acme", "…", "Reference", "Objects"],
			},
			{
				name: "keeps the workspace beside the last step",
				input: ["Acme", "…", "Objects"],
				expected: ["Acme", "…", "Objects"],
			},
			{
				name: "drops the workspace once the middle is gone",
				input: ["…", "Objects"],
				expected: ["…", "Objects"],
			},
			{
				name: "leaves the shortest path to be cut off when nothing fits",
				input: ["…"],
				expected: ["…", "Objects"],
			},
		])("$name", async ({ input, expected }, { expect }) => {
			const wrapper = await mountRow()

			await resizeLine(written(input).length)

			expect(breadcrumbs(wrapper)).toBe(written(expected))
		})

		it("writes the whole path again once the line has room for it", async ({
			expect,
		}) => {
			const wrapper = await mountRow()
			await resizeLine(written(["…", "Objects"]).length)
			expect(breadcrumbs(wrapper)).toBe(written(["…", "Objects"]))

			await resizeLine(written(BREADCRUMBS).length)

			expect(breadcrumbs(wrapper)).toBe(written(BREADCRUMBS))
		})

		it("fits the path again when the parent pages change", async ({
			expect,
		}) => {
			const wrapper = await mountRow()
			await resizeLine(written(["Acme", "…", "Objects"]).length)

			await wrapper.setRowProps({ breadcrumbs: ["Acme", "Objects"] })
			await flushPromises()

			expect(breadcrumbs(wrapper)).toBe(written(["Acme", "Objects"]))
		})

		it("has nothing to drop from a path of one step", async ({ expect }) => {
			const wrapper = await mountRow({}, { breadcrumbs: ["Acme"] })

			await resizeLine(1)

			expect(breadcrumbs(wrapper)).toBe("Acme")
		})
	})
})

async function mountRow(
	document: Record<string, unknown> = {},
	props: Record<string, unknown> = {},
) {
	const rowProps = reactive<Record<string, unknown>>({
		document: {
			title: "Shipments",
			titleHtml: null,
			icon: "mingcute:truck-line",
			...document,
		},
		href: HREF,
		breadcrumbs: BREADCRUMBS,
		meta: "1h",
		metaTooltip: "Last edited on Mar 14",
		branchLabel: null,
		current: false,
		...props,
	})
	const wrapper = await mountUnderTooltipProvider(SearchResultRow, {
		props: rowProps,
	})

	// the wrapper is the provider's, so its setProps would not reach the row
	return Object.assign(wrapper, {
		setRowProps: async (next: Record<string, unknown>) => {
			Object.assign(rowProps, next)
			await nextTick()
		},
	})
}

function breadcrumbs(wrapper: Awaited<ReturnType<typeof mountRow>>) {
	return wrapper.find(".truncate:not(.font-semibold)").text()
}

// the path as the line writes it, a character counting as one unit of width
function written(crumbs: string[]) {
	return crumbs
		.map((crumb) =>
			crumb === "…" ? t("sidebar.search.breadcrumb-collapsed") : crumb,
		)
		.join(t("sidebar.search.breadcrumb-separator"))
}

async function resizeLine(capacity: number) {
	breadcrumbLine.capacity = capacity
	breadcrumbLine.width.value = capacity
	await flushPromises()
}
