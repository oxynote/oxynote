import { beforeEach, describe, it, vi } from "vitest"
import * as Sentry from "@sentry/node"
import { bestEffort, reported } from "./reporting.js"

vi.mock("@sentry/node", () => ({
	captureException: vi.fn(),
}))

const captureException = vi.mocked(Sentry.captureException)

// the sentry module mock is a per-file singleton, so call-count assertions
// on it cannot be isolated across interleaving tests. vitest 4's
// restoreMocks does not reset a hand-made vi.fn(), so it is cleared here.
describe("reported", { concurrent: false }, () => {
	beforeEach(() => {
		captureException.mockClear()
	})

	it("returns what the work resolved to", async ({ expect }) => {
		expect(await reported(() => Promise.resolve("value"))).toBe(
			"value",
		)
		expect(captureException).toHaveBeenCalledTimes(0)
	})

	it("reports the failure and lets the caller see it", async ({
		expect,
	}) => {
		const failure = new Error("core unreachable")

		await expect(
			reported(() => Promise.reject(failure)),
		).rejects.toBe(failure)
		expect(captureException).toHaveBeenCalledTimes(1)
		expect(captureException).toHaveBeenCalledWith(failure)
	})

	// axios rejects with its own error shape rather than an Error, and
	// nothing here inspects the value on its way past
	it("reports a rejection that is not an Error", async ({ expect }) => {
		const rejection = { response: { status: 500 } }
		const run = vi.fn().mockRejectedValue(rejection)

		await expect(reported(run)).rejects.toBe(rejection)
		expect(captureException).toHaveBeenCalledWith(rejection)
	})

	it("reports a synchronous throw from the work itself", async ({
		expect,
	}) => {
		const failure = new Error("built the request wrong")

		await expect(
			reported(() => {
				throw failure
			}),
		).rejects.toBe(failure)
		expect(captureException).toHaveBeenCalledTimes(1)
	})
})

describe("bestEffort", { concurrent: false }, () => {
	beforeEach(() => {
		captureException.mockClear()
	})

	it("runs the effect and reports nothing when it succeeds", async ({
		expect,
	}) => {
		const effect = vi.fn().mockResolvedValue(undefined)

		await bestEffort(effect)

		expect(effect).toHaveBeenCalledTimes(1)
		expect(captureException).toHaveBeenCalledTimes(0)
	})

	// the work this effect belongs to has already succeeded, so a failure
	// here must stay visible without reaching the caller
	it("reports the failure and resolves anyway", async ({ expect }) => {
		const failure = new Error("core unreachable")

		await expect(
			bestEffort(() => Promise.reject(failure)),
		).resolves.toBeUndefined()
		expect(captureException).toHaveBeenCalledWith(failure)
	})

	it("reports a synchronous throw from the effect itself", async ({
		expect,
	}) => {
		const failure = new Error("nothing to disconnect")

		await expect(
			bestEffort(() => {
				throw failure
			}),
		).resolves.toBeUndefined()
		expect(captureException).toHaveBeenCalledWith(failure)
	})

	// hocuspocus's disconnect is declared as returning void or a promise,
	// so the helper has to accept an effect that returns neither
	it("accepts an effect that returns nothing", async ({ expect }) => {
		const effect = vi.fn()

		await expect(bestEffort(effect)).resolves.toBeUndefined()
		expect(captureException).toHaveBeenCalledTimes(0)
	})
})
