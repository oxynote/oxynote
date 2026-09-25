import type { JSONContent } from "@tiptap/core"
import type { Node as PMNode } from "@tiptap/pm/model"
import {
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { NODE_COMMENT_ID_ATTR } from "~/components/editor/attribute-names"
import { SIMULATION_ACTIVE_ATTR } from "~/components/editor/blocks/metrics/simulation"
import {
	FILE_BLOCK_NAME,
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
])

// attributes that change together and count as one change, per node
// type. Each attribute maps to the name of its group.
const ATTRIBUTE_GROUPS: Record<string, Record<string, string>> = {
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

// count the changes of a modified node against its original. A changed
// attribute counts as one removal and one addition, a newly set one as an
// addition and a cleared one as a removal. A group of attributes counts
// like one attribute. Changed text counts by run.
export function countNodeChanges(node: PMNode): ChangeCount {
	// an expanded textblock drops its oldNode, since its marks carry the
	// original instead
	const oldNode = node.attrs.oldNode as JSONContent | null
	const attributes = oldNode
		? countAttributeChanges(node.type.name, oldNode.attrs ?? {}, node.attrs)
		: { removed: 0, added: 0 }
	const text = countTextChanges(node)

	return {
		removed: attributes.removed + text.removed,
		added: attributes.added + text.added,
	}
}

function countAttributeChanges(
	typeName: string,
	oldAttrs: Record<string, unknown>,
	newAttrs: Record<string, unknown>,
): ChangeCount {
	const count: ChangeCount = { removed: 0, added: 0 }
	const oldUnits = attributeUnits(typeName, oldAttrs)
	const newUnits = attributeUnits(typeName, newAttrs)
	const names = new Set([...oldUnits.keys(), ...newUnits.keys()])

	for (const name of names) {
		const oldUnit = oldUnits.get(name)
		const newUnit = newUnits.get(name)
		if (jsonStableStringify(oldUnit) === jsonStableStringify(newUnit)) {
			continue
		}

		if (oldUnit) {
			count.removed++
		}

		if (newUnit) {
			count.added++
		}
	}

	return count
}

// sort the set attributes into units, keyed by their group name, or by
// their own name when they have no group. A list attribute with item fields
// becomes one unit per field of each item instead. A unit whose attributes
// are all empty is left out.
function attributeUnits(
	typeName: string,
	attrs: Record<string, unknown>,
): Map<string, Record<string, unknown>> {
	const groups = ATTRIBUTE_GROUPS[typeName] ?? {}
	const itemFields = ITEM_FIELDS[typeName] ?? {}
	const units = new Map<string, Record<string, unknown>>()

	for (const [key, value] of Object.entries(attrs)) {
		if (IGNORED_ATTRIBUTES.has(key) || value === null || value === undefined) {
			continue
		}

		const fields = itemFields[key]
		if (fields && Array.isArray(value)) {
			addItemUnits(units, key, value as Record<string, unknown>[], fields)
			continue
		}

		const name = groups[key] ?? key
		units.set(name, { ...units.get(name), [key]: value })
	}

	return units
}

function addItemUnits(
	units: Map<string, Record<string, unknown>>,
	key: string,
	items: Record<string, unknown>[],
	fields: string[],
) {
	for (const [index, item] of items.entries()) {
		for (const field of fields) {
			const value = item[field]
			if (value !== null && value !== undefined) {
				units.set(`${key}.${index}.${field}`, { [field]: value })
			}
		}
	}
}

// a modified textblock holds both sides of its text, told apart by the
// added and removed marks. Each unbroken run of one mark is one change,
// so several can sit on the same line.
function countTextChanges(node: PMNode): ChangeCount {
	const count: ChangeCount = { removed: 0, added: 0 }
	let previous: string | null = null

	node.descendants((child) => {
		if (!child.isText) {
			return true
		}

		const marks = child.marks.map((mark) => mark.type.name)
		let current: string | null = null
		if (marks.includes(DIFF_TEXT_REMOVED_MARK_NAME)) {
			current = DIFF_TEXT_REMOVED_MARK_NAME
		} else if (marks.includes(DIFF_TEXT_ADDED_MARK_NAME)) {
			current = DIFF_TEXT_ADDED_MARK_NAME
		}

		if (current === DIFF_TEXT_REMOVED_MARK_NAME && previous !== current) {
			count.removed++
		}

		if (current === DIFF_TEXT_ADDED_MARK_NAME && previous !== current) {
			count.added++
		}

		previous = current

		return false
	})

	return count
}
