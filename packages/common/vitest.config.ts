import { defineConfig } from "vitest/config"

export default defineConfig({
	test: {
		// every test runs concurrently, and no mock, stubbed global or env
		// stub may leak between tests
		sequence: {
			concurrent: true,
		},
		restoreMocks: true,
		// vitest clears the call history of every mock before each test.
		// Under concurrency that also wipes the mocks of tests still
		// running. Each test builds its own mocks, so nothing needs
		// clearing.
		clearMocks: false,
		unstubGlobals: true,
		unstubEnvs: true,

		// the code runs in the browser and on servers alike, so it is tested
		// as plain code with no DOM
		environment: "node",
		include: ["src/**/*.test.ts"],

		coverage: {
			provider: "v8",
			// only a summary in the terminal, so no coverage/ directory appears
			reporter: ["text"],
			include: ["src/**/*.ts"],
			exclude: [
				// a constant has no code to run
				"src/schema-version.ts",
				"src/**/test-helpers.ts",
				// a benchmark measures code, it is not code to cover
				"src/**/*.bench.ts",
			],
			// set from the measured coverage. Raise them as the tests grow,
			// but never lower them.
			thresholds: {
				statements: 100,
				branches: 92.6,
				functions: 100,
				lines: 100,
			},
		},
	},
})
