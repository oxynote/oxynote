import type { ForgeConfig } from "@electron-forge/shared-types"
import { FusesPlugin } from "@electron-forge/plugin-fuses"
import { FuseV1Options, FuseVersion } from "@electron/fuses"

// Allowlist of top-level paths bundled into the packaged app.
// `.vite/build` — compiled main + preload (via @electron-forge/plugin-vite).
// `.output/public` — renderer SPA produced by `nuxt generate` (static preset).
// `package.json` — Electron reads `"main"` from here.
const ALLOWED = ["/.vite", "/.output/public"]

const config: ForgeConfig = {
	packagerConfig: {
		asar: true,
		name: "Oxynote",
		executableName: "oxynote",
		appBundleId: "io.oxynote.desktop",
		appCategoryType: "public.app-category.developer-tools",
		// Registers the oxynote:// deep-link scheme with the OS at install time.
		// Better Auth's electron() plugin redirects the OAuth callback to this
		// scheme; the OS focuses Electron, setupMain() exchanges the code.
		protocols: [{ name: "Oxynote", schemes: ["oxynote"] }],
		ignore: (filePath) => {
			if (filePath === "") return false
			if (filePath === "/package.json") return false
			for (const prefix of ALLOWED) {
				if (filePath === prefix || filePath.startsWith(prefix + "/"))
					return false
			}
			return true
		},
	},
	rebuildConfig: {},
	makers: [
		{
			name: "@electron-forge/maker-squirrel",
			config: {},
		},
		{
			name: "@electron-forge/maker-zip",
			platforms: ["darwin"],
			config: {},
		},
		{
			name: "@electron-forge/maker-deb",
			config: {},
		},
		{
			name: "@electron-forge/maker-rpm",
			config: {},
		},
	],
	plugins: [
		{
			name: "@electron-forge/plugin-auto-unpack-natives",
			config: {},
		},
		{
			name: "@electron-forge/plugin-vite",
			config: {
				build: [
					{
						entry: "electron/main.ts",
						config: "vite.electron.config.ts",
						target: "main",
					},
					{
						entry: "electron/preload.ts",
						config: "vite.electron.config.ts",
						target: "preload",
					},
				],
				renderer: [],
			},
		},
		new FusesPlugin({
			version: FuseVersion.V1,
			[FuseV1Options.RunAsNode]: false,
			[FuseV1Options.EnableCookieEncryption]: true,
			[FuseV1Options.EnableNodeOptionsEnvironmentVariable]: false,
			[FuseV1Options.EnableNodeCliInspectArguments]: false,
			[FuseV1Options.EnableEmbeddedAsarIntegrityValidation]: true,
			[FuseV1Options.OnlyLoadAppFromAsar]: true,
		}),
	],
}

export default config
