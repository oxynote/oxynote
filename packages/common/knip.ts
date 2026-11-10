import type { KnipConfig } from "knip"

const config: KnipConfig = {
	project: ["src/**/*.ts"],

	// some types are exported only so that another export in the same file
	// can name them. They are part of that file's contract, so they are not
	// dead exports.
	ignoreExportsUsedInFile: {
		interface: true,
		type: true,
	},
}

export default config
