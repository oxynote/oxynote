import * as Sentry from "@sentry/nuxt"

// https://docs.sentry.io/platforms/javascript/guides/nuxt/manual-setup/
Sentry.init({
	// Since Sentry on the server side needs to be loaded before
	// useRuntimeConfig() is fully available, environment variables are
	// only accessible via process.env.
	dsn: process.env.NUXT_PUBLIC_SENTRY_DSN,

	// Enable logs to be sent to Sentry
	enableLogs: true,

	// Enable sending of user PII (Personally Identifiable Information)
	// https://docs.sentry.io/platforms/javascript/guides/nuxt/configuration/options/#sendDefaultPii
	sendDefaultPii: false,

	// Setting this option to true will print useful information to the console while you're setting up Sentry.
	debug: false,
})
