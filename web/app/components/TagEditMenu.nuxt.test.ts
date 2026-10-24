import { mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import {
	at,
	clearTeleportedOverlays,
	findButtonByText,
	raisedToasts,
	settleMutations,
	t,
} from "./test-helpers"
import TagEditMenu from "./TagEditMenu.vue"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const TAG_A = makeXid("taga")
const TAG_B = makeXid("tagb")
const UPDATE_URL = `/api/tags/${TAG_A}`

enableAutoUnmount(afterEach)

// the query cache, the mocked toast and the popover teleported into
// <body> are shared by every mount
describe("<TagEditMenu>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		vi.mocked(toast.custom).mockClear()
		seedQueryData(["tags", "tree"], tree())
		mockEndpoint("GET", "/api/tags/tree", () => tree())
	})
	afterEach(disposeMockEndpoints)

	it("fills the form with the tag's name and colour", async ({ expect }) => {
		const wrapper = await mountMenu()

		expect
			.soft((wrapper.get("input").element as HTMLInputElement).value)
			.toBe("Production")
		expect
			.soft(colorButton(wrapper).get("div").attributes("style"))
			.toContain("background-color: var(--selectable-color-1)")
	})

	it("renames the tag, trimmed, and says so once saved", async ({ expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountMenu()
		await wrapper.get("input").setValue(" Live ")

		await wrapper.get("form").trigger("submit")
		await settleMutations()

		expect.soft(calls.map((call) => call.body)).toEqual([{ tagName: "Live" }])
		expect.soft(wrapper.emitted("close")).toHaveLength(1)
		expect.soft(raisedToasts()).toEqual([
			expect.objectContaining({
				type: "success",
				title: t("sidebar.tag-edit-menu.update-success", { name: "Live" }),
			}),
		])
	})

	it("recolours the tag and says so once saved", async ({ expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountMenu()
		await pickColor(wrapper, 3)

		await wrapper.get("form").trigger("submit")
		await settleMutations()

		expect.soft(calls.map((call) => call.body)).toEqual([{ color: "yellow" }])
		expect.soft(wrapper.emitted("close")).toHaveLength(1)
		expect.soft(raisedToasts()).toEqual([
			expect.objectContaining({
				type: "success",
				title: t("sidebar.tag-edit-menu.update-success", {
					name: "Production",
				}),
			}),
		])
	})

	it.for([
		{ name: "while nothing changed", input: "Production" },
		{ name: "without a name", input: "  " },
		{ name: "with a name another tag uses", input: "Staging" },
	])("saves nothing $name", async ({ input }, { expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountMenu()
		await wrapper.get("input").setValue(input)

		await wrapper.get("form").trigger("submit")
		await settleMutations()

		expect.soft(saveButton(wrapper).attributes("disabled")).toBeDefined()
		expect.soft(calls).toHaveLength(0)
		expect.soft(wrapper.emitted("close")).toBeUndefined()
	})

	it("says when another tag already uses the name", async ({ expect }) => {
		const wrapper = await mountMenu()

		await wrapper.get("input").setValue("Staging")

		expect(wrapper.text()).toContain(t("sidebar.tag-edit-menu.name-taken"))
	})

	it.for([
		{
			name: "keeps ArrowLeft in the name field",
			input: { key: "ArrowLeft", target: "input" },
			expected: 0,
		},
		{
			name: "leaves ArrowLeft on a button to the menu",
			input: { key: "ArrowLeft", target: "button" },
			expected: 1,
		},
		{
			name: "keeps Tab inside the form",
			input: { key: "Tab", target: "button" },
			expected: 0,
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountMenu()
		const reachedMenu = vi.fn()
		wrapper
			.get("form")
			.element.parentElement?.addEventListener("keydown", reachedMenu)

		wrapper
			.get(input.target)
			.element.dispatchEvent(
				new KeyboardEvent("keydown", { key: input.key, bubbles: true }),
			)

		expect(reachedMenu).toHaveBeenCalledTimes(expected)
	})

	it.for([
		{
			name: "names the clash when the server says the name is taken",
			input: { status: 409, body: { code: "tag.duplicate_name" } },
			expected: "sidebar.tag-edit-menu.name-taken",
		},
		{
			name: "raises a toast when the tag cannot be saved",
			input: { status: 500, body: { code: "general" } },
			expected: "sidebar.errors.update-tag-failed",
		},
	])("$name", async ({ input, expected }, { expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, (_call, event) => {
			setResponseStatus(event, input.status)

			return input.body
		})
		const wrapper = await mountMenu()
		await wrapper.get("input").setValue("Live")

		await wrapper.get("form").trigger("submit")
		await settleMutations()

		expect.soft(calls).toHaveLength(1)
		expect.soft(wrapper.emitted("close")).toHaveLength(1)
		expect
			.soft(raisedToasts())
			.toEqual([expect.objectContaining({ type: "error", title: t(expected) })])
	})
})

function productionTag(): TagTreeElement {
	return {
		id: TAG_A,
		tagName: "Production",
		color: "red",
		hidden: false,
		documents: null,
	}
}

function tree(): TagTreeElement[] {
	return [
		productionTag(),
		{
			id: TAG_B,
			tagName: "Staging",
			color: "blue",
			hidden: false,
			documents: null,
		},
	]
}

function mountMenu() {
	return mountSuspended(TagEditMenu, { props: { tag: productionTag() } })
}

function colorButton(wrapper: VueWrapper) {
	return wrapper.get(
		`button[title='${t("sidebar.tag-edit-menu.color-title")}']`,
	)
}

function saveButton(wrapper: VueWrapper) {
	return findButtonByText(wrapper, t("sidebar.tag-edit-menu.update-button"))
}

// the swatches sit in a popover, teleported into <body>
async function pickColor(wrapper: VueWrapper, index: number) {
	await colorButton(wrapper).trigger("click")

	const swatches = document.body.querySelectorAll<HTMLButtonElement>(
		"[data-slot='popover-content'] button",
	)
	at(Array.from(swatches), index).click()
	await nextTick()
}
