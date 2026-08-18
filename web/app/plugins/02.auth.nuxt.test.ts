import { beforeEach, describe, it, vi } from "vitest"
import plugin from "./02.auth"

interface CapturedAuthClientOptions {
	baseURL?: string
	fetchOptions?: {
		headers?: Record<string, string>
		credentials?: RequestCredentials
	}
	plugins?: unknown[]
}

const { authClientStub, createAuthClientMock } = vi.hoisted(() => {
	// getSession must exist because the websocket plugin refreshes the
	// session query while the test app bootstraps with this module mock in
	// place
	const authClientStub = {
		getSession: vi.fn(() => Promise.resolve({ data: null, error: null })),
	}

	return {
		authClientStub,
		createAuthClientMock: vi.fn(
			(_options: CapturedAuthClientOptions) => authClientStub,
		),
	}
})

vi.mock("better-auth/vue", () => {
	return {
		createAuthClient: createAuthClientMock,
	}
})

// the tests assert call accounting on a shared module-level mock, so they
// cannot interleave
describe("02.auth", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		createAuthClientMock.mockReset()
		authClientStub.getSession.mockReset()
	})

	it("provides an auth client configured for the public auth api", ({
		expect,
	}) => {
		// the plugin type is a union with the void/promise setup variants;
		// this object-form setup synchronously returns its provide block
		const result = plugin(useNuxtApp()) as unknown as {
			provide: { authClient: unknown }
		}

		expect(createAuthClientMock).toHaveBeenCalledTimes(1)

		const options = createAuthClientMock.mock.calls[0]?.[0]
		expect(options?.baseURL).toBe("http://test.local/auth-realtime/api/auth")
		expect(options?.fetchOptions?.credentials).toBe("include")
		expect(options?.fetchOptions?.headers).toBeUndefined()
		expect(options?.plugins).toHaveLength(2)
		expect(result.provide.authClient).toBe(authClientStub)
	})
})
