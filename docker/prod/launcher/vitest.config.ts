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

		// the launcher is plain node — no DOM, no framework runtime, so
		// there is one environment and tests are named .test.ts
		environment: "node",
		include: ["src/**/*.test.ts"],

		coverage: {
			provider: "v8",
			// terminal summary only — no coverage/ artifact
			// directory for git, prettier, and eslint to trip over.
			reporter: ["text"],
			include: ["src/**/*.ts"],
			exclude: [
				// the composition root: it reads the environment,
				// spawns the children and installs the signal
				// handlers, so evaluating it is starting the
				// container. What it wires together is covered
				// through the modules it calls.
				"src/index.ts",
			],
			// set from the measured baseline and ratcheted up as
			// suites grow — never lowered.
			thresholds: {
				statements: 97.8,
				branches: 92.9,
				functions: 98,
				lines: 97.8,
			},
		},
	},
})
