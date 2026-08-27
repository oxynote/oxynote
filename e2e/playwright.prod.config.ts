import { defineConfig, devices } from "@playwright/test"

// the same suite, driven against the all-in-one image instead of the four
// containers the dev stack builds. Every flow test runs unchanged — that is
// the point, since the image is what an operator actually installs and
// nothing else proves it serves the product — plus the prod-*.test.ts
// files, which hold the cases that only mean something here.
//
// The stack is named before playwright loads the global setup or forks a
// worker, because both inherit this process's environment;
// helpers/config.ts reads it to resolve the stack's ports and
// helpers/stack.ts to pick the compose file and make targets.
process.env.E2E_STACK = "prod"

export default defineConfig({
	testDir: "./tests",
	fullyParallel: true,
	// the dev suite's width, deliberately: one container running caddy,
	// web, core and auth-realtime together contends harder than four
	// containers do, which is exactly the pressure worth keeping on.
	workers: 4,
	timeout: 60_000,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	reporter: process.env.CI
		? [["github"], ["html", { open: "never" }]]
		: [["list"]],
	globalSetup: "./global-setup.ts",
	globalTeardown: "./global-teardown.ts",
	use: {
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
