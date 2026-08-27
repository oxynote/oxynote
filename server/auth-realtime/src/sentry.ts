import * as Sentry from "@sentry/node"

if (process.env.SENTRY_DSN) {
	Sentry.init({
		dsn: process.env.SENTRY_DSN,
		environment: process.env.NODE_ENV ?? "development",
		// tracing stays off, deliberately and unconfigurably: the
		// production bundle folds sentry into the app, which loses the
		// require hooks tracing relies on, and error capture is the
		// only thing this service uses.
		tracesSampleRate: 0,
	})
}
