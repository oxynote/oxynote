import { defineConfig, devices } from "@playwright/test"
import { BASE_URL } from "./helpers/config"

export default defineConfig({
	testDir: "./tests",
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	// github gives inline annotations on the PR; html is what the
	// workflow uploads when a run fails, traces and all.
	reporter: process.env.CI
		? [["github"], ["html", { open: "never" }]]
		: [["list"]],
	globalSetup: "./global-setup.ts",
	use: {
		baseURL: BASE_URL,
		trace: "on-first-retry",
		screenshot: "only-on-failure",
	},
	projects: [
		{
			name: "chromium",
			use: { ...devices["Desktop Chrome"] },
		},
	],
})
