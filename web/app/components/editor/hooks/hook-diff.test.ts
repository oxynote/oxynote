import { describe, it } from "vitest"
import {
	blockChangeCount,
	diffHooks,
	entryChangeCount,
	hookChangeCount,
	listFieldRows,
	scalarFieldRows,
	type HookDiffEntry,
} from "./hook-diff"
import { DiffStatus } from "../diff/position-map"
import {
	DocumentHookType,
	type DocumentHook,
	type DocumentHookSettings,
} from "~/utils/api/document"

describe("diffHooks", () => {
	it.for([
		{
			name: "unchanged while their settings match",
			input: "https://a.com",
			expected: DiffStatus.Unchanged,
		},
		{
			name: "modified once their settings differ",
			input: "https://b.com",
			expected: DiffStatus.Modified,
		},
	])(
		"pairs hooks sharing a cross-branch id as $name",
		({ input, expected }, { expect }) => {
			const active = linked(urlHook("a1", "block-1", input), "x1")
			const target = linked(urlHook("t1", "block-1", "https://a.com"), "x1")

			const entries = diffHooks([active], [target])

			expect(entries).toEqual([
				{ status: expected, hook: active, targetHook: target },
			])
		},
	)

	it.for([
		{
			name: "a hook with identical settings",
			input: () => ({
				active: urlHook("a1", "block-1", "https://a.com"),
				target: urlHook("t1", "block-1", "https://a.com"),
			}),
		},
		{
			name: "a website hook with another address",
			input: () => ({
				active: urlHook("a1", "block-1", "https://b.com"),
				target: urlHook("t1", "block-1", "https://a.com"),
			}),
		},
		{
			name: "a reminder with another date",
			input: () => ({
				active: {
					...reminderHook("2026-03-01T00:00:00Z"),
					id: "r2",
					crossBranchId: "r2",
				},
				target: reminderHook("2026-02-01T00:00:00Z"),
			}),
		},
	])(
		"shows $name that replaced a removed one as added and removed",
		({ input }, { expect }) => {
			const { active, target } = input()

			const entries = diffHooks([active], [target])

			expect(entries).toEqual([
				{ status: DiffStatus.Added, hook: active, targetHook: null },
				{ status: DiffStatus.Removed, hook: target, targetHook: null },
			])
		},
	)

	it("pairs a hook with its linked one, not with an identical unlinked one", ({
		expect,
	}) => {
		const active = linked(urlHook("a1", "block-1", "https://b.com"), "x1")
		const identical = urlHook("t1", "block-1", "https://b.com")
		const source = linked(urlHook("t2", "block-1", "https://a.com"), "x1")

		const entries = diffHooks([active], [identical, source])

		expect(entries).toEqual([
			{ status: DiffStatus.Modified, hook: active, targetHook: source },
			{ status: DiffStatus.Removed, hook: identical, targetHook: null },
		])
	})

	it("lists the active branch's hooks in their order, then the removed ones", ({
		expect,
	}) => {
		const added = urlHook("a1", "block-1", "https://new.com")
		const kept = linked(urlHook("a2", "block-1", "https://a.com"), "x1")
		const source = linked(urlHook("t1", "block-1", "https://a.com"), "x1")
		const removed = urlHook("t2", "block-1", "https://old.com")

		const entries = diffHooks([added, kept], [removed, source])

		expect(entries).toEqual([
			{ status: DiffStatus.Added, hook: added, targetHook: null },
			{ status: DiffStatus.Unchanged, hook: kept, targetHook: source },
			{ status: DiffStatus.Removed, hook: removed, targetHook: null },
		])
	})

	it("returns nothing when neither branch has hooks", ({ expect }) => {
		expect(diffHooks([], [])).toEqual([])
	})
})

describe("hookChangeCount", () => {
	it.for([
		{
			name: "counts a moved reminder date as one change",
			input: () => [
				reminderHook("2026-03-01T00:00:00Z"),
				reminderHook("2026-02-01T00:00:00Z"),
			],
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts nothing for the same reminder date in another form",
			input: () => [
				reminderHook("2026-03-01T00:00:00Z"),
				reminderHook("2026-03-01T01:00:00+01:00"),
			],
			expected: { removed: 0, added: 0 },
		},
		{
			name: "counts a changed url as one change",
			input: () => [
				urlHook("a1", "block-1", "https://b.com"),
				urlHook("t1", "block-1", "https://a.com"),
			],
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts a changed image as one change",
			input: () => [
				imageHook("a1", "block-1", "postgres:17"),
				imageHook("t1", "block-1", "postgres:16"),
			],
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts the github repository, branch and each file",
			input: () => [
				githubHook("org/new", "dev", ["a.md", "c.md", "d.md"]),
				githubHook("org/old", "main", ["a.md", "b.md"]),
			],
			expected: { removed: 3, added: 4 },
		},
		{
			name: "counts nothing for identical github settings",
			input: () => [
				githubHook("org/repo", "main", ["a.md"]),
				githubHook("org/repo", "main", ["a.md"]),
			],
			expected: { removed: 0, added: 0 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const [hook, targetHook] = input()

		expect(hookChangeCount(must(hook), must(targetHook))).toEqual(expected)
	})
})

describe("entryChangeCount", () => {
	it.for([
		{
			name: "counts an added hook as one addition",
			input: (): HookDiffEntry => ({
				status: DiffStatus.Added,
				hook: urlHook("a1", "block-1", "https://a.com"),
				targetHook: null,
			}),
			expected: { removed: 0, added: 1 },
		},
		{
			name: "counts a removed hook as one removal",
			input: (): HookDiffEntry => ({
				status: DiffStatus.Removed,
				hook: urlHook("t1", "block-1", "https://a.com"),
				targetHook: null,
			}),
			expected: { removed: 1, added: 0 },
		},
		{
			name: "counts a modified hook by its fields",
			input: (): HookDiffEntry => ({
				status: DiffStatus.Modified,
				hook: githubHook("org/repo", "dev", ["a.md", "b.md"]),
				targetHook: githubHook("org/repo", "main", ["a.md"]),
			}),
			expected: { removed: 1, added: 2 },
		},
		{
			name: "counts nothing for a modified hook without its target version",
			input: (): HookDiffEntry => ({
				status: DiffStatus.Modified,
				hook: urlHook("a1", "block-1", "https://a.com"),
				targetHook: null,
			}),
			expected: { removed: 0, added: 0 },
		},
		{
			name: "counts nothing for an unchanged hook",
			input: (): HookDiffEntry => ({
				status: DiffStatus.Unchanged,
				hook: urlHook("a1", "block-1", "https://a.com"),
				targetHook: urlHook("t1", "block-1", "https://a.com"),
			}),
			expected: { removed: 0, added: 0 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(entryChangeCount(input())).toEqual(expected)
	})
})

describe("blockChangeCount", () => {
	it("adds up the changes to one block's hooks alone", ({ expect }) => {
		const entries = diffHooks(
			[
				urlHook("a1", "block-1", "https://new.com"),
				imageHook("a2", "block-1", "postgres:17"),
				urlHook("a3", "block-2", "https://other.com"),
			],
			[
				urlHook("t1", "block-1", "https://old.com"),
				reminderHook("2026-03-01T00:00:00Z", "block-1"),
			],
		)

		expect(blockChangeCount(entries, "block-1")).toEqual({
			removed: 2,
			added: 2,
		})
	})

	it("counts the page's own hooks under a null block", ({ expect }) => {
		const entries = diffHooks([urlHook("a1", null, "https://a.com")], [])

		expect(blockChangeCount(entries, null)).toEqual({ removed: 0, added: 1 })
		expect(blockChangeCount(entries, "block-1")).toEqual({
			removed: 0,
			added: 0,
		})
	})
})

describe("scalarFieldRows", () => {
	it.for([
		{
			name: "shows a value alone outside a modified hook",
			input: ["https://a.com", null] as const,
			expected: [{ value: "https://a.com", status: DiffStatus.Unchanged }],
		},
		{
			name: "shows a value alone when the target holds the same",
			input: ["https://a.com", "https://a.com"] as const,
			expected: [{ value: "https://a.com", status: DiffStatus.Unchanged }],
		},
		{
			name: "shows the new value over the old one when they differ",
			input: ["https://b.com", "https://a.com"] as const,
			expected: [
				{ value: "https://b.com", status: DiffStatus.Added },
				{ value: "https://a.com", status: DiffStatus.Removed },
			],
		},
	])("$name", ({ input, expected }, { expect }) => {
		expect(scalarFieldRows(input[0], input[1])).toEqual(expected)
	})
})

describe("listFieldRows", () => {
	it("shows every value as unchanged outside a modified hook", ({ expect }) => {
		expect(listFieldRows(["a.md", "b.md"], null)).toEqual([
			{ value: "a.md", status: DiffStatus.Unchanged },
			{ value: "b.md", status: DiffStatus.Unchanged },
		])
	})

	it("marks new values and puts the dropped ones last", ({ expect }) => {
		expect(listFieldRows(["a.md", "c.md"], ["a.md", "b.md"])).toEqual([
			{ value: "a.md", status: DiffStatus.Unchanged },
			{ value: "c.md", status: DiffStatus.Added },
			{ value: "b.md", status: DiffStatus.Removed },
		])
	})
})

function makeHook(
	id: string,
	type: DocumentHookType,
	blockId: string | null,
	settings: DocumentHookSettings,
): DocumentHook {
	return {
		id: id,
		crossBranchId: id,
		type: type,
		documentId: "doc-1",
		organizationId: "org-1",
		branchId: "branch-1",
		blockId: blockId,
		settings: settings,
		state: {},
		status: "active",
		score: "100",
		createdAt: "2026-01-01",
	}
}

// linked marks a hook as the copy, or the source, of every other hook
// carrying the same cross-branch id
function linked(hook: DocumentHook, crossBranchId: string): DocumentHook {
	return { ...hook, crossBranchId: crossBranchId }
}

function urlHook(
	id: string,
	blockId: string | null,
	url: string,
): DocumentHook {
	return makeHook(id, DocumentHookType.URLWatcher, blockId, { url: url })
}

function imageHook(id: string, blockId: string, image: string): DocumentHook {
	return makeHook(id, DocumentHookType.ContainerImageWatcher, blockId, {
		image: image,
	})
}

function reminderHook(schedule: string, blockId = "block-1"): DocumentHook {
	return makeHook("r1", DocumentHookType.ScheduledReminder, blockId, {
		scale: "linear",
		duration: "24h",
		schedule: schedule,
	})
}

function githubHook(
	repository: string,
	branch: string,
	paths: string[],
): DocumentHook {
	return makeHook("g1", DocumentHookType.GitHubTracking, "block-1", {
		repository: repository,
		branch: branch,
		paths: paths,
	})
}

function must<T>(value: T | undefined): T {
	if (value === undefined) {
		throw new Error("the case needs two hooks")
	}

	return value
}
