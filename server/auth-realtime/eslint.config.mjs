// @ts-check

import eslintConfigPrettier from "eslint-config-prettier/flat"
import eslint from "@eslint/js"
import vitest from "@vitest/eslint-plugin"
import tseslint from "typescript-eslint"

export default tseslint.config(
	{ ignores: ["dist"] },
	eslint.configs.recommended,
	tseslint.configs.strictTypeChecked,
	tseslint.configs.stylisticTypeChecked,
	eslintConfigPrettier,

	// a disable comment whose rule no longer fires is stale and must be
	// deleted, not carried along
	{
		name: "oxynote/linter-options",
		linterOptions: {
			reportUnusedDisableDirectives: "error",
		},
	},

	// type-aware rules need a TS program, which the service's own
	// tsconfig provides for everything under src/. The two overrides
	// belong here rather than in the project-wide block below: both are
	// type-aware, so applying them to the config files that opt out of
	// typed linting would crash the run.
	{
		name: "oxynote/typescript/project",
		files: ["src/**/*.ts"],
		languageOptions: {
			parserOptions: {
				projectService: true,
				tsconfigRootDir: import.meta.dirname,
			},
		},
		rules: {
			// swapping || for ?? on primitive operands changes
			// behavior for ""/0/false — only flag nullable object
			// types, where the swap is always safe
			"@typescript-eslint/prefer-nullish-coalescing": [
				"error",
				{ ignorePrimitives: true },
			],
			// interpolating a number is legitimate, so it is the
			// single allowance on top of the strict set. Every
			// option is spelled out because eslint replaces the
			// preset's option object rather than merging into it —
			// omitting them silently restores the rule's permissive
			// defaults.
			"@typescript-eslint/restrict-template-expressions": [
				"error",
				{
					allowAny: false,
					allowBoolean: false,
					allowNever: false,
					allowNullish: false,
					allowNumber: true,
					allowRegExp: false,
				},
			],
		},
	},

	// build/tooling configs live outside the tsconfig project, so
	// type-aware rules cannot run on them
	{
		name: "oxynote/typescript/disable-type-checked",
		files: [
			"eslint.config.mjs",
			"knip.ts",
			"prettier.config.js",
			"vitest.config.ts",
		],
		languageOptions: {
			parserOptions: {
				program: null,
				project: false,
				projectService: false,
			},
		},
		rules: tseslint.configs.disableTypeChecked.rules,
	},

	// test files: vitest's recommended rules, plus every test file must
	// wrap its tests in a root describe naming the unit under test
	{
		name: "oxynote/vitest",
		files: ["src/**/*.test.ts"],
		plugins: {
			vitest,
		},
		rules: {
			...vitest.configs.recommended.rules,
			"vitest/require-top-level-describe": "error",
		},
	},

	{
		name: "oxynote/rules",
		rules: {
			"no-tabs": "off",
			indent: ["off"], // clashes with prettier
			"@typescript-eslint/no-explicit-any": "off",
			// an assignment must be its own statement — no chaining
			// and no assigning from inside an expression, e.g.
			// `const g = (map[k] ??= {})`
			"no-multi-assign": [
				"error",
				{ ignoreNonDeclaration: false },
			],
		},
	},
)
