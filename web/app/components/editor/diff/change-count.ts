import type { JSONContent } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import {
	NODE_COMMENT_ID_ATTR,
	UPLOADING_ATTR,
} from "~/components/editor/attribute-names"
import { SIMULATION_ACTIVE_ATTR } from "~/components/editor/blocks/metrics/simulation"
import {
	CODE_BLOCK_NAME,
	FILE_BLOCK_NAME,
	IMAGE_BLOCK_NAME,
	METRIC_BLOCK_NAME,
} from "~/components/editor/blocks/node-names"

export interface ChangeCount {
	removed: number
	added: number
}

// the attributes the diff itself writes onto merged nodes
const DIFF_META_ATTRIBUTES = [
	"diffStatus",
	"modifiedIndex",
	"originalIndex",
	"oldNode",
	"modifiedTextContent",
]

const IGNORED_ATTRIBUTES = new Set([
	...DIFF_META_ATTRIBUTES,
	NODE_COMMENT_ID_ATTR,
	SIMULATION_ACTIVE_ATTR,
	UPLOADING_ATTR,
])

// attributes no one can change from the editor, per node type. They only
// arrive with pasted HTML, so they are not counted.
const UNTRACKED_ATTRIBUTES: Record<string, string[]> = {
	[IMAGE_BLOCK_NAME]: ["alt", "title"],
}

// attributes that change together and count as one change, per node
// type. Each attribute maps to the name of its group.
const ATTRIBUTE_GROUPS: Record<string, Record<string, string>> = {
	[CODE_BLOCK_NAME]: {
		language: "language",
		auto: "language",
	},
	[FILE_BLOCK_NAME]: {
		src: "file",
		name: "file",
		size: "file",
		contentType: "file",
	},
}

// list attributes whose items count field by field, per node type. Only
// the listed fields of each item are counted.
const ITEM_FIELDS: Record<string, Record<string, string[]>> = {
	[METRIC_BLOCK_NAME]: {
		queries: ["query", "legendFormat"],
	},
}

export interface AttributeChange {
	// the attribute's name, its group's name, or "<list>.<index>.<field>"
	// for a field of a list item
	name: string
	// undefined on the side where the unit is not set. A group's value is
	// the record of its set attributes.
	oldValue: unknown
	newValue: unknown
}

// one row of a card that lists changed fields
export interface FieldChange {
	label: string
	// null on the side where the field is not set
	oldValue: string | null
	newValue: string | null
	// code such as a query reads better in a monospace font
	code?: boolean
}

export enum TextSegmentKind {
	Equal = "equal",
	Removed = "removed",
	Added = "added",
}

export interface TextSegment {
	kind: TextSegmentKind
	text: string
}

// count the changes of a modified node against its original. A changed
// attribute counts as one removal and one addition, a newly set one as an
// addition and a cleared one as a removal. A group of attributes counts
// like one attribute. Changed text counts by run.
export function countNodeChanges(node: PMNode): ChangeCount {
	const count: ChangeCount = { removed: 0, added: 0 }

	for (const change of collectAttributeChanges(node)) {
		if (change.oldValue !== undefined) {
			count.removed++
		}

		if (change.newValue !== undefined) {
			count.added++
		}
	}

	for (const segment of textSegments(node)) {
		if (segment.kind === TextSegmentKind.Removed) {
			count.removed++
		}

		if (segment.kind === TextSegmentKind.Added) {
			count.added++
		}
	}

	return count
}

export function collectAttributeChanges(node: PMNode): AttributeChange[] {
	// an expanded textblock drops its oldNode, since its marks carry the
	// original instead
	const oldNode = node.attrs.oldNode as JSONContent | null
	if (!oldNode) {
		return []
	}

	const oldUnits = attributeUnits(node.type.name, oldNode.attrs ?? {})
	const newUnits = attributeUnits(node.type.name, node.attrs)
	const names = new Set([...oldUnits.keys(), ...newUnits.keys()])
	const changes: AttributeChange[] = []

	for (const name of names) {
		const oldValue = oldUnits.get(name)
		const newValue = newUnits.get(name)
		if (jsonStableStringify(oldValue) !== jsonStableStringify(newValue)) {
			changes.push({ name: name, oldValue: oldValue, newValue: newValue })
		}
	}

	return changes
}

// the text a card shows for an attribute value
export function displayValue(value: unknown): string {
	if (typeof value === "string") {
		return value
	}

	if (typeof value === "number" || typeof value === "boolean") {
		return String(value)
	}

	return jsonStableStringify(value)
}

// both sides of a change as text. A side that is not set shows unset, or
// is left out when unset is null.
export function formatChange(
	change: AttributeChange,
	format: (value: unknown) => string = displayValue,
	unset: string | null = null,
): { oldValue: string | null; newValue: string | null } {
	return {
		oldValue: change.oldValue === undefined ? unset : format(change.oldValue),
		newValue: change.newValue === undefined ? unset : format(change.newValue),
	}
}

// a modified textblock holds both sides of its text, told apart by the
// added and removed marks. Each unbroken run of one kind becomes one
// segment, so several changes can sit on the same line.
export function textSegments(node: PMNode): TextSegment[] {
	const segments: TextSegment[] = []

	node.descendants((child) => {
		if (!child.isText) {
			return true
		}

		const marks = child.marks.map((mark) => mark.type.name)
		let kind = TextSegmentKind.Equal
		if (marks.includes(DIFF_TEXT_REMOVED_MARK_NAME)) {
			kind = TextSegmentKind.Removed
		} else if (marks.includes(DIFF_TEXT_ADDED_MARK_NAME)) {
			kind = TextSegmentKind.Added
		}

		const last = segments.at(-1)
		if (last?.kind === kind) {
			last.text += child.text ?? ""
		} else {
			segments.push({ kind: kind, text: child.text ?? "" })
		}

		return false
	})

	return segments
}

// sort the set attributes into units, keyed by their group name, or by
// their own name when they have no group. A list attribute with item fields
// becomes one unit per field of each item instead. A unit whose attributes
// are all empty is left out.
function attributeUnits(
	typeName: string,
	attrs: Record<string, unknown>,
): Map<string, unknown> {
	const groups = ATTRIBUTE_GROUPS[typeName] ?? {}
	const itemFields = ITEM_FIELDS[typeName] ?? {}
	const untracked = UNTRACKED_ATTRIBUTES[typeName] ?? []
	const units = new Map<string, unknown>()

	for (const [key, value] of Object.entries(attrs)) {
		if (
			IGNORED_ATTRIBUTES.has(key) ||
			untracked.includes(key) ||
			value === null ||
			value === undefined
		) {
			continue
		}

		const fields = itemFields[key]
		if (fields && Array.isArray(value)) {
			addItemUnits(units, key, value as Record<string, unknown>[], fields)
			continue
		}

		const group = groups[key]
		if (group) {
			units.set(group, { ...(units.get(group) as object), [key]: value })
		} else {
			units.set(key, value)
		}
	}

	return units
}

function addItemUnits(
	units: Map<string, unknown>,
	key: string,
	items: Record<string, unknown>[],
	fields: string[],
) {
	for (const [index, item] of items.entries()) {
		for (const field of fields) {
			const value = item[field]
			if (value !== null && value !== undefined) {
				units.set(`${key}.${index}.${field}`, value)
			}
		}
	}
}
