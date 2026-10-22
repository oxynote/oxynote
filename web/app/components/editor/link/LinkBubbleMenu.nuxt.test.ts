import { mountSuspended } from "@nuxt/test-utils/runtime"
import { Node } from "@tiptap/core"
import type { Editor } from "@tiptap/vue-3"
import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import LinkBubbleMenu from "./LinkBubbleMenu.vue"
import { DiffLinkChangedMark } from "../diff/diff-text-marks"
import { DIFF_LINK_CHANGED_MARK_NAME } from "../mark-names"
import { destroyLinkEditor, linkElement, makeLinkEditor } from "./test-helpers"
import { at, t, WAIT_FOR_OPTIONS } from "~/components/test-helpers"

const HOVER_DELAY_MS = 200
const CLOSE_DELAY_MS = 500

const DOCUMENT = '<p>see <a href="https://oxynote.test">docs</a> here</p>'

// an anchor no link mark renders, like a file block's card
const AnchorBlock = Node.create({
	name: "anchorBlock",
	group: "block",
	atom: true,

	renderHTML() {
		return ["a", { "data-anchor-block": "", href: "https://file.test" }]
	},
})

let editor: Editor | null = null

function makeEditorFor(content = DOCUMENT): Editor {
	editor = makeLinkEditor(content, [DiffLinkChangedMark, AnchorBlock])

	return editor
}

function mountMenu(target: Editor, props: { readOnly?: boolean } = {}) {
	return mountSuspended(LinkBubbleMenu, {
		props: { editor: target, container: null, ...props },
	})
}

function popover(): HTMLElement | null {
	return document.body.querySelector(".z-popover")
}

function popoverInputs(): HTMLInputElement[] {
	return Array.from(popover()?.querySelectorAll("input") ?? [])
}

function popoverButton(title: string): HTMLButtonElement {
	const button = Array.from(
		popover()?.querySelectorAll<HTMLButtonElement>("button") ?? [],
	).find(
		(candidate) =>
			candidate.getAttribute("title") === title ||
			candidate.textContent.includes(title),
	)
	if (!button) {
		throw new Error(`no popover button for "${title}"`)
	}

	return button
}

async function hoverLink(target: Editor) {
	linkElement(target).dispatchEvent(
		new MouseEvent("mouseenter", { bubbles: true }),
	)
	await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)
	await nextTick()
}

async function leaveLink(target: Editor) {
	linkElement(target).dispatchEvent(
		new MouseEvent("mouseleave", { bubbles: true }),
	)
	await vi.advanceTimersByTimeAsync(CLOSE_DELAY_MS)
	await nextTick()
}

// calls the editor's click handlers directly. happy-dom has no layout, so
// a real click never reaches them.
function clickOn(
	target: Editor,
	element: Element,
	init: MouseEventInit = {},
): boolean {
	const event = new MouseEvent("click", { button: 0, ...init })
	Object.defineProperty(event, "target", { value: element })

	return !!target.view.someProp("handleClick", (handle) =>
		handle(target.view, 0, event),
	)
}

async function typeIn(input: HTMLInputElement, value: string) {
	input.value = value
	input.dispatchEvent(new Event("input", { bubbles: true }))
	await nextTick()
	await nextTick()
}

// the popover is teleported into a shared <body>, the hover delays need
// the global fake timers, and the editable flag is a shared cookie state,
// so these tests cannot interleave
describe("<LinkBubbleMenu>", { concurrent: false }, () => {
	// the popover lives in <body> for as long as the menu is mounted, so
	// each test's menu comes down before the next one goes up
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		vi.useFakeTimers()
		useEditorMeta().setEditable(true)
	})

	afterEach(() => {
		vi.useRealTimers()

		if (editor) {
			destroyLinkEditor(editor)
			editor = null
		}
	})

	it("stays out of the way until a link is hovered", async ({ expect }) => {
		const target = makeEditorFor()

		await mountMenu(target)

		expect(popover()).toBeNull()
	})

	it("shows the address of the hovered link", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)

		await hoverLink(target)

		expect(popover()?.textContent).toContain("https://oxynote.test")
	})

	it("shows the previous address of a link the diff marks as changed", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		target
			.chain()
			.setTextSelection({ from: 5, to: 9 })
			.setMark(DIFF_LINK_CHANGED_MARK_NAME, { oldHref: "https://old.test" })
			.run()
		await mountMenu(target)

		await hoverLink(target)

		expect(popover()?.textContent).toContain(t("editor.link.previous"))
		expect(popover()?.textContent).toContain("https://old.test")
		expect(popover()?.textContent).toContain("https://oxynote.test")
	})

	it("waits before showing the popover", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)

		linkElement(target).dispatchEvent(
			new MouseEvent("mouseenter", { bubbles: true }),
		)
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS - 1)

		expect(popover()).toBeNull()
	})

	it("hides the popover once the pointer leaves the link", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)

		await leaveLink(target)

		expect(popover()).toBeNull()
	})

	it("keeps the popover open while the pointer is on it", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)

		popover()?.dispatchEvent(new MouseEvent("mouseenter", { bubbles: true }))
		await leaveLink(target)

		expect(popover()).not.toBeNull()
	})

	it("hides the popover when the pointer leaves it again", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popover()?.dispatchEvent(new MouseEvent("mouseenter", { bubbles: true }))

		popover()?.dispatchEvent(new MouseEvent("mouseleave", { bubbles: true }))
		await nextTick()

		expect(popover()).toBeNull()
	})

	it("keeps the popover open when the pointer moves onto a touching link", async ({
		expect,
	}) => {
		const target = makeEditorFor(
			'<p><a href="https://old.test">old</a><a href="https://new.test">new</a></p>',
		)
		await mountMenu(target)
		await hoverLink(target)
		const anchors = Array.from(target.view.dom.querySelectorAll("a"))

		at(anchors, 0).dispatchEvent(
			new MouseEvent("mouseleave", { bubbles: true }),
		)
		at(anchors, 1).dispatchEvent(
			new MouseEvent("mouseenter", { bubbles: true }),
		)
		await vi.advanceTimersByTimeAsync(CLOSE_DELAY_MS)
		await nextTick()

		expect(popover()?.textContent).toContain("https://new.test")
	})

	it("ignores the pointer moving over anything but a link", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)

		target.view.dom.dispatchEvent(
			new MouseEvent("mouseenter", { bubbles: true }),
		)
		await vi.advanceTimersByTimeAsync(HOVER_DELAY_MS)

		expect(popover()).toBeNull()
	})

	it("opens the link in a new tab", async ({ expect }) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)

		popoverButton(t("editor.link.open")).click()
		await nextTick()

		expect(open).toHaveBeenCalledTimes(1)
		expect(open).toHaveBeenCalledWith(
			"https://oxynote.test",
			"_blank",
			"noopener",
		)
	})

	it("opens a clicked link in a new tab", async ({ expect }) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		await mountMenu(target)

		const handled = clickOn(target, linkElement(target))

		expect(handled).toBe(true)
		expect(open).toHaveBeenCalledTimes(1)
		expect(open).toHaveBeenCalledWith(
			"https://oxynote.test",
			"_blank",
			"noopener",
		)
	})

	it.for([
		{
			name: "leaves a shift-click to the selection",
			input: { shiftKey: true },
		},
		{ name: "ignores a click with another button", input: { button: 2 } },
	])("$name", async ({ input }, { expect }) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		await mountMenu(target)

		const handled = clickOn(target, linkElement(target), input)

		expect(handled).toBe(false)
		expect(open).not.toHaveBeenCalled()
	})

	it("leaves a click in a read-only editor to the browser", async ({
		expect,
	}) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		target.setEditable(false)
		await mountMenu(target)

		const handled = clickOn(target, linkElement(target))

		expect(handled).toBe(false)
		expect(open).not.toHaveBeenCalled()
	})

	it("leaves a click on an anchor no link renders to its owner", async ({
		expect,
	}) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		target.commands.insertContentAt(target.state.doc.content.size, {
			type: AnchorBlock.name,
		})
		await mountMenu(target)
		const anchor = target.view.dom.querySelector("a[data-anchor-block]")
		if (!anchor) {
			throw new Error("the test document holds no anchor block")
		}

		const handled = clickOn(target, anchor)

		expect(handled).toBe(false)
		expect(open).not.toHaveBeenCalled()
	})

	it("ignores a click outside any link", async ({ expect }) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		await mountMenu(target)

		const handled = clickOn(target, target.view.dom)

		expect(handled).toBe(false)
		expect(open).not.toHaveBeenCalled()
	})

	it("offers no edit action in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const target = makeEditorFor()
		await mountMenu(target)

		await hoverLink(target)

		expect(popover()?.querySelectorAll("button")).toHaveLength(1)
	})

	it("offers no edit action when read only", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target, { readOnly: true })

		await hoverLink(target)

		expect(popover()?.querySelectorAll("button")).toHaveLength(1)
	})

	it("shows the link's address and text for editing", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)

		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		expect(popoverInputs().map((input) => input.value)).toEqual([
			"https://oxynote.test",
			"docs",
		])
	})

	it("stores a new address as the reader types it", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 0), "https://other.test")

		expect(target.getHTML()).toContain('href="https://other.test"')
	})

	it("leaves an address without a scheme alone while it is typed", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 0), "other.test")

		expect(target.getHTML()).toContain('href="other.test"')
		expect(at(popoverInputs(), 0).value).toBe("other.test")
	})

	it.for([
		{ input: "other.test", expected: "https://other.test" },
		{ input: "http://other.test", expected: "http://other.test" },
		{ input: "mailto:team@oxynote.test", expected: "mailto:team@oxynote.test" },
	])(
		"stores $input as $expected once the address field is left",
		async ({ input, expected }, { expect }) => {
			const target = makeEditorFor()
			await mountMenu(target)
			await hoverLink(target)
			popoverButton(t("editor.link.edit")).click()
			await nextTick()
			await typeIn(at(popoverInputs(), 0), input)

			at(popoverInputs(), 0).dispatchEvent(new FocusEvent("blur"))
			await nextTick()
			await nextTick()

			expect(target.getHTML()).toContain(`href="${expected}"`)
			expect(at(popoverInputs(), 0).value).toBe(expected)
		},
	)

	it("adds https to an address without a scheme when the popover closes", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()
		await typeIn(at(popoverInputs(), 0), "other.test")

		window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		expect(target.getHTML()).toContain('href="https://other.test"')
		expect(popover()).toBeNull()
	})

	it("stores new link text as the reader types it", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 1), "documentation")

		expect(target.getText()).toContain("documentation")
	})

	it("keeps editing when the link text shrinks to one character", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 1), "x")

		expect(target.getText()).toBe("see x here")
		expect(popoverInputs().map((input) => input.value)).toEqual([
			"https://oxynote.test",
			"x",
		])
	})

	it("keeps the link and the popover while its text is empty", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 1), "")

		expect(target.getHTML()).toContain('href="https://oxynote.test"')
		expect(target.getText()).toBe("see docs here")
		expect(popoverInputs().map((input) => input.value)).toEqual([
			"https://oxynote.test",
			"",
		])
	})

	it("keeps the link and the popover while its address is empty", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 0), "")

		expect(target.getHTML()).toContain('href="https://oxynote.test"')
		expect(popoverInputs().map((input) => input.value)).toEqual(["", "docs"])
	})

	it("takes new text typed after the text was emptied", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()
		await typeIn(at(popoverInputs(), 1), "")

		await typeIn(at(popoverInputs(), 1), "guide")

		expect(target.getText()).toBe("see guide here")
		expect(target.getHTML()).toContain("guide</a>")
	})

	it.for([
		{
			name: "removes the link when escape closes the popover with empty text",
			input: () => {
				window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
			},
		},
		{
			name: "removes the link when a click outside closes the popover with empty text",
			input: () => {
				document.body.dispatchEvent(
					new MouseEvent("mousedown", { bubbles: true }),
				)
			},
		},
	])("$name", async ({ input }, { expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()
		await typeIn(at(popoverInputs(), 1), "")

		input()
		await nextTick()

		expect(target.getHTML()).not.toContain("<a")
		expect(target.getText()).toBe("see docs here")
		expect(popover()).toBeNull()
	})

	it("removes the link when the popover closes with an empty address", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()
		await typeIn(at(popoverInputs(), 0), "")

		window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		expect(target.getHTML()).not.toContain("<a")
		expect(target.getText()).toBe("see docs here")
		expect(popover()).toBeNull()
	})

	it("leaves the focus where a click outside put it", async ({
		expect,
		onTestFinished,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()
		await typeIn(at(popoverInputs(), 1), "")
		const outside = document.createElement("input")
		document.body.appendChild(outside)
		onTestFinished(() => {
			outside.remove()
		})
		outside.focus()

		outside.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await vi.advanceTimersByTimeAsync(CLOSE_DELAY_MS)

		expect(target.getHTML()).not.toContain("<a")
		expect(document.activeElement).toBe(outside)
	})

	it("removes the link on request", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		popoverButton(t("editor.link.delete")).click()
		await nextTick()

		expect(target.getHTML()).not.toContain("<a")
		expect(popover()).toBeNull()
	})

	it("abandons editing on escape", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }))
		await nextTick()

		expect(popover()).toBeNull()
	})

	it.for([
		{ name: "closes the popover on enter in the address field", input: 0 },
		{ name: "closes the popover on enter in the text field", input: 1 },
	])("$name", async ({ input }, { expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		at(popoverInputs(), input).dispatchEvent(
			new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
		)
		await nextTick()

		expect(popover()).toBeNull()
		expect(target.getHTML()).toContain('href="https://oxynote.test"')
	})

	it("abandons editing on a click outside the popover", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popover()).toBeNull()
	})

	it("keeps editing while clicking inside the popover", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		popover()?.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popover()).not.toBeNull()
	})

	it("abandons editing when the link is gone from the document", async ({
		expect,
	}) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		target.commands.setContent("<p>nothing here</p>")
		await vi.waitFor(() => {
			expect(popover()).toBeNull()
		}, WAIT_FOR_OPTIONS)
	})

	it("stops opening clicked links once it is gone", async ({ expect }) => {
		const open = vi.fn()
		vi.stubGlobal("open", open)
		const target = makeEditorFor()
		const wrapper = await mountMenu(target)

		wrapper.unmount()
		const handled = clickOn(target, linkElement(target))

		expect(handled).toBe(false)
		expect(open).not.toHaveBeenCalled()
	})

	it("stops listening to the editor once it is gone", async ({ expect }) => {
		const target = makeEditorFor()
		const wrapper = await mountMenu(target)

		wrapper.unmount()
		await hoverLink(target)

		expect(popover()).toBeNull()
	})
})
