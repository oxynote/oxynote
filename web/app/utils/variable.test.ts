import { describe, it } from "vitest"
import type { VariableTreeElement, VariableTreeResponse } from "./api/variable"
import {
	referencedVariableValues,
	referencedWords,
	resolvedVariableValue,
	variableByReference,
} from "./variable"

describe("variable", () => {
	describe("referencedWords", () => {
		it("lists each referenced word once", ({ expect }) => {
			expect(referencedWords('up{env="$env"} > $limit or $env')).toEqual([
				"env",
				"limit",
			])
		})

		it("takes the whole word after the dollar sign", ({ expect }) => {
			expect(referencedWords("$envName $__interval")).toEqual([
				"envName",
				"__interval",
			])
		})

		it("skips a dollar sign before a digit", ({ expect }) => {
			expect(referencedWords("costs $5")).toEqual([])
		})
	})

	describe("variableByReference", () => {
		it("finds the variable a reference names", ({ expect }) => {
			expect(variableByReference(makeTree(), "$region")?.id).toBe("var-region")
		})

		it("finds nothing for a reference to no variable", ({ expect }) => {
			expect(variableByReference(makeTree(), "$zone")).toBeUndefined()
		})
	})

	describe("resolvedVariableValue", () => {
		it.for([
			{
				name: "gives the user's own value",
				input: { value: "eu-west", defaultValue: "us-east" },
				expected: "eu-west",
			},
			{
				name: "gives the default without a value of the user's own",
				input: { value: null, defaultValue: "us-east" },
				expected: "us-east",
			},
		])("$name", ({ input, expected }, { expect }) => {
			expect(
				resolvedVariableValue(makeVariable({ type: "text", ...input })),
			).toBe(expected)
		})

		it("gives the text of a list's chosen option", ({ expect }) => {
			const variable = makeList({ value: "opt-dev" })

			expect(resolvedVariableValue(variable)).toBe("dev")
		})

		it("gives the text of a list's default option", ({ expect }) => {
			expect(resolvedVariableValue(makeList({ value: null }))).toBe("prod")
		})

		it("gives nothing for a list option that no longer exists", ({
			expect,
		}) => {
			expect(resolvedVariableValue(makeList({ value: "opt-gone" }))).toBeNull()
		})
	})

	describe("referencedVariableValues", () => {
		it("maps each referenced key to its value", ({ expect }) => {
			const values = referencedVariableValues(makeTree(), [
				'up{env="$env"}',
				"sum by ($region) (up) > $missing",
			])

			expect(values).toEqual({ env: "prod", region: "us-east" })
		})

		it("maps nothing for texts without a reference", ({ expect }) => {
			expect(referencedVariableValues(makeTree(), ["up"])).toEqual({})
		})
	})
})

function makeTree(): VariableTreeResponse {
	return [
		{
			id: "group-1",
			organizationId: "org-1",
			groupName: "Deploys",
			color: "green",
			sortIndex: 0,
			builtin: false,
			createdAt: "2026-01-01T00:00:00Z",
			createdBy: null,
			variables: [
				makeList({ value: null }),
				makeVariable({
					id: "var-region",
					key: "region",
					type: "text",
					defaultValue: "us-east",
					value: null,
				}),
			],
		},
	]
}

function makeList(
	overrides: Partial<VariableTreeElement>,
): VariableTreeElement {
	return makeVariable({
		id: "var-env",
		key: "env",
		type: "list",
		defaultValue: "opt-prod",
		options: [
			{ id: "opt-prod", variableId: "var-env", value: "prod", sortIndex: 0 },
			{ id: "opt-dev", variableId: "var-env", value: "dev", sortIndex: 1 },
		],
		...overrides,
	})
}

function makeVariable(
	overrides: Partial<VariableTreeElement>,
): VariableTreeElement {
	return {
		id: "var-1",
		organizationId: "org-1",
		groupId: "group-1",
		key: "key",
		name: "Name",
		description: null,
		type: "text",
		defaultValue: null,
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-01-01T00:00:00Z",
		createdBy: null,
		options: [],
		value: null,
		...overrides,
	}
}
