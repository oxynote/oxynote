import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import FileSelectInputItem from "./FileSelectInputItem.vue"
import { at, renderedIconNames, t } from "~/components/test-helpers"
import type { GitHubFileTreeItem } from "~/utils/api/github"

function file(name: string): GitHubFileTreeItem {
	return { type: "file", name: name, items: null, checksum: `sum-${name}` }
}

function folder(
	name: string,
	items: GitHubFileTreeItem[] | null = [],
): GitHubFileTreeItem {
	return { type: "folder", name: name, items: items, checksum: `sum-${name}` }
}

// the component renders itself for every sub-folder, and mounting it
// directly would make @nuxt/test-utils' mount helper answer that
// self-reference instead of the real component — so it goes inside a
// host of its own
function mountItem(props: Record<string, unknown> = {}) {
	return mountSuspended({
		setup: () => () =>
			h(FileSelectInputItem, {
				item: file("docs/readme.md"),
				selected: [],
				parentItems: [],
				emptyFolderPlaceholder: t(
					"editor.hooks.github-tracking.empty-folder-placeholder",
				),
				...props,
			}),
	})
}

function emittedSelections(wrapper: VueWrapper): unknown[][] {
	return wrapper.findComponent(FileSelectInputItem).emitted("selected") ?? []
}

function rows(wrapper: VueWrapper) {
	return wrapper.findAll("[class*='cursor-pointer']")
}

describe("<FileSelectInputItem>", () => {
	it("shows only the last part of a file's path", async ({ expect }) => {
		const wrapper = await mountItem()

		expect(wrapper.text()).toContain("readme.md")
		expect(wrapper.text()).not.toContain("docs/readme.md")
	})

	it("marks a file with a file icon", async ({ expect }) => {
		const wrapper = await mountItem()

		expect(renderedIconNames(wrapper)).toEqual(["mingcute:file-line"])
	})

	it("marks a folder with a closed folder icon", async ({ expect }) => {
		const wrapper = await mountItem({ item: folder("docs") })

		expect(renderedIconNames(wrapper)).toContain("mingcute:folder-fill")
	})

	it("ticks a file that is already selected", async ({ expect }) => {
		const wrapper = await mountItem({ selected: ["docs/readme.md"] })

		expect(renderedIconNames(wrapper)).toContain("mingcute:check-fill")
		expect(at(rows(wrapper), 0).attributes("data-selected")).toBe("")
	})

	it("leaves an unselected file unticked", async ({ expect }) => {
		const wrapper = await mountItem({ selected: ["docs/other.md"] })

		expect(renderedIconNames(wrapper)).not.toContain("mingcute:check-fill")
	})

	it("reports a file being selected", async ({ expect }) => {
		const item = file("docs/readme.md")
		const wrapper = await mountItem({ item: item })

		await at(rows(wrapper), 0).trigger("click")

		expect(emittedSelections(wrapper)).toEqual([[item, true, []]])
	})

	it("reports a selected file being deselected", async ({ expect }) => {
		const item = file("docs/readme.md")
		const wrapper = await mountItem({
			item: item,
			selected: ["docs/readme.md"],
		})

		await at(rows(wrapper), 0).trigger("click")

		expect(emittedSelections(wrapper)).toEqual([[item, false, []]])
	})

	it("keeps a folder's contents out of sight until it is opened", async ({
		expect,
	}) => {
		const wrapper = await mountItem({
			item: folder("docs", [file("docs/readme.md")]),
		})

		expect(wrapper.text()).not.toContain("readme.md")
	})

	it("lists a folder's contents once it is opened", async ({ expect }) => {
		const wrapper = await mountItem({
			item: folder("docs", [file("docs/readme.md")]),
		})

		await wrapper.get(".show-on-parent-hover").trigger("click")

		expect(wrapper.text()).toContain("readme.md")
		expect(renderedIconNames(wrapper)).toContain("mingcute:folder-open-fill")
	})

	it("says so when an opened folder is empty", async ({ expect }) => {
		const wrapper = await mountItem({ item: folder("docs", []) })

		await wrapper.get(".show-on-parent-hover").trigger("click")

		expect(wrapper.text()).toContain(
			t("editor.hooks.github-tracking.empty-folder-placeholder"),
		)
	})

	it("puts sub-folders before files", async ({ expect }) => {
		const wrapper = await mountItem({
			item: folder("docs", [
				file("docs/z.md"),
				folder("docs/api"),
				file("docs/a.md"),
			]),
		})

		await wrapper.get(".show-on-parent-hover").trigger("click")

		expect(
			rows(wrapper)
				.slice(1)
				.map((row) => row.text()),
		).toEqual(["api", "a.md", "z.md"])
	})

	it("reports a nested selection with the path that leads to it", async ({
		expect,
	}) => {
		const nested = file("docs/readme.md")
		const parent = folder("docs", [nested])
		const wrapper = await mountItem({ item: parent })
		await wrapper.get(".show-on-parent-hover").trigger("click")

		await at(rows(wrapper), 1).trigger("click")

		expect(emittedSelections(wrapper)).toEqual([[nested, true, [parent]]])
	})

	it("shows nothing under a folder that has no listing", async ({ expect }) => {
		const wrapper = await mountItem({ item: folder("docs", null) })

		await wrapper.get(".show-on-parent-hover").trigger("click")

		expect(wrapper.text()).toContain(
			t("editor.hooks.github-tracking.empty-folder-placeholder"),
		)
	})
})
