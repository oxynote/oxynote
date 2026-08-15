// @ts-check
import vueI18n from "@intlify/eslint-plugin-vue-i18n"
import pluginTs from "@typescript-eslint/eslint-plugin"
import eslintConfigPrettier from "eslint-config-prettier/flat"
import withNuxt from "./.nuxt/eslint.config.mjs"

// collects the merged rule entries of one or more flat configs
const rulesOf = (configs) =>
	(Array.isArray(configs) ? configs : [configs]).reduce(
		(acc, c) => ({ ...acc, ...c.rules }),
		{},
	)

export default withNuxt([
	eslintConfigPrettier,

	// the generated Nuxt preset with a tsconfigPath applies the recommended
	// and strict type-checked rule sets, but drops the non-type-aware
	// strict-only rules and omits stylistic entirely — add them back here.
	{
		name: "oxynote/typescript/strict-stylistic",
		files: ["**/*.ts", "**/*.tsx", "**/*.mts", "**/*.cts", "**/*.vue"],
		rules: {
			...rulesOf(pluginTs.configs["flat/strict"]),
			...rulesOf(pluginTs.configs["flat/stylistic"]),
			...rulesOf(pluginTs.configs["flat/stylistic-type-checked-only"]),
		},
	},

	// root-level build/tooling configs live outside every tsconfig project,
	// so type-aware rules cannot run on them. Same for electron/preload.d.ts:
	// TypeScript drops it from the electron project because a sibling
	// preload.ts exists, making the .d.ts look like a build output.
	{
		name: "oxynote/typescript/disable-type-checked",
		files: [
			"colada.options.ts",
			"forge.config.ts",
			"vite.electron.config.ts",
			"electron/preload.d.ts",
		],
		languageOptions: {
			parserOptions: {
				program: null,
				project: false,
				projectService: false,
			},
		},
		rules: rulesOf(pluginTs.configs["flat/disable-type-checked"]),
	},

	...vueI18n.configs["flat/recommended"],
	{
		name: "oxynote/vue-i18n",
		settings: {
			"vue-i18n": {
				localeDir: "./i18n/locales/en/*.json",
				messageSyntaxVersion: "^11.0.0",
			},
		},
		rules: {
			"@intlify/vue-i18n/no-raw-text": "error",
			"@intlify/vue-i18n/no-missing-keys": "error",
			"@intlify/vue-i18n/no-unused-keys": [
				"error",
				{ extensions: [".ts", ".vue"] },
			],
		},
	},

	{
		rules: {
			"no-tabs": "off",
			indent: ["off"], // clashes with prettier
			"vue/require-default-prop": "off",
			"@typescript-eslint/no-explicit-any": "off",
			"@typescript-eslint/no-dynamic-delete": "off",
			// keeps call-signature type literals, e.g. the
			// defineEmits<{ (e: ...): void }>() style used across components
			"@typescript-eslint/prefer-function-type": "off",
		},
	},
])
