import { mountSuspended } from "@nuxt/test-utils/runtime"
import { Editor } from "@tiptap/vue-3"
import { BubbleMenu } from "@tiptap/vue-3/menus"
import Blockquote from "@tiptap/extension-blockquote"
import Bold from "@tiptap/extension-bold"
import Code from "@tiptap/extension-code"
import Document from "@tiptap/extension-document"
import Heading from "@tiptap/extension-heading"
import Italic from "@tiptap/extension-italic"
import Link from "@tiptap/extension-link"
import Paragraph from "@tiptap/extension-paragraph"
import Strike from "@tiptap/extension-strike"
import Text from "@tiptap/extension-text"
import Underline from "@tiptap/extension-underline"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import TextBubbleMenu from "./TextBubbleMenu.vue"
import { CommentMark } from "./comments/comment-mark"
import LinkCreateMenu from "./link/LinkCreateMenu.vue"
import { clearQueryCache } from "~/composables/api/test-helpers"
import {
	at,
	renderedIconNames,
	seedAuthOrganization,
	seedAuthSession,
	t,
} from "~/components/test-helpers"

let editor: Editor | null = null

function makeMenuEditor(content: string): Editor {
	const element = document.createElement("div")
	document.body.appendChild(element)

	editor = new Editor({
		element: element,
		extensions: [
			Document,
			Paragraph,
			Text,
			Heading.configure({ levels: [1, 2, 3] }),
			Bold,
			Italic,
			Underline,
			Strike,
			Code,
			Blockquote,
			Link.configure({ openOnClick: false }),
			CommentMark,
		],
		content: content,
	})

	return editor
}

function mountMenu(
	target: Editor,
	props: Record<string, unknown> = {},
): Promise<VueWrapper> {
	return mountSuspended(TextBubbleMenu, {
		props: { editor: target, container: null, ...props },
	})
}

function selectAll(target: Editor) {
	target.commands.setTextSelection({
		from: 1,
		to: target.state.doc.content.size - 1,
	})
}

function buttonIcons(wrapper: VueWrapper): string[] {
	return renderedIconNames(wrapper)
}

// the menu is rendered into the editor's own dom, so each test takes its
// menu and editor down again
describe("<TextBubbleMenu>", { concurrent: false }, () => {
	enableAutoUnmount(afterEach)

	afterEach(() => {
		if (editor) {
			const element = editor.view.dom.parentElement

			editor.destroy()
			element?.remove()
			editor = null
		}
	})

	it("offers the formatting actions for ordinary text", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)

		const wrapper = await mountMenu(target)

		expect(buttonIcons(wrapper)).toEqual([
			"lucide:bold",
			"lucide:italic",
			"lucide:underline",
			"lucide:strikethrough",
			"lucide:link",
			"lucide:square-code",
			"lucide:message-square-text",
		])
	})

	it("offers only a comment inside a heading", async ({ expect }) => {
		const target = makeMenuEditor("<h1>hello</h1>")
		selectAll(target)

		const wrapper = await mountMenu(target)

		expect(buttonIcons(wrapper)).toEqual(["lucide:message-square-text"])
	})

	it("offers no second comment inside an existing one", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello</p>")
		selectAll(target)
		target.commands.setMark(CommentMark.name, { commentId: "c-1" })
		target.commands.setTextSelection({ from: 2, to: 4 })

		const wrapper = await mountMenu(target)

		expect(buttonIcons(wrapper)).not.toContain("lucide:message-square-text")
		expect(buttonIcons(wrapper)).toContain("lucide:bold")
	})

	it("offers nothing inside a commented heading", async ({ expect }) => {
		const target = makeMenuEditor("<h1>hello</h1>")
		selectAll(target)
		target.commands.setMark(CommentMark.name, { commentId: "c-1" })
		target.commands.setTextSelection({ from: 2, to: 4 })

		const wrapper = await mountMenu(target)

		expect(buttonIcons(wrapper)).toEqual([])
	})

	it.for([
		{ icon: "lucide:bold", mark: "bold" },
		{ icon: "lucide:italic", mark: "italic" },
		{ icon: "lucide:underline", mark: "underline" },
		{ icon: "lucide:strikethrough", mark: "strike" },
		{ icon: "lucide:square-code", mark: "code" },
	])("applies $mark to the selection", async ({ icon, mark }, { expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)
		const wrapper = await mountMenu(target)

		await at(
			wrapper.findAll("button"),
			buttonIcons(wrapper).indexOf(icon),
		).trigger("click")

		expect(target.isActive(mark)).toBe(true)
	})

	it("marks the actions already applied to the selection", async ({
		expect,
	}) => {
		const target = makeMenuEditor("<p><strong>hello</strong></p>")
		selectAll(target)

		const wrapper = await mountMenu(target)

		expect(at(wrapper.findAll("button"), 0).attributes("data-status")).toBe(
			"active",
		)
	})

	it("asks for an address when the selection has no link", async ({
		expect,
	}) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)
		const wrapper = await mountMenu(target)

		await at(
			wrapper.findAll("button"),
			buttonIcons(wrapper).indexOf("lucide:link"),
		).trigger("click")

		expect(wrapper.findComponent(LinkCreateMenu).exists()).toBe(true)
	})

	it("hands an existing link to the host to edit", async ({ expect }) => {
		const target = makeMenuEditor(
			'<p><a href="https://oxynote.test">docs</a></p>',
		)
		selectAll(target)
		const wrapper = await mountMenu(target)

		await at(
			wrapper.findAll("button"),
			buttonIcons(wrapper).indexOf("lucide:link"),
		).trigger("click")

		expect(wrapper.emitted("edit-link")).toHaveLength(1)
		expect(wrapper.findComponent(LinkCreateMenu).exists()).toBe(false)
	})

	it("puts the formatting actions back once a link is made", async ({
		expect,
	}) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)
		const wrapper = await mountMenu(target)
		await at(
			wrapper.findAll("button"),
			buttonIcons(wrapper).indexOf("lucide:link"),
		).trigger("click")

		await wrapper.get("input").trigger("keydown", { key: "Escape" })

		expect(wrapper.findComponent(LinkCreateMenu).exists()).toBe(false)
		expect(buttonIcons(wrapper)).toContain("lucide:bold")
	})

	it("asks the host for a comment thread", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)
		const wrapper = await mountMenu(target)

		await at(
			wrapper.findAll("button"),
			buttonIcons(wrapper).indexOf("lucide:message-square-text"),
		).trigger("click")

		expect(wrapper.emitted("add-thread")).toHaveLength(1)
	})

	it("offers only a comment while a diff is shown", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)

		const wrapper = await mountMenu(target, {
			diffContext: { positionMap: [] },
		})

		expect(buttonIcons(wrapper)).toEqual(["lucide:message-square-text"])
		expect(wrapper.text()).toContain(t("editor.bubble-menu.comment-label"))
	})

	it("asks the host for a comment thread while a diff is shown", async ({
		expect,
	}) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)
		const wrapper = await mountMenu(target, {
			diffContext: { positionMap: [] },
		})

		await wrapper.get("button").trigger("click")

		expect(wrapper.emitted("add-thread")).toHaveLength(1)
	})

	it("shows the diff menu over a focused selection", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)

		const wrapper = await mountMenu(target, {
			diffContext: { positionMap: [] },
		})

		expect(shouldShow(wrapper, target)).toBe(true)
	})

	// the role is read from the auth cache, which every mount shares
	describe("for a viewer", { concurrent: false }, () => {
		beforeEach(() => {
			seedAuthSession({ id: "u1" })
			seedAuthOrganization({ members: [{ userId: "u1", role: "viewer" }] })
		})

		afterEach(clearQueryCache)

		it("shows no diff menu, since its only action is a comment", async ({
			expect,
		}) => {
			const target = makeMenuEditor("<p>hello world</p>")
			selectAll(target)

			const wrapper = await mountMenu(target, {
				diffContext: { positionMap: [] },
			})

			expect(shouldShow(wrapper, target)).toBe(false)
			expect(wrapper.findAll("button")).toHaveLength(0)
		})

		it("keeps the formatting menu outside a diff", async ({ expect }) => {
			const target = makeMenuEditor("<p>hello world</p>")
			selectAll(target)

			const wrapper = await mountMenu(target)

			expect(shouldShow(wrapper, target)).toBe(true)
		})
	})

	describe("when the role changes", { concurrent: false }, () => {
		afterEach(clearQueryCache)

		it("removes an open diff menu once the role turns to viewer", async ({
			expect,
		}) => {
			seedAuthSession({ id: "u1" })
			seedAuthOrganization({ members: [{ userId: "u1", role: "editor" }] })
			const target = makeMenuEditor("<p>hello world</p>")
			selectAll(target)
			const wrapper = await mountMenu(target, {
				diffContext: { positionMap: [] },
			})
			expect(wrapper.findAll("button")).toHaveLength(1)

			seedAuthOrganization({ members: [{ userId: "u1", role: "viewer" }] })
			await nextTick()

			expect(wrapper.findAll("button")).toHaveLength(0)
		})
	})

	it("stays invisible until the menu is positioned", async ({ expect }) => {
		const target = makeMenuEditor("<p>hello world</p>")
		selectAll(target)

		const wrapper = await mountMenu(target)

		expect(
			wrapper.get("[data-visible], .opacity-0").attributes("data-visible"),
		).toBeUndefined()
	})
})

// asks the bubble menu whether it would show over the current selection.
// The test editor never holds the focus, so the view reports that it does.
function shouldShow(wrapper: VueWrapper, target: Editor): boolean {
	const check = wrapper.findComponent(BubbleMenu).props("shouldShow") as (
		opts: Record<string, unknown>,
	) => boolean

	return check({
		editor: target,
		state: target.state,
		view: { hasFocus: () => true },
		from: target.state.selection.from,
		to: target.state.selection.to,
	})
}
