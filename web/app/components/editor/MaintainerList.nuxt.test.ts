import type { VueWrapper } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import MaintainerList from "./MaintainerList.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import {
	clearTeleportedOverlays,
	mountUnderTooltipProvider,
	seedAuthOrganization,
	t,
	WAIT_FOR_OPTIONS,
} from "~/components/test-helpers"
import type WsState from "~/utils/websocket"

const DOCUMENT_ID = makeXid("doc")

function member(id: string, name: string, image?: string) {
	return { userId: id, user: { name: name, image: image } }
}

function seedMembers(...members: ReturnType<typeof member>[]) {
	seedAuthOrganization({ members: members })
}

// the maintainer avatars each carry a tooltip, whose context the app
// installs once at page level
function mountList() {
	return mountUnderTooltipProvider(MaintainerList, {})
}

// the trigger and the popover both list the maintainers; the trigger is
// the only part inside the wrapper
function stackNames(wrapper: VueWrapper): string[] {
	return wrapper.findAll("li").map((item) => item.text())
}

// the editor store, the auth cache and the websocket store are shared
// app-wide, so these tests cannot interleave
describe("<MaintainerList>", { concurrent: false }, () => {
	beforeEach(() => {
		clearTeleportedOverlays()
		clearQueryCache()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useWebSocketStateStore().state = null
	})

	afterEach(disposeMockEndpoints)

	it("shows nothing while the page has no maintainers", async ({ expect }) => {
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/maintainers`, () => [])
		seedMembers(member(makeXid("usr"), "Ada"))

		const wrapper = await mountList()

		expect(wrapper.text()).toBe("")
	})

	it("names the maintainers of the page", async ({ expect }) => {
		const adaId = makeXid("usa")
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/maintainers`, () => [
			adaId,
		])
		seedMembers(member(adaId, "Ada"), member(makeXid("usb"), "Grace"))

		const wrapper = await mountList()

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(t("editor.name-editor.maintainers"))
		}, WAIT_FOR_OPTIONS)
		expect(stackNames(wrapper)).toEqual(["A"])
	})

	it("leaves out members who do not maintain the page", async ({ expect }) => {
		const adaId = makeXid("usa")
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/maintainers`, () => [
			adaId,
		])
		seedMembers(member(adaId, "Ada"), member(makeXid("usb"), "Grace"))

		const wrapper = await mountList()

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(t("editor.name-editor.maintainers"))
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).not.toContain("G")
	})

	it("lists every maintainer in the popover", async ({ expect }) => {
		const adaId = makeXid("usa")
		const graceId = makeXid("usb")
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/maintainers`, () => [
			adaId,
			graceId,
		])
		seedMembers(member(adaId, "Ada"), member(graceId, "Grace"))
		const wrapper = await mountList()
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain(t("editor.name-editor.maintainers"))
		}, WAIT_FOR_OPTIONS)

		await wrapper.get("[data-slot='popover-trigger']").trigger("click")
		await nextTick()

		expect(document.body.textContent).toContain("Ada")
		expect(document.body.textContent).toContain("Grace")
	})

	it("shows a maintainer's picture when they have one", async ({ expect }) => {
		const adaId = makeXid("usa")
		mockEndpoint("GET", `/api/documents/${DOCUMENT_ID}/maintainers`, () => [
			adaId,
		])
		seedMembers(member(adaId, "Ada", "https://cdn.test/ada.png"))

		const wrapper = await mountList()

		await vi.waitFor(() => {
			expect(wrapper.find("img").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get("img").attributes("src")).toBe(
			"https://cdn.test/ada.png",
		)
	})

	it("refetches the maintainers when the server says they changed", async ({
		expect,
	}) => {
		const adaId = makeXid("usa")
		const calls = mockEndpoint(
			"GET",
			`/api/documents/${DOCUMENT_ID}/maintainers`,
			() => [adaId],
		)
		seedMembers(member(adaId, "Ada"))
		const handlers: (() => void)[] = []
		const subscribe = vi.fn((_topic: string, handler: () => void) => {
			handlers.push(handler)

			return () => undefined
		})
		useWebSocketStateStore().state = {
			subscribe: subscribe,
		} as unknown as WsState

		await mountList()

		await vi.waitFor(() => {
			expect(calls.length).toBeGreaterThan(0)
		}, WAIT_FOR_OPTIONS)
		const before = calls.length
		handlers.forEach((handler) => {
			handler()
		})
		await vi.waitFor(() => {
			expect(calls.length).toBeGreaterThan(before)
		}, WAIT_FOR_OPTIONS)
		expect(subscribe).toHaveBeenCalledTimes(1)
		expect(subscribe.mock.calls[0]?.[0]).toBe(
			makeWsDocumentMaintainersChangeTopic(DOCUMENT_ID),
		)
	})

	it("subscribes to nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)
		const subscribe = vi.fn()
		useWebSocketStateStore().state = {
			subscribe: subscribe,
		} as unknown as WsState

		await mountList()

		expect(subscribe).toHaveBeenCalledTimes(0)
	})
})
