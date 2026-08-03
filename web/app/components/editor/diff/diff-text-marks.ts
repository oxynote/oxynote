import { Mark } from "@tiptap/core"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"

// mark applied to inserted text in expanded modified textblocks.
// renders as a styled span with the same class used by the inline
// decoration fallback so existing CSS applies unchanged.
export const DiffTextAddedMark = Mark.create({
	name: DIFF_TEXT_ADDED_MARK_NAME,
	inclusive: false,
	excludes: "",

	parseHTML() {
		return [{ tag: "span.diff-text-added" }]
	},

	renderHTML() {
		return ["span", { class: "diff-text-added" }, 0]
	},
})

// mark applied to deleted text injected into expanded modified
// textblocks. the text occupies real ProseMirror positions so it
// can be selected and commented on.
export const DiffTextRemovedMark = Mark.create({
	name: DIFF_TEXT_REMOVED_MARK_NAME,
	inclusive: false,
	excludes: "",

	parseHTML() {
		return [{ tag: "span.diff-text-removed" }]
	},

	renderHTML() {
		return ["span", { class: "diff-text-removed" }, 0]
	},
})
