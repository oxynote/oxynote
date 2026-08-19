import { describe, it, vi } from "vitest"
import { nextTick, ref } from "vue"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { useDiffTitle } from "./useDiffTitle"

// the debounce tests hold fake timers across awaited ticks, so the tests
// in this file cannot interleave
describe("useDiffTitle", { concurrent: false }, () => {
	it("reports no change for identical titles", ({ expect }) => {
		const { hasTitleChanged } = useDiffTitle(ref("My Doc"), ref("My Doc"))

		expect(hasTitleChanged.value).toBe(false)
	})

	it("flags a change only after the debounce interval", async ({ expect }) => {
		vi.useFakeTimers()

		try {
			const modified = ref("My Doc")
			const { hasTitleChanged } = useDiffTitle(ref("My Doc"), modified)

			modified.value = "My Doc v2"
			// the debounce timer is armed by a pre-flush watcher on the source
			await nextTick()

			expect(hasTitleChanged.value).toBe(false)

			vi.advanceTimersByTime(250)

			expect(hasTitleChanged.value).toBe(true)
		} finally {
			vi.useRealTimers()
		}
	})

	it("honors a custom debounce interval", async ({ expect }) => {
		vi.useFakeTimers()

		try {
			const modified = ref("My Doc")
			const { hasTitleChanged } = useDiffTitle(ref("My Doc"), modified, {
				debounceMs: 100,
			})

			modified.value = "My Doc v2"
			await nextTick()
			vi.advanceTimersByTime(100)

			expect(hasTitleChanged.value).toBe(true)
		} finally {
			vi.useRealTimers()
		}
	})

	it("renders an unchanged title as plain text", ({ expect }) => {
		const { diffContent } = useDiffTitle(ref("My Doc"), ref("My Doc"))

		expect(diffContent.value).toEqual({
			type: "doc",
			content: [
				{
					type: "paragraph",
					content: [{ type: "text", text: "My Doc" }],
				},
			],
		})
	})

	it("marks an inserted word as added", ({ expect }) => {
		const { diffContent } = useDiffTitle(
			ref("Hello world"),
			ref("Hello brave world"),
		)

		expect(diffContent.value.content?.[0]?.content).toEqual([
			{ type: "text", text: "Hello " },
			{
				type: "text",
				text: "brave ",
				marks: [{ type: DIFF_TEXT_ADDED_MARK_NAME }],
			},
			{ type: "text", text: "world" },
		])
	})

	it("marks a deleted word as removed", ({ expect }) => {
		const { diffContent } = useDiffTitle(
			ref("Hello brave world"),
			ref("Hello world"),
		)

		expect(diffContent.value.content?.[0]?.content).toEqual([
			{ type: "text", text: "Hello " },
			{
				type: "text",
				text: "brave ",
				marks: [{ type: DIFF_TEXT_REMOVED_MARK_NAME }],
			},
			{ type: "text", text: "world" },
		])
	})

	it("marks a replaced word as removed and added", ({ expect }) => {
		const { diffContent } = useDiffTitle(ref("old title"), ref("new title"))

		expect(diffContent.value.content?.[0]?.content).toEqual([
			{
				type: "text",
				text: "old",
				marks: [{ type: DIFF_TEXT_REMOVED_MARK_NAME }],
			},
			{
				type: "text",
				text: "new",
				marks: [{ type: DIFF_TEXT_ADDED_MARK_NAME }],
			},
			{ type: "text", text: " title" },
		])
	})

	it("renders an empty paragraph for empty titles", ({ expect }) => {
		const { diffContent } = useDiffTitle(ref(""), ref(""))

		expect(diffContent.value).toEqual({
			type: "doc",
			content: [{ type: "paragraph", content: undefined }],
		})
	})
})
