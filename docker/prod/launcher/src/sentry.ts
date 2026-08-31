// the launcher's own crash reporting. Every other process in the image
// reports for itself — core, auth-realtime and web each carry their own DSN
// — and this covers the one thing none of them can: the supervisor's view.
// A child killed by the OOM killer, a boot that fails before any child
// starts, an invalid environment that stops the container from serving at
// all: in each case the process that would have reported is dead or was
// never alive, and without this the operator sees only a container that
// exited.

// the categories the SDK collects, as passed below. Spelled out here
// rather than imported so the injected client stays a structural type a
// test can satisfy with three functions.
export interface DataCollection {
	userInfo: boolean
	cookies: boolean
	httpHeaders: { request: boolean; response: boolean }
	httpBodies: never[]
	urlQueryParams: boolean
	genAI: { inputs: boolean; outputs: boolean }
	databaseQueryData: boolean
	stackFrameVariables: boolean
}

export interface SentryClient {
	init(options: {
		dsn: string
		environment: string
		tracesSampleRate: number
		dataCollection: DataCollection
	}): void
	captureException(error: unknown): void
	flush(timeout: number): Promise<boolean>
}

export interface CrashReporter {
	// report captures an error and waits for it to leave the process.
	// Every caller exits immediately afterwards, so a fire-and-forget
	// capture would be discarded before it was ever sent.
	report(error: unknown): Promise<void>
}

// how long a report may take before the shutdown it precedes goes ahead
// anyway. A crash must not become a hang because the network is down.
const flushTimeoutMs = 2_000

const noop: CrashReporter = {
	report: () => Promise.resolve(),
}

// createCrashReporter returns a reporter that is inert unless the image was
// built with a DSN and the operator has left reporting on.
//
// The disabled flag is read from the raw environment by the caller rather
// than from the parsed Config, and deliberately: config parsing is itself
// one of the failures worth reporting, so the reporter has to exist before
// there is a Config to consult. The flag is a plain "true"/"false" string,
// so reading it twice cannot disagree.
export function createCrashReporter(
	client: SentryClient,
	dsn: string,
	disabled: boolean,
): CrashReporter {
	if (dsn === "" || disabled) {
		return noop
	}

	client.init({
		dsn,
		environment: "production",
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
		// tracing stays off, as in auth-realtime: the bundle folds
		// sentry in, which loses the require hooks tracing relies on,
		// and error capture is the only thing this process wants.
		tracesSampleRate: 0,
	})

	return {
		report: async (error) => {
			try {
				client.captureException(error)
				await client.flush(flushTimeoutMs)
			} catch {
				// a reporter that throws would replace the crash
				// being reported with one about reporting it.
			}
		},
	}
}
