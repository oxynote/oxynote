import type { SentryDsns } from "./mapping.js"

// the central telemetry DSNs are injected here by esbuild --define at image
// build time; a build without them (local builds, tests) falls back to
// empty, which disables sentry entirely. A DSN is a write-only ingest
// address, safe to carry in a public image, and
// OXYNOTE_CRASH_REPORTING_DISABLED switches reporting off at runtime.
declare const __SENTRY_WEB_DSN__: string | undefined
declare const __SENTRY_CORE_DSN__: string | undefined
declare const __SENTRY_AUTH_REALTIME_DSN__: string | undefined
declare const __SENTRY_LAUNCHER_DSN__: string | undefined

export const bakedSentryDsns: SentryDsns = {
	webDsn:
		typeof __SENTRY_WEB_DSN__ === "string"
			? __SENTRY_WEB_DSN__
			: "",
	coreDsn:
		typeof __SENTRY_CORE_DSN__ === "string"
			? __SENTRY_CORE_DSN__
			: "",
	authRealtimeDsn:
		typeof __SENTRY_AUTH_REALTIME_DSN__ === "string"
			? __SENTRY_AUTH_REALTIME_DSN__
			: "",
}

// the launcher's own, kept out of SentryDsns because that bag is what gets
// handed to the children — this one is never passed to anybody.
export const bakedLauncherSentryDsn =
	typeof __SENTRY_LAUNCHER_DSN__ === "string"
		? __SENTRY_LAUNCHER_DSN__
		: ""
