import { describe, expect, it, vi } from "vitest"

const { createAuthClientMock, electronClientMock, organizationClientMock } =
	vi.hoisted(() => ({
		createAuthClientMock: vi.fn((_config: unknown) => ({ marker: "client" })),
		electronClientMock: vi.fn((_options: unknown) => ({
			marker: "electron-plugin",
		})),
		organizationClientMock: vi.fn(() => ({ marker: "organization-plugin" })),
	}))

const { storeGetMock, storeSetMock, storeCtorMock } = vi.hoisted(() => ({
	storeGetMock: vi.fn(),
	storeSetMock: vi.fn(),
	storeCtorMock: vi.fn(),
}))

vi.mock("better-auth/client", () => ({
	createAuthClient: createAuthClientMock,
}))
vi.mock("@better-auth/electron/client", () => ({
	electronClient: electronClientMock,
}))
vi.mock("better-auth/client/plugins", () => ({
	organizationClient: organizationClientMock,
}))
vi.mock("electron-store", () => ({
	default: class Store {
		get = storeGetMock
		set = storeSetMock

		constructor(options: unknown) {
			storeCtorMock(options)
		}
	},
}))

describe("authClient", () => {
	// a single test because the unit's one act is its module evaluation:
	// importing it a second time would hit the module cache, so every
	// wiring facet is asserted against the same import
	it("wires the auth client from build-time config and electron storage", async () => {
		await import("./auth-client")

		expect(storeCtorMock).toHaveBeenCalledTimes(1)
		expect(storeCtorMock).toHaveBeenCalledWith({ name: "oxynote-auth" })

		expect(organizationClientMock).toHaveBeenCalledTimes(1)
		expect(electronClientMock).toHaveBeenCalledTimes(1)

		expect(createAuthClientMock).toHaveBeenCalledTimes(1)
		const config = createAuthClientMock.mock.calls[0]?.[0] as {
			baseURL: string
			plugins: unknown[]
		}
		expect(config.baseURL).toBe("http://test.local/core/api/auth")
		expect(config.plugins).toEqual([
			{ marker: "organization-plugin" },
			{ marker: "electron-plugin" },
		])

		const electronOptions = electronClientMock.mock.calls[0]?.[0] as {
			signInURL: string
			protocol: { scheme: string }
			storagePrefix: string
			cookiePrefix: string
			storage: {
				getItem: (key: string) => unknown
				setItem: (key: string, value: string) => void
			}
		}
		expect(electronOptions.signInURL).toBe("http://test.local/login")
		expect(electronOptions.protocol).toEqual({ scheme: "oxynote" })
		expect(electronOptions.storagePrefix).toBe("auth")
		expect(electronOptions.cookiePrefix).toBe("auth")

		// the storage adapter delegates to electron-store and normalizes
		// missing values to null
		storeGetMock.mockReturnValue("stored-value")
		expect(electronOptions.storage.getItem("k1")).toBe("stored-value")
		expect(storeGetMock).toHaveBeenCalledWith("k1")

		storeGetMock.mockReturnValue(undefined)
		expect(electronOptions.storage.getItem("k2")).toBeNull()

		electronOptions.storage.setItem("k3", "v3")
		expect(storeSetMock).toHaveBeenCalledTimes(1)
		expect(storeSetMock).toHaveBeenCalledWith("k3", "v3")
	})
})
