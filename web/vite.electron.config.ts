import { defineConfig } from "vite"

// Shared by both forge plugin-vite entries (electron/main.ts, electron/preload.ts).
// The plugin auto-wires `build.lib.entry` and output filename per entry; this
// config only declares the runtime target and what must not be bundled.
// Force `.cjs` output so Node loads the bundles as CommonJS regardless of the
// `"type": "module"` setting in the root package.json. Vite already emits CJS
// for Electron main/preload, but with a `.js` extension Node would treat it as
// ESM and crash on the bundled `require()` calls.

// Vite doesn't expose `.env` values to `process.env` (only to
// `import.meta.env` in the renderer); load .env into the build process so the
// electron bundle inherits the same vars Nuxt sees. No fallbacks — values
// must exist somewhere (shell, .env, CI) or the build fails.
try {
	process.loadEnvFile()
} catch {
	// no .env file — env must come from the shell or CI
}

// Auth env vars are baked into the bundle at build time. Runtime env can't
// override them — this is the security boundary for the desktop app.
const apiBaseURL = process.env.NUXT_PUBLIC_NODEJS_API_BASE_HTTP_URL
const appBaseURL = process.env.NUXT_PUBLIC_APP_BASE_URL

if (!apiBaseURL || !appBaseURL) {
	throw new Error(
		"electron build requires NUXT_PUBLIC_NODEJS_API_BASE_HTTP_URL and NUXT_PUBLIC_APP_BASE_URL env vars",
	)
}

export default defineConfig({
	define: {
		__API_BASE_URL__: JSON.stringify(apiBaseURL),
		__APP_BASE_URL__: JSON.stringify(appBaseURL),
	},
	build: {
		target: "node22",
		rollupOptions: {
			external: ["electron", /^node:/],
			output: {
				format: "cjs",
				entryFileNames: "[name].cjs",
			},
		},
	},
})
