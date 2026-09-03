import { mockNuxtImport, mountSuspended } from "@nuxt/test-utils/runtime"
import { enableAutoUnmount, type VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import type { Ref } from "vue"
import ChatBox from "./ChatBox.vue"
import { makeXid } from "~/composables/api/test-helpers"
import { findButtonByText, t } from "~/components/test-helpers"

// the real composable talks to the server over a websocket; the suite
// drives the state it exposes instead
const chat = vi.hoisted(() => {
	return {
		messages: null as unknown as Ref<{ role: string; text: string }[]>,
		isConnected: null as unknown as Ref<boolean>,
		isStreaming: null as unknown as Ref<boolean>,
		toolStatus: null as unknown as Ref<string | null>,
		streamingText: null as unknown as Ref<string>,
		pendingConfirm: null as unknown as Ref<{
			actions: { summary: string; documentName?: string }[]
		} | null>,
		connect: vi.fn(),
		disconnect: vi.fn(),
		sendMessage: vi.fn(),
		setActiveDocument: vi.fn(),
		answerConfirm: vi.fn(),
		resetChat: vi.fn(),
	}
})

mockNuxtImport("useAIChat", () => {
	return () => chat
})

function mountChat(props: Record<string, unknown> = {}) {
	return mountSuspended(ChatBox, { props: props })
}

function messageTexts(wrapper: VueWrapper): string[] {
	return wrapper.findAll(".prose").map((message) => message.text())
}

function sendButton(wrapper: VueWrapper) {
	return findButtonByText(wrapper, t("editor.ai-chat.send"))
}

// the mocked chat state and the editor store are shared by the whole
// file, so these tests cannot interleave
describe("<ChatBox>", { concurrent: false }, () => {
	// every mounted box watches the open page, so a leftover one would
	// answer for the next test too
	enableAutoUnmount(afterEach)

	beforeEach(() => {
		chat.messages = ref([])
		chat.isConnected = ref(true)
		chat.isStreaming = ref(false)
		chat.toolStatus = ref<string | null>(null)
		chat.streamingText = ref("")
		chat.pendingConfirm = ref(null)
		chat.connect.mockClear()
		chat.disconnect.mockClear()
		chat.sendMessage.mockClear()
		chat.setActiveDocument.mockClear()
		chat.answerConfirm.mockClear()
		chat.resetChat.mockClear()
		useEditorStore().updateActiveDocumentId(null)
	})

	it("connects when it opens", async ({ expect }) => {
		await mountChat()

		expect(chat.connect).toHaveBeenCalledTimes(1)
	})

	it("disconnects when it closes", async ({ expect }) => {
		const wrapper = await mountChat()

		wrapper.unmount()

		expect(chat.disconnect).toHaveBeenCalledTimes(1)
	})

	it("tells the assistant which page is open", async ({ expect }) => {
		const documentId = makeXid("doc")
		useEditorStore().updateActiveDocumentId(documentId)

		await mountChat()

		expect(chat.setActiveDocument).toHaveBeenCalledWith(documentId, null)
	})

	it("tells the assistant which branch is open", async ({ expect }) => {
		const documentId = makeXid("doc")
		const branchId = makeXid("branch")
		useEditorStore().updateActiveDocumentId(documentId)
		useEditorStore().updateActiveBranchId(branchId)

		await mountChat()

		expect(chat.setActiveDocument).toHaveBeenCalledWith(documentId, branchId)
	})

	it("tells the assistant when the reader navigates away", async ({
		expect,
	}) => {
		await mountChat()

		useEditorStore().updateActiveDocumentId(makeXid("other"))
		await nextTick()

		expect(chat.setActiveDocument).toHaveBeenCalledTimes(2)
	})

	it("invites the reader to start a conversation", async ({ expect }) => {
		const wrapper = await mountChat()

		expect(wrapper.text()).toContain(t("editor.ai-chat.title"))
		expect(wrapper.text()).toContain("Tell me what's on your mind")
	})

	it("shows the conversation so far", async ({ expect }) => {
		chat.messages.value = [
			{ role: ChatMessageRole.User, text: "hello" },
			{ role: ChatMessageRole.Assistant, text: "quack" },
		]

		const wrapper = await mountChat()

		expect(messageTexts(wrapper)).toEqual(["hello", "quack"])
	})

	// happy-dom lays nothing out, so the container is told how tall its
	// content is and the scroll position is what the box does with that
	function messageList(wrapper: VueWrapper, scrollHeight: number) {
		const container = wrapper.get(".overflow-y-auto").element
		Object.defineProperty(container, "scrollHeight", { value: scrollHeight })

		return container
	}

	it("keeps the newest message in view", async ({ expect }) => {
		const wrapper = await mountChat()
		const container = messageList(wrapper, 480)

		chat.messages.value.push({ role: ChatMessageRole.User, text: "hello" })
		await nextTick()
		await nextTick()

		expect(container.scrollTop).toBe(480)
	})

	it("keeps a streaming answer in view", async ({ expect }) => {
		const answer = ref({ role: ChatMessageRole.Assistant, text: "qua" })
		chat.messages.value = [answer.value]
		const wrapper = await mountChat()
		const container = messageList(wrapper, 640)

		answer.value.text = "quack"
		await nextTick()
		await nextTick()

		expect(container.scrollTop).toBe(640)
	})

	it("renders an answer's markdown", async ({ expect }) => {
		chat.messages.value = [
			{ role: ChatMessageRole.Assistant, text: "**bold** answer" },
		]

		const wrapper = await mountChat()

		expect(wrapper.get(".prose strong").text()).toBe("bold")
	})

	// DOMPurify drops target from the sanitized markup, so the rel pair is
	// the part of the link rule that reaches the page
	it("marks links in an answer as safe to follow", async ({ expect }) => {
		chat.messages.value = [
			{
				role: ChatMessageRole.Assistant,
				text: "[docs](https://oxynote.test)",
			},
		]

		const wrapper = await mountChat()

		const link = wrapper.get(".prose a")

		expect(link.attributes("href")).toBe("https://oxynote.test")
		expect(link.attributes("rel")).toBe("noopener noreferrer")
		expect(link.text()).toBe("docs")
	})

	it("strips scripting out of an answer", async ({ expect }) => {
		chat.messages.value = [
			{
				role: ChatMessageRole.Assistant,
				text: "<img src=x onerror=alert(1)>",
			},
		]

		const wrapper = await mountChat()

		expect(wrapper.find(".prose img").exists()).toBe(false)
		expect(wrapper.get(".prose").text()).toContain("<img")
	})

	it("shows the answer as it streams in", async ({ expect }) => {
		chat.streamingText.value = "thinking out loud"

		const wrapper = await mountChat()

		expect(wrapper.text()).toContain("thinking out loud")
	})

	it("says what the assistant is busy with", async ({ expect }) => {
		chat.isStreaming.value = true
		chat.toolStatus.value = t("editor.ai-chat.tool-status.read-document")

		const wrapper = await mountChat()

		expect(wrapper.text()).toContain(
			t("editor.ai-chat.tool-status.read-document"),
		)
	})

	it("shows a waiting indicator before the first token", async ({ expect }) => {
		chat.isStreaming.value = true

		const wrapper = await mountChat()

		expect(wrapper.findAll(".animate-bounce")).toHaveLength(3)
	})

	it("reports the connection being up", async ({ expect }) => {
		const wrapper = await mountChat()

		expect(wrapper.find(".bg-green-500").exists()).toBe(true)
	})

	it("reports the connection being down", async ({ expect }) => {
		chat.isConnected.value = false

		const wrapper = await mountChat()

		expect(wrapper.find(".bg-red-500").exists()).toBe(true)
	})

	it("sends what the reader typed", async ({ expect }) => {
		const wrapper = await mountChat()
		await wrapper.get("textarea").setValue("  how do i deploy?  ")

		await sendButton(wrapper).trigger("click")

		expect(chat.sendMessage).toHaveBeenCalledTimes(1)
		expect(chat.sendMessage).toHaveBeenCalledWith("how do i deploy?")
		expect((wrapper.get("textarea").element as HTMLTextAreaElement).value).toBe(
			"",
		)
	})

	it("sends the message on enter", async ({ expect }) => {
		const wrapper = await mountChat()
		await wrapper.get("textarea").setValue("hello")

		await wrapper.get("textarea").trigger("keydown", { key: "Enter" })

		expect(chat.sendMessage).toHaveBeenCalledTimes(1)
	})

	it("starts a new line on shift-enter", async ({ expect }) => {
		const wrapper = await mountChat()
		await wrapper.get("textarea").setValue("hello")

		await wrapper
			.get("textarea")
			.trigger("keydown", { key: "Enter", shiftKey: true })

		expect(chat.sendMessage).toHaveBeenCalledTimes(0)
	})

	it("sends nothing while the box is empty", async ({ expect }) => {
		const wrapper = await mountChat()
		await wrapper.get("textarea").setValue("   ")

		await wrapper.get("textarea").trigger("keydown", { key: "Enter" })

		expect(chat.sendMessage).toHaveBeenCalledTimes(0)
		expect(sendButton(wrapper).attributes("disabled")).toBe("")
	})

	it("stops the reader typing while the connection is down", async ({
		expect,
	}) => {
		chat.isConnected.value = false

		const wrapper = await mountChat()

		expect(wrapper.get("textarea").attributes("disabled")).toBe("")
	})

	it("stops the reader typing while an answer is streaming", async ({
		expect,
	}) => {
		chat.isStreaming.value = true

		const wrapper = await mountChat()

		expect(wrapper.get("textarea").attributes("disabled")).toBe("")
	})

	it("starts a new conversation on request", async ({ expect }) => {
		const wrapper = await mountChat()

		await findButtonByText(wrapper, t("editor.ai-chat.new-chat")).trigger(
			"click",
		)

		expect(chat.resetChat).toHaveBeenCalledTimes(1)
	})

	it("asks the reader to confirm the changes the assistant wants", async ({
		expect,
	}) => {
		chat.pendingConfirm.value = {
			actions: [{ summary: "Rewrite the intro", documentName: "Runbook" }],
		}

		const wrapper = await mountChat()

		expect(wrapper.text()).toContain(t("editor.ai-chat.confirm.title"))
		expect(wrapper.text()).toContain("Rewrite the intro")
		expect(wrapper.text()).toContain("Runbook")
	})

	it("approves the changes once", async ({ expect }) => {
		chat.pendingConfirm.value = { actions: [{ summary: "Rewrite" }] }
		const wrapper = await mountChat()

		await findButtonByText(
			wrapper,
			t("editor.ai-chat.confirm.approve"),
		).trigger("click")

		expect(chat.answerConfirm).toHaveBeenCalledTimes(1)
		expect(chat.answerConfirm).toHaveBeenCalledWith(true)
	})

	it("approves every change from now on", async ({ expect }) => {
		chat.pendingConfirm.value = { actions: [{ summary: "Rewrite" }] }
		const wrapper = await mountChat()

		await findButtonByText(
			wrapper,
			t("editor.ai-chat.confirm.approve-all"),
		).trigger("click")

		expect(chat.answerConfirm).toHaveBeenCalledWith(true, true)
	})

	it("declines the changes", async ({ expect }) => {
		chat.pendingConfirm.value = { actions: [{ summary: "Rewrite" }] }
		const wrapper = await mountChat()

		await findButtonByText(
			wrapper,
			t("editor.ai-chat.confirm.decline"),
		).trigger("click")

		expect(chat.answerConfirm).toHaveBeenCalledWith(false)
	})

	it("stops the reader typing while a confirmation is pending", async ({
		expect,
	}) => {
		chat.pendingConfirm.value = { actions: [{ summary: "Rewrite" }] }

		const wrapper = await mountChat()

		expect(wrapper.get("textarea").attributes("disabled")).toBe("")
	})

	it("offers no close button on a wide screen", async ({ expect }) => {
		const wrapper = await mountChat()

		expect(wrapper.findAll("header button")).toHaveLength(0)
	})

	it("closes itself when asked to on a narrow screen", async ({ expect }) => {
		const wrapper = await mountChat({ mobile: true })

		await wrapper.get("header button").trigger("click")

		expect(wrapper.emitted("close-chat-box")).toHaveLength(1)
	})
})
