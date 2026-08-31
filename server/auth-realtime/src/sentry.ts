import * as Sentry from "@sentry/node"

if (process.env.SENTRY_DSN) {
	Sentry.init({
		dsn: process.env.SENTRY_DSN,
		environment: process.env.NODE_ENV ?? "development",
		// every category the SDK may attach, stated in full: an
		// omitted field takes a default that collects. Errors and
		// stack traces only, since anything else here would carry
		// document content.
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
		// tracing stays off, deliberately and unconfigurably: the
		// production bundle folds sentry into the app, which loses the
		// require hooks tracing relies on, and error capture is the
		// only thing this service uses.
		tracesSampleRate: 0,
	})
}
