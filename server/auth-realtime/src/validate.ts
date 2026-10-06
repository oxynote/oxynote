// Checks edits against the editor's ProseMirror schema before they are
// applied. The schema is the one source of what may sit where, so core
// sends blocks without checking their placement, and a refusal here
// leaves the document untouched.
import { schema } from "./ydocument.js"
import type { PMNode } from "./operations.js"

type ContentMatch = (typeof schema.topNodeType)["contentMatch"]

// how many node types a message lists before it stops.
const LISTED_TYPES = 5

// checkBlock refuses a block whose own content the schema does not
// accept: an unknown node type, children of the wrong kind or order, or
// a mark its node does not allow.
export function checkBlock(block: PMNode): void {
	const content = block.content ?? []

	checkChildren(
		block.type,
		content.map((child) => child.type),
	)

	for (const child of content) {
		if (child.type !== "text") {
			checkBlock(child)
			continue
		}

		// the editor cannot load a mark its node does not allow, so the
		// edit would be lost while reporting success.
		for (const mark of child.marks ?? []) {
			const markType = schema.marks[mark.type]

			if (
				!markType ||
				!schema.nodes[block.type]?.allowsMarkType(
					markType,
				)
			) {
				throw new Error(
					`the ${mark.type} mark is not allowed in ` +
						`${block.type}: write the text without it`,
				)
			}
		}
	}
}

// checkChildren refuses a parent holding the named children, in that
// order, when the schema does not accept them. parent is a node type
// name, "doc" for the document root. advice ends the message when a
// child does not fit, for a caller that knows how to place it instead.
export function checkChildren(
	parent: string,
	children: string[],
	advice = "",
): void {
	const parentType = schema.nodes[parent]
	if (!parentType) {
		throw new Error(`${parent} is not a known block type`)
	}

	const isRoot = parent === schema.topNodeType.name
	const place = isRoot ? "the document root" : parent
	let match = parentType.contentMatch

	for (const child of children) {
		const childType = schema.nodes[child]
		const next = childType ? match.matchType(childType) : null

		if (!next) {
			const message =
				`${child} is not allowed there in ${place}, ` +
				`which takes ${acceptedTypes(match)} at that point.`

			throw new Error(
				advice ? `${message} ${advice}` : message,
			)
		}

		match = next
	}

	if (!match.validEnd) {
		const next = isRoot
			? "Add one there first."
			: "Add one there first, or delete the block that holds it."

		throw new Error(
			`${place} would end without ${acceptedTypes(match)}. ${next}`,
		)
	}
}

// acceptedTypes lists the node types match takes next.
function acceptedTypes(match: ContentMatch): string {
	const names: string[] = []

	for (let i = 0; i < match.edgeCount; i++) {
		names.push(match.edge(i).type.name)
	}

	if (names.length === 0) {
		return "nothing more"
	}

	if (names.length > LISTED_TYPES) {
		names.splice(LISTED_TYPES, Infinity, "another block")
	}

	return new Intl.ListFormat("en", { type: "disjunction" }).format(names)
}
