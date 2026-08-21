import { mountSuspended } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import AIChatSidebar from "./AIChatSidebar.vue"
import { emitFrom } from "./test-helpers"

// matchMedia is what useMediaQuery reads; happy-dom's own implementation
// always reports "not matching", so the mobile layout needs a stub
function stubViewport(matches: boolean) {
	vi.stubGlobal(
		"matchMedia",
		vi.fn((query: string) => ({
			matches: matches,
			media: query,
			onchange: null,
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			dispatchEvent: vi.fn(),
		})),
	)
}

function mountSidebar() {
	return mountSuspended(AIChatSidebar, {
		// the chat box drags in the whole tiptap editor stack, which this
		// component only positions
		global: { stubs: { EditorAiChatBox: true } },
	})
}

// the editor store is an app-wide pinia singleton and the viewport stub is
// a global, both shared by every mount in the file
describe("<AIChatSidebar>", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorStore().aiAssistantOpen = false
	})

	it("collapses the panel to nothing while the assistant is closed", async ({
		expect,
	}) => {
		stubViewport(false)

		const wrapper = await mountSidebar()

		expect(wrapper.get("div").attributes("style")).toContain("width: 0rem")
	})

	it("expands the panel when the assistant is open", async ({ expect }) => {
		stubViewport(false)
		useEditorStore().aiAssistantOpen = true

		const wrapper = await mountSidebar()

		expect(wrapper.get("div").attributes("style")).toContain("width: 30rem")
	})

	it("keeps the chat box mounted on wide viewports", async ({ expect }) => {
		stubViewport(false)

		const wrapper = await mountSidebar()

		expect(wrapper.find("editor-ai-chat-box-stub").exists()).toBe(true)
	})

	it("leaves the sheet closed on narrow viewports while the assistant is closed", async ({
		expect,
	}) => {
		stubViewport(true)

		await mountSidebar()

		expect(
			document.body.querySelector("[data-slot='sheet-content']"),
		).toBeNull()
	})

	it("opens a sheet on narrow viewports when the assistant is open", async ({
		expect,
	}) => {
		stubViewport(true)
		useEditorStore().aiAssistantOpen = true

		await mountSidebar()

		expect(
			document.body.querySelector("[data-slot='sheet-content']"),
		).not.toBeNull()
	})

	it("closes the assistant when the sheet is dismissed", async ({ expect }) => {
		stubViewport(true)
		const editorStore = useEditorStore()
		editorStore.aiAssistantOpen = true
		const wrapper = await mountSidebar()

		emitFrom(wrapper, "Sheet", "update:open", false)
		await nextTick()

		expect(editorStore.aiAssistantOpen).toBe(false)
	})
})
