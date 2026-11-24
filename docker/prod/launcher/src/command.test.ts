import { describe, it } from "vitest"
import { parseCommand, usage } from "./command.js"

describe("parseCommand", () => {
	const cases = {
		"no argument": { input: [], expected: { name: "serve" } },
		backup: { input: ["backup"], expected: { name: "backup" } },
		restore: {
			input: ["restore"],
			expected: { name: "restore", replace: false },
		},
		"restore --replace": {
			input: ["restore", "--replace"],
			expected: { name: "restore", replace: true },
		},
	}

	for (const [name, c] of Object.entries(cases)) {
		it(`reads ${name}`, ({ expect }) => {
			expect(parseCommand(c.input)).toEqual(c.expected)
		})
	}

	for (const input of [
		["serve"],
		["backup", "--replace"],
		["restore", "--force"],
		["restore", "--replace", "--replace"],
	]) {
		it(`answers ${input.join(" ")} with the usage line`, ({
			expect,
		}) => {
			expect(() => parseCommand(input)).toThrow(usage)
		})
	}
})
