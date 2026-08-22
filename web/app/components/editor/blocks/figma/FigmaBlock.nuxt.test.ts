import type { VueWrapper } from "@vue/test-utils"
import { beforeEach, describe, it, vi } from "vitest"
import FigmaBlock from "./FigmaBlock.vue"
import { makeNode, mountNodeView } from "../../test-helpers/node-view"
import { clearTeleportedOverlays, t } from "~/components/test-helpers"

const FIGMA_URL = "https://www.figma.com/file/abc123/Design"
const OTHER_FIGMA_URL = "https://www.figma.com/file/xyz789/Other"

const MIN_SIZE = 128
const DEFAULT_HEIGHT = 450

function mountFigma(
	attrs: Record<string, unknown> = {},
	updateAttributes: (attrs: Record<string, unknown>) => void = () => undefined,
) {
	return mountNodeView(FigmaBlock, {
		node: makeNode({ uid: "figma-1", ...attrs }),
		updateAttributes: updateAttributes,
	})
}

// the url popover is teleported into <body>, so it is out of the
// wrapper's reach
function popoverInput(): HTMLInputElement | null {
	return document.body.querySelector("input")
}

function typeUrl(value: string) {
	const input = popoverInput()
	if (!input) {
		throw new Error("the url popover is not open")
	}

	input.value = value
	input.dispatchEvent(new Event("input", { bubbles: true }))
}

function pressKey(key: string) {
	const input = popoverInput()
	if (!input) {
		throw new Error("the url popover is not open")
	}

	input.dispatchEvent(new KeyboardEvent("keydown", { key: key, bubbles: true }))
}

function dragBy(dx: number, dy: number) {
	window.dispatchEvent(
		new MouseEvent("mousemove", { clientX: 100 + dx, clientY: 200 + dy }),
	)
}

async function startDrag(wrapper: VueWrapper) {
	await wrapper
		.get("[aria-label='Resize']")
		.trigger("mousedown", { clientX: 100, clientY: 200 })
}

function embedSrc(wrapper: VueWrapper): string | undefined {
	return wrapper.get("iframe").attributes("src")
}

// the editable flag, the colour mode and the editor store are all shared
// app-wide, so these tests cannot interleave
describe("<FigmaBlock>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		useEditorMeta().setEditable(true)
		useEditorStore().setReviewableDiffActive(false)
		useAppearance().changeColorTheme("light")
	})

	it("identifies the wrapper by the node's uid", async ({ expect }) => {
		const wrapper = await mountFigma()

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("id")).toBe("figma-1")
		expect(root.attributes("data-uid")).toBe("figma-1")
	})

	it("exposes the node's comment id and diff status on the wrapper", async ({
		expect,
	}) => {
		const wrapper = await mountFigma({
			nodeCommentId: "comment-1",
			diffStatus: "added",
		})

		const root = wrapper.get("[data-node-view-wrapper]")

		expect(root.attributes("data-node-comment-id")).toBe("comment-1")
		expect(root.attributes("data-diff-status")).toBe("added")
	})

	it("invites the reader to embed a design while the block is empty", async ({
		expect,
	}) => {
		const wrapper = await mountFigma()

		expect(wrapper.text()).toBe(t("editor.figma.description"))
		expect(wrapper.find("iframe").exists()).toBe(false)
	})

	it("reports an empty block as empty in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFigma()

		expect(wrapper.text()).toBe(t("editor.figma.empty"))
	})

	it("embeds the design the node points at", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })

		expect(embedSrc(wrapper)).toBe(
			"https://embed.figma.com/design/abc123?embed-host=oxynote&theme=light",
		)
	})

	it("embeds the design in the reader's colour theme", async ({ expect }) => {
		useAppearance().changeColorTheme("dark")

		const wrapper = await mountFigma({ src: FIGMA_URL })

		expect(embedSrc(wrapper)).toContain("theme=dark")
	})

	it("stays empty for a src that is not a figma url", async ({ expect }) => {
		const wrapper = await mountFigma({ src: "https://example.com/thing" })

		expect(wrapper.find("iframe").exists()).toBe(false)
		expect(wrapper.text()).toBe(t("editor.figma.description"))
	})

	it.for([
		{ status: "added", expected: "diff-added" },
		{ status: "removed", expected: "diff-removed" },
		{ status: "modified", expected: "diff-modified" },
	])(
		"marks a $status embed with its diff overlay",
		async ({ status, expected }, { expect }) => {
			const wrapper = await mountFigma({ src: FIGMA_URL, diffStatus: status })

			expect(wrapper.get(".diff-overlay").classes()).toContain(expected)
		},
	)

	it("shows no diff overlay on an unchanged embed", async ({ expect }) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			diffStatus: "unchanged",
		})

		expect(wrapper.find(".diff-overlay").exists()).toBe(false)
	})

	it("gives the embed the height the node stores", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL, height: 320 })

		expect(wrapper.get("iframe").attributes("style")).toContain("height: 320px")
	})

	it("falls back to a default height when the node stores none", async ({
		expect,
	}) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })

		expect(wrapper.get("iframe").attributes("style")).toContain(
			`height: ${DEFAULT_HEIGHT}px`,
		)
	})

	it("reads a stored size written as a string", async ({ expect }) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			width: "300",
			height: "200",
		})

		expect(wrapper.get("iframe").attributes("style")).toContain("height: 200px")
		expect(wrapper.get(".group").attributes("style")).toContain("width: 300px")
	})

	it("ignores a stored size that is not a number", async ({ expect }) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			width: "wide",
			height: {},
		})

		expect(wrapper.get(".group").attributes("style")).toBeUndefined()
		expect(wrapper.get("iframe").attributes("style")).toContain(
			`height: ${DEFAULT_HEIGHT}px`,
		)
	})

	it("offers the edit and resize controls while editing", async ({
		expect,
	}) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })

		expect(wrapper.find("[aria-label='Edit URL']").exists()).toBe(true)
		expect(wrapper.find("[aria-label='Resize']").exists()).toBe(true)
	})

	it("hides the edit and resize controls in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)

		const wrapper = await mountFigma({ src: FIGMA_URL })

		expect(wrapper.find("[aria-label='Edit URL']").exists()).toBe(false)
		expect(wrapper.find("[aria-label='Resize']").exists()).toBe(false)
	})

	it("opens the url popover from the empty state", async ({ expect }) => {
		const wrapper = await mountFigma()

		await wrapper.get("[data-node-view-wrapper] > div").trigger("click")

		expect(popoverInput()).not.toBeNull()
	})

	it("prefills the popover with the current url", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })

		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		expect(popoverInput()?.value).toBe(FIGMA_URL)
	})

	it("keeps the popover closed in read mode", async ({ expect }) => {
		useEditorMeta().setEditable(false)
		const wrapper = await mountFigma()

		await wrapper.get("[data-node-view-wrapper] > div").trigger("click")

		expect(popoverInput()).toBeNull()
	})

	it("stores a figma url entered in the popover", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma({ src: FIGMA_URL }, updateAttributes)
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		typeUrl(`  ${OTHER_FIGMA_URL}  `)
		pressKey("Enter")
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({ src: OTHER_FIGMA_URL })
		expect(popoverInput()).toBeNull()
	})

	it("keeps the popover open for a url that is not a figma one", async ({
		expect,
	}) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma({ src: FIGMA_URL }, updateAttributes)
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		typeUrl("https://example.com/thing")
		pressKey("Enter")
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(0)
		expect(popoverInput()).not.toBeNull()
	})

	it("keeps the popover open for an empty url", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma({ src: FIGMA_URL }, updateAttributes)
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		typeUrl("   ")
		pressKey("Enter")
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(0)
		expect(popoverInput()).not.toBeNull()
	})

	it("abandons the popover on escape", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma({ src: FIGMA_URL }, updateAttributes)
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		typeUrl(OTHER_FIGMA_URL)
		pressKey("Escape")
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(0)
		expect(popoverInput()).toBeNull()
	})

	it("keeps the popover open on any other key", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		pressKey("a")
		await nextTick()

		expect(popoverInput()).not.toBeNull()
	})

	it("closes the popover from its close button", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })
		await wrapper.get("[aria-label='Edit URL']").trigger("click")

		document.body.querySelectorAll("button")[0]?.click()
		await nextTick()

		expect(popoverInput()).toBeNull()
	})

	it("closes the popover on a click outside it", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })
		await wrapper.get("[aria-label='Edit URL']").trigger("click")
		await nextTick()

		document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }))
		await nextTick()

		expect(popoverInput()).toBeNull()
	})

	it("keeps the popover open while clicking inside it", async ({ expect }) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })
		await wrapper.get("[aria-label='Edit URL']").trigger("click")
		await nextTick()

		popoverInput()?.dispatchEvent(
			new MouseEvent("mousedown", { bubbles: true }),
		)
		await nextTick()

		expect(popoverInput()).not.toBeNull()
	})

	it("resizes the embed while the handle is dragged", async ({ expect }) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			width: 300,
			height: 200,
		})

		await startDrag(wrapper)
		dragBy(50, 30)
		await nextTick()

		expect(wrapper.get(".group").attributes("style")).toContain("width: 350px")
		expect(wrapper.get("iframe").attributes("style")).toContain("height: 230px")
	})

	it("stores the new size once the drag ends", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma(
			{ src: FIGMA_URL, width: 300, height: 200 },
			updateAttributes,
		)

		await startDrag(wrapper)
		dragBy(50, 30)
		window.dispatchEvent(new MouseEvent("mouseup"))
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(1)
		expect(updateAttributes).toHaveBeenCalledWith({ width: 350, height: 230 })
	})

	it("stores nothing when the handle is released without a drag", async ({
		expect,
	}) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma(
			{ src: FIGMA_URL, width: 300, height: 200 },
			updateAttributes,
		)

		await startDrag(wrapper)
		window.dispatchEvent(new MouseEvent("mouseup"))
		await nextTick()

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})

	it("keeps the embed at its minimum size when dragged past it", async ({
		expect,
	}) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			width: 300,
			height: 200,
		})

		await startDrag(wrapper)
		dragBy(-500, -500)
		await nextTick()

		expect(wrapper.get(".group").attributes("style")).toContain(
			`width: ${MIN_SIZE}px`,
		)
		expect(wrapper.get("iframe").attributes("style")).toContain(
			`height: ${MIN_SIZE}px`,
		)
	})

	it("keeps the embed no taller than it is wide", async ({ expect }) => {
		const wrapper = await mountFigma({
			src: FIGMA_URL,
			width: 300,
			height: 200,
		})

		await startDrag(wrapper)
		dragBy(0, 400)
		await nextTick()

		expect(wrapper.get("iframe").attributes("style")).toContain("height: 300px")
	})

	it("starts a drag from the element's own width when the node stores none", async ({
		expect,
	}) => {
		const wrapper = await mountFigma({ src: FIGMA_URL })

		await startDrag(wrapper)
		dragBy(200, 0)
		await nextTick()

		expect(wrapper.get(".group").attributes("style")).toContain("width: 200px")
	})

	it("stops resizing once the block is unmounted", async ({ expect }) => {
		const updateAttributes = vi.fn()
		const wrapper = await mountFigma(
			{ src: FIGMA_URL, width: 300, height: 200 },
			updateAttributes,
		)
		await startDrag(wrapper)

		wrapper.unmount()
		dragBy(50, 30)
		window.dispatchEvent(new MouseEvent("mouseup"))

		expect(updateAttributes).toHaveBeenCalledTimes(0)
	})
})
