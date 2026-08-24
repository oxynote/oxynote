import { defineConfig, devices } from "@playwright/test"
import { BASE_URL } from "./helpers/config"

export default defineConfig({
	testDir: "./tests",
	fullyParallel: true,
	// nearly every test signs up, verifies, logs in and creates a
	// workspace before it starts, and the stack answers all of them from
	// one core and one postgres. Past a few workers the setup itself
	// starts timing out, which reads as flaky tests when it is only load.
	// CI gets fewer still: a private-repo runner has two vCPUs for the
	// whole stack and the browsers together.
	workers: process.env.CI ? 2 : 4,
	// the default 30s budget contradicts the suite's own waits — a login
	// redirect is allowed 15s and an invite acceptance 30s, because they
	// are cross-service chains that stretch under load. CI runs the whole
	// stack on two cores, where a test that takes 7s locally takes 40.
	timeout: 60_000,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	// github gives inline annotations on the PR; html is what the
	// workflow uploads when a run fails, traces and all.
	reporter: process.env.CI
		? [["github"], ["html", { open: "never" }]]
		: [["list"]],
	globalSetup: "./global-setup.ts",
	globalTeardown: "./global-teardown.ts",
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
