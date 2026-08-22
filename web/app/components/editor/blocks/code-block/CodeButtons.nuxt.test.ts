import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import CodeButtons from "./CodeButtons.vue"
import {
	makeEditor,
	makeNode,
	mountNodeView,
	type EditorStub,
} from "../../test-helpers/node-view"
import { emitFrom } from "~/components/test-helpers"
import { Select } from "~/components/shadcn/ui/select"

const GO_SNIPPET =
	'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("hi")\n}\n'

const COPIED_RESET_MS = 700

function mountButtons(
	options: {
		attrs?: Record<string, unknown>
		textContent?: string
		type?: string
		editor?: EditorStub
		updateAttributes?: (attrs: Record<string, unknown>) => void
		onError?: (error: unknown) => void
	} = {},
) {
	return mountNodeView(
		CodeButtons,
		{
			node: makeNode(
				{ uid: "code-1", language: null, ...options.attrs },
				{ textContent: options.textContent ?? "" },
			),
			extension: { options: { type: options.type ?? "document" } },
			editor: (options.editor ?? makeEditor()).editor,
			...(options.updateAttributes
				? { updateAttributes: options.updateAttributes }
				: {}),
		},
		{ onError: options.onError },
	)
}

// the only plain button in the row is the copy one; the other is the
// language select's trigger
function copyButton(wrapper: VueWrapper) {
	return wrapper.get("[data-slot='button']")
}

function selectedLanguage(wrapper: VueWrapper): string {
	return wrapper.get("[data-slot='select-value']").text()
}

// the copy icon turns green for a moment once the code is on the clipboard
function copyIconMarked(wrapper: VueWrapper): boolean {
	return copyButton(wrapper).get(".iconify").classes().includes("text-chart-2")
}

// the editable flag is a shared cookie state, and the clipboard spy and
// fake timers are global, so these tests cannot interleave
describe("<CodeButtons>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorMeta().setEditable(true)
	})

	it("shows the language stored on the node", async ({ expect }) => {
		const wrapper = await mountButtons({
			attrs: { language: "python" },
			textContent: GO_SNIPPET,
		})

		expect(selectedLanguage(wrapper)).toBe("Python")
	})

	it("detects the language of an unlabelled block from its code", async ({
		expect,
	}) => {
		const wrapper = await mountButtons({ textContent: GO_SNIPPET })

		expect(selectedLanguage(wrapper)).toBe("Go")
	})

	it("falls back to plaintext for an empty block", async ({ expect }) => {
		const wrapper = await mountButtons({ textContent: "   \n  " })

		expect(selectedLanguage(wrapper)).toBe("Plaintext")
	})

	it("falls back to plaintext when nothing recognises the code", async ({
		expect,
	}) => {
		const wrapper = await mountButtons({ textContent: "hello" })

		expect(selectedLanguage(wrapper)).toBe("Plaintext")
	})

	it("falls back to plaintext when the best guess is a weak one", async ({
		expect,
	}) => {
		const wrapper = await mountButtons({ textContent: "x" })

		expect(selectedLanguage(wrapper)).toBe("Plaintext")
	})

	it("stores a language the user picks", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountButtons({ updateAttributes: updateAttributes })

		emitFrom(wrapper, Select, "update:modelValue", "rust")

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({ language: "rust" })
	})

	it("stores nothing when the picked language is already set", async ({
		expect,
	}) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountButtons({
			attrs: { language: "rust" },
			updateAttributes: updateAttributes,
		})

		emitFrom(wrapper, Select, "update:modelValue", "rust")

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("re-detects the language when the editor reports an update", async ({
		expect,
	}) => {
		const editor = makeEditor()
		const wrapper = await mountButtons({
			textContent: "hello",
			editor: editor,
		})
		await wrapper.setProps({
			node: makeNode(
				{ uid: "code-1", language: null },
				{ textContent: GO_SNIPPET },
			),
		})

		editor.on.mock.calls.forEach(([, handler]) => {
			;(handler as () => void)()
		})
		await nextTick()

		expect(editor.on).toHaveBeenCalledTimes(1)
		expect(selectedLanguage(wrapper)).toBe("Go")
	})

	it("keeps a stored language when the editor reports an update", async ({
		expect,
	}) => {
		const editor = makeEditor()
		const wrapper = await mountButtons({
			attrs: { language: "python" },
			textContent: GO_SNIPPET,
			editor: editor,
		})

		editor.on.mock.calls.forEach(([, handler]) => {
			;(handler as () => void)()
		})
		await nextTick()

		expect(selectedLanguage(wrapper)).toBe("Python")
	})

	it("stops listening for editor updates once unmounted", async ({
		expect,
	}) => {
		const editor = makeEditor()
		const wrapper = await mountButtons({ editor: editor })

		wrapper.unmount()

		expect(editor.off).toHaveBeenCalledTimes(1)
		expect(editor.off.mock.calls[0]?.[0]).toBe("update")
		expect(editor.off.mock.calls[0]?.[1]).toBe(editor.on.mock.calls[0]?.[1])
	})

	it("copies the block's code to the clipboard", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)
		const wrapper = await mountButtons({ textContent: GO_SNIPPET })

		await copyButton(wrapper).trigger("click")
		await flushPromises()

		expect(writeText).toHaveBeenCalledTimes(1)
		expect(writeText).toHaveBeenCalledWith(GO_SNIPPET)
		expect(copyIconMarked(wrapper)).toBe(true)
	})

	it("copies an empty string for a block with no code", async ({ expect }) => {
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue(undefined)
		const wrapper = await mountButtons()

		await copyButton(wrapper).trigger("click")
		await flushPromises()

		expect(writeText).toHaveBeenCalledWith("")
	})

	it("drops the copied marker shortly after copying", async ({ expect }) => {
		vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined)
		const wrapper = await mountButtons({ textContent: GO_SNIPPET })
		vi.useFakeTimers()

		await copyButton(wrapper).trigger("click")
		await flushPromises()
		await vi.advanceTimersByTimeAsync(COPIED_RESET_MS)

		expect(copyIconMarked(wrapper)).toBe(false)
	})

	it("marks nothing as copied when the clipboard rejects", async ({
		expect,
	}) => {
		const failure = new Error("denied")
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockRejectedValue(failure)
		const errors: unknown[] = []
		const wrapper = await mountButtons({
			textContent: GO_SNIPPET,
			onError: (error) => errors.push(error),
		})

		await copyButton(wrapper).trigger("click")
		await flushPromises()

		expect(writeText).toHaveBeenCalledTimes(1)
		expect(copyIconMarked(wrapper)).toBe(false)
		expect(errors).toEqual([failure])
	})

	it("hides itself for a block carrying a diff status", async ({ expect }) => {
		const wrapper = await mountButtons({ attrs: { diffStatus: "added" } })

		expect((wrapper.element as HTMLElement).style.display).toBe("none")
	})

	it("lets the language be changed while editing", async ({ expect }) => {
		const wrapper = await mountButtons()

		expect(
			wrapper.get("[data-slot='select-trigger']").attributes("disable"),
		).toBe("false")
	})

	it("blocks the language select in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountButtons()

		expect(
			wrapper.get("[data-slot='select-trigger']").attributes("disable"),
		).toBe("true")
	})

	it("blocks the language select while the editor is not editable", async ({
		expect,
	}) => {
		const wrapper = await mountButtons({
			editor: makeEditor({ isEditable: false }),
		})

		expect(
			wrapper.get("[data-slot='select-trigger']").attributes("disable"),
		).toBe("true")
	})

	it("sizes a comment block's controls smaller", async ({ expect }) => {
		const wrapper = await mountButtons({ type: "comment" })

		expect(wrapper.get("[data-slot='select-trigger']").classes()).toContain(
			"text-2sm",
		)
	})
})
