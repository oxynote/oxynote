import Paragraph from "@tiptap/extension-paragraph"
import { describe, it, vi } from "vitest"
import { nextTick, shallowRef } from "vue"
import * as Y from "yjs"
import { useDiffEditor } from "./useDiffEditor"

function paragraphElem(text: string) {
	const elem = new Y.XmlElement("paragraph")
	elem.insert(0, [new Y.XmlText(text)])

	return elem
}

function makeYdoc(...texts: string[]) {
	const ydoc = new Y.Doc()
	ydoc.getXmlFragment("content").insert(0, texts.map(paragraphElem))

	return ydoc
}

// the debounced recompute holds fake timers, so the tests cannot
// interleave — each test destroys the editor it creates
describe("useDiffEditor", { concurrent: false }, () => {
	it("starts with the merged content of both documents", ({ expect }) => {
		const diff = useDiffEditor(
			() => makeYdoc("hello"),
			() => makeYdoc("hello", "world"),
			[Paragraph],
		)

		expect(diff.editor.value?.getText()).toContain("hello")
		expect(diff.editor.value?.getText()).toContain("world")
		expect(diff.positionMap.value).toHaveLength(2)

		diff.destroy()

		expect(diff.editor.value).toBeNull()
	})

	it("stays empty when both documents are empty", ({ expect }) => {
		const diff = useDiffEditor(
			() => new Y.Doc(),
			() => new Y.Doc(),
			[Paragraph],
		)

		expect(diff.editor.value?.getText()).toBe("")
		expect(diff.positionMap.value).toEqual([])

		diff.destroy()
	})

	it("recomputes after the debounce when a document changes", ({ expect }) => {
		vi.useFakeTimers()

		try {
			const active = makeYdoc("hello")
			const diff = useDiffEditor(
				() => makeYdoc("hello"),
				() => active,
				[Paragraph],
			)

			active.getXmlFragment("content").insert(1, [paragraphElem("world")])

			expect(diff.editor.value?.getText()).not.toContain("world")

			vi.advanceTimersByTime(250)

			expect(diff.editor.value?.getText()).toContain("world")

			diff.destroy()
		} finally {
			vi.useRealTimers()
		}
	})

	it("skips one recompute after suppressNextRecompute", ({ expect }) => {
		vi.useFakeTimers()

		try {
			const active = makeYdoc("hello")
			const diff = useDiffEditor(
				() => makeYdoc("hello"),
				() => active,
				[Paragraph],
			)

			diff.suppressNextRecompute()
			active.getXmlFragment("content").insert(1, [paragraphElem("world")])
			vi.advanceTimersByTime(250)

			expect(diff.editor.value?.getText()).not.toContain("world")

			active.getXmlFragment("content").insert(2, [paragraphElem("more")])
			vi.advanceTimersByTime(250)

			expect(diff.editor.value?.getText()).toContain("world")
			expect(diff.editor.value?.getText()).toContain("more")

			diff.destroy()
		} finally {
			vi.useRealTimers()
		}
	})

	it("recomputes immediately when a document reference is swapped", async ({
		expect,
	}) => {
		const target = shallowRef(makeYdoc("one"))
		const diff = useDiffEditor(
			() => target.value,
			() => makeYdoc("one"),
			[Paragraph],
		)

		target.value = makeYdoc("two")
		await nextTick()

		expect(diff.editor.value?.getText()).toContain("two")

		diff.destroy()
	})

	it("stops reacting after destroy", ({ expect }) => {
		vi.useFakeTimers()

		try {
			const active = makeYdoc("hello")
			const diff = useDiffEditor(
				() => makeYdoc("hello"),
				() => active,
				[Paragraph],
			)

			diff.destroy()

			expect(() => {
				active.getXmlFragment("content").insert(1, [paragraphElem("world")])
				vi.advanceTimersByTime(250)
			}).not.toThrow()
			expect(diff.editor.value).toBeNull()
		} finally {
			vi.useRealTimers()
		}
	})
})
