import { mountSuspended } from "@nuxt/test-utils/runtime"
import { DOMWrapper, enableAutoUnmount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { setResponseStatus } from "h3"
import TagEditModal from "./TagEditModal.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockDeferredEndpoint,
	mockEndpoint,
	seedQueryData,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	settleMutations,
	t,
	teleportedButton,
} from "./test-helpers"
import { stubSelectableColors } from "./editor/test-helpers/theme"

const TAG_ID = makeXid("tag")
const TARGET = { id: TAG_ID, tagName: "Production", color: "#010000" }
const UPDATE_URL = `/api/tags/${TAG_ID}`

enableAutoUnmount(afterEach)

// the modal, query cache and palette share the app's document
describe("<TagEditModal>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		stubSelectableColors()
		const context = {
			clearRect: () => undefined,
			fillRect: () => undefined,
			fillStyle: "",
			getImageData: () => ({
				data: [
					...(context.fillStyle.startsWith("rgb(")
						? (context.fillStyle.match(/\d+/g) ?? []).map(Number)
						: [1, 3, 5].map((offset) =>
								Number.parseInt(
									context.fillStyle.slice(offset, offset + 2),
									16,
								),
							)),
					255,
				],
			}),
		}
		vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
			context as unknown as CanvasRenderingContext2D,
		)
		seedQueryData(["tags", "tree"], [{ ...TARGET, hidden: false }])
		mockEndpoint("GET", "/api/tags/tree", () => [{ ...TARGET, hidden: false }])
	})
	afterEach(disposeMockEndpoints)

	it("stays closed without a target", async ({ expect }) => {
		await mountModal(null)

		expect(dialog()).toBeNull()
	})

	it("starts with the existing name and colour and no unsaved changes", async ({
		expect,
	}) => {
		await mountModal()

		expect(nameInput().element.value).toBe(TARGET.tagName)
		expect(
			teleportedButton(t("sidebar.tag-edit-modal.color-label")).querySelector(
				"span",
			)?.style.backgroundColor,
		).toBe("#010000")
		expect(saveButton().disabled).toBe(true)
	})

	it("opens the colour picker above the dialog", async ({ expect }) => {
		await mountModal()

		teleportedButton(t("sidebar.tag-edit-modal.color-label")).click()
		await nextTick()

		expect(
			document.body
				.querySelector("[data-slot='popover-content']")
				?.classList.contains("z-[calc(theme(zIndex.modal)+5)]!"),
		).toBe(true)
	})

	it("keeps Save disabled when the selected CSS colour matches the saved hex", async ({
		expect,
	}) => {
		document.documentElement.style.setProperty(
			"--selectable-color-1",
			"rgb(1, 0, 0)",
		)
		await mountModal()

		await chooseColour(0)

		expect(saveButton().disabled).toBe(true)
	})

	it("discards changes when the backdrop is clicked", async ({ expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountModal()
		await nameInput().setValue("Discard me")
		await settleMutations()

		clickBackdrop()
		await nextTick()

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("discards changes on Escape", async ({ expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountModal()
		await nameInput().setValue("Discard me")

		nameInput().element.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
		)
		await nextTick()

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it.for([
		{
			name: "renames",
			rename: true,
			recolour: false,
			expected: { tagName: "Live" },
		},
		{
			name: "recolours",
			rename: false,
			recolour: true,
			expected: { color: "#040000" },
		},
		{
			name: "renames and recolours",
			rename: true,
			recolour: true,
			expected: { tagName: "Live", color: "#040000" },
		},
	])(
		"$name the target and closes after saving",
		async ({ rename, recolour, expected }, { expect }) => {
			const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
			const wrapper = await mountModal()

			if (rename) {
				await nameInput().setValue(" Live ")
			}

			if (recolour) {
				await chooseColour()
			}

			saveButton().click()
			await settleMutations()

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual(expected)
			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
		},
	)

	it.for(["", "   "])(
		"does not save an empty name '%s'",
		async (name, { expect }) => {
			const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
			await mountModal()

			await nameInput().setValue(name)
			await form().trigger("submit")
			await settleMutations()

			expect(saveButton().disabled).toBe(true)
			expect(calls).toHaveLength(0)
		},
	)

	it.for([
		"sidebar.tag-edit-modal.cancel-button",
		"general.modal-close-screen-reader-hint",
	])("discards changes through %s", async (key, { expect }) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountModal()
		await nameInput().setValue("Live")

		teleportedButton(t(key)).click()
		await nextTick()

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})

	it("restores saved values when reopened after cancellation", async ({
		expect,
	}) => {
		const calls = mockEndpoint("PUT", UPDATE_URL, () => ({}))
		const wrapper = await mountModal()
		await nameInput().setValue("Discard me")
		await wrapper.setProps({ modelValue: null })

		await wrapper.setProps({ modelValue: { ...TARGET } })

		expect(nameInput().element.value).toBe(TARGET.tagName)
		expect(saveButton().disabled).toBe(true)
		expect(calls).toHaveLength(0)
	})

	it.for([
		{
			name: "duplicate",
			code: "tag.duplicate_name",
			key: "sidebar.tag-edit-modal.duplicate-name",
		},
		{
			name: "failed",
			code: "unknown",
			key: "sidebar.errors.update-tag-failed",
		},
	])(
		"keeps a $name save open so it can be corrected and retried",
		async ({ code, key }, { expect }) => {
			let fails = true
			const calls = mockEndpoint("PUT", UPDATE_URL, (_call, event) => {
				if (fails) {
					setResponseStatus(event, 409)
					return { code }
				}

				return {}
			})
			const wrapper = await mountModal()
			await nameInput().setValue("Live")

			saveButton().click()
			await settleMutations()

			expect(dialog()?.querySelector("[role='alert']")?.textContent).toBe(
				t(key),
			)
			expect(wrapper.emitted("update:modelValue")).toBeUndefined()
			expect(nameInput().element.value).toBe("Live")
			expect(saveButton().disabled).toBe(false)
			fails = false
			saveButton().click()
			await settleMutations()

			expect(calls).toHaveLength(2)
			expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
		},
	)

	it("prevents repeated submissions and dismissal while saving", async ({
		expect,
	}) => {
		const request = mockDeferredEndpoint("PUT", UPDATE_URL)
		const wrapper = await mountModal()
		await nameInput().setValue("Live")
		saveButton().click()
		await request.reached

		clickBackdrop()
		await form().trigger("submit")
		teleportedButton(t("general.modal-close-screen-reader-hint")).click()
		nameInput().element.dispatchEvent(
			new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
		)
		await nextTick()

		expect(request.calls).toHaveLength(1)
		expect(saveButton().disabled).toBe(true)
		expect(nameInput().element.disabled).toBe(true)
		expect(
			teleportedButton(t("sidebar.tag-edit-modal.cancel-button")).disabled,
		).toBe(true)
		expect(wrapper.emitted("update:modelValue")).toBeUndefined()
		request.resolve({})
		await settleMutations()

		expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([null])
	})
})

function mountModal(target: typeof TARGET | null = { ...TARGET }) {
	return mountSuspended(TagEditModal, { props: { modelValue: target } })
}

function dialog() {
	return document.body.querySelector<HTMLElement>(
		"[data-slot='dialog-content']",
	)
}

function nameInput() {
	const input = dialog()?.querySelector<HTMLInputElement>("input")
	if (!input) {
		throw new Error("missing tag name input")
	}

	return new DOMWrapper(input)
}

function saveButton() {
	return teleportedButton(t("sidebar.tag-edit-modal.save-button"))
}

async function chooseColour(index = 3) {
	teleportedButton(t("sidebar.tag-edit-modal.color-label")).click()
	await nextTick()
	const button = document.body.querySelectorAll<HTMLButtonElement>(
		"[data-slot='popover-content'] button",
	)[index]
	if (!button) {
		throw new Error("missing tag colour swatch")
	}

	button.click()
	await nextTick()
}

function form() {
	const element = dialog()?.querySelector<HTMLFormElement>("form")
	if (!element) {
		throw new Error("missing tag form")
	}

	return new DOMWrapper(element)
}

function clickBackdrop() {
	document.body.querySelector("[data-slot='dialog-overlay']")?.dispatchEvent(
		new PointerEvent("pointerdown", {
			bubbles: true,
			pointerType: "mouse",
			button: 0,
		}),
	)
}
