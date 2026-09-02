// @ts-check
import js from "@eslint/js"
import vueI18n from "@intlify/eslint-plugin-vue-i18n"
import pluginTs from "@typescript-eslint/eslint-plugin"
import vitest from "@vitest/eslint-plugin"
import eslintConfigPrettier from "eslint-config-prettier/flat"
import withNuxt from "./.nuxt/eslint.config.mjs"

// collects the merged rule entries of one or more flat configs
/**
 * @param {{ rules?: Record<string, any> } | { rules?: Record<string, any> }[]} configs
 * @returns {Record<string, any>}
 */
const rulesOf = (configs) =>
	(Array.isArray(configs) ? configs : [configs]).reduce(
		(acc, c) => ({ ...acc, ...c.rules }),
		{},
	)

export default withNuxt([
	eslintConfigPrettier,

	// a disable comment whose rule no longer fires is stale and must be
	// deleted, not carried along
	{
		name: "oxynote/linter-options",
		linterOptions: {
			reportUnusedDisableDirectives: "error",
		},
	},

	// the Nuxt preset ships no base JS rules at all, so plain mistakes like
	// unreachable code went unreported. eslint-recommended then switches off
	// the entries TypeScript already covers better.
	{
		name: "oxynote/js/recommended",
		rules: {
			...rulesOf(js.configs.recommended),
			...rulesOf(pluginTs.configs["flat/eslint-recommended"]),
		},
	},

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
			// swapping || for ?? on primitive operands changes behavior for
			// ""/0/false — only flag nullable object types, where the swap
			// is always safe
			"@typescript-eslint/prefer-nullish-coalescing": [
				"error",
				{ ignorePrimitives: true },
			],
			// interpolating a number is legitimate, so it is the single
			// allowance on top of the strict set. Every option is spelled out
			// because eslint replaces the preset's option object rather than
			// merging into it — omitting them silently restores the rule's
			// permissive defaults and lets `${maybeUndefined}` reach the UI.
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

	// root-level build/tooling configs live outside every tsconfig project,
	// so type-aware rules cannot run on them. Same for electron/preload.d.ts:
	// TypeScript drops it from the electron project because a sibling
	// preload.ts exists, making the .d.ts look like a build output.
	{
		name: "oxynote/typescript/disable-type-checked",
		files: [
			"colada.options.ts",
			"forge.config.ts",
			"knip.ts",
			"vite.electron.config.ts",
			"vitest.config.ts",
			"vitest.nuxt-setup.ts",
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

	// test files: vitest's recommended rules, plus every test file must
	// wrap its tests in a root describe naming the unit under test
	{
		name: "oxynote/vitest",
		files: ["**/*.test.ts", "**/*.nuxt.test.ts", "**/*.browser.test.ts"],
		plugins: {
			vitest,
		},
		rules: {
			...vitest.configs.recommended.rules,
			"vitest/require-top-level-describe": "error",
		},
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
			"@intlify/vue-i18n/no-raw-text": [
				"error",
				// punctuation-only fragments (list separators etc.) are not
				// translation targets
				{ ignorePattern: "^[-–—.,:;!?()\\s\"']+$" },
			],
			"@intlify/vue-i18n/no-missing-keys": "error",
			"@intlify/vue-i18n/no-unused-keys": [
				"error",
				{
					extensions: [".ts", ".vue"],
					// the rule cannot see dynamically built keys
					// (t(`ns.${x}`)) or literal keys stored in data
					// structures and passed to t() later. Namespaces
					// accessed that way are ignored — keys under them are
					// never reported unused.
					ignores: [
						// dynamically built keys: t(`ns.${x}`)
						"/^editor\\.hooks\\.time-expiration\\.duration-options\\./",
						"/^editor\\.metrics\\.config\\.modal-title-diff-/",
						"/^editor\\.metrics\\.config\\.query-placeholder\\./",
						"/^editor\\.metrics\\.config\\.refresh-interval-options(-short)?\\./",
						"/^editor\\.metrics\\.config\\.time-range-options\\./",
						"/^editor\\.metrics\\.config\\.unit-options\\./",
						"/^editor\\.metrics\\.simulation\\.preset-options\\./",
						"/^editor\\.navbar\\.document-modes\\./",
						"/^settings\\.action-modals\\.data-source-removal\\.title\\./",
						"/^settings\\.action-modals\\.data-source-upsert\\./",
						"/^settings\\.data-sources\\./",
						// literal keys kept in data structures and passed to
						// t() later (the rule only tracks direct call sites)
						"/^editor\\.ai-chat\\.tool-status\\./",
						"/^editor\\.hooks\\.[a-z-]+\\.existing-item-(block|full-document)-explanation$/",
						"/^shortcuts\\.keys\\./",
						"editor.icon-stack.more",
						"editor.metrics.config.data-source-label",
						"onboarding.verify-email.title",
						"onboarding.verify-email.sent-title",
					],
				},
			],
		},
	},

	// shadcn-vue components are vendored (regenerated by the shadcn CLI) —
	// their hardcoded sr-only strings are not project translation targets
	{
		name: "oxynote/vue-i18n/shadcn",
		files: ["app/components/shadcn/ui/**"],
		rules: {
			"@intlify/vue-i18n/no-raw-text": "off",
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
			// an assignment must be its own statement — no chaining and no
			// assigning from inside an expression, e.g.
			// `const g = (map[k] ??= {})`
			"no-multi-assign": ["error", { ignoreNonDeclaration: false }],
		},
	},
])
