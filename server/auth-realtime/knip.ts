import type { KnipConfig } from "knip"

const config: KnipConfig = {
	project: ["src/**/*.ts"],

	// a type exported only so another export in the same file can name it
	// — a union member, a table in the Database map, a dependency bag on a
	// factory — is part of that file's contract, not a dead export. Values
	// are deliberately left out: an exported const nobody imports is worth
	// hearing about.
	ignoreExportsUsedInFile: {
		interface: true,
		type: true,
	},
}

export default config
