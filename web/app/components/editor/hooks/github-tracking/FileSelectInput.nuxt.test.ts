import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it } from "vitest"
import FileSelectInput from "./FileSelectInput.vue"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"
import type { GitHubFileTreeItem } from "~/utils/api/github"

function file(name: string): GitHubFileTreeItem {
	return { type: "file", name: name, items: null, checksum: `sum-${name}` }
}

function folder(
	name: string,
	items: GitHubFileTreeItem[] = [],
): GitHubFileTreeItem {
	return { type: "folder", name: name, items: items, checksum: `sum-${name}` }
}

function mountInput(props: Record<string, unknown> = {}) {
	return mountSuspended(FileSelectInput, {
		props: {
			placeholder: t("editor.hooks.github-tracking.path-select-placeholder"),
			emptyFolderPlaceholder: t(
				"editor.hooks.github-tracking.empty-folder-placeholder",
			),
			options: [file("readme.md")],
			...props,
		},
		slots: {
			label: () => t("editor.hooks.github-tracking.path-select-label"),
			"selection-text": (slotProps: { count: number }) =>
				`${slotProps.count} files selected`,
		},
	})
}

// the file tree is teleported into <body> with the popover
async function openTree(wrapper: VueWrapper) {
	await wrapper.get("button").trigger("click")
	await nextTick()
}

function treeRow(text: string): HTMLElement {
	const row = Array.from(
		document.body.querySelectorAll<HTMLElement>(
			"[data-slot='popover-content'] [class*='cursor-pointer']",
		),
	).find((candidate) => candidate.textContent.trim() === text)
	if (!row) {
		throw new Error(`no tree row rendering "${text}"`)
	}

	return row
}

function treeRows(): string[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>(
			"[data-slot='popover-content'] [class*='cursor-pointer']",
		),
	).map((row) => row.textContent.trim())
}

// the popover body is teleported into a shared <body>, so these tests
// cannot interleave
describe("<FileSelectInput>", { concurrent: false }, () => {
	beforeEach(clearTeleportedOverlays)

	it("labels the field", async ({ expect }) => {
		const wrapper = await mountInput()

		expect(wrapper.text()).toContain(
			t("editor.hooks.github-tracking.path-select-label"),
		)
	})

	it("prompts for a selection while nothing is picked", async ({ expect }) => {
		const wrapper = await mountInput()

		expect(wrapper.get("button").text()).toContain(
			t("editor.hooks.github-tracking.path-select-placeholder"),
		)
	})

	it("counts what the reader has picked", async ({ expect }) => {
		const wrapper = await mountInput({ modelValue: ["a.md", "b.md"] })

		expect(wrapper.get("button").text()).toContain("2 files selected")
	})

	it("stays editable by default", async ({ expect }) => {
		const wrapper = await mountInput()

		expect(wrapper.get("button").attributes("disabled")).toBeUndefined()
	})

	it("locks the field when the host asks it to", async ({ expect }) => {
		const wrapper = await mountInput({ disabled: true })

		expect(wrapper.get("button").attributes("disabled")).toBeDefined()
	})

	it("puts folders before files in the tree", async ({ expect }) => {
		const wrapper = await mountInput({
			options: [file("z.md"), folder("docs"), file("a.md")],
		})

		await openTree(wrapper)

		expect(treeRows()).toEqual(["docs", "a.md", "z.md"])
	})

	it("picks the file the reader clicked", async ({ expect }) => {
		const wrapper = await mountInput({ options: [file("readme.md")] })
		await openTree(wrapper)

		treeRow("readme.md").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([[["readme.md"]]])
	})

	it("adds to what was already picked", async ({ expect }) => {
		const wrapper = await mountInput({
			options: [file("a.md"), file("b.md")],
			modelValue: ["a.md"],
		})
		await openTree(wrapper)

		treeRow("b.md").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([[["a.md", "b.md"]]])
	})

	it("drops a file the reader unpicked", async ({ expect }) => {
		const wrapper = await mountInput({
			options: [file("a.md"), file("b.md")],
			modelValue: ["a.md", "b.md"],
		})
		await openTree(wrapper)

		treeRow("a.md").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([[["b.md"]]])
	})

	it("picks a whole folder along with everything inside it", async ({
		expect,
	}) => {
		const wrapper = await mountInput({
			options: [folder("docs", [file("docs/a.md"), file("docs/b.md")])],
		})
		await openTree(wrapper)

		treeRow("docs").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[["docs", "docs/a.md", "docs/b.md"]],
		])
	})

	it("drops a whole folder along with everything inside it", async ({
		expect,
	}) => {
		const wrapper = await mountInput({
			options: [folder("docs", [file("docs/a.md")])],
			modelValue: ["docs", "docs/a.md", "other.md"],
		})
		await openTree(wrapper)

		treeRow("docs").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([[["other.md"]]])
	})

	it("picks a folder's contents recursively", async ({ expect }) => {
		const wrapper = await mountInput({
			options: [
				folder("docs", [folder("docs/api", [file("docs/api/index.md")])]),
			],
		})
		await openTree(wrapper)

		treeRow("docs").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[["docs", "docs/api", "docs/api/index.md"]],
		])
	})

	it("picks a file only once", async ({ expect }) => {
		const wrapper = await mountInput({
			options: [folder("docs", [file("docs/a.md")])],
			modelValue: ["docs/a.md"],
		})
		await openTree(wrapper)

		treeRow("docs").click()
		await nextTick()

		expect(wrapper.emitted("update:modelValue")).toEqual([
			[["docs/a.md", "docs"]],
		])
	})
})
