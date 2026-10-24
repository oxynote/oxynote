import { Schema } from "@tiptap/pm/model"
import { describe, it } from "vitest"
import type {
	VariableGroupTreeElement,
	VariableTreeElement,
	VariableTreeResponse,
} from "~/utils"
import { METRIC_BLOCK_NAME } from "../editor/blocks/node-names"
import {
	draggableItem,
	groupMoveIndex,
	hasOwnGroups,
	isCustomValue,
	keyFromName,
	keyProblem,
	moveBefore,
	optionLabel,
	orderedGroups,
	orderedOptions,
	pageVariableKeys,
	usedVariableIds,
	variableMoveRequest,
	variableReference,
	variableSections,
} from "./variables"

const SCHEMA = new Schema({
	nodes: {
		doc: { content: "block+" },
		paragraph: { group: "block", content: "text*" },
		[METRIC_BLOCK_NAME]: {
			group: "block",
			attrs: { queries: { default: [] }, timeRange: { default: null } },
		},
		text: {},
	},
})

describe("variables", () => {
	describe("variableReference", () => {
		it("prefixes the key with a dollar sign", ({ expect }) => {
			expect(variableReference("region")).toBe("$region")
		})
	})

	describe("keyFromName", () => {
		it.for([
			{
				name: "joins words with underscores",
				input: "Hotfix threshold",
				expected: "hotfix_threshold",
			},
			{ name: "folds accents", input: "Région", expected: "region" },
			{
				name: "collapses runs of other characters",
				input: "a - b / c",
				expected: "a_b_c",
			},
			{
				name: "drops a leading digit or underscore",
				input: "2nd region",
				expected: "nd_region",
			},
			{
				name: "drops a trailing underscore",
				input: "Region!",
				expected: "region",
			},
			{
				name: "yields nothing for a name without letters",
				input: "42",
				expected: "",
			},
		])("$name", ({ input, expected }, { expect }) => {
			expect(keyFromName(input)).toBe(expected)
		})
	})

	describe("keyProblem", () => {
		it("reports nothing for an empty key", ({ expect }) => {
			expect(keyProblem("", makeTree(), null)).toBeNull()
		})

		it.for(["Region", "1region", "re-gion", "_region"])(
			"reports %s as invalid",
			(key, { expect }) => {
				expect(keyProblem(key, makeTree(), null)).toBe("invalid")
			},
		)

		it("reports a key another variable uses as taken", ({ expect }) => {
			expect(keyProblem("env", makeTree(), null)).toBe("taken")
		})

		it("accepts the key the variable itself already has", ({ expect }) => {
			expect(keyProblem("env", makeTree(), "var-env")).toBeNull()
		})

		it("accepts a free, well formed key", ({ expect }) => {
			expect(keyProblem("region_2", makeTree(), null)).toBeNull()
		})
	})

	describe("pageVariableKeys", () => {
		it("collects keys from text and block attributes", ({ expect }) => {
			const doc = SCHEMA.node("doc", null, [
				SCHEMA.node("paragraph", null, [
					SCHEMA.text("Deploys to $env in $region."),
				]),
				SCHEMA.node(METRIC_BLOCK_NAME, {
					queries: [{ query: 'up{env="$env", job="$job_name"}' }],
				}),
			])

			const keys = pageVariableKeys(doc)

			expect([...keys].sort()).toEqual(["env", "job_name", "region"])
		})

		it("collects the built-in a metric chart follows", ({ expect }) => {
			const doc = SCHEMA.node("doc", null, [
				SCHEMA.node(METRIC_BLOCK_NAME, { timeRange: "$metric_range" }),
			])

			expect([...pageVariableKeys(doc)]).toEqual(["metric_range"])
		})

		it("leaves the built-ins out for a chart with a fixed setting", ({
			expect,
		}) => {
			const doc = SCHEMA.node("doc", null, [
				SCHEMA.node(METRIC_BLOCK_NAME, { timeRange: "last_1_hour" }),
			])

			expect(pageVariableKeys(doc).size).toBe(0)
		})

		it("ignores a dollar sign before a digit", ({ expect }) => {
			const doc = SCHEMA.node("doc", null, [
				SCHEMA.node("paragraph", null, [SCHEMA.text("costs $5")]),
			])

			expect(pageVariableKeys(doc).size).toBe(0)
		})
	})

	describe("usedVariableIds", () => {
		it("names the variables whose keys the page uses", ({ expect }) => {
			const ids = usedVariableIds(makeTree(), new Set(["env", "metric_range"]))

			expect([...ids]).toEqual(["var-range", "var-env"])
		})
	})

	describe("orderedGroups", () => {
		it("puts the built-in group first and keeps the rest in order", ({
			expect,
		}) => {
			const tree = makeTree()

			expect(orderedGroups([...tree].reverse()).map((g) => g.id)).toEqual([
				"grp-metrics",
				"grp-deploys",
				"grp-env",
			])
		})
	})

	describe("variableSections", () => {
		it("splits a group used in part across both sections", ({ expect }) => {
			const sections = variableSections(makeTree(), new Set(["var-env"]))

			expect(sections.map(sectionShape)).toEqual([
				{ id: "on-this-page", groups: [["grp-env", ["var-env"]]] },
				{
					id: "other",
					groups: [
						["grp-metrics", ["var-range"]],
						["grp-env", ["var-region"]],
						["grp-deploys", []],
					],
				},
			])
		})

		it("leaves out a section without groups", ({ expect }) => {
			const sections = variableSections(
				[
					makeGroup({ id: "grp-metrics", builtin: true }, [
						makeVariable({ id: "var-range" }),
					]),
				],
				new Set(["var-range"]),
			)

			expect(sections.map((s) => s.id)).toEqual(["on-this-page"])
		})
	})

	describe("hasOwnGroups", () => {
		it("is false while only the built-in group exists", ({ expect }) => {
			expect(hasOwnGroups([makeGroup({ builtin: true })])).toBe(false)
		})

		it("is true once a user group exists", ({ expect }) => {
			expect(hasOwnGroups(makeTree())).toBe(true)
		})
	})

	describe("optionLabel", () => {
		const t = (key: string) => `label:${key}`

		it("names a built-in refresh option by its interval label", ({
			expect,
		}) => {
			expect(optionLabel(makeVariable({ builtin: true }), "5m", t)).toBe(
				"label:general.refresh-interval-options.5m",
			)
		})

		it.for([
			{
				name: "a user's option that looks like an interval",
				input: { builtin: false, value: "5m" },
				expected: "5m",
			},
			{
				name: "a built-in option that is no interval",
				input: { builtin: true, value: "off" },
				expected: "off",
			},
		])("keeps the value of $name", ({ input, expected }, { expect }) => {
			expect(
				optionLabel(makeVariable({ builtin: input.builtin }), input.value, t),
			).toBe(expected)
		})
	})

	describe("orderedOptions", () => {
		const options = ["5m", "off", "1d", "5s", "30m"].map((value, i) => ({
			id: `option-${value}`,
			variableId: "variable",
			value: value,
			sortIndex: i,
		}))

		it("shows the built-in intervals in their own order", ({ expect }) => {
			const variable = makeVariable({ builtin: true, options: options })

			expect(orderedOptions(variable).map((o) => o.value)).toEqual([
				"5s",
				"5m",
				"30m",
				"1d",
				"off",
			])
		})

		it("keeps a user's options as stored", ({ expect }) => {
			const variable = makeVariable({ builtin: false, options: options })

			expect(orderedOptions(variable)).toEqual(options)
		})
	})

	describe("isCustomValue", () => {
		it.for([
			{ name: "no value set", input: null, expected: false },
			{ name: "the default picked", input: "a", expected: false },
			{ name: "another value picked", input: "b", expected: true },
		])("is $expected with $name", ({ input, expected }, { expect }) => {
			expect(
				isCustomValue(makeVariable({ defaultValue: "a", value: input })),
			).toBe(expected)
		})
	})

	describe("moveBefore", () => {
		it("places the item right before the target", ({ expect }) => {
			expect(
				moveBefore(items("a", "b", "c"), "c", "a").map((i) => i.id),
			).toEqual(["c", "a", "b"])
		})

		it("places the item at the end without a target", ({ expect }) => {
			expect(
				moveBefore(items("a", "b", "c"), "a", null).map((i) => i.id),
			).toEqual(["b", "c", "a"])
		})

		it("leaves the list alone for an unknown item", ({ expect }) => {
			expect(moveBefore(items("a", "b"), "x", "a").map((i) => i.id)).toEqual([
				"a",
				"b",
			])
		})
	})

	describe("groupMoveIndex", () => {
		it("counts the index past the built-in group", ({ expect }) => {
			expect(groupMoveIndex(makeTree(), "grp-deploys", "grp-env")).toBe(0)
		})

		it("moves a group to the end without a target", ({ expect }) => {
			expect(groupMoveIndex(makeTree(), "grp-env", null)).toBe(1)
		})

		it("reports no move when the group stays put", ({ expect }) => {
			expect(groupMoveIndex(makeTree(), "grp-env", "grp-deploys")).toBeNull()
		})

		it("reports no move for the built-in group", ({ expect }) => {
			expect(groupMoveIndex(makeTree(), "grp-metrics", "grp-env")).toBeNull()
		})
	})

	describe("variableMoveRequest", () => {
		it("reorders within a group", ({ expect }) => {
			expect(
				variableMoveRequest(makeTree(), "var-region", "grp-env", "var-env"),
			).toEqual({
				id: "var-region",
				groupId: "grp-env",
				sortIndex: 0,
			})
		})

		it("moves into the end of another group", ({ expect }) => {
			expect(
				variableMoveRequest(makeTree(), "var-env", "grp-deploys", null),
			).toEqual({
				id: "var-env",
				groupId: "grp-deploys",
				sortIndex: 0,
			})
		})

		it("reports no move when the variable stays put", ({ expect }) => {
			expect(
				variableMoveRequest(makeTree(), "var-env", "grp-env", "var-region"),
			).toBeNull()
		})

		it.for([
			{
				name: "into the built-in group",
				input: { id: "var-env", groupId: "grp-metrics" },
			},
			{
				name: "of a built-in variable",
				input: { id: "var-range", groupId: "grp-env" },
			},
			{
				name: "into an unknown group",
				input: { id: "var-env", groupId: "grp-missing" },
			},
		])("reports no move $name", ({ input }, { expect }) => {
			expect(
				variableMoveRequest(makeTree(), input.id, input.groupId, null),
			).toBeNull()
		})
	})

	describe("draggableItem", () => {
		it("fills in the sidebar item fields drag and drop ignores", ({
			expect,
		}) => {
			expect(
				draggableItem({
					id: "a",
					parentId: "p",
					dragGroup: "g",
					draggable: true,
					acceptsChildren: false,
				}),
			).toEqual({
				id: "a",
				parentId: "p",
				dragGroup: "g",
				draggable: true,
				acceptsChildren: false,
				name: "",
				active: false,
				actions: [],
				children: null,
			})
		})
	})
})

function makeTree(): VariableTreeResponse {
	return [
		makeGroup({ id: "grp-metrics", builtin: true }, [
			makeVariable({ id: "var-range", key: "metric_range", builtin: true }),
		]),
		makeGroup({ id: "grp-env" }, [
			makeVariable({ id: "var-env", key: "env", groupId: "grp-env" }),
			makeVariable({ id: "var-region", key: "region", groupId: "grp-env" }),
		]),
		makeGroup({ id: "grp-deploys" }, []),
	]
}

function makeGroup(
	group: Partial<VariableGroupTreeElement>,
	variables: VariableTreeElement[] = [],
): VariableGroupTreeElement {
	return {
		id: "grp",
		organizationId: "org",
		groupName: group.id ?? "Group",
		color: "green",
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-10-01T00:00:00Z",
		createdBy: null,
		...group,
		variables: variables,
	}
}

function makeVariable(
	variable: Partial<VariableTreeElement>,
): VariableTreeElement {
	return {
		id: "var",
		organizationId: "org",
		groupId: "grp-metrics",
		key: "key",
		name: "Variable",
		description: null,
		type: "text",
		defaultValue: null,
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-10-01T00:00:00Z",
		createdBy: null,
		options: [],
		value: null,
		...variable,
	}
}

function items(...ids: string[]) {
	return ids.map((id) => ({ id: id }))
}

function sectionShape(section: ReturnType<typeof variableSections>[number]) {
	return {
		id: section.id,
		groups: section.groups.map((g) => [
			g.group.id,
			g.variables.map((v) => v.id),
		]),
	}
}
