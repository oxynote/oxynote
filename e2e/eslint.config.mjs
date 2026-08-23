// @ts-check

import eslint from "@eslint/js"
import eslintConfigPrettier from "eslint-config-prettier/flat"
import playwright from "eslint-plugin-playwright"
import tseslint from "typescript-eslint"

export default tseslint.config(
	{ ignores: ["playwright-report", "test-results"] },
	eslint.configs.recommended,
	// type-aware, as in web/: it is what catches a promise that was never
	// awaited, which in a playwright suite is the difference between an
	// assertion that ran and one that only looked like it did.
	tseslint.configs.strictTypeChecked,
	tseslint.configs.stylisticTypeChecked,
	{
		languageOptions: {
			parserOptions: {
				projectService: true,
				tsconfigRootDir: import.meta.dirname,
			},
		},
		linterOptions: {
			reportUnusedDisableDirectives: "error",
		},
	},
	// the config files are plain JS and are not in the tsconfig project
	{
		files: ["**/*.mjs", "**/*.js"],
		extends: [tseslint.configs.disableTypeChecked],
	},
	// the playwright rules encode the suite's own conventions: no
	// waitForTimeout (the "never sleep" rule), no committed .only or
	// .skip, and no assertion left unawaited.
	{
		files: ["tests/**/*.ts", "helpers/**/*.ts"],
		extends: [playwright.configs["flat/recommended"]],
	},
	eslintConfigPrettier,
	{
		rules: {
			"no-tabs": "off",
			indent: ["off"], // clashes with prettier
		},
	},
)
