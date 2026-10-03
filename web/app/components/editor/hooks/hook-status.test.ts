import { describe, it } from "vitest"
import { hookGroupStatus, hookStatus } from "./hook-status"
import {
	DocumentHookType,
	type DocumentHook,
	type DocumentHookState,
} from "~/utils/api/document"

const FRESH = { score: "100", state: { status: "active" } }
const TRIGGERED = { score: "0", state: { status: "active" } }
const FAILED = { score: "100", state: { status: "unreachable_url" } }

describe("hookStatus", () => {
	it.for([
		{
			name: "shows a waiting hook as fresh",
			input: { score: "100", state: { status: "active" } },
			expected: "fresh",
		},
		{
			name: "shows a reminder, which stores no status, as fresh",
			input: { score: "40", state: { lastActiveAt: "2026-01-01" } },
			expected: "fresh",
		},
		{
			name: "shows a hook at score 0 as triggered",
			input: { score: "0", state: { status: "active" } },
			expected: "triggered",
		},
		{
			name: "shows an unreachable website as needing attention",
			input: { score: "100", state: { status: "unreachable_url" } },
			expected: "needs-attention",
		},
		{
			name: "shows an unreachable image as needing attention",
			input: { score: "100", state: { status: "unauthorized", digest: "" } },
			expected: "needs-attention",
		},
		{
			name: "shows a missing github installation as needing attention",
			input: {
				score: "100",
				state: { status: "missing_installation", pathsChecksums: {} },
			},
			expected: "needs-attention",
		},
		{
			name: "shows a missing repository as needing attention",
			input: {
				score: "100",
				state: { status: "missing_repository", pathsChecksums: {} },
			},
			expected: "needs-attention",
		},
		{
			name: "shows a missing branch as needing attention",
			input: {
				score: "100",
				state: { status: "missing_branch", pathsChecksums: {} },
			},
			expected: "needs-attention",
		},
		{
			name: "puts a failed check before a trigger",
			input: { score: "0", state: { status: "unreachable_url" } },
			expected: "needs-attention",
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(hookStatus(makeHook(input))).toBe(expected)
	})
})

describe("hookGroupStatus", () => {
	it.for([
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
			input: [{ score: "0", state: { status: "unreachable_url" } }],
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
	state: Record<string, unknown>
}): DocumentHook {
	return {
		id: "hook-1",
		type: DocumentHookType.URLWatcher,
		documentId: "doc-1",
		organizationId: "org-1",
		branchId: "branch-1",
		blockId: null,
		settings: { url: "https://example.com" },
		state: input.state as unknown as DocumentHookState,
		score: input.score,
		createdAt: new Date("2026-01-01T00:00:00Z"),
	}
}
