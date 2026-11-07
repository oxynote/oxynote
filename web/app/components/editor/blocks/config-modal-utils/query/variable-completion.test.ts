import { describe, it } from "vitest"
import { interpolatingT, variableTree } from "./test-helpers"
import { variableCompletions } from "./variable-completion"

describe("variable-completion", () => {
	describe("variableCompletions", () => {
		it("offers each variable by its key and type", ({ expect }) => {
			const completions = variableCompletions(variableTree(), interpolatingT)

			expect(completions).toEqual([
				{
					label: "$env",
					detail: interpolatingT("general.variable-types.list"),
					type: "variable",
					info: interpolatingT(
						"editor.config-modal-utils.query-info.variable",
						{
							value: "prod",
						},
					),
				},
				{
					label: "$window",
					detail: interpolatingT("general.variable-types.text"),
					type: "variable",
					info: interpolatingT(
						"editor.config-modal-utils.query-info.variable",
						{
							value: "1h",
						},
					),
				},
			])
		})

		it("puts the description above the current value", ({ expect }) => {
			const tree = variableTree()
			const env = tree[0]?.variables[0]
			if (!env) {
				throw new Error("the fixture has no variables")
			}

			env.description = "Where the service runs"

			const [completion] = variableCompletions(tree, interpolatingT)

			expect(completion?.info).toBe(
				`Where the service runs\n\n${interpolatingT(
					"editor.config-modal-utils.query-info.variable",
					{ value: "prod" },
				)}`,
			)
		})
	})
})
