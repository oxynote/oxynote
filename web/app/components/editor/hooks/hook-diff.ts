import { DiffStatus } from "../diff/position-map"

export interface HookChangeCount {
	removed: number
	added: number
}

// HookFieldRow is one value a read only hook field shows, marked as new
// or old while the diff is shown.
export interface HookFieldRow {
	value: string
	status: DiffStatus
}

// HookDiffContext is what a hook's menu needs while the diff is shown.
export interface HookDiffContext {
	status: DiffStatus
	// the target's version of a modified hook
	targetHook: DocumentHook | null
	// whether the menu's rows keep a column for the + and − signs
	signColumn: boolean
}

// HookDiffEntry is one hook as the diff shows it: the active branch's
// hook, or the target's for a hook the active branch removed. A modified
// hook also carries the target's version it replaces.
export interface HookDiffEntry {
	status: DiffStatus
	hook: DocumentHook
	targetHook: DocumentHook | null
}

// diffHooks pairs the active branch's hooks with the target's. A fork or
// a merge copies hooks under new ids, so nothing links a hook to its copy.
// Hooks pair up by block and type instead: identical settings first, then
// the rest in creation order, which makes them modified. Whatever is left
// over was added or removed.
export function diffHooks(
	activeHooks: DocumentHook[],
	targetHooks: DocumentHook[],
): HookDiffEntry[] {
	const entries: HookDiffEntry[] = []
	const unpairedTarget = byCreation(targetHooks)
	const unpairedActive: DocumentHook[] = []

	for (const hook of activeHooks) {
		const index = unpairedTarget.findIndex(
			(target) => sameSlot(hook, target) && !hasChanges(hook, target),
		)
		const target = unpairedTarget[index]
		if (!target) {
			unpairedActive.push(hook)
			continue
		}

		unpairedTarget.splice(index, 1)
		entries.push({
			status: DiffStatus.Unchanged,
			hook: hook,
			targetHook: target,
		})
	}

	for (const hook of byCreation(unpairedActive)) {
		const index = unpairedTarget.findIndex((target) => sameSlot(hook, target))
		const target = unpairedTarget[index]
		if (!target) {
			entries.push({ status: DiffStatus.Added, hook: hook, targetHook: null })
			continue
		}

		unpairedTarget.splice(index, 1)
		entries.push({
			status: DiffStatus.Modified,
			hook: hook,
			targetHook: target,
		})
	}

	for (const target of unpairedTarget) {
		entries.push({ status: DiffStatus.Removed, hook: target, targetHook: null })
	}

	return entries
}

// hookChangeCount counts what changed between a hook and the target's
// version of it. A field counts as one change on each side, and a GitHub
// hook counts its tracked files one by one.
export function hookChangeCount(
	hook: DocumentHook,
	targetHook: DocumentHook,
): HookChangeCount {
	switch (hook.type) {
		case DocumentHookType.ScheduledReminder: {
			const settings = hook.settings as DocumentHookSettingsScheduledReminder
			const target =
				targetHook.settings as DocumentHookSettingsScheduledReminder

			return fieldCount(
				new Date(settings.schedule).getTime() !==
					new Date(target.schedule).getTime(),
			)
		}

		case DocumentHookType.GitHubTracking: {
			const settings = hook.settings as DocumentHookSettingsGitHubTracking
			const target = targetHook.settings as DocumentHookSettingsGitHubTracking
			const repository = fieldCount(settings.repository !== target.repository)
			const branch = fieldCount(settings.branch !== target.branch)

			return {
				removed:
					repository.removed +
					branch.removed +
					target.paths.filter((path) => !settings.paths.includes(path)).length,
				added:
					repository.added +
					branch.added +
					settings.paths.filter((path) => !target.paths.includes(path)).length,
			}
		}

		case DocumentHookType.URLWatcher:
			return fieldCount(
				(hook.settings as DocumentHookSettingsURLWatcher).url !==
					(targetHook.settings as DocumentHookSettingsURLWatcher).url,
			)
		case DocumentHookType.ContainerImageWatcher:
			return fieldCount(
				(hook.settings as DocumentHookSettingsContainerImageWatcher).image !==
					(targetHook.settings as DocumentHookSettingsContainerImageWatcher)
						.image,
			)
	}
}

// entryChangeCount counts one entry: a whole hook added or removed is one
// change, and a modified one counts its fields.
export function entryChangeCount(entry: HookDiffEntry): HookChangeCount {
	switch (entry.status) {
		case DiffStatus.Added:
			return { removed: 0, added: 1 }
		case DiffStatus.Removed:
			return { removed: 1, added: 0 }
		case DiffStatus.Modified:
			return entry.targetHook
				? hookChangeCount(entry.hook, entry.targetHook)
				: { removed: 0, added: 0 }
		default:
			return { removed: 0, added: 0 }
	}
}

// blockChangeCount adds up the changes to the hooks of one block, or of
// the whole page when blockId is null.
export function blockChangeCount(
	entries: HookDiffEntry[],
	blockId: string | null,
): HookChangeCount {
	return entries
		.filter((entry) => entry.hook.blockId === blockId)
		.map(entryChangeCount)
		.reduce(
			(sum, count) => ({
				removed: sum.removed + count.removed,
				added: sum.added + count.added,
			}),
			{ removed: 0, added: 0 },
		)
}

// scalarFieldRows lays out a field holding one value. A changed field shows
// its new value over its old one. targetValue is null outside a modified
// hook.
export function scalarFieldRows(
	value: string,
	targetValue: string | null,
): HookFieldRow[] {
	if (targetValue === null || targetValue === value) {
		return [{ value: value, status: DiffStatus.Unchanged }]
	}

	return [
		{ value: value, status: DiffStatus.Added },
		{ value: targetValue, status: DiffStatus.Removed },
	]
}

// listFieldRows lays out a field holding a list. The values the active
// branch dropped follow the ones it holds. targetValues is null outside a
// modified hook.
export function listFieldRows(
	values: string[],
	targetValues: string[] | null,
): HookFieldRow[] {
	if (!targetValues) {
		return values.map((value) => ({
			value: value,
			status: DiffStatus.Unchanged,
		}))
	}

	return [
		...values.map((value) => ({
			value: value,
			status: targetValues.includes(value)
				? DiffStatus.Unchanged
				: DiffStatus.Added,
		})),
		...targetValues
			.filter((value) => !values.includes(value))
			.map((value) => ({ value: value, status: DiffStatus.Removed })),
	]
}

function sameSlot(hook: DocumentHook, other: DocumentHook): boolean {
	return hook.blockId === other.blockId && hook.type === other.type
}

function hasChanges(hook: DocumentHook, targetHook: DocumentHook): boolean {
	const count = hookChangeCount(hook, targetHook)

	return count.removed > 0 || count.added > 0
}

function byCreation(hooks: DocumentHook[]): DocumentHook[] {
	return [...hooks].sort(
		(a, b) => new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime(),
	)
}

function fieldCount(changed: boolean): HookChangeCount {
	return changed ? { removed: 1, added: 1 } : { removed: 0, added: 0 }
}
