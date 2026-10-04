import { describe, it } from "vitest"
import { hookGroupStatus, hookStatus } from "./hook-status"
import {
	DocumentHookType,
	type DocumentHook,
	type DocumentHookStatus,
} from "~/utils/api/document"

const FRESH = { score: "100", status: "active" as const }
const TRIGGERED = { score: "0", status: "active" as const }
const FAILED = { score: "100", status: "unreachable_url" as const }

describe("hookStatus", () => {
	it.for<{
		name: string
		input: { score: string; status: DocumentHookStatus }
		expected: string
	}>([
		{
			name: "shows a waiting hook as fresh",
			input: FRESH,
			expected: "fresh",
		},
		{
			name: "shows a hook at score 0 as triggered",
			input: TRIGGERED,
			expected: "triggered",
		},
		{
			name: "shows a copy being set up by its source's score",
			input: { score: "0", status: "initializing" },
			expected: "triggered",
		},
		{
			name: "shows an unreachable website as needing attention",
			input: FAILED,
			expected: "needs-attention",
		},
		{
			name: "shows an integration missing on the server as needing attention",
			input: { score: "100", status: "unconfigured" },
			expected: "needs-attention",
		},
		{
			name: "shows an unreachable image as needing attention",
			input: { score: "100", status: "unauthorized" },
			expected: "needs-attention",
		},
		{
			name: "shows a missing image as needing attention",
			input: { score: "100", status: "image_not_found" },
			expected: "needs-attention",
		},
		{
			name: "shows a missing github installation as needing attention",
			input: { score: "100", status: "missing_installation" },
			expected: "needs-attention",
		},
		{
			name: "shows a repository too large to compare as needing attention",
			input: { score: "100", status: "tree_truncated" },
			expected: "needs-attention",
		},
		{
			name: "puts a failed check before a trigger",
			input: { score: "0", status: "unreachable_url" },
			expected: "needs-attention",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(hookStatus(makeHook(input))).toBe(expected)
	})
})

describe("hookGroupStatus", () => {
	it.for<{
		name: string
		input: { score: string; status: DocumentHookStatus }[]
		expected: string | null
	}>([
		{
			name: "has no status without hooks",
			input: [],
			expected: null,
		},
		{
			name: "is fresh while every hook is",
			input: [FRESH, FRESH],
			expected: "fresh",
		},
		{
			name: "is triggered once one hook is",
			input: [FRESH, TRIGGERED],
			expected: "triggered",
		},
		{
			name: "needs attention once a check failed",
			input: [FRESH, FAILED],
			expected: "needs-attention",
		},
		{
			name: "counts a failed check that also scored 0 as the failure alone",
			input: [{ score: "0", status: "unreachable_url" }],
			expected: "needs-attention",
		},
		{
			name: "is mixed with a trigger next to a failed check",
			input: [FAILED, FRESH, TRIGGERED],
			expected: "mixed",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(hookGroupStatus(input.map(makeHook))).toBe(expected)
	})
})

function makeHook(input: {
	score: string
	status: DocumentHookStatus
}): DocumentHook {
	return {
		id: "hook-1",
		crossBranchId: "hook-1",
		type: DocumentHookType.URLWatcher,
		documentId: "doc-1",
		organizationId: "org-1",
		branchId: "branch-1",
		blockId: null,
		settings: { url: "https://example.com" },
		state: {},
		status: input.status,
		score: input.score,
		createdAt: new Date("2026-01-01T00:00:00Z"),
	}
}
