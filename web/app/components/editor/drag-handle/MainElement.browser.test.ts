import { Editor } from "@tiptap/core"
import Document from "@tiptap/extension-document"
import Paragraph from "@tiptap/extension-paragraph"
import Text from "@tiptap/extension-text"
import { PluginKey } from "@tiptap/pm/state"
import { mount, type VueWrapper } from "@vue/test-utils"
import { afterEach, describe, it, vi } from "vitest"
import { DragHandle } from "./MainElement"

const cleanups: (() => void)[] = []

function makeEditor(): Editor {
	const container = document.createElement("div")
	document.body.appendChild(container)

	const editor = new Editor({
		element: container,
		injectCSS: false,
		extensions: [Document, Paragraph, Text],
		content: "<p>hello</p>",
	})

	cleanups.push(() => {
		editor.destroy()
		container.remove()
	})

	return editor
}

type HandleProps = InstanceType<typeof DragHandle>["$props"]

function mountHandle(
	props: HandleProps,
	slot = "<span>drag</span>",
): VueWrapper {
	const wrapper = mount(DragHandle, {
		props,
		slots: { default: slot },
		attachTo: document.body,
	})

	cleanups.push(() => {
		wrapper.unmount()
	})

	return wrapper
}

// every case drives a real editor attached to the shared page, and the
// plugin appends its own overlay to the document
describe("DragHandle", { concurrent: false }, () => {
	afterEach(() => {
		for (const cleanup of cleanups.splice(0)) {
			cleanup()
		}
	})

	it("renders a div with the default class around the slot content", ({
		expect,
	}) => {
		const wrapper = mountHandle({ editor: makeEditor() })

		expect((wrapper.element as HTMLElement).tagName).toBe("DIV")
		expect(wrapper.classes()).toContain("drag-handle")
		expect(wrapper.html()).toContain("<span>drag</span>")
	})

	it("renders the class given by the class prop", ({ expect }) => {
		const wrapper = mountHandle({
			editor: makeEditor(),
			class: "custom-handle",
		})

		expect(wrapper.classes()).toEqual(["custom-handle"])
	})

	it("registers the drag handle plugin against its own rendered element", ({
		expect,
	}) => {
		const editor = makeEditor()
		const pluginKey = new PluginKey("handleUnderTest")
		const wrapper = mountHandle({ editor, pluginKey })

		// the plugin's view hook only reaches these properties if the
		// component handed it the element it rendered
		expect(pluginKey.getState(editor.state)).toBeDefined()
		expect((wrapper.element as HTMLElement).draggable).toBe(true)
		expect((wrapper.element as HTMLElement).style.pointerEvents).toBe("auto")
	})

	it("registers a plugin for a plugin key given as a string", ({ expect }) => {
		const editor = makeEditor()
		const before = editor.state.plugins.length

		mountHandle({ editor, pluginKey: "stringHandleKey" })

		expect(editor.state.plugins).toHaveLength(before + 1)
	})

	it("unregisters the plugin when the component is unmounted", ({ expect }) => {
		const editor = makeEditor()
		const pluginKey = new PluginKey("unmountedHandle")
		const before = editor.state.plugins.length
		const wrapper = mountHandle({ editor, pluginKey })

		wrapper.unmount()

		expect(pluginKey.getState(editor.state)).toBeUndefined()
		expect(editor.state.plugins).toHaveLength(before)
	})

	it("locks the handle when the locked prop turns on", async ({ expect }) => {
		const editor = makeEditor()
		const wrapper = mountHandle({ editor })
		const element = wrapper.element as HTMLElement

		expect(element.draggable).toBe(true)

		await wrapper.setProps({ locked: true })

		// the watcher sends the lock through the editor, and the plugin's
		// view reacts to that transaction by pinning the handle
		expect(element.draggable).toBe(false)
	})

	it("unlocks the handle when the locked prop turns off", async ({
		expect,
	}) => {
		const editor = makeEditor()
		const wrapper = mountHandle({ editor, locked: true })
		const element = wrapper.element as HTMLElement

		await wrapper.setProps({ locked: false })

		expect(element.draggable).toBe(true)
	})

	it("starts locked when the locked prop is set from the beginning", ({
		expect,
	}) => {
		const editor = makeEditor()
		const wrapper = mountHandle({ editor, locked: true })
		const element = wrapper.element as HTMLElement

		editor.commands.insertContent("x")

		expect(element.draggable).toBe(false)
	})

	it("forwards the node change callback to the plugin", ({ expect }) => {
		const editor = makeEditor()
		const onNodeChange = vi.fn()

		mountHandle({ editor, onNodeChange })
		editor.view.focus()
		editor.view.dom.dispatchEvent(
			new KeyboardEvent("keydown", { key: "a", bubbles: true }),
		)

		expect(onNodeChange).toHaveBeenCalledWith({
			editor,
			node: null,
			pos: -1,
			depth: 0,
		})
	})
})
