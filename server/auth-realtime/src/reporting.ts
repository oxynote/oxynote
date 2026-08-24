import * as Sentry from "@sentry/node"

// runs work whose failure the caller must still see, reporting the error on
// its way out. The frameworks this service hands callbacks to swallow the
// cause — better-auth turns a rejected hook into a failed request and
// hocuspocus into a refused connection — so without this the only trace of
// what went wrong is a status code.
export async function reported<T>(run: () => Promise<T>): Promise<T> {
	try {
		return await run()
	} catch (err) {
		Sentry.captureException(err)
		throw err
	}
}

// runs an effect whose failure must not fail the caller: the work it
// belongs to has already succeeded and the caller has nothing useful to do
// about it. The error is reported and swallowed, so the failure is visible
// without being fatal.
export async function bestEffort(run: () => unknown): Promise<void> {
	try {
		await run()
	} catch (err) {
		Sentry.captureException(err)
	}
}
