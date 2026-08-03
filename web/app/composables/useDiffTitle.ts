import { computed } from "vue"
import type { Ref } from "vue"
import type { JSONContent } from "@tiptap/core"
import { diffWordsWithSpace } from "diff"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"

export interface UseDiffTitleOptions {
	/** debounce interval in ms (default: 250) */
	debounceMs?: number
	/** maximum time to wait before forcing an update during continuous edits (default: 1500) */
	maxWaitMs?: number
}

export function useDiffTitle(
	originalTitle: Ref<string>,
	modifiedTitle: Ref<string>,
	options: UseDiffTitleOptions = {},
) {
	const debounceMs = options.debounceMs ?? 250
	const maxWaitMs = options.maxWaitMs ?? 1500

	const debouncedOriginal = refDebounced(originalTitle, debounceMs, {
		maxWait: maxWaitMs,
	})
	const debouncedModified = refDebounced(modifiedTitle, debounceMs, {
		maxWait: maxWaitMs,
	})

	const hasTitleChanged = computed(() => {
		return debouncedOriginal.value !== debouncedModified.value
	})

	const diffContent = computed<JSONContent>(() => {
		const changes = diffWordsWithSpace(
			debouncedOriginal.value,
			debouncedModified.value,
		)
		const content: JSONContent[] = []

		for (const change of changes) {
			if (!change.value) continue

			if (change.removed) {
				content.push({
					type: "text",
					text: change.value,
					marks: [{ type: DIFF_TEXT_REMOVED_MARK_NAME }],
				})
			} else if (change.added) {
				content.push({
					type: "text",
					text: change.value,
					marks: [{ type: DIFF_TEXT_ADDED_MARK_NAME }],
				})
			} else {
				content.push({
					type: "text",
					text: change.value,
				})
			}
		}

		return {
			type: "doc",
			content: [
				{
					type: "paragraph",
					content: content.length > 0 ? content : undefined,
				},
			],
		}
	})

	return { hasTitleChanged, diffContent }
}
