import { describe, it, vi } from "vitest"
import { createCrashReporter, type SentryClient } from "./sentry.js"

type InitOptions = Parameters<SentryClient["init"]>[0]

function harness(overrides: { flush?: () => Promise<boolean> } = {}): {
	init: ReturnType<typeof vi.fn>
	captureException: ReturnType<typeof vi.fn>
	flush: ReturnType<typeof vi.fn>
	client: SentryClient
	order: string[]
} {
	const order: string[] = []

	const init = vi.fn<(options: InitOptions) => void>()
	const captureException = vi.fn<(error: unknown) => void>(() => {
		order.push("capture")
	})
	const flush = vi.fn<(timeout: number) => Promise<boolean>>(() => {
		order.push("flush")

		return overrides.flush?.() ?? Promise.resolve(true)
	})

	return {
		init,
		captureException,
		flush,
		client: { init, captureException, flush },
		order,
	}
}

describe("createCrashReporter", () => {
	it("initializes sentry with the baked dsn, no tracing and no data collection", ({
		expect,
	}) => {
		const h = harness()

		createCrashReporter(
			h.client,
			"https://key@sentry.test/1",
			false,
		)

		expect(h.init).toHaveBeenCalledWith({
			dsn: "https://key@sentry.test/1",
			environment: "production",
			tracesSampleRate: 0,
			dataCollection: {
				userInfo: false,
				cookies: false,
				httpHeaders: {
					request: false,
					response: false,
				},
				httpBodies: [],
				urlQueryParams: false,
				genAI: { inputs: false, outputs: false },
				databaseQueryData: false,
				stackFrameVariables: false,
			},
		})
	})

	it.for([
		{
			name: "the image was built without a dsn",
			input: { dsn: "", disabled: false },
		},
		{
			name: "the operator turned reporting off",
			input: {
				dsn: "https://key@sentry.test/1",
				disabled: true,
			},
		},
	])("stays inert when $name", async ({ input }, { expect }) => {
		const h = harness()

		await createCrashReporter(
			h.client,
			input.dsn,
			input.disabled,
		).report(new Error("boom"))

		expect(h.init).not.toHaveBeenCalled()
		expect(h.captureException).not.toHaveBeenCalled()
	})

	it("captures the error and flushes before resolving", async ({
		expect,
	}) => {
		const h = harness()
		const error = new Error("boom")

		await createCrashReporter(
			h.client,
			"https://key@sentry.test/1",
			false,
		).report(error)

		expect(h.captureException).toHaveBeenCalledWith(error)
		// the callers all exit immediately afterwards, so a report that
		// had not left the process by now would never be sent.
		expect(h.order).toStrictEqual(["capture", "flush"])
	})

	it("resolves when flushing fails, so a crash never becomes a hang", async ({
		expect,
	}) => {
		const h = harness({
			flush: () => Promise.reject(new Error("offline")),
		})

		await expect(
			createCrashReporter(
				h.client,
				"https://key@sentry.test/1",
				false,
			).report(new Error("boom")),
		).resolves.toBeUndefined()
	})
})
