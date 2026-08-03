declare module "electron-squirrel-startup" {
	const handled: boolean
	export default handled
}

// Baked in at build time by Vite via `vite.electron.config.ts`. Missing env
// vars at build time fail the build (see vite.electron.config.ts).
declare const __API_BASE_URL__: string
declare const __APP_BASE_URL__: string
