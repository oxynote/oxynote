// shared helpers for the api composable test suites in this directory.
// Deliberately not re-exported from app/composables/index.ts: nested
// composables/ files only enter the app's auto-imports through that
// re-export, so nothing here leaks into the app bundle.
import { registerEndpoint } from "@nuxt/test-utils/runtime"
import type { EntryKey } from "@pinia/colada"
import { getQuery, readBody, type H3Event } from "h3"
import { expect as staticExpect } from "vitest"

// the composables' queries inject() their option defaults, which vue only
// allows inside a component or an app context — runWithContext provides
// the latter. The assertion is for eslint's ts program, which resolves
// calls through runWithContext as error typed; vue-tsc infers them fine.
export function runInApp<T>(fn: () => T): T {
	return useNuxtApp().runWithContext(fn) as T
}

// isXid() only checks for a 20-character length; padding a short prefix
// makes ids that pass the mutations' id guards
export function makeXid(prefix: string): string {
	return prefix.padEnd(20, "0")
}

export interface RecordedCall {
	query: Record<string, unknown>
	body: unknown
}

const disposers: (() => void)[] = []

// registers a request handler on the test-time h3 app that the real api
// clients route through, recording each call for accounting. The returned
// array fills up as requests arrive. The raw event is passed through for
// responders that need to shape the response beyond a body (e.g.
// setResponseStatus).
export function mockEndpoint(
	method: "GET" | "POST" | "PUT" | "DELETE",
	url: string,
	respond: (call: RecordedCall, event: H3Event) => unknown,
): RecordedCall[] {
	const calls: RecordedCall[] = []

	const dispose = registerEndpoint(url, {
		method,
		handler: async (event: H3Event) => {
			const call: RecordedCall = {
				query: getQuery(event),
				body:
					method === "POST" || method === "PUT"
						? await readBody(event)
						: undefined,
			}

			calls.push(call)

			return respond(call, event)
		},
	})

	disposers.push(dispose)

	return calls
}

// packages the deferred-response pattern used to inspect optimistic cache
// state while the request is held in flight
export function mockDeferredEndpoint(
	method: "GET" | "POST" | "PUT" | "DELETE",
	url: string,
) {
	let resolveRequest: (value: unknown) => void = () => undefined
	let rejectRequest: (err: unknown) => void = () => undefined
	let requestReached: () => void = () => undefined
	const reached = new Promise<void>((resolve) => {
		requestReached = resolve
	})

	const calls = mockEndpoint(method, url, () => {
		requestReached()

		return new Promise((resolve, reject) => {
			resolveRequest = resolve
			rejectRequest = reject
		})
	})

	return {
		calls,
		reached,
		resolve: (value: unknown) => {
			resolveRequest(value)
		},
		reject: (err: unknown) => {
			rejectRequest(err)
		},
	}
}

// folds a custom registerEndpoint helper's dispose function into
// disposeMockEndpoints' cleanup
export function trackEndpointDisposal(dispose: () => void) {
	disposers.push(dispose)
}

// removes every tracked handler registration — call from an afterEach so
// registrations never leak between tests
export function disposeMockEndpoints() {
	disposers.splice(0).forEach((dispose) => {
		dispose()
	})
}

// queries with equal keys share app-wide cache entries between tests —
// dropping every entry from a beforeEach gives each test a fresh cache
export function clearQueryCache() {
	runInApp(() => {
		const queryCache = useQueryCache()

		queryCache.getEntries().forEach((entry) => {
			queryCache.remove(entry)
		})
	})
}

export function seedQueryData(key: EntryKey, data: unknown) {
	runInApp(() => {
		useQueryCache().setQueryData(key, data)
	})
}

export function readQueryData(key: EntryKey): unknown {
	return runInApp(() => useQueryCache().getQueryData(key))
}

// the api composables read the auth session and organization through
// useAuthSession's cache entries; seeding them fresh BEFORE a composable
// is created keeps the eager auth loads from firing and the seeds intact
export function seedAuthSession(userId: string) {
	seedQueryData(["auth", "session"], {
		data: { user: { id: userId } },
		error: null,
	})
}

export function seedAuthOrganization(organizationId: string) {
	seedQueryData(["auth", "organization"], {
		data: { id: organizationId },
		error: null,
	})
}

// asymmetric matchers are typed `any`, which the type-aware lint rejects
// inside object literals; typing them here keeps the assertions unchanged
export const ANY_STRING = staticExpect.any(String) as unknown as string
export const ANY_DATE = staticExpect.any(Date) as unknown as Date

export function matchingString(pattern: RegExp): string {
	return staticExpect.stringMatching(pattern) as string
}
