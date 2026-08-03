// @ts-check
import withNuxt from "./.nuxt/eslint.config.mjs"
import eslintConfigPrettier from "eslint-config-prettier/flat"

export default withNuxt([
	eslintConfigPrettier,
	{
		rules: {
			"no-tabs": "off",
			indent: ["off"], // clashes with prettier
			"vue/require-default-prop": "off",
			"@typescript-eslint/no-explicit-any": "off",
			"@typescript-eslint/no-dynamic-delete": "off",
		},
	},
])
