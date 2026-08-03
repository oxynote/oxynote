import * as Sentry from "@sentry/nuxt"

// https://docs.sentry.io/platforms/javascript/guides/nuxt/manual-setup/
Sentry.init({
	dsn: useRuntimeConfig().public.sentryDSN,

	// Enable logs to be sent to Sentry
	enableLogs: true,

	// Enable sending of user PII (Personally Identifiable Information)
	// https://docs.sentry.io/platforms/javascript/guides/nuxt/configuration/options/#sendDefaultPii
	sendDefaultPii: false,

	// Setting this option to true will print useful information to the console while you're setting up Sentry.
	debug: false,
})
