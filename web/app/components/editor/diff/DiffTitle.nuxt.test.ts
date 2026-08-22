import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it, vi } from "vitest"
import * as Y from "yjs"
import DiffTitle from "./DiffTitle.vue"
import { WAIT_FOR_OPTIONS } from "~/components/test-helpers"

function titleDoc(text?: string): Y.Doc {
	const ydoc = new Y.Doc()

	if (text !== undefined) {
		const paragraph = new Y.XmlElement("paragraph")
		paragraph.insert(0, [new Y.XmlText(text)])
		ydoc.getXmlFragment("name").insert(0, [paragraph])
	}

	return ydoc
}

function setTitle(ydoc: Y.Doc, text: string) {
	const fragment = ydoc.getXmlFragment("name")
	const paragraph = new Y.XmlElement("paragraph")
	paragraph.insert(0, [new Y.XmlText(text)])
	fragment.delete(0, fragment.length)
	fragment.insert(0, [paragraph])
}

function mountTitle(target: Y.Doc, active: Y.Doc) {
	return mountSuspended(DiffTitle, {
		props: { targetBranchYdoc: target, activeBranchYdoc: active },
	})
}

function added(wrapper: VueWrapper): string[] {
	return wrapper.findAll(".diff-text-added").map((span) => span.text())
}

function removed(wrapper: VueWrapper): string[] {
	return wrapper.findAll(".diff-text-removed").map((span) => span.text())
}

// the diff is debounced behind a shared timer, so these tests cannot
// interleave
describe("<DiffTitle>", { concurrent: false }, () => {
	it("shows an unchanged title plainly", async ({ expect }) => {
		const wrapper = await mountTitle(titleDoc("Runbook"), titleDoc("Runbook"))

		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Runbook")
		}, WAIT_FOR_OPTIONS)
		expect(added(wrapper)).toEqual([])
		expect(removed(wrapper)).toEqual([])
	})

	it("marks the word the draft added to the title", async ({ expect }) => {
		const wrapper = await mountTitle(
			titleDoc("Payments runbook"),
			titleDoc("Payments runbook v2"),
		)

		await vi.waitFor(() => {
			expect(added(wrapper)).toEqual(["v2"])
		}, WAIT_FOR_OPTIONS)
		expect(removed(wrapper)).toEqual([])
	})

	it("marks the word the draft removed from the title", async ({ expect }) => {
		const wrapper = await mountTitle(
			titleDoc("Payments runbook v2"),
			titleDoc("Payments runbook"),
		)

		await vi.waitFor(() => {
			expect(removed(wrapper)).toEqual(["v2"])
		}, WAIT_FOR_OPTIONS)
		expect(added(wrapper)).toEqual([])
	})

	it("marks a reworded title on both sides", async ({ expect }) => {
		const wrapper = await mountTitle(titleDoc("Run"), titleDoc("Runbook"))

		await vi.waitFor(() => {
			expect(added(wrapper)).toEqual(["Runbook"])
		}, WAIT_FOR_OPTIONS)
		expect(removed(wrapper)).toEqual(["Run"])
	})

	it("shows nothing for two empty titles", async ({ expect }) => {
		const wrapper = await mountTitle(titleDoc(), titleDoc())

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.text()).toBe("")
	})

	it("renders the title read-only and without spellchecking", async ({
		expect,
	}) => {
		const wrapper = await mountTitle(titleDoc("Runbook"), titleDoc("Runbook"))

		await vi.waitFor(() => {
			expect(wrapper.find(".ProseMirror").exists()).toBe(true)
		}, WAIT_FOR_OPTIONS)
		expect(wrapper.get(".ProseMirror").attributes("contenteditable")).toBe(
			"false",
		)
		expect(wrapper.get(".ProseMirror").attributes("spellcheck")).toBe("false")
	})

	it("follows a change made to the draft title", async ({ expect }) => {
		const active = titleDoc("Runbook")
		const wrapper = await mountTitle(titleDoc("Runbook"), active)
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Runbook")
		}, WAIT_FOR_OPTIONS)

		setTitle(active, "Runbook v2")

		await vi.waitFor(() => {
			expect(added(wrapper)).toEqual(["v2"])
		}, WAIT_FOR_OPTIONS)
	})

	it("follows a change made to the main title", async ({ expect }) => {
		const target = titleDoc("Runbook")
		const wrapper = await mountTitle(target, titleDoc("Runbook"))
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Runbook")
		}, WAIT_FOR_OPTIONS)

		setTitle(target, "Runbook v2")

		await vi.waitFor(() => {
			expect(removed(wrapper)).toEqual(["v2"])
		}, WAIT_FOR_OPTIONS)
	})

	// a change made after the title is gone must not reach the torn-down
	// editor; the document itself still takes it
	it("stops following the documents once it is gone", async ({ expect }) => {
		const active = titleDoc("Runbook")
		const wrapper = await mountTitle(titleDoc("Runbook"), active)
		await vi.waitFor(() => {
			expect(wrapper.text()).toContain("Runbook")
		}, WAIT_FOR_OPTIONS)

		wrapper.unmount()
		setTitle(active, "Runbook v2")

		expect(active.getXmlFragment("name").length).toBe(1)
	})
})
