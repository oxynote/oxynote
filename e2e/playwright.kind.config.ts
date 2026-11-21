import { defineConfig, devices } from "@playwright/test"

// the Helm chart on kind. kind/run.sh installs each release and passes its
// address in E2E_BASE_URL, so there is no global setup.
process.env.E2E_STACK = "kind"

export default defineConfig({
	testDir: "./tests",
	testMatch: "kind-*.test.ts",
	timeout: 60_000,
	forbidOnly: !!process.env.CI,
	reporter: process.env.CI
		? [["github"], ["html", { open: "never" }]]
		: [["list"]],
	use: {
		// no retries here, so on-first-retry would record nothing.
		trace: "retain-on-failure",
		screenshot: "only-on-failure",
	},
	projects: [
		{
			name: "chromium",
			use: { ...devices["Desktop Chrome"] },
		},
	],
})
