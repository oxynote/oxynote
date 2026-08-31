import * as Sentry from "@sentry/nuxt"

// https://docs.sentry.io/platforms/javascript/guides/nuxt/manual-setup/
Sentry.init({
	// Since Sentry on the server side needs to be loaded before
	// useRuntimeConfig() is fully available, environment variables are
	// only accessible via process.env.
	dsn: process.env.NUXT_PUBLIC_SENTRY_DSN,

	// Enable logs to be sent to Sentry
	enableLogs: true,

	// every category the SDK may attach, stated in full: an omitted field
	// takes a default that collects. Errors and stack traces only, since
	// anything else here would carry document content.
	dataCollection: {
		userInfo: false,
		cookies: false,
		httpHeaders: { request: false, response: false },
		httpBodies: [],
		urlQueryParams: false,
		genAI: { inputs: false, outputs: false },
		databaseQueryData: false,
		stackFrameVariables: false,
	},

	// Setting this option to true will print useful information to the console while you're setting up Sentry.
	debug: false,
})
