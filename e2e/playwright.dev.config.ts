import { defineConfig, devices } from "@playwright/test"

// the stack this suite drives. It is set here, before playwright loads the
// global setup or forks a worker, because both inherit this process's
// environment — helpers/config.ts reads it to resolve the stack's ports.
// Setting it explicitly also stops a value exported in the shell from
// pointing this config at the production stack.
process.env.E2E_STACK = "dev"

export default defineConfig({
	testDir: "./tests",
	// the prod-*.test.ts files hold the cases that only mean something
	// against the all-in-one image; playwright.prod.config.ts runs them.
	testIgnore: "prod-*.test.ts",
	fullyParallel: true,
	// every test signs up, verifies, logs in and creates a workspace
	// before it starts, and the stack answers all of them from one core
	// and one postgres — so the suite is worth running at a width that
	// keeps that contention honest. A test that fails only here is a
	// missing wait, not a reason to turn this down.
	workers: 4,
	// the default 30s budget contradicts the suite's own waits — a login
	// redirect is allowed 15s and an invite acceptance 30s, because they
	// are cross-service chains that stretch under load, and a run that
	// shares its machine with the stack stretches them further still.
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
	// no baseURL: the two suites drive two stacks on two ports, and the
	// origin is resolved by helpers/config.ts instead — visit() splices it
	// on, and every other navigation and request here is already absolute.
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
