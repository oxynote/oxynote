import { Schema, type Node as PMNode } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import { countNodeChanges } from "./change-count"
import { DiffStatus } from "./position-map"
import {
	COMMENT_MARK_NAME,
	DIFF_TEXT_ADDED_MARK_NAME,
	DIFF_TEXT_REMOVED_MARK_NAME,
} from "~/components/editor/mark-names"
import { NODE_COMMENT_ID_ATTR } from "~/components/editor/attribute-names"
import { SIMULATION_ACTIVE_ATTR } from "~/components/editor/blocks/metrics/simulation"
import {
	FILE_BLOCK_NAME,
	METRIC_BLOCK_NAME,
} from "~/components/editor/blocks/node-names"

const ADDED = DIFF_TEXT_ADDED_MARK_NAME
const REMOVED = DIFF_TEXT_REMOVED_MARK_NAME

// an atom stands in for attribute-driven blocks (image, metrics), a code
// textblock for mermaid. Attrs mirror what computeMergedDocument stamps.
const schema = new Schema({
	nodes: {
		doc: { content: "block*" },
		atom: {
			group: "block",
			atom: true,
			attrs: {
				uid: { default: null },
				src: { default: null },
				alt: { default: null },
				width: { default: null },
				queries: { default: null },
				[NODE_COMMENT_ID_ATTR]: { default: null },
				[SIMULATION_ACTIVE_ATTR]: { default: null },
				diffStatus: { default: null },
				modifiedIndex: { default: null },
				originalIndex: { default: null },
				oldNode: { default: null },
			},
		},
		[FILE_BLOCK_NAME]: {
			group: "block",
			atom: true,
			attrs: {
				uid: { default: null },
				src: { default: null },
				name: { default: null },
				size: { default: null },
				contentType: { default: null },
				uploading: { default: false },
				diffStatus: { default: null },
				oldNode: { default: null },
			},
		},
		[METRIC_BLOCK_NAME]: {
			group: "block",
			atom: true,
			attrs: {
				uid: { default: null },
				title: { default: null },
				queries: { default: null },
				diffStatus: { default: null },
				oldNode: { default: null },
			},
		},
		code: {
			group: "block",
			content: "text*",
			marks: "_",
			attrs: {
				uid: { default: null },
				diffStatus: { default: null },
				oldNode: { default: null },
				modifiedTextContent: { default: null },
			},
		},
		text: {},
	},
	marks: {
		[COMMENT_MARK_NAME]: {},
		[DIFF_TEXT_ADDED_MARK_NAME]: {},
		[DIFF_TEXT_REMOVED_MARK_NAME]: {},
	},
})

describe("countNodeChanges", () => {
	it.for([
		{
			name: "counts a changed attribute as one removal and one addition",
			input: { old: { src: "a.png" }, new: { src: "b.png" } },
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts a newly set attribute as one addition",
			input: { old: { alt: null }, new: { alt: "a cat" } },
			expected: { removed: 0, added: 1 },
		},
		{
			name: "counts an attribute missing from the original as an addition",
			input: { old: {}, new: { alt: "a cat" } },
			expected: { removed: 0, added: 1 },
		},
		{
			name: "counts a cleared attribute as one removal",
			input: { old: { width: 300 }, new: { width: null } },
			expected: { removed: 1, added: 0 },
		},
		{
			name: "sums the changes across attributes",
			input: {
				old: { src: "a.png", alt: "a cat", width: null },
				new: { src: "b.png", alt: null, width: 300 },
			},
			expected: { removed: 2, added: 2 },
		},
		{
			name: "compares object attributes by content, not key order",
			input: {
				old: { queries: [{ expr: "up", legend: "x" }] },
				new: { queries: [{ legend: "x", expr: "up" }] },
			},
			expected: { removed: 0, added: 0 },
		},
		{
			name: "ignores the attributes the diff leaves out of its comparison",
			input: {
				old: { [NODE_COMMENT_ID_ATTR]: "c1", [SIMULATION_ACTIVE_ATTR]: false },
				new: { [NODE_COMMENT_ID_ATTR]: "c2", [SIMULATION_ACTIVE_ATTR]: true },
			},
			expected: { removed: 0, added: 0 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const node = schema.nodes.atom.create({
			uid: "block-1",
			...input.new,
			diffStatus: DiffStatus.Modified,
			modifiedIndex: 0,
			originalIndex: 0,
			oldNode: { type: "atom", attrs: { uid: "block-1", ...input.old } },
		})

		expect(countNodeChanges(node)).toEqual(expected)
	})

	it("counts no attribute changes on a node without its original", ({
		expect,
	}) => {
		const node = schema.nodes.atom.create({
			uid: "block-1",
			src: "a.png",
			diffStatus: DiffStatus.Modified,
		})

		expect(countNodeChanges(node)).toEqual({ removed: 0, added: 0 })
	})

	it.for([
		{
			name: "counts an uploaded file as one addition",
			input: { old: {}, new: storedFile("a.zip") },
			expected: { removed: 0, added: 1 },
		},
		{
			name: "counts a replaced file as one removal and one addition",
			input: { old: storedFile("a.zip"), new: storedFile("b.zip") },
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts a removed file as one removal",
			input: { old: storedFile("a.zip"), new: {} },
			expected: { removed: 1, added: 0 },
		},
		{
			name: "counts the attributes outside the file group on their own",
			input: {
				old: storedFile("a.zip"),
				new: { ...storedFile("a.zip"), uploading: true },
			},
			expected: { removed: 1, added: 1 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const node = schema.nodes[FILE_BLOCK_NAME].create({
			uid: "file-1",
			...input.new,
			diffStatus: DiffStatus.Modified,
			oldNode: {
				type: FILE_BLOCK_NAME,
				attrs: { uid: "file-1", uploading: false, ...input.old },
			},
		})

		expect(countNodeChanges(node)).toEqual(expected)
	})

	it.for([
		{
			name: "counts an added query by its query and legend",
			input: {
				old: [query("up", "")],
				new: [query("up", ""), query("rate", "")],
			},
			expected: { removed: 0, added: 2 },
		},
		{
			name: "counts every added query",
			input: {
				old: [query("up", "")],
				new: [query("up", ""), query("rate", ""), query("sum", "{{job}}")],
			},
			expected: { removed: 0, added: 4 },
		},
		{
			name: "counts a removed query by its query and legend",
			input: {
				old: [query("up", ""), query("rate", "")],
				new: [query("up", "")],
			},
			expected: { removed: 2, added: 0 },
		},
		{
			name: "counts an edited legend as one removal and one addition",
			input: { old: [query("up", "")], new: [query("up", "{{job}}")] },
			expected: { removed: 1, added: 1 },
		},
		{
			name: "ignores the generated query names",
			input: {
				old: [{ ...query("up", ""), name: "Query 1" }],
				new: [{ ...query("up", ""), name: "Query 2" }],
			},
			expected: { removed: 0, added: 0 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const node = schema.nodes[METRIC_BLOCK_NAME].create({
			uid: "metric-1",
			queries: input.new,
			diffStatus: DiffStatus.Modified,
			oldNode: {
				type: METRIC_BLOCK_NAME,
				attrs: { uid: "metric-1", queries: input.old },
			},
		})

		expect(countNodeChanges(node)).toEqual(expected)
	})

	it.for([
		{
			name: "counts a replaced run as one removal and one addition",
			input: [
				["graph TD\n"],
				["A-->B", REMOVED],
				["A-->C", ADDED],
				["\nB-->D"],
			],
			expected: { removed: 1, added: 1 },
		},
		{
			name: "counts every run on the same line",
			input: [
				["A-->"],
				["B", REMOVED],
				["C", ADDED],
				[" & "],
				["D", REMOVED],
				[" & E"],
				[" & F", ADDED],
			],
			expected: { removed: 2, added: 2 },
		},
		{
			name: "counts a run spanning several lines once",
			input: [["graph TD"], ["\nA-->B\nB-->C", REMOVED]],
			expected: { removed: 1, added: 0 },
		},
		{
			name: "counts a run split by another mark once",
			input: [["graph TD\n"], ["A-->", ADDED], ["B", ADDED, COMMENT_MARK_NAME]],
			expected: { removed: 0, added: 1 },
		},
		{
			name: "counts no runs when the text is unchanged",
			input: [["graph TD\nA-->B"]],
			expected: { removed: 0, added: 0 },
		},
	])("$name", ({ input, expected }, { expect }) => {
		const node = schema.nodes.code.create(
			{ uid: "code-1", diffStatus: DiffStatus.Modified },
			input.map(([text, ...marks]) => textNode(text ?? "", marks)),
		)

		expect(countNodeChanges(node)).toEqual(expected)
	})
})

function query(expr: string, legendFormat: string): Record<string, unknown> {
	return { name: "Query", query: expr, legendFormat: legendFormat }
}

function storedFile(name: string): Record<string, unknown> {
	return {
		src: `/files/${name}`,
		name: name,
		size: 1024,
		contentType: "application/zip",
	}
}

function textNode(text: string, marks: string[]): PMNode {
	return schema.text(
		text,
		marks.map((mark) => schema.mark(mark)),
	)
}
