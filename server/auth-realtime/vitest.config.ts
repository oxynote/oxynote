import { defineConfig } from "vitest/config"

export default defineConfig({
	test: {
		// every test runs concurrently, and no mock, stubbed global, or
		// env stub may leak between tests
		sequence: {
			concurrent: true,
		},
		restoreMocks: true,
		unstubGlobals: true,
		unstubEnvs: true,

		// the service is plain node — no DOM, no framework runtime, so
		// there is one environment and tests are named .test.ts
		environment: "node",
		include: ["src/**/*.test.ts"],
		// turns yjs's premature-access warning into a failure; see the
		// file for why a warning is not enough
		setupFiles: ["./src/test-setup.ts"],

		coverage: {
			provider: "v8",
			// terminal summary only — no coverage/ artifact
			// directory for git, prettier, and eslint to trip over.
			// Pass --coverage.reporter=html ad hoc for the
			// browsable report.
			reporter: ["text"],
			include: ["src/**/*.ts"],
			exclude: [
				// the composition root: it reads the environment,
				// opens connections and listens, so evaluating it
				// is starting the service. What it wires together
				// is covered through the factories it calls.
				"src/index.ts",
				// loaded through node's --import before the app
				// graph exists, so it can hold no imports of ours
				"src/sentry.ts",
				"src/**/test-helpers.ts",
				"src/test-setup.ts",
			],
			// set from the measured baseline and ratcheted up as
			// suites grow — never lowered. 100% is not the target:
			// genuinely untestable branches stay visible in the
			// report, marked with NOCOV comments in place
			thresholds: {
				statements: 98.5,
				branches: 96.6,
				functions: 98.5,
				lines: 98.5,
			},
		},
	},
})
