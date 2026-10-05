import { describe, it } from "vitest"
import { organizationNames, pickOrganization } from "./organization-names.js"

// what the web forms accept as a workspace name and as its slug.
const NAME_RULE = /^[a-zA-Z0-9-_ ]+$/
const SLUG_RULE = /^[a-zA-Z0-9-_]+$/
// the web app cuts a name it puts in a link at this length.
const MAX_NAME_LENGTH = 20

describe("organizationNames", () => {
	it("holds only names the workspace name rule accepts", ({ expect }) => {
		expect(organizationNames.length).toBeGreaterThan(200)
		expect.soft(
			organizationNames.filter(
				(name) =>
					!NAME_RULE.test(name) ||
					name.length > MAX_NAME_LENGTH,
			),
		).toEqual([])
	})
})

describe("pickOrganization", () => {
	it.for([
		{
			name: "the first name at zero",
			input: 0,
			expected: {
				name: "Sea of Tranquility",
				slug: "sea-of-tranquility",
			},
		},
		{
			name: "the last name just below one",
			input: 0.999999,
			expected: {
				name: "Photon Sphere",
				slug: "photon-sphere",
			},
		},
	])("picks $name", ({ input, expected }, { expect }) => {
		expect(pickOrganization(() => input)).toEqual(expected)
	})

	it.for([
		{ name: "one", input: 1 },
		{ name: "a negative number", input: -0.5 },
	])(
		"throws when the random number is $name",
		({ input }, { expect }) => {
			expect(() => pickOrganization(() => input)).toThrow(
				"the random number is not between zero and one",
			)
		},
	)

	it("gives every name a form the workspace rules accept", ({
		expect,
	}) => {
		// the half step keeps a rounding error from landing on the name
		// before
		const organizations = organizationNames.map((_, index) =>
			pickOrganization(
				() => (index + 0.5) / organizationNames.length,
			),
		)

		expect(organizations.map(({ name }) => name)).toEqual(
			organizationNames,
		)
		expect.soft(
			organizations.filter(
				({ slug }) => !SLUG_RULE.test(slug),
			),
		).toEqual([])
		expect.soft(
			new Set(organizations.map(({ slug }) => slug)).size,
		).toBe(organizationNames.length)
	})
})
