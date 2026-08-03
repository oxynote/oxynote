// @ts-check

import eslintConfigPrettier from "eslint-config-prettier/flat"
import eslint from "@eslint/js"
import tseslint from "typescript-eslint"

export default tseslint.config(
	eslint.configs.recommended,
	tseslint.configs.strict,
	tseslint.configs.stylistic,
	eslintConfigPrettier,
	{
		rules: {
			"no-tabs": "off",
			indent: ["off"], // clashes with prettier
			"@typescript-eslint/no-explicit-any": "off",
		},
	},
)
