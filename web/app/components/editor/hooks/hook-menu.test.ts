import { describe, it } from "vitest"
import { hookNoticeKeypath } from "./hook-menu"
import type { HookStatus } from "./hook-status"
import { DocumentHookType } from "~/utils/api/document"

describe("hookNoticeKeypath", () => {
	it.for<{ name: string; input: HookStatus | null; expected: string }>([
		{
			name: "says what a hook that is still being made does",
			input: null,
			expected: "editor.hooks.url-watcher.new-notice",
		},
		{
			name: "says what a waiting hook will do",
			input: "fresh",
			expected: "editor.hooks.url-watcher.fresh-notice",
		},
		{
			name: "says what a triggered hook did",
			input: "triggered",
			expected: "editor.hooks.url-watcher.triggered-notice",
		},
		{
			name: "says what a hook that needs attention will do",
			input: "needs-attention",
			expected: "editor.hooks.url-watcher.fresh-notice",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(hookNoticeKeypath(DocumentHookType.URLWatcher, input)).toBe(expected)
	})
})
