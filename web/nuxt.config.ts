import tailwindcss from "@tailwindcss/vite"
import { selectableIconList } from "./app/utils/icon"

const lezerPromQLPath = new URL(
	"./packages/lezer-promql/dist/index.es.js",
	import.meta.url,
).pathname

// DESKTOP_BUILD selects how `__DESKTOP_BUILD__` is materialized in the bundle:
//   - "1"      : pure desktop build (production renderer). __DESKTOP_BUILD__ is
//                the literal `true` and web branches DCE away.
//   - "hybrid" : dev-only, used by `start:dev:desktop`. One Nuxt dev server is
//                hit by both the electron renderer and the system browser
//                opened for OAuth. Both branches stay in the bundle and
//                __DESKTOP_BUILD__ becomes a runtime probe for the preload
//                bridges so each context picks the right path.
//   - "0"      : pure web build. __DESKTOP_BUILD__ is the literal `false`.
//   - unset    : same as "0".
const desktopBuildMode = (process.env.DESKTOP_BUILD ?? "0") as
	"0" | "1" | "hybrid"
const isDesktopBuild = desktopBuildMode === "1" || desktopBuildMode === "hybrid"
const isHybridDesktopBuild = desktopBuildMode === "hybrid"

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
	compatibilityDate: "2025-07-15",
	ssr: !isDesktopBuild,

	vite: {
		define: {
			// In pure builds Vite substitutes a literal boolean so unused
			// branches DCE away. In hybrid mode the substituted text is a
			// runtime probe of `window.__isElectron` (set by the preload), so
			// the electron renderer and the system browser served by the same
			// dev server each take the right branch from the same bundle.
			__DESKTOP_BUILD__: isHybridDesktopBuild
				? `(typeof window !== "undefined" && window.__isElectron === true)`
				: JSON.stringify(isDesktopBuild),
		},
		plugins: [tailwindcss()],
		resolve: {
			alias: {
				"@prometheus-io/lezer-promql": lezerPromQLPath,
			},
		},
		optimizeDeps: {
			include: [
				"@better-auth/electron/proxy",
				"@pinia/colada-plugin-auto-refetch",
				"@tiptap/core",
				"@tiptap/extension-file-handler",
				"@tiptap/extension-list",
				"@tiptap/vue-3",
				"better-auth/client/plugins",
				"better-auth/vue",
				"class-variance-authority",
				"clsx",
				"fast-deep-equal",
				"nanoid",
				"reka-ui",
				"safe-stable-stringify",
				"slugify",
				"tailwind-merge",
				"vue-sonner",
			],
		},
	},

	nitro: {
		// https://content.nuxt.com/docs/deploy/cloudflare-pages
		// NITRO_PRESET only applies to web builds — e.g. `node-server` for the
		// self-hosted docker image. Desktop builds are always static.
		preset: isDesktopBuild
			? "static"
			: (process.env.NITRO_PRESET ?? "cloudflare_pages"),
	},

	modules: [
		"@nuxt/eslint",
		"@nuxt/fonts",
		"@nuxt/icon",
		"shadcn-nuxt",
		"@vueuse/nuxt",
		"@nuxtjs/i18n",
		"@pinia/nuxt",
		"@pinia/colada-nuxt",
		"@sentry/nuxt/module",
	],

	css: [
		"~/assets/css/main.css",
		"vue-sonner/style.css",
		"vue-virtual-scroller/dist/vue-virtual-scroller.css",
	],

	devtools: { enabled: false },

	eslint: {
		config: {
			typescript: {
				tsconfigPath: "tsconfig.json",
			},
		},
	},

	typescript: {
		typeCheck: true,
		tsConfig: {
			compilerOptions: {
				sourceMap: true,
				removeComments: true,
				noUnusedLocals: true,
				noUnusedParameters: true,
				noFallthroughCasesInSwitch: true,
				noImplicitAny: true,
				forceConsistentCasingInFileNames: true,
				noUncheckedIndexedAccess: true,
				noImplicitOverride: true,
				verbatimModuleSyntax: true,
				allowUnreachableCode: false,
			},
		},
	},

	runtimeConfig: {
		// server-only SSR overrides. When the app runs inside a container, the
		// public localhost URLs are unreachable from the server side — these
		// point SSR fetches at the backend services directly and fall back to
		// the public URLs when unset.
		coreAPIInternalHttpURL: "",
		authRealtimeAPIInternalHttpURL: "",
		public: {
			sentryDSN: "",
			appBaseURL: "",
			linkToMoreInfoAboutProduct: "",
			authRealtimeAPIBaseHttpURL: "",
			authRealtimeAPIBaseWsURL: "",
			coreAPIBaseHttpURL: "",
			coreAPIBaseWsURL: "",
			termsOfServiceURL: "",
			privacyPolicyURL: "",
			postgresqlReadOnlyUserSetupGuideURL: "",
			mysqlReadOnlyUserSetupGuideURL: "",
			mariadbReadOnlyUserSetupGuideURL: "",
			prometheusQueryGuideURL: "",
			postgresqlQueryGuideURL: "",
			mysqlQueryGuideURL: "",
			mariadbQueryGuideURL: "",
		},
	},

	app: {
		head: {
			title: "Oxynote", // default title (can be overridden in pages/components)
			meta: [{ name: "apple-mobile-web-app-title", content: "Oxynote" }],
			link: [
				{
					rel: "icon",
					type: "image/png",
					href: "/favicon-96x96.png",
					sizes: "96x96",
				},
				{ rel: "icon", type: "image/svg+xml", href: "/favicon.svg" },
				{ rel: "shortcut icon", href: "/favicon.ico" },
				{
					rel: "apple-touch-icon",
					sizes: "180x180",
					href: "/apple-touch-icon.png",
				},
				{ rel: "manifest", href: "/site.webmanifest" },
			],
		},
	},

	fonts: {
		defaults: {
			weights: [400, 500, 600, 700],
		},
	},

	icon: {
		mode: "css",
		cssLayer: "base",
		clientBundle: {
			icons: selectableIconList(),
			scan: true,
			sizeLimitKb: 256,
		},
		customCollections: [
			{
				prefix: "custom-icons",
				dir: "./app/assets/custom-icons",
			},
		],
	},

	i18n: {
		defaultLocale: "en",
		strategy: "no_prefix",
		locales: [
			{
				code: "en",
				language: "en-US",
				// all files are combined in one big i18n file, so all of them
				// need to have a root namespace object, like: "sidebar: {...}"
				files: [
					"en/general.json",
					"en/sidebar.json",
					"en/editor.json",
					"en/onboarding.json",
					"en/settings.json",
					"en/shortcuts.json",
					"en/notification.json",
					"en/apps.json",
					"en/oauth.json",
				],
			},
		],
	},

	shadcn: {
		prefix: "ShadcnUi",
		// https://github.com/unovue/shadcn-vue/issues/763#issuecomment-2349290087
		componentDir: "./app/components/shadcn/ui",
	},

	sentry: {
		org: "oxynote",
		project: "bifrost",
		authToken: process.env.SENTRY_AUTH_TOKEN,
		telemetry: false,
	},

	// hidden sourcemaps exist only for the Sentry upload. Skip generating
	// them when no auth token is present (e.g. self-hosted docker builds) —
	// they roughly double the build's memory footprint.
	sourcemap: process.env.SENTRY_AUTH_TOKEN
		? {
				client: "hidden",
				server: "hidden",
			}
		: false,
})
