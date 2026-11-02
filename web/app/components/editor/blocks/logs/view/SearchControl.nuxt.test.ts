import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, describe, it } from "vitest"
import SearchControl from "./SearchControl.vue"
import { findButtonByText, t } from "~/components/test-helpers"

// the focus tests attach to the document, which keeps them mounted
enableAutoUnmount(afterEach)

// focus is document-wide, so these tests cannot interleave
describe("<SearchControl>", { concurrent: false }, () => {
	it("shows only the search button while the search is empty", async ({
		expect,
	}) => {
		const wrapper = await mountSearch("")

		expect(wrapper.find("input").exists()).toBe(false)
		expect(
			findButtonByText(wrapper, t("editor.logs.view.search-label")).exists(),
		).toBe(true)
	})

	it("stays open while a search is applied", async ({ expect }) => {
		const wrapper = await mountSearch("timeout")

		expect(searchInput(wrapper).element.value).toBe("timeout")
		expect(searchInput(wrapper).attributes("placeholder")).toBe(
			t("editor.logs.view.search-placeholder"),
		)
	})

	it("opens and focuses the field from the search button", async ({
		expect,
	}) => {
		const wrapper = await mountSearch("")

		await findButtonByText(wrapper, t("editor.logs.view.search-label")).trigger(
			"click",
		)
		await nextTick()

		expect(document.activeElement).toBe(searchInput(wrapper).element)
	})

	it("passes on what the reader types", async ({ expect }) => {
		const wrapper = await mountSearch("")
		await openSearch(wrapper)

		await searchInput(wrapper).setValue("timeout")

		expect(wrapper.emitted("update:modelValue")).toEqual([["timeout"]])
	})

	it("clears the text on the first escape and stays open", async ({
		expect,
	}) => {
		const wrapper = await mountSearch("timeout")

		await searchInput(wrapper).trigger("keydown", { key: "Escape" })

		expect(wrapper.emitted("update:modelValue")).toEqual([[""]])
		expect(wrapper.find("input").exists()).toBe(true)
	})

	it("closes on an escape once the text is empty", async ({ expect }) => {
		const wrapper = await mountSearch("")
		await openSearch(wrapper)

		await searchInput(wrapper).trigger("keydown", { key: "Escape" })

		expect(wrapper.find("input").exists()).toBe(false)
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
	})

	it("closes when an empty field loses focus", async ({ expect }) => {
		const wrapper = await mountSearch("")
		await openSearch(wrapper)

		await searchInput(wrapper).trigger("blur")

		expect(wrapper.find("input").exists()).toBe(false)
	})

	it("stays open when a field with text loses focus", async ({ expect }) => {
		const wrapper = await mountSearch("timeout")

		await searchInput(wrapper).trigger("blur")

		expect(wrapper.find("input").exists()).toBe(true)
	})

	it("clears the text and keeps the focus from the clear button", async ({
		expect,
	}) => {
		const wrapper = await mountSearch("timeout")

		await findButtonByText(wrapper, t("editor.logs.view.clear-search")).trigger(
			"click",
		)

		expect(wrapper.emitted("update:modelValue")).toEqual([[""]])
		expect(document.activeElement).toBe(searchInput(wrapper).element)
	})
})

// the model follows every update, as a parent's v-model would
async function mountSearch(search: string) {
	const wrapper: VueWrapper = await mountSuspended(SearchControl, {
		props: {
			modelValue: search,
			"onUpdate:modelValue": (value: string) =>
				wrapper.setProps({ modelValue: value }),
		},
		attachTo: document.body,
	})

	return wrapper
}

async function openSearch(wrapper: VueWrapper) {
	await findButtonByText(wrapper, t("editor.logs.view.search-label")).trigger(
		"click",
	)
	await nextTick()
}

function searchInput(wrapper: VueWrapper) {
	return wrapper.get<HTMLInputElement>("input")
}
