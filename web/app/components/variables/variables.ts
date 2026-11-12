import type { Node as PMNode } from "@tiptap/pm/model"
import type {
	VariableGroupTreeElement,
	VariableMoveRequest,
	VariableTreeElement,
	VariableTreeResponse,
} from "~/utils"
import { RefreshInterval, TimeRangePreset } from "~/utils/time-range"
import { KEY_PATTERN, referencedWords } from "~/utils/variable"
import type { SidebarItem } from "../sidebar"

export const DEFAULT_TIME_RANGE = TimeRangePreset.Last24Hours

export const VARIABLE_KEY_PREFIX = "$"

// password managers offer to fill any text field that could be a login.
// No field here is one, and each manager reads its own opt-out attribute.
export const NO_AUTOFILL = {
	autocomplete: "off",
	"data-1p-ignore": "",
	"data-lpignore": "true",
	"data-bwignore": "",
	"data-form-type": "other",
	"data-protonpass-ignore": "true",
} as const

export const VARIABLE_DRAG_GROUPS = {
	group: "variable-groups",
	variable: "variables",
	option: "variable-options",
	// nothing shares these, so no row can be dropped onto or before a
	// built-in one
	builtinGroup: "variable-builtin-group",
	builtinVariable: "variable-builtin",
} as const

export interface VariableSection {
	id: "on-this-page" | "other" | "all"
	groups: {
		group: VariableGroupTreeElement
		variables: VariableTreeElement[]
	}[]
}

export type DraggableItem = SidebarItem & { parentId: string | null }

// DeletionTarget is what the deletion modal asks about. A group that still
// holds variables cannot be deleted, so the modal only explains why.
export type DeletionTarget =
	| {
			kind: "variable"
			name: string
			reference: string
			confirm: () => Promise<void>
	  }
	| {
			kind: "group"
			name: string
			variableCount: number
			confirm: () => Promise<void>
	  }

export interface OptionDraft {
	id: string
	existing: boolean
	value: string
}

export function variableReference(key: string): string {
	return `${VARIABLE_KEY_PREFIX}${key}`
}

// keyFromName turns a name into a key: lowercase letters and digits, with
// every other run of characters as one underscore, starting with a letter
export function keyFromName(name: string): string {
	return name
		.normalize("NFKD")
		.replace(/[\u0300-\u036f]/g, "")
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "_")
		.replace(/^[^a-z]+|_+$/g, "")
}

// keyProblem says what keeps a key from being saved. An empty key has no
// problem to show yet; the form stays unsendable instead.
export function keyProblem(
	key: string,
	tree: VariableTreeResponse,
	ownId: string | null,
): "invalid" | "taken" | null {
	if (!key) {
		return null
	}

	if (!KEY_PATTERN.test(key)) {
		return "invalid"
	}

	const taken = tree.some((group) =>
		group.variables.some((v) => v.key === key && v.id !== ownId),
	)

	return taken ? "taken" : null
}

// pageVariableKeys collects every word the page references as $word, in
// text or in any block attribute, such as a metric query or a chart that
// follows a built-in variable
export function pageVariableKeys(doc: PMNode): Set<string> {
	const keys = new Set<string>()

	doc.descendants((node) => {
		const source = node.isText ? (node.text ?? "") : JSON.stringify(node.attrs)

		referencedWords(source).forEach((word) => keys.add(word))
	})

	return keys
}

// usedVariableIds names the variables whose keys the page uses
export function usedVariableIds(
	tree: VariableTreeResponse,
	keys: Set<string>,
): Set<string> {
	return new Set(
		tree.flatMap((group) =>
			group.variables.filter((v) => keys.has(v.key)).map((v) => v.id),
		),
	)
}

export function orderedGroups(
	tree: VariableTreeResponse,
): VariableGroupTreeElement[] {
	return [...tree.filter((g) => g.builtin), ...tree.filter((g) => !g.builtin)]
}

// variableSections splits the groups by whether the page uses their
// variables. A group used only in part shows in both sections, and an
// empty group always lands under other.
export function variableSections(
	tree: VariableTreeResponse,
	usedIds: Set<string>,
): VariableSection[] {
	const onThisPage: VariableSection = { id: "on-this-page", groups: [] }
	const other: VariableSection = { id: "other", groups: [] }

	orderedGroups(tree).forEach((group) => {
		const used = group.variables.filter((v) => usedIds.has(v.id))
		const unused = group.variables.filter((v) => !usedIds.has(v.id))

		if (used.length) {
			onThisPage.groups.push({ group: group, variables: used })
		}

		if (unused.length || !group.variables.length) {
			other.groups.push({ group: group, variables: unused })
		}
	})

	return [onThisPage, other].filter((section) => section.groups.length)
}

export function hasOwnGroups(tree: VariableTreeResponse): boolean {
	return tree.some((group) => !group.builtin)
}

// optionLabel names a list option. The built-in refresh interval stores
// interval values as its options, so they read like the metric charts'
// own refresh picker.
export function optionLabel(
	variable: VariableTreeElement,
	value: string,
	t: (key: string) => string,
): string {
	const intervals: string[] = Object.values(RefreshInterval)
	const interval = intervals.find((i) => i === value)
	if (!variable.builtin || !interval) {
		return value
	}

	return t(`general.refresh-interval-options.${interval}`)
}

// orderedOptions lists a list's options as the select shows them. The
// built-in refresh interval stores its default first, because a list's
// default is its first option, but it shows its intervals in their own
// order, shortest first.
export function orderedOptions(
	variable: VariableTreeElement,
): VariableTreeElement["options"] {
	if (!variable.builtin) {
		return variable.options
	}

	const intervals: string[] = Object.values(RefreshInterval)
	const rank = (value: string) => {
		const index = intervals.indexOf(value)

		return index === -1 ? intervals.length : index
	}

	return variable.options.toSorted((a, b) => rank(a.value) - rank(b.value))
}

// isCustomValue tells whether the user's value strays from the default,
// which is what offers them a reset
export function isCustomValue(variable: VariableTreeElement): boolean {
	return variable.value !== null && variable.value !== variable.defaultValue
}

// moveBefore returns the items with the one carrying id placed right
// before insertBeforeId, or at the end when that is null
export function moveBefore<T extends { id: string }>(
	items: T[],
	id: string,
	insertBeforeId: string | null,
): T[] {
	const moved = items.find((item) => item.id === id)
	const rest = items.filter((item) => item.id !== id)
	if (!moved) {
		return items
	}

	const index = insertBeforeId
		? rest.findIndex((item) => item.id === insertBeforeId)
		: rest.length

	rest.splice(index === -1 ? rest.length : index, 0, moved)

	return rest
}

// groupMoveIndex is the sort index a group takes when dropped before
// another one, or at the end. The built-in group stays first, so the
// index counts only the groups after it. Null means the group stays put.
export function groupMoveIndex(
	tree: VariableTreeResponse,
	id: string,
	insertBeforeId: string | null,
): number | null {
	const movable = orderedGroups(tree).filter((g) => !g.builtin)
	const oldIndex = movable.findIndex((g) => g.id === id)
	const newIndex = moveBefore(movable, id, insertBeforeId).findIndex(
		(g) => g.id === id,
	)

	return oldIndex === -1 || oldIndex === newIndex ? null : newIndex
}

// variableMoveRequest places a variable before another one of the target
// group, or at its end. Null means the variable stays put.
export function variableMoveRequest(
	tree: VariableTreeResponse,
	id: string,
	groupId: string,
	insertBeforeId: string | null,
): VariableMoveRequest | null {
	const group = tree.find((g) => g.id === groupId)
	const variable = tree.flatMap((g) => g.variables).find((v) => v.id === id)
	if (!group || group.builtin || !variable || variable.builtin) {
		return null
	}

	const placed = moveBefore(
		[...group.variables.filter((v) => v.id !== id), variable],
		id,
		insertBeforeId,
	)
	const sortIndex = placed.findIndex((v) => v.id === id)
	const oldIndex = group.variables.findIndex((v) => v.id === id)

	if (variable.groupId === groupId && oldIndex === sortIndex) {
		return null
	}

	return { id: id, groupId: groupId, sortIndex: sortIndex }
}

// draggableItem shapes a row for useSidebarDraggable, which knows rows only
// as sidebar items
export function draggableItem(row: {
	id: string
	parentId: string | null
	dragGroup: string
	draggable: boolean
	acceptsChildren: boolean
}): DraggableItem {
	return {
		...row,
		name: "",
		active: false,
		actions: [],
		children: null,
	}
}
