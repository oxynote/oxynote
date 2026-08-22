import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { Editor } from "@tiptap/vue-3"
import { enableAutoUnmount } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import LinkBubbleMenu from "./LinkBubbleMenu.vue"
import { destroyLinkEditor, linkElement, makeLinkEditor } from "./test-helpers"
import { at, t, WAIT_FOR_OPTIONS } from "~/components/test-helpers"

const HOVER_DELAY_MS = 200
const CLOSE_DELAY_MS = 500

const DOCUMENT = '<p>see <a href="https://oxynote.test">docs</a> here</p>'

let editor: Editor | null = null

function makeEditorFor(content = DOCUMENT): Editor {
	editor = makeLinkEditor(content)

	return editor
}

function mountMenu(target: Editor) {
	return mountSuspended(LinkBubbleMenu, {
		props: { editor: target, container: null },
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

	it("offers no edit action in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const target = makeEditorFor()
		await mountMenu(target)

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

	it("gives a scheme-less address one", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 0), "other.test")

		expect(target.getHTML()).toContain('href="https://other.test"')
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

	it("removes the link when its text is emptied", async ({ expect }) => {
		const target = makeEditorFor()
		await mountMenu(target)
		await hoverLink(target)
		popoverButton(t("editor.link.edit")).click()
		await nextTick()

		await typeIn(at(popoverInputs(), 1), "")

		expect(target.getHTML()).not.toContain("<a")
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

	it("stops listening to the editor once it is gone", async ({ expect }) => {
		const target = makeEditorFor()
		const wrapper = await mountMenu(target)

		wrapper.unmount()
		await hoverLink(target)

		expect(popover()).toBeNull()
	})
})
